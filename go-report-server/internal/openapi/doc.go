// Package openapi embeds and serves the modular OpenAPI 3.0 specification.
//
// Source files live in the repository-root openapi/ directory. At serve time the
// modules are merged into a single document so clients and Swagger UI receive a
// self-contained /api/openapi.yaml and /api/openapi.json.
package openapi
