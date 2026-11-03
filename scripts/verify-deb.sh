#!/usr/bin/env bash
# Verify a finished .deb (not merely the staging directory).
# Usage: scripts/verify-deb.sh <oss|professional> [VERSION]

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

DEB_ARCH="${DEB_ARCH:-amd64}"
DEB_REVISION="${DEB_REVISION:-}"
if [[ -n "$DEB_REVISION" ]]; then
  DEB_VERSION="${VERSION}-${DEB_REVISION}"
else
  DEB_VERSION="$VERSION"
fi

if [[ "$EDITION" == "professional" ]]; then
  DEB_PACKAGE="acugis-report-server-professional"
else
  DEB_PACKAGE="acugis-report-server"
fi

DIST_DIR="${DIST_DIR:-$ROOT/dist}"
DEB_FILE="$DIST_DIR/${DEB_PACKAGE}_${DEB_VERSION}_${DEB_ARCH}.deb"
WORK="${VERIFY_DEB_DIR:-$DIST_DIR/verify-deb-$EDITION}"

log() { echo "[verify-deb-$EDITION] $*"; }
fail() { echo "[verify-deb-$EDITION] FAIL: $*" >&2; FAIL=1; }
pass() { echo "[verify-deb-$EDITION] ok: $*"; }
FAIL=0

host_path() {
  local p="$1"
  if command -v cygpath >/dev/null 2>&1; then
    cygpath -w "$p"
  elif [[ "$(uname -o 2>/dev/null)" == Msys ]]; then
    (cd "$p" && pwd -W)
  else
    echo "$p"
  fi
}

have_docker() {
  command -v docker >/dev/null 2>&1 || return 1
  if command -v timeout >/dev/null 2>&1; then
    timeout 90 docker info >/dev/null 2>&1
  else
    docker info >/dev/null 2>&1
  fi
}

extract_deb() {
  if command -v dpkg-deb >/dev/null 2>&1; then
    dpkg-deb --info "$DEB_FILE" > "$INFO"
    dpkg-deb --contents "$DEB_FILE" > "$CONTENTS"
    mkdir -p "$WORK/control-files" "$WORK/data/opt/acugis-report-server/database"
    dpkg-deb --control "$DEB_FILE" "$WORK/control-files"
    dpkg-deb --fsys-tarfile "$DEB_FILE" | tar -x -C "$WORK/data" \
      ./opt/acugis-report-server/RELEASE \
      ./opt/acugis-report-server/database/schema.sql \
      ./opt/acugis-report-server/bin/jrs-server \
      ./opt/acugis-report-server/scripts/native-setup.sh \
      ./opt/acugis-report-server/sdk/javascript/dist/acugis-report.js \
      ./opt/acugis-report-server/sdk/javascript/dist/acugis-report.min.js \
      ./opt/acugis-report-server/sdk/javascript/dist/acugis-report.esm.js \
      ./opt/acugis-report-server/sdk/javascript/dist/index.d.ts
    return
  fi
  have_docker || { echo "dpkg-deb not available (install dpkg-dev or Docker)" >&2; exit 1; }
  local hostdeb hostwork cid
  hostdeb="$(host_path "$DEB_FILE")"
  hostwork="$(host_path "$WORK")"
  cid="$(MSYS_NO_PATHCONV=1 docker create ubuntu:24.04 sleep 600)"
  MSYS_NO_PATHCONV=1 docker start "$cid" >/dev/null
  MSYS_NO_PATHCONV=1 docker exec "$cid" mkdir -p /deb /out /out/control-files /out/data/opt/acugis-report-server/database
  MSYS_NO_PATHCONV=1 docker cp "$hostdeb" "${cid}:/deb/pkg.deb"
  MSYS_NO_PATHCONV=1 docker exec "$cid" bash -c 'set -euo pipefail
    dpkg-deb --info /deb/pkg.deb > /out/info.txt
    dpkg-deb --contents /deb/pkg.deb > /out/contents.txt
    dpkg-deb --control /deb/pkg.deb /out/control-files
    dpkg-deb --fsys-tarfile /deb/pkg.deb | tar -x -C /out/data \
      ./opt/acugis-report-server/RELEASE \
      ./opt/acugis-report-server/database/schema.sql \
      ./opt/acugis-report-server/scripts/native-setup.sh \
      ./opt/acugis-report-server/sdk/javascript/dist/acugis-report.js \
      ./opt/acugis-report-server/sdk/javascript/dist/acugis-report.min.js \
      ./opt/acugis-report-server/sdk/javascript/dist/acugis-report.esm.js \
      ./opt/acugis-report-server/sdk/javascript/dist/index.d.ts'
  MSYS_NO_PATHCONV=1 docker cp "${cid}:/out/." "$hostwork/"
  MSYS_NO_PATHCONV=1 docker rm -f "$cid" >/dev/null
}

[[ -f "$DEB_FILE" ]] || { echo "missing deb: $DEB_FILE" >&2; exit 1; }

rm -rf "$WORK"
mkdir -p "$WORK"

INFO="$WORK/info.txt"
CONTENTS="$WORK/contents.txt"
extract_deb

CONTROL="$WORK/control-files/control"
[[ -f "$CONTROL" ]] || CONTROL="$WORK/data/DEBIAN/control"
if [[ ! -f "$CONTROL" ]]; then
  # dpkg-deb --control writes to the given dir
  CONTROL="$WORK/control-files/control"
fi
if [[ ! -f "$CONTROL" ]]; then
  fail "control file missing after dpkg-deb --control"
else
  pass "control extracted"
fi

need_grep() {
  local file="$1" pat="$2" msg="$3"
  if grep -E -q "$pat" "$file"; then
    pass "$msg"
  else
    fail "$msg (pattern $pat)"
  fi
}

need_grep "$INFO" "^ Package: ${DEB_PACKAGE}\$" "package name $DEB_PACKAGE"
need_grep "$INFO" "^ Version: ${DEB_VERSION}\$" "version $DEB_VERSION"
need_grep "$INFO" "^ Architecture: ${DEB_ARCH}\$" "architecture $DEB_ARCH"
need_grep "$INFO" "openjdk-17-jre-headless" "Depends includes Java 17"
need_grep "$INFO" "postgresql," "Depends includes postgresql"
need_grep "$INFO" "postgresql-client" "Depends includes postgresql-client"
need_grep "$INFO" "php-cli" "Depends includes php-cli"
need_grep "$INFO" "php-pgsql" "Depends includes php-pgsql"
need_grep "$INFO" "php-xml" "Depends includes php-xml"
need_grep "$INFO" "php-curl" "Depends includes php-curl"
need_grep "$INFO" "php-gd" "Depends includes php-gd"
need_grep "$INFO" "openssl" "Depends includes openssl"
need_grep "$INFO" " sudo" "Depends includes sudo"
need_grep "$INFO" "^ Depends: .*apache2" "Depends includes apache2"
need_grep "$INFO" "Homepage: https://acugis.com/" "Homepage"

if grep -qE '^ Suggests: apache2' "$INFO"; then
  fail "apache2 must be Depends, not Suggests"
else
  pass "apache2 is not Suggests"
fi

if grep -qE '^ Recommends: postgresql' "$INFO"; then
  fail "postgresql must be Depends, not Recommends"
else
  pass "postgresql is not Recommends"
fi

if grep -qiE 'golang|gcc|g\+\+|make | git' "$INFO"; then
  fail "control should not declare build-time Depends (golang/gcc/make/git)"
else
  pass "no build-tool Depends"
fi

if [[ "$EDITION" == "professional" ]]; then
  need_grep "$INFO" "AcuGIS Report Server Professional" "Professional description"
  need_grep "$INFO" "^ Conflicts: acugis-report-server\$" "Conflicts OSS"
  need_grep "$INFO" "^ Replaces: acugis-report-server\$" "Replaces OSS"
  need_grep "$INFO" "^ Provides: acugis-report-server\$" "Provides OSS"
else
  need_grep "$INFO" "AcuGIS Report Server Open Source" "Open Source description"
  need_grep "$INFO" "^ Conflicts: acugis-report-server-professional\$" "Conflicts Professional"
  if grep -q "^ Replaces: acugis-report-server-professional" "$INFO"; then
    fail "OSS must not Replaces Professional (downgrade path)"
  else
    pass "OSS does not Replaces Professional"
  fi
fi

ROOTDIR="$WORK/data/opt/acugis-report-server"

contents_has() {
  local rel="$1"
  grep -E -q " \./${rel}$" "$CONTENTS"
}

require_file() {
  local rel="$1"
  if contents_has "$rel"; then
    pass "/$rel"
  else
    fail "missing /$rel"
  fi
}
forbid_file() {
  local rel="$1"
  if contents_has "$rel" || grep -E -q " \./${rel}(/|$)" "$CONTENTS"; then
    fail "should be absent: /$rel"
  else
    pass "absent /$rel"
  fi
}

require_file "opt/acugis-report-server/bin/jrs-server"
require_file "opt/acugis-report-server/bin/jrs-run-schedule"
require_file "opt/acugis-report-server/scripts/run_schedule.php"
require_file "opt/acugis-report-server/scripts/native-setup.sh"
require_file "opt/acugis-report-server/scripts/configure-instance.sh"
require_file "opt/acugis-report-server/lib/bootstrap.php"
# lib/map_report.php is Professional-only. The OSS export excludes it, and
# bootstrap.php loads it only when the file is present.
if [[ "$EDITION" == "professional" ]]; then
  require_file "opt/acugis-report-server/lib/map_report.php"
else
  forbid_file "opt/acugis-report-server/lib/map_report.php"
fi
require_file "opt/acugis-report-server/systemd/acugis-report-server.service"
require_file "opt/acugis-report-server/systemd/acugis-tomcat.service"
require_file "lib/systemd/system/acugis-report-server.service"
require_file "lib/systemd/system/acugis-tomcat.service"
require_file "opt/acugis-report-server/database/schema.sql"
require_file "opt/acugis-report-server/tomcat/acugis-report-server.war"
# Oracle JDBC may ship in Professional. IBM Db2 is customer-supplied.
forbid_file "opt/acugis-report-server/tomcat/lib/db2jcc4.jar"
if [[ "$EDITION" == "professional" ]]; then
  require_file "opt/acugis-report-server/tomcat/lib/ojdbc11.jar"
else
  forbid_file "opt/acugis-report-server/tomcat/lib/ojdbc11.jar"
fi
require_file "opt/acugis-report-server/config/apache-acugis-report-server.conf"
require_file "opt/acugis-report-server/RELEASE"
require_file "opt/acugis-report-server/sdk/javascript/dist/acugis-report.js"
require_file "opt/acugis-report-server/sdk/javascript/dist/acugis-report.min.js"
require_file "opt/acugis-report-server/sdk/javascript/dist/acugis-report.esm.js"
require_file "opt/acugis-report-server/sdk/javascript/dist/index.d.ts"
forbid_file "opt/acugis-report-server/bin/jrs-schedule-dispatcher"
forbid_file "opt/acugis-report-server/go-report-server/go.mod"
forbid_file "opt/acugis-report-server/database/schema.oss.sql"

if grep -E './opt/acugis-report-server/bin/jrs-server$' "$CONTENTS" | grep -q '^-rwxr-xr-x'; then
  pass "jrs-server executable in deb contents"
else
  fail "jrs-server not executable in package"
fi
if grep -E './opt/acugis-report-server/bin/jrs-run-schedule$' "$CONTENTS" | grep -q '^-rwxr-xr-x'; then
  pass "jrs-run-schedule executable in deb contents"
else
  fail "jrs-run-schedule not executable in package"
fi

need_grep "$CONTENTS" "lib/systemd/system/acugis-report-server.service" "systemd unit in deb"
need_grep "$CONTENTS" "lib/systemd/system/acugis-tomcat.service" "tomcat unit in deb"

# env and first-boot credentials are created at configure time, not shipped.
if grep -q '/etc/acugis-report-server/env' "$CONTENTS"; then
  fail "must not ship /etc/acugis-report-server/env inside the .deb (upgrade-safe create-if-missing)"
else
  pass "env file not packaged (created on configure)"
fi
if grep -q 'initial-credentials' "$CONTENTS"; then
  fail "must not ship initial-credentials inside the .deb"
else
  pass "initial-credentials not packaged (created on first boot)"
fi

if grep -q "edition=${EDITION}" "$ROOTDIR/RELEASE"; then
  pass "RELEASE edition=$EDITION"
else
  fail "RELEASE edition mismatch"
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
    forbid_file "opt/acugis-report-server/$rel"
  done
  for rel in "${ARCGIS_SQL[@]}"; do
    require_file "opt/acugis-report-server/$rel"
  done
  forbid_file "opt/acugis-report-server/database/migrations/professional"
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
    require_file "opt/acugis-report-server/$rel"
  done
  for rel in "${ARCGIS_SQL[@]}"; do
    require_file "opt/acugis-report-server/$rel"
  done
fi

if [[ -f "$ROOTDIR/scripts/native-setup.sh" ]] && grep -q 'install_schedule_sudoers' "$ROOTDIR/scripts/native-setup.sh"; then
  pass "native-setup installs schedule sudoers"
else
  fail "native-setup missing install_schedule_sudoers"
fi

if [[ -f "$ROOTDIR/scripts/native-setup.sh" ]] && grep -q 'ensure_embed_token_secret' "$ROOTDIR/scripts/native-setup.sh"; then
  pass "native-setup generates embed token secret"
else
  fail "native-setup missing ensure_embed_token_secret"
fi

if grep -q 'embedReport' "$ROOTDIR/sdk/javascript/dist/acugis-report.js" \
  && grep -q 'embedReport' "$ROOTDIR/sdk/javascript/dist/acugis-report.min.js"; then
  pass "packaged SDK bundles export embedReport"
else
  fail "packaged SDK bundles missing embedReport"
fi
if grep -q 'node_modules' "$CONTENTS"; then
  fail "deb contains node_modules"
else
  pass "deb has no node_modules"
fi

for script in preinst postinst prerm postrm; do
  script_file="$WORK/control-files/$script"
  if [[ ! -f "$script_file" ]]; then
    fail "missing maintainer script $script"
    continue
  fi
  if LC_ALL=C grep -q $'\r' "$script_file"; then
    fail "$script contains CRLF line endings"
  else
    pass "$script has Unix LF line endings"
  fi
  shebang="$(head -n 1 "$script_file")"
  if [[ "$shebang" == "#!/bin/bash" ]]; then
    pass "$script shebang is #!/bin/bash"
  else
    fail "$script shebang is not #!/bin/bash"
  fi
done

if [[ "$FAIL" -ne 0 ]]; then
  echo "[verify-deb-$EDITION] FAILED" >&2
  exit 1
fi
echo "[verify-deb-$EDITION] PASSED $(basename "$DEB_FILE")"
cat "$INFO"
