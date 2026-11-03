package contentresource

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/fileown"
	"github.com/acugis/acugis-report-server-go/internal/filesystem"
)

// PathStore stores Image Resources at slash paths for Jasper fill.
// Example: canonical "/finance/images/family.png" → {Dir}/finance/images/family.png
type PathStore struct {
	Dir       string
	FileOwner string
	FileGroup string
}

// NewPathStore constructs a path store rooted at dir.
func NewPathStore(dir string) *PathStore {
	return &PathStore{Dir: strings.TrimSpace(dir)}
}

// SetFileOwner configures optional chown for image files (Tomcat must read them).
func (s *PathStore) SetFileOwner(owner, group string) {
	if s == nil {
		return
	}
	s.FileOwner = strings.TrimSpace(owner)
	s.FileGroup = strings.TrimSpace(group)
}

// EnsureDir creates the path-store directory if missing.
func (s *PathStore) EnsureDir() error {
	if s == nil || s.Dir == "" {
		return ErrUnavailable
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	s.own(s.Dir)
	return nil
}

// AbsPath resolves a canonical resource path under the store root.
func (s *PathStore) AbsPath(canonical string) (string, error) {
	if s == nil || s.Dir == "" {
		return "", ErrUnavailable
	}
	rel, ok := ResourcePathRelative(canonical)
	if !ok {
		return "", ErrInvalidInput
	}
	return filesystem.SafeJoin(s.Dir, filepath.FromSlash(rel))
}

// Write stores bytes at the canonical path (creates parent dirs).
// Files and parent dirs are chowned to FileOwner/FileGroup when configured
// (default tomcat:tomcat) so the Jasper engine can read them.
func (s *PathStore) Write(canonical string, data []byte) error {
	if err := s.EnsureDir(); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	path, err := s.AbsPath(canonical)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("%w: mkdir: %v", ErrUnavailable, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("%w: write: %v", ErrUnavailable, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("%w: rename: %v", ErrUnavailable, err)
	}
	s.ownPathTree(path)
	return nil
}

// Read loads bytes at the canonical path.
func (s *PathStore) Read(canonical string) ([]byte, error) {
	path, err := s.AbsPath(canonical)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return data, nil
}

// Remove deletes the file at the canonical path if present.
func (s *PathStore) Remove(canonical string) error {
	if s == nil || s.Dir == "" || strings.TrimSpace(canonical) == "" {
		return nil
	}
	path, err := s.AbsPath(canonical)
	if err != nil {
		return nil
	}
	err = os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

func (s *PathStore) own(path string) {
	if s == nil || strings.TrimSpace(s.FileOwner) == "" && strings.TrimSpace(s.FileGroup) == "" {
		return
	}
	if err := fileown.Apply(path, s.FileOwner, s.FileGroup); err != nil {
		// Surface clearly in process logs — silent root ownership breaks Tomcat fills.
		fmt.Fprintf(os.Stderr, "jrs: chown %s to %s:%s: %v\n", path, s.FileOwner, s.FileGroup, err)
	}
}

// ownPathTree chowns the file and every directory from the store root down to it.
func (s *PathStore) ownPathTree(filePath string) {
	if s == nil || s.Dir == "" || filePath == "" {
		return
	}
	root := filepath.Clean(s.Dir)
	cur := filepath.Clean(filePath)
	for {
		s.own(cur)
		if cur == root {
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		// Stay under the store root.
		rel, err := filepath.Rel(root, parent)
		if err != nil || strings.HasPrefix(rel, "..") {
			break
		}
		cur = parent
	}
}
