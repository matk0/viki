#!/usr/bin/env bash

set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

for required in .githooks/pre-commit scripts/run_required_lefthook.sh; do
  if [[ ! -x "$required" ]]; then
    echo "error: required executable is missing or not executable: $required" >&2
    exit 1
  fi
done

if ! command -v lefthook >/dev/null 2>&1 || ! lefthook version >/dev/null 2>&1; then
  cat >&2 <<'EOF'
error: Lefthook is required before committing in this repository.
Install Lefthook, then run:
  make install-git-hooks
EOF
  exit 1
fi

git config core.hooksPath .githooks
configured_hooks_path="$(git config --get core.hooksPath)"
if [[ "$configured_hooks_path" != ".githooks" ]]; then
  echo "error: expected core.hooksPath=.githooks, got $configured_hooks_path" >&2
  exit 1
fi

echo "Git hooks configured with core.hooksPath=.githooks"
