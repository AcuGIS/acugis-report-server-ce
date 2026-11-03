#!/bin/bash
# Noninteractive instance configure/reconfigure for native and Debian installs.
# Intended for AcuGIS Agent after writing /etc/acugis-report-server/env
# (or after apt has generated first-boot credentials).
#
# Usage (no prompts):
#   sudo /opt/acugis-report-server/scripts/configure-instance.sh
#
# Optional environment (used when creating a missing env file, or to fill
# empty JRS_DB_PASS / JRS_ADMIN_PASSWORD on first boot):
#   JRS_DB_HOST JRS_DB_PORT JRS_DB_NAME JRS_DB_USER JRS_DB_PASS
#   JRS_ADMIN_USER JRS_ADMIN_PASSWORD
#   JRS_SKIP_TOMCAT=1  JRS_SKIP_DB=1  JRS_SKIP_APACHE=1  JRS_SKIP_SYSTEMD=1

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/scripts/native-setup.sh"

guard_edition
configure_instance
