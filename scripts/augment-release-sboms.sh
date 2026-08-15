#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dist_dir="${1:-${repo_root}/dist}"
components_file="${repo_root}/tools/parinfer-rust/components.json"

shopt -s nullglob
archives=("${dist_dir}"/breyta_*_darwin_*.tar.gz \
          "${dist_dir}"/breyta_*_linux_*.tar.gz \
          "${dist_dir}"/breyta_*_windows_*.zip)
if [[ "${#archives[@]}" -ne 5 ]]; then
  echo "expected five release archives, found ${#archives[@]}" >&2
  exit 1
fi

for archive in "${archives[@]}"; do
  archive_name="$(basename "${archive}")"
  case "${archive_name}" in
    *_darwin_amd64.tar.gz) target=darwin/amd64; parinfer_name=parinfer-rust ;;
    *_darwin_arm64.tar.gz) target=darwin/arm64; parinfer_name=parinfer-rust ;;
    *_linux_amd64.tar.gz) target=linux/amd64; parinfer_name=parinfer-rust ;;
    *_linux_arm64.tar.gz) target=linux/arm64; parinfer_name=parinfer-rust ;;
    *_windows_amd64.zip) target=windows/amd64; parinfer_name=parinfer-rust.exe ;;
    *) echo "unsupported release archive: ${archive_name}" >&2; exit 1 ;;
  esac

  helper_file="$(mktemp "${TMPDIR:-/tmp}/breyta-parinfer-artifact.XXXXXX")"
  sbom="${archive}.spdx.json"
  augmented="${sbom}.tmp"
  trap 'rm -f "${helper_file}" "${augmented}"' EXIT
  if [[ "${archive}" == *.zip ]]; then
    unzip -p "${archive}" "${parinfer_name}" > "${helper_file}"
  else
    tar -xOf "${archive}" "${parinfer_name}" > "${helper_file}"
  fi

  helper_hash="$(shasum -a 256 "${helper_file}" | awk '{print $1}')"
  expected_hash="$(awk -F= -v target="${target}/${parinfer_name}" \
    '$1 == target { print $2 }' "${repo_root}/tools/parinfer-rust/SOURCE.lock")"
  if [[ -z "${expected_hash}" || "${helper_hash}" != "${expected_hash}" ]]; then
    echo "cannot augment SBOM for unpinned helper in ${archive_name}" >&2
    exit 1
  fi
  if [[ ! -f "${sbom}" ]]; then
    echo "missing Syft SPDX document for ${archive_name}" >&2
    exit 1
  fi

  jq --arg helper_hash "${helper_hash}" --slurpfile cargo "${components_file}" '
    def spdx_license:
      gsub("Apache-2.0 / MIT"; "Apache-2.0 OR MIT")
      | gsub("/"; " OR ");
    ($cargo[0].packages | to_entries | map({
      SPDXID: ("SPDXRef-Package-parinfer-cargo-" + (.key | tostring)),
      name: .value.name,
      versionInfo: .value.version,
      downloadLocation: ("https://crates.io/api/v1/crates/" + .value.name + "/" + .value.version + "/download"),
      filesAnalyzed: false,
      licenseConcluded: (.value.license | spdx_license),
      licenseDeclared: (.value.license | spdx_license),
      copyrightText: "NOASSERTION",
      externalRefs: [{
        referenceCategory: "PACKAGE-MANAGER",
        referenceType: "purl",
        referenceLocator: ("pkg:cargo/" + .value.name + "@" + .value.version)
      }]
    })) as $cargo_packages
    | {
        SPDXID: "SPDXRef-Package-parinfer-rust",
        name: "parinfer-rust",
        versionInfo: "0.4.3",
        downloadLocation: "https://github.com/eraserhd/parinfer-rust/tree/7b67f166e8ad12899903a0ea451333c41ed80544",
        filesAnalyzed: false,
        checksums: [{algorithm: "SHA256", checksumValue: $helper_hash}],
        licenseConcluded: "ISC",
        licenseDeclared: "ISC",
        copyrightText: "Copyright (c) 2018 Jason Felice",
        externalRefs: [{
          referenceCategory: "PACKAGE-MANAGER",
          referenceType: "purl",
          referenceLocator: "pkg:cargo/parinfer_rust@0.4.3"
        }]
      } as $parinfer
    | .packages += ([$parinfer] + $cargo_packages)
    | .documentDescribes += ["SPDXRef-Package-parinfer-rust"]
    | .relationships += ($cargo_packages | map({
        spdxElementId: "SPDXRef-Package-parinfer-rust",
        relationshipType: "DEPENDS_ON",
        relatedSpdxElement: .SPDXID
      }))
  ' "${sbom}" > "${augmented}"
  mv "${augmented}" "${sbom}"
  rm -f "${helper_file}"
  trap - EXIT
  echo "Added pinned Parinfer Cargo graph to $(basename "${sbom}")"
done
