# Embed events and methods

## Events

```javascript
report.on("load", () => {});
report.on("error", (e) => console.error(e.code, e.message));
report.on("parameterChange", (e) => console.log(e.parameters));
report.on("pageChange", (e) => {});
report.on("selection", (e) => {});
report.on("filterChange", (e) => {});
report.on("refresh", () => {});
report.on("export", (e) => {});
report.on("resize", (e) => {});
```

Error codes include: `AUTH_REQUIRED`, `ACCESS_DENIED`, `CONTENT_NOT_FOUND`, `INVALID_PARAMETER`, `SERVER_ERROR`, `EMBED_ORIGIN_DENIED`, `TOKEN_EXPIRED`, `TOKEN_INVALID`.

## Methods

| Method | Notes |
|--------|--------|
| `ready()` | Promise resolved on viewer `load` |
| `setParameters` / `getParameters` | |
| `refresh` / `resize` / `setHeight` / `setWidth` | |
| `export(format)` | pdf, xlsx, csv, … via Report Server |
| `zoomIn` / `zoomOut` / `setZoom` | Report viewer |
| `nextPage` / `previousPage` / `goToPage` | When supported by viewer |
| `on` / `off` / `destroy` | |

## Responsive

```javascript
AcuGIS.embedReport({
  /* … */
  token: embToken,
  width: "100%",
  height: "800px",
  autoResize: true,
});
```

With `autoResize`, the viewer posts dimension updates over the validated `postMessage` channel.
