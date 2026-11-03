package datasources

import (
	"strings"
	"testing"
	"time"
)

func TestOracleDSN_ServiceName(t *testing.T) {
	dsn, err := oracleDSN(&Datasource{
		Host: "ora.example", Port: 1521, DBName: "ORCLPDB1",
		Username: "app", Password: "secret",
	}, 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dsn, "ora.example:1521/ORCLPDB1") {
		t.Fatalf("service DSN=%q", dsn)
	}
	if strings.Contains(dsn, "SID=") {
		t.Fatalf("unexpected SID in service DSN=%q", dsn)
	}
}

func TestOracleDSN_SID(t *testing.T) {
	dsn, err := oracleDSN(&Datasource{
		Host: "ora.example", Port: 1521, DBName: "sid:ORCL",
		Username: "app", Password: "secret",
	}, 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dsn, "SID=ORCL") {
		t.Fatalf("SID DSN=%q", dsn)
	}
}
