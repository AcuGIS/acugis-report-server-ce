package bigquery

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// TestConnection performs a real authenticated BigQuery operation (list datasets/tables).
func TestConnection(cfg Config) (*TestResult, error) {
	ctx, cancel := withTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := openClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	project := ProjectIDFrom(cfg)
	result := &TestResult{
		Connected: true,
		Database:  project,
		Driver:    "cloud.google.com/go/bigquery",
		Message:   "Connected",
	}

	datasets, err := listDatasets(ctx, client, cfg)
	if err != nil {
		return nil, err
	}
	result.SchemaCount = len(datasets)

	if prefer := DefaultDatasetFrom(cfg); prefer != "" {
		found := false
		for _, d := range datasets {
			if strings.EqualFold(d.Name, prefer) {
				found = true
				tables, tErr := listTables(ctx, client, d.Name)
				if tErr != nil {
					return nil, tErr
				}
				result.TableCount = len(tables)
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("dataset not found: %s", prefer)
		}
	} else {
		result.TableCount = countTables(ctx, client, datasets)
	}

	result.Message = fmt.Sprintf(
		"Connected - project %s\nDatabase: %s\nDriver: %s\nSchemas Found: %d\nTables/Views Found: %d",
		project, project, result.Driver, result.SchemaCount, result.TableCount,
	)
	log.Printf("[acugis-bigquery] test ok project=%s datasets=%d tables=%d",
		project, result.SchemaCount, result.TableCount)
	return result, nil
}
