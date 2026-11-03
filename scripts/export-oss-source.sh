#!/usr/bin/env bash
# Export a clean public OSS/Community source tree.
#
# The private repository is not modified. Professional files are omitted from
# the output directory only. Exclusions are listed in
# scripts/oss-export-exclude.txt.
#
# Usage:
#   scripts/export-oss-source.sh [OUTPUT_DIR]
#
# Default output: dist/oss-source
# A previous output directory is replaced. The source tree is not.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEST="${1:-$ROOT/dist/oss-source}"
MANIFEST="$ROOT/scripts/oss-export-exclude.txt"

if [[ ! -f "$MANIFEST" ]]; then
  echo "missing exclusion manifest: $MANIFEST" >&2
  exit 1
fi

# Resolve DEST without following a not-yet-created path's parent incorrectly.
mkdir -p "$(dirname "$DEST")"
DEST="$(cd "$(dirname "$DEST")" && pwd)/$(basename "$DEST")"

if [[ "$DEST" == "$ROOT" || "$ROOT" == "$DEST"/* ]]; then
  echo "refusing to export onto the source tree: $DEST" >&2
  exit 1
fi

PYTHON=""
if command -v python3 >/dev/null 2>&1; then
  PYTHON="python3"
elif command -v python >/dev/null 2>&1; then
  PYTHON="python"
else
  echo "python is required to export the OSS tree" >&2
  exit 1
fi

if [[ -e "$DEST" ]]; then
  rm -rf "$DEST"
fi

echo "Exporting OSS source to $DEST"
"$PYTHON" "$ROOT/scripts/oss_export.py" export "$ROOT" "$DEST"

if ! command -v go >/dev/null 2>&1; then
  echo "go is required to tidy and build the exported tree" >&2
  exit 1
fi

echo "Running go mod tidy in the exported module"
(
  cd "$DEST/go-report-server"
  go mod tidy
)

"$PYTHON" "$ROOT/scripts/oss_export.py" leak-check "$ROOT" "$DEST"

BUILD="$(mktemp -d "${TMPDIR:-/tmp}/jrs-oss-build.XXXXXX")"
cleanup() { rm -rf "$BUILD"; }
trap cleanup EXIT

echo "Building exported OSS entry points"
(
  cd "$DEST/go-report-server"
  go build -tags oss -o "$BUILD/jrs-server" ./cmd/jrs-server
  go build -tags oss -o "$BUILD/jrs-run-schedule" ./cmd/jrs-run-schedule
  go build -tags oss -o "$BUILD/jrs-schedule-dispatcher" ./cmd/jrs-schedule-dispatcher
  go build -tags oss -o "$BUILD/jrs-auth-check" ./cmd/jrs-auth-check
)

echo "OSS export verified: $DEST"
