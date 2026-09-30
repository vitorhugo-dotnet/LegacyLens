package filesystem

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"legacylens/core/internal/domain"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Source struct{}

func NewSource() *Source { return &Source{} }

func (s *Source) List(ctx context.Context, project domain.Project, paths []string) ([]domain.Artifact, error) {
	root, err := openRoot(project.Root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	var artifacts []domain.Artifact
	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel := filepath.ToSlash(path)
		if rel == "." {
			return nil
		}
		if excluded(rel, project.Excludes) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic link is not an approved project file: %s", rel)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || !matches(rel, project.Includes) || !requested(rel, paths) {
			return nil
		}
		content, err := root.ReadFile(rel)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(content)
		artifacts = append(artifacts, domain.Artifact{ProjectID: project.ID, Path: rel, Language: languageFor(rel), Origin: "filesystem", ContentHash: hex.EncodeToString(digest[:])})
		return nil
	})
	return artifacts, err
}

func (s *Source) Read(ctx context.Context, project domain.Project, artifact domain.Artifact) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := openRoot(project.Root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	rel, err := safeRelative(artifact.Path)
	if err != nil {
		return nil, err
	}
	if excluded(filepath.ToSlash(rel), project.Excludes) || !matches(filepath.ToSlash(rel), project.Includes) {
		return nil, errors.New("artifact is excluded by project policy")
	}
	return root.ReadFile(filepath.ToSlash(rel))
}

func openRoot(path string) (*os.Root, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("project root is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return os.OpenRoot(abs)
}

func safeRelative(path string) (string, error) {
	if path == "" || filepath.IsAbs(path) || filepath.VolumeName(path) != "" {
		return "", errors.New("artifact path must be relative")
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("artifact path escapes project root")
	}
	return clean, nil
}

func excluded(path string, patterns []string) bool {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for n := range parts {
		prefix := strings.Join(parts[:n+1], "/")
		for _, pattern := range patterns {
			pattern = filepath.ToSlash(pattern)
			if globMatch(pattern, prefix) || pattern == prefix {
				return true
			}
		}
	}
	return false
}
func matches(path string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, pattern := range patterns {
		if globMatch(filepath.ToSlash(pattern), path) {
			return true
		}
	}
	return false
}

func globMatch(pattern, value string) bool {
	var expression strings.Builder
	expression.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
					expression.WriteString("(?:.*/)?")
				} else {
					expression.WriteString(".*")
				}
			} else {
				expression.WriteString("[^/]*")
			}
		case '?':
			expression.WriteString("[^/]")
		default:
			expression.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	expression.WriteString("$")
	matched, err := regexp.MatchString(expression.String(), value)
	return err == nil && matched
}
func requested(path string, paths []string) bool {
	if len(paths) == 0 {
		return true
	}
	for _, item := range paths {
		if filepath.ToSlash(item) == path {
			return true
		}
	}
	return false
}
func languageFor(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".xhtml", ".xml":
		return "xhtml"
	case ".html", ".htm":
		return "html"
	case ".java":
		return "java"
	case ".js", ".mjs":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".sql":
		return "sql"
	default:
		return "text"
	}
}
