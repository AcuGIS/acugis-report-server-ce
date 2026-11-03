#!/usr/bin/env bash
# Resolve the source revision recorded in a production package.
# REVISION, when set, is the full commit SHA of the tree being packaged.
# A git checkout supplies HEAD when REVISION is unset.
# An exported tree with no .git supplies SOURCE_REVISION, written by the
# OSS export from the private repository. That file is not used when git
# HEAD is available.
# Callers must fail the build when this returns non-zero.

resolve_source_revision() {
  local root="$1"
  local explicit="${REVISION-}"
  explicit="${explicit//$'\r'/}"
  if [[ -n "$explicit" ]]; then
    if [[ "$explicit" == "unknown" ]]; then
      echo "source revision must be a full git commit SHA, not unknown" >&2
      return 1
    fi
    if [[ ! "$explicit" =~ ^[0-9a-fA-F]{40}(-dirty)?$ ]]; then
      echo "source revision must be a full 40-character git commit SHA: ${explicit}" >&2
      return 1
    fi
    printf '%s\n' "$explicit"
    return 0
  fi

  if git -C "$root" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    local sha
    sha="$(git -C "$root" rev-parse HEAD 2>/dev/null)" || {
      echo "git HEAD is not available in ${root}" >&2
      return 1
    }
    if [[ ! "$sha" =~ ^[0-9a-fA-F]{40}$ ]]; then
      echo "git HEAD is not a full commit SHA: ${sha}" >&2
      return 1
    fi
    if [[ -n "$(git -C "$root" status --porcelain 2>/dev/null)" ]]; then
      sha="${sha}-dirty"
    fi
    printf '%s\n' "$sha"
    return 0
  fi

  local recorded="${root}/SOURCE_REVISION"
  if [[ -f "$recorded" ]]; then
    local sha
    sha="$(tr -d '[:space:]' < "$recorded")"
    if [[ ! "$sha" =~ ^[0-9a-fA-F]{40}(-dirty)?$ ]]; then
      echo "recorded source revision must be a full 40-character git commit SHA: ${recorded}" >&2
      return 1
    fi
    printf '%s\n' "$sha"
    return 0
  fi

  echo "source revision is required: set REVISION to the full commit SHA, build from a git checkout, or export SOURCE_REVISION" >&2
  return 1
}
