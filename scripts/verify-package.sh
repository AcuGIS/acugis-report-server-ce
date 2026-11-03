#!/usr/bin/env bash
# Verify a finished native release archive (not the source tree).
# Usage: scripts/verify-package.sh <oss|professional> [VERSION]

set -euo pipefail

EDITION="${1:-oss}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${2:-${VERSION:-}}"
if [[ -z "$VERSION" ]]; then
  VERSION="$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || true)"
fi
if [[ -z "$VERSION" ]]; then
  VERSION="0.0.0"
fi

DIST_DIR="${DIST_DIR:-$ROOT/dist}"
ARCH="linux-amd64"
NAME="acugis-report-server-${EDITION}-${VERSION}-${ARCH}.tar.gz"
TARBALL="$DIST_DIR/$NAME"
WORK="${VERIFY_DIR:-$DIST_DIR/verify-$EDITION}"

log() { echo "[verify-$EDITION] $*"; }
fail() { echo "[verify-$EDITION] FAIL: $*" >&2; FAIL=1; }
pass() { echo "[verify-$EDITION] ok: $*"; }

FAIL=0
require_file() {
  local path="$1"
  if [[ -f "$path" ]]; then
    pass "$path"
  else
    fail "missing $path"
  fi
}
forbid_file() {
  local path="$1"
  if [[ -e "$path" ]]; then
    fail "should be absent: $path"
  else
    pass "absent $path"
  fi
}

[[ -f "$TARBALL" ]] || { echo "missing archive: $TARBALL" >&2; exit 1; }
[[ -f "${TARBALL}.sha256" ]] || fail "missing ${TARBALL}.sha256"

rm -rf "$WORK"
mkdir -p "$WORK"
tar -xzf "$TARBALL" -C "$WORK"
ROOTDIR="$WORK/acugis-report-server"
[[ -d "$ROOTDIR" ]] || { echo "archive root acugis-report-server/ missing" >&2; exit 1; }

require_file "$ROOTDIR/bin/jrs-server"
require_file "$ROOTDIR/bin/jrs-run-schedule"
forbid_file "$ROOTDIR/bin/jrs-schedule-dispatcher"
forbid_file "$ROOTDIR/go-report-server/go.mod"
forbid_file "$ROOTDIR/.git"
require_file "$ROOTDIR/assets/js/map-style.js"
require_file "$ROOTDIR/sdk/javascript/dist/acugis-report.js"
require_file "$ROOTDIR/sdk/javascript/dist/acugis-report.min.js"
require_file "$ROOTDIR/sdk/javascript/dist/acugis-report.esm.js"
require_file "$ROOTDIR/sdk/javascript/dist/index.d.ts"
if grep -q 'embedReport' "$ROOTDIR/sdk/javascript/dist/acugis-report.js" \
  && grep -q 'embedReport' "$ROOTDIR/sdk/javascript/dist/acugis-report.min.js"; then
  pass "SDK bundles export embedReport"
else
  fail "SDK bundles missing embedReport"
fi
if [[ -d "$ROOTDIR/sdk/javascript/node_modules" ]]; then
  fail "SDK node_modules must not be packaged"
else
  pass "SDK node_modules absent"
fi
require_file "$ROOTDIR/database/schema.sql"
forbid_file "$ROOTDIR/database/schema.oss.sql"
require_file "$ROOTDIR/database/migrate_geoserver.sql"
require_file "$ROOTDIR/database/migrate_datasource_options.sql"
require_file "$ROOTDIR/tomcat/acugis-report-server.war"
require_file "$ROOTDIR/tomcat/lib/postgresql-42.7.3.jar"
require_file "$ROOTDIR/scripts/run_schedule.php"
require_file "$ROOTDIR/scripts/seed_admin.php"
require_file "$ROOTDIR/scripts/native-setup.sh"
require_file "$ROOTDIR/scripts/configure-instance.sh"
if grep -q 'install_schedule_sudoers' "$ROOTDIR/scripts/native-setup.sh"; then
  pass "native-setup installs schedule sudoers"
else
  fail "native-setup missing install_schedule_sudoers"
fi
if grep -q 'ensure_embed_token_secret' "$ROOTDIR/scripts/native-setup.sh"; then
  pass "native-setup generates embed token secret"
else
  fail "native-setup missing ensure_embed_token_secret"
fi
# lib/map_report.php is Professional-only. The OSS export excludes it, and
# bootstrap.php loads it only when the file is present.
if [[ "$EDITION" == "professional" ]]; then
  require_file "$ROOTDIR/lib/map_report.php"
else
  forbid_file "$ROOTDIR/lib/map_report.php"
fi
require_file "$ROOTDIR/systemd/acugis-report-server.service"
require_file "$ROOTDIR/install.sh"
require_file "$ROOTDIR/RELEASE"
require_file "$ROOTDIR/release.json"
require_file "$ROOTDIR/reports/portal.css"
require_file "$ROOTDIR/shared/ui/shell.css"
require_file "$ROOTDIR/email_tmpl/email_template.html"
require_file "$ROOTDIR/docs/INSTALL.md"
require_file "$ROOTDIR/lib/bootstrap.php"
require_file "$ROOTDIR/config.php"

if grep -q "edition=${EDITION}" "$ROOTDIR/RELEASE"; then
  pass "RELEASE edition=$EDITION"
else
  fail "RELEASE edition mismatch"
fi
if grep -q "\"edition\": \"${EDITION}\"" "$ROOTDIR/release.json"; then
  pass "release.json edition=$EDITION"
else
  fail "release.json edition mismatch"
fi
if grep -q "version=${VERSION}" "$ROOTDIR/RELEASE"; then
  pass "RELEASE version=$VERSION"
else
  fail "RELEASE version mismatch (got $(grep ^version= "$ROOTDIR/RELEASE" || true))"
fi

# Scheduling + GeoServer present in both editions.
if [[ -d "$ROOTDIR/schedules" ]]; then
  pass "schedules directory"
else
  fail "schedules directory missing"
fi

PRO_SQL=(
  database/migrations/009_dataset_intelligence.sql
  database/migrations/010_audit_log.sql
  database/migrations/011_oidc_external_identity.sql
  database/migrations/012_saml_sso.sql
  database/migrations/013_usage_analytics_indexes.sql
  database/migrations/014_governance_metadata.sql
  database/migrations/015_audit_export.sql
  database/migrate_qgis.sql
  assets/js/spatial-selection.js
  docker/report-server/qgis-connections.properties
)
ARCGIS_SQL=(
  database/migrate_arcgis.sql
)

if [[ "$EDITION" == "oss" ]]; then
  for rel in "${PRO_SQL[@]}"; do
    forbid_file "$ROOTDIR/$rel"
  done
  for rel in "${ARCGIS_SQL[@]}"; do
    require_file "$ROOTDIR/$rel"
  done
  forbid_file "$ROOTDIR/database/migrations/professional"
  if grep -qiE 'jasper_qgis|auth_oidc_settings|auth_saml_settings|dataset_intelligence' "$ROOTDIR/database/schema.sql"; then
    fail "OSS schema.sql still contains Professional objects"
  else
    pass "OSS schema.sql has no Professional objects"
  fi
  if grep -q 'jasper_arcgis' "$ROOTDIR/database/schema.sql"; then
    pass "OSS schema.sql contains jasper_arcgis"
  else
    fail "OSS schema.sql missing jasper_arcgis"
  fi
else
  for rel in "${PRO_SQL[@]}"; do
    require_file "$ROOTDIR/$rel"
  done
  for rel in "${ARCGIS_SQL[@]}"; do
    require_file "$ROOTDIR/$rel"
  done
  if grep -qiE 'jasper_qgis|jasper_arcgis|auth_oidc_settings' "$ROOTDIR/database/schema.sql"; then
    pass "Professional schema.sql contains Professional objects"
  else
    fail "Professional schema.sql missing expected Professional objects"
  fi
fi

# -version on the Linux binary (docker or native Linux).
run_version() {
  local bin="$ROOTDIR/bin/jrs-server"
  if [[ "$(uname -s 2>/dev/null)" == "Linux" ]]; then
    ( cd "$ROOTDIR" && ./bin/jrs-server -version )
    return
  fi
  if command -v docker >/dev/null 2>&1; then
    if command -v timeout >/dev/null 2>&1; then
      if ! timeout 90 docker info >/dev/null 2>&1; then
        log "skip -version (Docker daemon not ready)"
        return 0
      fi
    elif ! docker info >/dev/null 2>&1; then
      log "skip -version (Docker daemon not ready)"
      return 0
    fi
    local hostdir="$ROOTDIR"
    if command -v cygpath >/dev/null 2>&1; then
      hostdir="$(cygpath -w "$ROOTDIR")"
    elif [[ "$(uname -o 2>/dev/null)" == Msys ]]; then
      hostdir="$(cd "$ROOTDIR" && pwd -W)"
    fi
    MSYS_NO_PATHCONV=1 docker run --rm \
      -v "${hostdir}:/opt/acugis-report-server:ro" \
      -e JRS_JASPER_HOME=/opt/acugis-report-server \
      -w /opt/acugis-report-server \
      --entrypoint /opt/acugis-report-server/bin/jrs-server \
      golang:1.26.3-bookworm \
      -version
    return
  fi
  log "skip -version (not Linux and Docker unavailable)"
  return 0
}

if ver_out="$(run_version 2>&1)"; then
  echo "$ver_out"
  if echo "$ver_out" | grep -q "AcuGIS Report Server"; then
    pass "jrs-server -version"
  elif echo "$ver_out" | grep -q "skip -version"; then
    pass "jrs-server -version skipped"
  else
    fail "jrs-server -version unexpected output"
  fi
else
  fail "jrs-server -version failed: $ver_out"
fi

if [[ "$FAIL" -ne 0 ]]; then
  echo "[verify-$EDITION] FAILED" >&2
  exit 1
fi
echo "[verify-$EDITION] PASSED $NAME"
