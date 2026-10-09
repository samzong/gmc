//go:build windows

package worktree

import (
	"encoding/binary"
	"io/fs"
	"unsafe"

	"golang.org/x/sys/windows"
)

type fileStandardInfo struct {
	AllocationSize int64
	EndOfFile      int64
	NumberOfLinks  uint32
	DeletePending  uint8
	Directory      uint8
}

type fileIDInfo struct {
	VolumeSerialNumber uint64
	FileID             [16]byte
}

func statAllocated(path string, _ fs.FileInfo) (allocatedEntry, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return allocatedEntry{}, err
	}
	handle, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return allocatedEntry{}, err
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	var standard fileStandardInfo
	if err := windows.GetFileInformationByHandleEx(handle, windows.FileStandardInfo,
		(*byte)(unsafe.Pointer(&standard)), uint32(unsafe.Sizeof(standard))); err != nil {
		return allocatedEntry{}, err
	}
	if standard.AllocationSize < 0 {
		return allocatedEntry{}, errAllocatedOverflow
	}
	var identity fileIDInfo
	if err := windows.GetFileInformationByHandleEx(handle, windows.FileIdInfo,
		(*byte)(unsafe.Pointer(&identity)), uint32(unsafe.Sizeof(identity))); err != nil {
		return allocatedEntry{}, err
	}
	return allocatedEntry{
		bytes: uint64(standard.AllocationSize),
		id: fileID{
			volume: identity.VolumeSerialNumber,
			index: [2]uint64{
				binary.LittleEndian.Uint64(identity.FileID[8:]),
				binary.LittleEndian.Uint64(identity.FileID[:8]),
			},
		},
		hardlinks: standard.NumberOfLinks > 1 && standard.Directory == 0,
	}, nil
}
