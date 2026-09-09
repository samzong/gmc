package rustcache

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

const ignoreContent = "/config.toml\n/.gitignore\n"

func Eligible(root string) (bool, string) {
	info, err := os.Lstat(filepath.Join(root, "Cargo.toml"))
	if err != nil || !info.Mode().IsRegular() {
		return false, ""
	}
	if _, err := os.Lstat(filepath.Join(root, ".cargo")); !errors.Is(err, os.ErrNotExist) {
		return false, "existing .cargo directory or file"
	}
	for _, key := range []string{
		"RUSTC_WRAPPER", "RUSTC_WORKSPACE_WRAPPER", "CARGO_BUILD_RUSTC_WRAPPER", "CARGO_BUILD_RUSTC_WORKSPACE_WRAPPER",
	} {
		if _, ok := os.LookupEnv(key); ok {
			return false, "existing " + key
		}
	}
	dirs := []string{}
	for dir := filepath.Dir(root); ; dir = filepath.Dir(dir) {
		dirs = append(dirs, filepath.Join(dir, ".cargo"))
		if filepath.Dir(dir) == dir {
			break
		}
	}
	home := os.Getenv("CARGO_HOME")
	if home != "" && !filepath.IsAbs(home) {
		return false, "relative CARGO_HOME"
	}
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return false, "cannot resolve Cargo home"
		}
		home = filepath.Join(userHome, ".cargo")
	}
	dirs = append(dirs, home)
	for _, dir := range dirs {
		for _, name := range []string{"config", "config.toml"} {
			path := filepath.Join(dir, name)
			info, err := os.Lstat(path)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				return false, "cannot inspect inherited Cargo configuration"
			}
			data, err := os.ReadFile(path)
			var cfg map[string]any
			if err != nil || toml.Unmarshal(data, &cfg) != nil {
				return false, "cannot inspect inherited Cargo configuration"
			}
			if _, ok := cfg["include"]; ok {
				return false, "inherited Cargo configuration includes other files"
			}
			if build, ok := cfg["build"].(map[string]any); ok {
				for _, key := range []string{"rustc-wrapper", "rustc-workspace-wrapper"} {
					if _, ok := build[key]; ok {
						return false, "inherited Cargo " + key
					}
				}
			}
		}
	}
	return true, ""
}

func Enable(root, helper string) error {
	data, err := toml.Marshal(map[string]any{"build": map[string]string{"rustc-wrapper": helper}})
	if err != nil {
		return err
	}
	cargo := filepath.Join(root, ".cargo")
	if err := os.Mkdir(cargo, 0755); err != nil {
		return err
	}
	configPath := filepath.Join(cargo, "config.toml")
	if err := exclusiveWrite(configPath, data); err != nil {
		_ = os.Remove(cargo)
		return err
	}
	if err := exclusiveWrite(filepath.Join(cargo, ".gitignore"), []byte(ignoreContent)); err != nil {
		_ = removeMatching(configPath, data)
		_ = os.Remove(cargo)
		return err
	}
	return nil
}

func exclusiveWrite(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return errors.Join(err, os.Remove(path))
	}
	return nil
}

func removeMatching(path string, expected []byte) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("kept changed %s; remove the gmc cache configuration manually", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, expected) {
		return fmt.Errorf("kept edited %s; remove the gmc cache configuration manually", path)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
