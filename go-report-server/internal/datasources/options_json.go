package datasources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// EnsureOptionsColumn adds the options jsonb column when missing.
func (r *Repository) EnsureOptionsColumn(ctx context.Context) error {
	if !r.Configured() {
		return ErrUnavailable
	}
	_, err := r.pool.Exec(ctx, `
ALTER TABLE `+r.qualified("jasper_datasource")+`
ADD COLUMN IF NOT EXISTS options jsonb NOT NULL DEFAULT '{}'::jsonb`)
	if err != nil {
		return nil
	}
	return nil
}

func encodeOptions(opts map[string]string) []byte {
	if len(opts) == 0 {
		return []byte("{}")
	}
	b, err := json.Marshal(opts)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func decodeOptions(raw []byte) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	var out map[string]string
	if err := json.Unmarshal(raw, &out); err != nil {
		var anyMap map[string]any
		if err2 := json.Unmarshal(raw, &anyMap); err2 != nil {
			return nil
		}
		out = make(map[string]string, len(anyMap))
		for k, v := range anyMap {
			if v == nil {
				continue
			}
			out[k] = strings.TrimSpace(fmt.Sprint(v))
		}
		return out
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (r *Repository) loadOptions(ctx context.Context, name string) map[string]string {
	_ = r.EnsureOptionsColumn(ctx)
	var raw []byte
	err := r.pool.QueryRow(ctx, `
SELECT COALESCE(options, '{}'::jsonb) FROM `+r.qualified("jasper_datasource")+`
WHERE enabled = true AND name = $1 LIMIT 1`, name).Scan(&raw)
	if err != nil {
		return nil
	}
	return decodeOptions(raw)
}
