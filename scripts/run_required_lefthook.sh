#!/bin/sh

set -eu

hook_name="${1:?hook name is required}"
shift

if ! command -v lefthook >/dev/null 2>&1 || ! lefthook version >/dev/null 2>&1; then
  cat >&2 <<'EOF'
error: Lefthook is required before committing in this repository.
Install Lefthook, then run:
  make install-git-hooks
EOF
  exit 1
fi

exec lefthook run "$hook_name" "$@" --no-auto-install
