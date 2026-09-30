package application

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
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

var ErrLocationChanged = errors.New("source location changed before launch")

type locationSnapshot struct {
	info   os.FileInfo
	digest [32]byte
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
	file, snapshot, err := openLocationSnapshot(ctx, root, relative)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return OpenResult{}, err
		}
		return OpenResult{}, errors.New("location does not identify a file within the project")
	}
	defer file.Close()
	absPath := filepath.Join(rootPath, relative)
	if err := verifyLocationSnapshot(ctx, root, relative, snapshot); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return OpenResult{File: absPath, Line: location.Line}, err
		}
		return OpenResult{File: absPath, Line: location.Line, Message: "Source changed before launch; refresh the indexed location."}, err
	}
	if err := ctx.Err(); err != nil {
		return OpenResult{File: absPath, Line: location.Line}, err
	}
	return s.editor.Open(ctx, domain.Location{Path: absPath, Line: location.Line, Column: location.Column})
}

func openLocationSnapshot(ctx context.Context, root *os.Root, relative string) (*os.File, locationSnapshot, error) {
	file, err := root.Open(relative)
	if err != nil {
		return nil, locationSnapshot{}, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, locationSnapshot{}, errors.New("location is not a regular file")
	}
	digest, err := hashLocationFile(ctx, file)
	if err != nil {
		file.Close()
		return nil, locationSnapshot{}, err
	}
	return file, locationSnapshot{info: info, digest: digest}, nil
}

func verifyLocationSnapshot(ctx context.Context, root *os.Root, relative string, expected locationSnapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	current, err := root.Open(relative)
	if err != nil {
		return ErrLocationChanged
	}
	defer current.Close()
	info, err := current.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(expected.info, info) {
		return ErrLocationChanged
	}
	digest, err := hashLocationFile(ctx, current)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return ErrLocationChanged
	}
	if digest != expected.digest {
		return ErrLocationChanged
	}
	return nil
}

func hashLocationFile(ctx context.Context, file *os.File) ([32]byte, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, contextReader{ctx: ctx, reader: file}); err != nil {
		return [32]byte{}, err
	}
	var digest [32]byte
	copy(digest[:], hash.Sum(nil))
	return digest, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(data)
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
