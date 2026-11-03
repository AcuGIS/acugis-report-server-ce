//go:build oss

package explorer

import (
	"context"
	"time"
	"strings"
)

type Service struct {
	Views ViewStore
}

type SavedView struct {
	ID              int64
	Key             string
	Name            string
	Description     string
	OwnerUsername   string
	Datasource      string
	Schema          string
	Table           string
	IsDashboard     bool
	DashboardWidget bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (v *SavedView) NormalizeDashboardFlag() {}

type ViewStore interface {
	Get(ctx context.Context, owner, key string) (*SavedView, error)
	List(ctx context.Context, owner string) ([]SavedView, error)
	ListAll(ctx context.Context) ([]SavedView, error)
	Save(ctx context.Context, view *SavedView) error
	Delete(ctx context.Context, owner, key string) error
	ListDashboardWidgets(ctx context.Context) ([]SavedView, error)
}

type FileViewStore struct{}

func NewFileViewStore(string) *FileViewStore { return &FileViewStore{} }

func (*FileViewStore) Get(context.Context, string, string) (*SavedView, error) {
	return nil, ErrNotFound
}
func (*FileViewStore) List(context.Context, string) ([]SavedView, error) { return nil, nil }
func (*FileViewStore) ListAll(context.Context) ([]SavedView, error)      { return nil, nil }
func (*FileViewStore) Save(context.Context, *SavedView) error            { return ErrNotFound }
func (*FileViewStore) Delete(context.Context, string, string) error      { return nil }
func (*FileViewStore) ListDashboardWidgets(context.Context) ([]SavedView, error) {
	return nil, nil
}

var ErrNotFound = errNotFound("not found")

type errNotFound string

func (e errNotFound) Error() string { return string(e) }

func ViewRef(owner, key string) string {
	return strings.TrimSpace(owner) + "/" + strings.TrimSpace(key)
}

func ParseViewRef(ref string) (owner, key string, ok bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", false
	}
	i := strings.IndexByte(ref, '/')
	if i <= 0 || i >= len(ref)-1 {
		return "", "", false
	}
	return strings.TrimSpace(ref[:i]), strings.TrimSpace(ref[i+1:]), true
}
