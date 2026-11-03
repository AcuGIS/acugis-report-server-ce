#!/bin/bash
# Native installer for AcuGIS Report Server release packages.
# Installs precompiled package contents. Does not clone Git or invoke Go builds.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/scripts/native-setup.sh"

ASSUME_YES="${JRS_ASSUME_YES:-1}"

need_root() {
  if [[ "$(id -u)" -ne 0 ]]; then
    die "Run as root (sudo ./install.sh)"
  fi
}

is_debian() { [[ -f /etc/debian_version ]] || have_cmd apt-get; }

ensure_package() {
  [[ -x "$SCRIPT_DIR/bin/jrs-server" ]] || die "bin/jrs-server missing — run this installer from a release package, not a source checkout"
  [[ -x "$SCRIPT_DIR/bin/jrs-run-schedule" ]] || die "bin/jrs-run-schedule missing"
  [[ -f "$SCRIPT_DIR/tomcat/acugis-report-server.war" ]] || die "tomcat/acugis-report-server.war missing"
  [[ -f "$SCRIPT_DIR/database/schema.sql" ]] || die "database/schema.sql missing"
  [[ -f "$SCRIPT_DIR/RELEASE" ]] || die "RELEASE manifest missing"
  [[ -f "$SCRIPT_DIR/scripts/native-setup.sh" ]] || die "scripts/native-setup.sh missing"
}

apt_install() {
  [[ "${JRS_SKIP_DEPS:-0}" == "1" ]] && return 0
  is_debian || return 0
  DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends "$@"
}

install_runtime_deps() {
  [[ "${JRS_SKIP_DEPS:-0}" == "1" ]] && { log "Skipping dependency installation"; return 0; }
  is_debian || { warn "Not a Debian/Ubuntu host; install Java 17, Tomcat 10.1, PostgreSQL client, and PHP CLI yourself"; return 0; }

  log "Installing/checking runtime dependencies (not build tools)"
  apt-get update -y
  local pkgs=()
  have_cmd sudo || pkgs+=(sudo)
  have_cmd curl || pkgs+=(curl ca-certificates)
  have_cmd openssl || pkgs+=(openssl)
  have_cmd rsync || pkgs+=(rsync)
  have_cmd tar || pkgs+=(tar)
  if ! have_cmd java; then
    pkgs+=(openjdk-17-jre-headless)
  fi
  have_cmd psql || pkgs+=(postgresql-client)
  if [[ "${JRS_INSTALL_POSTGRES:-1}" == "1" && "${JRS_DB_HOST:-localhost}" == "localhost" ]]; then
    have_cmd postgres || have_cmd pg_isready || pkgs+=(postgresql)
  fi
  if [[ "${JRS_SKIP_PHP:-0}" != "1" ]]; then
    have_cmd php || pkgs+=(php-cli php-pgsql php-xml)
  fi
  if [[ "${JRS_SKIP_APACHE:-0}" != "1" ]]; then
    have_cmd apache2 || have_cmd apachectl || pkgs+=(apache2)
  fi
  if [[ ${#pkgs[@]} -gt 0 ]]; then
    apt_install "${pkgs[@]}"
  fi
}

copy_product_files() {
  log "Installing package files to $BASE"
  mkdir -p "$BASE"
  local rsync_excludes=(
    --exclude 'reports/json/***'
    --exclude 'schedules/output/***'
    --exclude 'schedules/*_env.sh'
    --exclude 'schedules/logs/***'
    --exclude 'resources/***'
    --exclude 'thumbnails/***'
    --exclude 'assets/reports/***'
    --exclude '.env'
    --exclude 'docker/report-server/context.xml'
    --exclude 'docker/report-server/*.properties'
  )
  if have_cmd rsync; then
    rsync -a "${rsync_excludes[@]}" \
      --exclude 'install.sh' \
      "$SCRIPT_DIR/" "$BASE/"
  else
    cp -a "$SCRIPT_DIR/bin" "$BASE/"
    cp -a "$SCRIPT_DIR/database" "$BASE/"
    cp -a "$SCRIPT_DIR/tomcat" "$BASE/"
    cp -a "$SCRIPT_DIR/scripts" "$BASE/"
    cp -a "$SCRIPT_DIR/lib" "$BASE/"
    cp -a "$SCRIPT_DIR/systemd" "$BASE/"
    cp -a "$SCRIPT_DIR/config" "$BASE/"
    cp -a "$SCRIPT_DIR/docs" "$BASE/"
    cp -a "$SCRIPT_DIR/shared" "$BASE/" 2>/dev/null || true
    mkdir -p "$BASE/reports" "$BASE/assets/js" "$BASE/email_tmpl"
    cp -a "$SCRIPT_DIR/reports/portal.css" "$BASE/reports/portal.css"
    if [[ -d "$SCRIPT_DIR/assets/js" ]]; then
      mkdir -p "$BASE/assets/js"
      cp -a "$SCRIPT_DIR/assets/js/." "$BASE/assets/js/"
    fi
    cp -a "$SCRIPT_DIR/config.php" "$BASE/config.php"
    cp -a "$SCRIPT_DIR/RELEASE" "$BASE/RELEASE"
    cp -a "$SCRIPT_DIR/release.json" "$BASE/release.json"
    [[ -d "$SCRIPT_DIR/email_tmpl" ]] && cp -a "$SCRIPT_DIR/email_tmpl/." "$BASE/email_tmpl/"
    [[ -d "$SCRIPT_DIR/docker" ]] && mkdir -p "$BASE/docker" && cp -a "$SCRIPT_DIR/docker/." "$BASE/docker/"
  fi

  ensure_runtime_dirs
}

install_systemd_units() {
  [[ "${JRS_SKIP_SYSTEMD:-0}" == "1" ]] && return 0
  have_cmd systemctl || { warn "systemctl not available"; return 0; }
  log "Installing systemd units"
  local unit_src dest
  for unit_src in acugis-tomcat.service acugis-report-server.service; do
    dest="/etc/systemd/system/$unit_src"
    sed \
      -e "s|/opt/acugis-report-server|${BASE}|g" \
      -e "s|/opt/tomcat|${TOMCAT_HOME}|g" \
      -e "s|User=www-data|User=${SERVICE_USER}|g" \
      -e "s|Group=www-data|Group=${SERVICE_USER}|g" \
      -e "s|SupplementaryGroups=tomcat|SupplementaryGroups=${TOMCAT_USER}|g" \
      -e "s|User=tomcat|User=${TOMCAT_USER}|g" \
      -e "s|Group=tomcat|Group=${TOMCAT_USER}|g" \
      -e "s|/etc/acugis-report-server/env|${ENV_FILE}|g" \
      "$SCRIPT_DIR/systemd/$unit_src" > "$dest"
  done
  systemctl daemon-reload
  systemctl enable acugis-tomcat.service acugis-report-server.service
}

main() {
  log "Installing AcuGIS Report Server (native package)"
  ensure_package
  need_root
  guard_edition
  log "Package edition=$(package_edition) version=$(package_version)"
  log "Install directory: $BASE"

  install_runtime_deps
  ensure_users
  install_tomcat
  copy_product_files
  install_jasper_runtime
  write_env_file
  ensure_first_boot_credentials
  load_env
  configure_apache
  install_systemd_units
  install_schedule_sudoers
  fix_permissions
  ensure_local_database
  bootstrap_database
  seed_admin
  start_services

  print_instance_summary
}

main "$@"
