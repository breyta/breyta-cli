#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <checked-out-parinfer-rust-source>" >&2
  exit 2
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source_dir="$(cd "$1" && pwd)"
lock_file="${repo_root}/tools/parinfer-rust/SOURCE.lock"
expected_commit="$(awk -F= '$1 == "commit" { print $2 }' "${lock_file}")"

if [[ "$(git -C "${source_dir}" rev-parse HEAD)" != "${expected_commit}" ]]; then
  echo "parinfer-rust checkout does not match pinned commit ${expected_commit}" >&2
  exit 1
fi

for source_file in Cargo.lock Cargo.toml; do
  cmp "${repo_root}/tools/parinfer-rust/${source_file}" "${source_dir}/${source_file}"
done
cmp "${repo_root}/tools/parinfer-rust/LICENSE.md" "${source_dir}/LICENSE.md"

metadata_file="$(mktemp "${TMPDIR:-/tmp}/breyta-parinfer-metadata.XXXXXX")"
trap 'rm -f "${metadata_file}"' EXIT
cargo metadata --locked --format-version 1 \
  --manifest-path "${source_dir}/Cargo.toml" > "${metadata_file}"

generated_components="$(mktemp "${TMPDIR:-/tmp}/breyta-parinfer-components.XXXXXX")"
trap 'rm -f "${metadata_file}" "${generated_components}"' EXIT
jq -S '{
  schema: 1,
  source: {
    repository: "https://github.com/eraserhd/parinfer-rust.git",
    tag: "v0.4.3",
    commit: "7b67f166e8ad12899903a0ea451333c41ed80544"
  },
  packages: [.packages[] | select(.name != "parinfer_rust") |
    {name, version, license, source}] | sort_by(.name, .version)
}' "${metadata_file}" > "${generated_components}"
diff -u "${repo_root}/tools/parinfer-rust/components.json" "${generated_components}"

package_count="$(jq '[.packages[] | select(.name != "parinfer_rust")] | length' "${metadata_file}")"
notice_count="$(awk -F '|' '/^\| [^ -]/ && $2 !~ /^ Package / { count++ } END { print count + 0 }' \
  "${repo_root}/tools/parinfer-rust/THIRD_PARTY_NOTICES.md")"
if [[ "${package_count}" -ne "${notice_count}" ]]; then
  echo "Cargo notice count ${notice_count} does not match locked graph ${package_count}" >&2
  exit 1
fi

while IFS=$'\t' read -r name version license; do
  if [[ -z "${license}" || "${license}" == "UNKNOWN" ]]; then
    echo "Cargo package has no declared license: ${name}@${version}" >&2
    exit 1
  fi
  case "${license}" in
    *MIT*|*Apache-2.0*|*BSD-2-Clause*|*BSD-3-Clause*|*ISC*|*Unlicense*|*BSL-1.0*) ;;
    *) echo "Cargo package uses an unapproved license: ${name}@${version} (${license})" >&2; exit 1 ;;
  esac
  expected_row="| ${name} | ${version} | ${license} |"
  if ! grep -Fqx "${expected_row}" "${repo_root}/tools/parinfer-rust/THIRD_PARTY_NOTICES.md"; then
    echo "Cargo notices are missing: ${expected_row}" >&2
    exit 1
  fi
done < <(jq -r '.packages[] | select(.name != "parinfer_rust") | [.name, .version, (.license // "UNKNOWN")] | @tsv' \
  "${metadata_file}")

echo "Pinned parinfer-rust source, lock graph, licenses, and notices verified"
