//go:build oss

package extid

import "context"

type Identity struct {
	ProviderType string
	ProviderID   string
	Email        string
	Subject      string
}

type Repository struct{}

func (*Repository) Configured() bool { return false }

func (*Repository) ListByUserIDs(context.Context, []int64) (map[int64][]Identity, error) {
	return map[int64][]Identity{}, nil
}

func (*Repository) ListByUserID(context.Context, int64) ([]Identity, error) {
	return nil, nil
}

const (
	ProviderTypeOIDC = "oidc"
	ProviderTypeSAML = "saml"
)
