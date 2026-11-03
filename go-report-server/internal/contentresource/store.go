package contentresource

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/filesystem"
)

// FileStore writes resource bytes under ResourcesDir using UUID-based names.
type FileStore struct {
	Dir string
}

// NewFileStore constructs a file store rooted at dir (e.g. {appRoot}/resources).
func NewFileStore(dir string) *FileStore {
	return &FileStore{Dir: strings.TrimSpace(dir)}
}

// EnsureDir creates the resources directory if missing.
func (s *FileStore) EnsureDir() error {
	if s == nil || s.Dir == "" {
		return ErrUnavailable
	}
	return os.MkdirAll(s.Dir, 0o750)
}

// AbsPath resolves a relative storage_filename under the resources dir.
func (s *FileStore) AbsPath(storageFilename string) (string, error) {
	if s == nil || s.Dir == "" {
		return "", ErrUnavailable
	}
	name := filepath.Base(strings.TrimSpace(storageFilename))
	if name == "" || name == "." || name == ".." {
		return "", ErrInvalidInput
	}
	return filesystem.SafeJoin(s.Dir, name)
}

// Write stores data at storage_filename and returns the SHA-256 hex digest.
func (s *FileStore) Write(storageFilename string, data []byte) (shaHex string, err error) {
	if err := s.EnsureDir(); err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	path, err := s.AbsPath(storageFilename)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	shaHex = hex.EncodeToString(sum[:])
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o640); err != nil {
		return "", fmt.Errorf("%w: write: %v", ErrUnavailable, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("%w: rename: %v", ErrUnavailable, err)
	}
	return shaHex, nil
}

// Read loads resource bytes by storage_filename.
func (s *FileStore) Read(storageFilename string) ([]byte, error) {
	path, err := s.AbsPath(storageFilename)
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

// Delete removes the on-disk file if present.
func (s *FileStore) Delete(storageFilename string) error {
	path, err := s.AbsPath(storageFilename)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

// HashBytes returns the SHA-256 hex digest of data.
func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
