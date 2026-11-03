# Embedding overview

The **JavaScript Embedding SDK** (`@acugis/report-embed`) embeds Report Server reports, dashboards, views, and map reports in your application using a secure iframe and `postMessage` bridge.

This is separate from:

- REST API
- CLI
- Generated language SDKs (Python, .NET, Java, Go)

## Embed tokens are required

Browser embeds **must** use a short-lived **embed token** (`emb_…`):

1. Backend authenticates with a session or Personal Access Token (`jrs_…`).
2. Backend calls `POST /api/v1/embed/token` for the content id.
3. Frontend passes only the `emb_` token to the SDK.

Do **not** put PATs in the browser or iframe URLs.

## Next

- [Quick Start](quickstart.md)
- [Authentication](authentication.md)
- [Security](security.md)
