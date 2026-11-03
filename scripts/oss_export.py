#!/usr/bin/env python3
"""Copy a public OSS source tree. Never modifies the private working tree.

Invoked by scripts/export-oss-source.sh. Subcommands:

    export <root> <dest>     copy, schema swap, doc scrub, SDK detach, Apache LICENSE
    leak-check <root> <dest> fail if Professional source remains in dest

scripts/oss-apache-2.0.txt is the unmodified Apache License 2.0 text. It is
copied to <dest>/LICENSE only. It does not license the private repository.
"""

from __future__ import annotations

import fnmatch
import re
import subprocess
import sys
from pathlib import Path


SENTINELS = (
    "go-report-server/internal/mcp",
    "go-report-server/internal/server/mcp.go",
    "go-report-server/internal/server/mcp_test.go",
    "go-report-server/internal/server/routes_professional.go",
    "go-report-server/cmd/jrs-server/wire_professional.go",
    "go-report-server/internal/modules/professional.go",
    "documentation/docs/administration/mcp-server.md",
    "stand-alone-tomcat-10/map-src/main/java/com/acugis/map/ArcGISAuth.java",
    "stand-alone-tomcat-10/map-src/main/java/com/acugis/map/ArcGISMapRenderer.java",
    "stand-alone-tomcat-10/map-src/main/java/com/acugis/map/QGISAuth.java",
    "stand-alone-tomcat-10/map-src/main/java/com/acugis/map/QGISFilter.java",
    "stand-alone-tomcat-10/map-src/main/java/com/acugis/map/QGISMapRenderer.java",
)

# Professional objects. jasper_arcgis is not in this list: it is the Open
# Source ArcGIS Feature Server catalog (schema.oss.sql, wire_oss.go, and the
# OSS package verification). Professional ArcGIS map/report code is excluded
# by path (Java ArcGISAuth / ArcGISMapRenderer, server/arcgis.go).
SCHEMA_MARKERS = (
    "jasper_qgis",
    "auth_oidc_settings",
    "auth_saml_settings",
    "dataset_intelligence",
    "di_report_dataset",
    "governance_metadata",
    "audit_export_destination",
)
OSS_SCHEMA_REQUIRED = "jasper_arcgis"
OSS_RELEASE = "1.0.21"

# Professional packages may keep only their Open Source stubs.
PROFESSIONAL_PACKAGE_DIRS = {
    "go-report-server/internal/mcp": "none",
    "go-report-server/internal/qgis": "stub",
    "go-report-server/internal/explorer": "stub",
    "go-report-server/internal/governance": "stub",
    "go-report-server/internal/oidcauth": "stub",
    "go-report-server/internal/samlauth": "stub",
    "go-report-server/internal/auditexport": "stub",
    "go-report-server/internal/usage": "stub",
    "go-report-server/internal/extid": "stub",
    "go-report-server/internal/datasetintel": "oss-named",
}

BUILD_CONSTRAINT = re.compile(r"^\s*//\s*(?:go:build|\+build)\b(.*)$")


def load_rules(manifest: Path) -> list[str]:
    rules: list[str] = []
    for raw in manifest.read_text(encoding="utf-8").splitlines():
        line = raw.split("#", 1)[0].strip().replace("\\", "/")
        if line:
            rules.append(line)
    return rules


def is_generated_artifact(rel: str) -> bool:
    """Build and package outputs are not public source.

    Tracked copies (sdk/javascript/node_modules, the committed Professional
    WAR) and files created later in a tree (npm ci, Maven target/, .deb,
    release tarballs) all match. The manifest lists the known paths; this
    shape check still rejects them if a manifest line is missing.
    """
    rel = rel.replace("\\", "/").strip("/")
    if not rel or rel.startswith("../"):
        return False
    parts = [part for part in rel.split("/") if part]
    if any(part in {"node_modules", "target", "_war_peek"} for part in parts):
        return True
    name = parts[-1]
    if name.endswith(".deb") or name.endswith(".deb.sha256"):
        return True
    if name.endswith(".war") or name.endswith(".war.bak"):
        return True
    if name.endswith(".class"):
        return True
    if name in {"catalina.out", "demo.duckdb"} or name.endswith(".duckdb"):
        return True
    if name.endswith(".jasper"):
        return True
    if name.endswith(".log"):
        return True
    if name in {".war-tmp.zip", "_peek.zip"} or (
        name.endswith(".zip") and ("war" in name.lower() or name.startswith("_"))
    ):
        return True
    if rel.startswith("schedules/logs/") or rel.startswith("schedules/output/"):
        return True
    if rel.startswith("schedules/") and name.endswith("_env.sh"):
        return True
    if name.endswith(".pem") or name.endswith(".key"):
        return True
    if parts[0] == "dist" and (
        name.endswith(".tar.gz") or name.endswith(".tar.gz.sha256")
    ):
        return True
    if name in {"ojdbc11.jar", "db2jcc4.jar"}:
        return True
    return False


def excluded(rel: str, rules: list[str]) -> bool:
    rel = rel.replace("\\", "/")
    if is_generated_artifact(rel):
        return True
    for rule in rules:
        if rule.endswith("/"):
            if rel == rule[:-1] or rel.startswith(rule):
                return True
        elif any(ch in rule for ch in "*?["):
            if fnmatch.fnmatch(rel, rule):
                return True
        elif rel == rule:
            return True
    return False


def git_files(root: Path) -> list[str]:
    out = subprocess.check_output(
        ["git", "-C", str(root), "ls-files", "-z"],
    )
    return [p.decode("utf-8") for p in out.split(b"\0") if p]


def export_files(root: Path) -> list[str]:
    """Tracked files, plus Community map sources present in the worktree.

    git ls-files omits map-src until it is committed. The exporter still has
    to ship those sources, and the exclusion manifest still drops the
    Professional QGIS and ArcGIS renderers.
    """
    seen: set[str] = set()
    found: list[str] = []
    for rel in git_files(root):
        rel = rel.replace("\\", "/")
        if rel not in seen:
            seen.add(rel)
            found.append(rel)
    base = root / "stand-alone-tomcat-10" / "map-src"
    if base.is_dir():
        for path in base.rglob("*"):
            if not path.is_file():
                continue
            rel = path.relative_to(root).as_posix()
            if rel not in seen:
                seen.add(rel)
                found.append(rel)
    # Release Compose files may be uncommitted while the image is built.
    # git ls-files omits them until they are committed.
    for extra in ("docker/compose.build.yml", "docker/jdbc-drivers/.gitkeep"):
        path = root / extra
        if path.is_file() and extra not in seen:
            seen.add(extra)
            found.append(extra)
    return found


def has_not_oss_constraint(text: str) -> bool:
    """True when a Go build constraint before package selects !oss.

    A comment that merely mentions the constraint, such as the note in
    modules.go, is not a constraint line and does not match.
    """
    for line in text.lstrip("\ufeff").splitlines():
        stripped = line.strip()
        if stripped.startswith("package "):
            break
        match = BUILD_CONSTRAINT.match(stripped)
        if match and re.search(r"(^|[^A-Za-z0-9_])!oss([^A-Za-z0-9_]|$)", match.group(1)):
            return True
    return False


def go_build_oss_files(root: Path) -> list[str]:
    """Source files tagged !oss. These must all be named in the manifest."""
    found: list[str] = []
    for path in root.rglob("*.go"):
        if any(part in {".git", "node_modules", "dist"} for part in path.parts):
            continue
        try:
            text = path.read_text(encoding="utf-8")
        except OSError:
            continue
        if has_not_oss_constraint(text):
            found.append(path.relative_to(root).as_posix())
    return found


def oss_stub_name(name: str) -> bool:
    return (
        name == "stub_oss.go"
        or name.endswith("_oss.go")
        or name.endswith("_oss_test.go")
        or name in {"build_oss.go", "wire_oss.go"}
    )


def copy_tree(root: Path, dest: Path, rules: list[str]) -> int:
    if dest.exists():
        raise SystemExit(f"output already exists: {dest}")
    dest.mkdir(parents=True)
    copied = 0
    for rel in export_files(root):
        rel_posix = rel.replace("\\", "/")
        if excluded(rel_posix, rules):
            continue
        src = root / rel
        if not src.is_file():
            continue
        target = dest / rel
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(src.read_bytes())
        copied += 1
    return copied


def apply_schema(dest: Path) -> None:
    oss = dest / "database" / "schema.oss.sql"
    public = dest / "database" / "schema.sql"
    if not oss.is_file():
        raise SystemExit("export is missing database/schema.oss.sql")
    if public.exists():
        raise SystemExit("professional database/schema.sql was copied into the export")
    public.write_bytes(oss.read_bytes())


def git_source_revision(root: Path) -> str:
    """Same revision a git checkout build would record. Never invented."""
    try:
        sha = subprocess.check_output(
            ["git", "-C", str(root), "rev-parse", "HEAD"],
            text=True,
        ).strip()
        status = subprocess.check_output(
            ["git", "-C", str(root), "status", "--porcelain"],
            text=True,
        )
    except (subprocess.CalledProcessError, FileNotFoundError) as exc:
        raise SystemExit(f"cannot determine source revision from {root}: {exc}") from exc
    if not re.fullmatch(r"[0-9a-fA-F]{40}", sha):
        raise SystemExit(f"git HEAD is not a full commit SHA: {sha}")
    if status.strip():
        sha = f"{sha}-dirty"
    return sha


def record_source_revision(root: Path, dest: Path) -> None:
    revision = git_source_revision(root)
    (dest / "SOURCE_REVISION").write_bytes((revision + "\n").encode("ascii"))
    print(f"recorded source revision {revision}")


def replace_once(path: Path, old: str, new: str) -> None:
    text = path.read_text(encoding="utf-8")
    if old not in text:
        raise SystemExit(f"doc scrub missed expected text in {path}")
    path.write_text(text.replace(old, new, 1), encoding="utf-8")


def scrub_docs(dest: Path) -> None:
    """Remove Professional MCP and excluded-feature navigation from docs that stay."""
    docs = dest / "documentation" / "docs"
    mkdocs = dest / "documentation" / "mkdocs.yml"
    text = mkdocs.read_text(encoding="utf-8")
    drop_lines = {
        "      - MCP Server: administration/mcp-server.md\n",
        "      - OpenID Connect: administration/oidc.md\n",
        "      - SAML: administration/saml.md\n",
        "      - Audit Export: administration/audit-export.md\n",
        "      - Usage Analytics: administration/usage-analytics.md\n",
        "      - Map Reports: user-guide/map-reports.md\n",
        "          - Explorer: developer/rest-api/explorer.md\n",
        "          - Explorer: developer/cli/explorer.md\n",
        "          - Map Reports: developer/cli/map-reports.md\n",
        "          - Dashboards & Views: developer/embedding/dashboards-views.md\n",
        "          - Map Reports: developer/embedding/map-reports.md\n",
    }
    lines = text.splitlines(keepends=True)
    out: list[str] = []
    i = 0
    while i < len(lines):
        if lines[i] == "      - Data Explorer:\n":
            i += 1
            while i < len(lines) and lines[i].startswith("          - "):
                i += 1
            continue
        if lines[i] in drop_lines:
            i += 1
            continue
        out.append(lines[i])
        i += 1
    rewritten = "".join(out)
    leftover = (
        "mcp-server.md",
        "user-guide/explorer/",
        "administration/oidc.md",
        "administration/saml.md",
        "administration/audit-export.md",
        "administration/usage-analytics.md",
        "map-reports.md",
    )
    if rewritten == text or any(token in rewritten for token in leftover):
        raise SystemExit("mkdocs.yml scrub did not remove Professional navigation")
    mkdocs.write_text(rewritten, encoding="utf-8")

    replace_once(
        docs / "administration" / "api-tokens.md",
        "- Never put long-lived tokens in query strings or frontend bundles\n"
        "- MCP clients use this same token. See [MCP Server](mcp-server.md)\n",
        "- Never put long-lived tokens in query strings or frontend bundles\n",
    )
    replace_once(
        docs / "administration" / "api-tokens.md",
        "## Related\n\n- [MCP Server](mcp-server.md)\n- [Authentication](authentication.md)\n",
        "## Related\n\n- [Authentication](authentication.md)\n",
    )
    replace_once(
        docs / "administration" / "auditing.md",
        "| `analysis` | Data Explorer query, export, save view/dashboard; MCP `mcp_list_datasets`, `mcp_describe_dataset`, `mcp_query_dataset` |\n",
        "| `analysis` | Data Explorer query, export, save view/dashboard |\n",
    )
    replace_once(
        docs / "administration" / "auditing.md",
        "MCP tool calls are recorded in `analysis` when auditing is enabled. A query\n"
        "event stores the dataset id, report key, returned row count, and whether the\n"
        "result was truncated. It does not store parameter values or result rows. See\n"
        "[MCP Server](mcp-server.md).\n\n",
        "",
    )
    replace_once(
        docs / "administration" / "auditing.md",
        "## Related\n\n- [MCP Server](mcp-server.md)\n- [Logging](logging.md)\n",
        "## Related\n\n- [Logging](logging.md)\n",
    )
    replace_once(
        docs / "administration" / "configuration.md",
        "| `JRS_AUDIT_LOG_SQL` | Allow raw SQL in audit details | `false` |\n"
        "| `JRS_MCP_ENABLED` | Professional MCP endpoint (`/api/v1/mcp`). See [MCP Server](mcp-server.md) | unset / false |\n"
        "| `JRS_MCP_ALLOWED_HOSTS` | Public hostnames MCP accepts on the loopback listener, comma-separated | empty |\n",
        "| `JRS_AUDIT_LOG_SQL` | Allow raw SQL in audit details | `false` |\n",
    )
    replace_once(
        docs / "administration" / "configuration.md",
        "- [Designer Web](designer-web.md)\n- [MCP Server](mcp-server.md)\n- [Auditing](auditing.md)\n",
        "- [Designer Web](designer-web.md)\n- [Auditing](auditing.md)\n",
    )
    replace_once(
        docs / "administration" / "permissions.md",
        "- Revoke tokens when staff leave or automation is retired\n"
        "- MCP uses the same report view permission. See [MCP Server](mcp-server.md)\n",
        "- Revoke tokens when staff leave or automation is retired\n",
    )
    replace_once(
        docs / "administration" / "permissions.md",
        "## Related\n\n- [MCP Server](mcp-server.md)\n- [Users](users.md)\n",
        "## Related\n\n- [Users](users.md)\n",
    )
    replace_once(
        docs / "administration" / "reverse-proxy.md",
        " License activation compares the browser `Origin` to that public host. "
        "MCP on the loopback listener accepts that public hostname when it is listed in `JRS_MCP_ALLOWED_HOSTS`.",
        " License activation compares the browser `Origin` to that public host.",
    )
    replace_once(
        docs / "administration" / "reverse-proxy.md",
        "## MCP\n\n"
        "`/api/v1/mcp` uses the same authentication, edition, and license checks as the rest of Report Server. "
        "With the default loopback listen address, a loopback `Host` (`localhost`, `127.0.0.1`, `::1`) is accepted, "
        "and so is each public hostname in `JRS_MCP_ALLOWED_HOSTS` (comma-separated). Set that to the hostname Apache preserves. "
        "Client setup, tools, and troubleshooting are in [MCP Server](mcp-server.md).\n\n"
        "```bash\n"
        "JRS_MCP_ENABLED=true\n"
        "JRS_MCP_ALLOWED_HOSTS=reports.example.com\n"
        "```\n\n"
        "Comparison ignores case and the default port (`:443` for https, `:80` for http). "
        "Any other hostname on a loopback connection receives `403` with `Forbidden: invalid Host header`.\n\n",
        "",
    )
    replace_once(
        docs / "administration" / "reverse-proxy.md",
        "## Related\n\n- [MCP Server](mcp-server.md)\n- [Deployment](deployment.md)\n",
        "## Related\n\n- [Deployment](deployment.md)\n",
    )
    replace_once(
        docs / "administration" / "security.md",
        "- [API Tokens](api-tokens.md)\n- [MCP Server](mcp-server.md)\n- [Permissions](permissions.md)\n",
        "- [API Tokens](api-tokens.md)\n- [Permissions](permissions.md)\n",
    )
    replace_once(
        docs / "reference" / "configuration.md",
        "| Embedding | [Embedding security](../developer/embedding/security.md) |\n"
        "| MCP | [MCP Server](../administration/mcp-server.md) |\n"
        "| SMTP and scheduler | Environment variable reference |\n",
        "| Embedding | [Embedding security](../developer/embedding/security.md) |\n"
        "| SMTP and scheduler | Environment variable reference |\n",
    )
    replace_once(
        docs / "reference" / "environment-variables.md",
        "## MCP\n\n"
        "Read by `jrs-server`. The route exists only in the Professional binary and requires Dataset Intelligence, which a license must enable. See [MCP Server](../administration/mcp-server.md).\n\n"
        "| Variable | Purpose | Default |\n"
        "|----------|---------|---------|\n"
        "| `JRS_MCP_ENABLED` | Enable `/api/v1/mcp` | off unless `1`/`true`/`yes`/`on` |\n"
        "| `JRS_MCP_ALLOWED_HOSTS` | Extra public hostnames accepted when the connection arrives on loopback | empty (loopback `Host` values only) |\n\n",
        "",
    )
    replace_once(
        docs / "reference" / "compatibility-matrix.md",
        "| Dashboards, Map Reports, Dataset Intelligence, MCP | No |\n",
        "| Dashboards, Map Reports, Dataset Intelligence | No |\n",
    )
    replace_once(
        docs / "reference" / "compatibility-matrix.md",
        "A valid license on the Professional package entitles the compiled modules the allow-list denies. That includes Map Reports, dashboards, Dataset Intelligence, MCP (still off until `JRS_MCP_ENABLED` is set), audit administration, usage analytics, governance, audit export, OpenID Connect, SAML, the ArcGIS Designer provider and its connection page, and the Designer providers GeoServer and QGIS.\n\n"
        "Data Explorer and embedded QGIS are already allowed before that license.\n\n"
        "MCP uses Dataset Intelligence and the signed-in user's report view permission. See [MCP Server](../administration/mcp-server.md).\n",
        "A valid license on the Professional package entitles the compiled modules the allow-list denies. That includes Map Reports, dashboards, Dataset Intelligence, audit administration, usage analytics, governance, audit export, OpenID Connect, SAML, the ArcGIS Designer provider and its connection page, and the Designer providers GeoServer and QGIS.\n\n"
        "Data Explorer and embedded QGIS are already allowed before that license.\n",
    )
    replace_once(
        docs / "getting-started" / "authentication.md",
        "- [CLI authentication](../developer/cli/authentication.md)\n"
        "- [MCP Server](../administration/mcp-server.md)\n"
        "- [Security administration](../administration/security.md)\n",
        "- [CLI authentication](../developer/cli/authentication.md)\n"
        "- [Security administration](../administration/security.md)\n",
    )
    replace_once(
        docs / "getting-started" / "concepts.md",
        "## API Token\n\n"
        "A personal access token with prefix `jrs_` used as `Authorization: Bearer …` for CLI, SDKs, CI, "
        "and the [MCP Server](../administration/mcp-server.md). Managed under Profile → API Tokens or "
        "`/api/v1/auth/tokens`. There is no separate MCP token scope.\n\n"
        "## Dataset Intelligence\n\n"
        "The Professional catalog of report datasets: parameters, fields, business descriptions, governance, "
        "stored profiles, and lineage. It requires a valid license. The [MCP Server](../administration/mcp-server.md) uses this catalog so "
        "an AI client can ask questions about datasets the user is allowed to view.\n\n"
        "## MCP Server\n\n"
        "A Professional endpoint at `/api/v1/mcp`, available only on a licensed Professional package. Compatible AI clients discover and run existing report main datasets. "
        "The client authenticates with a Personal Access Token and does not receive database credentials or the ability "
        "to submit SQL. Disabled until an administrator sets `JRS_MCP_ENABLED`. "
        "See [MCP Server](../administration/mcp-server.md).\n\n",
        "## API Token\n\n"
        "A personal access token with prefix `jrs_` used as `Authorization: Bearer …` for CLI, SDKs, and CI. "
        "Managed under Profile → API Tokens or `/api/v1/auth/tokens`.\n\n"
        "## Dataset Intelligence\n\n"
        "The Professional catalog of report datasets: parameters, fields, business descriptions, governance, "
        "stored profiles, and lineage. It requires a valid license. That implementation is not in this Open Source source tree.\n\n",
    )
    replace_once(
        docs / "developer" / "authentication.md",
        "- [Administration — API Tokens](../administration/api-tokens.md)\n"
        "- [MCP Server](../administration/mcp-server.md)\n"
        "- [Security](../administration/security.md)\n",
        "- [Administration — API Tokens](../administration/api-tokens.md)\n"
        "- [Security](../administration/security.md)\n",
    )
    replace_once(
        docs / "user-guide" / "index.md",
        "Use this section when you want to understand **what Report Server does** — reports, folders, parameters, "
        "Data Explorer, maps, and permissions — without starting from REST endpoints or SDK samples.\n",
        "Use this section when you want to understand **what Report Server does** — reports, folders, parameters, "
        "maps, and permissions — without starting from REST endpoints or SDK samples.\n",
    )
    replace_once(
        docs / "user-guide" / "index.md",
        "- Report authors and publishers\n- Analysts using Data Explorer\n- Users who run reports and manage catalog content\n",
        "- Report authors and publishers\n- Users who run reports and manage catalog content\n",
    )
    replace_once(
        docs / "user-guide" / "index.md",
        "| [Folders](folders.md) | Organizing catalog content |\n"
        "| [Data Explorer](explorer/overview.md) | Interactive analysis over tables and report queries |\n"
        "| [MCP Server](../administration/mcp-server.md) | Ask an AI client about datasets you are already allowed to view (Professional) |\n"
        "| [Report Designer](designer/overview.md) | Authoring JRXML, Embedded Maps, and Map Reports |\n",
        "| [Folders](folders.md) | Organizing catalog content |\n"
        "| [Report Designer](designer/overview.md) | Authoring JRXML, Embedded Maps, and Map Reports |\n",
    )
    replace_once(
        docs / "user-guide" / "index.md",
        "### Asking questions with an AI client\n\n"
        "Professional installations can let an MCP client ask questions about datasets you are already allowed to view. "
        "You do not supply SQL or database credentials. See [MCP Server](../administration/mcp-server.md).\n\n"
        "### Analysis\n\n"
        "- [Data Explorer overview](explorer/overview.md)\n"
        "- [Queries](explorer/queries.md)\n"
        "- [Statistics](explorer/statistics.md)\n"
        "- [Spatial analysis](explorer/spatial-analysis.md)\n"
        "- [Saved views](explorer/saved-views.md)\n"
        "- [Dashboards](explorer/dashboards.md)\n\n",
        "",
    )
    replace_once(
        docs / "user-guide" / "permissions.md",
        "- Revoke tokens when staff leave or automation is retired\n"
        "- An MCP client uses the same view permission as opening the parent report. "
        "See [MCP Server](../administration/mcp-server.md)\n",
        "- Revoke tokens when staff leave or automation is retired\n",
    )
    replace_once(
        docs / "user-guide" / "permissions.md",
        "- [Permissions (administration)](../administration/permissions.md)\n"
        "- [MCP Server](../administration/mcp-server.md)\n",
        "- [Permissions (administration)](../administration/permissions.md)\n",
    )
    replace_once(
        dest / "go-report-server" / "docs" / "configuration.md",
        "| `JRS_AUDIT_LOG_SQL` / `AUDIT_LOG_SQL` | Allow raw SQL in audit details | `false` |\n"
        "| `JRS_MCP_ENABLED` | Professional MCP endpoint (`/api/v1/mcp`). Ignored by OSS builds | `false` |\n"
        "| `JRS_MCP_ALLOWED_HOSTS` | Comma-separated public hostnames accepted by MCP in addition to loopback, when the connection arrives on a loopback address | empty |\n",
        "| `JRS_AUDIT_LOG_SQL` / `AUDIT_LOG_SQL` | Allow raw SQL in audit details | `false` |\n",
    )


def detach_sdk(dest: Path) -> None:
    """Drop compile references to excluded Professional SDK clients. Export only."""
    go_client = dest / "sdk" / "go" / "client.go"
    replace_once(go_client, "\tExplorer     *ExplorerService\n", "")
    replace_once(go_client, "\tc.Explorer = &ExplorerService{c: c}\n", "")

    java = dest / "sdk" / "java" / "src" / "main" / "java" / "com" / "acugis" / "report" / "ReportServerClient.java"
    replace_once(java, "import com.acugis.report.services.ExplorerService;\n", "")
    replace_once(java, "    private final ExplorerService explorer;\n", "")
    replace_once(java, "        this.explorer = new ExplorerService(transport);\n", "")
    replace_once(
        java,
        "    public ExplorerService explorer() {\n        return explorer;\n    }\n\n",
        "",
    )

    cs = dest / "sdk" / "dotnet" / "src" / "AcuGIS.Report" / "ReportServerClient.cs"
    replace_once(cs, "        Explorer = new ExplorerService(_transport);\n", "")
    replace_once(cs, "    public ExplorerService Explorer { get; }\n", "")

    app = dest / "sdk" / "python" / "acugis_report" / "cli" / "app.py"
    replace_once(app, "    dashboards,\n", "")
    replace_once(app, "    explorer,\n", "")
    replace_once(app, "    views,\n", "")
    replace_once(app, '    help="AcuGIS Report Server CLI — automate reports, folders, resources, and explorer.",\n',
                 '    help="AcuGIS Report Server CLI — automate reports, folders, and resources.",\n')
    replace_once(app, 'app.add_typer(views.app, name="views")\n', "")
    replace_once(app, 'app.add_typer(dashboards.app, name="dashboards")\n', "")
    replace_once(app, 'app.add_typer(explorer.app, name="explorer")\n', "")

    py_client = dest / "sdk" / "python" / "acugis_report" / "client.py"
    replace_once(py_client, "    ExplorerService,\n", "")
    replace_once(py_client, "        self.explorer = ExplorerService(self)\n", "")
    text = py_client.read_text(encoding="utf-8")
    start = text.find("    # ------------------------------------------------------------ explorer\n")
    end = text.find("    # ------------------------------------------------- future / reserved\n")
    if start < 0 or end < 0 or end <= start:
        raise SystemExit("python client explorer block was not found")
    py_client.write_text(text[:start] + text[end:], encoding="utf-8")

    services = dest / "sdk" / "python" / "acugis_report" / "services.py"
    text = services.read_text(encoding="utf-8")
    start = text.find("class ExplorerService:\n")
    end = text.find("class ReservedService:\n")
    if start < 0 or end < 0 or end <= start:
        raise SystemExit("python ExplorerService class was not found")
    services.write_text(text[:start] + text[end:], encoding="utf-8")


def leak_check(root: Path, dest: Path, *, require_tidy: bool) -> None:
    rules = load_rules(root / "scripts" / "oss-export-exclude.txt")
    errors: list[str] = []

    missing_from_manifest = [
        rel for rel in go_build_oss_files(root) if not excluded(rel, rules)
    ]
    if missing_from_manifest:
        errors.append(
            "private !oss files missing from scripts/oss-export-exclude.txt:\n  "
            + "\n  ".join(missing_from_manifest)
        )

    for rel in go_build_oss_files(dest):
        errors.append(f"exported Go file still has //go:build !oss: {rel}")

    for rel in SENTINELS:
        if (dest / rel).exists():
            errors.append(f"sentinel still present: {rel}")

    for rule in rules:
        if rule.endswith("/"):
            if (dest / rule[:-1]).exists():
                errors.append(f"excluded directory still present: {rule}")
            continue
        if any(ch in rule for ch in "*?["):
            continue
        if rule == "database/schema.sql":
            continue
        if (dest / rule).exists():
            errors.append(f"excluded path still present: {rule}")

    schema = dest / "database" / "schema.sql"
    oss_schema = dest / "database" / "schema.oss.sql"
    if not schema.is_file():
        errors.append("database/schema.sql missing from export")
    if not oss_schema.is_file():
        errors.append("database/schema.oss.sql missing from export")
    if schema.is_file() and oss_schema.is_file():
        if schema.read_bytes() != oss_schema.read_bytes():
            errors.append("database/schema.sql and database/schema.oss.sql differ")
        body = schema.read_text(encoding="utf-8", errors="replace").lower()
        for marker in SCHEMA_MARKERS:
            if marker in body:
                errors.append(f"Community schema still contains {marker}")
        if OSS_SCHEMA_REQUIRED not in body:
            errors.append(
                "Community schema is missing jasper_arcgis "
                "(Open Source ArcGIS Feature Server catalog)"
            )
    revision_file = dest / "SOURCE_REVISION"
    if not revision_file.is_file():
        errors.append("SOURCE_REVISION missing from export")
    else:
        revision = revision_file.read_text(encoding="utf-8").strip()
        if not re.fullmatch(r"[0-9a-fA-F]{40}(-dirty)?", revision):
            errors.append(f"SOURCE_REVISION is not a full git commit SHA: {revision}")
        else:
            try:
                expected = git_source_revision(root)
            except SystemExit as exc:
                errors.append(str(exc))
            else:
                if revision != expected:
                    errors.append(
                        f"SOURCE_REVISION {revision} does not match private repository {expected}"
                    )
    if (dest / "database" / "migrations" / "professional").exists():
        errors.append("database/migrations/professional still present")

    generated: list[str] = []
    seen_generated: set[str] = set()
    if dest.is_dir():
        for path in dest.rglob("*"):
            rel = path.relative_to(dest).as_posix()
            if not is_generated_artifact(rel):
                continue
            parts = rel.split("/")
            label = rel
            for index, part in enumerate(parts):
                if part in {"node_modules", "target"}:
                    label = "/".join(parts[: index + 1]) + "/"
                    break
            if label not in seen_generated:
                seen_generated.add(label)
                generated.append(label)
            if len(generated) >= 30:
                break
    if generated:
        errors.append(
            "generated build artifacts still present:\n  " + "\n  ".join(generated)
        )

    gomod = dest / "go-report-server" / "go.mod"
    gosum = dest / "go-report-server" / "go.sum"
    joined = ""
    if gomod.is_file():
        joined += gomod.read_text(encoding="utf-8")
    if gosum.is_file():
        joined += gosum.read_text(encoding="utf-8")
    if require_tidy and gomod.is_file():
        gomod_text = gomod.read_text(encoding="utf-8")
        if "modelcontextprotocol" in gomod_text:
            errors.append(
                "exported go.mod still requires github.com/modelcontextprotocol/go-sdk"
            )
        if "github.com/crewjam/saml" in gomod_text:
            errors.append("exported go.mod still requires github.com/crewjam/saml")

    doc_hits: list[str] = []
    for doc_root in (dest / "documentation", dest / "go-report-server" / "docs"):
        if not doc_root.is_dir():
            continue
        for path in doc_root.rglob("*"):
            if not path.is_file() or path.suffix not in {".md", ".yml", ".yaml"}:
                continue
            text = path.read_text(encoding="utf-8", errors="replace")
            if "mcp-server.md" in text or "JRS_MCP_" in text or "/api/v1/mcp" in text:
                doc_hits.append(path.relative_to(dest).as_posix())
    if doc_hits:
        errors.append("exported docs still describe MCP:\n  " + "\n  ".join(doc_hits))

    errors.extend(professional_package_errors(dest))
    errors.extend(forbidden_binary_errors(dest))
    errors.extend(secret_errors(dest))
    errors.extend(publication_errors(root, dest))
    errors.extend(php_missing_include_errors(dest))

    if errors:
        raise SystemExit("OSS export leak check failed:\n- " + "\n- ".join(errors))
    print("leak check passed")


def replace_all_required(path: Path, old: str, new: str) -> None:
    text = path.read_text(encoding="utf-8")
    if old not in text:
        raise SystemExit(f"export scrub missed expected text in {path}")
    path.write_text(text.replace(old, new), encoding="utf-8")


def scrub_oss_doc_copyright(dest: Path) -> None:
    """Drop the proprietary footer from the exported documentation only."""
    replace_once(
        dest / "documentation" / "mkdocs.yml",
        "copyright: Copyright &copy; 2025 AcuGIS. All rights reserved.\n",
        "copyright: Copyright &copy; 2026 AcuGIS\n",
    )
    replace_once(
        dest / "documentation" / "overrides" / "partials" / "copyright.html",
        "        © 2025 AcuGIS. All rights reserved.\n",
        "        Copyright © 2026 AcuGIS\n",
    )


def scrub_shared_php(dest: Path) -> None:
    """Drop the legacy ArcGIS connection admin from the export copy of bootstrap.php."""
    path = dest / "lib" / "bootstrap.php"
    text = path.read_text(encoding="utf-8")
    start = text.find("/**\n * Enabled ArcGIS connections")
    end = text.find("/**\n * Open a PHP connection for SQL-backed report parameters.")
    if start < 0 or end < 0 or end <= start:
        raise SystemExit("ArcGIS PHP block was not found in exported bootstrap.php")
    text = text[:start] + text[end:]
    nav = '            <a class="nav-link" href="arcgis_connections.php">ArcGIS</a>\n'
    if nav not in text:
        raise SystemExit("ArcGIS nav link was not found in exported bootstrap.php")
    path.write_text(text.replace(nav, "", 1), encoding="utf-8")
    shell_config = dest / "shared" / "ui" / "shell-config.js"
    replace_once(
        shell_config,
        "    {\n"
        "      label: 'ArcGIS',\n"
        "      icon: 'map',\n"
        "      href: 'reports/arcgis_connections.php',\n"
        "      pathMatch: '/reports/arcgis_connections.php',\n"
        "      description: 'ArcGIS connections for Designer map publish',\n"
        "    },\n",
        "",
    )
    replace_once(
        shell_config,
        "              '/reports/arcgis_connections',\n",
        "",
    )
    replace_once(
        dest / "shared" / "ui" / "shell.js",
        "      '/reports/arcgis_connections',\n",
        "",
    )


def write_apache_license(root: Path, dest: Path) -> None:
    """Write the unmodified Apache License 2.0 text to the export only."""
    src = root / "scripts" / "oss-apache-2.0.txt"
    if not src.is_file():
        raise SystemExit(f"missing Apache license text: {src}")
    text = src.read_bytes()
    if b"Apache License" not in text or b"Version 2.0, January 2004" not in text:
        raise SystemExit("scripts/oss-apache-2.0.txt is not the Apache License 2.0")
    (dest / "LICENSE").write_bytes(text)


def scrub_public_readme(dest: Path) -> None:
    """Adjust the copied private README for the public Open Source tree."""
    readme = dest / "README.md"
    replace_once(
        readme,
        "# AcuGIS Report Server\n",
        f"# AcuGIS Report Server {OSS_RELEASE}\n",
    )
    replace_once(
        readme,
        "This project does not publish a hosted APT repository. Supported installs are a local Debian package, a release tarball, or Docker Compose.",
        "Supported installs are a local Debian package, a release tarball, or Docker Compose.",
    )
    replace_once(
        readme,
        "- The same datasource administration screen on both packages: PostgreSQL, MySQL, Oracle, SQL Server, Snowflake, SQLite, Cassandra, BigQuery, and DuckDB\n"
        "- Embedded maps for PostGIS, MySQL, and GeoServer on both packages; QGIS on the Professional package; ArcGIS when a Professional license is valid\n"
        "- Data Explorer on the Professional package, including before a license is activated\n"
        "- Dashboards, Map Reports, and Dataset Intelligence when a Professional license is valid\n"
        "- Audit recording when auditing is enabled; audit administration, usage analytics, governance, and audit export when a Professional license is valid\n"
        "- OpenID Connect and SAML when a Professional license is valid\n"
        "- MCP Server on a licensed Professional package, after an administrator enables it\n",
        "- The same datasource administration screen: PostgreSQL, MySQL, Oracle, SQL Server, Snowflake, SQLite, Cassandra, BigQuery, and DuckDB\n"
        "- Embedded maps for PostGIS, MySQL, and GeoServer\n"
        "- Audit recording when auditing is enabled\n"
        "- QGIS, ArcGIS, Data Explorer, dashboards, Map Reports, Dataset Intelligence, audit administration, usage analytics, governance, audit export, OpenID Connect, SAML, and MCP are Professional features. Their source is not in this tree.\n",
    )
    replace_once(
        readme,
        "## Editions\n\n"
        "Report Server is published as **Open Source** and **Professional**.\n\n"
        "The **Open Source** package (`acugis-report-server`) includes the catalog, execution, scheduling, datasources, local login, personal access tokens, report embedding, and embedded maps for PostGIS, MySQL, and GeoServer. Professional modules are omitted from that binary. A license file cannot add them.\n\n"
        "The **Professional** package (`acugis-report-server-professional`) contains the additional modules. Three runtime states matter:\n\n"
        "1. **Open Source package.** Professional modules are absent.\n"
        "2. **Professional package without a valid license.** The process starts. Data Explorer and embedded QGIS are available. Map Reports, dashboards, Dataset Intelligence, MCP, audit administration, usage analytics, governance, audit export, OpenID Connect, SAML, and ArcGIS stay off. The UI identifies this running edition as Open Source.\n"
        "3. **Professional package with a valid license.** Those compiled features are entitled until the license expires. MCP stays off until an administrator enables it.\n\n"
        "`ACUGIS_EDITION` does not activate Professional features. Installing the Professional package does not activate a license. Activate under **Administration → License**.\n\n"
        "See [Licensing](documentation/docs/administration/licensing.md) and the [compatibility matrix](documentation/docs/reference/compatibility-matrix.md).\n",
        "## Editions\n\n"
        "This repository is the AcuGIS Report Server Open Source source tree.\n\n"
        "The Open Source server includes the catalog, execution, scheduling, datasources, local login, personal access tokens, report embedding, and embedded maps for PostGIS, MySQL, and GeoServer. Professional modules are not in this source tree. A license file cannot add them.\n\n"
        "Professional is a separate proprietary product. Data Explorer, embedded QGIS, Map Reports, dashboards, Dataset Intelligence, audit administration, usage analytics, governance, audit export, OpenID Connect, SAML, and ArcGIS are not included here.\n\n"
        "`ACUGIS_EDITION` does not activate Professional features.\n\n"
        "See [Licensing](documentation/docs/administration/licensing.md) and the [compatibility matrix](documentation/docs/reference/compatibility-matrix.md).\n",
    )
    replace_once(
        readme,
        "The Professional package file is `acugis-report-server-professional_<version>_amd64.deb`.\n",
        "The Professional package is a separate proprietary product and is not built from this source tree.\n",
    )
    replace_once(
        readme,
        "The tarball is a precompiled archive. `<edition>` is `oss` or `professional`.\n",
        "The tarball is a precompiled Open Source archive. `<edition>` is `oss`.\n",
    )
    replace_once(
        readme,
        "The Compose file pulls `acugis/report-server:1.0.19-professional`. Set `REPORT_SERVER_IMAGE=acugis/report-server:1.0.19-oss` for the Open Source image. A local source build is `docker compose -f compose.yml -f compose.build.yml up -d --build`. Optional services, including the QGIS profile, are documented in [Docker](documentation/docs/administration/docker.md).\n",
        f"The Compose file pulls `acugis/report-server:{OSS_RELEASE}-oss`. Professional modules are not in this tree. A local source build is `docker compose -f compose.yml -f compose.build.yml up -d --build`. Optional services are documented in [Docker](documentation/docs/administration/docker.md).\n",
    )
    replace_once(
        readme,
        "| Licensing | [Licensing](documentation/docs/administration/licensing.md) |\n"
        "| MCP Server | [MCP Server](documentation/docs/administration/mcp-server.md) |\n"
        "| Datasources | [Supported datasources](documentation/docs/reference/supported-datasources.md) |\n",
        "| Licensing | [Licensing](documentation/docs/administration/licensing.md) |\n"
        "| Datasources | [Supported datasources](documentation/docs/reference/supported-datasources.md) |\n",
    )
    replace_once(
        readme,
        "## MCP Server\n\n"
        "AcuGIS Report Server Professional can expose a Model Context Protocol (MCP) endpoint so compatible AI clients can work with datasets the signed-in user is already allowed to view.\n\n"
        "MCP is a governed interface to existing Report Server capabilities. Report Server does not embed a language model, and it does not need one to run. Clients authenticate with a Report Server personal access token. They do not receive database credentials, and they do not submit SQL. The tools are `list_datasets`, `describe_dataset`, and `query_dataset`.\n\n"
        "The route is compiled only into the Professional package. It answers only when a license is valid and an administrator sets `JRS_MCP_ENABLED`. The Open Source package does not register it.\n\n"
        "See [MCP Server](documentation/docs/administration/mcp-server.md).\n\n",
        "",
    )
    replace_once(
        readme,
        "`make run` in `go-report-server` starts the Go process from a source checkout. That is a developer run, not a product install. See [Developer checkout](documentation/docs/administration/installation.md#developer-checkout).\n\n"
        "`install.sh` belongs to the release tarball. It is not a source-build installer.\n\n"
        "The root `Makefile` builds release artifacts with `package-oss`, `package-professional`, `deb-oss`, and `deb-professional`.\n",
        "`make run` in `go-report-server` starts the Open Source Go process (`-tags oss`). That is a developer run, not a product install. See [Developer checkout](documentation/docs/administration/installation.md#developer-checkout).\n\n"
        "`install.sh` belongs to the release tarball. It is not a source-build installer.\n\n"
        "The root `Makefile` builds the Open Source release artifacts. `package` and `deb` are the Open Source package targets.\n",
    )
    replace_once(
        readme,
        "## License\n\n"
        "This private development repository has no root `LICENSE` file. It contains both Open Source and Professional source. Professional source remains proprietary and is not Apache-licensed.\n\n"
        "Debian packages from [`scripts/package-deb.sh`](scripts/package-deb.sh) are edition-aware. The Open Source package declares Apache License 2.0 and copyright 2026 AcuGIS. The Professional package declares `License: Proprietary`, copyright AcuGIS, and all rights reserved.\n\n"
        "[`scripts/export-oss-source.sh`](scripts/export-oss-source.sh) writes the public Open Source tree. That tree, not this repository, includes the Apache License 2.0 `LICENSE` file.\n\n"
        "Professional feature activation is a separate product license, stored as `license.lic` after activation. See [Licensing](documentation/docs/administration/licensing.md).\n\n"
        "The client SDKs in `sdk/python`, `sdk/go`, `sdk/java`, `sdk/dotnet`, and `sdk/javascript` remain under the MIT License. Copyright AcuGIS. Those MIT licenses do not license the server.\n",
        "## License\n\n"
        "AcuGIS Report Server Open Source is licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE). Copyright 2026 AcuGIS.\n\n"
        "Professional is a separate proprietary product. Its source is not included in this tree.\n\n"
        "Professional feature activation (`license.lic`) applies to that proprietary product. It is not the copyright license for this source tree. See [Licensing](documentation/docs/administration/licensing.md).\n\n"
        "The client SDKs in `sdk/python`, `sdk/go`, `sdk/java`, `sdk/dotnet`, and `sdk/javascript` remain under their own MIT License. Copyright AcuGIS. Those MIT licenses do not license the server.\n",
    )


def scrub_oss_build_defaults(dest: Path) -> None:
    """Make the exported tree build as Open Source. Private defaults stay unchanged."""
    replace_once(
        dest / "Makefile",
        ".PHONY: package package-oss package-professional \\\n"
        "\tverify-package verify-package-oss verify-package-professional \\\n"
        "\tdeb deb-oss deb-professional deb-clean \\\n"
        "\tverify-deb verify-deb-oss verify-deb-professional \\\n"
        "\ttest-deb-install \\\n"
        "\tdocker-build-oss docker-build-professional \\\n"
        "\tdocker-push-oss docker-push-professional \\\n"
        "\tbuild-oss build-professional\n",
        ".PHONY: package package-oss \\\n"
        "\tverify-package verify-package-oss \\\n"
        "\tdeb deb-oss deb-clean \\\n"
        "\tverify-deb verify-deb-oss \\\n"
        "\ttest-deb-install \\\n"
        "\tdocker-build-oss \\\n"
        "\tdocker-push-oss \\\n"
        "\tbuild-oss\n",
    )
    replace_once(
        dest / "Makefile",
        "package-professional:\n"
        "\tbash scripts/package-release.sh professional $(VERSION)\n\n"
        "package: package-oss package-professional\n",
        "package: package-oss\n",
    )
    replace_once(
        dest / "Makefile",
        "verify-package-professional:\n"
        "\tbash scripts/verify-package.sh professional $(VERSION)\n\n"
        "verify-package: verify-package-oss verify-package-professional\n",
        "verify-package: verify-package-oss\n",
    )
    replace_once(
        dest / "Makefile",
        "deb-professional:\n"
        "\tDEB_ARCH=$(DEB_ARCH) DEB_REVISION=$(DEB_REVISION) bash scripts/package-deb.sh professional $(VERSION)\n\n"
        "deb: deb-oss deb-professional\n",
        "deb: deb-oss\n",
    )
    replace_once(
        dest / "Makefile",
        "verify-deb-professional:\n"
        "\tDEB_ARCH=$(DEB_ARCH) DEB_REVISION=$(DEB_REVISION) bash scripts/verify-deb.sh professional $(VERSION)\n\n"
        "verify-deb: verify-deb-oss verify-deb-professional\n",
        "verify-deb: verify-deb-oss\n",
    )
    replace_once(
        dest / "Makefile",
        "build-professional:\n"
        "\t$(MAKE) -C go-report-server build-professional VERSION=$(VERSION)\n\n",
        "",
    )
    replace_once(
        dest / "Makefile",
        "docker-build-professional:\n"
        "\tbash -c 'printf \"%s\\n\" \"$(SOURCE_REVISION)\" | grep -Eq \"^[0-9a-fA-F]{40}(-dirty)?$$\"'\n"
        "\tdocker build -f $(DOCKERFILE) \\\n"
        "\t\t--build-arg BUILD_EDITION=professional \\\n"
        "\t\t--build-arg VERSION=$(VERSION) \\\n"
        "\t\t--build-arg SOURCE_REVISION=$(SOURCE_REVISION) \\\n"
        "\t\t-t acugis/report-server:$(VERSION)-professional \\\n"
        "\t\t-t acugis/report-server:professional \\\n"
        "\t\t$(DOCKER_CONTEXT)\n\n",
        "",
    )
    replace_once(
        dest / "Makefile",
        "docker-push-professional:\n"
        "\tdocker push acugis/report-server:$(VERSION)-professional\n"
        "\tdocker push acugis/report-server:professional\n",
        "",
    )

    gmake = dest / "go-report-server" / "Makefile"
    replace_once(
        gmake,
        ".PHONY: build build-oss build-professional build-cli build-cli-oss run test test-oss test-professional vet fmt tidy openapi-check openapi-export openapi-sync dist-oss dist-professional package package-oss package-professional deb deb-oss deb-professional deb-clean verify-deb-oss verify-deb-professional docker-build-oss docker-build-professional docker-push-oss docker-push-professional\n",
        ".PHONY: build build-oss build-cli run test test-oss vet fmt tidy openapi-check openapi-export openapi-sync dist-oss package package-oss deb deb-oss deb-clean verify-deb-oss docker-build-oss docker-push-oss\n",
    )
    replace_once(
        gmake,
        "# Untagged build remains the full Professional product.\n"
        "build: build-professional\n\n"
        "# saml-nofuzz-modfile replaces crewjam/saml with a copy whose xmlenc/fuzz.go\n"
        "# harness is removed. That harness embeds a test RSA private key and has no\n"
        "# build tag, so it is otherwise linked into every Professional binary that\n"
        "# imports SAML.\n"
        "build-professional: openapi-sync\n"
        "\tmodfile=\"$$($(GO) run ./scripts/saml-nofuzz-modfile.go)\"; \\\n"
        "\tmodargs=\"\"; \\\n"
        "\tif [ -n \"$$modfile\" ]; then modargs=\"-modfile $$modfile -mod=readonly\"; fi; \\\n"
        "\t$(GO) build $$modargs -ldflags \"$(LDFLAGS)\" -o $(BIN) ./cmd/jrs-server && \\\n"
        "\t$(GO) build $$modargs -ldflags \"$(LDFLAGS)\" -o $(CLI_BIN) ./cmd/jrs-run-schedule && \\\n"
        "\t$(GO) build $$modargs -ldflags \"$(LDFLAGS)\" -o $(DISPATCHER_BIN) ./cmd/jrs-schedule-dispatcher\n\n"
        "build-oss: openapi-sync\n",
        "# This tree is Open Source. Default builds use -tags oss.\n"
        "build: build-oss\n\n"
        "build-oss: openapi-sync\n",
    )
    replace_once(
        gmake,
        "build-cli:\n"
        "\t$(GO) build -ldflags \"$(LDFLAGS)\" -o $(CLI_BIN) ./cmd/jrs-run-schedule\n\n"
        "run: openapi-sync\n"
        "\t$(GO) run ./cmd/jrs-server\n\n"
        "test: test-professional\n\n"
        "test-professional: openapi-sync\n"
        "\t$(GO) test ./cmd/... ./internal/...\n\n"
        "test-oss: openapi-sync\n",
        "build-cli:\n"
        "\t$(GO) build -tags $(OSS_TAGS) -ldflags \"$(LDFLAGS)\" -o $(CLI_BIN) ./cmd/jrs-run-schedule\n\n"
        "run: openapi-sync\n"
        "\t$(GO) run -tags $(OSS_TAGS) ./cmd/jrs-server\n\n"
        "test: test-oss\n\n"
        "test-oss: openapi-sync\n",
    )
    replace_once(
        gmake,
        "\t$(GO) test ./internal/openapi/ ./internal/server/ -run 'OpenAPI|Bundle|Marshal|Deprecated' -count=1\n",
        "\t$(GO) test -tags $(OSS_TAGS) ./internal/openapi/ ./internal/server/ -run 'OpenAPI|Bundle|Marshal|Deprecated' -count=1\n",
    )
    replace_once(
        gmake,
        "vet:\n\t$(GO) vet ./...\n",
        "vet:\n\t$(GO) vet -tags $(OSS_TAGS) ./...\n",
    )
    replace_once(
        gmake,
        "package-professional:\n"
        "\t$(MAKE) -C .. package-professional VERSION=$(VERSION)\n\n",
        "",
    )
    replace_once(
        gmake,
        "deb-professional:\n"
        "\t$(MAKE) -C .. deb-professional VERSION=$(VERSION)\n\n",
        "",
    )
    replace_once(
        gmake,
        "verify-deb-professional:\n"
        "\t$(MAKE) -C .. verify-deb-professional VERSION=$(VERSION)\n\n",
        "",
    )
    replace_once(
        gmake,
        "dist-professional: package-professional\n\n",
        "",
    )
    replace_once(
        gmake,
        "docker-build-professional:\n"
        "\tbash -c 'printf \"%s\\n\" \"$(SOURCE_REVISION)\" | grep -Eq \"^[0-9a-fA-F]{40}(-dirty)?$$\"'\n"
        "\tdocker build -f $(DOCKERFILE) \\\n"
        "\t\t--build-arg BUILD_EDITION=professional \\\n"
        "\t\t--build-arg VERSION=$(VERSION) \\\n"
        "\t\t--build-arg SOURCE_REVISION=$(SOURCE_REVISION) \\\n"
        "\t\t-t acugis/report-server:$(VERSION)-professional \\\n"
        "\t\t-t acugis/report-server:professional \\\n"
        "\t\t$(DOCKER_CONTEXT)\n\n",
        "",
    )
    replace_once(
        gmake,
        "docker-push-professional:\n"
        "\tdocker push acugis/report-server:$(VERSION)-professional\n"
        "\tdocker push acugis/report-server:professional\n",
        "",
    )

    dockerfile = dest / "docker" / "report-server" / "Dockerfile"
    replace_once(
        dockerfile,
        "# professional (default, untagged Go build) or oss (-tags oss).\n"
        "ARG BUILD_EDITION=professional\n",
        "# Open Source build (-tags oss).\n"
        "ARG BUILD_EDITION=oss\n",
    )
    replace_all_required(dockerfile, "ARG BUILD_EDITION=professional\n", "ARG BUILD_EDITION=oss\n")
    replace_once(
        dockerfile,
        "    tags=\"\"; \\\n"
        "    if [ \"$BUILD_EDITION\" = \"oss\" ]; then tags=\"-tags oss\"; fi; \\\n",
        "    tags=\"-tags oss\"; \\\n",
    )
    replace_once(
        dockerfile,
        "COPY stand-alone-tomcat-10/lib/ojdbc11.jar /usr/local/tomcat/lib/\n",
        "",
    )
    replace_once(
        dockerfile,
        "# Builds existing jrs-server + packages existing Tomcat 10 Jakarta WAR/JDBC.\n",
        "# Builds existing jrs-server and the Tomcat 10 Jakarta WAR from stand-alone-tomcat-10.\n",
    )
    replace_once(
        dockerfile,
        "# -----------------------------------------------------------------------------\n"
        "# Stage 2: runtime = JDK + Tomcat 10 + WAR + JDBC + Go + PHP seed tooling\n"
        "# -----------------------------------------------------------------------------\n",
        "# -----------------------------------------------------------------------------\n"
        "# Stage 2: Jakarta WAR from source (stand-alone-tomcat-10/build.sh)\n"
        "# -----------------------------------------------------------------------------\n"
        "FROM maven:3.9.9-eclipse-temurin-17 AS war-builder\n"
        "WORKDIR /src\n"
        "COPY stand-alone-tomcat-10/pom.xml stand-alone-tomcat-10/pom.xml\n"
        "COPY stand-alone-tomcat-10/src stand-alone-tomcat-10/src\n"
        "COPY stand-alone-tomcat-10/map-src stand-alone-tomcat-10/map-src\n"
        "COPY stand-alone-tomcat-10/sample-acugis.properties stand-alone-tomcat-10/sample-acugis.properties\n"
        "COPY stand-alone-tomcat-10/dist-README.txt stand-alone-tomcat-10/dist-README.txt\n"
        "WORKDIR /src/stand-alone-tomcat-10\n"
        "RUN mvn -B -Pstandalone -DskipTests package\n"
        "\n"
        "# -----------------------------------------------------------------------------\n"
        "# Stage 3: runtime = JDK + Tomcat 10 + WAR + JDBC + Go + PHP seed tooling\n"
        "# -----------------------------------------------------------------------------\n",
    )
    replace_once(
        dockerfile,
        "COPY stand-alone-tomcat-10/dist/acugis-report-server.war /usr/local/tomcat/webapps/ROOT.war\n",
        "COPY --from=war-builder /src/stand-alone-tomcat-10/target/acugis-report-server.war /usr/local/tomcat/webapps/ROOT.war\n",
    )
    replace_once(
        dockerfile,
        "COPY stand-alone-tomcat-10/dist/sample-acugis.properties /usr/local/tomcat/conf/acugis.properties\n",
        "COPY stand-alone-tomcat-10/sample-acugis.properties /usr/local/tomcat/conf/acugis.properties\n",
    )
    replace_once(
        dockerfile,
        "RUN chmod +x /usr/local/bin/docker-entrypoint.sh /usr/local/bin/jrs-server /usr/local/bin/jrs-run-schedule /usr/local/bin/jrs-schedule-dispatcher\n",
        "RUN sed -i 's/\\r$//' /usr/local/bin/docker-entrypoint.sh \\\n"
        "    && chmod +x /usr/local/bin/docker-entrypoint.sh /usr/local/bin/jrs-server /usr/local/bin/jrs-run-schedule /usr/local/bin/jrs-schedule-dispatcher\n",
    )
    for ignore_name in (".dockerignore", "docker/.dockerignore"):
        replace_once(dest / ignore_name, "stand-alone-tomcat-10/src\n", "")

    replace_all_required(
        dest / "docker" / "compose.yml",
        "acugis/report-server:1.0.19-professional",
        f"acugis/report-server:{OSS_RELEASE}-oss",
    )
    replace_all_required(
        dest / "docker" / "compose.build.yml",
        "BUILD_EDITION: ${BUILD_EDITION:-professional}",
        "BUILD_EDITION: oss",
    )
    replace_all_required(
        dest / "docker" / "report-server" / "docker-entrypoint.sh",
        "ACUGIS_BUILD_EDITION:-professional",
        "ACUGIS_BUILD_EDITION:-oss",
    )
    replace_all_required(dest / "docker" / ".env.example", "BUILD_EDITION=professional", "BUILD_EDITION=oss")
    replace_all_required(
        dest / "docker" / ".env.example",
        "acugis/report-server:1.0.19-professional",
        f"acugis/report-server:{OSS_RELEASE}-oss",
    )
    replace_once(
        dest / "docker" / "README.md",
        "Published images are `acugis/report-server:1.0.19-oss` and\n"
        "`acugis/report-server:1.0.19-professional`. An OSS image cannot unlock\n"
        "Professional modules. A Professional image without a valid ACL1 license runs\n"
        "as Community. `ACUGIS_EDITION` is not a license.\n",
        "This source tree builds the Open Source image only (`-tags oss`).\n"
        "Professional modules are not included. `ACUGIS_EDITION` is not a license.\n",
    )
    replace_once(
        dest / "docker" / "README.md",
        "## Editions\n\n"
        "Published images:\n\n"
        "```text\n"
        "acugis/report-server:1.0.19-oss\n"
        "acugis/report-server:oss\n"
        "acugis/report-server:1.0.19-professional\n"
        "acugis/report-server:professional\n"
        "```\n\n"
        "Build them with:\n\n"
        "```bash\n"
        "make docker-build-oss VERSION=1.0.19\n"
        "make docker-build-professional VERSION=1.0.19\n"
        "```\n\n"
        "`make docker-push-oss` and `make docker-push-professional` push those tags.\n"
        "Package builds do not push. Customer Compose pulls\n"
        "`acugis/report-server:1.0.19-professional` unless `REPORT_SERVER_IMAGE` is set.\n\n"
        "`BUILD_EDITION=oss|professional` selects the Go build tags for a local image\n"
        "build (`compose.build.yml`). The OSS image omits Professional modules. The\n"
        "Professional image verifies an ACL1 license locally.\n",
        "## Editions\n\n"
        "This source tree publishes the Open Source image:\n\n"
        "```text\n"
        f"acugis/report-server:{OSS_RELEASE}-oss\n"
        "acugis/report-server:oss\n"
        "```\n\n"
        "```bash\n"
        f"make docker-build-oss VERSION={OSS_RELEASE}\n"
        "```\n\n"
        "`make docker-push-oss` pushes those tags. Package builds do not push.\n"
        f"Customer Compose pulls `acugis/report-server:{OSS_RELEASE}-oss`.\n\n"
        "The image compiles with `-tags oss`. Professional modules are not in this tree.\n",
    )
    replace_once(
        dest / "documentation" / "docs" / "administration" / "docker.md",
        "Customer Compose pulls `acugis/report-server:1.0.19-professional`. Set `REPORT_SERVER_IMAGE=acugis/report-server:1.0.19-oss` for the Open Source image. An Open Source image does not compile Professional modules. A Professional image without a valid license still includes Data Explorer and embedded QGIS. Map Reports, dashboards, Dataset Intelligence, MCP, and the other license-gated modules stay off until a license is valid. `ACUGIS_EDITION` is not a license. See the [compatibility matrix](../reference/compatibility-matrix.md).\n",
        f"Customer Compose pulls `acugis/report-server:{OSS_RELEASE}-oss`. The image compiles the Open Source server. Professional modules are not in this tree. `ACUGIS_EDITION` is not a license. See the [compatibility matrix](../reference/compatibility-matrix.md).\n",
    )
    replace_once(
        dest / "documentation" / "docs" / "reference" / "environment-variables.md",
        "| `ACUGIS_BUILD_EDITION` | Which schema file the entrypoint applies | `professional` unless the image was built `oss` |\n",
        "| `ACUGIS_BUILD_EDITION` | Which schema file the entrypoint applies | `oss` |\n",
    )
    replace_once(
        dest / "documentation" / "docs" / "reference" / "environment-variables.md",
        "`BUILD_EDITION` is a Docker build argument (`professional` or `oss`). It chooses which binary is compiled into the image. It is not read by a running `jrs-server` as a license.\n",
        "`BUILD_EDITION` is `oss` for this source tree. The image compiles with `-tags oss`. It is not read by a running `jrs-server` as a license.\n",
    )
    for script in ("package-release.sh", "package-deb.sh"):
        replace_all_required(dest / "scripts" / script, "oss|professional", "oss")
    replace_once(
        dest / "scripts" / "package-release.sh",
        "    ojdbc11.jar \\\n",
        "",
    )
    # db2jcc4.jar is not in the distributable jar list. If it is reintroduced,
    # still omit it from the Open Source package.
    release_sh = dest / "scripts" / "package-release.sh"
    db2_line = "    db2jcc4.jar \\\n"
    release_text = release_sh.read_text(encoding="utf-8")
    if db2_line in release_text:
        release_sh.write_text(release_text.replace(db2_line, "", 1), encoding="utf-8")
    # arcgis-connections.properties feeds Java ArcGISAuth, which is not in this
    # tree. migrate_arcgis.sql and jasper_arcgis in schema.sql stay.
    arcgis_props = re.compile(
        r"^([ \t]*)docker/report-server/arcgis-connections\.properties\r?\n",
        re.M,
    )
    for verify_name in ("verify-package.sh", "verify-deb.sh"):
        verify = dest / "scripts" / verify_name
        text = verify.read_text(encoding="utf-8")
        updated, count = arcgis_props.subn("", text, count=1)
        if count != 1:
            raise SystemExit(f"export scrub missed ArcGIS properties check in {verify_name}")
        verify.write_text(updated, encoding="utf-8")
    replace_once(
        dest / "go-report-server" / "internal" / "packaging" / "revision_test.go",
        'for _, edition := range []string{"oss", "professional"} {',
        'for _, edition := range []string{"oss"} {',
    )
    replace_once(
        dest / "go-report-server" / "internal" / "packaging" / "revision_test.go",
        'if strings.Count(src, "--build-arg SOURCE_REVISION=$(SOURCE_REVISION)") < 2 {\n'
        '\t\t\tt.Fatal("docker Makefile does not pass SOURCE_REVISION for both editions")\n'
        '\t\t}',
        'if strings.Count(src, "--build-arg SOURCE_REVISION=$(SOURCE_REVISION)") < 1 {\n'
        '\t\t\tt.Fatal("docker Makefile does not pass SOURCE_REVISION")\n'
        '\t\t}',
    )
    renderer = dest / "go-report-server" / "internal" / "packaging" / "map_renderer_test.go"
    renderer_text = renderer.read_text(encoding="utf-8")
    jar_fatal = '\tif _, err := os.Stat(jar); err != nil {\n\t\tt.Fatalf("built map jar missing: %s", jar)\n\t}'
    if jar_fatal in renderer_text:
        renderer.write_text(
            renderer_text.replace(
                jar_fatal,
                '\tif _, err := os.Stat(jar); err != nil {\n\t\tt.Skip("built map jar is not part of this source tree")\n\t}',
                1,
            ),
            encoding="utf-8",
        )
    replace_once(
        renderer,
        '\tif _, err := os.Stat(war); err != nil {\n\t\tt.Fatalf("report server WAR missing: %s", war)\n\t}',
        '\tif _, err := os.Stat(war); err != nil {\n\t\tt.Skip("report server WAR is produced by stand-alone-tomcat-10/build.sh and is not part of this source tree")\n\t}',
    )
    for rel in (
        "README.md",
        "docker/README.md",
        "docker/compose.yml",
        "docker/compose.build.yml",
        "docker/.env.example",
        "documentation/docs/administration/docker.md",
    ):
        path = dest / rel
        text = path.read_text(encoding="utf-8")
        if "1.0.19" in text:
            path.write_text(text.replace("1.0.19", OSS_RELEASE), encoding="utf-8")


PUBLIC_GITIGNORE = """\
# Public Open Source tree. Written by scripts/export-oss-source.sh.
# The private repository .gitignore is not this file.

node_modules/
*.war
*.war.bak
*.zip
catalina.out
*.duckdb
*.jasper
schedules/*_env.sh
schedules/logs/
schedules/output/
*.pem
*.key
**/target/
**/bin/
dist/

# Text files that ship beside the WAR output. The WAR itself stays ignored.
!stand-alone-tomcat-10/dist/
stand-alone-tomcat-10/dist/*
!stand-alone-tomcat-10/dist/README.txt
!stand-alone-tomcat-10/dist/sample-acugis.properties

*.log
*.class
.env
.env.*
!.env.example
docker/report-server/context.xml
debug.php
**/obj/
__pycache__/
*.py[cod]
.idea/
.vscode/
.DS_Store
Thumbs.db
documentation/site/
go-report-server/openapi.bundle.yaml
"""


def write_public_gitignore(dest: Path) -> None:
    (dest / ".gitignore").write_text(PUBLIC_GITIGNORE, encoding="utf-8", newline="\n")


def scrub_debug_and_viewer(dest: Path) -> None:
    """Drop the debug embedded-map route after its handler is excluded."""
    routes = dest / "go-report-server" / "internal" / "server" / "routes.go"
    replace_once(
        routes,
        "\t\tmux.Get(\"/reports/debug_embedded_map\", s.handleDebugEmbeddedMap)\n"
        "\t\tmux.Get(\"/reports/debug_embedded_map.php\", s.handleDebugEmbeddedMap)\n"
        "\t\tmux.Get(\"/api/v1/embedded-map/debug\", s.handleDebugEmbeddedMap)\n",
        "",
    )
    viewer = dest / "go-report-server" / "internal" / "server" / "viewer.html"
    replace_once(viewer, '<script src="../assets/js/spatial-selection.js"></script>\n', "")


def scrub_php_for_missing_map_libs(dest: Path) -> None:
    """Keep ordinary PHP report pages loadable after Map Report PHP is omitted.

    Community embedded maps in Go and Java stay. The PHP Map Report libraries
    are not in this tree. Report pages must not fatal merely because those
    files are absent. An enabled PHP embedded map still needs the renderer,
    which this tree does not ship.
    """
    embedded = dest / "lib" / "embedded_map.php"
    replace_once(
        embedded,
        "require_once __DIR__ . '/native_paths.php';\n"
        "require_once __DIR__ . '/map_report.php';\n"
        "require_once __DIR__ . '/map_renderer.php';\n"
        "require_once __DIR__ . '/point_style.php';\n"
        "require_once __DIR__ . '/map_thematic.php';\n",
        "require_once __DIR__ . '/native_paths.php';\n"
        "foreach (array('map_report.php', 'map_request.php', 'map_renderer.php', 'point_style.php', 'map_thematic.php') as $jrsMapLib) {\n"
        "    $jrsMapLibPath = __DIR__ . '/' . $jrsMapLib;\n"
        "    if (is_file($jrsMapLibPath)) {\n"
        "        require_once $jrsMapLibPath;\n"
        "    }\n"
        "}\n"
        "if (!defined('JRS_MAP_RENDERER_POSTGIS')) {\n"
        "    define('JRS_MAP_RENDERER_POSTGIS', 'postgis');\n"
        "}\n"
        "if (!defined('JRS_MAP_RENDERER_GEOSERVER')) {\n"
        "    define('JRS_MAP_RENDERER_GEOSERVER', 'geoserver');\n"
        "}\n"
        "if (!defined('JRS_MAP_RENDERER_QGIS')) {\n"
        "    define('JRS_MAP_RENDERER_QGIS', 'qgis');\n"
        "}\n"
        "if (!defined('JRS_EMBEDDED_MAP_PARAM')) {\n"
        "    define('JRS_EMBEDDED_MAP_PARAM', 'P_MAP_IMAGE');\n"
        "}\n"
        "if (!defined('JRS_EMBEDDED_MAP_BANDS')) {\n"
        "    define('JRS_EMBEDDED_MAP_BANDS', array(\n"
        "        'summary' => 'Summary',\n"
        "        'detail' => 'Detail',\n"
        "        'groupHeader' => 'Group Header',\n"
        "    ));\n"
        "}\n",
    )
    view = dest / "reports" / "view.php"
    replace_once(
        view,
        "require_once __DIR__ . '/../lib/map_report.php';\n",
        "if (is_file(__DIR__ . '/../lib/map_report.php')) {\n"
        "    require_once __DIR__ . '/../lib/map_report.php';\n"
        "}\n",
    )
    replace_once(
        view,
        "$isMapReport = jrs_is_map_report($definition);\n",
        "$isMapReport = function_exists('jrs_is_map_report') && jrs_is_map_report($definition);\n",
    )


def professional_package_errors(dest: Path) -> list[str]:
    """Fail when a Professional package still contains implementation files."""
    errors: list[str] = []
    if not dest.is_dir():
        return errors
    for path in dest.rglob("*"):
        if not path.is_file():
            continue
        rel = path.relative_to(dest).as_posix()
        for prefix, policy in PROFESSIONAL_PACKAGE_DIRS.items():
            if rel != prefix and not rel.startswith(prefix + "/"):
                continue
            name = path.name
            if policy == "none":
                errors.append(f"Professional implementation still present: {rel}")
            elif policy == "stub" and not oss_stub_name(name):
                errors.append(f"Professional implementation still present: {rel}")
            elif policy == "oss-named" and not oss_stub_name(name):
                errors.append(f"Professional implementation still present: {rel}")
            break
    return errors


def forbidden_binary_errors(dest: Path) -> list[str]:
    errors: list[str] = []
    if not dest.is_dir():
        return errors
    for path in dest.rglob("*"):
        if not path.is_file():
            continue
        name = path.name.lower()
        if name in {"ojdbc11.jar", "db2jcc4.jar"} or name.startswith("ojdbc") or name.startswith("db2jcc"):
            errors.append(f"forbidden JDBC binary: {path.relative_to(dest).as_posix()}")
    return errors


def secret_errors(dest: Path) -> list[str]:
    """Flag likely secret material. Never include the secret value."""
    errors: list[str] = []
    if not dest.is_dir():
        return errors
    pem = re.compile(
        r"-----BEGIN (?:RSA |OPENSSH |EC |DSA )?PRIVATE KEY-----\s*[A-Za-z0-9+/=\r\n]{80,}-----END",
    )
    token = re.compile(
        r"\b(?:ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|AKIA[0-9A-Z]{16}|xox[baprs]-[A-Za-z0-9-]{10,})"
    )
    url_cred = re.compile(
        r"(?i)(?:postgres(?:ql)?|mysql|mongodb|redis|amqp)://[^/\s:]+:([^@\s/'\"`]{4,})@"
    )
    placeholders = {
        "password",
        "secret",
        "changeme",
        "change-me",
        "example",
        "xxx",
        "redacted",
        "todo",
    }
    scanned = 0
    for path in dest.rglob("*"):
        if not path.is_file():
            continue
        rel = path.relative_to(dest).as_posix()
        if any(part in {"node_modules", ".git"} for part in rel.split("/")):
            continue
        name = path.name.lower()
        if name.endswith((".pem", ".key")) or name in {"id_rsa", "id_dsa", "id_ecdsa"}:
            errors.append(f"secret material: private key file {rel}")
            continue
        if path.suffix.lower() not in {
            ".go", ".php", ".js", ".ts", ".yml", ".yaml", ".env", ".sh",
            ".properties", ".json", ".md", ".xml", ".html", ".sql", ".conf",
            ".example", ".txt", ".service",
        }:
            continue
        try:
            if path.stat().st_size > 1_000_000:
                continue
            text = path.read_text(encoding="utf-8", errors="replace")
        except OSError:
            continue
        scanned += 1
        if pem.search(text):
            errors.append(f"secret material: private key block in {rel}")
        if token.search(text):
            errors.append(f"secret material: token-like value in {rel}")
        for match in url_cred.finditer(text):
            secret = match.group(1).strip().lower()
            if secret in placeholders or secret.startswith("change-me"):
                continue
            host = text[match.end(): match.end() + 80]
            if re.match(r"(?i)(localhost|127\.0\.0\.1|0\.0\.0\.0|example\.(com|org|net)|host)\b", host):
                continue
            if path.name.endswith("_test.go") and len(secret) < 12:
                continue
            errors.append(f"secret material: database URL credential in {rel}")
            break
        if len(errors) >= 40:
            break
    if scanned == 0:
        errors.append("secret scan did not read any exported text files")
    return errors


_PHP_REQUIRE_LINE = re.compile(
    r"(?:^|[^\w])(?:require_once|include_once|require|include)(?![\w])\s*\(?\s*(?P<body>[^;]+)"
)
_PHP_DIR_CONCAT = re.compile(
    r"""^__DIR__\s*\.\s*(['"])(?P<path>[^'"]+)\1\s*\)?\s*$"""
)
_PHP_DIRNAME_CONCAT = re.compile(
    r"""^dirname\s*\(\s*__DIR__\s*(?:,\s*(?P<levels>\d+)\s*)?\)\s*\.\s*(['"])(?P<path>[^'"]+)\2\s*\)?\s*$"""
)
_PHP_BARE_PATH = re.compile(
    r"""^(['"])(?P<path>[^'"]+)\1\s*\)?\s*$"""
)
_PHP_VAR_PATH = re.compile(
    r"""^(?P<var>\$[A-Za-z_][A-Za-z0-9_]*)\s*\)?\s*$"""
)


def _php_code_lines(text: str) -> list[tuple[int, str]]:
    """PHP lines with comments removed. Strings that contain comment marks can be misread."""
    visible: list[tuple[int, str]] = []
    in_block = False
    for index, raw in enumerate(text.splitlines(), start=1):
        line = raw
        if in_block:
            end = line.find("*/")
            if end < 0:
                continue
            line = line[end + 2 :]
            in_block = False
        out: list[str] = []
        cursor = 0
        while cursor < len(line):
            if line.startswith("/*", cursor):
                end = line.find("*/", cursor + 2)
                if end < 0:
                    in_block = True
                    break
                cursor = end + 2
                continue
            if line.startswith("//", cursor) or (line.startswith("#", cursor) and cursor == 0):
                break
            out.append(line[cursor])
            cursor += 1
        code = "".join(out).strip()
        if code:
            visible.append((index, code))
    return visible


def _join_php_path(base: Path, rel: str) -> Path:
    current = base
    for part in rel.replace("\\", "/").split("/"):
        if part in ("", "."):
            continue
        if part == "..":
            current = current.parent
            continue
        current = current / part
    return current


def _php_require_guarded(window: list[str], needle: str) -> bool:
    blob = "\n".join(window)
    return "is_file" in blob and needle in blob


def php_missing_include_errors(dest: Path) -> list[str]:
    """Fail when an exported PHP file includes a local file the export omitted.

    A require guarded by is_file() is allowed. That is how bootstrap.php and
    the Map Report scrub load lib/map_report.php only when the file exists.
    """
    errors: list[str] = []
    if not dest.is_dir():
        return errors
    dest_resolved = dest.resolve()
    for path in dest.rglob("*.php"):
        rel = path.relative_to(dest).as_posix()
        if any(part in {".git", "node_modules", "vendor"} for part in rel.split("/")):
            continue
        try:
            if path.stat().st_size > 1_000_000:
                continue
            text = path.read_text(encoding="utf-8", errors="replace")
        except OSError:
            continue
        code_lines = _php_code_lines(text)
        for pos, (line_no, code) in enumerate(code_lines):
            found = list(_PHP_REQUIRE_LINE.finditer(code))
            if not found:
                continue
            window = [item for _, item in code_lines[max(0, pos - 15) : pos + 1]]
            for match in found:
                body = match.group("body").strip()
                errors.extend(
                    _php_require_target_errors(
                        dest_resolved, path, rel, line_no, body, window
                    )
                )
                if len(errors) >= 40:
                    return errors
    return errors


def _php_require_target_errors(
    dest_resolved: Path,
    path: Path,
    rel: str,
    line_no: int,
    body: str,
    window: list[str],
) -> list[str]:
    dir_match = _PHP_DIR_CONCAT.match(body)
    dirname_match = _PHP_DIRNAME_CONCAT.match(body)
    bare_match = _PHP_BARE_PATH.match(body)
    var_match = _PHP_VAR_PATH.match(body)
    if dir_match or dirname_match or bare_match:
        if dir_match:
            base = path.parent
            rel_path = dir_match.group("path")
            quote = dir_match.group(1)
        elif dirname_match:
            levels = int(dirname_match.group("levels") or "1")
            base = path.parent
            for _ in range(levels):
                base = base.parent
            rel_path = dirname_match.group("path")
            quote = dirname_match.group(2)
        else:
            assert bare_match is not None
            base = path.parent
            rel_path = bare_match.group("path")
            quote = bare_match.group(1)
        if _php_require_guarded(window, quote + rel_path + quote):
            return []
        target = _join_php_path(base, rel_path)
        try:
            target_resolved = target.resolve()
        except OSError:
            return [f"broken PHP require: {rel}:{line_no} -> {rel_path}"]
        inside = target_resolved == dest_resolved or dest_resolved in target_resolved.parents
        if not inside or not target.is_file():
            return [f"broken PHP require: {rel}:{line_no} -> {rel_path}"]
        return []
    if var_match:
        if _php_require_guarded(window, var_match.group("var")):
            return []
        return [f"unresolved PHP require: {rel}:{line_no} {var_match.group('var')}"]
    return [f"unresolved PHP require: {rel}:{line_no} {body[:120]}"]


def publication_errors(root: Path, dest: Path) -> list[str]:
    errors: list[str] = []
    license_src = root / "scripts" / "oss-apache-2.0.txt"
    license_out = dest / "LICENSE"
    if not license_out.is_file():
        errors.append("LICENSE missing from export")
    elif not license_src.is_file() or license_out.read_bytes() != license_src.read_bytes():
        errors.append("LICENSE is not the Apache License 2.0 text")
    if (dest / ".git").exists():
        errors.append("private .git was copied into the export")
    boot = dest / "lib" / "bootstrap.php"
    if boot.is_file() and "jrs_write_arcgis_connections_file" in boot.read_text(encoding="utf-8"):
        errors.append("exported bootstrap.php still writes ArcGIS connection files")
    shell_config = dest / "shared" / "ui" / "shell-config.js"
    if shell_config.is_file() and "arcgis_connections.php" in shell_config.read_text(encoding="utf-8"):
        errors.append("exported shell still links to the ArcGIS connection page")
    readme = dest / "README.md"
    if readme.is_file():
        text = readme.read_text(encoding="utf-8")
        if "Apache License, Version 2.0" not in text:
            errors.append("exported README does not identify Apache-2.0")
        if "not included in this tree" not in text:
            errors.append("exported README does not say Professional source is absent")
        if "package-professional" in text or "defaults to `professional`" in text:
            errors.append("exported README still describes Professional build defaults")
        if f"AcuGIS Report Server {OSS_RELEASE}" not in text:
            errors.append(f"exported README does not identify release {OSS_RELEASE}")
        forbidden_readme = (
            "private development repository",
            "does not publish a hosted APT repository",
            "no hosted APT repository",
            "1.0.19",
            "1.0.19-professional",
        )
        for phrase in forbidden_readme:
            if phrase in text:
                errors.append(f"exported README still says {phrase!r}")
    gmake = dest / "go-report-server" / "Makefile"
    if gmake.is_file():
        text = gmake.read_text(encoding="utf-8")
        if "build: build-oss" not in text or "build-professional:" in text:
            errors.append("exported go Makefile does not default to the OSS build")
    dockerfile = dest / "docker" / "report-server" / "Dockerfile"
    if dockerfile.is_file():
        text = dockerfile.read_text(encoding="utf-8")
        if "BUILD_EDITION=professional" in text or "ojdbc11.jar" in text or "db2jcc4.jar" in text:
            errors.append("exported Dockerfile still defaults to Professional or copies omitted JDBC jars")
        if 'tags="-tags oss"' not in text:
            errors.append("exported Dockerfile does not compile with -tags oss")
        if "COPY stand-alone-tomcat-10/dist/acugis-report-server.war" in text:
            errors.append("exported Dockerfile still copies a prebuilt WAR")
        if "AS war-builder" not in text or "mvn -B -Pstandalone -DskipTests package" not in text:
            errors.append("exported Dockerfile does not build the WAR from source")
        if "map-src" not in text:
            errors.append("exported Dockerfile does not compile the Community map renderer sources")
        map_renderer = dest / "stand-alone-tomcat-10/map-src/main/java/com/acugis/map/MapRenderer.java"
        if not map_renderer.is_file():
            errors.append("exported tree is missing Community MapRenderer.java")
        if "map-lib" in text or "barcode-lib" in text:
            errors.append("exported Dockerfile still copies gitignored map-lib or barcode-lib jars")
    release_sh = dest / "scripts" / "package-release.sh"
    if release_sh.is_file() and "    db2jcc4.jar \\" in release_sh.read_text(encoding="utf-8"):
        errors.append("exported package-release.sh still packages db2jcc4.jar")
    for ignore_name in (".dockerignore", "docker/.dockerignore"):
        ignore = dest / ignore_name
        if ignore.is_file() and "stand-alone-tomcat-10/src" in ignore.read_text(encoding="utf-8"):
            errors.append(f"exported {ignore_name} still excludes the WAR source")
    docs = dest / "documentation"
    if docs.is_dir():
        for path in docs.rglob("*"):
            if not path.is_file() or path.suffix.lower() not in {".md", ".yml", ".html"}:
                continue
            if "All rights reserved" in path.read_text(encoding="utf-8", errors="replace"):
                errors.append(f"exported docs still say All rights reserved: {path.relative_to(dest).as_posix()}")
    compose = dest / "docker" / "compose.yml"
    if compose.is_file():
        text = compose.read_text(encoding="utf-8")
        if ":-professional" in text or "1.0.19-professional" in text or "acugis-report-server:local" in text:
            errors.append("exported Compose still defaults to a professional or local build image")
        if f"acugis/report-server:{OSS_RELEASE}-oss" not in text:
            errors.append("exported Compose does not pull the Open Source image")
        if "1.0.19" in text:
            errors.append("exported Compose still names release 1.0.19")
    return errors


def export_tree(root: Path, dest: Path) -> None:
    root = root.resolve()
    dest = dest.resolve()
    if dest == root or root == dest or str(root).startswith(str(dest) + "\\") or str(root).startswith(str(dest) + "/"):
        raise SystemExit(f"refusing to export over the source tree: {dest}")
    rules = load_rules(root / "scripts" / "oss-export-exclude.txt")
    uncovered = [rel for rel in go_build_oss_files(root) if not excluded(rel, rules)]
    if uncovered:
        raise SystemExit(
            "add these //go:build !oss files to scripts/oss-export-exclude.txt before exporting:\n  "
            + "\n  ".join(uncovered)
        )
    copied = copy_tree(root, dest, rules)
    apply_schema(dest)
    record_source_revision(root, dest)
    scrub_docs(dest)
    detach_sdk(dest)
    scrub_public_readme(dest)
    scrub_oss_build_defaults(dest)
    scrub_shared_php(dest)
    scrub_php_for_missing_map_libs(dest)
    scrub_debug_and_viewer(dest)
    scrub_oss_doc_copyright(dest)
    write_apache_license(root, dest)
    write_public_gitignore(dest)
    print(f"exported {copied} files to {dest}")


def main(argv: list[str]) -> None:
    if len(argv) != 4 or argv[1] not in {"export", "leak-check"}:
        raise SystemExit("usage: oss_export.py export|leak-check <root> <dest>")
    root = Path(argv[2])
    dest = Path(argv[3])
    if argv[1] == "export":
        export_tree(root, dest)
        leak_check(root, dest, require_tidy=False)
    else:
        leak_check(root, dest, require_tidy=True)


if __name__ == "__main__":
    main(sys.argv)
