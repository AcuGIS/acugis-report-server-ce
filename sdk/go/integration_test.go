package report_test

import (
	"context"
	"os"
	"testing"

	report "github.com/acugis/acugis-report-go"
)

func TestIntegrationLiveServer(t *testing.T) {
	baseURL := os.Getenv("ACUGIS_REPORT_TEST_URL")
	token := os.Getenv("ACUGIS_REPORT_TEST_TOKEN")
	if baseURL == "" || token == "" {
		t.Skip("set ACUGIS_REPORT_TEST_URL and ACUGIS_REPORT_TEST_TOKEN to run integration tests")
	}

	client, err := report.NewClient(baseURL, report.WithToken(token))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	me, err := client.Auth.Me(ctx)
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if me.Username == "" && !me.AuthDisabled {
		t.Fatalf("unexpected me: %+v", me)
	}

	list, err := client.Reports.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	t.Logf("reports=%d user=%s", list.Count, me.Username)

	if len(list.Items) > 0 {
		key := list.Items[0].Key
		bin, err := client.Reports.Run(ctx, key, &report.RunOptions{Format: "pdf"})
		if err != nil {
			t.Fatalf("Run(%s): %v", key, err)
		}
		if len(bin.Data) == 0 {
			t.Fatal("empty report bytes")
		}
	}
}
