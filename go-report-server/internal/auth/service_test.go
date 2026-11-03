package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/auth"
)

// Repository unit tests cover classification helpers via the service mock path
// and document the SQL contract. Live DB tests are optional (no fixtures here).

func TestRepositoryConfiguredNil(t *testing.T) {
	var r *auth.Repository
	if r.Configured() {
		t.Fatal("nil repository should not be configured")
	}
	r = auth.NewRepository(nil)
	if r.Configured() {
		t.Fatal("nil pool should not be configured")
	}
}

func TestServiceCheckTokenOutcomes(t *testing.T) {
	ctx := context.Background()
	svc := auth.NewService(&mockResolver{
		configured: true,
		user:       &auth.User{ID: 3, Username: "carol", Enabled: true},
	}, auth.Options{})
	res := svc.CheckToken(ctx, "jrs-test-token-not-a-secret")
	if res.Status != auth.StatusAuthenticated || res.User.Username != "carol" {
		t.Fatalf("res=%+v", res)
	}

	svc = auth.NewService(&mockResolver{configured: true, err: auth.ErrExpiredSession}, auth.Options{})
	res = svc.CheckToken(ctx, "tok")
	if res.Status != auth.StatusExpired {
		t.Fatalf("status=%s", res.Status)
	}

	svc = auth.NewService(&mockResolver{configured: true, err: errors.Join(auth.ErrDatabase, errors.New("dial"))}, auth.Options{})
	res = svc.CheckToken(ctx, "tok")
	if res.Status != auth.StatusDatabaseError {
		t.Fatalf("status=%s", res.Status)
	}
}
