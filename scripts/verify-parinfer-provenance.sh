#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
lock_file="${repo_root}/tools/parinfer-rust/SOURCE.lock"
artifact_root="${1:-${repo_root}/tools/parinfer-rust}"

required_metadata=(repository tag commit rust_toolchain cargo_lock_sha256 cargo_toml_sha256 components_sha256 notices_sha256)
for key in "${required_metadata[@]}"; do
  count="$(awk -F= -v key="${key}" '$1 == key { count++ } END { print count + 0 }' "${lock_file}")"
  if [[ "${count}" -ne 1 ]]; then
    echo "SOURCE.lock must contain exactly one ${key} entry" >&2
    exit 1
  fi
done

required_binaries=(
  linux/amd64/parinfer-rust
  linux/arm64/parinfer-rust
  darwin/amd64/parinfer-rust
  darwin/arm64/parinfer-rust
  windows/amd64/parinfer-rust.exe
)
for target in "${required_binaries[@]}"; do
  count="$(awk -F= -v target="${target}" '$1 == target { count++ } END { print count + 0 }' "${lock_file}")"
  if [[ "${count}" -ne 1 ]]; then
    echo "SOURCE.lock must contain exactly one ${target} entry" >&2
    exit 1
  fi
done

for source_file in Cargo.lock Cargo.toml; do
  key="$(tr '[:upper:].' '[:lower:]_' <<< "${source_file}")_sha256"
  expected_hash="$(awk -F= -v key="${key}" '$1 == key { print $2 }' "${lock_file}")"
  actual_hash="$(shasum -a 256 "${repo_root}/tools/parinfer-rust/${source_file}" | awk '{print $1}')"
  if [[ "${actual_hash}" != "${expected_hash}" ]]; then
    echo "parinfer-rust ${source_file} hash mismatch" >&2
    exit 1
  fi
done

for provenance_entry in \
  "components.json:components_sha256" \
  "THIRD_PARTY_NOTICES.md:notices_sha256"; do
  source_file="${provenance_entry%%:*}"
  key="${provenance_entry#*:}"
  expected_hash="$(awk -F= -v key="${key}" '$1 == key { print $2 }' "${lock_file}")"
  actual_hash="$(shasum -a 256 "${repo_root}/tools/parinfer-rust/${source_file}" | awk '{print $1}')"
  if [[ "${actual_hash}" != "${expected_hash}" ]]; then
    echo "parinfer-rust ${source_file} hash mismatch" >&2
    exit 1
  fi
done

binary_count=0
while IFS='=' read -r relative_path expected_hash; do
  case "${relative_path}" in
    linux/*|darwin/*|windows/*)
      binary_count=$((binary_count + 1))
      binary_path="${artifact_root}/${relative_path}"
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

if [[ "${binary_count}" -ne 5 ]]; then
  echo "SOURCE.lock must pin exactly five supported parinfer-rust binaries" >&2
  exit 1
fi

echo "parinfer-rust source and binary provenance hashes verified"
