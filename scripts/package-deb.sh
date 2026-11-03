#!/usr/bin/env bash
# Build a Debian .deb from the same edition staging used for native tarballs.
# Usage: scripts/package-deb.sh <oss> [VERSION]
#
# On Linux amd64, always compiles Go binaries from the current source tree
# (host Go + gcc) and packs with host dpkg-deb. Does not use Docker, cached
# go-builder images, existing dist/*.tar.gz, or previously compiled binaries.
#
# Docker remains optional for Windows/cross-platform builds
# (PACKAGE_USE_DOCKER=1, or when the host is not Linux amd64).
#
# Optional:
#   DEB_ARCH=amd64          (arm64 can be set later without redesign)
#   DEB_REVISION=1          → Debian version 1.0.0-1 (default: VERSION only)
#   PACKAGE_USE_DOCKER=1    force Docker go-builder + (if needed) dpkg-deb
#   DIST_DIR, STAGE_DIR     same as package-release.sh

set -euo pipefail

EDITION="${1:-}"
case "$EDITION" in
  oss) ;;
  *)
    echo "Usage: $0 <oss> [VERSION]" >&2
    exit 2
    ;;
esac

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
  DEB_SUMMARY="AcuGIS Report Server Professional"
  DEB_LONG="AcuGIS Report Server Professional - precompiled native Linux package
 including Jasper reporting, scheduling, dashboards, map reports, and
 enterprise authentication modules."
else
  DEB_PACKAGE="acugis-report-server"
  DEB_SUMMARY="AcuGIS Report Server Open Source"
  DEB_LONG="AcuGIS Report Server Open Source - precompiled native Linux package
 including Jasper reporting, scheduling, email, JDBC datasources, report
 embed, and PostGIS/MySQL Spatial/GeoServer embedded maps."
fi

DIST_DIR="${DIST_DIR:-$ROOT/dist}"
STAGE="${STAGE_DIR:-$DIST_DIR/stage-$EDITION}"
STAGE_NAME="acugis-report-server"
TARBALL="$DIST_DIR/${STAGE_NAME}-${EDITION}-${VERSION}-linux-${DEB_ARCH}.tar.gz"
DEB_FILE="$DIST_DIR/${DEB_PACKAGE}_${DEB_VERSION}_${DEB_ARCH}.deb"
DEB_TREE="${DEB_TREE:-$DIST_DIR/deb-tree-$EDITION}"
MAINTAINER="${DEB_MAINTAINER:-AcuGIS <info@acugis.com>}"
HOMEPAGE="${DEB_HOMEPAGE:-https://acugis.com/}"

log() { echo "[deb-$EDITION] $*"; }
die() { echo "[deb-$EDITION] ERROR: $*" >&2; exit 1; }

# shellcheck source=source-revision.sh
source "$ROOT/scripts/source-revision.sh"
# Git HEAD when this tree is a checkout. Otherwise SOURCE_REVISION from the OSS export.
REVISION="$(resolve_source_revision "$ROOT")" || die "cannot determine source revision"
export REVISION

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

is_linux() { [[ "$(uname -s 2>/dev/null)" == "Linux" ]]; }

is_linux_amd64() {
  is_linux || return 1
  case "$(uname -m 2>/dev/null)" in
    x86_64|amd64) return 0 ;;
    *) return 1 ;;
  esac
}

# ---------------------------------------------------------------------------
# Obtain the edition-specific staged tree (same contents as the tarball).
# ---------------------------------------------------------------------------
overlay_native_helpers() {
  # Shared installer helpers (not edition-specific application files).
  mkdir -p "$STAGE/scripts"
  cp -a "$ROOT/scripts/native-setup.sh" "$STAGE/scripts/native-setup.sh"
  cp -a "$ROOT/scripts/configure-instance.sh" "$STAGE/scripts/configure-instance.sh"
  cp -a "$ROOT/installer.sh" "$STAGE/install.sh"
  chmod +x "$STAGE/scripts/native-setup.sh" "$STAGE/scripts/configure-instance.sh" "$STAGE/install.sh"
}

obtain_stage() {
  # Always compile from the current tree. Do not reuse tarballs, stages, or
  # previously compiled binaries.
  log "Staging from current source tree (edition=$EDITION revision=$REVISION)"
  SKIP_STAGE=0 SKIP_ARCHIVE=1 FORCE_STAGE=1 STAGE_DIR="$STAGE" DIST_DIR="$DIST_DIR" \
    bash "$ROOT/scripts/package-release.sh" "$EDITION" "$VERSION"
  [[ -f "$STAGE/RELEASE" ]] || die "staging failed: $STAGE/RELEASE missing"
  [[ -f "$STAGE/bin/jrs-server" && -s "$STAGE/bin/jrs-server" ]] || die "staging missing freshly built jrs-server"
  overlay_native_helpers
}

write_control() {
  local control="$1"
  local size_kb
  size_kb="$(du -sk "$DEB_TREE" | awk '{print $1}')"
  # Subtract DEBIAN/ from Installed-Size (approx).
  local debian_kb
  debian_kb="$(du -sk "$DEB_TREE/DEBIAN" | awk '{print $1}')"
  size_kb=$((size_kb - debian_kb))
  if [[ "$size_kb" -lt 1 ]]; then size_kb=1; fi

  local relations=""
  if [[ "$EDITION" == "professional" ]]; then
    relations="Conflicts: acugis-report-server
Replaces: acugis-report-server
Provides: acugis-report-server"
  else
    relations="Conflicts: acugis-report-server-professional"
  fi

  cat > "$control" <<EOF
Package: ${DEB_PACKAGE}
Version: ${DEB_VERSION}
Section: web
Priority: optional
Architecture: ${DEB_ARCH}
Maintainer: ${MAINTAINER}
Homepage: ${HOMEPAGE}
Installed-Size: ${size_kb}
Depends: openjdk-17-jre-headless | java17-runtime-headless, postgresql, postgresql-client, apache2, php-cli, php-pgsql, php-xml, php-curl, php-gd, curl, ca-certificates, openssl, tar, adduser, sudo
${relations}
Description: ${DEB_SUMMARY}
 ${DEB_LONG}
 First boot generates admin and database passwords (see
 /etc/acugis-report-server/initial-credentials). Tomcat 10.1 is downloaded
 to /opt/tomcat on first configure when absent. Apache reverse-proxies
 external HTTP to 127.0.0.1:9088.
EOF
}

subst_script() {
  local src="$1" dest="$2"
  sed \
    -e "s/@EDITION@/${EDITION}/g" \
    -e "s/@PACKAGE@/${DEB_PACKAGE}/g" \
    -e "s/@VERSION@/${DEB_VERSION}/g" \
    "$src" > "$dest"
  chmod 0755 "$dest"
}

# dpkg executes maintainer scripts by their shebang. A trailing CR makes the
# kernel look up /bin/bash\r and fails with "No such file or directory".
reject_crlf_maintainer_scripts() {
  local dir="$1"
  local name file shebang
  for name in preinst postinst prerm postrm; do
    file="${dir}/${name}"
    [[ -f "$file" ]] || die "missing Debian maintainer script: $file"
    if LC_ALL=C grep -q $'\r' "$file"; then
      die "Debian maintainer script contains CRLF line endings: $file"
    fi
    shebang="$(head -n 1 "$file")"
    if [[ "$shebang" != "#!/bin/bash" ]]; then
      die "Debian maintainer script shebang must be #!/bin/bash: $file"
    fi
  done
}

write_copyright() {
  local dest="$1"
  # Edition-aware. The Open Source package is Apache-2.0. Professional stays proprietary.
  if [[ "$EDITION" == "oss" ]]; then
    cat > "$dest" <<EOF
Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/
Upstream-Name: AcuGIS Report Server
Upstream-Contact: AcuGIS <${HOMEPAGE}>
Source: ${HOMEPAGE}
Homepage: ${HOMEPAGE}

Files: *
Copyright: 2026 AcuGIS
License: Apache-2.0

License: Apache-2.0
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at
 .
 https://www.apache.org/licenses/LICENSE-2.0
 .
 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
 .
 On Debian systems, the complete text of the Apache License Version 2.0 can be
 found in "/usr/share/common-licenses/Apache-2.0".
EOF
    return 0
  fi
  cat > "$dest" <<EOF
Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/
Upstream-Name: AcuGIS Report Server
Upstream-Contact: AcuGIS <${HOMEPAGE}>
Source: ${HOMEPAGE}
Homepage: ${HOMEPAGE}

Files: *
Copyright: AcuGIS
License: Proprietary
 All rights reserved. See ${HOMEPAGE}
EOF
}

write_changelog() {
  local dest="$1"
  local when
  when="$(date -u +"%a, %d %b %Y %H:%M:%S +0000" 2>/dev/null || echo "Sun, 16 Aug 2026 00:00:00 +0000")"
  cat > "$dest" <<EOF
${DEB_PACKAGE} (${DEB_VERSION}) stable; urgency=medium

  * ${DEB_SUMMARY} ${VERSION}

 -- ${MAINTAINER}  ${when}
EOF
}

write_readme_debian() {
  local dest="$1"
  cat > "$dest" <<EOF
AcuGIS Report Server for Debian/Ubuntu
======================================

This package installs into the same paths as the native tarball installer.

apt install
-----------

  sudo apt-get -y install ${DEB_PACKAGE}
  (or: sudo apt-get -y install ./${DEB_PACKAGE}_${DEB_VERSION}_${DEB_ARCH}.deb)

A local PostgreSQL server is a package dependency. On first configure the
postinst generates JRS_DB_PASS and JRS_ADMIN_PASSWORD when unset, creates
the application database, seeds admin, and starts the services. Credentials
are written to /etc/acugis-report-server/env and (when generated) to
/etc/acugis-report-server/initial-credentials (mode 600). Upgrades never
replace the env file or rotate those secrets.

To use an existing/remote database, write JRS_DB_* (including JRS_DB_PASS)
into /etc/acugis-report-server/env before install, or after install run:

  sudo /opt/acugis-report-server/scripts/configure-instance.sh

AcuGIS Agent may write env first; existing passwords are kept.

Apache reverse proxy
--------------------

apache2 is a package dependency. On configure the native helper enables
proxy, proxy_http, headers, and rewrite, installs the shipped snippet
to /etc/apache2/conf-available/acugis-report-server.conf (if that file
does not already exist), enables it, and reloads Apache. Report Server
remains bound to 127.0.0.1:9088. External HTTP access is through Apache.

Upgrades do not overwrite an administrator-customized Apache snippet.

Tomcat
------

Apache Tomcat ${TOMCAT_VERSION:-10.1.57} is downloaded to /opt/tomcat on first
configure when catalina.sh is absent. Existing Tomcat trees, context.xml,
acugis.properties, and /opt/tomcat/acugis/ data are preserved.

Scheduled reports
-----------------

Configure writes /etc/sudoers.d/acugis-report-server so the service user can
install dynamic jrs-schedule-* systemd timers via sudo -n (ln, rm, systemctl).
jrs-server does not run as root and /etc/systemd/system is not world-writable.
JRS_JASPER_SYSTEMD_USE_SUDO=true is set in env when missing.

Remove vs purge
---------------

  apt remove  — removes packaged files; keeps env, reports, uploads,
                licenses, schedule data, Tomcat AcuGIS data, and the database.
  apt purge   — same data retention. Purge does not drop the PostgreSQL
                database or delete reports/uploads/licenses.

OSS → Professional
------------------

  apt install acugis-report-server-professional

replaces acugis-report-server while preserving configuration and data.
Professional → Open Source is refused.

lib/map_report.php
------------------

Professional packages include lib/map_report.php. Open Source packages omit
it: the Community export excludes the file, and lib/bootstrap.php loads it
only when it is present. Additional Professional assets and migrations remain
omitted from Open Source.
EOF
}

ensure_tarball() {
  # Always stage from the current tree. A leftover dist tarball would omit
  # newly built SDK assets and the current native-setup.sh.
  log "Staging archive from the current source tree"
  rm -f "$TARBALL" "${TARBALL}.sha256"
  SKIP_STAGE=0 SKIP_ARCHIVE=0 STAGE_DIR="$STAGE" DIST_DIR="$DIST_DIR" \
    bash "$ROOT/scripts/package-release.sh" "$EDITION" "$VERSION"
  [[ -f "$TARBALL" ]] || die "tarball not created: $TARBALL"
}

prepare_extras() {
  local extras="$1"
  rm -rf "$extras"
  mkdir -p "$extras/DEBIAN" "$extras/doc" "$extras/scripts" "$extras/systemd"
  reject_crlf_maintainer_scripts "$ROOT/packaging/debian"
  subst_script "$ROOT/packaging/debian/preinst" "$extras/DEBIAN/preinst"
  subst_script "$ROOT/packaging/debian/postinst" "$extras/DEBIAN/postinst"
  subst_script "$ROOT/packaging/debian/prerm" "$extras/DEBIAN/prerm"
  subst_script "$ROOT/packaging/debian/postrm" "$extras/DEBIAN/postrm"
  reject_crlf_maintainer_scripts "$extras/DEBIAN"
  write_copyright "$extras/doc/copyright"
  write_changelog "$extras/doc/changelog.Debian"
  write_readme_debian "$extras/doc/README.Debian"
  cp -a "$ROOT/scripts/native-setup.sh" "$extras/scripts/native-setup.sh"
  cp -a "$ROOT/scripts/configure-instance.sh" "$extras/scripts/configure-instance.sh"
  cp -a "$ROOT/installer.sh" "$extras/install.sh"
  cp -a "$ROOT/systemd/acugis-report-server.service" "$extras/systemd/"
  cp -a "$ROOT/systemd/acugis-tomcat.service" "$extras/systemd/"
  local saved_tree="$DEB_TREE"
  DEB_TREE="$extras"
  write_control "$extras/DEBIAN/control"
  DEB_TREE="$saved_tree"
}

assemble_tree() {
  log "Assembling Debian tree $DEB_TREE"
  if [[ -d "$DEB_TREE" ]]; then
    local old="${DEB_TREE}.old.$$"
    mv "$DEB_TREE" "$old"
    rm -rf "$old" || true
  fi
  mkdir -p \
    "$DEB_TREE/DEBIAN" \
    "$DEB_TREE/opt/acugis-report-server" \
    "$DEB_TREE/lib/systemd/system" \
    "$DEB_TREE/usr/share/doc/${DEB_PACKAGE}"

  # tar copy is reliable on Windows/MSYS where cp -a into an existing dest fails.
  tar -C "$STAGE" -cf - . | tar -C "$DEB_TREE/opt/acugis-report-server" -xf -

  # Overlay current maintainer helpers so a stale tarball still ships this tree's scripts.
  cp -a "$ROOT/scripts/native-setup.sh" "$DEB_TREE/opt/acugis-report-server/scripts/"
  cp -a "$ROOT/scripts/configure-instance.sh" "$DEB_TREE/opt/acugis-report-server/scripts/"
  cp -a "$ROOT/installer.sh" "$DEB_TREE/opt/acugis-report-server/install.sh"

  # systemd units: Debian ships them in /lib/systemd/system (same unit names/paths).
  cp -a "$STAGE/systemd/acugis-report-server.service" "$DEB_TREE/lib/systemd/system/"
  cp -a "$STAGE/systemd/acugis-tomcat.service" "$DEB_TREE/lib/systemd/system/"

  chmod 0755 "$DEB_TREE/opt/acugis-report-server/bin/jrs-server" \
    "$DEB_TREE/opt/acugis-report-server/bin/jrs-run-schedule" \
    "$DEB_TREE/opt/acugis-report-server/install.sh" \
    "$DEB_TREE/opt/acugis-report-server/scripts/native-setup.sh" \
    "$DEB_TREE/opt/acugis-report-server/scripts/configure-instance.sh"

  write_copyright "$DEB_TREE/usr/share/doc/${DEB_PACKAGE}/copyright"
  write_changelog "$DEB_TREE/usr/share/doc/${DEB_PACKAGE}/changelog.Debian"
  write_readme_debian "$DEB_TREE/usr/share/doc/${DEB_PACKAGE}/README.Debian"
  local cl="$DEB_TREE/usr/share/doc/${DEB_PACKAGE}/changelog.Debian"
  if command -v gzip >/dev/null 2>&1; then
    gzip -9n "$cl" 2>/dev/null || gzip -9 "$cl"
  elif command -v python >/dev/null 2>&1; then
    python -c "import gzip,shutil,sys; shutil.copyfileobj(open(sys.argv[1],'rb'), gzip.open(sys.argv[1]+'.gz','wb',9));" "$cl"
    rm -f "$cl"
  fi

  reject_crlf_maintainer_scripts "$ROOT/packaging/debian"
  subst_script "$ROOT/packaging/debian/preinst" "$DEB_TREE/DEBIAN/preinst"
  subst_script "$ROOT/packaging/debian/postinst" "$DEB_TREE/DEBIAN/postinst"
  subst_script "$ROOT/packaging/debian/prerm" "$DEB_TREE/DEBIAN/prerm"
  subst_script "$ROOT/packaging/debian/postrm" "$DEB_TREE/DEBIAN/postrm"
  reject_crlf_maintainer_scripts "$DEB_TREE/DEBIAN"

  # No conffiles: env and Apache snippets are created if missing and never
  # silently replaced (avoids dpkg conffile prompts in noninteractive apt).

  if command -v find >/dev/null 2>&1 && command -v md5sum >/dev/null 2>&1; then
    (
      cd "$DEB_TREE"
      find . -type f ! -path './DEBIAN/*' -print0 | sort -z | xargs -0 md5sum
    ) > "$DEB_TREE/DEBIAN/md5sums" || true
  fi

  write_control "$DEB_TREE/DEBIAN/control"
  chmod 0644 "$DEB_TREE/DEBIAN/control"
  [[ -f "$DEB_TREE/DEBIAN/md5sums" ]] && chmod 0644 "$DEB_TREE/DEBIAN/md5sums"
}

build_deb_docker() {
  ensure_tarball
  local extras name hosttar hostextras
  extras="${DIST_DIR}/deb-extras-${EDITION}-$$"
  name="$(basename "$DEB_FILE")"
  prepare_extras "$extras"
  hosttar="$(host_path "$TARBALL")"
  hostextras="$(host_path "$extras")"
  log "Building $DEB_FILE in Docker from $(basename "$TARBALL")"
  rm -f "$DEB_FILE"
  local cid hostdeb
  hostdeb="$(host_path "$DEB_FILE")"
  cid="$(MSYS_NO_PATHCONV=1 docker create ubuntu:24.04 sleep 1800)"
  MSYS_NO_PATHCONV=1 docker start "$cid" >/dev/null
  MSYS_NO_PATHCONV=1 docker exec "$cid" mkdir -p /in /extras
  MSYS_NO_PATHCONV=1 docker cp "$hosttar" "${cid}:/in/release.tar.gz"
  MSYS_NO_PATHCONV=1 docker cp "${hostextras}/." "${cid}:/extras/"
  MSYS_NO_PATHCONV=1 docker exec -e DEB_PACKAGE="$DEB_PACKAGE" "$cid" bash -c "set -euo pipefail
      mkdir -p /tmp/build/opt /tmp/build/DEBIAN /tmp/build/lib/systemd/system /tmp/build/usr/share/doc/\${DEB_PACKAGE}
      tar -xzf /in/release.tar.gz -C /tmp
      test -d /tmp/acugis-report-server
      mv /tmp/acugis-report-server /tmp/build/opt/acugis-report-server
      cp -a /extras/scripts/native-setup.sh /tmp/build/opt/acugis-report-server/scripts/
      cp -a /extras/scripts/configure-instance.sh /tmp/build/opt/acugis-report-server/scripts/
      cp -a /extras/install.sh /tmp/build/opt/acugis-report-server/install.sh
      cp -a /extras/systemd/acugis-report-server.service /tmp/build/lib/systemd/system/
      cp -a /extras/systemd/acugis-tomcat.service /tmp/build/lib/systemd/system/
      cp -a /extras/systemd/*.service /tmp/build/opt/acugis-report-server/systemd/
      mkdir -p /tmp/build/usr/share/doc/\${DEB_PACKAGE}
      cp -a /extras/doc/* /tmp/build/usr/share/doc/\${DEB_PACKAGE}/
      gzip -9n /tmp/build/usr/share/doc/\${DEB_PACKAGE}/changelog.Debian 2>/dev/null || gzip -9 /tmp/build/usr/share/doc/\${DEB_PACKAGE}/changelog.Debian
      cp -a /extras/DEBIAN/. /tmp/build/DEBIAN/
      chmod 0755 /tmp/build/DEBIAN
      chmod 0755 /tmp/build/DEBIAN/preinst /tmp/build/DEBIAN/postinst /tmp/build/DEBIAN/prerm /tmp/build/DEBIAN/postrm
      chmod 0755 /tmp/build/opt/acugis-report-server/bin/jrs-server /tmp/build/opt/acugis-report-server/bin/jrs-run-schedule
      chmod 0755 /tmp/build/opt/acugis-report-server/install.sh /tmp/build/opt/acugis-report-server/scripts/*.sh
      size_kb=\$(du -sk /tmp/build | awk '{print \$1}')
      debian_kb=\$(du -sk /tmp/build/DEBIAN | awk '{print \$1}')
      size_kb=\$((size_kb - debian_kb))
      sed -i \"s/^Installed-Size: .*/Installed-Size: \${size_kb}/\" /tmp/build/DEBIAN/control
      (cd /tmp/build && find . -type f ! -path './DEBIAN/*' -print0 | sort -z | xargs -0 md5sum) > /tmp/build/DEBIAN/md5sums
      chmod 0644 /tmp/build/DEBIAN/control /tmp/build/DEBIAN/md5sums
      test -f /tmp/build/opt/acugis-report-server/bin/jrs-server
      dpkg-deb --root-owner-group --build /tmp/build /tmp/pkg.deb
      test \$(stat -c%s /tmp/pkg.deb) -gt 1000000" \
    || { MSYS_NO_PATHCONV=1 docker rm -f "$cid" >/dev/null 2>&1 || true; rm -rf "$extras"; die "dpkg-deb inside container failed"; }
  MSYS_NO_PATHCONV=1 docker cp "${cid}:/tmp/pkg.deb" "$hostdeb"
  MSYS_NO_PATHCONV=1 docker rm -f "$cid" >/dev/null
  rm -rf "$extras"
}

build_deb() {
  mkdir -p "$DIST_DIR"
  rm -f "$DEB_FILE"
  if command -v dpkg-deb >/dev/null 2>&1; then
    reject_crlf_maintainer_scripts "$DEB_TREE/DEBIAN"
    log "Building $DEB_FILE with host dpkg-deb"
    dpkg-deb --root-owner-group --build "$DEB_TREE" "$DEB_FILE"
  elif is_linux_amd64; then
    die "dpkg-deb required on Linux (install dpkg-dev). Docker is not used for native Linux DEB builds"
  elif have_docker; then
    build_deb_docker
  else
    die "dpkg-deb required (install dpkg-dev). Docker is optional for Windows/cross-platform builds"
  fi
  [[ -e "$DEB_FILE" ]] || die "deb not created: $DEB_FILE"
  local sz
  sz="$(wc -c < "$DEB_FILE" | tr -d '[:space:]')"
  if [[ -z "$sz" || "$sz" -lt 1000000 ]]; then
    die "deb is too small (${sz:-0} bytes); payload was not packed: $DEB_FILE"
  fi
  log "Wrote $DEB_FILE (${sz} bytes)"
}

sha256_file() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
  else
    openssl dgst -sha256 "$file" | awk '{print $NF}'
  fi
}

print_build_summary() {
  local bin="$STAGE/bin/jrs-server"
  local cli="$STAGE/bin/jrs-run-schedule"
  local gover="n/a (compiled via Docker go-builder)"
  if is_linux_amd64 && command -v go >/dev/null 2>&1 && [[ "${PACKAGE_USE_DOCKER:-0}" != "1" ]]; then
    gover="$(go version 2>/dev/null || echo unknown)"
  fi
  log "Build summary"
  log "  source revision: $REVISION"
  log "  edition: $EDITION"
  log "  go version: $gover"
  log "  GOOS/GOARCH: linux/${DEB_ARCH}"
  if [[ -f "$bin" ]]; then
    log "  jrs-server SHA256: $(sha256_file "$bin")"
  fi
  if [[ -f "$cli" ]]; then
    log "  jrs-run-schedule SHA256: $(sha256_file "$cli")"
  fi
  log "  DEB: $DEB_FILE"
}

main() {
  log "Debian package $DEB_PACKAGE $DEB_VERSION ($DEB_ARCH) from $EDITION staging"
  if is_linux_amd64; then
    command -v dpkg-deb >/dev/null 2>&1 || die "dpkg-deb required on Linux (install dpkg-dev). Docker is not used for native Linux DEB builds"
    obtain_stage
    assemble_tree
    build_deb
  elif command -v dpkg-deb >/dev/null 2>&1; then
    obtain_stage
    assemble_tree
    build_deb
  elif have_docker; then
    log "Host is not Linux amd64; using optional Docker dpkg-deb path"
    build_deb
  else
    die "Linux amd64: install Go, gcc, and dpkg-dev. Other platforms: Docker"
  fi
  local sum name
  sum="$(sha256_file "$DEB_FILE")"
  name="$(basename "$DEB_FILE")"
  echo "$sum  $name" > "${DEB_FILE}.sha256"
  log "SHA-256 $sum"
  print_build_summary
  log "Done: $DEB_FILE"
}

main
