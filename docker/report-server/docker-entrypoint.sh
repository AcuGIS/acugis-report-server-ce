#!/usr/bin/env bash
# AcuGIS Report Server — Docker entrypoint (packaging only).
# Starts Tomcat 10 + existing jrs-server in one container.
# Does not modify application source.

set -euo pipefail

log() { echo "[acugis-docker] $*"; }

IMAGE_ROOT="${ACUGIS_DOCKER_IMAGE_ROOT:-/opt/acugis-image}"
APP_HOME="${JRS_JASPER_HOME:-/opt/acugis-report-server}"
CATALINA_HOME="${CATALINA_HOME:-/usr/local/tomcat}"
CATALINA_BASE="${CATALINA_BASE:-$CATALINA_HOME}"
CONF_PERSIST="${ACUGIS_DOCKER_CONF_PERSIST:-/var/acugis/tomcat-conf}"
SEED_DEMO="${ACUGIS_DOCKER_SEED_DEMO:-1}"

export CATALINA_HOME CATALINA_BASE
export JRS_JASPER_HOME="$APP_HOME"

# Tomcat and jrs-server share this container's user. Native packages chown
# report files to the tomcat account so a separate Tomcat service can read
# them. This image does not create that account. Compose already sets root,
# which is the image user; a plain docker run otherwise inherits the native
# default and logs "chown tomcat:tomcat: invalid user".
if [[ -z "${JRS_REPORTS_OWNER:-}" ]]; then
  export JRS_REPORTS_OWNER="$(id -un)"
fi
if [[ -z "${JRS_REPORTS_GROUP:-}" ]]; then
  export JRS_REPORTS_GROUP="$(id -gn)"
fi

mkdir -p \
  "$APP_HOME" \
  "$CONF_PERSIST" \
  "$CATALINA_BASE/acugis/reports" \
  "$CATALINA_BASE/acugis/uploads" \
  "$CATALINA_BASE/acugis/fonts" \
  "$CATALINA_BASE/acugis/drivers" \
  "$CATALINA_BASE/acugis/licenses" \
  "$CATALINA_BASE/acugis/temp" \
  "$CATALINA_BASE/acugis/resource-paths" \
  /data

# ---------------------------------------------------------------------------
# Seed persistent app home from immutable image content (first boot only).
# ---------------------------------------------------------------------------
seed_app_home() {
  if [[ -f "$APP_HOME/.acugis-docker-seeded" ]]; then
    return 0
  fi
  log "Seeding $APP_HOME from $IMAGE_ROOT"
  # Preserve any already-present volume files; copy missing tree from image.
  cp -a "$IMAGE_ROOT"/. "$APP_HOME"/
  mkdir -p \
    "$APP_HOME/reports/json" \
    "$APP_HOME/resources" \
    "$APP_HOME/thumbnails" \
    "$APP_HOME/schedules/output" \
    "$APP_HOME/schedules/logs" \
    "$APP_HOME/email_tmpl" \
    "$APP_HOME/explorer/views" \
    "$APP_HOME/assets/reports" \
    "$APP_HOME/assets/js" \
    "$APP_HOME/shared/ui" \
    "$APP_HOME/var/duckdb"
  touch "$APP_HOME/.acugis-docker-seeded"
}

# Refresh immutable UI assets every boot so existing volumes pick up CSS/JS
# after image upgrades (first-boot seed alone is not enough).
sync_ui_assets() {
  mkdir -p "$APP_HOME/reports" "$APP_HOME/shared/ui" "$APP_HOME/assets/js" "$APP_HOME/assets/reports"
  if [[ -f "$IMAGE_ROOT/reports/portal.css" ]]; then
    cp -a "$IMAGE_ROOT/reports/portal.css" "$APP_HOME/reports/portal.css"
  else
    log "WARNING: $IMAGE_ROOT/reports/portal.css missing from image"
  fi
  if [[ -d "$IMAGE_ROOT/shared/ui" ]]; then
    cp -a "$IMAGE_ROOT/shared/ui/." "$APP_HOME/shared/ui/"
  else
    log "WARNING: $IMAGE_ROOT/shared/ui missing from image"
  fi
  if [[ -d "$IMAGE_ROOT/assets/js" ]]; then
    cp -a "$IMAGE_ROOT/assets/js/." "$APP_HOME/assets/js/"
  fi
}

# ---------------------------------------------------------------------------
# Persist / restore Tomcat conf files shared with Go (context.xml, props).
# ---------------------------------------------------------------------------
sync_tomcat_conf() {
  mkdir -p "$CONF_PERSIST"
  local f
  for f in context.xml geoserver-connections.properties qgis-connections.properties arcgis-connections.properties acugis.properties; do
    if [[ -f "$CONF_PERSIST/$f" ]]; then
      cp -a "$CONF_PERSIST/$f" "$CATALINA_BASE/conf/$f"
    elif [[ -f "$CATALINA_BASE/conf/$f" ]]; then
      cp -a "$CATALINA_BASE/conf/$f" "$CONF_PERSIST/$f"
    fi
  done

  if [[ ! -f "$CATALINA_BASE/conf/context.xml" ]]; then
    if [[ -f "$IMAGE_ROOT/docker-fixtures/context.xml.default" ]]; then
      cp -a "$IMAGE_ROOT/docker-fixtures/context.xml.default" "$CATALINA_BASE/conf/context.xml"
    else
      printf '%s\n' '<?xml version="1.0" encoding="UTF-8"?>' '<Context>' '</Context>' \
        >"$CATALINA_BASE/conf/context.xml"
    fi
    cp -a "$CATALINA_BASE/conf/context.xml" "$CONF_PERSIST/context.xml"
  fi

  # Ensure connection property files exist without truncating restored copies.
  for f in geoserver-connections.properties qgis-connections.properties arcgis-connections.properties; do
    [[ -f "$CATALINA_BASE/conf/$f" ]] || : >"$CATALINA_BASE/conf/$f"
    [[ -f "$CONF_PERSIST/$f" ]] || cp -a "$CATALINA_BASE/conf/$f" "$CONF_PERSIST/" || true
  done
}

persist_tomcat_conf() {
  mkdir -p "$CONF_PERSIST"
  local f
  for f in context.xml geoserver-connections.properties qgis-connections.properties arcgis-connections.properties acugis.properties; do
    if [[ -f "$CATALINA_BASE/conf/$f" ]]; then
      cp -a "$CATALINA_BASE/conf/$f" "$CONF_PERSIST/$f"
    fi
  done
}

# ---------------------------------------------------------------------------
# PostgreSQL readiness + schema/migrations (existing SQL files only).
# ---------------------------------------------------------------------------
wait_for_postgres() {
  local host="${JRS_DB_HOST:-postgres}"
  local port="${JRS_DB_PORT:-5432}"
  local user="${JRS_DB_USER:-jasper}"
  local db="${JRS_DB_NAME:-jasper_reports}"
  local i
  log "Waiting for PostgreSQL at ${host}:${port}…"
  for i in $(seq 1 60); do
    if PGPASSWORD="${JRS_DB_PASS:-}" pg_isready -h "$host" -p "$port" -U "$user" -d "$db" >/dev/null 2>&1; then
      log "PostgreSQL is ready"
      return 0
    fi
    sleep 2
  done
  log "ERROR: PostgreSQL not ready after timeout"
  return 1
}

psql_app() {
  PGPASSWORD="${JRS_DB_PASS:-}" psql \
    -v ON_ERROR_STOP=1 \
    -h "${JRS_DB_HOST:-postgres}" \
    -p "${JRS_DB_PORT:-5432}" \
    -U "${JRS_DB_USER:-jasper}" \
    -d "${JRS_DB_NAME:-jasper_reports}" \
    "$@"
}

apply_sql_file() {
  local file="$1"
  log "  SQL: $(basename "$file")"
  if ! psql_app -f "$file"; then
    log "  WARNING: $(basename "$file") reported errors (may already be applied)"
  fi
}

bootstrap_database() {
  local marker_table="acugis_docker_schema_applied"
  local applied
  applied="$(psql_app -Atc "SELECT to_regclass('public.${marker_table}') IS NOT NULL" | tr -d '[:space:]')"
  if [[ "$applied" == "t" ]]; then
    log "Database schema already applied"
    if [[ "${ACUGIS_BUILD_EDITION:-oss}" != "oss" && -f "$APP_HOME/database/migrate_qgis.sql" ]]; then
      log "Ensuring QGIS table"
      apply_sql_file "$APP_HOME/database/migrate_qgis.sql"
    fi
    if [[ -f "$APP_HOME/database/migrate_arcgis.sql" ]]; then
      log "Ensuring ArcGIS table"
      apply_sql_file "$APP_HOME/database/migrate_arcgis.sql"
    fi
    return 0
  fi

  if [[ "${ACUGIS_BUILD_EDITION:-oss}" == "oss" && -f "$APP_HOME/database/schema.oss.sql" ]]; then
    log "Applying database/schema.oss.sql"
    psql_app -f "$APP_HOME/database/schema.oss.sql"
  else
    log "Applying database/schema.sql"
    psql_app -f "$APP_HOME/database/schema.sql"
  fi

  local mig
  if [[ -d "$APP_HOME/database/migrations" ]]; then
    log "Applying database/migrations/*.sql"
    # shellcheck disable=SC2012
    for mig in $(ls -1 "$APP_HOME/database/migrations"/*.sql 2>/dev/null | sort); do
      apply_sql_file "$mig"
    done
  fi

  # Legacy top-level migrate_*.sql files (datasource columns, GeoServer/ArcGIS).
  log "Applying database/migrate_*.sql"
  # shellcheck disable=SC2012
  for mig in $(ls -1 "$APP_HOME/database"/migrate_*.sql 2>/dev/null | sort); do
    apply_sql_file "$mig"
  done

  psql_app -c "CREATE TABLE IF NOT EXISTS public.${marker_table} (applied_at timestamptz NOT NULL DEFAULT now());"
  psql_app -c "INSERT INTO public.${marker_table} DEFAULT VALUES;"
  log "Database bootstrap complete"
}

# ---------------------------------------------------------------------------
# Admin seed via existing scripts/seed_admin.php (unchanged).
# ---------------------------------------------------------------------------
seed_admin() {
  if [[ -z "${JRS_ADMIN_PASSWORD:-}" ]]; then
    log "JRS_ADMIN_PASSWORD not set — skipping admin seed"
    return 0
  fi
  log "Seeding administrator via scripts/seed_admin.php"
  (
    cd "$APP_HOME"
    export JRS_DB_HOST JRS_DB_PORT JRS_DB_NAME JRS_DB_USER JRS_DB_PASS JRS_DB_SCHEMA
    export JRS_ADMIN_USER="${JRS_ADMIN_USER:-admin}"
    export JRS_ADMIN_PASSWORD
    php scripts/seed_admin.php
  )
}

# ---------------------------------------------------------------------------
# Optional Docker demo fixtures (packaging only — not application code).
# ---------------------------------------------------------------------------
# Upsert jdbc/DemoPG credentials into context.xml without wiping other Resources.
# (Previously rewrote the whole file on every boot, which dropped user datasources.)
write_demo_context_xml() {
  local ctx="$CATALINA_BASE/conf/context.xml"
  local pass="${JRS_DB_PASS:-}"
  local user="${JRS_DB_USER:-jasper}"
  local db="${JRS_DB_NAME:-jasper_reports}"
  local host="${JRS_DB_HOST:-postgres}"
  local port="${JRS_DB_PORT:-5432}"
  mkdir -p "$(dirname "$ctx")" "$CONF_PERSIST"
  php -r '
$path = $argv[1];
$host = $argv[2];
$port = $argv[3];
$db = $argv[4];
$user = $argv[5];
$pass = $argv[6];
$esc = static function (string $s): string {
    return htmlspecialchars($s, ENT_XML1 | ENT_QUOTES, "UTF-8");
};
$url = "jdbc:postgresql://" . $host . ":" . $port . "/" . $db;
$resource =
    "    <Resource\n" .
    "            name=\"jdbc/DemoPG\"\n" .
    "            auth=\"Container\"\n" .
    "            type=\"javax.sql.DataSource\"\n" .
    "            factory=\"org.apache.tomcat.jdbc.pool.DataSourceFactory\"\n" .
    "            driverClassName=\"org.postgresql.Driver\"\n" .
    "            url=\"" . $esc($url) . "\"\n" .
    "            username=\"" . $esc($user) . "\"\n" .
    "            password=\"" . $esc($pass) . "\"\n" .
    "            initialSize=\"0\"\n" .
    "            maxTotal=\"20\"\n" .
    "            maxIdle=\"10\"\n" .
    "            maxWaitMillis=\"10000\"\n" .
    "            validationQuery=\"SELECT 1\"\n" .
    "            testOnBorrow=\"true\"/>";
if (!is_file($path) || trim((string) file_get_contents($path)) === "") {
    $xml = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"
        . "<Context>\n"
        . "    <WatchedResource>WEB-INF/web.xml</WatchedResource>\n"
        . $resource . "\n"
        . "</Context>\n";
    file_put_contents($path, $xml);
    exit(0);
}
$xml = (string) file_get_contents($path);
$xml = preg_replace(
    "/\\s*<Resource\\b(?=[^>]*\\bname=\"jdbc\\/DemoPG\")[^>]*\\/>/s",
    "",
    $xml
);
$xml = preg_replace(
    "/\\s*<Resource\\b(?=[^>]*\\bname=\"jdbc\\/DemoPG\")[^>]*>.*?<\\/Resource>/s",
    "",
    $xml
);
if (!preg_match("/<\\/Context>/i", $xml)) {
    fwrite(STDERR, "context.xml missing </Context>; refusing to rewrite\n");
    exit(1);
}
$xml = preg_replace("/<\\/Context>/i", $resource . "\n</Context>", $xml, 1);
file_put_contents($path, $xml);
' "$ctx" "$host" "$port" "$db" "$user" "$pass"
  cp -a "$ctx" "$CONF_PERSIST/context.xml"
}

seed_demo_content() {
  if [[ "$SEED_DEMO" != "1" && "$SEED_DEMO" != "true" ]]; then
    return 0
  fi
  if [[ -f "$APP_HOME/.acugis-docker-demo-seeded" ]]; then
    # Keep DemoPG password in sync with current env on every boot.
    write_demo_context_xml
    PGPASSWORD="${JRS_DB_PASS:-}" psql \
      -h "${JRS_DB_HOST:-postgres}" \
      -p "${JRS_DB_PORT:-5432}" \
      -U "${JRS_DB_USER:-jasper}" \
      -d "${JRS_DB_NAME:-jasper_reports}" \
      -c "UPDATE public.jasper_datasource SET password = '${JRS_DB_PASS:-}', username = '${JRS_DB_USER:-jasper}', host = '${JRS_DB_HOST:-postgres}' WHERE name = 'DemoPG';" \
      >/dev/null 2>&1 || true
    return 0
  fi

  log "Seeding Docker demo report fixtures"
  mkdir -p "$CATALINA_BASE/acugis/reports" "$APP_HOME/reports/json"

  if [[ -d "$APP_HOME/docker-fixtures/jrxml" ]]; then
    cp -a "$APP_HOME/docker-fixtures/jrxml/." "$CATALINA_BASE/acugis/reports/"
  fi
  if [[ -d "$APP_HOME/docker-fixtures/reports-json" ]]; then
    cp -a "$APP_HOME/docker-fixtures/reports-json/." "$APP_HOME/reports/json/"
  fi

  if [[ -f "$APP_HOME/docker-fixtures/sql/demo_seed.sql" ]]; then
    # Password comes from the environment. The SQL file has no default.
    psql_app -v jrs_db_pass="${JRS_DB_PASS:-}" -f "$APP_HOME/docker-fixtures/sql/demo_seed.sql" \
      || log "WARNING: demo SQL seed had errors"
  fi

  write_demo_context_xml

  # Optional DuckDB demo database under /data (packaging fixture; uses existing JDBC jar).
  if [[ ! -f /data/demo.duckdb ]]; then
    log "Creating /data/demo.duckdb via DuckDB JDBC"
    cat >/tmp/CreateDuckDemo.java <<'JAVA'
import java.sql.*;
public class CreateDuckDemo {
  public static void main(String[] args) throws Exception {
    Class.forName("org.duckdb.DuckDBDriver");
    try (Connection c = DriverManager.getConnection("jdbc:duckdb:/data/demo.duckdb");
         Statement s = c.createStatement()) {
      s.execute("CREATE TABLE IF NOT EXISTS items(id INTEGER, name VARCHAR, quantity INTEGER)");
      s.execute("DELETE FROM items");
      s.execute("INSERT INTO items VALUES (1,'DuckAlpha',11),(2,'DuckBeta',22)");
    }
  }
}
JAVA
    if javac -cp "/usr/local/tomcat/lib/duckdb_jdbc-1.5.5.1-linux_amd64.jar" -d /tmp /tmp/CreateDuckDemo.java \
      && java -cp "/tmp:/usr/local/tomcat/lib/duckdb_jdbc-1.5.5.1-linux_amd64.jar" CreateDuckDemo; then
      psql_app <<SQL || log "WARNING: DuckDB datasource catalog insert failed"
ALTER TABLE public.jasper_datasource ADD COLUMN IF NOT EXISTS db_type varchar(20) NOT NULL DEFAULT 'postgresql';
INSERT INTO public.jasper_datasource (name, host, port, dbname, username, password, schema_name, db_type, enabled)
VALUES ('DemoDuckDB', '', 0, '/data/demo.duckdb', '', '', 'main', 'duckdb', true)
ON CONFLICT (name) DO UPDATE SET dbname = EXCLUDED.dbname, db_type = 'duckdb', enabled = true;
SQL
      # Minimal DuckDB report JRXML + JSON (optional; execute via Go gateway with _duckdbSession).
      if [[ -f "$APP_HOME/docker-fixtures/jrxml/hello_duckdb.jrxml" ]]; then
        cp -a "$APP_HOME/docker-fixtures/jrxml/hello_duckdb.jrxml" "$CATALINA_BASE/acugis/reports/"
      fi
      if [[ -f "$APP_HOME/docker-fixtures/reports-json/hello_duckdb.report.json" ]]; then
        cp -a "$APP_HOME/docker-fixtures/reports-json/hello_duckdb.report.json" "$APP_HOME/reports/json/"
      fi
    else
      log "WARNING: DuckDB demo file creation failed — DuckDB demo skipped"
    fi
  fi

  touch "$APP_HOME/.acugis-docker-demo-seeded"
}

# ---------------------------------------------------------------------------
# Tomcat lifecycle
# ---------------------------------------------------------------------------
TOMCAT_PID=""
GO_PID=""

# Optional customer JDBC jars (IBM Db2, or Oracle when the image does not include it).
# The drivers directory is not on Tomcat's classpath; copy jars into shared lib.
install_customer_jdbc() {
  local src="${JRS_CUSTOM_JDBC_DIR:-$CATALINA_BASE/acugis/drivers}"
  local jar base
  [[ -d "$src" ]] || return 0
  shopt -s nullglob
  for jar in "$src"/*.jar; do
    base="$(basename "$jar")"
    cp -f "$jar" "$CATALINA_HOME/lib/$base"
    log "Installed customer JDBC driver ${base}"
  done
  shopt -u nullglob
}

start_tomcat() {
  log "Starting Tomcat 10…"
  # shellcheck disable=SC2086
  "$CATALINA_HOME/bin/catalina.sh" run &
  TOMCAT_PID=$!
  local i
  for i in $(seq 1 90); do
    if bash -c "echo >/dev/tcp/127.0.0.1/8080" >/dev/null 2>&1; then
      log "Tomcat is accepting connections on :8080"
      return 0
    fi
    if ! kill -0 "$TOMCAT_PID" 2>/dev/null; then
      log "ERROR: Tomcat exited before becoming ready"
      return 1
    fi
    sleep 2
  done
  log "ERROR: Tomcat did not open :8080 in time"
  return 1
}

shutdown_all() {
  log "Shutdown signal received"
  persist_tomcat_conf || true
  if [[ -n "${GO_PID}" ]] && kill -0 "$GO_PID" 2>/dev/null; then
    kill -TERM "$GO_PID" 2>/dev/null || true
    wait "$GO_PID" 2>/dev/null || true
  fi
  if [[ -n "${TOMCAT_PID}" ]] && kill -0 "$TOMCAT_PID" 2>/dev/null; then
    "$CATALINA_HOME/bin/catalina.sh" stop 20 || kill -TERM "$TOMCAT_PID" 2>/dev/null || true
    wait "$TOMCAT_PID" 2>/dev/null || true
  fi
  persist_tomcat_conf || true
  log "Shutdown complete"
  exit 0
}

wait_for_jasper() {
  local url="${JRS_JASPER_INTERNAL_REPORT_URL:-http://127.0.0.1:8080/report}"
  local rest="${url#http://}"
  rest="${rest#https://}"
  local hostport="${rest%%/*}"
  local host="${hostport%%:*}"
  local port="${hostport##*:}"
  if [[ -z "$host" ]]; then
    host="127.0.0.1"
  fi
  if [[ -z "$port" || "$port" == "$host" ]]; then
    if [[ "$url" == https://* ]]; then
      port="443"
    else
      port="8080"
    fi
  fi
  log "Waiting for Jasper execute endpoint at ${host}:${port}…"
  local i
  for i in $(seq 1 90); do
    if bash -c "echo >/dev/tcp/${host}/${port}" >/dev/null 2>&1; then
      log "Jasper is reachable at ${host}:${port}"
      return 0
    fi
    sleep 2
  done
  log "ERROR: Jasper not reachable at ${host}:${port}"
  return 1
}

is_scheduler_role() {
  [[ "${ACUGIS_DOCKER_ROLE:-}" == "scheduler" ]] || [[ "${1:-}" == "jrs-schedule-dispatcher" ]]
}

run_scheduler() {
  mkdir -p \
    "$APP_HOME/schedules/output" \
    "$APP_HOME/schedules/logs" \
    "$APP_HOME/email_tmpl"
  wait_for_postgres
  wait_for_jasper
  log "Starting jrs-schedule-dispatcher (no Tomcat in this container)…"
  if [[ $# -eq 0 || "$1" == "jrs-server" || "$1" == "jrs-schedule-dispatcher" ]]; then
    exec /usr/local/bin/jrs-schedule-dispatcher
  fi
  exec "$@"
}

trap shutdown_all SIGTERM SIGINT

# ---------------------------------------------------------------------------
# Customer credentials come from .env. Refuse the public examples, and refuse
# empty values, before PostgreSQL work or administrator seeding. report-server
# and report-scheduler both reach this check. The message is the same because
# they share .env. Password values are not printed.
# ---------------------------------------------------------------------------
normalize_credential() {
  local value="$1"
  value="${value//$'\r'/}"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  if [[ ${#value} -ge 2 ]]; then
    if [[ "${value:0:1}" == '"' && "${value: -1}" == '"' ]]; then
      value="${value:1:${#value}-2}"
    elif [[ "${value:0:1}" == "'" && "${value: -1}" == "'" ]]; then
      value="${value:1:${#value}-2}"
    fi
  fi
  printf '%s' "$value"
}

reject_default_credentials() {
  local db admin
  db="$(normalize_credential "${JRS_DB_PASS:-}")"
  admin="$(normalize_credential "${JRS_ADMIN_PASSWORD:-}")"
  if [[ "$db" == "change-me-strong-password" || "$admin" == "change-me-admin-password" ]]; then
    cat >&2 <<'EOF'
ERROR: Default Report Server credentials detected.

Edit .env and replace the example values for:

  JRS_DB_PASS
  JRS_ADMIN_PASSWORD

Then run:

  docker compose up -d
EOF
    exit 1
  fi
  if [[ -z "$db" || -z "$admin" ]]; then
    cat >&2 <<'EOF'
ERROR: Report Server credentials are missing.

Edit .env and set strong, unique values for:

  JRS_DB_PASS
  JRS_ADMIN_PASSWORD

Then run:

  docker compose up -d
EOF
    exit 1
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
reject_default_credentials

if is_scheduler_role "${1:-}"; then
  run_scheduler "$@"
fi

seed_app_home
mkdir -p "$APP_HOME/var/license"
sync_ui_assets
sync_tomcat_conf
wait_for_postgres
bootstrap_database
seed_admin
seed_demo_content
persist_tomcat_conf
install_customer_jdbc

start_tomcat

log "Starting jrs-server…"
if [[ $# -eq 0 || "$1" == "jrs-server" ]]; then
  /usr/local/bin/jrs-server &
else
  "$@" &
fi
GO_PID=$!

# Persist conf periodically is not needed; flush on shutdown.
wait "$GO_PID"
EXIT_CODE=$?
persist_tomcat_conf || true
if [[ -n "${TOMCAT_PID}" ]] && kill -0 "$TOMCAT_PID" 2>/dev/null; then
  "$CATALINA_HOME/bin/catalina.sh" stop 20 || true
  wait "$TOMCAT_PID" 2>/dev/null || true
fi
exit "$EXIT_CODE"
