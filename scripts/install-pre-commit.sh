#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

hook_src="$repo_root/githooks/pre-commit"
hook_dest="$repo_root/.git/hooks/pre-commit"

if [ ! -f "$hook_src" ]; then
  echo "pre-commit hook not found: $hook_src" >&2
  exit 1
fi

mkdir -p "$(dirname "$hook_dest")"
cp "$hook_src" "$hook_dest"
chmod +x "$hook_dest"

echo "Installed pre-commit hook to $hook_dest"
