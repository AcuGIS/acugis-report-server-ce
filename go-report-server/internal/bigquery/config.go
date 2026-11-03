// Package bigquery provides Google BigQuery collection fill for Report Server.
// Logic mirrors AcuGIS Report Designer (internal/provider/bigquery).
package bigquery

import (
	"fmt"
	"strings"
)

const (
	ProviderID = "bigquery"

	OptionCredentialsResourceUUID = "credentialsResourceUuid"
	OptionDefaultDataset          = "defaultDataset"
	OptionLocation                = "location"
	OptionProjectID               = "projectId"
)

// Config holds BigQuery connection settings.
// CredentialsJSON is resolved server-side from a content Resource and must never
// be persisted on the datasource row or returned to the browser.
type Config struct {
	ProjectID       string
	DefaultDataset  string
	Location        string
	CredentialsJSON []byte
	Options         map[string]string
}

// ProjectIDFrom returns the configured GCP project id.
func ProjectIDFrom(cfg Config) string {
	if p := strings.TrimSpace(cfg.ProjectID); p != "" {
		return p
	}
	if cfg.Options != nil {
		if p := strings.TrimSpace(cfg.Options[OptionProjectID]); p != "" {
			return p
		}
	}
	return ""
}

// DefaultDatasetFrom returns the optional default dataset id.
func DefaultDatasetFrom(cfg Config) string {
	if d := strings.TrimSpace(cfg.DefaultDataset); d != "" {
		return d
	}
	if cfg.Options == nil {
		return ""
	}
	return strings.TrimSpace(cfg.Options[OptionDefaultDataset])
}

// LocationFrom returns the optional BigQuery location (e.g. US, EU, us-central1).
func LocationFrom(cfg Config) string {
	if loc := strings.TrimSpace(cfg.Location); loc != "" {
		return loc
	}
	if cfg.Options == nil {
		return ""
	}
	return strings.TrimSpace(cfg.Options[OptionLocation])
}

// CredentialsResourceUUID returns the Service Account JSON resource UUID.
func CredentialsResourceUUID(opts map[string]string) string {
	if opts == nil {
		return ""
	}
	return strings.TrimSpace(opts[OptionCredentialsResourceUUID])
}

// TestResult is returned by TestConnection.
type TestResult struct {
	Connected   bool
	Message     string
	Database    string
	Driver      string
	SchemaCount int
	TableCount  int
}

func validateConfig(cfg Config) error {
	if ProjectIDFrom(cfg) == "" {
		return fmt.Errorf("BigQuery project ID is required")
	}
	if len(cfg.CredentialsJSON) == 0 {
		return fmt.Errorf("service account JSON credentials are required")
	}
	if !looksLikeServiceAccountJSON(string(cfg.CredentialsJSON)) {
		return fmt.Errorf("invalid service account credential")
	}
	return nil
}

func looksLikeServiceAccountJSON(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "{") && strings.Contains(s, "private_key") &&
		(strings.Contains(s, "client_email") || strings.Contains(s, "type"))
}
