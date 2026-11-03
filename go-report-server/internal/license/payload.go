package license

import (
	"encoding/json"
	"strings"
	"time"
)

const (
	// SchemaVersion is the only payload schema this verifier accepts.
	SchemaVersion   = 1
	maxLicenseIDLen = 128

	// ProductReportServerProfessional is the Report Server Professional entitlement.
	ProductReportServerProfessional = "report-server-professional"
	// ProductReportDesignerWeb is a separate product and does not entitle Report Server.
	ProductReportDesignerWeb = "report-designer-web"
	// ProductReportDesignerDesktop is a separate product and does not entitle Report Server.
	ProductReportDesignerDesktop = "report-designer-desktop"

	// RefreshGracePeriod matches the central license service. An expired token
	// may be submitted to /v1/refresh during this window. It does not extend
	// Professional entitlement past expires_at.
	RefreshGracePeriod = 30 * 24 * time.Hour

	// RefreshInterval is how often a running server asks the license service
	// to replace a stored token. It is not a retry loop.
	RefreshInterval = 24 * time.Hour

	// LicenseTypePaid is a purchased entitlement. Tokens that omit license_type
	// are treated as paid. The field is not an authorization control.
	LicenseTypePaid = "paid"
	// LicenseTypeTrial marks a trial ACL1. Product use still depends on
	// signature, product, installation, and expires_at.
	LicenseTypeTrial = "trial"
)

// Payload is the signed AcuGIS license schema version 1.
type Payload struct {
	SchemaVersion  int    `json:"schema_version"`
	LicenseID      string `json:"license_id"`
	InstallationID string `json:"installation_id"`
	ProductCode    string `json:"product_code"`
	IssuedAt       string `json:"issued_at"`
	ExpiresAt      string `json:"expires_at"`
	// LicenseType is optional lifecycle metadata. Empty means purchased or legacy.
	LicenseType string `json:"license_type,omitempty"`
}

// InformationalLicenseType returns paid or trial. Other values are ignored
// and do not make a signed token invalid.
func InformationalLicenseType(raw string) string {
	switch strings.TrimSpace(raw) {
	case LicenseTypePaid, LicenseTypeTrial:
		return strings.TrimSpace(raw)
	default:
		return ""
	}
}

func parsePayload(raw []byte) (Payload, error) {
	if !json.Valid(raw) {
		return Payload{}, errInvalid()
	}
	var p Payload
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	if err := dec.Decode(&p); err != nil {
		return Payload{}, errInvalid()
	}
	return p, nil
}

func supportedProduct(code string) bool {
	switch strings.TrimSpace(code) {
	case ProductReportServerProfessional, ProductReportDesignerWeb, ProductReportDesignerDesktop:
		return true
	default:
		return false
	}
}

func validLicenseID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > maxLicenseIDLen {
		return false
	}
	if strings.ContainsAny(id, " \t\n\r") {
		return false
	}
	for _, r := range id {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}

func parseLicenseTime(raw string) (time.Time, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return time.Time{}, errInvalid()
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, errInvalid()
	}
	return t.UTC(), nil
}
