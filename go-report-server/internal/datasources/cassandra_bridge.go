package datasources

import (
	"github.com/acugis/acugis-report-server-go/internal/cassandra"
)

// CassandraConfig maps a catalog Datasource onto the Cassandra client Config
// using Designer-compatible field conventions.
func CassandraConfig(ds Datasource) cassandra.Config {
	opts := map[string]string{}
	for k, v := range ds.Options {
		opts[k] = v
	}
	if _, ok := opts[cassandra.OptionConnectionMethod]; !ok {
		opts[cassandra.OptionConnectionMethod] = ds.CassandraConnectionMethod()
	}
	return cassandra.Config{
		Host:     ds.Host,
		Port:     ds.Port,
		Database: ds.DBName,
		Username: ds.Username,
		Password: ds.Password,
		Options:  opts,
	}
}
