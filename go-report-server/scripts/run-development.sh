#!/usr/bin/env bash
# Run only the parallel Go development server.
# Never stops Apache, PHP, Tomcat, or PostgreSQL.
# Never alters routing or existing configuration files.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [[ -f "$ROOT/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT/.env"
  set +a
fi

# Prefer the sibling PHP install root when JRS_JASPER_HOME is unset.
if [[ -z "${JRS_JASPER_HOME:-}" ]]; then
  PARENT="$(cd "$ROOT/.." && pwd)"
  if [[ -f "$PARENT/config.php" ]]; then
    export JRS_JASPER_HOME="$PARENT"
  fi
fi

export JRS_LISTEN_ADDR="${JRS_LISTEN_ADDR:-127.0.0.1:9088}"
export JRS_BASE_PATH="${JRS_BASE_PATH:-/}"
export JRS_ENVIRONMENT="${JRS_ENVIRONMENT:-development}"

echo "Starting AcuGIS Report Server (Go) on ${JRS_LISTEN_ADDR} (base path ${JRS_BASE_PATH})"
echo "PHP application is unchanged; this process is development-only."
exec go run ./cmd/jrs-server
