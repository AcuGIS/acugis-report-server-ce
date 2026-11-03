package cassandra

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
)

// ContactPoints parses one or more hosts from cfg.Host (comma / whitespace separated).
func ContactPoints(cfg Config) []string {
	raw := strings.ReplaceAll(cfg.Host, ";", ",")
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Strip accidental port from host entry; Port field is authoritative.
		if h, _, err := net.SplitHostPort(p); err == nil && h != "" {
			p = h
		}
		key := strings.ToLower(p)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	return out
}

func portOf(cfg Config) int {
	if cfg.Port > 0 {
		return cfg.Port
	}
	return 9042
}

func openSession(cfg Config) (*gocql.Session, error) {
	hosts := ContactPoints(cfg)
	if len(hosts) == 0 {
		return nil, fmt.Errorf("unable to contact Cassandra host: hosts are required")
	}
	port := portOf(cfg)
	cluster := gocql.NewCluster(hosts...)
	cluster.Port = port
	cluster.Timeout = 15 * time.Second
	cluster.ConnectTimeout = 10 * time.Second
	cluster.Consistency = gocql.LocalOne
	cluster.NumConns = 2
	if ks := strings.TrimSpace(cfg.Database); ks != "" {
		cluster.Keyspace = ks
	}
	if dc := LocalDatacenter(cfg); dc != "" {
		cluster.PoolConfig.HostSelectionPolicy = gocql.TokenAwareHostPolicy(gocql.DCAwareRoundRobinPolicy(dc))
	} else {
		cluster.PoolConfig.HostSelectionPolicy = gocql.TokenAwareHostPolicy(gocql.RoundRobinHostPolicy())
	}
	if user := strings.TrimSpace(cfg.Username); user != "" {
		cluster.Authenticator = gocql.PasswordAuthenticator{
			Username: user,
			Password: cfg.Password,
		}
	}

	log.Printf("[acugis-cassandra] connecting hosts=%s port=%d keyspace=%q datacenter=%q user=%q",
		strings.Join(hosts, ","), port, cfg.Database, LocalDatacenter(cfg), cfg.Username)

	session, err := cluster.CreateSession()
	if err != nil {
		return nil, friendlyErr(err)
	}
	return session, nil
}

func hostPortLabel(cfg Config) string {
	hosts := ContactPoints(cfg)
	if len(hosts) == 0 {
		return ":" + strconv.Itoa(portOf(cfg))
	}
	if len(hosts) == 1 {
		return fmt.Sprintf("%s:%d", hosts[0], portOf(cfg))
	}
	return fmt.Sprintf("%s(+%d):%d", hosts[0], len(hosts)-1, portOf(cfg))
}
