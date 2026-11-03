#!/usr/bin/env bash
# Fail unless acugis-report-map.jar exposes the 9-argument MapRenderer.render
# overload required by the current coordinate-map report generator:
#
#   render(Connection, String, int, int, String styleJson, String basemap,
#          String srs, String latitudeField, String longitudeField)
#
# The contract is the JVM method descriptor in MapRenderer.class, not a
# timestamp. python3 (or python) reads the jar/WAR directly. javap is used
# when a Python interpreter is not available.
#
# Usage:
#   scripts/verify-map-renderer-api.sh --war path/to/acugis-report-server.war
#   scripts/verify-map-renderer-api.sh --jar path/to/acugis-report-map.jar

set -euo pipefail

DESCRIPTOR='(Ljava/sql/Connection;Ljava/lang/String;IILjava/lang/String;Ljava/lang/String;Ljava/lang/String;Ljava/lang/String;Ljava/lang/String;)Ljava/awt/image/BufferedImage;'
JAVAP_NEEDLE='render(java.sql.Connection,java.lang.String,int,int,java.lang.String,java.lang.String,java.lang.String,java.lang.String,java.lang.String)'

die() { echo "ERROR: $*" >&2; exit 1; }

usage() {
  echo "Usage: $0 --war FILE | --jar FILE" >&2
  exit 2
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

verify_with_python() {
  local mode="$1" path="$2"
  local py
  py="$(python_bin)"
  local status=0
  "$py" - "$mode" "$path" "$DESCRIPTOR" <<'PY' || status=$?
import io, sys, zipfile
mode, path, desc = sys.argv[1], sys.argv[2], sys.argv[3]
entry_class = "com/acugis/map/MapRenderer.class"
entry_jar = "WEB-INF/lib/acugis-report-map.jar"
try:
    if mode == "jar":
        jar_bytes = open(path, "rb").read()
    elif mode == "war":
        with zipfile.ZipFile(path) as war:
            try:
                jar_bytes = war.read(entry_jar)
            except KeyError:
                sys.exit(2)
    else:
        sys.exit(4)
    if not jar_bytes:
        sys.exit(2)
    with zipfile.ZipFile(io.BytesIO(jar_bytes)) as jar:
        try:
            cls = jar.read(entry_class)
        except KeyError:
            sys.exit(3)
except FileNotFoundError:
    sys.exit(5)
sys.exit(0 if desc.encode("utf-8") in cls else 1)
PY
  case "$status" in
    0) return 0 ;;
    1)
      die "$path does not expose MapRenderer.render(Connection, String, int, int, String, String, String, String latitudeField, String longitudeField)"
      ;;
    2) die "$path is missing WEB-INF/lib/acugis-report-map.jar" ;;
    3) die "$path is missing com/acugis/map/MapRenderer.class" ;;
    5) die "file not found: $path" ;;
    *) die "MapRenderer API check failed for $path (status $status)" ;;
  esac
}

verify_with_javap() {
  local mode="$1" path="$2"
  command -v javap >/dev/null 2>&1 || die "python3 or javap is required to verify MapRenderer.render"
  command -v unzip >/dev/null 2>&1 || die "unzip is required to read the archive when python3 is unavailable"
  local tmp jar
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' RETURN
  jar="$path"
  if [[ "$mode" == "war" ]]; then
    jar="$tmp/acugis-report-map.jar"
    unzip -p "$path" WEB-INF/lib/acugis-report-map.jar > "$jar" || die "$path is missing WEB-INF/lib/acugis-report-map.jar"
    [[ -s "$jar" ]] || die "$path is missing WEB-INF/lib/acugis-report-map.jar"
  fi
  [[ -f "$jar" && -s "$jar" ]] || die "MapRenderer jar missing or empty: $path"
  local out flat
  out="$(javap -public -classpath "$jar" com.acugis.map.MapRenderer 2>&1)" || {
    printf '%s\n' "$out" >&2
    die "javap failed for $path"
  }
  flat="$(printf '%s' "$out" | tr -d '[:space:]')"
  [[ "$flat" == *"$JAVAP_NEEDLE"* ]] || {
    printf '%s\n' "$out" >&2
    die "$path does not expose MapRenderer.render(Connection, String, int, int, String, String, String, String latitudeField, String longitudeField)"
  }
}

verify_target() {
  local mode="$1" path="$2"
  [[ -f "$path" && -s "$path" ]] || die "file missing or empty: $path"
  if python_bin >/dev/null 2>&1; then
    verify_with_python "$mode" "$path"
  else
    verify_with_javap "$mode" "$path"
  fi
}

MODE=""
TARGET=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --war|--jar)
      MODE="${1#--}"
      TARGET="${2:-}"
      [[ -n "$TARGET" ]] || usage
      shift 2
      ;;
    -h|--help) usage ;;
    *) usage ;;
  esac
done
[[ -n "$MODE" && -n "$TARGET" ]] || usage
verify_target "$MODE" "$TARGET"
