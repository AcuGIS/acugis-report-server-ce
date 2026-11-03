# Parameters

Report **parameters** are inputs defined in JRXML and supplied when a report runs. In the viewer they appear as form fields; in automation they are passed as named values.

## Why parameters matter

Parameters let one report definition serve many scenarios — invoice ID, date range, region, customer, and so on — without republishing JRXML for every variation.

## How they appear in the UI

When you open a report in the viewer, Report Server loads config metadata that describes available parameters (names, types, and lookup-backed lists when configured). Cascading parameters can depend on earlier choices via parameter lookup.

## Supplying values

| Context | How values are provided |
|---------|-------------------------|
| Viewer | Form fields before run |
| Multi-value | Multiple selections for the same parameter name |
| Map Reports | Spatial selection/extent can derive parameters before fill |

Keep JRXML parameter names stable. Prefer explicit types in JRXML so invalid values fail clearly at fill time.

## Parameter lookup

Dynamic lists (for cascading inputs) are backed by lookup queries associated with the report. Dependent values are sent when resolving a child parameter list.

## Map Report parameters

On Report Server Professional, Map Report flows can derive spatial parameters from the interactive map before the Jasper report runs. The Open Source server does not implement Map Reports.

## Tips

- Document expected parameter names for report consumers
- Prefer explicit JRXML types
- Treat gateway execute errors as operational messages (often plain text)

## Related

- [Reports](reports.md)
- [Report execution](report-execution.md)
- Map Reports are available in Professional and are not part of this Open Source guide.

---

## For Developers

| Integration path | Go here |
|------------------|---------|
| REST / execute gateway | [REST API](../developer/rest-api/overview.md) · [Parameters & execute](../developer/rest-api/parameters.md) |
| CLI `--param` | [CLI reports](../developer/cli/reports.md) |
| SDK maps | [SDK overview](../developer/sdk/overview.md) |
