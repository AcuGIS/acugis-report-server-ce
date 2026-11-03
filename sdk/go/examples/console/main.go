package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	report "github.com/acugis/acugis-report-go"
)

func main() {
	client, err := report.NewClientFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "client: %v\n", err)
		os.Exit(1)
	}
	ctx := context.Background()

	me, err := client.Auth.Me(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "auth/me: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Authenticated as %s (admin=%v)\n", me.Username, me.IsAdmin)

	list, err := client.Reports.List(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reports: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Found %d reports\n", list.Count)
	if len(list.Items) == 0 {
		fmt.Println("No reports available to run.")
		return
	}

	key := list.Items[0].Key
	out := filepath.Join(".", key+".pdf")
	res, err := client.Reports.RunToFile(ctx, key, out, &report.RunOptions{Format: "pdf"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "run: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %s (%d bytes, %s)\n", out, len(res.Data), res.ContentType)
}
