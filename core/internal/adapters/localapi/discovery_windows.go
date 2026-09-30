//go:build windows

package localapi

import (
	"encoding/csv"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

var sidPattern = regexp.MustCompile(`^S-1-[0-9]+(?:-[0-9]+)+$`)

func secureUserPath(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("path is unavailable")
	}
	if directory && !info.IsDir() || !directory && !info.Mode().IsRegular() {
		return errors.New("path has the wrong type")
	}
	identity, err := exec.Command("whoami.exe", "/user", "/fo", "csv", "/nh").Output()
	if err != nil {
		return errors.New("current user identity is unavailable")
	}
	row, err := csv.NewReader(strings.NewReader(string(identity))).Read()
	if err != nil || len(row) < 2 {
		return errors.New("current user identity is invalid")
	}
	sid := strings.TrimSpace(row[len(row)-1])
	if !sidPattern.MatchString(sid) {
		return errors.New("current user identity is invalid")
	}
	grant := "*" + sid + ":(F)"
	if directory {
		grant = "*" + sid + ":(OI)(CI)F"
	}
	command := exec.Command("icacls.exe", path, "/inheritance:r", "/grant:r", grant)
	command.Stdout = nil
	command.Stderr = nil
	if err := command.Run(); err != nil {
		return errors.New("user-only access control could not be applied")
	}
	return nil
}
