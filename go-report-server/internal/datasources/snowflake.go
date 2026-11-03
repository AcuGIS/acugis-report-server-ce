package datasources

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SnowflakeOpener opens Snowflake connections for Dataset Intelligence.
type SnowflakeOpener interface {
	OpenSnowflake(ctx context.Context, ds *Datasource) (*sql.DB, error)
}

// snowflakeOpen is installed by sfdriver.Register. This package does not
// import gosnowflake: Designer links gosnowflake v2, and both modules
// register the database/sql driver name "snowflake".
var snowflakeOpen func(ctx context.Context, ds *Datasource, timeout time.Duration) (*sql.DB, error)

// SetSnowflakeOpen installs the Snowflake opener used by PGConnector.
func SetSnowflakeOpen(fn func(ctx context.Context, ds *Datasource, timeout time.Duration) (*sql.DB, error)) {
	snowflakeOpen = fn
}

// OpenSnowflake opens one Snowflake *sql.DB. Caller must Close it.
func (c PGConnector) OpenSnowflake(ctx context.Context, ds *Datasource) (*sql.DB, error) {
	if snowflakeOpen == nil {
		return nil, fmt.Errorf("snowflake driver is not registered")
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	return snowflakeOpen(ctx, ds, timeout)
}
