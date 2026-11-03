#!/bin/bash
set -e

APP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

php "$APP_DIR/reports/generate_jasper_context.php"
cd "$APP_DIR/docker/report-server"
docker compose restart
