#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dist_dir="${1:-${repo_root}/dist}"
archive_count=0

shopt -s nullglob
archives=("${dist_dir}"/breyta_*_darwin_*.tar.gz \
          "${dist_dir}"/breyta_*_linux_*.tar.gz \
          "${dist_dir}"/breyta_*_windows_*.zip)

for archive in "${archives[@]}"; do
  archive_count=$((archive_count + 1))
  archive_name="$(basename "${archive}")"
  case "${archive_name}" in
    *_darwin_amd64.tar.gz) target=darwin/amd64; binary_name=breyta; parinfer_name=parinfer-rust ;;
    *_darwin_arm64.tar.gz) target=darwin/arm64; binary_name=breyta; parinfer_name=parinfer-rust ;;
    *_linux_amd64.tar.gz) target=linux/amd64; binary_name=breyta; parinfer_name=parinfer-rust ;;
    *_linux_arm64.tar.gz) target=linux/arm64; binary_name=breyta; parinfer_name=parinfer-rust ;;
    *_windows_amd64.zip) target=windows/amd64; binary_name=breyta.exe; parinfer_name=parinfer-rust.exe ;;
    *) echo "unsupported release archive: ${archive_name}" >&2; exit 1 ;;
  esac

  extract_dir="$(mktemp -d "${TMPDIR:-/tmp}/breyta-release-archive.XXXXXX")"
  expected_dir="$(mktemp -d "${TMPDIR:-/tmp}/breyta-release-compliance.XXXXXX")"
  if [[ "${archive}" == *.zip ]]; then
    unzip -q "${archive}" -d "${extract_dir}"
  else
    tar -xzf "${archive}" -C "${extract_dir}"
  fi

  for required in "${binary_name}" "${parinfer_name}" LICENSE THIRD_PARTY_NOTICES.md \
    licenses/parinfer-rust-ISC.md licenses/parinfer-rust-THIRD_PARTY_NOTICES.md \
    provenance/parinfer-rust-SOURCE.lock provenance/parinfer-rust-Cargo.lock \
    provenance/parinfer-rust-Cargo.toml provenance/parinfer-rust-components.json; do
    if [[ ! -f "${extract_dir}/${required}" ]]; then
      echo "${archive_name} is missing ${required}" >&2
      exit 1
    fi
  done

  "${repo_root}/scripts/generate-go-compliance.sh" \
    "${extract_dir}/${binary_name}" "${expected_dir}"
  diff -u "${expected_dir}/THIRD_PARTY_NOTICES.md" \
    "${extract_dir}/THIRD_PARTY_NOTICES.md"
  diff -ru "${expected_dir}/licenses/go" "${extract_dir}/licenses/go"

  cmp "${repo_root}/tools/parinfer-rust/SOURCE.lock" \
    "${extract_dir}/provenance/parinfer-rust-SOURCE.lock"
  cmp "${repo_root}/tools/parinfer-rust/Cargo.lock" \
    "${extract_dir}/provenance/parinfer-rust-Cargo.lock"
  cmp "${repo_root}/tools/parinfer-rust/Cargo.toml" \
    "${extract_dir}/provenance/parinfer-rust-Cargo.toml"
  cmp "${repo_root}/tools/parinfer-rust/components.json" \
    "${extract_dir}/provenance/parinfer-rust-components.json"

  expected_hash="$(awk -F= -v target="${target}/${parinfer_name}" \
    '$1 == target { print $2 }' "${repo_root}/tools/parinfer-rust/SOURCE.lock")"
  actual_hash="$(shasum -a 256 "${extract_dir}/${parinfer_name}" | awk '{print $1}')"
  if [[ -z "${expected_hash}" || "${actual_hash}" != "${expected_hash}" ]]; then
    echo "${archive_name} contains an unpinned parinfer-rust binary" >&2
    exit 1
  fi

  sbom="${archive}.spdx.json"
  if [[ ! -f "${sbom}" ]]; then
    echo "${archive_name} is missing its artifact-derived SPDX SBOM" >&2
    exit 1
  fi
  jq -e '.spdxVersion | startswith("SPDX-")' "${sbom}" >/dev/null
  jq -e --arg hash "${actual_hash}" '
    [.packages[] | select(.SPDXID == "SPDXRef-Package-parinfer-rust") |
      .checksums[] | select(.algorithm == "SHA256" and .checksumValue == $hash)] |
    length == 1
  ' "${sbom}" >/dev/null
  while IFS=$'\t' read -r name version; do
    jq -e --arg purl "pkg:cargo/${name}@${version}" '
      [.packages[] | select(.externalRefs[]?.referenceLocator == $purl)] |
      length == 1
    ' "${sbom}" >/dev/null
  done < <(jq -r '.packages[] | [.name, .version] | @tsv' \
    "${repo_root}/tools/parinfer-rust/components.json")
  for module in $(cut -f1 "${repo_root}/compliance/go-modules.tsv" | rg -v '^#'); do
    if go version -m "${extract_dir}/${binary_name}" | rg -F $'\tdep\t'"${module}"$'\t' >/dev/null; then
      jq -e --arg module "${module}" \
        '[.packages[]? | select((.name == $module) or (.externalRefs[]?.referenceLocator | contains($module)))] | length > 0' \
        "${sbom}" >/dev/null
    fi
  done

  rm -rf "${extract_dir}" "${expected_dir}"
  echo "Verified ${archive_name} (${target})"
done

if [[ "${archive_count}" -ne 5 ]]; then
  echo "expected five release archives, found ${archive_count}" >&2
  exit 1
fi

echo "Verified five non-publishing release candidates and their SBOMs"
