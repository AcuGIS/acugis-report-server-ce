#!/usr/bin/env bash
# Smoke tests for Docker packaging (run from docker/ after compose is up).
set -euo pipefail
BASE="${JRS_SMOKE_URL:-http://127.0.0.1:9088}"
USER="${JRS_ADMIN_USER:-admin}"
PASS="${JRS_ADMIN_PASSWORD:?JRS_ADMIN_PASSWORD required}"

echo "== health =="
curl -fsS "$BASE/health" | tee /tmp/jrs-health.json
echo

echo "== ready =="
curl -fsS "$BASE/ready" | tee /tmp/jrs-ready.json
echo

echo "== login =="
curl -fsS -c /tmp/jrs-cookies.txt -X POST "$BASE/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"$USER\",\"password\":\"$PASS\"}" | tee /tmp/jrs-login.json
echo

echo "== PDF hello (DemoPG) via execute proxy =="
# Public execute path used by the Go server (session cookie).
curl -fsS -b /tmp/jrs-cookies.txt -o /tmp/hello.pdf \
  "$BASE/reports/execute?_repName=hello&_repFormat=PDF&_dataSource=DemoPG"
file /tmp/hello.pdf || true
head -c 5 /tmp/hello.pdf | od -An -tx1
# PDF magic %PDF-
python3 - <<'PY'
from pathlib import Path
b = Path('/tmp/hello.pdf').read_bytes()[:5]
assert b == b'%PDF-', b
print('PDF OK', Path('/tmp/hello.pdf').stat().st_size, 'bytes')
PY

echo "== DuckDB hello (if seeded) =="
if curl -fsS -b /tmp/jrs-cookies.txt -o /tmp/hello_duckdb.pdf \
  "$BASE/reports/execute?_repName=hello_duckdb&_repFormat=PDF&_dataSource=DemoDuckDB"; then
  python3 - <<'PY'
from pathlib import Path
b = Path('/tmp/hello_duckdb.pdf').read_bytes()[:5]
assert b == b'%PDF-', b
print('DuckDB PDF OK', Path('/tmp/hello_duckdb.pdf').stat().st_size, 'bytes')
PY
else
  echo "DuckDB demo execute skipped/failed (see known limitations if expected)"
fi

echo "SMOKE OK"
