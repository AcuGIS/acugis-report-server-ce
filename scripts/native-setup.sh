#!/bin/bash
# Shared native setup helpers for the tarball installer and Debian maintainer scripts.
# Safe to source. Does not install package files (rsync/dpkg already did that).

# Caller may pre-set SCRIPT_DIR (product root: RELEASE, bin/, tomcat/, …).
if [[ -z "${SCRIPT_DIR:-}" ]]; then
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fi

: "${BASE:=${JRS_INSTALL_DIR:-/opt/acugis-report-server}}"
: "${CONF:=${JRS_APACHE_CONF:-/etc/apache2/conf-available/acugis-report-server.conf}}"
: "${TOMCAT_HOME:=${JRS_TOMCAT_HOME:-/opt/tomcat}}"
: "${TOMCAT_VERSION:=${JRS_TOMCAT_VERSION:-10.1.57}}"
: "${ENV_FILE:=${JRS_SYSTEMD_ENV_FILE:-/etc/acugis-report-server/env}}"
: "${CREDENTIALS_FILE:=${JRS_CREDENTIALS_FILE:-/etc/acugis-report-server/initial-credentials}}"
: "${SERVICE_USER:=${JRS_SERVICE_USER:-www-data}}"
: "${TOMCAT_USER:=${JRS_TOMCAT_USER:-tomcat}}"
: "${EDITION_STATE:=${JRS_EDITION_STATE:-/var/lib/acugis-report-server/edition}}"
: "${SUDOERS_FILE:=${JRS_SUDOERS_FILE:-/etc/sudoers.d/acugis-report-server}}"
ENV_CREATED=0

log() { echo "=== $*"; }
warn() { echo "WARNING: $*" >&2; }
die() { echo "ERROR: $*" >&2; exit 1; }

have_cmd() { command -v "$1" >/dev/null 2>&1; }

read_kv() {
  local file="$1" key="$2"
  [[ -f "$file" ]] || { echo ""; return 0; }
  grep -E "^${key}=" "$file" 2>/dev/null | tail -n1 | cut -d= -f2- || true
}

package_edition() { read_kv "$SCRIPT_DIR/RELEASE" edition; }
package_version() { read_kv "$SCRIPT_DIR/RELEASE" version; }

installed_edition() {
  if [[ -f "$BASE/RELEASE" ]]; then
    read_kv "$BASE/RELEASE" edition
    return 0
  fi
  if [[ -f "$EDITION_STATE" ]]; then
    tr -d '[:space:]' < "$EDITION_STATE"
    return 0
  fi
  if [[ -f "$ENV_FILE" ]]; then
    local e
    e="$(read_kv "$ENV_FILE" ACUGIS_EDITION)"
    e="$(echo "$e" | tr '[:upper:]' '[:lower:]')"
    case "$e" in
      professional|pro|commercial) echo professional ; return 0 ;;
      community|ce|oss) echo oss ; return 0 ;;
    esac
  fi
  echo ""
}

record_edition_state() {
  local ed dir
  ed="$(package_edition)"
  [[ -n "$ed" ]] || return 0
  dir="$(dirname "$EDITION_STATE")"
  mkdir -p "$dir"
  echo "$ed" > "$EDITION_STATE"
  # www-data creates installation-id and license.lic. Do not replace those files.
  # A root-owned 0755 directory makes Load() fail and leaves the install without an ID.
  if id -u "$SERVICE_USER" >/dev/null 2>&1; then
    chown "$SERVICE_USER:$SERVICE_USER" "$dir" || true
  fi
  chmod 0750 "$dir" || true
}

# Refuse Professional → OSS. Used by the tarball installer and OSS Debian preinst/postinst.
guard_edition() {
  local pkg inst
  pkg="$(package_edition)"
  inst="$(installed_edition)"
  if [[ -n "$inst" && "$inst" == "professional" && "$pkg" == "oss" ]]; then
    die "Refusing Professional → OSS. Professional database objects/data may exist. Install Professional, or install OSS on a new directory/database."
  fi
}

ensure_users() {
  if ! id -u "$SERVICE_USER" >/dev/null 2>&1; then
    useradd --system --home-dir "$BASE" --shell /usr/sbin/nologin "$SERVICE_USER" || true
  fi
  if ! id -u "$TOMCAT_USER" >/dev/null 2>&1; then
    useradd --system --home-dir "$TOMCAT_HOME" --shell /usr/sbin/nologin "$TOMCAT_USER" || true
  fi
}

# Tomcat 10.1.x is downloaded to /opt/tomcat (not a Debian Depends). Existing trees are reused.
install_tomcat() {
  [[ "${JRS_SKIP_TOMCAT:-0}" == "1" ]] && return 0
  if [[ -x "$TOMCAT_HOME/bin/catalina.sh" ]]; then
    log "Using existing Tomcat at $TOMCAT_HOME"
    return 0
  fi
  log "Installing Apache Tomcat ${TOMCAT_VERSION} to $TOMCAT_HOME"
  local tmp tarball url
  tmp="$(mktemp -d)"
  tarball="$tmp/tomcat.tar.gz"
  url="https://archive.apache.org/dist/tomcat/tomcat-10/v${TOMCAT_VERSION}/bin/apache-tomcat-${TOMCAT_VERSION}.tar.gz"
  if ! curl -fsSL "$url" -o "$tarball"; then
    if ! curl -fsSL "https://dlcdn.apache.org/tomcat/tomcat-10/v${TOMCAT_VERSION}/bin/apache-tomcat-${TOMCAT_VERSION}.tar.gz" -o "$tarball"; then
      die "Failed to download Apache Tomcat ${TOMCAT_VERSION}. Check network access to archive.apache.org, or set JRS_TOMCAT_HOME to an existing Tomcat 10.1 (JRS_SKIP_TOMCAT=1 to skip)."
    fi
  fi
  if [[ ! -s "$tarball" ]]; then
    die "Downloaded Tomcat archive is empty (${TOMCAT_VERSION})"
  fi
  mkdir -p "$(dirname "$TOMCAT_HOME")"
  tar -xzf "$tarball" -C "$tmp"
  mv "$tmp/apache-tomcat-${TOMCAT_VERSION}" "$TOMCAT_HOME"
  rm -rf "$tmp"
  rm -rf "$TOMCAT_HOME/webapps/"*
}

# Deploy WAR/JDBC into Tomcat. Preserves context.xml, acugis.properties, and map connection files.
install_jasper_runtime() {
  [[ "${JRS_SKIP_TOMCAT:-0}" == "1" ]] && return 0
  log "Installing WAR and JDBC jars into $TOMCAT_HOME"
  mkdir -p \
    "$TOMCAT_HOME/webapps" \
    "$TOMCAT_HOME/lib" \
    "$TOMCAT_HOME/conf" \
    "$TOMCAT_HOME/acugis/reports" \
    "$TOMCAT_HOME/acugis/uploads" \
    "$TOMCAT_HOME/acugis/fonts" \
    "$TOMCAT_HOME/acugis/drivers" \
    "$TOMCAT_HOME/acugis/licenses" \
    "$TOMCAT_HOME/acugis/temp" \
    "$TOMCAT_HOME/acugis/resource-paths"

  if have_cmd systemctl && systemctl is-active --quiet acugis-tomcat 2>/dev/null; then
    systemctl stop acugis-tomcat || true
  fi
  rm -rf "$TOMCAT_HOME/webapps/ROOT"
  cp -a "$SCRIPT_DIR/tomcat/acugis-report-server.war" "$TOMCAT_HOME/webapps/ROOT.war"
  cp -a "$SCRIPT_DIR/tomcat/lib/"*.jar "$TOMCAT_HOME/lib/"

  if [[ ! -f "$TOMCAT_HOME/conf/acugis.properties" && -f "$SCRIPT_DIR/tomcat/sample-acugis.properties" ]]; then
    cp -a "$SCRIPT_DIR/tomcat/sample-acugis.properties" "$TOMCAT_HOME/conf/acugis.properties"
  fi
  if [[ ! -f "$TOMCAT_HOME/conf/context.xml" && -f "$SCRIPT_DIR/tomcat/context.xml.example" ]]; then
    cp -a "$SCRIPT_DIR/tomcat/context.xml.example" "$TOMCAT_HOME/conf/context.xml"
  fi
  local props
  for props in geoserver-connections.properties qgis-connections.properties arcgis-connections.properties; do
    if [[ ! -f "$TOMCAT_HOME/conf/$props" ]]; then
      if [[ -f "$SCRIPT_DIR/docker/report-server/$props" ]]; then
        cp -a "$SCRIPT_DIR/docker/report-server/$props" "$TOMCAT_HOME/conf/$props"
      else
        : > "$TOMCAT_HOME/conf/$props"
      fi
    fi
  done
}

# Administrator-owned. Created on first install; upgrades never replace it.
# ACUGIS_EDITION is not entitlement. installation-id and license.lic under
# /var/lib/acugis-report-server are created by the server and must not be
# overwritten here. JRS_APP_VERSION is refreshed.
write_env_file() {
  mkdir -p "$(dirname "$ENV_FILE")"
  local pkg_ver
  pkg_ver="$(package_version)"

  if [[ -f "$ENV_FILE" ]]; then
    ENV_CREATED=0
    log "Preserving existing $ENV_FILE"
    if grep -q '^JRS_APP_VERSION=' "$ENV_FILE"; then
      sed -i "s/^JRS_APP_VERSION=.*/JRS_APP_VERSION=${pkg_ver}/" "$ENV_FILE"
    else
      echo "JRS_APP_VERSION=${pkg_ver}" >> "$ENV_FILE"
    fi
    ensure_schedule_sudo_env
    return 0
  fi

  ENV_CREATED=1
  log "Writing $ENV_FILE"
  cat > "$ENV_FILE" <<EOF
JRS_ENVIRONMENT=production
JRS_APP_VERSION=${pkg_ver}
JRS_LISTEN_ADDR=${JRS_LISTEN_ADDR:-127.0.0.1:9088}
JRS_BASE_PATH=${JRS_BASE_PATH:-/}
JRS_JASPER_HOME=${BASE}
JRS_DB_HOST=${JRS_DB_HOST:-localhost}
JRS_DB_PORT=${JRS_DB_PORT:-5432}
JRS_DB_NAME=${JRS_DB_NAME:-jasper_reports}
JRS_DB_USER=${JRS_DB_USER:-jasper}
JRS_DB_PASS=${JRS_DB_PASS:-}
JRS_DB_SCHEMA=${JRS_DB_SCHEMA:-public}
JRS_DB_SSLMODE=${JRS_DB_SSLMODE:-disable}
CATALINA_HOME=${TOMCAT_HOME}
CATALINA_BASE=${TOMCAT_HOME}
JRS_JRXML_DIR=${TOMCAT_HOME}/acugis/reports
JRS_JASPER_CONTEXT_PATH=${TOMCAT_HOME}/conf/context.xml
JRS_JASPER_REPORTS_CONTAINER_DIR=${TOMCAT_HOME}/acugis/reports
JRS_RESOURCE_PATHS_DIR=${TOMCAT_HOME}/acugis/resource-paths
JRS_GEOSERVER_CONNECTIONS_PATH=${TOMCAT_HOME}/conf/geoserver-connections.properties
JRS_QGIS_CONNECTIONS_PATH=${TOMCAT_HOME}/conf/qgis-connections.properties
JRS_ARCGIS_CONNECTIONS_PATH=${TOMCAT_HOME}/conf/arcgis-connections.properties
JRS_JASPER_INTERNAL_REPORT_URL=${JRS_JASPER_INTERNAL_REPORT_URL:-http://127.0.0.1:8080/report}
JRS_JASPER_INTERNAL_IMAGE_URL=${JRS_JASPER_INTERNAL_IMAGE_URL:-http://127.0.0.1:8080/acugis_report_image}
JRS_REPORTS_OWNER=${TOMCAT_USER}
JRS_REPORTS_GROUP=${TOMCAT_USER}
JRS_JASPER_SYSTEMD_UNIT_DIR=${BASE}/systemd
JRS_JASPER_SYSTEMD_USE_SUDO=true
EOF
  chmod 640 "$ENV_FILE"
  chown "root:${SERVICE_USER}" "$ENV_FILE" 2>/dev/null || true
  if [[ ! -f "$BASE/.env" ]]; then
    sync_install_dotenv
  fi
}

load_env() {
  [[ -f "$ENV_FILE" ]] || return 0
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
}

db_credentials_present() {
  [[ -n "${JRS_DB_PASS:-}" ]]
}

# Alphanumeric secret safe for SQL string literals and systemd EnvironmentFile.
gen_secret() {
  local s=""
  if have_cmd openssl; then
    s="$(openssl rand -hex 12 2>/dev/null || true)"
  fi
  if [[ ${#s} -lt 24 ]] && have_cmd python3; then
    s="$(python3 -c 'import secrets; print(secrets.token_hex(12))' 2>/dev/null || true)"
  fi
  [[ ${#s} -ge 24 ]] || die "Could not generate a random secret (need openssl or python3)"
  printf '%s' "${s:0:24}"
}

# 32-byte signing key. Not the short SQL-safe secret used for database passwords.
gen_embed_secret() {
  local s=""
  if have_cmd openssl; then
    s="$(openssl rand -hex 32 2>/dev/null || true)"
  fi
  if [[ ${#s} -ne 64 ]] && have_cmd python3; then
    s="$(python3 -c 'import secrets; print(secrets.token_hex(32))' 2>/dev/null || true)"
  fi
  [[ ${#s} -eq 64 && "$s" =~ ^[0-9a-fA-F]+$ ]] || die "Could not generate ACUGIS_EMBED_TOKEN_SECRET (need openssl or python3)"
  printf '%s' "$s"
}

# A signing secret already stored for this installation. Either name is enough.
embed_secret_in_file() {
  local file="$1" v
  v="$(read_kv "$file" ACUGIS_EMBED_TOKEN_SECRET)"
  if [[ -n "$v" ]]; then
    printf '%s' "$v"
    return 0
  fi
  printf '%s' "$(read_kv "$file" JRS_EMBED_TOKEN_SECRET)"
}

# Assignment lines that must never be copied into $BASE/.env.
# systemd EnvironmentFile and the Go/PHP dotenv loaders both read ENV_FILE
# (/etc/acugis-report-server/env, mode 640, root:www-data). The install-tree
# mirror is only a hand-start copy of the other settings. fix_permissions
# applies chmod -R 755 to $BASE, so a signing secret placed there becomes
# world-readable.
embed_secret_line_re='^(export[[:space:]]+)?(ACUGIS_EMBED_TOKEN_SECRET|JRS_EMBED_TOKEN_SECRET)='

# Drop embed signing keys from an existing install-tree mirror (upgrade).
strip_install_dotenv_embed_secrets() {
  [[ -f "$BASE/.env" ]] || return 0
  if ! grep -qE "$embed_secret_line_re" "$BASE/.env"; then
    return 0
  fi
  local tmp
  tmp="$(mktemp)"
  grep -E -v "$embed_secret_line_re" "$BASE/.env" > "$tmp" || true
  cat "$tmp" > "$BASE/.env"
  rm -f "$tmp"
}

# The mirror still holds database and admin passwords. Keep it readable by
# the service group and not by everyone else. chmod -R 755 in fix_permissions
# would otherwise reopen it.
protect_install_dotenv() {
  [[ -f "$BASE/.env" ]] || return 0
  strip_install_dotenv_embed_secrets
  if id -u "$SERVICE_USER" >/dev/null 2>&1; then
    chown "root:${SERVICE_USER}" "$BASE/.env" 2>/dev/null || true
  fi
  chmod 640 "$BASE/.env" 2>/dev/null || true
}

# Rewrite $BASE/.env from ENV_FILE without embed signing keys.
sync_install_dotenv() {
  [[ -f "$ENV_FILE" ]] || return 0
  mkdir -p "$BASE"
  local tmp
  tmp="$(mktemp)"
  grep -E -v "$embed_secret_line_re" "$ENV_FILE" > "$tmp" || true
  cat "$tmp" > "$BASE/.env"
  rm -f "$tmp"
  protect_install_dotenv
}

upsert_env_key() {
  local file="$1" key="$2" val="$3"
  mkdir -p "$(dirname "$file")"
  touch "$file"
  local tmp
  tmp="$(mktemp)"
  awk -v k="$key" -v v="$val" '
    BEGIN { done = 0 }
    $0 ~ "^" k "=" { print k "=" v; done = 1; next }
    { print }
    END { if (!done) print k "=" v }
  ' "$file" > "$tmp"
  mv "$tmp" "$file"
}

# Native .deb runs jrs-server as www-data. Direct os.Symlink into
# /etc/systemd/system always fails; sudo -n is required. Coerce a leftover
# false from older packages so EnvironmentFile matches that behavior.
ensure_schedule_sudo_env() {
  [[ -f "$ENV_FILE" ]] || return 0
  local cur
  cur="$(read_kv "$ENV_FILE" JRS_JASPER_SYSTEMD_USE_SUDO)"
  case "$(echo "$cur" | tr '[:upper:]' '[:lower:]')" in
    false|0|no|off|"")
      set_kv "$ENV_FILE" JRS_JASPER_SYSTEMD_USE_SUDO true
      ;;
  esac
}

# Permit the Report Server user to install/remove dynamic jrs-schedule-* units.
# Unit files are written under $BASE/systemd (service-user writable). Privileged
# ln/rm/systemctl run via sudo -n, matching lib/schedule_systemd.php.
# Do not make /etc/systemd/system writable and do not run jrs-server as root.
install_schedule_sudoers() {
  local dest="${SUDOERS_FILE:-/etc/sudoers.d/acugis-report-server}"
  local user="$SERVICE_USER"

  log "Installing schedule sudoers for ${user} at ${dest}"
  mkdir -p "$(dirname "$dest")"
  local tmp
  tmp="$(mktemp)"
  {
    echo "# AcuGIS Report Server — dynamic jrs-schedule-* unit install"
    echo "# Generated by native-setup. Static acugis-*.service units are packaged separately."
    echo
    local bin
    for bin in /usr/bin/ln /bin/ln; do
      echo "${user} ALL=(root) NOPASSWD: ${bin}"
    done
    for bin in /usr/bin/rm /bin/rm; do
      echo "${user} ALL=(root) NOPASSWD: ${bin}"
    done
    for bin in /usr/bin/test /bin/test; do
      echo "${user} ALL=(root) NOPASSWD: ${bin}"
    done
    echo "${user} ALL=(root) NOPASSWD: /bin/systemctl"
  } > "$tmp"

  if have_cmd install; then
    install -m 0440 -o root -g root "$tmp" "$dest"
  else
    cp "$tmp" "$dest"
    chmod 0440 "$dest"
    chown root:root "$dest" 2>/dev/null || true
  fi
  rm -f "$tmp"
  if have_cmd visudo; then
    if ! visudo -c >/dev/null; then
      rm -f "$dest"
      die "schedule sudoers $dest failed visudo -c; file was not left in place"
    fi
  fi
}

sql_ident_ok() {
  [[ "$1" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]
}

sql_quote() {
  local s="$1"
  s="${s//\'/\'\'}"
  printf "'%s'" "$s"
}

db_host_is_local() {
  local host="${1:-${JRS_DB_HOST:-localhost}}"
  [[ "$host" == "localhost" || "$host" == "127.0.0.1" || "$host" == "::1" ]]
}

run_postgres() {
  if [[ "$(id -u)" -eq 0 ]]; then
    if have_cmd runuser; then
      runuser -u postgres -- "$@"
      return
    fi
    su -s /bin/sh postgres -c "$(printf '%q ' "$@")"
    return
  fi
  if have_cmd sudo; then
    sudo -u postgres "$@"
    return
  fi
  die "Cannot run commands as postgres (need root or sudo)"
}

wait_for_local_postgres() {
  local host="${JRS_DB_HOST:-localhost}" port="${JRS_DB_PORT:-5432}" i=0
  have_cmd pg_isready || die "pg_isready is missing (install postgresql-client)"
  while [[ "$i" -lt 30 ]]; do
    if pg_isready -h "$host" -p "$port" >/dev/null 2>&1; then
      return 0
    fi
    i=$((i + 1))
    sleep 1
  done
  die "PostgreSQL is not ready on ${host}:${port}. Start the postgresql service, or set JRS_DB_HOST to a reachable server."
}

write_initial_credentials() {
  [[ -f "$CREDENTIALS_FILE" ]] && return 0
  mkdir -p "$(dirname "$CREDENTIALS_FILE")"
  cat > "$CREDENTIALS_FILE" <<EOF
# Generated on first install. Not updated on upgrade. Mode 600.
# Store these secrets, then you may delete this file.
JRS_ADMIN_USER=${JRS_ADMIN_USER:-admin}
JRS_ADMIN_PASSWORD=${JRS_ADMIN_PASSWORD:-}
JRS_DB_HOST=${JRS_DB_HOST:-localhost}
JRS_DB_PORT=${JRS_DB_PORT:-5432}
JRS_DB_NAME=${JRS_DB_NAME:-jasper_reports}
JRS_DB_USER=${JRS_DB_USER:-jasper}
JRS_DB_PASS=${JRS_DB_PASS:-}
JRS_URL=http://127.0.0.1:9088/
EOF
  chmod 600 "$CREDENTIALS_FILE"
  chown root:root "$CREDENTIALS_FILE" 2>/dev/null || true
}

# Fill empty JRS_DB_PASS / JRS_ADMIN_PASSWORD on first boot so apt install can finish.
# Never rotates secrets on upgrade. Remote PostgreSQL still requires JRS_DB_PASS.
ensure_first_boot_credentials() {
  [[ -f "$ENV_FILE" ]] || write_env_file
  local db_pass admin_pass admin_user host generated_db=0 generated_admin=0
  db_pass="${JRS_DB_PASS:-$(read_kv "$ENV_FILE" JRS_DB_PASS)}"
  admin_pass="${JRS_ADMIN_PASSWORD:-$(read_kv "$ENV_FILE" JRS_ADMIN_PASSWORD)}"
  admin_user="${JRS_ADMIN_USER:-$(read_kv "$ENV_FILE" JRS_ADMIN_USER)}"
  host="${JRS_DB_HOST:-$(read_kv "$ENV_FILE" JRS_DB_HOST)}"
  host="${host:-localhost}"
  admin_user="${admin_user:-admin}"

  if [[ -z "$db_pass" && -f "$CREDENTIALS_FILE" ]]; then
    db_pass="$(read_kv "$CREDENTIALS_FILE" JRS_DB_PASS)"
  fi
  if [[ -z "$admin_pass" && -f "$CREDENTIALS_FILE" ]]; then
    admin_pass="$(read_kv "$CREDENTIALS_FILE" JRS_ADMIN_PASSWORD)"
  fi

  if [[ -z "$db_pass" ]]; then
    if [[ "${JRS_SKIP_DB:-0}" != "1" ]] && ! db_host_is_local "$host"; then
      die "JRS_DB_PASS is required when JRS_DB_HOST=${host}. Write it in $ENV_FILE and re-run $BASE/scripts/configure-instance.sh"
    fi
    log "Generating JRS_DB_PASS (first boot)"
    db_pass="$(gen_secret)"
    generated_db=1
  fi
  if [[ -z "$admin_pass" && ( "$generated_db" == "1" || "$ENV_CREATED" == "1" ) ]]; then
    log "Generating JRS_ADMIN_PASSWORD (first boot)"
    admin_pass="$(gen_secret)"
    generated_admin=1
  fi

  export JRS_DB_HOST="$host"
  export JRS_DB_PASS="$db_pass"
  export JRS_ADMIN_USER="$admin_user"
  [[ -n "$admin_pass" ]] && export JRS_ADMIN_PASSWORD="$admin_pass"

  upsert_env_key "$ENV_FILE" "JRS_DB_PASS" "$db_pass"
  upsert_env_key "$ENV_FILE" "JRS_ADMIN_USER" "$admin_user"
  if [[ -n "$admin_pass" ]]; then
    upsert_env_key "$ENV_FILE" "JRS_ADMIN_PASSWORD" "$admin_pass"
  fi
  chmod 640 "$ENV_FILE" 2>/dev/null || true
  chown "root:${SERVICE_USER}" "$ENV_FILE" 2>/dev/null || true
  ensure_embed_token_secret
  if [[ -f "$BASE/.env" ]]; then
    sync_install_dotenv
  fi

  if [[ "$generated_db" == "1" || "$generated_admin" == "1" ]]; then
    write_initial_credentials
  fi
}

# Fill ACUGIS_EMBED_TOKEN_SECRET when this installation does not already have
# one. A value already in the env file is kept, including across package
# upgrades. An administrator-supplied ACUGIS_EMBED_TOKEN_SECRET or
# JRS_EMBED_TOKEN_SECRET is written only when the file has neither key.
# The secret is not printed and is not copied to $BASE/.env.
ensure_embed_token_secret() {
  [[ -f "$ENV_FILE" ]] || write_env_file
  if [[ -z "$(embed_secret_in_file "$ENV_FILE")" ]]; then
    local supplied="${ACUGIS_EMBED_TOKEN_SECRET:-${JRS_EMBED_TOKEN_SECRET:-}}"
    if [[ -n "$supplied" ]]; then
      upsert_env_key "$ENV_FILE" "ACUGIS_EMBED_TOKEN_SECRET" "$supplied"
    else
      log "Generating ACUGIS_EMBED_TOKEN_SECRET"
      upsert_env_key "$ENV_FILE" "ACUGIS_EMBED_TOKEN_SECRET" "$(gen_embed_secret)"
    fi
    chmod 640 "$ENV_FILE" 2>/dev/null || true
    chown "root:${SERVICE_USER}" "$ENV_FILE" 2>/dev/null || true
  fi
  # Upgrade: an older package copied the signing secret into the mirror.
  if [[ -f "$BASE/.env" ]]; then
    sync_install_dotenv
  fi
}

print_instance_summary() {
  echo
  echo "Configured $(package_edition) $(package_version) at $BASE"
  echo "Config: $ENV_FILE (never replaced on upgrade)"
  echo "Embed:  ACUGIS_EMBED_TOKEN_SECRET in $ENV_FILE only (not in $BASE/.env; not printed)"
  echo "UI:     http://127.0.0.1:9088/"
  if [[ -f "$CREDENTIALS_FILE" ]]; then
    echo "Login:  $CREDENTIALS_FILE"
  fi
  echo "Docs:   $BASE/docs/INSTALL.md"
}

# Reverse proxy to Go on 127.0.0.1:9088. JRS_APACHE_OVERWRITE=1 (tarball default)
# replaces the snippet; Debian sets JRS_APACHE_OVERWRITE=0 so an admin-customized
# vhost is kept. JRS_SKIP_APACHE=1 skips this (tests / non-Apache hosts).
configure_apache() {
  [[ "${JRS_SKIP_APACHE:-0}" == "1" ]] && return 0
  have_cmd a2enmod || { warn "Apache not present; skip reverse proxy"; return 0; }
  log "Configuring Apache reverse proxy to Go on :9088"
  local src="$SCRIPT_DIR/config/apache-acugis-report-server.conf"
  if [[ -f "$CONF" && "${JRS_APACHE_OVERWRITE:-1}" != "1" ]]; then
    log "Preserving existing $CONF"
  elif [[ -f "$src" ]]; then
    mkdir -p "$(dirname "$CONF")"
    cp -a "$src" "$CONF"
  else
    mkdir -p "$(dirname "$CONF")"
    cat > "$CONF" <<'EOF'
# License origin checks trust X-Forwarded-Host, then Host.
# MCP also accepts public hostnames listed in JRS_MCP_ALLOWED_HOSTS.
ProxyPreserveHost On
ProxyAddHeaders On
RequestHeader set X-Forwarded-Proto "https" env=HTTPS
ProxyPass / http://127.0.0.1:9088/
ProxyPassReverse / http://127.0.0.1:9088/
EOF
  fi
  a2enmod proxy proxy_http headers rewrite >/dev/null 2>&1 || true
  a2enconf acugis-report-server >/dev/null 2>&1 || true
  if have_cmd systemctl; then
    systemctl enable apache2 >/dev/null 2>&1 || systemctl enable apache2.service >/dev/null 2>&1 || true
    systemctl reload apache2 2>/dev/null || systemctl restart apache2 2>/dev/null || \
      systemctl reload apache2.service 2>/dev/null || systemctl restart apache2.service 2>/dev/null || true
  fi
}

ensure_runtime_dirs() {
  mkdir -p \
    "$BASE/reports/json" \
    "$BASE/assets/reports" \
    "$BASE/schedules/output" \
    "$BASE/schedules/logs" \
    "$BASE/email_tmpl" \
    "$BASE/systemd" \
    "$BASE/shared/ui" \
    "$BASE/resources" \
    "$BASE/thumbnails"
}

# Files jrs-server (SERVICE_USER, typically www-data) generates that Tomcat must read.
# Content is never replaced here — only ownership/mode. install_jasper_runtime seeds
# missing files and preserves existing context.xml / connection properties.
jrs_generated_tomcat_files() {
  local conf="${TOMCAT_HOME}/conf"
  echo "${JRS_JASPER_CONTEXT_PATH:-${conf}/context.xml}"
  echo "${JRS_GEOSERVER_CONNECTIONS_PATH:-${conf}/geoserver-connections.properties}"
  echo "${JRS_QGIS_CONNECTIONS_PATH:-${conf}/qgis-connections.properties}"
  echo "${JRS_ARCGIS_CONNECTIONS_PATH:-${conf}/arcgis-connections.properties}"
}

# Dual-user Tomcat runtime:
#   * jrs-server writes context.xml via temp+rename (parent dir must be writable)
#   * jrs-server WriteFile's GeoServer/QGIS/ArcGIS connection properties
#   * jrs-server writes JRXML / map cache / resource-paths under acugis/
#   * acugis-tomcat (tomcat) must read those files
# Do not chmod 777. Do not recursively open $TOMCAT_HOME/conf (server.xml stays
# tomcat-owned; sticky bit blocks the service user from replacing it).
fix_tomcat_runtime_permissions() {
  [[ "${JRS_SKIP_TOMCAT:-0}" == "1" ]] && return 0
  [[ -d "$TOMCAT_HOME" ]] || return 0
  id -u "$SERVICE_USER" >/dev/null 2>&1 || return 0
  id -u "$TOMCAT_USER" >/dev/null 2>&1 || return 0

  usermod -aG "$TOMCAT_USER" "$SERVICE_USER" 2>/dev/null || true

  chown -R "$TOMCAT_USER:$TOMCAT_USER" "$TOMCAT_HOME" || true

  mkdir -p "$TOMCAT_HOME/conf" \
    "$TOMCAT_HOME/acugis/reports" \
    "$TOMCAT_HOME/acugis/uploads" \
    "$TOMCAT_HOME/acugis/fonts" \
    "$TOMCAT_HOME/acugis/drivers" \
    "$TOMCAT_HOME/acugis/licenses" \
    "$TOMCAT_HOME/acugis/temp" \
    "$TOMCAT_HOME/acugis/resource-paths"

  # conf/ stays tomcat:tomcat. 1770 + sticky: service user (tomcat group) can
  # create temp files / replace files it owns; cannot unlink Tomcat-owned conf.
  chown "$TOMCAT_USER:$TOMCAT_USER" "$TOMCAT_HOME/conf"
  chmod 1770 "$TOMCAT_HOME/conf"

  local f
  while IFS= read -r f; do
    [[ -n "$f" ]] || continue
    mkdir -p "$(dirname "$f")"
    if [[ ! -e "$f" ]]; then
      : > "$f"
    fi
    chown "$SERVICE_USER:$TOMCAT_USER" "$f"
    chmod 0640 "$f"
  done < <(jrs_generated_tomcat_files)

  # Writer = service user, reader/group = tomcat. setgid keeps new dirs in the
  # tomcat group so Jasper can follow resource-paths / _map_cache created at runtime.
  chown -R "$SERVICE_USER:$TOMCAT_USER" "$TOMCAT_HOME/acugis"
  find "$TOMCAT_HOME/acugis" -type d -exec chmod 2770 {} +
  find "$TOMCAT_HOME/acugis" -type f -exec chmod 0660 {} +
}

fix_permissions() {
  ensure_runtime_dirs
  if id -u "$SERVICE_USER" >/dev/null 2>&1; then
    chown -R "$SERVICE_USER:$SERVICE_USER" "$BASE" || true
  fi
  chmod -R 755 "$BASE" || true
  chmod 750 "$(dirname "$ENV_FILE")" 2>/dev/null || true
  if [[ -f "$ENV_FILE" ]]; then
    chown "root:${SERVICE_USER}" "$ENV_FILE" 2>/dev/null || true
    chmod 640 "$ENV_FILE" 2>/dev/null || true
  fi
  chmod -R 775 "$BASE/reports/json" "$BASE/assets/reports" "$BASE/schedules" "$BASE/resources" "$BASE/thumbnails" "$BASE/systemd" 2>/dev/null || true
  fix_tomcat_runtime_permissions
  # chmod -R 755 above reopens $BASE/.env. Strip any embed signing key an
  # older install left there, then restore group-only read.
  protect_install_dotenv
}

psql_app() {
  PGPASSWORD="${JRS_DB_PASS:-}" psql \
    -v ON_ERROR_STOP=1 \
    -h "${JRS_DB_HOST:-localhost}" \
    -p "${JRS_DB_PORT:-5432}" \
    -U "${JRS_DB_USER:-jasper}" \
    -d "${JRS_DB_NAME:-jasper_reports}" \
    "$@"
}

ensure_local_database() {
  [[ "${JRS_SKIP_DB:-0}" == "1" ]] && return 0
  [[ "${JRS_INSTALL_POSTGRES:-1}" == "1" ]] || return 0
  db_host_is_local "${JRS_DB_HOST:-localhost}" || return 0
  have_cmd psql || die "psql is missing (install postgresql-client)"
  if [[ -z "${JRS_DB_PASS:-}" ]]; then
    die "JRS_DB_PASS is empty — cannot create the local application database"
  fi
  if have_cmd systemctl; then
    systemctl enable --now postgresql 2>/dev/null || systemctl enable --now postgresql.service 2>/dev/null || true
  fi
  wait_for_local_postgres
  if ! id -u postgres >/dev/null 2>&1; then
    die "The postgres user is missing. Install the postgresql server package."
  fi
  local user name
  user="${JRS_DB_USER:-jasper}"
  name="${JRS_DB_NAME:-jasper_reports}"
  sql_ident_ok "$user" || die "JRS_DB_USER must be a simple SQL identifier (got: $user)"
  sql_ident_ok "$name" || die "JRS_DB_NAME must be a simple SQL identifier (got: $name)"
  local pass_sql
  pass_sql="$(sql_quote "$JRS_DB_PASS")"
  if run_postgres psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='${user}'" | grep -q 1; then
    log "PostgreSQL role ${user} already exists"
  else
    log "Creating PostgreSQL role ${user}"
    run_postgres psql -v ON_ERROR_STOP=1 -c "CREATE USER ${user} WITH PASSWORD ${pass_sql}"
  fi
  if run_postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='${name}'" | grep -q 1; then
    log "PostgreSQL database ${name} already exists"
  else
    log "Creating PostgreSQL database ${name}"
    run_postgres createdb -O "$user" "$name"
  fi
  run_postgres psql -d "$name" -c "GRANT ALL ON SCHEMA public TO ${user}" >/dev/null 2>&1 || true
}

apply_sql_file() {
  local file="$1"
  log "  SQL: $(basename "$file")"
  if ! psql_app -f "$file"; then
    warn "$(basename "$file") reported errors (may already be applied)"
  fi
}

# Forward-only schema apply. Never drops Professional objects.
bootstrap_database() {
  [[ "${JRS_SKIP_DB:-0}" == "1" ]] && return 0
  have_cmd psql || { warn "psql not installed; apply $BASE/database/schema.sql manually"; return 0; }
  if [[ -z "${JRS_DB_PASS:-}" ]]; then
    warn "JRS_DB_PASS unset; skipping schema apply"
    return 0
  fi

  local pkg
  pkg="$(package_edition)"
  if [[ "$pkg" == "oss" ]]; then
    local qgis
    qgis="$(psql_app -Atc "SELECT to_regclass('public.jasper_qgis') IS NOT NULL" 2>/dev/null | tr -d '[:space:]' || true)"
    if [[ "$qgis" == "t" ]]; then
      die "Database already has Professional objects (jasper_qgis). Refusing OSS install."
    fi
  fi

  local users
  users="$(psql_app -Atc "SELECT to_regclass('public.dashboard_users') IS NOT NULL" 2>/dev/null | tr -d '[:space:]' || true)"
  if [[ "$users" != "t" ]]; then
    log "Applying database/schema.sql"
    psql_app -f "$BASE/database/schema.sql"
  else
    log "Application schema already present — applying forward migrations only"
  fi

  local mig
  if [[ -d "$BASE/database/migrations" ]]; then
    while IFS= read -r mig; do
      [[ -n "$mig" ]] || continue
      apply_sql_file "$mig"
    done < <(find "$BASE/database/migrations" -maxdepth 1 -name '*.sql' | sort)
  fi
  while IFS= read -r mig; do
    [[ -n "$mig" ]] || continue
    apply_sql_file "$mig"
  done < <(find "$BASE/database" -maxdepth 1 -name 'migrate_*.sql' | sort)
}

seed_admin() {
  [[ "${JRS_SKIP_ADMIN:-0}" == "1" ]] && return 0
  if [[ -z "${JRS_ADMIN_PASSWORD:-}" ]]; then
    log "JRS_ADMIN_PASSWORD not set — skipping admin seed"
    return 0
  fi
  have_cmd php || { warn "php not installed; skip admin seed"; return 0; }
  log "Seeding administrator via scripts/seed_admin.php"
  (
    cd "$BASE"
    export JRS_DB_HOST JRS_DB_PORT JRS_DB_NAME JRS_DB_USER JRS_DB_PASS JRS_DB_SCHEMA
    export JRS_ADMIN_USER="${JRS_ADMIN_USER:-admin}"
    export JRS_ADMIN_PASSWORD
    php scripts/seed_admin.php
  )
}

reload_systemd() {
  have_cmd systemctl || return 0
  systemctl daemon-reload || true
}

enable_services() {
  have_cmd systemctl || return 0
  systemctl enable acugis-tomcat.service acugis-report-server.service >/dev/null 2>&1 || true
}

disable_services() {
  have_cmd systemctl || return 0
  systemctl disable acugis-tomcat.service acugis-report-server.service >/dev/null 2>&1 || true
}

stop_services() {
  have_cmd systemctl || return 0
  systemctl stop acugis-report-server.service 2>/dev/null || true
  systemctl stop acugis-tomcat.service 2>/dev/null || true
}

start_services() {
  [[ "${JRS_SKIP_SYSTEMD:-0}" == "1" ]] && return 0
  have_cmd systemctl || return 0
  log "Starting services"
  systemctl restart acugis-tomcat.service
  systemctl restart acugis-report-server.service
}

start_services_best_effort() {
  [[ "${JRS_SKIP_SYSTEMD:-0}" == "1" ]] && return 0
  have_cmd systemctl || return 0
  log "Starting services"
  systemctl restart acugis-tomcat.service || warn "acugis-tomcat.service did not start"
  systemctl restart acugis-report-server.service || warn "acugis-report-server.service did not start"
}

# Full instance configure used by Debian postinst and Agent after writing env.
# First boot generates DB/admin passwords when unset (local PostgreSQL).
configure_instance() {
  ensure_users
  install_tomcat
  install_jasper_runtime
  write_env_file
  ensure_first_boot_credentials
  record_edition_state
  load_env
  install_schedule_sudoers
  configure_apache
  ensure_runtime_dirs
  if [[ "${JRS_SKIP_SYSTEMD:-0}" != "1" ]]; then
    reload_systemd
    enable_services
  fi
  fix_permissions
  if [[ "${JRS_SKIP_DB:-0}" == "1" ]]; then
    log "JRS_SKIP_DB=1 — skipping database bootstrap and service start"
  elif db_credentials_present; then
    ensure_local_database
    bootstrap_database
    seed_admin
    start_services_best_effort
  else
    die "JRS_DB_PASS is not set. For a remote database, set it in $ENV_FILE and run $BASE/scripts/configure-instance.sh"
  fi
}
