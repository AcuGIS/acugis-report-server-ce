package auth

// Repository classification is exercised through ResolveUserFromSession against
// a live database when available. This file keeps package-level SQL contract
// documentation for reviewers.

const sessionLookupContract = `
PHP jrs_auth_lookup_user_by_session conditions (exact):
  FROM dashboard_sessions s
  JOIN dashboard_users u ON u.username = s.username
  WHERE s.token_hash = $1
    AND s.expires_at > NOW()
    AND COALESCE(u.enabled, TRUE) = TRUE

Go ResolveUserFromSession uses the same predicates. When no row matches,
classifyMissingSession distinguishes expired vs disabled vs unknown without
mutating session rows.
`
