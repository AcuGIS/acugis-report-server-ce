package acl_test

import (
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/acl"
)

func TestRepositoryConfiguredNil(t *testing.T) {
	var r *acl.Repository
	if r.Configured() {
		t.Fatal("nil repo")
	}
	r = acl.NewRepository(nil)
	if r.Configured() {
		t.Fatal("nil pool")
	}
}
