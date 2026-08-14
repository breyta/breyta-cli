#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
lock_file="${repo_root}/tools/parinfer-rust/SOURCE.lock"

while IFS='=' read -r relative_path expected_hash; do
  case "${relative_path}" in
    linux/*|darwin/*|windows/*)
      binary_path="${repo_root}/tools/parinfer-rust/${relative_path}"
      if [[ ! -f "${binary_path}" ]]; then
        echo "missing pinned parinfer-rust binary: ${relative_path}" >&2
        exit 1
      fi
      actual_hash="$(shasum -a 256 "${binary_path}" | awk '{print $1}')"
      if [[ "${actual_hash}" != "${expected_hash}" ]]; then
        echo "parinfer-rust hash mismatch: ${relative_path}" >&2
        exit 1
      fi
      ;;
  esac
done < "${lock_file}"

echo "parinfer-rust provenance hashes verified"
