package openapi

import "embed"

// Spec YAML is authored under the repository-root openapi/ directory and
// mirrored into spec/ for embedding (go:embed cannot use ../ paths).
//
// After editing openapi/*.yaml run: make openapi-sync
//
//go:embed all:spec
var embeddedSpec embed.FS
