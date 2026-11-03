# Release notes

This page is not a product changelog. The repository does not contain a historical list of product releases that this site can publish. Nothing below is a version history of Report Server.

## What a running server reports

The process reports `JRS_APP_VERSION`. That value is also OpenAPI `info.version` at `/api/openapi.yaml`. The compiled fallback, when the variable is unset and the binary was not stamped at build time, is `1.0.0`. Packages and images use the version the build supplies. Do not treat `1.0.0` as the version of every installation.

## Documentation site

The MkDocs pages under `documentation/docs/` describe the current tree. A change to those pages is a documentation change. It is not a product release.

When a product release is published, use the version on the server and the Git tag or GitHub release that produced that build. This page will not invent releases from git history.
