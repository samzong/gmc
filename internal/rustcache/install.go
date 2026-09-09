package rustcache

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const Version = "0.18.0"

type release struct {
	name string
	sha  string
}

var releases = map[string]release{
	"darwin/arm64": {
		"kache-aarch64-apple-darwin.tar.gz",
		"48decca430d16cb84a9cd055f7a352e35b3d8d18bace646be8eda734f516ea74",
	},
	"darwin/amd64": {
		"kache-x86_64-apple-darwin.tar.gz",
		"c96eef46bec2999d8c7393e04452926d2b3f66bebef4ff485b115c6e8fafa363",
	},
	"linux/arm64": {
		"kache-aarch64-unknown-linux-musl.tar.gz",
		"0a895ca560e17cdbf00c596a67c6af54baa907a95fac4d6cd04adb66268d7434",
	},
	"linux/amd64": {
		"kache-x86_64-unknown-linux-musl.tar.gz",
		"2209e1e4de2c49a3a732a6ba86ad9ad4502c5cdc139b53723aa1e6c33efd8662",
	},
	"windows/amd64": {
		"kache-x86_64-pc-windows-msvc.exe",
		"10b84a60e533bd58ba56cdb421f6b722f5d589aeacf8a37d9c7fa3c8b69c5d9c",
	},
}

func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func Ensure() (string, error) {
	r, ok := releases[runtime.GOOS+"/"+runtime.GOARCH]
	if !ok {
		return "", errors.New("kache is unavailable for this platform")
	}
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	root, err := filepath.Abs(filepath.Join(dataHome, "gmc", "rust-cache"))
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "kache-"+Version+"-"+runtime.GOOS+"-"+runtime.GOARCH)
	backend := filepath.Join(dir, executableName("kache"))
	if info, statErr := os.Lstat(backend); statErr != nil || !info.Mode().IsRegular() {
		if err := installRelease(dir, r); err != nil {
			return "", err
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		return "", err
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	helperDir := filepath.Join(dir, "adapter-"+hash)
	if err := os.MkdirAll(helperDir, 0700); err != nil {
		return "", err
	}
	helper := filepath.Join(helperDir, executableName("gmc-rustc"))
	if _, err := os.Lstat(helper); errors.Is(err, os.ErrNotExist) {
		if err := atomicWrite(helper, data, 0755); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	return helper, nil
}

func installRelease(dir string, r release) error {
	client := http.Client{Timeout: 30 * time.Second}
	url := "https://github.com/kunobi-ninja/kache/releases/download/v" + Version + "/" + r.name
	response, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download Kache: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download Kache: HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return err
	}
	binary, err := releaseBinary(data, r)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(dir, "config.toml"), []byte("[cache]\n"), 0600); err != nil {
		return err
	}
	return atomicWrite(filepath.Join(dir, executableName("kache")), binary, 0755)
}

func releaseBinary(data []byte, r release) ([]byte, error) {
	if fmt.Sprintf("%x", sha256.Sum256(data)) != r.sha {
		return nil, errors.New("kache download checksum mismatch")
	}
	if strings.HasSuffix(r.name, ".exe") {
		return data, nil
	}
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	archive := tar.NewReader(reader)
	for {
		header, err := archive.Next()
		if err != nil {
			return nil, fmt.Errorf("read Kache archive: %w", err)
		}
		if filepath.Base(header.Name) == "kache" && header.Typeflag == tar.TypeReg {
			if header.Size < 1 || header.Size > 64<<20 {
				return nil, errors.New("invalid Kache binary size")
			}
			return io.ReadAll(archive)
		}
	}
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".gmc-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	if err := os.Chmod(file.Name(), mode); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
