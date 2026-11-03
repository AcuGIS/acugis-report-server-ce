#!/usr/bin/env bash
# Regenerate sdk/openapi.bundle.yaml from go-report-server OpenAPI sources.
# Run from the repository root (or any cwd; script resolves paths itself).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT/go-report-server"

go run ./scripts/sync-openapi.go .
go run ./scripts/export-openapi.go "$ROOT/sdk/openapi.bundle.yaml"

echo "Wrote $ROOT/sdk/openapi.bundle.yaml"
echo "Commit the updated bundle when OpenAPI sources change."
