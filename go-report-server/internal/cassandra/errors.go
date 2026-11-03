package cassandra

import (
	"fmt"
	"strings"
)

// friendlyErr maps driver / network errors to clear Designer messages.
// Never includes passwords or credential material.
func friendlyErr(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "authentication"), strings.Contains(lower, "credentials"),
		strings.Contains(lower, "bad credentials"), strings.Contains(lower, "unauthorized"):
		return fmt.Errorf("authentication failed")
	case strings.Contains(lower, "local datacenter"), strings.Contains(lower, "no local dc"),
		strings.Contains(lower, "datacenter"):
		if strings.Contains(lower, "local") || strings.Contains(lower, "dc") {
			return fmt.Errorf("invalid local datacenter: %s", shortErr(msg))
		}
	case strings.Contains(lower, "keyspace"), strings.Contains(lower, "does not exist"):
		if strings.Contains(lower, "keyspace") {
			return fmt.Errorf("keyspace does not exist: %s", shortErr(msg))
		}
	case strings.Contains(lower, "no hosts available"), strings.Contains(lower, "unable to connect to any hosts"),
		strings.Contains(lower, "no connections were made"):
		return fmt.Errorf("no hosts available: %s", shortErr(msg))
	case strings.Contains(lower, "no such host"), strings.Contains(lower, "lookup"),
		strings.Contains(lower, "unknown host"):
		return fmt.Errorf("unable to contact Cassandra host: %s", shortErr(msg))
	case strings.Contains(lower, "connection refused"), strings.Contains(lower, "connectex"),
		strings.Contains(lower, "dial tcp"), strings.Contains(lower, "i/o timeout"),
		strings.Contains(lower, "deadline exceeded"), strings.Contains(lower, "timeout"):
		if strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline") {
			return fmt.Errorf("connection timed out: %s", shortErr(msg))
		}
		return fmt.Errorf("unable to contact Cassandra host: %s", shortErr(msg))
	}
	return fmt.Errorf("cannot connect to Cassandra: %s", shortErr(msg))
}

func friendlyQueryErr(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "deadline exceeded"):
		return fmt.Errorf("query timeout: %s", shortErr(msg))
	case strings.Contains(lower, "unconfigured table"), strings.Contains(lower, "table") && strings.Contains(lower, "doesn't exist"):
		return fmt.Errorf("unknown table: %s", shortErr(msg))
	case strings.Contains(lower, "keyspace") && (strings.Contains(lower, "does not exist") || strings.Contains(lower, "unknown")):
		return fmt.Errorf("unknown keyspace: %s", shortErr(msg))
	case strings.Contains(lower, "syntax error"), strings.Contains(lower, "invalidquery"),
		strings.Contains(lower, "invalid query"):
		return fmt.Errorf("invalid CQL: %s", shortErr(msg))
	case strings.Contains(lower, "partition key"), strings.Contains(lower, "need to provide"),
		strings.Contains(lower, "filtering"), strings.Contains(lower, "allow filtering"):
		return fmt.Errorf("%s", shortErr(msg))
	case strings.Contains(lower, "authentication"), strings.Contains(lower, "unauthorized"):
		return fmt.Errorf("authentication failed")
	default:
		return fmt.Errorf("%s", shortErr(msg))
	}
}

func shortErr(msg string) string {
	msg = strings.TrimSpace(msg)
	// Redact accidental credential fragments.
	lower := strings.ToLower(msg)
	if i := strings.Index(lower, "password"); i >= 0 {
		msg = msg[:i] + "[redacted]"
	}
	if len(msg) > 280 {
		return msg[:280] + "…"
	}
	return msg
}
