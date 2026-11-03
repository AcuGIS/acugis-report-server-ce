package embedorigins

import (
	"context"
	"errors"
	"strings"
	"sync"
)

const (
	// SourceAdmin is an origin saved from the Admin UI.
	SourceAdmin = "admin"
	// SourceSystem is an origin supplied outside the Admin UI, including
	// the environment allow-list and development defaults baked in at startup.
	SourceSystem = "system"
)

// Entry is one origin in the effective list.
type Entry struct {
	Origin    string `json:"origin"`
	Source    string `json:"source"`
	Removable bool   `json:"removable"`
}

// Policy merges the startup environment list with persisted Admin origins.
// Admin mutations update memory immediately after the persister succeeds.
type Policy struct {
	mu      sync.RWMutex
	admin   []string
	persist Persister
}

// New returns a policy. persist may be a database repository or a memory store.
func New(persist Persister) *Policy {
	return &Policy{persist: persist}
}

// Reload replaces the in-memory Admin list from the persister.
func (p *Policy) Reload(ctx context.Context) error {
	if p == nil || p.persist == nil || !p.persist.Configured() {
		return ErrUnavailable
	}
	origins, err := p.persist.List(ctx)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.admin = append([]string(nil), origins...)
	p.mu.Unlock()
	return nil
}

// Entries is the effective list. Environment values keep their original text
// and are not removable. Admin values that duplicate an environment value,
// compared case-insensitively, are not repeated.
func (p *Policy) Entries(env []string) []Entry {
	admin := p.adminCopy()
	out := make([]Entry, 0, len(env)+len(admin))
	seen := map[string]struct{}{}
	for _, origin := range env {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		key := strings.ToLower(origin)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, Entry{Origin: origin, Source: SourceSystem, Removable: false})
	}
	for _, origin := range admin {
		key := strings.ToLower(origin)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, Entry{Origin: origin, Source: SourceAdmin, Removable: true})
	}
	return out
}

// Effective returns the origin strings used by CSP, CORS, token checks, and
// viewer postMessage configuration.
func (p *Policy) Effective(env []string) []string {
	entries := p.Entries(env)
	out := make([]string, len(entries))
	for i, entry := range entries {
		out[i] = entry.Origin
	}
	return out
}

// Allows reports whether origin may participate in Embedded Analytics.
// Environment entries match with the historical case-insensitive comparison
// and are not widened by origin normalization. Admin entries also match their
// canonical form, so a default port on the request still matches a stored origin.
func (p *Policy) Allows(env []string, origin string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" || p == nil {
		return false
	}
	for _, allowed := range p.Effective(env) {
		if strings.EqualFold(allowed, origin) {
			return true
		}
	}
	norm, err := NormalizeOrigin(origin)
	if err != nil || strings.EqualFold(norm, origin) {
		return false
	}
	for _, allowed := range p.adminCopy() {
		if strings.EqualFold(allowed, norm) {
			return true
		}
	}
	return false
}

// Add persists a normalized origin. created is false when it is already
// effective, including when the environment already supplies it. The
// environment value is not copied into the Admin table.
func (p *Policy) Add(ctx context.Context, env []string, raw string, userID int64) (created bool, err error) {
	norm, err := NormalizeOrigin(raw)
	if err != nil {
		return false, ErrInvalid
	}
	if p == nil || p.persist == nil || !p.persist.Configured() {
		return false, ErrUnavailable
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if containsFold(p.admin, norm) || containsFold(env, norm) {
		return false, nil
	}
	if err := p.persist.Insert(ctx, norm, userID); err != nil {
		return false, err
	}
	p.admin = append(p.admin, norm)
	return true, nil
}

// Remove deletes a persisted Admin origin. An environment origin is refused
// and left in place, even if the same value was also stored by Admin.
func (p *Policy) Remove(ctx context.Context, env []string, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ErrInvalid
	}
	if containsFold(env, raw) {
		return ErrSystem
	}
	norm, err := NormalizeOrigin(raw)
	if err != nil {
		return ErrInvalid
	}
	if containsFold(env, norm) {
		return ErrSystem
	}
	if p == nil || p.persist == nil || !p.persist.Configured() {
		return ErrUnavailable
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !containsFold(p.admin, norm) {
		return ErrNotFound
	}
	if err := p.persist.Delete(ctx, norm); err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	p.admin = withoutFold(p.admin, norm)
	return nil
}

func (p *Policy) adminCopy() []string {
	if p == nil {
		return nil
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return append([]string(nil), p.admin...)
}

func containsFold(list []string, origin string) bool {
	for _, item := range list {
		if strings.EqualFold(strings.TrimSpace(item), origin) {
			return true
		}
	}
	return false
}

func withoutFold(list []string, origin string) []string {
	out := make([]string, 0, len(list))
	for _, item := range list {
		if strings.EqualFold(item, origin) {
			continue
		}
		out = append(out, item)
	}
	return out
}
