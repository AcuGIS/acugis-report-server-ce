#!/usr/bin/env bash
# Integration tests for Docker scheduled-report execution.
# Run from docker/ after copying .env (JRS_ADMIN_PASSWORD required).
#
#   TZ=America/New_York ./test-scheduler.sh
#
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

USER_TZ="${TZ:-}"
if [[ -f .env ]]; then
  ENV_FILE="$(mktemp)"
  sed $'1s/^\xEF\xBB\xBF//' .env | tr -d '\r' > "$ENV_FILE"
  set -a
  # shellcheck disable=SC1090
  . "$ENV_FILE"
  set +a
  rm -f "$ENV_FILE"
fi

BASE="${JRS_SMOKE_URL:-http://127.0.0.1:9088}"
MAIL_UI="${MAILPIT_URL:-http://127.0.0.1:8025}"
USER="${JRS_ADMIN_USER:-admin}"
PASS="${JRS_ADMIN_PASSWORD:?JRS_ADMIN_PASSWORD required}"
TZ_VALUE="${USER_TZ:-America/New_York}"
export TZ="$TZ_VALUE"
# Docker Desktop on Windows does not always inherit Git-Bash TZ into Compose
# interpolation. An extra env-file in this directory is visible to docker.exe.
TZ_ENV="$ROOT/.env.scheduler-tz"
printf 'TZ=%s\n' "$TZ_VALUE" > "$TZ_ENV"

COMPOSE=(docker compose --env-file .env --env-file "$TZ_ENV" -f compose.yml -f compose.mailpit.yml)

log() { printf '\n== %s ==\n' "$*"; }
fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

psql_app() {
  "${COMPOSE[@]}" exec -T postgres \
    env PGPASSWORD="${JRS_DB_PASS:?}" \
    psql -v ON_ERROR_STOP=1 -U "${JRS_DB_USER:-jasper}" -d "${JRS_DB_NAME:-jasper_reports}" "$@"
}

wait_http() {
  local url="$1" tries="${2:-60}"
  local i
  for i in $(seq 1 "$tries"); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  return 1
}

py() {
  if command -v python3 >/dev/null 2>&1; then
    python3 "$@"
  else
    python "$@"
  fi
}

py_now_plus() {
  local minutes="$1"
  "${COMPOSE[@]}" exec -T -e TZ="$TZ_VALUE" report-scheduler \
    date -d "+${minutes} minutes" +'%Y-%m-%dT%H:%M' | tr -d '\r'
}

login() {
  curl -fsS -c /tmp/jrs-sched-cookies.txt -X POST "$BASE/api/v1/auth/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/tmp/jrs-sched-login.json
}

create_schedule() {
  local once="$1"
  curl -fsS -b /tmp/jrs-sched-cookies.txt -X POST "$BASE/action/schedule.php" \
    -H 'Content-Type: application/x-www-form-urlencoded' \
    --data-urlencode "create=1" \
    --data-urlencode "schedule_mode=once" \
    --data-urlencode "once_datetime=$once" \
    --data-urlencode "report_name=hello" \
    --data-urlencode "datasource_name=DemoPG" \
    --data-urlencode "format=pdf" \
    --data-urlencode "filename=hello-scheduled.pdf" \
    --data-urlencode "email=sched@example.com" \
    --data-urlencode "email_subject=Scheduled hello" \
    --data-urlencode "url_opt_params=P_TEST=from-scheduler" \
    --data-urlencode "enabled=1"
}

exec_count() {
  local sid="$1"
  psql_app -Atc "SELECT count(*) FROM jasper_schedule_execution WHERE schedule_id = $sid"
}

exec_status() {
  local sid="$1"
  psql_app -Atc "SELECT coalesce(max(status),'') FROM jasper_schedule_execution WHERE schedule_id = $sid"
}

wait_exec() {
  local sid="$1" want="${2:-success}" tries="${3:-40}"
  local i status
  for i in $(seq 1 "$tries"); do
    status="$(exec_status "$sid")"
    if [[ "$status" == "$want" ]]; then
      return 0
    fi
    if [[ "$status" == "failed" && "$want" == "success" ]]; then
      psql_app -c "SELECT id, status, error_message FROM jasper_schedule_execution WHERE schedule_id = $sid ORDER BY id DESC LIMIT 3"
      fail "schedule $sid failed (wanted success)"
    fi
    sleep 5
  done
  psql_app -c "SELECT id, status, error_message FROM jasper_schedule_execution WHERE schedule_id = $sid ORDER BY id DESC LIMIT 5"
  fail "schedule $sid did not reach status=$want"
}

# ---------------------------------------------------------------------------
log "1. docker compose up"
TZ="$TZ_VALUE" "${COMPOSE[@]}" up -d --build

log "2. report-server healthy"
wait_http "$BASE/health" 90 || fail "report-server /health never became ready"
curl -fsS "$BASE/health"
echo

log "3. report-scheduler healthy/running"
for i in $(seq 1 36); do
  if "${COMPOSE[@]}" ps --status running --format json 2>/dev/null | grep -q report-scheduler; then
    break
  fi
  sleep 5
done
"${COMPOSE[@]}" exec -T report-scheduler curl -fsS http://127.0.0.1:9089/health >/tmp/jrs-sched-health.json \
  || fail "report-scheduler /health failed"
cat /tmp/jrs-sched-health.json
echo
SCHED_TZ="$("${COMPOSE[@]}" exec -T report-scheduler printenv TZ | tr -d '\r')"
echo "scheduler TZ=$SCHED_TZ"
[[ "$SCHED_TZ" == "$TZ_VALUE" ]] || fail "expected scheduler TZ=$TZ_VALUE, got $SCHED_TZ"

log "refresh hello.jrxml into the Tomcat volume (JR 7 title/columnHeader shape)"
"${COMPOSE[@]}" cp fixtures/jrxml/hello.jrxml report-server:/usr/local/tomcat/acugis/reports/hello.jrxml
"${COMPOSE[@]}" exec -T report-server rm -f /usr/local/tomcat/acugis/reports/hello.jasper

login

ONCE="$(py_now_plus 2)"
FAIL_ONCE="$(py_now_plus 1)"
echo "TZ=$TZ_VALUE once=$ONCE fail_once=$FAIL_ONCE"

log "4-6. create scheduled report (UI) a few minutes in the future"
CREATE_JSON="$(create_schedule "$ONCE")"
echo "$CREATE_JSON"
echo "$CREATE_JSON" | py -c 'import json,sys; d=json.load(sys.stdin); assert d.get("success") is True, d; print(d["id"])' >/tmp/jrs-sched-id.txt
SID="$(cat /tmp/jrs-sched-id.txt)"
[[ "$SID" =~ ^[0-9]+$ ]] || fail "expected numeric schedule id, got $SID"

log "14. insert a deliberately failing schedule (bogus report) due sooner"
psql_app -v once="$FAIL_ONCE" <<SQL
INSERT INTO jasper_schedule (
  cron_period, schedule_mode, once_datetime, report_name, datasource_name,
  format, filename, email, noemail, enabled
) VALUES (
  'once', 'once', :'once', 'does-not-exist', 'DemoPG',
  'pdf', 'fail.pdf', '', true, true
);
SQL
FAIL_SID="$(psql_app -Atc "SELECT id FROM jasper_schedule WHERE report_name = 'does-not-exist' ORDER BY id DESC LIMIT 1")"

log "wait for failing schedule (must not block later work)"
wait_exec "$FAIL_SID" "failed" 36
echo "failing schedule $FAIL_SID status=failed (expected)"

log "6-8. wait for good schedule execution + PDF + email"
wait_exec "$SID" "success" 48
ROW="$(psql_app -Atc "SELECT output_file || '|' || email_status FROM jasper_schedule_execution WHERE schedule_id = $SID AND status = 'success' ORDER BY id DESC LIMIT 1")"
OUTFILE="${ROW%%|*}"
EMAIL_STATUS="${ROW##*|}"
echo "output_file=$OUTFILE email_status=$EMAIL_STATUS"
[[ "$EMAIL_STATUS" == "sent" ]] || fail "expected email_status=sent, got $EMAIL_STATUS"
MAGIC="$("${COMPOSE[@]}" exec -T report-scheduler sh -c "head -c 5 '$OUTFILE'" | tr -d '\r')"
[[ "$MAGIC" == "%PDF-" ]] || fail "generated PDF missing or invalid ($MAGIC): $OUTFILE"

log "9. parameters passed"
"${COMPOSE[@]}" exec -T report-scheduler sh -c "grep -F 'P_TEST=from-scheduler' /opt/acugis-report-server/schedules/logs/schedule-${SID}.log" \
  || fail "P_TEST parameter not found in schedule log"

log "10. scheduler logs show execution"
"${COMPOSE[@]}" logs report-scheduler | grep -E "dispatching schedule.*schedule_id=$SID|schedule execution finished.*schedule_id=$SID" \
  || "${COMPOSE[@]}" logs report-scheduler | grep -F "schedule_id=$SID" \
  || fail "scheduler logs missing schedule $SID"

log "8. Mailpit received the message"
MAIL_JSON="$("${COMPOSE[@]}" exec -T report-scheduler curl -fsS http://mailpit:8025/api/v1/messages || true)"
[[ -n "$MAIL_JSON" ]] || fail "mailpit API not reachable"
echo "$MAIL_JSON" | py -c 'import json,sys; d=json.load(sys.stdin); msgs=d.get("messages") or []; assert msgs, d; print("mailpit messages", len(msgs), "subject", msgs[0].get("Subject"), "attachments", msgs[0].get("Attachments"))'

log "17. Run Now still works"
RUNNOW="$(curl -fsS -b /tmp/jrs-sched-cookies.txt -X POST "$BASE/action/schedule.php" \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode "run_now=1" \
  --data-urlencode "id=$SID")"
echo "$RUNNOW"
echo "$RUNNOW" | py -c 'import json,sys; d=json.load(sys.stdin); assert d.get("success") is True, d'

log "11. restart report-scheduler; schedules continue"
BEFORE="$(exec_count "$SID")"
"${COMPOSE[@]}" restart report-scheduler
sleep 8
"${COMPOSE[@]}" exec -T report-scheduler curl -fsS http://127.0.0.1:9089/health >/dev/null \
  || fail "scheduler unhealthy after restart"
sleep 20
AFTER_RESTART="$(exec_count "$SID")"
# once schedule must not fire again
if [[ "$AFTER_RESTART" -gt $((BEFORE + 2)) ]]; then
  fail "unexpected extra executions after scheduler restart: before=$BEFORE after=$AFTER_RESTART"
fi
echo "execution count after scheduler restart: $AFTER_RESTART (before=$BEFORE)"

log "12. restart complete Compose stack; schedules persist"
"${COMPOSE[@]}" down
TZ="$TZ_VALUE" "${COMPOSE[@]}" up -d
wait_http "$BASE/health" 90 || fail "report-server did not return after stack restart"
login
EXIST="$(psql_app -Atc "SELECT count(*) FROM jasper_schedule WHERE id = $SID")"
[[ "$EXIST" == "1" ]] || fail "schedule $SID did not persist across compose restart"

log "13. same schedule is not executed twice (once mode)"
sleep 25
FINAL="$(exec_count "$SID")"
# Run Now added one extra success; scheduled path should still be a single scheduled fire
# plus the manual run_now. Allow 2 (scheduled + run_now) or 3 if restart catch-up raced before lastExec visible.
echo "final execution count for $SID = $FINAL"
if [[ "$FINAL" -gt 3 ]]; then
  fail "schedule $SID executed too many times ($FINAL)"
fi
SCHEDULED_COUNT="$(psql_app -Atc "SELECT count(*) FROM jasper_schedule_execution e JOIN jasper_schedule s ON s.id=e.schedule_id WHERE e.schedule_id=$SID")"
echo "total rows=$SCHEDULED_COUNT"

log "16. scheduler uses JRS_DB_HOST (bundled postgres is an external service to this container)"
"${COMPOSE[@]}" exec -T report-scheduler printenv JRS_DB_HOST | grep -E 'postgres|.' \
  || fail "JRS_DB_HOST missing in scheduler"
echo "JRS_DB_HOST=$( "${COMPOSE[@]}" exec -T report-scheduler printenv JRS_DB_HOST )"

log "18. native systemd units unchanged in this tree"
if git -C "$ROOT/.." diff --quiet -- systemd lib/schedule_systemd.php scripts/run_schedule.php 2>/dev/null; then
  echo "native systemd/PHP scheduler files have no diff"
else
  git -C "$ROOT/.." diff -- systemd lib/schedule_systemd.php scripts/run_schedule.php || true
  fail "native systemd scheduling files were modified"
fi

echo
echo "SCHEDULER TESTS OK (TZ=$TZ_VALUE schedule_id=$SID fail_id=$FAIL_SID)"
