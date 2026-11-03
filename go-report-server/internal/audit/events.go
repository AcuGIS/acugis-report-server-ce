package audit

// Top-level categories (stable set).
const (
	CategoryAuthentication = "authentication"
	CategoryReport         = "report"
	CategoryDatasource     = "datasource"
	CategoryContent        = "content"
	CategorySecurity       = "security"
	CategoryAnalysis       = "analysis"
	CategorySystem         = "system"
	// CategoryGovernance records Professional Governance / Certification changes.
	CategoryGovernance = "governance"
)

// Event status values.
const (
	StatusSuccess = "success"
	StatusFailure = "failure"
)

// Authentication actions.
const (
	ActionLoginSuccess = "login_success"
	ActionLoginFailed  = "login_failed"
	ActionLogout       = "logout"
	ActionTokenSuccess = "token_success"
	ActionTokenFailed  = "token_failed"

	ActionOIDCLoginSuccess     = "oidc_login_success"
	ActionOIDCLoginFailed      = "oidc_login_failed"
	ActionOIDCUserCreated      = "oidc_user_created"
	ActionOIDCIdentityLinked   = "oidc_identity_linked"
	ActionOIDCLogout           = "oidc_logout"
	ActionLocalBreakGlassLogin = "local_break_glass_login"
	ActionOIDCConfigChanged    = "oidc_config_changed"

	ActionSAMLLoginSuccess       = "saml_login_success"
	ActionSAMLLoginFailed        = "saml_login_failed"
	ActionSAMLUserCreated        = "saml_user_created"
	ActionSAMLIdentityLinked     = "saml_identity_linked"
	ActionSAMLLogout             = "saml_logout"
	ActionSAMLConfigChanged      = "saml_config_changed"
	ActionSAMLMetadataRefreshed  = "saml_metadata_refreshed"
	ActionSAMLCertificateChanged = "saml_certificate_changed"
)

// Report actions.
const (
	ActionReportView     = "view"
	ActionReportExecute  = "execute"
	ActionReportExport   = "export"
	ActionReportDownload = "download"
	ActionCreate         = "create"
	ActionUpdate         = "update"
	ActionDelete         = "delete"
	ActionMove           = "move"
	ActionTest           = "test"
	ActionOpen           = "open"
	ActionQuery          = "query"
	ActionSave           = "save"
	ActionGrant          = "grant"
	ActionRevoke         = "revoke"
	ActionChange         = "change"
	// ActionAPIRequest records selective /api/v1/* HTTP usage (system category).
	ActionAPIRequest = "api_request"
	// ActionMCPListDatasets records a Professional MCP list_datasets call.
	ActionMCPListDatasets = "mcp_list_datasets"
	// ActionMCPDescribeDataset records a Professional MCP describe_dataset call.
	ActionMCPDescribeDataset = "mcp_describe_dataset"
	// ActionMCPQueryDataset records a Professional MCP query_dataset call.
	ActionMCPQueryDataset = "mcp_query_dataset"
)

// Object types used in audit events.
const (
	ObjectReport                 = "report"
	ObjectView                   = "view"
	ObjectDashboard              = "dashboard"
	ObjectFolder                 = "folder"
	ObjectResource               = "resource"
	ObjectDatasource             = "datasource"
	ObjectDataset                = "dataset"
	ObjectUser                   = "user"
	ObjectGroup                  = "group"
	ObjectPermission             = "permission"
	ObjectToken                  = "token"
	ObjectSession                = "session"
	ObjectAPI                    = "api"
	ObjectAuditExportDestination = "audit_export_destination"
)

// DetailsChannelEmbed marks events that occurred via embed-token /embed routes.
const DetailsChannelEmbed = "embed"

// Details keys used by Usage Analytics (never store secrets under these).
const (
	DetailsKeyChannel   = "channel"
	DetailsKeyOrigin    = "origin"
	DetailsKeyOperation = "operation"
	DetailsKeyMethod    = "method"
	DetailsKeyRoute     = "route"
	DetailsKeyStatus    = "status_code"
)

// Common event action labels for display (category + action remain separate columns).
