# Embed parameters

Initial:

```javascript
AcuGIS.embedReport({
  /* … */
  token: embToken,
  parameters: {
    p_country: "USA",
    p_state: ["CA", "TX", "NY"],
    p_year: 2026,
  },
});
```

Runtime:

```javascript
report.setParameters({ p_country: "Canada" });
report.getParameters();
```

Parameter names and values follow Report Server / Jasper parameter handling (same as the interactive viewer).

Values passed here are **interactive**. They are not a security boundary. A bound parameter (below) cannot be changed with `parameters` or `setParameters()`.

## Bound Parameters

Bound parameters are report parameters whose values are signed into the embed token and enforced by Report Server before Jasper, map queries, parameter lookups, and report-query execution.

They are not row-level security and they are not a tenant database. They are secure parameter binding for an embedded report.

Hiding a parameter control is not sufficient. The server overwrites the browser value with the signed value, and injects the signed value when the browser omits it.

### Host-bound vs interactive

| Kind | Example | Who sets it | Can the browser change it? |
|------|---------|-------------|----------------------------|
| Host-bound | `account_id` | Property Pro **backend**, inside the embed token | No. Mint a new token. |
| Interactive | `year`, `property_id` | Report viewer or `AcuGIS.embedReport({ parameters })` | Yes. |

### Property Pro

The host backend holds the Personal Access Token (`jrs_…`). The browser receives only the short-lived `emb_…` token. Do not put the PAT in JavaScript.

Server:

```http
POST /api/v1/embed/token
Authorization: Bearer jrs_…
Content-Type: application/json

{
  "content_type": "report",
  "content_id": "portfolio-performance",
  "origin": "https://property-pro.example.com",
  "parameters": {
    "account_id": "smith-residential"
  }
}
```

Browser:

```javascript
AcuGIS.embedReport({
  server: "https://reports.example.com",
  target: "#acugis-report-container",
  reportId: "portfolio-performance",
  token: embedToken,
  parameters: {
    year: 2026
  }
});
```

Switching the Property Pro company from Smith Residential to ABC Property Management means minting a **new** embed token with `account_id` set to `abc-property`. Do not call `setParameters({ account_id: "abc-property" })`.

The embedded viewer does not show an editable control for `account_id`. `year` and `property_id` still appear when the report defines them and they are not bound.

Sessions and personal access tokens are unchanged. Bound-parameter enforcement applies only when the request authenticated with an embed token that contains bound parameters. An embed token is also limited to its `content_id`: a token for `portfolio-performance` cannot execute a different report.

### What can be bound

`parameters` is a JSON object. Each value is a string, number, boolean, or an array of those. Numbers are kept as their JSON literal (`2026`, not a binary float). An empty object or an omitted field means no bound parameters, which is the previous token behavior.

A name is rejected when it:

- is empty, longer than 64 characters, or not `^[A-Za-z][A-Za-z0-9_]*$`
- starts with `_` (`_repName`, `_repFormat`, `_dataSource`, `_collectionData`, and any other system key)
- is a transport name, case-insensitively: `datasource`, `report`, `et`, `embed`, `toolbar`, `parameters`, `export`, `zoom`, `close`, `autoresize`, `pagination`
- is a Jasper built-in, case-insensitively: any `REPORT_*` name, plus `SORT_FIELDS`, `FILTER`, `SUBREPORT_DIR`, `P_MAP_IMAGE`

At most 32 names, 32 values each, and 2048 characters per value. Objects, nulls, and nested arrays are rejected.

The signed claim field is `parameters` (a map of string arrays). Tokens issued before this field existed still validate.
