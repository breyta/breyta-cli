#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: $0 <breyta-binary> <output-directory>" >&2
  exit 2
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
binary="$1"
output_dir="$2"
policy_file="${repo_root}/compliance/go-modules.tsv"

if [[ ! -f "${binary}" || ! -f "${policy_file}" ]]; then
  echo "missing binary or Go module policy" >&2
  exit 1
fi

tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/breyta-go-compliance.XXXXXX")"
trap 'rm -rf "${tmp_dir}"' EXIT

go version -m "${binary}" \
  | awk -F '\t' '$2 == "dep" { print $3 "\t" $4 "\t" $5 }' \
  | LC_ALL=C sort > "${tmp_dir}/artifact-modules.tsv"

if [[ ! -s "${tmp_dir}/artifact-modules.tsv" ]]; then
  echo "compiled artifact contains no Go module build information: ${binary}" >&2
  exit 1
fi

mkdir -p "${output_dir}/licenses/go"
notice_file="${output_dir}/THIRD_PARTY_NOTICES.md"
{
  echo "# Third-Party Notices"
  echo
  echo "This inventory is generated from the Go build information embedded in the"
  echo "accompanying \`breyta\` executable. License texts are retained under"
  echo "\`licenses/go/\`. The archive also contains the separately audited"
  echo "\`parinfer-rust\` provenance, notices, and license material."
  echo
  echo "| Go module | Version | License | Module checksum |"
  echo "| --- | --- | --- | --- |"
} > "${notice_file}"

while IFS=$'\t' read -r module version embedded_sum; do
  policy_row="$(awk -F '\t' -v module="${module}" '$1 == module { print; found++ } END { if (found != 1) exit 1 }' "${policy_file}")" || {
    echo "compiled Go module has no unique license policy entry: ${module}" >&2
    exit 1
  }
  IFS=$'\t' read -r _ license license_file <<< "${policy_row}"

  module_json="$(go mod download -json "${module}@${version}")"
  module_dir="$(jq -er '.Dir' <<< "${module_json}")"
  resolved_sum="$(jq -er '.Sum' <<< "${module_json}")"
  if [[ "${embedded_sum}" != "${resolved_sum}" ]]; then
    echo "module checksum mismatch for ${module}@${version}" >&2
    exit 1
  fi
  if [[ ! -f "${module_dir}/${license_file}" ]]; then
    echo "missing approved license file for ${module}@${version}: ${license_file}" >&2
    exit 1
  fi

  safe_name="$(sed 's#[^A-Za-z0-9._-]#_#g' <<< "${module}")"
  cp "${module_dir}/${license_file}" "${output_dir}/licenses/go/${safe_name}-${version}.txt"
  printf '| `%s` | `%s` | %s | `%s` |\n' \
    "${module}" "${version}" "${license}" "${resolved_sum}" >> "${notice_file}"
done < "${tmp_dir}/artifact-modules.tsv"

echo "Generated artifact-derived Go notices: ${notice_file}"
