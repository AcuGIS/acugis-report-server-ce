package cassandra

import (
	"strings"
	"testing"
)

func TestIsAstra(t *testing.T) {
	if IsAstra(Config{}) {
		t.Fatal("empty should be standard")
	}
	if IsAstra(Config{Options: map[string]string{OptionConnectionMethod: ConnectionMethodStandard}}) {
		t.Fatal("standard should not be astra")
	}
	if !IsAstra(Config{Options: map[string]string{OptionConnectionMethod: ConnectionMethodAstra}}) {
		t.Fatal("astra expected")
	}
}

func TestParseAstraTableRef(t *testing.T) {
	ks, table, err := parseAstraTableRef("SELECT * FROM default_keyspace.customers", "default_keyspace")
	if err != nil || ks != "default_keyspace" || table != "customers" {
		t.Fatalf("got %s.%s err=%v", ks, table, err)
	}
	ks, table, err = parseAstraTableRef("SELECT a, b FROM customers WHERE a = $P{p_a}", "default_keyspace")
	if err != nil || ks != "default_keyspace" || table != "customers" {
		t.Fatalf("got %s.%s err=%v", ks, table, err)
	}
	_, _, err = parseAstraTableRef("INSERT INTO t (a) VALUES (1)", "default_keyspace")
	if err == nil {
		t.Fatal("expected error for non-SELECT")
	}
}

func TestAstraFilterFromQuery(t *testing.T) {
	f := astraFilterFromQuery(
		"SELECT * FROM ks.t WHERE state = $P{p_state}",
		map[string]any{"p_state": "Texas"},
	)
	eq, ok := f["state"].(map[string]any)
	if !ok || eq["$eq"] != "Texas" {
		t.Fatalf("filter=%v", f)
	}
	f = astraFilterFromQuery(
		"SELECT * FROM ks.t WHERE state IN ($P{p_states})",
		map[string]any{"p_states": []any{"Texas", "Florida"}},
	)
	in, ok := f["state"].(map[string]any)
	if !ok {
		t.Fatalf("filter=%v", f)
	}
	arr, _ := in["$in"].([]any)
	if len(arr) != 2 {
		t.Fatalf("in=%v", in)
	}
}

func TestAstraColumnsFromDefinition(t *testing.T) {
	def := map[string]any{
		"columns": map[string]any{
			"user_id": map[string]any{"type": "uuid"},
			"name":    map[string]any{"type": "text"},
			"tags":    map[string]any{"type": "list", "valueType": "text"},
			"order_id": map[string]any{"type": "uuid"},
		},
		"primaryKey": map[string]any{
			"partitionBy": []any{"user_id"},
			"partitionSort": map[string]any{
				"order_id": float64(1),
			},
		},
	}
	cols := astraColumnsFromDefinition(def)
	infos := toColumnInfos(cols)
	foundPK, foundCK, foundList := false, false, false
	for _, c := range infos {
		if c.Name == "user_id" && c.IsPrimaryKey {
			foundPK = true
		}
		if c.Name == "order_id" && strings.Contains(c.UDTName, "clustering") {
			foundCK = true
		}
		if c.Name == "tags" && strings.HasPrefix(c.DataType, "list") {
			foundList = true
		}
	}
	if !foundPK || !foundCK || !foundList {
		t.Fatalf("cols=%+v", infos)
	}
}

func TestAstraKeyspaceDefault(t *testing.T) {
	if AstraKeyspace(Config{}) != "default_keyspace" {
		t.Fatal("default keyspace")
	}
	if AstraKeyspace(Config{Database: "sales"}) != "sales" {
		t.Fatal("custom keyspace")
	}
}

func TestNewAstraClientValidation(t *testing.T) {
	_, err := newAstraClient(Config{
		Host:     "http://insecure.example.com",
		Password: "AstraCS:x",
		Options:  map[string]string{OptionConnectionMethod: ConnectionMethodAstra},
	})
	if err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("expected https error, got %v", err)
	}
	_, err = newAstraClient(Config{
		Host:    "https://db.apps.astra.datastax.com",
		Options: map[string]string{OptionConnectionMethod: ConnectionMethodAstra},
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "token") {
		t.Fatalf("expected token error, got %v", err)
	}
}
