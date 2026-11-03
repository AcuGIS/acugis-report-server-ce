package contentresource

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/acugis/acugis-report-server-go/internal/filesystem"
)

const defaultThumbMaxEdge = 256

// ThumbnailStore writes UUID-named thumbnail PNGs under ThumbnailsDir.
type ThumbnailStore struct {
	Dir string
}

// NewThumbnailStore constructs a thumbnail store rooted at dir (e.g. {appRoot}/thumbnails).
func NewThumbnailStore(dir string) *ThumbnailStore {
	return &ThumbnailStore{Dir: strings.TrimSpace(dir)}
}

// EnsureDir creates the thumbnails directory if missing.
func (s *ThumbnailStore) EnsureDir() error {
	if s == nil || s.Dir == "" {
		return ErrUnavailable
	}
	return os.MkdirAll(s.Dir, 0o750)
}

// Filename returns the on-disk thumbnail name for a resource UUID (always .png).
func ThumbnailFilename(uuid string) string {
	return strings.TrimSpace(uuid) + ".png"
}

// AbsPath resolves a thumbnail filename under the thumbnails dir.
func (s *ThumbnailStore) AbsPath(filename string) (string, error) {
	if s == nil || s.Dir == "" {
		return "", ErrUnavailable
	}
	name := filepath.Base(strings.TrimSpace(filename))
	if name == "" || name == "." || name == ".." {
		return "", ErrInvalidInput
	}
	return filesystem.SafeJoin(s.Dir, name)
}

// WritePNG stores PNG thumbnail bytes as {uuid}.png.
func (s *ThumbnailStore) WritePNG(uuid string, pngBytes []byte) error {
	if err := s.EnsureDir(); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	path, err := s.AbsPath(ThumbnailFilename(uuid))
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, pngBytes, 0o640); err != nil {
		return fmt.Errorf("%w: write: %v", ErrUnavailable, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("%w: rename: %v", ErrUnavailable, err)
	}
	return nil
}

// ReadPNG loads a thumbnail PNG by resource UUID.
func (s *ThumbnailStore) ReadPNG(uuid string) ([]byte, error) {
	path, err := s.AbsPath(ThumbnailFilename(uuid))
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

// Delete removes the thumbnail for a UUID if present.
func (s *ThumbnailStore) Delete(uuid string) error {
	if s == nil || s.Dir == "" {
		return nil
	}
	path, err := s.AbsPath(ThumbnailFilename(uuid))
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

// GenerateThumbnailPNG builds a PNG thumbnail from image bytes.
// SVG returns ErrBadImageType (caller may skip or use original for preview).
func GenerateThumbnailPNG(data []byte, maxEdge int) ([]byte, error) {
	if maxEdge <= 0 {
		maxEdge = defaultThumbMaxEdge
	}
	if looksLikeSVG(data) {
		return nil, ErrBadImageType
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: decode: %v", ErrInvalidImage, err)
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("%w: empty dimensions", ErrInvalidImage)
	}
	nw, nh := w, h
	if w > maxEdge || h > maxEdge {
		if w >= h {
			nw = maxEdge
			nh = int(float64(h) * float64(maxEdge) / float64(w))
		} else {
			nh = maxEdge
			nw = int(float64(w) * float64(maxEdge) / float64(h))
		}
		if nw < 1 {
			nw = 1
		}
		if nh < 1 {
			nh = 1
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, fmt.Errorf("%w: encode: %v", ErrUnavailable, err)
	}
	return buf.Bytes(), nil
}

func looksLikeSVG(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	lower := bytes.ToLower(trimmed)
	return bytes.Contains(lower, []byte("<svg"))
}
