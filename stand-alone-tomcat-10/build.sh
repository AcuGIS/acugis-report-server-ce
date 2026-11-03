#!/usr/bin/env bash
# Linux/macOS wrapper — prefers Maven, falls back to PowerShell build.ps1 if pwsh is available.
set -euo pipefail
cd "$(dirname "$0")"

if command -v mvn >/dev/null 2>&1; then
  mvn -Pstandalone -DskipTests package
  mkdir -p dist
  cp -f target/acugis-report-server.war dist/
  cp -f sample-acugis.properties dist/
  cp -f dist-README.txt dist/README.txt
  echo "Built dist/acugis-report-server.war via Maven"
  exit 0
fi

if command -v pwsh >/dev/null 2>&1; then
  pwsh -File ./build.ps1 -SkipMaven
  exit 0
fi

if command -v powershell >/dev/null 2>&1; then
  powershell -File ./build.ps1 -SkipMaven
  exit 0
fi

echo "Install Maven or PowerShell/pwsh to build the standalone WAR." >&2
exit 1
