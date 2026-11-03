package acl

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/acugis/acugis-report-server-go/internal/reports"
)

// SyncStats mirrors PHP jrs_sync_all_jasper_report_resources return shape.
type SyncStats struct {
	OK            bool   `json:"ok"`
	Registered    int    `json:"registered"`
	AlreadyExists int    `json:"already_exists"`
	Errors        int    `json:"errors"`
	Skipped       bool   `json:"skipped"`
	Reason        string `json:"reason,omitempty"`
}

// ResourceSyncer backfills report resources into the catalog.
type ResourceSyncer interface {
	SyncAllReportResources(ctx context.Context, store *reports.Store) SyncStats
}

// SyncAllReportResources mirrors PHP jrs_sync_all_jasper_report_resources.
func (r *Repository) SyncAllReportResources(ctx context.Context, store *reports.Store) SyncStats {
	if !r.Configured() {
		return SyncStats{
			OK:      true,
			Skipped: true,
			Reason:  "no central connection",
		}
	}
	if store == nil {
		return SyncStats{OK: false, Errors: 1, Reason: "report store unavailable"}
	}

	items, err := store.List()
	if err != nil {
		return SyncStats{OK: false, Errors: 1, Reason: err.Error()}
	}

	verbose := resourceSyncVerbose()
	logger := slog.Default()
	logger.Info("resource sync starting", "count", len(items))

	stats := SyncStats{}
	for _, item := range items {
		key := strings.TrimSpace(item.Key)
		if key == "" {
			continue
		}
		title := strings.TrimSpace(item.Name)
		if title == "" {
			title = key
		}

		existed, err := r.jasperResourceExists(ctx, key)
		if err != nil {
			stats.Errors++
			logger.Warn("resource sync exists check failed", "report", key, "error", err)
			continue
		}
		id, err := r.registerResource(ctx, key, title, 0)
		if err != nil || id <= 0 {
			stats.Errors++
			logger.Warn("resource sync register failed", "report", key, "error", err)
			continue
		}
		if existed {
			if verbose {
				logger.Info("resource already exists", "report", key)
			}
			stats.AlreadyExists++
		} else {
			logger.Info("registered jasper_report", "report", key)
			r.grantDefaultReportPermissions(ctx, id)
			stats.Registered++
		}

		accessNames := accessGroupsFromSummary(store, key)
		_ = r.SyncReportAccessGroups(ctx, key, title, accessNames, 0)
	}

	_ = r.ensurePlatformServiceResources(ctx)
	stats.OK = stats.Errors == 0
	logger.Info("resource sync complete",
		"registered", stats.Registered,
		"already_exists", stats.AlreadyExists,
		"errors", stats.Errors,
	)
	return stats
}

func (r *Repository) jasperResourceExists(ctx context.Context, resourceKey string) (bool, error) {
	var one int
	err := r.pool.QueryRow(ctx, `
SELECT 1 FROM resources
WHERE resource_type = $1 AND resource_key = $2
LIMIT 1
`, ResourceTypeJasperReport, resourceKey).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) grantDefaultReportPermissions(ctx context.Context, resourceID int64) {
	_ = r.grantGroupResourcePermission(ctx, resourceID, GroupAdministrators, PermissionAdmin)
	_ = r.grantGroupResourcePermission(ctx, resourceID, GroupEditors, PermissionEdit)
	_ = r.grantGroupResourcePermission(ctx, resourceID, GroupPublic, PermissionView)
}

func accessGroupsFromSummary(store *reports.Store, key string) []string {
	def, err := store.Get(key)
	if err != nil || def == nil {
		return []string{GroupPublic}
	}
	raw, _ := def.Raw["access_groups"].(string)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{GroupPublic}
	}
	parts := strings.Split(raw, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{GroupPublic}
	}
	return out
}

func resourceSyncVerbose() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("JRS_RESOURCE_SYNC_VERBOSE")))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

var _ ResourceSyncer = (*Repository)(nil)
