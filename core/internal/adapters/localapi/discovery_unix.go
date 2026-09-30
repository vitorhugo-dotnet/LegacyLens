//go:build !windows

package localapi

import (
	"errors"
	"os"
)

func secureUserPath(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("path is unavailable")
	}
	if directory && !info.IsDir() || !directory && !info.Mode().IsRegular() {
		return errors.New("path has the wrong type")
	}
	mode := os.FileMode(0600)
	if directory {
		mode = 0700
	}
	return os.Chmod(path, mode)
}
