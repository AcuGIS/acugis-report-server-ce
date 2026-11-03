package datasources

import (
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/bigquery"
)

// BigQueryConfig maps a catalog Datasource onto the BigQuery client Config.
// CredentialsJSON must be attached by the caller after resolving the credential Resource.
func BigQueryConfig(ds Datasource, credentialsJSON []byte) bigquery.Config {
	opts := map[string]string{}
	for k, v := range ds.Options {
		opts[k] = v
	}
	project := strings.TrimSpace(ds.DBName)
	if project == "" {
		project = strings.TrimSpace(opts[OptionProjectID])
	}
	if project != "" {
		opts[OptionProjectID] = project
	}
	defDS := strings.TrimSpace(opts[OptionDefaultDataset])
	if defDS == "" {
		defDS = strings.TrimSpace(ds.SchemaName)
	}
	if defDS != "" {
		opts[OptionDefaultDataset] = defDS
	}
	return bigquery.Config{
		ProjectID:       project,
		DefaultDataset:  defDS,
		Location:        strings.TrimSpace(opts[OptionLocation]),
		CredentialsJSON: credentialsJSON,
		Options:         opts,
	}
}
