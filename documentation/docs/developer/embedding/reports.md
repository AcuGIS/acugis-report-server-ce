# Embed reports

```javascript
const report = AcuGIS.embedReport({
  server: "https://reports.example.com",
  target: "#report",
  reportId: "Invoice",
  token: embToken,
  parameters: { p_country: "USA" },
  viewer: { toolbar: true, parameters: true, export: true },
});

await report.ready();
report.setParameters({ p_country: "Canada" });
report.setZoom(125);
report.export("pdf");
report.on("parameterChange", (e) => console.log(e.parameters));
report.on("error", (e) => console.error(e.code, e.message));
```

`reportId` is the Report Server report **key** (not a filesystem UUID).

Reuse the existing viewer at `/embed/report/{key}` — the SDK does not render reports itself.
