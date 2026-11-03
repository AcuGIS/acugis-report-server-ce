package main

import (
	"fmt"
	"os"

	"github.com/acugis/acugis-report-server-go/internal/openapi"
)

func main() {
	out := "openapi.bundle.yaml"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	y, err := openapi.MarshalYAML("0.1.0-foundation")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, y, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	doc, err := openapi.Bundle("0.1.0-foundation")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := openapi.ValidateBasic(doc); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	paths, _ := doc["paths"].(map[string]any)
	fmt.Printf("wrote %s (%d bytes, %d paths)\n", out, len(y), len(paths))
}
