package bigquery

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
)

// SchemaInfo is a BigQuery dataset (mapped as schema).
type SchemaInfo struct {
	Name string
}

// TableInfo is a BigQuery table or view.
type TableInfo struct {
	Schema string
	Name   string
	Type   string // table | view
}

// ColumnInfo is a BigQuery field.
type ColumnInfo struct {
	Name       string
	DataType   string
	UDTName    string
	IsNullable bool
}

func listDatasets(ctx context.Context, client *bigquery.Client, cfg Config) ([]SchemaInfo, error) {
	it := client.Datasets(ctx)
	var out []SchemaInfo
	prefer := DefaultDatasetFrom(cfg)
	for {
		ds, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list BigQuery datasets: %w", sanitizeCredError(err))
		}
		if ds == nil || ds.DatasetID == "" {
			continue
		}
		out = append(out, SchemaInfo{Name: ds.DatasetID})
	}
	if prefer != "" {
		for i, s := range out {
			if strings.EqualFold(s.Name, prefer) {
				if i > 0 {
					out[0], out[i] = out[i], out[0]
				}
				break
			}
		}
	}
	log.Printf("[acugis-bigquery] listed %d datasets project=%s", len(out), ProjectIDFrom(cfg))
	return out, nil
}

func listTables(ctx context.Context, client *bigquery.Client, datasetID string) ([]TableInfo, error) {
	datasetID = strings.TrimSpace(datasetID)
	if datasetID == "" {
		return nil, fmt.Errorf("dataset is required")
	}
	it := client.Dataset(datasetID).Tables(ctx)
	var out []TableInfo
	for {
		t, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list BigQuery tables: %w", sanitizeCredError(err))
		}
		if t == nil || t.TableID == "" {
			continue
		}
		meta, mErr := t.Metadata(ctx)
		typ := "table"
		if mErr == nil && meta != nil {
			switch meta.Type {
			case bigquery.ViewTable, bigquery.MaterializedView:
				typ = "view"
			default:
				typ = "table"
			}
		}
		out = append(out, TableInfo{
			Schema: datasetID,
			Name:   t.TableID,
			Type:   typ,
		})
	}
	log.Printf("[acugis-bigquery] dataset=%s tables=%d", datasetID, len(out))
	return out, nil
}

func listColumns(ctx context.Context, client *bigquery.Client, datasetID, tableID string) ([]ColumnInfo, error) {
	datasetID = strings.TrimSpace(datasetID)
	tableID = strings.TrimSpace(tableID)
	if datasetID == "" || tableID == "" {
		return nil, fmt.Errorf("dataset and table are required")
	}
	meta, err := client.Dataset(datasetID).Table(tableID).Metadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("BigQuery table metadata: %w", sanitizeCredError(err))
	}
	cols := flattenSchema("", meta.Schema)
	log.Printf("[acugis-bigquery] columns dataset=%s table=%s count=%d", datasetID, tableID, len(cols))
	return cols, nil
}

func flattenSchema(prefix string, schema bigquery.Schema) []ColumnInfo {
	var out []ColumnInfo
	for _, f := range schema {
		if f == nil {
			continue
		}
		name := f.Name
		if prefix != "" {
			name = prefix + "." + f.Name
		}
		typ := typeNameFromField(f)
		out = append(out, ColumnInfo{
			Name:       name,
			DataType:   typ,
			UDTName:    typ,
			IsNullable: !f.Required,
		})
		if f.Type == bigquery.RecordFieldType && len(f.Schema) > 0 && !f.Repeated {
			out = append(out, flattenSchema(name, f.Schema)...)
		}
	}
	return out
}

func countTables(ctx context.Context, client *bigquery.Client, datasets []SchemaInfo) int {
	total := 0
	deadline := time.Now().Add(8 * time.Second)
	for _, ds := range datasets {
		if time.Now().After(deadline) {
			break
		}
		tables, err := listTables(ctx, client, ds.Name)
		if err == nil {
			total += len(tables)
		}
	}
	return total
}

// ListSchemas lists datasets (schemas) for Dataset Intelligence / catalog.
func ListSchemas(cfg Config) ([]SchemaInfo, error) {
	ctx, cancel := withTimeout(context.Background(), 45*time.Second)
	defer cancel()
	client, err := openClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	return listDatasets(ctx, client, cfg)
}

// ListTables lists tables/views in a dataset.
func ListTables(cfg Config, schema string) ([]TableInfo, error) {
	ctx, cancel := withTimeout(context.Background(), 45*time.Second)
	defer cancel()
	client, err := openClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	if strings.TrimSpace(schema) == "" {
		schema = DefaultDatasetFrom(cfg)
	}
	return listTables(ctx, client, schema)
}

// ListColumns lists fields for a table/view.
func ListColumns(cfg Config, schema, table string) ([]ColumnInfo, error) {
	ctx, cancel := withTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := openClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	if strings.TrimSpace(schema) == "" {
		schema = DefaultDatasetFrom(cfg)
	}
	return listColumns(ctx, client, schema, table)
}
