package embedorigins

import (
	"context"
	"errors"
	"testing"
)

func TestPolicyMergeAndWildcardPreserved(t *testing.T) {
	t.Parallel()
	mem := NewMemory()
	p := New(mem)
	ctx := context.Background()
	env := []string{"https://App.Example.com", "*"}
	created, err := p.Add(ctx, env, "https://portal.example.com:443", 7)
	if err != nil || !created {
		t.Fatalf("add portal: created=%v err=%v", created, err)
	}
	created, err = p.Add(ctx, env, "https://APP.example.com", 7)
	if err != nil || created {
		t.Fatalf("env duplicate should be a no-op: created=%v err=%v", created, err)
	}
	created, err = p.Add(ctx, env, "https://portal.example.com", 7)
	if err != nil || created {
		t.Fatalf("admin duplicate should be a no-op: created=%v err=%v", created, err)
	}
	if _, err := p.Add(ctx, env, "*", 7); err == nil {
		t.Fatal("admin add must reject *")
	}
	got := p.Effective(env)
	want := []string{"https://App.Example.com", "*", "https://portal.example.com"}
	if len(got) != len(want) {
		t.Fatalf("effective=%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("effective[%d]=%q want %q (full %v)", i, got[i], want[i], got)
		}
	}
	if p.Allows(env, "https://evil.example.com") {
		t.Fatal("* must not authorize every origin")
	}
	if !p.Allows(env, "*") {
		t.Fatal("literal * environment origin must still match")
	}
	if !p.Allows(env, "https://app.example.com") {
		t.Fatal("environment origin must still match case-insensitively")
	}
	if p.Allows(env, "https://app.example.com:443") {
		t.Fatal("normalization must not widen an environment origin onto a different port")
	}
	if !p.Allows(env, "https://PORTAL.example.com:443") {
		t.Fatal("admin origin should match its canonical form and default port")
	}
	if err := p.Remove(ctx, env, "*"); !errors.Is(err, ErrSystem) {
		t.Fatalf("remove *: %v", err)
	}
	if err := p.Remove(ctx, env, "https://app.example.com"); !errors.Is(err, ErrSystem) {
		t.Fatalf("remove env: %v", err)
	}
	rows, err := mem.List(ctx)
	if err != nil || len(rows) != 1 || rows[0] != "https://portal.example.com" {
		t.Fatalf("persisted=%v err=%v", rows, err)
	}
}

func TestPolicyReloadSurvivesNewInstance(t *testing.T) {
	t.Parallel()
	mem := NewMemory()
	ctx := context.Background()
	first := New(mem)
	if _, err := first.Add(ctx, nil, "https://APP.Example.com:443", 1); err != nil {
		t.Fatal(err)
	}
	second := New(mem)
	if second.Allows(nil, "https://app.example.com") {
		t.Fatal("new policy must not see origins before reload")
	}
	if err := second.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if !second.Allows(nil, "https://app.example.com") {
		t.Fatal("reloaded policy should see the persisted origin")
	}
	if err := second.Remove(ctx, nil, "https://app.example.com"); err != nil {
		t.Fatal(err)
	}
	third := New(mem)
	if err := third.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if third.Allows(nil, "https://app.example.com") {
		t.Fatal("removed origin should stay gone after reload")
	}
}

func TestPolicySystemRowIsNotDeleted(t *testing.T) {
	t.Parallel()
	mem := NewMemory()
	ctx := context.Background()
	if err := mem.Insert(ctx, "https://app.example.com", 1); err != nil {
		t.Fatal(err)
	}
	p := New(mem)
	if err := p.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	err := p.Remove(ctx, []string{"https://App.Example.com"}, "https://app.example.com")
	if !errors.Is(err, ErrSystem) {
		t.Fatalf("remove: %v", err)
	}
	rows, err := mem.List(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("row was deleted: %v %v", rows, err)
	}
	if !p.Allows([]string{"https://App.Example.com"}, "https://app.example.com") {
		t.Fatal("environment origin should remain allowed")
	}
}

func TestPolicyEmptyDenies(t *testing.T) {
	t.Parallel()
	p := New(NewMemory())
	if p.Allows(nil, "https://app.example.com") {
		t.Fatal("empty policy must deny")
	}
	if p.Allows([]string{}, "") {
		t.Fatal("empty origin must deny")
	}
	if FrameAncestors(p.Effective(nil)) != "'none'" {
		t.Fatal("empty effective list must stay frame-ancestors 'none'")
	}
}
