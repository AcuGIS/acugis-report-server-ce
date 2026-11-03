#!/usr/bin/env python3
"""One-shot helper: port reports/view.php into an embedded HTML template."""
from __future__ import annotations

import re
from pathlib import Path
from textwrap import dedent

ROOT = Path(__file__).resolve().parents[2]
php_path = ROOT / "reports" / "view.php"
out_html = ROOT / "go-report-server" / "internal" / "server" / "viewer.html"
out_go = ROOT / "go-report-server" / "internal" / "server" / "viewer_html.go"

php = php_path.read_text(encoding="utf-8")
start = php.index("<!DOCTYPE html>")
html = php[start:]

replacements = [
    (
        "<?php echo jrs_html($reportTitle); ?>",
        "{{.Title}}",
    ),
    (
        '  <?php if ($isMapReport): ?>\n  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/ol@v9.2.4/ol.css">\n  <?php endif; ?>\n',
        '  {{if .IsMap}}\n  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/ol@v9.2.4/ol.css">\n  {{end}}\n',
    ),
    (
        "<?php echo jrs_html($isMapReport ? 'Map Report Viewer' : 'Report Viewer'); ?>",
        "{{if .IsMap}}Map Report Viewer{{else}}Report Viewer{{end}}",
    ),
    (
        '        <?php if ($isMapReport): ?>\n'
        '        <button type="button" id="refreshFromMapBtn" class="btn-map" disabled title="Requires p_geometry and p_srid in the JRXML">Refresh Report from Map</button>\n'
        "        <?php endif; ?>\n",
        "",
    ),
    ("    <?php if ($isMapReport): ?>\n", "    {{if .IsMap}}\n"),
    (
        '    <?php else: ?>\n'
        '    <iframe id="reportFrame" title="Report output"></iframe>\n'
        "    <?php endif; ?>\n",
        "    {{else}}\n"
        '    <iframe id="reportFrame" title="Report output"></iframe>\n'
        "    {{end}}\n",
    ),
    (
        "<?php if ($isMapReport): ?>\n"
        '<script src="https://cdn.jsdelivr.net/npm/ol@v9.2.4/dist/ol.js"></script>\n'
        '<script src="../assets/js/map-style.js"></script>\n'
        "<?php endif; ?>\n",
        "{{if .IsMap}}\n"
        '<script src="https://cdn.jsdelivr.net/npm/ol@v9.2.4/dist/ol.js"></script>\n'
        '<script src="../assets/js/map-style.js"></script>\n'
        '<script src="../assets/js/spatial-selection.js"></script>\n'
        "{{end}}\n",
    ),
    (
        "const reportName = <?php echo json_encode($report, JSON_HEX_TAG | JSON_HEX_AMP | JSON_HEX_APOS | JSON_HEX_QUOT); ?>;",
        "const reportName = {{.ReportJS}};",
    ),
    (
        "const datasource = <?php echo json_encode($datasource, JSON_HEX_TAG | JSON_HEX_AMP | JSON_HEX_APOS | JSON_HEX_QUOT); ?>;",
        "const datasource = {{.DatasourceJS}};",
    ),
    (
        "const reportEndpoint = <?php echo json_encode($reportEndpoint, JSON_HEX_TAG | JSON_HEX_AMP | JSON_HEX_APOS | JSON_HEX_QUOT); ?>;",
        "const reportEndpoint = {{.ReportEndpointJS}};",
    ),
    (
        "const isMapReport = <?php echo $isMapReport ? 'true' : 'false'; ?>;",
        "const isMapReport = {{.IsMapJS}};",
    ),
]

for old, new in replacements:
    if old not in html:
        raise SystemExit(f"pattern not found:\n{old!r}")
    html = html.replace(old, new, 1)

if re.findall(r"<\?php", html):
    raise SystemExit("remaining PHP open tags")
php_vars = [m for m in re.findall(r"\$[a-zA-Z_][a-zA-Z0-9_]*", html) if m != "$X"]
if php_vars:
    raise SystemExit(f"remaining PHP vars: {php_vars[:30]}")

out_html.write_text(html, encoding="utf-8", newline="\n")

out_go.write_text(
    dedent(
        """\
        package server

        import (
        \t_ "embed"
        \t"html/template"
        )

        //go:embed viewer.html
        var viewHTML string

        type viewPageData struct {
        \tTitle            string
        \tIsMap            bool
        \tReportJS         template.JS
        \tDatasourceJS     template.JS
        \tReportEndpointJS template.JS
        \tIsMapJS          template.JS
        }

        var viewTemplate = template.Must(template.New("view").Parse(viewHTML))
        """
    ),
    encoding="utf-8",
    newline="\n",
)
print(f"wrote {out_html.name} ({len(html)} chars) and {out_go.name}")
