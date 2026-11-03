package bigquery

import (
	"context"
	"fmt"
	"log"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
)

type queryStats struct {
	BytesProcessed int64
}

// FetchTypedRows executes GoogleSQL for Jasper collection fill.
func FetchTypedRows(cfg Config, sql string, params map[string]any, limit int) ([]map[string]any, error) {
	ctx, cancel := withTimeout(context.Background(), 180*time.Second)
	defer cancel()
	client, err := openClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	if limit <= 0 {
		limit = 5000
	}
	rows, _, err := executeTypedRows(ctx, client, cfg, sql, params, limit)
	return rows, err
}

func executeTypedRows(ctx context.Context, client *bigquery.Client, cfg Config, sql string, params map[string]any, limit int) ([]map[string]any, *queryStats, error) {
	if err := ensureReadOnly(sql); err != nil {
		return nil, nil, err
	}
	bound, err := bindParameters(sql, params)
	if err != nil {
		return nil, nil, err
	}

	q := client.Query(bound.SQL)
	q.Parameters = bound.Parameters
	q.UseLegacySQL = false
	if ds := DefaultDatasetFrom(cfg); ds != "" {
		q.DefaultDatasetID = ds
	}
	if loc := LocationFrom(cfg); loc != "" {
		q.Location = loc
	}

	start := time.Now()
	log.Printf("[acugis-bigquery] fill query start project=%s limit=%d", ProjectIDFrom(cfg), limit)

	job, err := q.Run(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("BigQuery query failed: %w", sanitizeCredError(err))
	}
	status, err := job.Wait(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("BigQuery query failed: %w", sanitizeCredError(err))
	}
	if err := status.Err(); err != nil {
		return nil, nil, fmt.Errorf("BigQuery query failed: %w", sanitizeCredError(err))
	}

	stats := &queryStats{}
	if status.Statistics != nil {
		stats.BytesProcessed = status.Statistics.TotalBytesProcessed
	}

	it, err := job.Read(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("BigQuery query failed: %w", sanitizeCredError(err))
	}

	var out []map[string]any
	var schema bigquery.Schema
	for {
		if limit > 0 && len(out) >= limit {
			break
		}
		var row []bigquery.Value
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, stats, fmt.Errorf("BigQuery read failed: %w", sanitizeCredError(err))
		}
		if schema == nil {
			schema = it.Schema
		}
		m := make(map[string]any, len(schema))
		for i, f := range schema {
			if i < len(row) {
				m[f.Name] = convertValue(row[i])
			}
		}
		out = append(out, m)
	}
	log.Printf("[acugis-bigquery] fill query done duration=%s rows=%d bytes=%d",
		time.Since(start), len(out), stats.BytesProcessed)
	return out, stats, nil
}
