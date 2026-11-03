// Package resourcefs is the public Report Server resource adapter used by the
// Designer. It resolves report-owned files through the existing
// {key}.resource-base file and the resource-paths directory Tomcat already
// uses. It does not create a second resource library and it does not read
// content-resource credential objects.
//
// Logical paths are report-relative:
//
//	images/logo.png
//	data/sales.sqlite
//	reports/detail.jasper
//
// Those resolve under the report's resource base:
//
//	{resource-paths}/{resource-base}/images/logo.png
//
// Symlink policy: Designer resource operations reject symlinks entirely.
// A link inside images/, data/, or reports/ is not followed, listed, read,
// replaced, or deleted. Report Server resource storage does not use symlinks
// for these files.
//
// Credential and content-resource objects (private keys, service-account JSON,
// tokens, UUID blobs) are not part of this virtual filesystem. Only the three
// logical roots above are visible, even if other directories exist beside them.
package resourcefs

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/fileown"
	"github.com/acugis/acugis-report-server-go/internal/filesystem"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

const (
	dirImages  = "images"
	dirData    = "data"
	dirReports = "reports"
)

// Sentinel errors never include a filesystem path.
var (
	ErrInvalidKey  = reports.ErrInvalidKey
	ErrNotFound    = errors.New("resource not found")
	ErrNoBase      = errors.New("report resource base is not configured")
	ErrInvalidPath = errors.New("invalid resource path")
	ErrConflict    = errors.New("resource already exists")
	ErrUnavailable = errors.New("resources unavailable")
)

// Info is safe metadata for one report-owned file.
// Path is the logical report-relative path. The physical location is not returned.
type Info struct {
	Path    string
	Name    string
	Size    int64
	ModTime time.Time
}

// Store resolves one report's resource base.
type Store struct {
	jrxmlDir string
	pathsDir string
	owner    string
	group    string
}

// NewStore binds the JRXML directory (where {key}.resource-base lives) and the
// existing resource-paths directory. Neither path is taken from a browser.
func NewStore(jrxmlDir, resourcePathsDir string) *Store {
	return &Store{
		jrxmlDir: strings.TrimSpace(jrxmlDir),
		pathsDir: strings.TrimSpace(resourcePathsDir),
	}
}

// SetFileOwner configures the same optional chown used for JRXML and image
// resources. Empty values leave ownership unchanged. Modes still follow the
// Report Server resource policy (0644 files, 0755 directories).
func (s *Store) SetFileOwner(owner, group string) {
	if s == nil {
		return
	}
	s.owner = strings.TrimSpace(owner)
	s.group = strings.TrimSpace(group)
}

// ResourceBase returns the catalog folder recorded in {key}.resource-base,
// such as "finance". It does not return a filesystem path.
func (s *Store) ResourceBase(key string) (string, error) {
	if s == nil || s.jrxmlDir == "" || s.pathsDir == "" {
		return "", ErrUnavailable
	}
	base, err := reports.NewStore("", "", s.jrxmlDir).ReadResourceBase(key)
	if err != nil {
		if errors.Is(err, reports.ErrInvalidKey) {
			return "", ErrInvalidKey
		}
		if errors.Is(err, reports.ErrNotFound) {
			return "", ErrNoBase
		}
		return "", ErrNoBase
	}
	return base, nil
}

// CleanLogical validates a Designer logical path and returns it with forward
// slashes. The result is always images/..., data/..., or reports/...
func CleanLogical(raw string) (string, error) {
	if strings.ContainsRune(raw, 0) {
		return "", ErrInvalidPath
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrInvalidPath
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "file:") || strings.Contains(lower, "://") {
		return "", ErrInvalidPath
	}
	if filepath.IsAbs(raw) || strings.HasPrefix(raw, `\\`) || strings.HasPrefix(raw, "//") {
		return "", ErrInvalidPath
	}
	if len(raw) >= 2 && raw[1] == ':' {
		return "", ErrInvalidPath
	}
	if strings.Contains(raw, ":") {
		return "", ErrInvalidPath
	}
	slash := strings.ReplaceAll(raw, `\`, "/")
	if strings.HasPrefix(slash, "/") {
		return "", ErrInvalidPath
	}
	cleaned := path.Clean(slash)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
		return "", ErrInvalidPath
	}
	parts := strings.Split(cleaned, "/")
	if len(parts) < 2 {
		return "", ErrInvalidPath
	}
	switch parts[0] {
	case dirImages, dirData, dirReports:
	default:
		return "", ErrInvalidPath
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.Contains(part, ":") {
			return "", ErrInvalidPath
		}
	}
	return cleaned, nil
}

// Resolve returns the physical path for a logical resource. Only trusted
// server-side code may see the result. It is not written to project.json,
// catalog JSON, or browser responses.
func (s *Store) Resolve(key, logical string) (string, error) {
	logical, err := CleanLogical(logical)
	if err != nil {
		return "", err
	}
	root, err := s.root(key)
	if err != nil {
		return "", err
	}
	return confine(root, logical, false)
}

// Stat returns metadata for one logical resource.
func (s *Store) Stat(key, logical string) (Info, error) {
	full, err := s.Resolve(key, logical)
	if err != nil {
		return Info{}, err
	}
	st, err := os.Lstat(full)
	if err != nil {
		if os.IsNotExist(err) {
			return Info{}, ErrNotFound
		}
		return Info{}, ErrUnavailable
	}
	if !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 {
		return Info{}, ErrNotFound
	}
	logical, _ = CleanLogical(logical)
	return Info{
		Path:    logical,
		Name:    path.Base(logical),
		Size:    st.Size(),
		ModTime: st.ModTime(),
	}, nil
}

// Open reads one regular file. The caller must close the reader.
func (s *Store) Open(key, logical string) (*os.File, Info, error) {
	info, err := s.Stat(key, logical)
	if err != nil {
		return nil, Info{}, err
	}
	full, err := s.Resolve(key, logical)
	if err != nil {
		return nil, Info{}, err
	}
	f, err := os.Open(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, Info{}, ErrNotFound
		}
		return nil, Info{}, ErrUnavailable
	}
	return f, info, nil
}

// List returns regular files under images/, data/, and reports/.
// kindTop limits the walk to one of those roots when non-empty.
// Symlinks are omitted. Physical paths are not returned.
func (s *Store) List(key, kindTop string) ([]Info, error) {
	kindTop = strings.Trim(strings.ReplaceAll(kindTop, `\`, "/"), "/")
	var roots []string
	switch kindTop {
	case "":
		roots = []string{dirImages, dirData, dirReports}
	case dirImages, dirData, dirReports:
		roots = []string{kindTop}
	default:
		return nil, ErrInvalidPath
	}
	root, err := s.root(key)
	if err != nil {
		return nil, err
	}
	var out []Info
	for _, name := range roots {
		dir, err := confine(root, name, true)
		if err != nil {
			return nil, err
		}
		st, err := os.Lstat(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, ErrUnavailable
		}
		if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
			continue
		}
		err = filepath.WalkDir(dir, func(p string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				if os.IsNotExist(walkErr) {
					return nil
				}
				return ErrUnavailable
			}
			info, err := d.Info()
			if err != nil || info.Mode()&os.ModeSymlink != 0 {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return ErrUnavailable
			}
			logical := filepath.ToSlash(rel)
			if _, err := CleanLogical(logical); err != nil {
				return nil
			}
			out = append(out, Info{
				Path:    logical,
				Name:    d.Name(),
				Size:    info.Size(),
				ModTime: info.ModTime(),
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if out == nil {
		out = []Info{}
	}
	return out, nil
}

// Put writes data to a logical path. The previous file is left in place until
// the replacement has been fully written. overwrite false returns ErrConflict
// when the file exists.
func (s *Store) Put(key, logical string, data []byte, overwrite bool) (Info, error) {
	logical, err := CleanLogical(logical)
	if err != nil {
		return Info{}, err
	}
	if len(data) == 0 {
		return Info{}, ErrInvalidPath
	}
	root, err := s.root(key)
	if err != nil {
		return Info{}, err
	}
	full, err := confine(root, logical, false)
	if err != nil {
		return Info{}, err
	}
	if err := s.ensureParents(root, full); err != nil {
		return Info{}, err
	}
	if st, err := os.Lstat(full); err == nil {
		if st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() {
			return Info{}, ErrInvalidPath
		}
		if !overwrite {
			return Info{}, ErrConflict
		}
	} else if !os.IsNotExist(err) {
		return Info{}, ErrUnavailable
	}
	if err := install(full, data, overwrite); err != nil {
		return Info{}, err
	}
	s.ownTree(root, full)
	return s.Stat(key, logical)
}

// Delete removes one regular file. Symlinks and directories are rejected.
func (s *Store) Delete(key, logical string) error {
	full, err := s.Resolve(key, logical)
	if err != nil {
		return err
	}
	st, err := os.Lstat(full)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return ErrUnavailable
	}
	if st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() {
		return ErrInvalidPath
	}
	if err := os.Remove(full); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return ErrUnavailable
	}
	return nil
}

// CopyInto copies images/, data/, and reports/ into dest.
// Symlinks are skipped. Existing files in dest are not used as sources.
// The live tree is only read. dest is expected to be an empty temporary directory.
func (s *Store) CopyInto(key, dest string) error {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return ErrUnavailable
	}
	root, err := s.root(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return ErrUnavailable
	}
	destReal, err := filepath.Abs(dest)
	if err != nil {
		return ErrUnavailable
	}
	rootReal, err := filepath.Abs(root)
	if err != nil {
		return ErrUnavailable
	}
	if rel, relErr := filepath.Rel(rootReal, destReal); relErr == nil {
		// A different volume cannot be expressed as a child of the live root.
		// Same-volume destinations must stay outside that root.
		if rel == "." || rel == "" || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)) {
			return ErrInvalidPath
		}
	}
	for _, name := range []string{dirImages, dirData, dirReports} {
		from, err := confine(root, name, true)
		if err != nil {
			return err
		}
		st, err := os.Lstat(from)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return ErrUnavailable
		}
		if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
			continue
		}
		if err := copyTree(from, filepath.Join(dest, name)); err != nil {
			return err
		}
	}
	return nil
}

// EnsureIsolated creates images/, data/, and reports/ under resource-paths/base.
// base must be a non-empty catalog-relative path. The directory must not be
// the resource-paths root, so a Home report cannot see every other report's files.
func (s *Store) EnsureIsolated(base string) error {
	root, err := s.isolatedRoot(base)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return ErrUnavailable
	}
	s.own(root)
	for _, name := range []string{dirImages, dirData, dirReports} {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return ErrUnavailable
		}
		s.own(dir)
	}
	return nil
}

// CopyAllowed copies images/, data/, and reports/ from one report resource
// base to another. Symlinks are not followed. The two roots must differ.
func (s *Store) CopyAllowed(fromKey, toKey string) error {
	if fromKey == toKey {
		return ErrInvalidPath
	}
	from, err := s.root(fromKey)
	if err != nil {
		return err
	}
	to, err := s.root(toKey)
	if err != nil {
		return err
	}
	fromReal, err := filepath.Abs(from)
	if err != nil {
		return ErrUnavailable
	}
	toReal, err := filepath.Abs(to)
	if err != nil {
		return ErrUnavailable
	}
	if fromReal == toReal {
		return ErrInvalidPath
	}
	for _, name := range []string{dirImages, dirData, dirReports} {
		src, err := confine(from, name, true)
		if err != nil {
			return err
		}
		st, err := os.Lstat(src)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return ErrUnavailable
		}
		if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
			continue
		}
		dst, err := confine(to, name, true)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return ErrUnavailable
		}
		if err := copyTree(src, dst); err != nil {
			return err
		}
	}
	return nil
}

// RelocateIsolated moves a report-owned resource directory from one
// catalog-relative base to another. A missing source is not an error: the
// caller can point the sidecar at the new base. An existing destination is
// left untouched and returns ErrConflict so the sidecar can stay with the files.
func (s *Store) RelocateIsolated(fromBase, toBase string) error {
	if s == nil || s.pathsDir == "" {
		return ErrUnavailable
	}
	fromBase, err := reports.NormalizeResourceBase(fromBase)
	if err != nil || fromBase == "" {
		return ErrNoBase
	}
	toBase, err = reports.NormalizeResourceBase(toBase)
	if err != nil || toBase == "" {
		return ErrNoBase
	}
	if fromBase == toBase {
		return nil
	}
	from, err := s.isolatedRoot(fromBase)
	if err != nil {
		return err
	}
	to, err := s.isolatedRoot(toBase)
	if err != nil {
		return err
	}
	if from == to {
		return nil
	}
	if rel, relErr := filepath.Rel(from, to); relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
		return ErrInvalidPath
	}
	st, err := os.Lstat(from)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return ErrUnavailable
	}
	if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
		return ErrInvalidPath
	}
	if _, err := os.Lstat(to); err == nil {
		return ErrConflict
	} else if !os.IsNotExist(err) {
		return ErrUnavailable
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return ErrUnavailable
	}
	if err := os.Rename(from, to); err != nil {
		return ErrUnavailable
	}
	return nil
}

// RemoveIsolated deletes the resource directory only when {key}.resource-base
// equals expectedBase, expectedBase is non-empty, and the directory sits
// strictly inside resource-paths. It also removes the resource-base file.
func (s *Store) RemoveIsolated(key, expectedBase string) error {
	if s == nil || s.jrxmlDir == "" || s.pathsDir == "" {
		return ErrUnavailable
	}
	expected, err := reports.NormalizeResourceBase(expectedBase)
	if err != nil || expected == "" {
		return ErrNoBase
	}
	got, err := s.ResourceBase(key)
	if err != nil && !errors.Is(err, ErrNoBase) {
		return err
	}
	if err == nil && got != expected {
		return ErrInvalidPath
	}
	root, err := s.isolatedRoot(expected)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(root); err != nil && !os.IsNotExist(err) {
		return ErrUnavailable
	}
	baseFile := filepath.Join(s.jrxmlDir, key+".resource-base")
	if err := os.Remove(baseFile); err != nil && !os.IsNotExist(err) {
		return ErrUnavailable
	}
	return nil
}

func (s *Store) isolatedRoot(base string) (string, error) {
	if s == nil || s.pathsDir == "" {
		return "", ErrUnavailable
	}
	base, err := reports.NormalizeResourceBase(base)
	if err != nil || base == "" {
		return "", ErrNoBase
	}
	paths, err := filepath.Abs(s.pathsDir)
	if err != nil {
		return "", ErrUnavailable
	}
	if real, err := filepath.EvalSymlinks(paths); err == nil {
		paths = real
	} else if !os.IsNotExist(err) {
		return "", ErrUnavailable
	}
	full, err := filesystem.SafeJoin(paths, filepath.FromSlash(base))
	if err != nil {
		return "", ErrInvalidPath
	}
	if full == paths {
		return "", ErrInvalidPath
	}
	rel, err := filepath.Rel(paths, full)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", ErrInvalidPath
	}
	if err := walkNoSymlink(paths, full); err != nil {
		return "", err
	}
	return full, nil
}

// ApplyOwner sets Report Server ownership on one logical file and its parents
// under the resource root. It uses the existing fileown helper.
func (s *Store) ApplyOwner(key, logical string) error {
	root, err := s.root(key)
	if err != nil {
		return err
	}
	full, err := s.Resolve(key, logical)
	if err != nil {
		return err
	}
	s.ownTree(root, full)
	return nil
}

// StageDir creates a temporary directory inside the report resource root so a
// later rename stays on the same filesystem. The caller removes it.
func (s *Store) StageDir(key string) (string, error) {
	root, err := s.root(key)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", ErrUnavailable
	}
	s.own(root)
	dir, err := os.MkdirTemp(root, ".designer-stage-")
	if err != nil {
		return "", ErrUnavailable
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return "", ErrUnavailable
	}
	return dir, nil
}

// Pending is one staged file waiting to replace a live resource.
type Pending struct {
	Src string
	Dst string
}

// PromoteMany renames staged files onto their destinations and returns a
// rollback that restores the previous files. Call Commit to drop the backups
// after the rest of the transaction succeeds.
func PromoteMany(files []Pending) (commit func(), rollback func(), err error) {
	type item struct {
		dst    string
		bak    string
		had    bool
		placed bool
	}
	done := make([]item, 0, len(files))
	rollback = func() {
		for i := len(done) - 1; i >= 0; i-- {
			it := done[i]
			if it.placed {
				_ = os.Remove(it.dst)
			}
			if it.had {
				_ = os.Rename(it.bak, it.dst)
			}
		}
	}
	for _, f := range files {
		st, err := os.Lstat(f.Src)
		if err != nil || !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 {
			rollback()
			return nil, nil, ErrUnavailable
		}
		if cur, err := os.Lstat(f.Dst); err == nil && (cur.Mode()&os.ModeSymlink != 0 || !cur.Mode().IsRegular()) {
			rollback()
			return nil, nil, ErrInvalidPath
		}
		it := item{dst: f.Dst, bak: f.Dst + ".designer-bak"}
		_ = os.Remove(it.bak)
		if _, err := os.Lstat(f.Dst); err == nil {
			if err := os.Rename(f.Dst, it.bak); err != nil {
				rollback()
				return nil, nil, ErrUnavailable
			}
			it.had = true
		} else if !os.IsNotExist(err) {
			rollback()
			return nil, nil, ErrUnavailable
		}
		if err := os.MkdirAll(filepath.Dir(f.Dst), 0o755); err != nil {
			rollback()
			return nil, nil, ErrUnavailable
		}
		if err := os.Rename(f.Src, f.Dst); err != nil {
			rollback()
			return nil, nil, ErrUnavailable
		}
		it.placed = true
		_ = os.Chmod(f.Dst, 0o644)
		done = append(done, it)
	}
	commit = func() {
		for _, it := range done {
			if it.had {
				_ = os.Remove(it.bak)
			}
		}
	}
	return commit, rollback, nil
}

func (s *Store) root(key string) (string, error) {
	base, err := s.ResourceBase(key)
	if err != nil {
		return "", err
	}
	paths, err := filepath.Abs(s.pathsDir)
	if err != nil {
		return "", ErrUnavailable
	}
	if real, err := filepath.EvalSymlinks(paths); err == nil {
		paths = real
	} else if !os.IsNotExist(err) {
		return "", ErrUnavailable
	}
	full, err := filesystem.SafeJoin(paths, filepath.FromSlash(base))
	if err != nil {
		return "", ErrInvalidPath
	}
	if err := walkNoSymlink(paths, full); err != nil {
		return "", err
	}
	return full, nil
}

// confine joins a logical or single-segment path onto root and rejects symlink escapes.
// allowDir permits the path to name one of the three roots without a file leaf.
func confine(root, logical string, allowDir bool) (string, error) {
	logical = strings.Trim(strings.ReplaceAll(logical, `\`, "/"), "/")
	if logical == "" || strings.ContainsRune(logical, 0) || strings.Contains(logical, "..") {
		return "", ErrInvalidPath
	}
	if !allowDir {
		if _, err := CleanLogical(logical); err != nil {
			return "", err
		}
	} else {
		switch logical {
		case dirImages, dirData, dirReports:
		default:
			return "", ErrInvalidPath
		}
	}
	full, err := filesystem.SafeJoin(root, filepath.FromSlash(logical))
	if err != nil {
		return "", ErrInvalidPath
	}
	if err := walkNoSymlink(root, full); err != nil {
		return "", err
	}
	if st, err := os.Lstat(full); err == nil && st.Mode()&os.ModeSymlink == 0 {
		realRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			if !os.IsNotExist(err) {
				return "", ErrInvalidPath
			}
			realRoot = root
		}
		real, err := filepath.EvalSymlinks(full)
		if err != nil {
			return "", ErrInvalidPath
		}
		rel, err := filepath.Rel(realRoot, real)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return "", ErrInvalidPath
		}
	}
	return full, nil
}

func walkNoSymlink(root, full string) error {
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return ErrInvalidPath
	}
	cursor := root
	parts := strings.Split(rel, string(filepath.Separator))
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			return ErrInvalidPath
		}
		next := filepath.Join(cursor, part)
		st, err := os.Lstat(next)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return ErrUnavailable
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return ErrInvalidPath
		}
		if i < len(parts)-1 && !st.IsDir() {
			return ErrInvalidPath
		}
		cursor = next
	}
	return nil
}

func (s *Store) ensureParents(root, full string) error {
	parent := filepath.Dir(full)
	rel, err := filepath.Rel(root, parent)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ErrInvalidPath
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return ErrUnavailable
	}
	s.own(root)
	cursor := root
	if rel != "." {
		for _, part := range strings.Split(rel, string(filepath.Separator)) {
			if part == "" || part == "." || part == ".." {
				return ErrInvalidPath
			}
			cursor = filepath.Join(cursor, part)
			st, err := os.Lstat(cursor)
			if err != nil {
				if !os.IsNotExist(err) {
					return ErrUnavailable
				}
				if err := os.Mkdir(cursor, 0o755); err != nil && !os.IsExist(err) {
					return ErrUnavailable
				}
				s.own(cursor)
				continue
			}
			if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
				return ErrInvalidPath
			}
		}
	}
	return nil
}

func install(dest string, data []byte, overwrite bool) error {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return ErrUnavailable
	}
	tmp := dest + "." + hex.EncodeToString(buf) + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return ErrUnavailable
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return ErrUnavailable
	}
	if err := f.Sync(); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return ErrUnavailable
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return ErrUnavailable
	}
	if !overwrite {
		if err := os.Link(tmp, dest); err == nil {
			_ = os.Remove(tmp)
			_ = os.Chmod(dest, 0o644)
			return nil
		}
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			_ = os.Remove(tmp)
			if os.IsExist(err) {
				return ErrConflict
			}
			return ErrUnavailable
		}
		if _, err := out.Write(data); err != nil {
			out.Close()
			_ = os.Remove(dest)
			_ = os.Remove(tmp)
			return ErrUnavailable
		}
		_ = out.Sync()
		out.Close()
		_ = os.Remove(tmp)
		return nil
	}
	bak := dest + ".designer-bak"
	_ = os.Remove(bak)
	had := false
	if _, err := os.Lstat(dest); err == nil {
		if err := os.Rename(dest, bak); err != nil {
			_ = os.Remove(tmp)
			return ErrUnavailable
		}
		had = true
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		if had {
			_ = os.Rename(bak, dest)
		}
		return ErrUnavailable
	}
	_ = os.Chmod(dest, 0o644)
	_ = os.Remove(bak)
	return nil
}

func (s *Store) own(path string) {
	if s == nil {
		return
	}
	_ = os.Chmod(path, fileMode(path))
	_ = fileown.Apply(path, s.owner, s.group)
}

func (s *Store) ownTree(root, filePath string) {
	root = filepath.Clean(root)
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
		rel, err := filepath.Rel(root, parent)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			break
		}
		cur = parent
	}
}

func fileMode(path string) os.FileMode {
	st, err := os.Lstat(path)
	if err == nil && st.IsDir() {
		return 0o755
	}
	return 0o644
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return ErrUnavailable
		}
		info, err := d.Info()
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(src, p)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return ErrUnavailable
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyRegular(p, target)
	})
}

func copyRegular(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return ErrUnavailable
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return ErrUnavailable
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return ErrUnavailable
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(dst)
		return ErrUnavailable
	}
	if err := out.Sync(); err != nil {
		out.Close()
		_ = os.Remove(dst)
		return ErrUnavailable
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return ErrUnavailable
	}
	return nil
}
