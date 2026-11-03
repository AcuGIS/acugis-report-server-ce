#!/usr/bin/env bash
# Build a native Linux amd64 release archive for one compile-time edition.
# Usage: scripts/package-release.sh <oss> [VERSION]
#
# Edition is selected by the compiled artifact (go build -tags oss vs untagged).
# ACUGIS_EDITION is not used to choose which package is built.
#
# On Linux amd64, binaries are compiled with host Go from the current tree.
# Docker is used only when the host is not Linux amd64, or PACKAGE_USE_DOCKER=1.

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

ARCH="linux-amd64"
PRODUCT="AcuGIS Report Server"
STAGE_NAME="acugis-report-server"
DIST_DIR="${DIST_DIR:-$ROOT/dist}"
STAGE="${STAGE_DIR:-$DIST_DIR/stage-$EDITION}"
TARBALL="$DIST_DIR/${STAGE_NAME}-${EDITION}-${VERSION}-${ARCH}.tar.gz"
WAR="$ROOT/stand-alone-tomcat-10/dist/acugis-report-server.war"
GO_MODULE="github.com/acugis/acugis-report-server-go/internal/config.DefaultAppVersion"
LDFLAGS="-X ${GO_MODULE}=${VERSION}"
if [[ -n "${ACUGIS_LICENSE_PUBLIC_KEY_HEX:-}" ]]; then
  LDFLAGS="${LDFLAGS} -X github.com/acugis/acugis-report-server-go/internal/license.productionPublicKeyHex=${ACUGIS_LICENSE_PUBLIC_KEY_HEX}"
fi
BUILD_DATE="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
DOCKERFILE="$ROOT/docker/report-server/Dockerfile"

log() { echo "[package-$EDITION] $*"; }
die() { echo "[package-$EDITION] ERROR: $*" >&2; exit 1; }

# npm ci and the Maven WAR build write into this tree. A private checkout
# already has tracked node_modules. An OSS export has neither. Remove only
# the outputs this run created so the export does not retain
# sdk/javascript/node_modules, Maven target/, or a WAR that was not already
# in the tree. Release packaging always rebuilds the WAR before that cleanup.
BUILT_STANDALONE_WAR=0
SDK_NODE_MODULES_CREATED=0
STANDALONE_WAR_REBUILT=0
# These sources are omitted from the Open Source export. Community
# MapRenderer does not reference them, so the OSS WAR builds without them.
# On the private tree they are present and must be kept out of an OSS rebuild.
OSS_EXCLUDED_MAP_SOURCES=(
  stand-alone-tomcat-10/map-src/main/java/com/acugis/map/ArcGISAuth.java
  stand-alone-tomcat-10/map-src/main/java/com/acugis/map/ArcGISMapRenderer.java
  stand-alone-tomcat-10/map-src/main/java/com/acugis/map/QGISAuth.java
  stand-alone-tomcat-10/map-src/main/java/com/acugis/map/QGISFilter.java
  stand-alone-tomcat-10/map-src/main/java/com/acugis/map/QGISMapRenderer.java
)
restore_professional_map_sources() {
  local rel src hidden
  for rel in "${OSS_EXCLUDED_MAP_SOURCES[@]}"; do
    src="$ROOT/$rel"
    hidden="${src}.oss-packager-hidden"
    if [[ -f "$hidden" ]]; then
      mv "$hidden" "$src"
    fi
  done
}
cleanup_packaging_artifacts() {
  restore_professional_map_sources
  if [[ "${SDK_NODE_MODULES_CREATED:-0}" == "1" ]]; then
    rm -rf "$ROOT/sdk/javascript/node_modules"
  fi
  if [[ "${BUILT_STANDALONE_WAR:-0}" == "1" ]]; then
    rm -f "$WAR"
    rm -rf "$ROOT/stand-alone-tomcat-10/target"
  fi
}
trap cleanup_packaging_artifacts EXIT

# shellcheck source=source-revision.sh
source "$ROOT/scripts/source-revision.sh"
REVISION="$(resolve_source_revision "$ROOT")" || die "cannot determine source revision"
export REVISION

need_file() {
  [[ -f "$1" ]] || die "required file missing: $1"
}

# Linux DEB/tarball always ships the linux_amd64 DuckDB JDBC driver.
# The file is gitignored for other platforms; fetch from Maven when absent.
ensure_duckdb_jdbc() {
  local ver="1.5.5.1"
  local jar="duckdb_jdbc-${ver}-linux_amd64.jar"
  local dest="$ROOT/stand-alone-tomcat-10/lib/$jar"
  if [[ -f "$dest" && -s "$dest" ]]; then
    return 0
  fi
  local url="https://repo1.maven.org/maven2/org/duckdb/duckdb_jdbc/${ver}/${jar}"
  log "Downloading $jar (not in git clone)"
  mkdir -p "$(dirname "$dest")"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$dest"
  elif command -v wget >/dev/null 2>&1; then
    wget -q -O "$dest" "$url"
  else
    die "required file missing: $dest (install curl or wget to download DuckDB JDBC)"
  fi
  [[ -f "$dest" && -s "$dest" ]] || die "failed to download $url"
}

is_linux() { [[ "$(uname -s 2>/dev/null)" == "Linux" ]]; }

is_linux_amd64() {
  is_linux || return 1
  case "$(uname -m 2>/dev/null)" in
    x86_64|amd64) return 0 ;;
    *) return 1 ;;
  esac
}

have_docker() {
  command -v docker >/dev/null 2>&1 || return 1
  if command -v timeout >/dev/null 2>&1; then
    timeout 90 docker info >/dev/null 2>&1
  else
    docker info >/dev/null 2>&1
  fi
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

# Coordinate-map JRXML calls the 9-argument MapRenderer.render overload.
# Packaging must fail when the WAR's map jar does not provide that method.
verify_packaged_map_renderer() {
  local war="$1"
  local script="$ROOT/scripts/verify-map-renderer-api.sh"
  [[ -f "$script" ]] || die "required file missing: $script"
  log "Verifying MapRenderer 9-argument render() in $war"
  bash "$script" --war "$war"
}

# Release packaging always rebuilds the standalone WAR from the current
# stand-alone-tomcat-10 sources. The file under dist/ is committed, so its
# presence does not mean it matches this tree. The rebuild is the same Maven
# package used by stand-alone-tomcat-10/build.sh. clean is required so a
# previous compilation cannot leave Professional classes in target/.
# An Open Source rebuild moves the export-excluded map sources aside for that
# one Maven invocation. A second call in the same packaging run reuses the
# WAR just built.
hide_professional_map_sources() {
  local rel src
  [[ "$EDITION" == "oss" ]] || return 0
  for rel in "${OSS_EXCLUDED_MAP_SOURCES[@]}"; do
    src="$ROOT/$rel"
    if [[ -f "$src" ]]; then
      mv "$src" "${src}.oss-packager-hidden"
      log "OSS WAR rebuild excludes $rel"
    fi
  done
}
python_bin() {
  if command -v python3 >/dev/null 2>&1; then
    command -v python3
    return 0
  fi
  if command -v python >/dev/null 2>&1; then
    command -v python
    return 0
  fi
  return 1
}
verify_rebuilt_war_boundary() {
  local py
  py="$(python_bin)" || die "python3 is required to verify the rebuilt WAR"
  log "Verifying rebuilt WAR edition boundary ($EDITION)"
  "$py" - "$EDITION" "$WAR" <<'PY' || die "rebuilt WAR failed the edition boundary check"
import io, sys, zipfile
edition, path = sys.argv[1], sys.argv[2]
war = zipfile.ZipFile(path)
names = set(war.namelist())
required = [
    "WEB-INF/classes/com/acugis/reports/EmbedComponentExporter.class",
    "WEB-INF/classes/com/acugis/reports/ReportServlet.class",
    "WEB-INF/classes/com/acugis/reports/ReportService.class",
]
for entry in required:
    if entry not in names:
        sys.exit("missing " + entry)
if not any(n.startswith("WEB-INF/lib/protobuf-java-") and n.endswith(".jar") for n in names):
    sys.exit("missing protobuf-java")
if not any(n.startswith("WEB-INF/lib/mapbox-vector-tile-") and n.endswith(".jar") for n in names):
    sys.exit("missing mapbox-vector-tile")
jar_name = "WEB-INF/lib/acugis-report-map.jar"
if jar_name not in names:
    sys.exit("missing " + jar_name)
classes = set(zipfile.ZipFile(io.BytesIO(war.read(jar_name))).namelist())
for entry in (
    "com/acugis/map/MapRenderer.class",
    "com/acugis/map/BasemapTiles.class",
    "com/acugis/map/OpenFreeMapBasemap.class",
):
    if entry not in classes:
        sys.exit("missing " + entry)
professional = (
    "com/acugis/map/ArcGISAuth.class",
    "com/acugis/map/ArcGISMapRenderer.class",
    "com/acugis/map/QGISAuth.class",
    "com/acugis/map/QGISFilter.class",
    "com/acugis/map/QGISMapRenderer.class",
)
if edition == "oss":
    present = [entry for entry in professional if entry in classes]
    if present:
        sys.exit("OSS WAR contains " + ", ".join(present))
else:
    missing = [entry for entry in professional if entry not in classes]
    if missing:
        sys.exit("Professional WAR missing " + ", ".join(missing))
PY
}
ensure_standalone_war() {
  if [[ "${STANDALONE_WAR_REBUILT:-0}" == "1" && -f "$WAR" ]]; then
    return 0
  fi
  log "Rebuilding standalone WAR from current stand-alone-tomcat-10 source"
  if ! command -v mvn >/dev/null 2>&1; then
    die "mvn is required to build the Report Server WAR from source"
  fi
  if [[ ! -f "$WAR" ]]; then
    BUILT_STANDALONE_WAR=1
  fi
  hide_professional_map_sources
  (
    cd "$ROOT/stand-alone-tomcat-10"
    mvn -B -Pstandalone -DskipTests clean package
  ) || die "Maven WAR build failed"
  restore_professional_map_sources
  [[ -f "$WAR" ]] || die "Maven build did not write $WAR"
  verify_rebuilt_war_boundary
  STANDALONE_WAR_REBUILT=1
  log "Rebuilt WAR SHA-256 $(sha256_file "$WAR")"
}

copy_file() {
  local src="$1" dest="$2"
  mkdir -p "$(dirname "$dest")"
  cp -a "$src" "$dest"
}

host_path() {
  local p="$1"
  if command -v cygpath >/dev/null 2>&1; then
    cygpath -w "$p"
  else
    echo "$p"
  fi
}

# ---------------------------------------------------------------------------
# Linux amd64 binaries (CGO required for duckdb-go)
# Native host Go is required on Linux amd64. Docker is optional for
# Windows/cross-platform builds (PACKAGE_USE_DOCKER=1 forces it).
# ---------------------------------------------------------------------------
build_binaries_native() {
  local out="$1"
  command -v go >/dev/null 2>&1 || die "Go is required for native Linux amd64 builds (install golang-go)"
  if ! command -v gcc >/dev/null 2>&1 && ! command -v cc >/dev/null 2>&1; then
    die "gcc/cc is required for CGO (duckdb-go). Install build-essential"
  fi
  local tags=()
  if [[ "$EDITION" == "oss" ]]; then
    tags=(-tags oss)
  fi
  local tag_label="none (professional)"
  if [[ "$EDITION" == "oss" ]]; then
    tag_label="oss"
  fi
  local gover
  gover="$(go version 2>/dev/null || echo unknown)"
  log "Compiling from current source tree (native host Go)"
  log "  source revision: $REVISION"
  log "  edition: $EDITION"
  log "  go build tags: $tag_label"
  log "  go version: $gover"
  log "  GOOS/GOARCH: linux/amd64"
  rm -f "$out/jrs-server" "$out/jrs-run-schedule" "$out/jrs-schedule-dispatcher"
  (
    cd "$ROOT/go-report-server"
    modfile="$(go run ./scripts/saml-nofuzz-modfile.go)"
    modargs=()
    if [[ -n "$modfile" ]]; then modargs=(-modfile "$modfile" -mod=readonly); fi
    CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build "${tags[@]}" "${modargs[@]}" -ldflags "$LDFLAGS" -o "$out/jrs-server" ./cmd/jrs-server
    CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build "${tags[@]}" "${modargs[@]}" -ldflags "$LDFLAGS" -o "$out/jrs-run-schedule" ./cmd/jrs-run-schedule
  )
}

build_binaries_docker() {
  local out="$1"
  have_docker || die "Docker is not available (needed for non-Linux / PACKAGE_USE_DOCKER=1 builds)"
  local image="acugis-report-server-builder:${EDITION}-${VERSION}"
  log "Compiling Linux binaries with Docker go-builder (optional cross-build)"
  log "  source revision: $REVISION"
  log "  edition: $EDITION"
  log "  GOOS/GOARCH: linux/amd64"
  MSYS_NO_PATHCONV=1 docker build \
    -f "$(host_path "$DOCKERFILE")" \
    --target go-builder \
    --build-arg "BUILD_EDITION=$EDITION" \
    --build-arg "VERSION=$VERSION" \
    --build-arg "SOURCE_REVISION=$REVISION" \
    -t "$image" \
    "$(host_path "$ROOT")"
  local cid
  cid="$(docker create "$image")"
  MSYS_NO_PATHCONV=1 docker cp "$cid:/out/jrs-server" "$(host_path "$out/jrs-server")"
  MSYS_NO_PATHCONV=1 docker cp "$cid:/out/jrs-run-schedule" "$(host_path "$out/jrs-run-schedule")"
  docker rm "$cid" >/dev/null
}

build_binaries() {
  local out="$STAGE/bin"
  mkdir -p "$out"
  rm -f "$out/jrs-server" "$out/jrs-run-schedule" "$out/jrs-schedule-dispatcher"

  if [[ "${PACKAGE_USE_DOCKER:-0}" == "1" ]]; then
    build_binaries_docker "$out"
  elif is_linux_amd64; then
    build_binaries_native "$out"
  elif have_docker; then
    log "Host is not Linux amd64; using optional Docker go-builder"
    build_binaries_docker "$out"
  else
    die "Native builds require Linux amd64 with Go+gcc. For Windows/cross-platform, install Docker or set PACKAGE_USE_DOCKER=1"
  fi

  chmod +x "$out/jrs-server" "$out/jrs-run-schedule" 2>/dev/null || true
  [[ -f "$out/jrs-server" && -s "$out/jrs-server" ]] || die "jrs-server missing after build"
  [[ -f "$out/jrs-run-schedule" && -s "$out/jrs-run-schedule" ]] || die "jrs-run-schedule missing after build"
  # Native systemd uses scripts/run_schedule.php; dispatcher is container-only.
  [[ ! -e "$out/jrs-schedule-dispatcher" ]] || rm -f "$out/jrs-schedule-dispatcher"
  log "  jrs-server SHA256: $(sha256_file "$out/jrs-server")"
  log "  jrs-run-schedule SHA256: $(sha256_file "$out/jrs-run-schedule")"
}

# Browser SDK files served by /sdk/{file} and /sdk/v1/{file}.
SDK_ASSET_FILES=(
  acugis-report.js
  acugis-report.min.js
  acugis-report.esm.js
  index.d.ts
)

build_javascript_sdk() {
  local sdk="$ROOT/sdk/javascript"
  local out="$STAGE/sdk/javascript/dist"
  [[ -f "$sdk/package.json" ]] || die "JavaScript SDK missing at $sdk"
  command -v npm >/dev/null || die "npm is required to build sdk/javascript"
  log "Building JavaScript SDK"
  if [[ ! -e "$sdk/node_modules" ]]; then
    SDK_NODE_MODULES_CREATED=1
  fi
  (
    cd "$sdk"
    npm ci
    npm run build
  )
  mkdir -p "$out"
  local f
  for f in "${SDK_ASSET_FILES[@]}"; do
    [[ -s "$sdk/dist/$f" ]] || die "SDK build did not produce dist/$f"
    cp -a "$sdk/dist/$f" "$out/$f"
  done
  grep -q 'embedReport' "$out/acugis-report.js" || die "acugis-report.js is missing embedReport"
  grep -q 'embedReport' "$out/acugis-report.min.js" || die "acugis-report.min.js is missing embedReport"
}

# ---------------------------------------------------------------------------
# Stage runtime tree
# ---------------------------------------------------------------------------
stage_tree() {
  rm -rf "$STAGE"
  mkdir -p "$STAGE"

  build_binaries
  build_javascript_sdk

  mkdir -p \
    "$STAGE/database/migrations" \
    "$STAGE/tomcat/lib" \
    "$STAGE/scripts" \
    "$STAGE/systemd" \
    "$STAGE/config" \
    "$STAGE/docs" \
    "$STAGE/reports/json" \
    "$STAGE/shared/ui" \
    "$STAGE/assets/js" \
    "$STAGE/assets/reports" \
    "$STAGE/schedules/output" \
    "$STAGE/schedules/logs" \
    "$STAGE/email_tmpl" \
    "$STAGE/resources" \
    "$STAGE/thumbnails" \
    "$STAGE/docker/report-server"

  if [[ "$EDITION" == "oss" ]]; then
    need_file "$ROOT/database/schema.oss.sql"
    cp -a "$ROOT/database/schema.oss.sql" "$STAGE/database/schema.sql"
  else
    need_file "$ROOT/database/schema.sql"
    cp -a "$ROOT/database/schema.sql" "$STAGE/database/schema.sql"
  fi

  local f base
  for f in "$ROOT/database/migrations/"*.sql; do
    [[ -f "$f" ]] || continue
    base="$(basename "$f")"
    if [[ "$EDITION" == "oss" ]]; then
      case "$base" in
        009_*|010_*|011_*|012_*|013_*|014_*|015_*) continue ;;
      esac
    fi
    cp -a "$f" "$STAGE/database/migrations/"
  done
  if [[ "$EDITION" != "oss" && -d "$ROOT/database/migrations/professional" ]]; then
    cp -a "$ROOT/database/migrations/professional" "$STAGE/database/migrations/"
  fi

  for f in "$ROOT/database"/migrate_*.sql; do
    [[ -f "$f" ]] || continue
    base="$(basename "$f")"
    if [[ "$EDITION" == "oss" ]]; then
      case "$base" in
        migrate_qgis.sql) continue ;;
      esac
    fi
    cp -a "$f" "$STAGE/database/"
  done

  ensure_standalone_war
  need_file "$WAR"
  verify_packaged_map_renderer "$WAR"
  ensure_duckdb_jdbc
  cp -a "$WAR" "$STAGE/tomcat/acugis-report-server.war"
  if [[ -f "$ROOT/stand-alone-tomcat-10/dist/sample-acugis.properties" ]]; then
    cp -a "$ROOT/stand-alone-tomcat-10/dist/sample-acugis.properties" "$STAGE/tomcat/sample-acugis.properties"
  elif [[ -f "$ROOT/stand-alone-tomcat-10/sample-acugis.properties" ]]; then
    cp -a "$ROOT/stand-alone-tomcat-10/sample-acugis.properties" "$STAGE/tomcat/sample-acugis.properties"
  fi

  # IBM Db2 is not redistributed. Db2 application support stays; customers
  # supply db2jcc4.jar on the Tomcat classpath. Oracle ojdbc11.jar is
  # Professional-only. verify-deb.sh forbids it in the Open Source package.
  # The list entry stays so the OSS export can delete that line. This tree
  # still has the jar, so the oss edition skips the copy itself.
  local jar
  for jar in \
    postgresql-42.7.3.jar \
    mysql-connector-j-9.7.0.jar \
    mssql-jdbc.jar \
    sqlite-jdbc.jar \
    snowflake-jdbc.jar \
    duckdb_jdbc-1.5.5.1-linux_amd64.jar
  do
    if [[ "$EDITION" == "oss" && "$jar" == "ojdbc11.jar" ]]; then
      continue
    fi
    need_file "$ROOT/stand-alone-tomcat-10/lib/$jar"
    cp -a "$ROOT/stand-alone-tomcat-10/lib/$jar" "$STAGE/tomcat/lib/"
  done

  # License texts that are not stored inside the Snowflake and SQL Server JARs.
  # MySQL Connector/J keeps its license inside the JAR; do not strip it.
  local notice
  for notice in \
    snowflake-jdbc-3.22.0-LICENSE.txt \
    mssql-jdbc-12.8.1-LICENSE.txt \
    JDBC-DRIVER-LICENSES.txt
  do
    need_file "$ROOT/stand-alone-tomcat-10/lib/$notice"
    cp -a "$ROOT/stand-alone-tomcat-10/lib/$notice" "$STAGE/tomcat/lib/"
  done

  copy_file "$ROOT/scripts/seed_admin.php" "$STAGE/scripts/seed_admin.php"
  copy_file "$ROOT/scripts/run_schedule.php" "$STAGE/scripts/run_schedule.php"
  copy_file "$ROOT/scripts/native-setup.sh" "$STAGE/scripts/native-setup.sh"
  copy_file "$ROOT/scripts/configure-instance.sh" "$STAGE/scripts/configure-instance.sh"
  chmod +x "$STAGE/scripts/native-setup.sh" "$STAGE/scripts/configure-instance.sh"
  # Copy lib/ as it exists in this source tree. Professional includes
  # lib/map_report.php. The OSS export omits it, and verify-deb.sh forbids it
  # in the Open Source package. bootstrap.php loads it only when the file is
  # present. Edition filters still omit migrations 009-015, QGIS/ArcGIS, and
  # spatial-selection.js.
  cp -a "$ROOT/lib" "$STAGE/lib"
  if [[ "$EDITION" == "oss" ]]; then
    rm -f "$STAGE/lib/map_report.php"
  fi
  copy_file "$ROOT/config.php" "$STAGE/config.php"

  copy_file "$ROOT/systemd/acugis-report-server.service" "$STAGE/systemd/acugis-report-server.service"
  copy_file "$ROOT/systemd/acugis-tomcat.service" "$STAGE/systemd/acugis-tomcat.service"

  copy_file "$ROOT/config/env.example" "$STAGE/config/env.example"
  if grep -q '^JRS_APP_VERSION=' "$STAGE/config/env.example"; then
    sed -i.bak "s/^JRS_APP_VERSION=.*/JRS_APP_VERSION=${VERSION}/" "$STAGE/config/env.example"
    rm -f "$STAGE/config/env.example.bak"
  fi
  copy_file "$ROOT/packaging/apache-acugis-report-server.conf" "$STAGE/config/apache-acugis-report-server.conf"
  copy_file "$ROOT/packaging/INSTALL.md" "$STAGE/docs/INSTALL.md"
  copy_file "$ROOT/installer.sh" "$STAGE/install.sh"
  chmod +x "$STAGE/install.sh"

  copy_file "$ROOT/reports/portal.css" "$STAGE/reports/portal.css"
  if [[ -d "$ROOT/shared/ui" ]]; then
    cp -a "$ROOT/shared/ui/." "$STAGE/shared/ui/"
  fi
  if [[ -f "$ROOT/assets/js/map-style.js" ]]; then
    copy_file "$ROOT/assets/js/map-style.js" "$STAGE/assets/js/map-style.js"
  fi
  need_file "$ROOT/assets/js/report-param-submit.js"
  copy_file "$ROOT/assets/js/report-param-submit.js" "$STAGE/assets/js/report-param-submit.js"
  if [[ "$EDITION" != "oss" && -f "$ROOT/assets/js/spatial-selection.js" ]]; then
    copy_file "$ROOT/assets/js/spatial-selection.js" "$STAGE/assets/js/spatial-selection.js"
  fi
  if [[ -f "$ROOT/email_tmpl/email_template.html" ]]; then
    copy_file "$ROOT/email_tmpl/email_template.html" "$STAGE/email_tmpl/email_template.html"
  fi

  if [[ -f "$ROOT/docker/report-server/geoserver-connections.properties.example" ]]; then
    copy_file "$ROOT/docker/report-server/geoserver-connections.properties.example" \
      "$STAGE/docker/report-server/geoserver-connections.properties.example"
  fi
  : > "$STAGE/docker/report-server/geoserver-connections.properties"
  if [[ -f "$ROOT/docker/report-server/context.xml.example" ]]; then
    copy_file "$ROOT/docker/report-server/context.xml.example" "$STAGE/tomcat/context.xml.example"
    copy_file "$ROOT/docker/report-server/context.xml.example" "$STAGE/docker/report-server/context.xml.example"
  fi

  if [[ "$EDITION" != "oss" ]]; then
    for f in qgis-connections.properties.example qgis-connections.properties; do
      if [[ -f "$ROOT/docker/report-server/$f" ]]; then
        copy_file "$ROOT/docker/report-server/$f" "$STAGE/docker/report-server/$f"
      fi
    done
  fi
  # ArcGIS Feature Server connections are shared by both editions.
  for f in arcgis-connections.properties.example arcgis-connections.properties; do
    if [[ -f "$ROOT/docker/report-server/$f" ]]; then
      copy_file "$ROOT/docker/report-server/$f" "$STAGE/docker/report-server/$f"
    fi
  done

  # Keep empty runtime dirs in the tarball.
  touch "$STAGE/reports/json/.keep"
  touch "$STAGE/schedules/output/.keep"
  touch "$STAGE/schedules/logs/.keep"
  touch "$STAGE/assets/reports/.keep"

  write_manifest
}

write_manifest() {
  cat > "$STAGE/RELEASE" <<EOF
product=${PRODUCT}
version=${VERSION}
edition=${EDITION}
architecture=${ARCH}
build_date=${BUILD_DATE}
revision=${REVISION}
EOF
  cat > "$STAGE/release.json" <<EOF
{
  "product": "${PRODUCT}",
  "version": "${VERSION}",
  "edition": "${EDITION}",
  "architecture": "${ARCH}",
  "build_date": "${BUILD_DATE}",
  "revision": "${REVISION}"
}
EOF
}

archive_and_checksum() {
  mkdir -p "$DIST_DIR"
  local parent tmp
  parent="$(dirname "$STAGE")"
  tmp="$parent/$STAGE_NAME"
  rm -rf "$tmp"
  mv "$STAGE" "$tmp"
  log "Writing $TARBALL"
  local tar_opts=(-czf "$TARBALL")
  if tar --help 2>&1 | grep -q -- '--mode'; then
    tar_opts+=(--mode='u+rwx,g+rx,o+rx')
  fi
  tar "${tar_opts[@]}" -C "$parent" "$STAGE_NAME"
  rm -rf "$tmp"

  local sum
  sum="$(sha256_file "$TARBALL")"
  local name
  name="$(basename "$TARBALL")"
  echo "$sum  $name" > "${TARBALL}.sha256"
  local sums="$DIST_DIR/SHA256SUMS"
  if [[ -f "$sums" ]]; then
    grep -v " ${name}\$" "$sums" > "${sums}.tmp" || true
    mv "${sums}.tmp" "$sums"
  fi
  echo "$sum  $name" >> "$sums"
  log "SHA-256 $sum"
}

# ---------------------------------------------------------------------------
main() {
  log "Packaging $PRODUCT $EDITION $VERSION ($ARCH)"
  if [[ "${SKIP_STAGE:-0}" == "1" && -f "$STAGE/RELEASE" && -x "$STAGE/bin/jrs-server" ]]; then
    log "Reusing existing stage $STAGE"
    verify_packaged_map_renderer "$STAGE/tomcat/acugis-report-server.war"
  else
    ensure_standalone_war
    need_file "$WAR"
    verify_packaged_map_renderer "$WAR"
    stage_tree
  fi
  if [[ "${SKIP_ARCHIVE:-0}" == "1" ]]; then
    log "Staged at $STAGE (SKIP_ARCHIVE=1)"
    return 0
  fi
  archive_and_checksum
  log "Done: $TARBALL"
}

main
