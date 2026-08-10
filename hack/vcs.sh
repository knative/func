#!/usr/bin/env bash

# VCS abstraction layer for git and jj.
# Usage:
#   hack/vcs.sh ls-sources   # list tracked source files, excluding generated/vendored
#
# Set FUNC_VCS=git|jj to override auto-detection (e.g. FUNC_VCS=jj make check).

set -euo pipefail

# --- VCS detection ---

VCS=""
if [ -n "${FUNC_VCS:-}" ]; then
  VCS="$FUNC_VCS"
elif git rev-parse --is-inside-work-tree &>/dev/null; then
  VCS=git
elif [ -d .jj ]; then
  VCS=jj
fi

# --- Subcommands ---

cmd_ls_sources() {
  if [ "$VCS" = git ]; then
    git ls-files \
      | git check-attr --stdin linguist-generated | grep -Ev ': (set|true)$' | cut -d: -f1 \
      | git check-attr --stdin linguist-vendored  | grep -Ev ': (set|true)$' | cut -d: -f1 \
      | grep -Ev '(vendor/|third_party/|\.git)'
  elif [ "$VCS" = jj ]; then
    # jj has no .gitattributes support (https://github.com/jj-vcs/jj/issues/53),
    # so we replicate the linguist-generated filters from .gitattributes here.
    jj file list \
      | grep -Ev '(zz_filesystem_generated\.go$|zz_close_guarding_client_generated\.go$)' \
      | grep -Ev '(vendor/|third_party/|\.git|\.jj)'
  else
    find . -type f | sed 's|^\./||' \
      | grep -Ev '(zz_filesystem_generated\.go$|zz_close_guarding_client_generated\.go$)' \
      | grep -Ev '(vendor/|third_party/|\.git|\.jj)'
  fi
}

# --- Dispatch ---

case "${1:-}" in
  ls-sources) cmd_ls_sources ;;
  *)
    echo "Usage: $0 {ls-sources}" >&2
    exit 1
    ;;
esac
