package acl

import "errors"

var (
	ErrDatabase   = errors.New("acl: database unavailable")
	ErrDenied     = errors.New("acl: permission denied")
	ErrBadRequest = errors.New("acl: invalid permission or resource")
)

// DecisionSource identifies why a permission check allowed or denied access.
type DecisionSource string

const (
	SourceAuthDisabled DecisionSource = "auth_disabled"
	SourceAdminFlag    DecisionSource = "administrator_flag"
	SourceAdminGroup   DecisionSource = "administrator_group"
	SourceGroupGrant   DecisionSource = "group_grant"
	SourcePublicGrant  DecisionSource = "public_grant"
	SourceInherited    DecisionSource = "inherited_folder"
	SourceDefaultDeny  DecisionSource = "default_deny"
)

// Decision is a structured ACL outcome for logging.
type Decision struct {
	Allowed  bool
	Source   DecisionSource
	Permission string
	ResourceType string
	ResourceKey  string
}
