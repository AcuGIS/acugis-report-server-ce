package auth

import "time"

// User mirrors the identity fields returned by PHP jrs_auth_lookup_user_by_session
// and jrs_require_login (when auth is disabled).
//
// Password hashes are never loaded into this model.
type User struct {
	ID       int64
	Username string
	Email    string
	IsAdmin  bool
	Enabled  bool
	// Groups are membership names from auth_groups via auth_group_members,
	// optionally merged with legacy dashboard_users.groups JSON (PHP COALESCE).
	Groups []string
	// Embed is set only when this request authenticated with an emb_ token.
	// Sessions and personal access tokens leave it nil. json:"-" keeps it off
	// any accidental User encoding.
	Embed *EmbedGrant `json:"-"`
}

// EmbedGrant is the signed scope of the emb_ token that authenticated this request.
// Parameters are bound parameters: host-controlled report parameter values that
// Report Server must enforce. Nil Parameters means the token binds none.
type EmbedGrant struct {
	ContentType string
	ContentID   string
	Origin      string
	Parameters  map[string][]string
}

// NewEmbedUser builds the synthetic user for a validated emb_ token.
// The returned user carries the token's content scope and bound parameters.
func NewEmbedUser(id int64, username, contentType, contentID, origin string, parameters map[string][]string) *User {
	var params map[string][]string
	if len(parameters) > 0 {
		params = make(map[string][]string, len(parameters))
		for k, vs := range parameters {
			params[k] = append([]string(nil), vs...)
		}
	}
	return &User{
		ID:       id,
		Username: username,
		Enabled:  true,
		Embed: &EmbedGrant{
			ContentType: contentType,
			ContentID:   contentID,
			Origin:      origin,
			Parameters:  params,
		},
	}
}

// Anonymous reports whether this is an unauthenticated identity (id 0 / no username).
func (u *User) Anonymous() bool {
	if u == nil {
		return true
	}
	return u.ID <= 0 && u.Username == ""
}

// DisplayName returns a log-safe username label.
func (u *User) DisplayName() string {
	if u == nil {
		return "anonymous"
	}
	if u.Username != "" {
		return u.Username
	}
	if u.Anonymous() {
		return "anonymous"
	}
	return "unknown"
}

// IsPlatformAdmin matches PHP checks that treat dashboard_users.is_admin or
// admin-named groups as platform administrators.
func (u *User) IsPlatformAdmin() bool {
	if u == nil {
		return false
	}
	if u.IsAdmin {
		return true
	}
	return GroupsIncludeAdmin(u.Groups)
}

// Session is the minimal session row needed to validate authentication.
// The token hash is never stored on this model after lookup.
type Session struct {
	Username  string
	ExpiresAt time.Time
}

// LocalDevAdmin is the synthetic identity PHP installs when JRS_AUTH_DISABLED
// is truthy (jrs_require_login).
func LocalDevAdmin() *User {
	return &User{
		ID:       0,
		Username: "local-dev",
		Email:    "",
		IsAdmin:  true,
		Enabled:  true,
		Groups:   nil,
	}
}
