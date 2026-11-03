// Package cassandra provides native CQL (gocql) and DataStax Astra Data API
// clients for Report Server collection fill. Logic mirrors Report Designer
// (REMOVED_PROVIDER/cassandra).
package cassandra

import (
	"fmt"
	"strings"
)

// Config holds Cassandra / Astra connection settings using Designer field mapping:
// host (contact points or Astra API endpoint), port, database (keyspace),
// username/password (token for Astra), options.connectionMethod / localDatacenter.
type Config struct {
	Host     string
	Port     int
	Database string
	Username string
	Password string
	Options  map[string]string
}

// Option keys (Designer-compatible camelCase).
const (
	OptionLocalDatacenter    = "localDatacenter"
	OptionShowSystemKS       = "showSystemKeyspaces"
	OptionConnectionMethod   = "connectionMethod"
	ConnectionMethodStandard = "standard"
	ConnectionMethodAstra    = "astra"
)

// IsAstra reports whether the connection uses DataStax Astra Data API (HTTPS).
func IsAstra(cfg Config) bool {
	if cfg.Options == nil {
		return false
	}
	m := strings.ToLower(strings.TrimSpace(cfg.Options[OptionConnectionMethod]))
	return m == ConnectionMethodAstra || m == "datastax" || m == "datastax_astra" || m == "astra_db"
}

// NormalizeConnectionMethod returns "standard" or "astra".
func NormalizeConnectionMethod(v string) string {
	m := strings.ToLower(strings.TrimSpace(v))
	if m == ConnectionMethodAstra || m == "datastax" || m == "datastax_astra" || m == "astra_db" {
		return ConnectionMethodAstra
	}
	return ConnectionMethodStandard
}

// ConnectionMethod returns the normalized method from config options.
func ConnectionMethod(cfg Config) string {
	if cfg.Options == nil {
		return ConnectionMethodStandard
	}
	return NormalizeConnectionMethod(cfg.Options[OptionConnectionMethod])
}

// LocalDatacenter returns the configured local DC (may be empty).
func LocalDatacenter(cfg Config) string {
	if cfg.Options == nil {
		return ""
	}
	return strings.TrimSpace(cfg.Options[OptionLocalDatacenter])
}

// ShowSystemKeyspaces reports whether system keyspaces should be listed.
func ShowSystemKeyspaces(cfg Config) bool {
	if cfg.Options == nil {
		return false
	}
	v := strings.ToLower(strings.TrimSpace(cfg.Options[OptionShowSystemKS]))
	return v == "true" || v == "1" || v == "yes"
}

// TestResult is returned by TestConnection.
type TestResult struct {
	Connected     bool
	Message       string
	Database      string
	Driver        string
	ServerVersion string
	SchemaCount   int
	TableCount    int
}

// FetchTypedRows executes CQL (or Astra Data API find) for Jasper collection fill.
func FetchTypedRows(cfg Config, cql string, params map[string]any, limit int) ([]map[string]any, error) {
	if IsAstra(cfg) {
		return astraFetchTypedRows(cfg, cql, params, limit)
	}
	session, err := openSession(cfg)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	if limit <= 0 {
		limit = 5000
	}
	return executeTypedRows(session, cql, params, limit)
}

// TestConnection verifies connectivity (native Cassandra session or Astra Data API).
func TestConnection(cfg Config) (*TestResult, error) {
	if IsAstra(cfg) {
		return astraTestConnection(cfg)
	}
	session, err := openSession(cfg)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	result := &TestResult{
		Connected: true,
		Database:  cfg.Database,
		Driver:    "apache/cassandra-gocql-driver",
		Message:   "Connected",
	}
	if v := releaseVersion(session); v != "" {
		result.ServerVersion = v
	}
	schemas, err := listKeyspaces(session, ShowSystemKeyspaces(cfg))
	if err == nil {
		result.SchemaCount = len(schemas)
		if ks := strings.TrimSpace(cfg.Database); ks != "" {
			tables, tErr := listTables(session, ks)
			if tErr == nil {
				result.TableCount = len(tables)
			}
			found := false
			all, _ := listKeyspaces(session, true)
			for _, s := range all {
				if strings.EqualFold(s.Name, ks) {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("unknown keyspace: %s", ks)
			}
		} else {
			result.TableCount = countTablesInKeyspaces(session, schemas)
		}
	}
	result.Message = fmt.Sprintf("Connected · %s", hostPortLabel(cfg))
	if result.ServerVersion != "" {
		result.Message += " · Cassandra " + result.ServerVersion
	}
	return result, nil
}
