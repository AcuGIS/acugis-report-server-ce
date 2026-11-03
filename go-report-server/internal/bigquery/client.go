package bigquery

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/option"
)

func openClient(ctx context.Context, cfg Config) (*bigquery.Client, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	project := ProjectIDFrom(cfg)
	client, err := bigquery.NewClient(ctx, project, option.WithCredentialsJSON(cfg.CredentialsJSON))
	if err != nil {
		return nil, fmt.Errorf("BigQuery authentication failed: %w", sanitizeCredError(err))
	}
	if loc := LocationFrom(cfg); loc != "" {
		client.Location = loc
	}
	return client, nil
}

func withTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, d)
}

// sanitizeCredError strips credential material from Google API errors.
func sanitizeCredError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "private_key") || strings.Contains(msg, "BEGIN PRIVATE KEY") {
		return fmt.Errorf("invalid service account credential")
	}
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "permission") || strings.Contains(lower, "403") || strings.Contains(lower, "accessdenied"):
		return fmt.Errorf("permission denied: %s", truncateErr(msg, 180))
	case strings.Contains(lower, "not found") || strings.Contains(lower, "404"):
		return fmt.Errorf("project not accessible: %s", truncateErr(msg, 180))
	case strings.Contains(lower, "has not been used") || strings.Contains(lower, "api has not been enabled") || strings.Contains(lower, "accessNotConfigured"):
		return fmt.Errorf("BigQuery API disabled: %s", truncateErr(msg, 180))
	case strings.Contains(lower, "invalid_grant") || strings.Contains(lower, "invalid jwt") || strings.Contains(lower, "oauth2"):
		return fmt.Errorf("invalid service account credential")
	default:
		return err
	}
}

func truncateErr(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
