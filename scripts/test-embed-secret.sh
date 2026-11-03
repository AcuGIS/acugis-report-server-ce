#!/usr/bin/env bash
# Installer regression tests for ACUGIS_EMBED_TOKEN_SECRET.
# Does not install packages or print generated secrets.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SETUP="$ROOT/scripts/native-setup.sh"

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "ok: $*"; }

unset ACUGIS_EMBED_TOKEN_SECRET JRS_EMBED_TOKEN_SECRET

new_root() {
  local dir
  dir="$(mktemp -d)"
  mkdir -p "$dir/opt" "$dir/etc"
  printf 'edition=professional\nversion=1.0.16\n' > "$dir/RELEASE"
  printf '%s\n' "$dir"
}

source_setup() {
  local root="$1"
  export SCRIPT_DIR="$root"
  export BASE="$root/opt"
  export ENV_FILE="$root/etc/env"
  export CREDENTIALS_FILE="$root/etc/initial-credentials"
  export JRS_SKIP_DB=1
  export JRS_SKIP_TOMCAT=1
  # shellcheck disable=SC1090
  source "$SETUP"
}

file_mode() {
  local file="$1" mode
  mode="$(stat -c %a "$file" 2>/dev/null || stat -f %OLp "$file")"
  printf '%s' "$mode"
}

assert_mode_640() {
  local file="$1" mode
  mode="$(file_mode "$file")"
  [[ "$mode" == "640" ]] || fail "$file mode is $mode, want 640"
}

assert_root_www_data_when_possible() {
  local file="$1"
  [[ "$(id -u)" -eq 0 ]] || return 0
  id www-data >/dev/null 2>&1 || return 0
  [[ "$(stat -c %U "$file")" == "root" ]] || fail "$file owner is not root"
  [[ "$(stat -c %G "$file")" == "www-data" ]] || fail "$file group is not www-data"
}

assert_no_embed_assignment() {
  local file="$1"
  if [[ -f "$file" ]] && grep -qE '^(export[[:space:]]+)?(ACUGIS_EMBED_TOKEN_SECRET|JRS_EMBED_TOKEN_SECRET)=' "$file"; then
    fail "$file assigns an embed signing secret"
  fi
}

# World-readable or world-executable files must not carry the signing key or its value.
assert_secret_not_world_accessible() {
  local root="$1" protected="$2" secret f
  secret="$(secret_of "$protected")"
  [[ -n "$secret" ]] || fail "protected file has no embed secret"
  while IFS= read -r -d '' f; do
    if grep -qE '^(export[[:space:]]+)?(ACUGIS_EMBED_TOKEN_SECRET|JRS_EMBED_TOKEN_SECRET)=' "$f"; then
      fail "world-accessible file assigns an embed signing secret: $f"
    fi
    if grep -q -F -- "$secret" "$f"; then
      fail "embed secret value is in world-accessible file $f"
    fi
  done < <(find "$root" -type f -perm /005 -print0)
}

mint_from_protected_file() {
  local file="$1"
  [[ -n "$(secret_of "$file")" ]] || fail "cannot mint without a protected secret"
  (
    cd "$ROOT/go-report-server"
    JRS_TEST_EMBED_SECRET_FILE="$file" go test -count=1 -run TestMintFromInstalledSecretFile ./internal/embedtoken/
  ) || fail "token mint from the protected env file failed"
}

secret_of() {
  local file="$1"
  grep -E '^ACUGIS_EMBED_TOKEN_SECRET=' "$file" | tail -n1 | cut -d= -f2-
}

assert_secret_file() {
  local file="$1" label="$2"
  local secret
  secret="$(secret_of "$file")"
  [[ ${#secret} -eq 64 && "$secret" =~ ^[0-9a-fA-F]+$ ]] || fail "$label secret is not 64 hex chars"
  if grep -q "$secret" <<<"$3"; then
    fail "$label printed the secret"
  fi
}

# Fresh file: generate, do not print, second call keeps it.
fresh_a="$(new_root)"
log_a="$(
  source_setup "$fresh_a"
  ensure_embed_token_secret
)"
assert_secret_file "$fresh_a/etc/env" "fresh" "$log_a"
secret_a="$(secret_of "$fresh_a/etc/env")"
(
  source_setup "$fresh_a"
  ACUGIS_EMBED_TOKEN_SECRET='different-admin-value' ensure_embed_token_secret
)
[[ "$(secret_of "$fresh_a/etc/env")" == "$secret_a" ]] || fail "second run rotated a generated secret"
pass "fresh generate is stable and not printed"

fresh_b="$(new_root)"
(
  source_setup "$fresh_b"
  ensure_embed_token_secret
)
secret_b="$(secret_of "$fresh_b/etc/env")"
[[ "$secret_a" != "$secret_b" ]] || fail "two installations received the same secret"
pass "secrets differ across installations"

# Administrator value is stored when the file has none, then kept.
admin="$(new_root)"
admin_log="$(
  source_setup "$admin"
  ACUGIS_EMBED_TOKEN_SECRET='admin-supplied-embed-secret' ensure_embed_token_secret
)"
[[ "$(secret_of "$admin/etc/env")" == "admin-supplied-embed-secret" ]] || fail "admin secret was not stored"
if grep -q 'admin-supplied-embed-secret' <<<"$admin_log"; then
  fail "admin secret was printed"
fi
(
  source_setup "$admin"
  ACUGIS_EMBED_TOKEN_SECRET='rotate-me' ensure_embed_token_secret
)
[[ "$(secret_of "$admin/etc/env")" == "admin-supplied-embed-secret" ]] || fail "admin secret was rotated"
pass "administrator secret is preserved"

# Existing env without an embed key gets one; other keys stay.
upgrade="$(new_root)"
printf '%s\n' 'JRS_DB_PASS=keep-db' 'JRS_ADMIN_PASSWORD=keep-admin' > "$upgrade/etc/env"
(
  source_setup "$upgrade"
  ensure_embed_token_secret
)
grep -qx 'JRS_DB_PASS=keep-db' "$upgrade/etc/env" || fail "upgrade changed JRS_DB_PASS"
grep -qx 'JRS_ADMIN_PASSWORD=keep-admin' "$upgrade/etc/env" || fail "upgrade changed JRS_ADMIN_PASSWORD"
assert_secret_file "$upgrade/etc/env" "upgrade" ""
pass "missing secret is added without rotating other keys"

# Legacy JRS_EMBED_TOKEN_SECRET is left alone.
legacy="$(new_root)"
printf '%s\n' 'JRS_EMBED_TOKEN_SECRET=legacy-embed-secret' 'JRS_DB_PASS=keep-db' > "$legacy/etc/env"
(
  source_setup "$legacy"
  ACUGIS_EMBED_TOKEN_SECRET='should-not-replace' ensure_embed_token_secret
)
grep -qx 'JRS_EMBED_TOKEN_SECRET=legacy-embed-secret' "$legacy/etc/env" || fail "legacy secret changed"
if grep -q '^ACUGIS_EMBED_TOKEN_SECRET=' "$legacy/etc/env"; then
  fail "legacy secret was shadowed by a new ACUGIS_EMBED_TOKEN_SECRET"
fi
pass "legacy JRS_EMBED_TOKEN_SECRET is preserved"

# First-boot helper used by postinst writes the secret and not the credentials file.
boot="$(new_root)"
(
  source_setup "$boot"
  ensure_first_boot_credentials
)
assert_secret_file "$boot/etc/env" "first-boot" ""
if [[ -f "$boot/etc/initial-credentials" ]] && grep -q 'ACUGIS_EMBED_TOKEN_SECRET' "$boot/etc/initial-credentials"; then
  fail "initial-credentials contains the embed secret"
fi
grep -qE '^JRS_DB_PASS=.+' "$boot/etc/env" || fail "first boot did not create JRS_DB_PASS"
pass "first-boot credentials include an embed secret outside initial-credentials"

# Fresh install through the same permission step as package configure.
installed="$(new_root)"
install_log="$(
  source_setup "$installed"
  ensure_first_boot_credentials
  fix_permissions
)"
assert_secret_file "$installed/etc/env" "installed" "$install_log"
assert_mode_640 "$installed/etc/env"
assert_root_www_data_when_possible "$installed/etc/env"
[[ -f "$installed/opt/.env" ]] || fail "hand-start mirror was not created"
assert_no_embed_assignment "$installed/opt/.env"
assert_mode_640 "$installed/opt/.env"
assert_root_www_data_when_possible "$installed/opt/.env"
grep -qE '^JRS_DB_PASS=.+' "$installed/opt/.env" || fail "mirror dropped database settings"
if [[ -f "$installed/etc/initial-credentials" ]] && grep -q 'ACUGIS_EMBED_TOKEN_SECRET' "$installed/etc/initial-credentials"; then
  fail "initial-credentials contains the embed secret"
fi
assert_secret_not_world_accessible "$installed" "$installed/etc/env"
mint_from_protected_file "$installed/etc/env"
pass "fresh install protects the secret and can mint"

# Upgrade keeps the authoritative secret and deletes a world-readable copy.
leaked="$(new_root)"
printf '%s\n' 'JRS_ENVIRONMENT=production' 'JRS_DB_PASS=keep-db' 'JRS_ADMIN_PASSWORD=keep-admin' 'ACUGIS_EMBED_TOKEN_SECRET=keep-embed-secret' > "$leaked/etc/env"
printf '%s\n' 'JRS_DB_PASS=keep-db' 'ACUGIS_EMBED_TOKEN_SECRET=keep-embed-secret' 'JRS_EMBED_TOKEN_SECRET=legacy-copy' > "$leaked/opt/.env"
chmod 755 "$leaked/opt/.env"
(
  source_setup "$leaked"
  ensure_embed_token_secret
  fix_permissions
)
grep -qx 'ACUGIS_EMBED_TOKEN_SECRET=keep-embed-secret' "$leaked/etc/env" || fail "upgrade rotated the authoritative secret"
grep -qx 'JRS_DB_PASS=keep-db' "$leaked/etc/env" || fail "upgrade changed JRS_DB_PASS"
assert_mode_640 "$leaked/etc/env"
assert_no_embed_assignment "$leaked/opt/.env"
grep -qx 'JRS_DB_PASS=keep-db' "$leaked/opt/.env" || fail "upgrade dropped mirrored database settings"
assert_mode_640 "$leaked/opt/.env"
assert_root_www_data_when_possible "$leaked/opt/.env"
assert_secret_not_world_accessible "$leaked" "$leaked/etc/env"
pass "upgrade removes the install-tree embed secret and preserves the authoritative one"

# Administrator value stays in the protected file and is not mirrored.
admin_mirror="$(new_root)"
admin_mirror_log="$(
  source_setup "$admin_mirror"
  ACUGIS_EMBED_TOKEN_SECRET='admin-supplied-embed-secret' ensure_first_boot_credentials
  fix_permissions
)"
[[ "$(secret_of "$admin_mirror/etc/env")" == "admin-supplied-embed-secret" ]] || fail "admin secret was not stored"
if grep -q 'admin-supplied-embed-secret' <<<"$admin_mirror_log"; then
  fail "admin secret was printed"
fi
assert_no_embed_assignment "$admin_mirror/opt/.env"
assert_mode_640 "$admin_mirror/etc/env"
if [[ -f "$admin_mirror/etc/initial-credentials" ]] && grep -q 'ACUGIS_EMBED_TOKEN_SECRET\|admin-supplied-embed-secret' "$admin_mirror/etc/initial-credentials"; then
  fail "initial-credentials contains the administrator embed secret"
fi
mint_from_protected_file "$admin_mirror/etc/env"
pass "administrator secret stays only in the protected file and can mint"

rm -rf "$fresh_a" "$fresh_b" "$admin" "$upgrade" "$legacy" "$boot" "$installed" "$leaked" "$admin_mirror"
echo "PASSED embed secret tests"
