package worktree

import (
	"encoding/binary"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	bulkGetattrlist    = 461
	bulkDirMountStatus = 0x4
	bulkDirAllocSize   = 0x8
	bulkDirectoryType  = 2
)

type bulkEntry struct {
	name      string
	allocated allocatedEntry
	directory bool
	fallback  bool
	err       error
}

func readBulkAttributes(fd int, buffer []byte) (int, error) {
	attrs := unix.Attrlist{
		Bitmapcount: 5,
		Commonattr: unix.ATTR_CMN_RETURNED_ATTRS | unix.ATTR_CMN_ERROR | unix.ATTR_CMN_NAME |
			unix.ATTR_CMN_DEVID | unix.ATTR_CMN_OBJTYPE | unix.ATTR_CMN_FLAGS | unix.ATTR_CMN_FILEID,
		Dirattr:  bulkDirMountStatus | bulkDirAllocSize,
		Fileattr: unix.ATTR_FILE_LINKCOUNT | unix.ATTR_FILE_ALLOCSIZE,
	}
	n, _, errno := syscall.Syscall6(bulkGetattrlist, uintptr(fd), uintptr(unsafe.Pointer(&attrs)),
		uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), unix.FSOPT_PACK_INVAL_ATTRS, 0)
	runtime.KeepAlive(&attrs)
	runtime.KeepAlive(buffer)
	if errno != 0 {
		return 0, errno
	}
	if n > uintptr(len(buffer)/4) {
		return 0, unix.EIO
	}
	return int(n), nil
}

func parseBulkEntry(buffer []byte) (bulkEntry, []byte, error) {
	if len(buffer) < 4 {
		return bulkEntry{}, nil, unix.EIO
	}
	length := uint64(binary.LittleEndian.Uint32(buffer))
	if length < 68 || length > uint64(len(buffer)) {
		return bulkEntry{}, nil, unix.EIO
	}
	record := buffer[:length]
	nameOffset := int64(int32(binary.LittleEndian.Uint32(record[28:]))) + 28
	nameLength := int64(binary.LittleEndian.Uint32(record[32:]))
	if nameOffset < 68 || nameLength < 2 || nameOffset+nameLength > int64(length) {
		return bulkEntry{}, nil, unix.EIO
	}
	name := record[nameOffset : nameOffset+nameLength]
	if name[len(name)-1] != 0 {
		return bulkEntry{}, nil, unix.EIO
	}
	entry := bulkEntry{name: string(name[:len(name)-1])}
	if entry.name == "." || entry.name == ".." || strings.ContainsAny(entry.name, "/\x00") {
		return bulkEntry{}, nil, unix.EIO
	}
	if errno := binary.LittleEndian.Uint32(record[24:]); errno != 0 {
		entry.err = syscall.Errno(errno)
		return entry, buffer[length:], nil
	}
	common := binary.LittleEndian.Uint32(record[4:])
	dirs := binary.LittleEndian.Uint32(record[12:])
	files := binary.LittleEndian.Uint32(record[16:])
	required := uint32(unix.ATTR_CMN_DEVID | unix.ATTR_CMN_OBJTYPE | unix.ATTR_CMN_FLAGS | unix.ATTR_CMN_FILEID)
	entry.directory = binary.LittleEndian.Uint32(record[40:]) == bulkDirectoryType
	entry.fallback = common&required != required || binary.LittleEndian.Uint32(record[44:])&unix.SF_FIRMLINK != 0
	if entry.directory {
		requiredDirs := uint32(bulkDirAllocSize | bulkDirMountStatus)
		mountPoint := binary.LittleEndian.Uint32(record[56:])&1 != 0
		entry.fallback = entry.fallback || dirs&requiredDirs != requiredDirs || mountPoint
	} else {
		requiredFiles := uint32(unix.ATTR_FILE_LINKCOUNT | unix.ATTR_FILE_ALLOCSIZE)
		entry.fallback = entry.fallback || files&requiredFiles != requiredFiles
	}
	entry.allocated = allocatedEntry{
		bytes: binary.LittleEndian.Uint64(record[60:]),
		id: fileID{volume: deviceID(int32(binary.LittleEndian.Uint32(record[36:]))),
			index: [2]uint64{0, binary.LittleEndian.Uint64(record[48:])}},
		hardlinks: !entry.directory && binary.LittleEndian.Uint32(record[56:]) > 1,
	}
	if int64(entry.allocated.bytes) < 0 {
		return bulkEntry{}, nil, errAllocatedOverflow
	}
	return entry, buffer[length:], nil
}
