# Regenerate sdk/openapi.bundle.yaml from go-report-server OpenAPI sources.
$ErrorActionPreference = "Stop"
$Root = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Set-Location (Join-Path $Root "go-report-server")

go run ./scripts/sync-openapi.go .
go run ./scripts/export-openapi.go (Join-Path $Root "sdk\openapi.bundle.yaml")

Write-Host "Wrote $(Join-Path $Root 'sdk\openapi.bundle.yaml')"
Write-Host "Commit the updated bundle when OpenAPI sources change."
