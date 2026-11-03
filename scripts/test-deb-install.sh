#!/usr/bin/env bash
# Installation tests for finished .deb packages inside Ubuntu Docker.
# Usage: scripts/test-deb-install.sh [VERSION]
#
# Tests: fresh OSS, OSS→OSS, remove/purge, OSS→Professional via apt
#        (no manual remove), fresh Professional, Professional→Professional.
# Does not test Professional → OSS (unsupported).
#
# Same-edition upgrade/remove tests use dpkg --force-depends so OpenJDK and
# PostgreSQL are not pulled (JRS_SKIP_TOMCAT=1 JRS_SKIP_DB=1). openssl is
# installed so first-boot secret generation still runs. The OSS→Professional
# replacement uses apt-get install of the Professional .deb so
# Conflicts/Replaces is exercised.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-${VERSION:-1.0.0}}"
DEB_ARCH="${DEB_ARCH:-amd64}"
DIST_DIR="${DIST_DIR:-$ROOT/dist}"
IMAGE="${TEST_DEB_IMAGE:-ubuntu:24.04}"

if [[ -z "${NEXT_VERSION:-}" ]]; then
  if [[ "$VERSION" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+) ]]; then
    NEXT_VERSION="${BASH_REMATCH[1]}.${BASH_REMATCH[2]}.$((BASH_REMATCH[3] + 1))"
  else
    NEXT_VERSION="${VERSION}.1"
  fi
fi

OSS_DEB="$DIST_DIR/acugis-report-server_${VERSION}_${DEB_ARCH}.deb"
PRO_DEB="$DIST_DIR/acugis-report-server-professional_${VERSION}_${DEB_ARCH}.deb"
OSS_NEXT="$DIST_DIR/acugis-report-server_${NEXT_VERSION}_${DEB_ARCH}.deb"
PRO_NEXT="$DIST_DIR/acugis-report-server-professional_${NEXT_VERSION}_${DEB_ARCH}.deb"
WORKDIR="${DIST_DIR}/deb-test-scripts"

log() { echo "[test-deb] $*"; }
die() { echo "[test-deb] ERROR: $*" >&2; exit 1; }

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

have_docker || die "Docker is required for installation tests"
[[ -f "$OSS_DEB" ]] || die "missing $OSS_DEB — run: make deb-oss VERSION=$VERSION"
[[ -f "$PRO_DEB" ]] || die "missing $PRO_DEB — run: make deb-professional VERSION=$VERSION"

# docker cp (not bind-mount) so Windows/Docker Desktop cannot emit empty packages.
build_next() {
  local src="$1" dest="$2" from_ver="$3" to_ver="$4"
  if [[ -f "$dest" ]]; then
    log "Reusing $(basename "$dest")"
    return 0
  fi
  log "Building upgrade-test package $(basename "$dest") from $(basename "$src")"
  local cid
  cid="$(MSYS_NO_PATHCONV=1 docker create "$IMAGE" sleep 1800)"
  MSYS_NO_PATHCONV=1 docker start "$cid" >/dev/null
  MSYS_NO_PATHCONV=1 docker cp "$(host_path "$src")" "${cid}:/in.deb"
  MSYS_NO_PATHCONV=1 docker exec -e FROM_VER="$from_ver" -e TO_VER="$to_ver" "$cid" bash -c '
    set -euo pipefail
    dpkg-deb -R /in.deb /tmp/pkg
    chmod 0755 /tmp/pkg/DEBIAN
    sed -i "s/^Version: ${FROM_VER}\$/Version: ${TO_VER}/" /tmp/pkg/DEBIAN/control
    if [[ -f /tmp/pkg/opt/acugis-report-server/RELEASE ]]; then
      sed -i "s/^version=${FROM_VER}\$/version=${TO_VER}/" /tmp/pkg/opt/acugis-report-server/RELEASE || true
    fi
    dpkg-deb --root-owner-group --build /tmp/pkg /tmp/out.deb'
  MSYS_NO_PATHCONV=1 docker cp "${cid}:/tmp/out.deb" "$(host_path "$dest")"
  MSYS_NO_PATHCONV=1 docker rm -f "$cid" >/dev/null
  [[ -f "$dest" ]] || die "failed to write $dest"
}

run_in_container() {
  local script="$1"
  shift
  local cid
  cid="$(MSYS_NO_PATHCONV=1 docker create \
    -e DEBIAN_FRONTEND=noninteractive \
    -e JRS_SKIP_TOMCAT=1 \
    -e JRS_SKIP_DB=1 \
    -e JRS_SKIP_APACHE=1 \
    -e JRS_SKIP_SYSTEMD=1 \
    "$IMAGE" sleep 3600)"
  MSYS_NO_PATHCONV=1 docker start "$cid" >/dev/null
  MSYS_NO_PATHCONV=1 docker exec "$cid" mkdir -p /debs
  local f
  for f in "$@"; do
    log "Copying $(basename "$f") into container"
    MSYS_NO_PATHCONV=1 docker cp "$(host_path "$f")" "${cid}:/debs/$(basename "$f")"
  done
  MSYS_NO_PATHCONV=1 docker cp "$(host_path "$script")" "${cid}:/tmp/test.sh"
  if ! MSYS_NO_PATHCONV=1 docker exec "$cid" bash /tmp/test.sh; then
    MSYS_NO_PATHCONV=1 docker rm -f "$cid" >/dev/null 2>&1 || true
    die "container test failed: $(basename "$script")"
  fi
  MSYS_NO_PATHCONV=1 docker rm -f "$cid" >/dev/null
}

build_next "$OSS_DEB" "$OSS_NEXT" "$VERSION" "$NEXT_VERSION"
build_next "$PRO_DEB" "$PRO_NEXT" "$VERSION" "$NEXT_VERSION"

mkdir -p "$WORKDIR"
rm -f "$DIST_DIR"/test-oss-lifecycle.sh "$DIST_DIR"/test-pro-lifecycle.sh

cat > "$WORKDIR/oss-lifecycle.sh" <<EOF
#!/bin/bash
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
export JRS_SKIP_TOMCAT=1 JRS_SKIP_DB=1 JRS_SKIP_APACHE=1 JRS_SKIP_SYSTEMD=1
apt-get update -qq
apt-get install -y -qq adduser ca-certificates curl tar openssl sudo >/dev/null
dpkg-deb -f /debs/$(basename "$OSS_DEB") Depends | grep -q 'php-curl'
dpkg-deb -f /debs/$(basename "$OSS_DEB") Depends | grep -q 'php-gd'
dpkg --force-depends -i /debs/$(basename "$OSS_DEB")
dpkg -s acugis-report-server | grep -q 'Status: install ok installed'
test -x /opt/acugis-report-server/bin/jrs-server
test -f /lib/systemd/system/acugis-report-server.service
test -f /etc/acugis-report-server/env
grep -qE '^JRS_DB_PASS=.+' /etc/acugis-report-server/env
grep -qE '^JRS_ADMIN_PASSWORD=.+' /etc/acugis-report-server/env
grep -qE '^ACUGIS_EMBED_TOKEN_SECRET=[0-9a-fA-F]{64}$' /etc/acugis-report-server/env
test "\$(stat -c %a /etc/acugis-report-server/env)" = "640"
test "\$(stat -c %U /etc/acugis-report-server/env)" = "root"
test "\$(stat -c %G /etc/acugis-report-server/env)" = "www-data"
if grep -q 'ACUGIS_EMBED_TOKEN_SECRET' /etc/acugis-report-server/initial-credentials; then
  echo 'embed secret must not be copied into initial-credentials' >&2
  exit 1
fi
if [[ -f /opt/acugis-report-server/.env ]] && grep -qE '^(export[[:space:]]+)?(ACUGIS_EMBED_TOKEN_SECRET|JRS_EMBED_TOKEN_SECRET)=' /opt/acugis-report-server/.env; then
  echo 'install-tree .env contains an embed signing secret' >&2
  exit 1
fi
if [[ -f /opt/acugis-report-server/.env ]]; then
  test "\$(stat -c %a /opt/acugis-report-server/.env)" = "640"
  test "\$(stat -c %U /opt/acugis-report-server/.env)" = "root"
  test "\$(stat -c %G /opt/acugis-report-server/.env)" = "www-data"
fi
while IFS= read -r -d '' exposed; do
  if grep -qE '^(export[[:space:]]+)?(ACUGIS_EMBED_TOKEN_SECRET|JRS_EMBED_TOKEN_SECRET)=' "\$exposed"; then
    echo "world-accessible file assigns an embed signing secret: \$exposed" >&2
    exit 1
  fi
done < <(find /opt/acugis-report-server /etc/acugis-report-server -type f -perm /005 -print0)
test -f /opt/acugis-report-server/sdk/javascript/dist/acugis-report.js
test -f /opt/acugis-report-server/sdk/javascript/dist/acugis-report.min.js
test -f /opt/acugis-report-server/sdk/javascript/dist/acugis-report.esm.js
test -f /opt/acugis-report-server/sdk/javascript/dist/index.d.ts
grep -q embedReport /opt/acugis-report-server/sdk/javascript/dist/acugis-report.js
grep -q embedReport /opt/acugis-report-server/sdk/javascript/dist/acugis-report.min.js
test ! -d /opt/acugis-report-server/sdk/javascript/node_modules
grep -q '^JRS_JASPER_SYSTEMD_USE_SUDO=true' /etc/acugis-report-server/env
test -f /etc/sudoers.d/acugis-report-server
grep -qx 'www-data ALL=(root) NOPASSWD: /bin/ln' /etc/sudoers.d/acugis-report-server
grep -qx 'www-data ALL=(root) NOPASSWD: /bin/systemctl' /etc/sudoers.d/acugis-report-server
test -f /etc/acugis-report-server/initial-credentials
grep -q 'edition=oss' /opt/acugis-report-server/RELEASE
test ! -f /opt/acugis-report-server/database/migrations/009_dataset_intelligence.sql
echo OK_FRESH_OSS
echo 'JRS_DB_PASS=keep-me-secret' >> /etc/acugis-report-server/env
sed -i 's/^ACUGIS_EMBED_TOKEN_SECRET=.*/ACUGIS_EMBED_TOKEN_SECRET=keep-embed-secret/' /etc/acugis-report-server/env
mkdir -p /opt/acugis-report-server/reports/json
echo marker > /opt/acugis-report-server/reports/json/upgrade-marker.report.json
dpkg --force-depends -i /debs/$(basename "$OSS_NEXT")
dpkg -s acugis-report-server | grep -q 'Version: ${NEXT_VERSION}'
grep -q 'JRS_DB_PASS=keep-me-secret' /etc/acugis-report-server/env
grep -qx 'ACUGIS_EMBED_TOKEN_SECRET=keep-embed-secret' /etc/acugis-report-server/env
if [[ -f /opt/acugis-report-server/.env ]] && grep -qE '^(export[[:space:]]+)?(ACUGIS_EMBED_TOKEN_SECRET|JRS_EMBED_TOKEN_SECRET)=' /opt/acugis-report-server/.env; then
  echo 'upgrade left an embed signing secret in install-tree .env' >&2
  exit 1
fi
test "\$(stat -c %a /etc/acugis-report-server/env)" = "640"
test "\$(stat -c %U /etc/acugis-report-server/env)" = "root"
test "\$(stat -c %G /etc/acugis-report-server/env)" = "www-data"
test -f /opt/acugis-report-server/sdk/javascript/dist/acugis-report.js
test -f /opt/acugis-report-server/reports/json/upgrade-marker.report.json
echo OK_OSS_UPGRADE
echo keep > /opt/acugis-report-server/reports/json/keep.report.json
dpkg --remove acugis-report-server
test -f /etc/acugis-report-server/env
test -f /etc/acugis-report-server/initial-credentials
test -f /opt/acugis-report-server/reports/json/keep.report.json
test ! -x /opt/acugis-report-server/bin/jrs-server
test ! -f /etc/sudoers.d/acugis-report-server
dpkg --purge acugis-report-server || true
test -f /etc/acugis-report-server/env
test -f /etc/acugis-report-server/initial-credentials
test -f /opt/acugis-report-server/reports/json/keep.report.json
echo OK_REMOVE_PURGE
EOF

cat > "$WORKDIR/oss-to-pro.sh" <<EOF
#!/bin/bash
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
export JRS_SKIP_TOMCAT=1 JRS_SKIP_DB=1 JRS_SKIP_APACHE=1 JRS_SKIP_SYSTEMD=1
apt-get update -qq
apt-get install -y -qq adduser ca-certificates curl tar openssl sudo >/dev/null
dpkg-deb -f /debs/$(basename "$OSS_DEB") Depends | grep -q 'php-curl'
dpkg-deb -f /debs/$(basename "$OSS_DEB") Depends | grep -q 'php-gd'
dpkg-deb -f /debs/$(basename "$PRO_DEB") Depends | grep -q 'php-curl'
dpkg-deb -f /debs/$(basename "$PRO_DEB") Depends | grep -q 'php-gd'
dpkg --force-depends -i /debs/$(basename "$OSS_DEB")
dpkg -s acugis-report-server | grep -q 'Status: install ok installed'
test -f /etc/acugis-report-server/initial-credentials
echo 'JRS_DB_PASS=keep-me-secret' >> /etc/acugis-report-server/env
sed -i 's/^ACUGIS_EMBED_TOKEN_SECRET=.*/ACUGIS_EMBED_TOKEN_SECRET=keep-oss-embed-secret/' /etc/acugis-report-server/env
mkdir -p /opt/acugis-report-server/reports/json
echo marker > /opt/acugis-report-server/reports/json/upgrade-marker.report.json
# Required upgrade path: apt replaces OSS. Do not dpkg --remove first.
# -f: OSS was installed with --force-depends so unmet Depends exist until Pro replaces it.
apt-get install -y -f --no-install-recommends /debs/$(basename "$PRO_DEB")
dpkg -s acugis-report-server-professional | grep -q 'Status: install ok installed'
dpkg -s php-curl | grep -q 'Status: install ok installed'
dpkg -s php-gd | grep -q 'Status: install ok installed'
php -r 'function_exists("curl_init") or exit(1);'
php -r 'extension_loaded("gd") or exit(1);'
if dpkg -s acugis-report-server 2>/dev/null | grep -q 'Status: install ok installed'; then
  echo 'OSS package still installed after apt install of Professional' >&2
  exit 1
fi
grep -q 'edition=professional' /opt/acugis-report-server/RELEASE
grep -q 'JRS_DB_PASS=keep-me-secret' /etc/acugis-report-server/env
grep -qx 'ACUGIS_EMBED_TOKEN_SECRET=keep-oss-embed-secret' /etc/acugis-report-server/env
if [[ -f /opt/acugis-report-server/.env ]] && grep -qE '^(export[[:space:]]+)?(ACUGIS_EMBED_TOKEN_SECRET|JRS_EMBED_TOKEN_SECRET)=' /opt/acugis-report-server/.env; then
  echo 'upgrade left an embed signing secret in install-tree .env' >&2
  exit 1
fi
test "\$(stat -c %a /etc/acugis-report-server/env)" = "640"
test -f /opt/acugis-report-server/sdk/javascript/dist/acugis-report.js
if grep -q '^ACUGIS_EDITION=professional$' /etc/acugis-report-server/env; then
  echo 'Professional install must not grant entitlement via ACUGIS_EDITION' >&2
  exit 1
fi
test -f /opt/acugis-report-server/reports/json/upgrade-marker.report.json
test -f /etc/acugis-report-server/initial-credentials
test -f /opt/acugis-report-server/database/migrations/009_dataset_intelligence.sql
test -f /etc/sudoers.d/acugis-report-server
grep -q '^JRS_JASPER_SYSTEMD_USE_SUDO=true' /etc/acugis-report-server/env
echo OK_OSS_TO_PRO
EOF

cat > "$WORKDIR/pro-lifecycle.sh" <<EOF
#!/bin/bash
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
export JRS_SKIP_TOMCAT=1 JRS_SKIP_DB=1 JRS_SKIP_APACHE=1 JRS_SKIP_SYSTEMD=1
apt-get update -qq
apt-get install -y -qq adduser ca-certificates curl tar openssl sudo >/dev/null
dpkg-deb -f /debs/$(basename "$PRO_DEB") Depends | grep -q 'php-curl'
dpkg-deb -f /debs/$(basename "$PRO_DEB") Depends | grep -q 'php-gd'
dpkg --force-depends -i /debs/$(basename "$PRO_DEB")
dpkg -s acugis-report-server-professional | grep -q 'Status: install ok installed'
test -x /opt/acugis-report-server/bin/jrs-server
grep -q 'edition=professional' /opt/acugis-report-server/RELEASE
test -f /opt/acugis-report-server/database/migrations/009_dataset_intelligence.sql
grep -qE '^JRS_DB_PASS=.+' /etc/acugis-report-server/env
grep -qE '^ACUGIS_EMBED_TOKEN_SECRET=[0-9a-fA-F]{64}$' /etc/acugis-report-server/env
test "\$(stat -c %a /etc/acugis-report-server/env)" = "640"
if grep -q 'ACUGIS_EMBED_TOKEN_SECRET' /etc/acugis-report-server/initial-credentials; then
  echo 'embed secret must not be copied into initial-credentials' >&2
  exit 1
fi
if [[ -f /opt/acugis-report-server/.env ]] && grep -qE '^(export[[:space:]]+)?(ACUGIS_EMBED_TOKEN_SECRET|JRS_EMBED_TOKEN_SECRET)=' /opt/acugis-report-server/.env; then
  echo 'install-tree .env contains an embed signing secret' >&2
  exit 1
fi
if [[ -f /opt/acugis-report-server/.env ]]; then
  test "\$(stat -c %a /opt/acugis-report-server/.env)" = "640"
  test "\$(stat -c %U /opt/acugis-report-server/.env)" = "root"
  test "\$(stat -c %G /opt/acugis-report-server/.env)" = "www-data"
fi
test "\$(stat -c %U /etc/acugis-report-server/env)" = "root"
test "\$(stat -c %G /etc/acugis-report-server/env)" = "www-data"
while IFS= read -r -d '' exposed; do
  if grep -qE '^(export[[:space:]]+)?(ACUGIS_EMBED_TOKEN_SECRET|JRS_EMBED_TOKEN_SECRET)=' "\$exposed"; then
    echo "world-accessible file assigns an embed signing secret: \$exposed" >&2
    exit 1
  fi
done < <(find /opt/acugis-report-server /etc/acugis-report-server -type f -perm /005 -print0)
test -f /opt/acugis-report-server/sdk/javascript/dist/acugis-report.min.js
grep -q embedReport /opt/acugis-report-server/sdk/javascript/dist/acugis-report.min.js
test -f /etc/acugis-report-server/initial-credentials
grep -q '^JRS_JASPER_SYSTEMD_USE_SUDO=true' /etc/acugis-report-server/env
test -f /etc/sudoers.d/acugis-report-server
grep -qx 'www-data ALL=(root) NOPASSWD: /bin/ln' /etc/sudoers.d/acugis-report-server
grep -qx 'www-data ALL=(root) NOPASSWD: /bin/systemctl' /etc/sudoers.d/acugis-report-server
echo OK_FRESH_PRO
if grep -q '^ACUGIS_EDITION=professional$' /etc/acugis-report-server/env; then
  echo 'Professional install must not grant entitlement via ACUGIS_EDITION' >&2
  exit 1
fi
echo 'JRS_DB_PASS=pro-secret' >> /etc/acugis-report-server/env
sed -i 's/^ACUGIS_EMBED_TOKEN_SECRET=.*/ACUGIS_EMBED_TOKEN_SECRET=keep-pro-embed-secret/' /etc/acugis-report-server/env
mkdir -p /opt/acugis-report-server/reports/json /var/lib/acugis-report-server
echo pro-marker > /opt/acugis-report-server/reports/json/pro-upgrade.report.json
printf '%s\n' '11111111-1111-4111-8111-111111111111' > /var/lib/acugis-report-server/installation-id
printf '%s\n' 'ACL1.keep.me' > /var/lib/acugis-report-server/license.lic
dpkg --force-depends -i /debs/$(basename "$PRO_NEXT")
dpkg -s acugis-report-server-professional | grep -q 'Version: ${NEXT_VERSION}'
grep -q 'JRS_DB_PASS=pro-secret' /etc/acugis-report-server/env
grep -qx 'ACUGIS_EMBED_TOKEN_SECRET=keep-pro-embed-secret' /etc/acugis-report-server/env
if [[ -f /opt/acugis-report-server/.env ]] && grep -qE '^(export[[:space:]]+)?(ACUGIS_EMBED_TOKEN_SECRET|JRS_EMBED_TOKEN_SECRET)=' /opt/acugis-report-server/.env; then
  echo 'upgrade left an embed signing secret in install-tree .env' >&2
  exit 1
fi
test "\$(stat -c %a /etc/acugis-report-server/env)" = "640"
test "\$(stat -c %U /etc/acugis-report-server/env)" = "root"
test "\$(stat -c %G /etc/acugis-report-server/env)" = "www-data"
test -f /opt/acugis-report-server/reports/json/pro-upgrade.report.json
grep -qx '11111111-1111-4111-8111-111111111111' /var/lib/acugis-report-server/installation-id
grep -qx 'ACL1.keep.me' /var/lib/acugis-report-server/license.lic
echo OK_PRO_UPGRADE
EOF

# Drop leftover test containers from earlier runs.
MSYS_NO_PATHCONV=1 docker ps -aq --filter "name=acugis-deb" | while read -r id; do
  MSYS_NO_PATHCONV=1 docker rm -f "$id" >/dev/null 2>&1 || true
done

log "OSS fresh → upgrade → remove/purge"
run_in_container "$WORKDIR/oss-lifecycle.sh" "$OSS_DEB" "$OSS_NEXT"

log "Professional fresh → upgrade"
run_in_container "$WORKDIR/pro-lifecycle.sh" "$PRO_DEB" "$PRO_NEXT"

log "OSS → Professional via apt-get (no manual remove)"
run_in_container "$WORKDIR/oss-to-pro.sh" "$OSS_DEB" "$PRO_DEB"

rm -rf "$WORKDIR"
log "PASSED installation tests"
