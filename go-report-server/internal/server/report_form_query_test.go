package server

import "testing"

func TestResolveCassandraReportQueryPriority(t *testing.T) {
	jrxml := "SELECT a FROM ks.t"
	project := "SELECT b FROM ks.t"
	manual := "SELECT c FROM ks.t"

	if got := resolveCassandraReportQuery(jrxml, project, manual); got != jrxml {
		t.Fatalf("JRXML should win: got %q", got)
	}
	if got := resolveCassandraReportQuery("", project, manual); got != project {
		t.Fatalf("project.json should win when JRXML empty: got %q", got)
	}
	if got := resolveCassandraReportQuery("", "", manual); got != manual {
		t.Fatalf("manual override is last resort: got %q", got)
	}
	if got := resolveCassandraReportQuery("  ", "\n", ""); got != "" {
		t.Fatalf("whitespace-only should be empty: got %q", got)
	}
}
