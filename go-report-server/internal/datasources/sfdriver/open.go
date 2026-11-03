// Package sfdriver opens Snowflake connections with gosnowflake v1.
// Report Server processes call Register. Designer must not import this
// package: it links gosnowflake v2, which uses the same driver name.
package sfdriver

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	sf "github.com/snowflakedb/gosnowflake"
	"github.com/youmark/pkcs8"

	"github.com/acugis/acugis-report-server-go/internal/datasources"
)

// Register installs the Snowflake opener on the datasource connector.
func Register() {
	datasources.SetSnowflakeOpen(open)
}

func open(ctx context.Context, ds *datasources.Datasource, timeout time.Duration) (*sql.DB, error) {
	if ds == nil {
		return nil, datasources.ErrNotFound
	}
	if !ds.IsSnowflake() {
		return nil, fmt.Errorf("%w: not snowflake", datasources.ErrInvalidInput)
	}
	cfg, err := snowflakeConfig(ds)
	if err != nil {
		return nil, err
	}
	dsn, err := sf.DSN(cfg)
	if err != nil {
		return nil, fmt.Errorf("snowflake dsn: %w", err)
	}
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	connectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	db, err := sql.Open("snowflake", dsn)
	if err != nil {
		return nil, fmt.Errorf("connect datasource %q: %w", ds.Name, err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(timeout)
	if err := db.PingContext(connectCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect datasource %q: %w", ds.Name, err)
	}
	return db, nil
}

func snowflakeConfig(ds *datasources.Datasource) (*sf.Config, error) {
	account := snowflakeAccountID(ds.Host)
	user := strings.TrimSpace(ds.Username)
	dbName := strings.TrimSpace(ds.DBName)
	if account == "" || user == "" || dbName == "" {
		return nil, fmt.Errorf("incomplete snowflake connection settings")
	}
	schema := strings.TrimSpace(ds.SchemaName)
	if schema == "" {
		schema = "PUBLIC"
	}
	cfg := &sf.Config{
		Account:  account,
		User:     user,
		Database: dbName,
		Schema:   schema,
	}

	auth := datasources.NormalizeAuthType(ds.AuthType)
	if auth != datasources.AuthKeyPair && strings.TrimSpace(ds.PrivateKeyResourceUUID) != "" && len(ds.PrivateKeyPEM) > 0 {
		auth = datasources.AuthKeyPair
	}

	switch auth {
	case datasources.AuthKeyPair:
		if len(ds.PrivateKeyPEM) == 0 {
			return nil, fmt.Errorf("snowflake key-pair auth requires a resolved private key resource")
		}
		key, err := parseSnowflakePrivateKey(ds.PrivateKeyPEM, strings.TrimSpace(ds.Password))
		if err != nil {
			return nil, err
		}
		cfg.PrivateKey = key
		cfg.Authenticator = sf.AuthTypeJwt
		cfg.Password = ""
	default:
		cfg.Password = ds.Password
	}
	return cfg, nil
}

func parseSnowflakePrivateKey(pemBytes []byte, passphrase string) (*rsa.PrivateKey, error) {
	block, rest := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("snowflake private key: invalid PEM (expected -----BEGIN … PRIVATE KEY-----)")
	}
	_ = rest

	pass := []byte(passphrase)
	typ := strings.ToUpper(strings.TrimSpace(block.Type))

	if typ == "ENCRYPTED PRIVATE KEY" {
		key, err := pkcs8.ParsePKCS8PrivateKey(block.Bytes, pass)
		if err != nil {
			if passphrase == "" {
				return nil, fmt.Errorf("snowflake private key is encrypted; set the key passphrase in the datasource password field")
			}
			return nil, fmt.Errorf("snowflake encrypted private key: %w", err)
		}
		return asRSAPrivateKey(key)
	}

	if x509.IsEncryptedPEMBlock(block) { //nolint:staticcheck
		der, err := x509.DecryptPEMBlock(block, pass) //nolint:staticcheck
		if err != nil {
			if passphrase == "" {
				return nil, fmt.Errorf("snowflake private key is encrypted; set the key passphrase in the datasource password field")
			}
			return nil, fmt.Errorf("snowflake private key decrypt: %w", err)
		}
		return parseRSAPrivateKeyDER(der)
	}

	if key, err := pkcs8.ParsePKCS8PrivateKey(block.Bytes, nil); err == nil {
		return asRSAPrivateKey(key)
	}
	return parseRSAPrivateKeyDER(block.Bytes)
}

func parseRSAPrivateKeyDER(der []byte) (*rsa.PrivateKey, error) {
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		return asRSAPrivateKey(key)
	}
	if rsaKey, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return rsaKey, nil
	}
	return nil, fmt.Errorf("snowflake private key: unsupported format (need RSA PKCS#1 or PKCS#8)")
}

func asRSAPrivateKey(key any) (*rsa.PrivateKey, error) {
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok || rsaKey == nil {
		return nil, fmt.Errorf("snowflake private key: not RSA")
	}
	return rsaKey, nil
}

func snowflakeAccountID(host string) string {
	host = strings.TrimSpace(host)
	host = strings.TrimPrefix(strings.ToLower(host), "https://")
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimSuffix(host, "/")
	const suffix = ".snowflakecomputing.com"
	if i := strings.Index(strings.ToLower(host), suffix); i > 0 {
		host = host[:i]
	}
	return host
}
