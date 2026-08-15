#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
output_root="${repo_root}/.release-compliance"
tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/breyta-release-probes.XXXXXX")"
trap 'rm -rf "${tmp_dir}"' EXIT

rm -rf "${output_root}"

for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64; do
  goos="${target%/*}"
  goarch="${target#*/}"
  binary="${tmp_dir}/breyta-${goos}-${goarch}"
  if [[ "${goos}" == "windows" ]]; then
    binary="${binary}.exe"
  fi

  CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
    go build -trimpath -o "${binary}" ./cmd/breyta
  "${repo_root}/scripts/generate-go-compliance.sh" \
    "${binary}" "${output_root}/${goos}/${goarch}"
done

echo "Prepared release compliance for every supported CLI target"
