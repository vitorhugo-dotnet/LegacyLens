package application

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	"legacylens/core/internal/domain"
)

type LocationService struct {
	projects ProjectStore
	editor   Editor
}

func NewLocationService(projects ProjectStore, editor Editor) *LocationService {
	return &LocationService{projects: projects, editor: editor}
}

func (s *LocationService) Open(ctx context.Context, projectID domain.ID, location domain.Location) (OpenResult, error) {
	if s == nil || s.projects == nil {
		return OpenResult{}, errors.New("project store is required")
	}
	if s.editor == nil {
		return OpenResult{}, errors.New("editor is required")
	}
	if err := ctx.Err(); err != nil {
		return OpenResult{}, err
	}
	project, err := s.projects.LoadProject(ctx, projectID)
	if err != nil {
		return OpenResult{}, err
	}
	if project.ID != projectID {
		return OpenResult{}, errors.New("registered project identity does not match")
	}
	relative, err := safeLocationPath(location.Path)
	if err != nil {
		return OpenResult{}, err
	}
	if location.Line < 1 {
		return OpenResult{}, errors.New("location line must be one-based")
	}
	rootPath, err := filepath.Abs(project.Root)
	if err != nil {
		return OpenResult{}, errors.New("registered project root is invalid")
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return OpenResult{}, errors.New("registered project root is unavailable")
	}
	defer root.Close()
	file, err := root.Open(relative)
	if err != nil {
		return OpenResult{}, errors.New("location does not identify a file within the project")
	}
	info, statErr := file.Stat()
	closeErr := file.Close()
	if statErr != nil || closeErr != nil || !info.Mode().IsRegular() {
		return OpenResult{}, errors.New("location does not identify a regular project file")
	}
	absPath := filepath.Join(rootPath, relative)
	return s.editor.Open(ctx, domain.Location{Path: absPath, Line: location.Line, Column: location.Column})
}

func safeLocationPath(value string) (string, error) {
	if value == "" || strings.ContainsRune(value, '\\') || strings.ContainsRune(value, 0) || path.IsAbs(value) {
		return "", errors.New("location path must be relative to the project")
	}
	if len(value) >= 2 && unicode.IsLetter(rune(value[0])) && value[1] == ':' {
		return "", errors.New("location path must be relative to the project")
	}
	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || !fs.ValidPath(clean) {
		return "", errors.New("location path escapes the project")
	}
	return filepath.FromSlash(clean), nil
}

func CompareRevision(indexed, deployed domain.Revision) string {
	if indexed.ProjectID != "" && deployed.ProjectID != "" && indexed.ProjectID != deployed.ProjectID {
		return "mismatch"
	}
	if indexed.ID != "" && deployed.ID != "" && indexed.ID != deployed.ID {
		return "mismatch"
	}
	if indexed.Digest != "" && deployed.Digest != "" && indexed.Digest != deployed.Digest {
		return "mismatch"
	}
	if indexed.ProjectID == "" || deployed.ProjectID == "" || indexed.ID == "" || deployed.ID == "" || indexed.Digest == "" || deployed.Digest == "" {
		return "unconfirmed"
	}
	return "confirmed"
}
