#!/usr/bin/env bash
set -euo pipefail

release_tag="${1:-${GITHUB_REF_NAME:-}}"

if [[ -z "${release_tag}" ]]; then
  echo "usage: $0 <release-tag>" >&2
  exit 1
fi

if [[ ! "${release_tag}" =~ ^v([0-9]{4})\.([0-9]+)\.([0-9]+)$ ]]; then
  echo "release tag must match vYYYY.M.PATCH, got: ${release_tag}" >&2
  exit 1
fi

year="${BASH_REMATCH[1]}"
month_raw="${BASH_REMATCH[2]}"
patch_raw="${BASH_REMATCH[3]}"

month=$((10#${month_raw}))
patch=$((10#${patch_raw}))

if (( month < 1 || month > 12 )); then
  echo "release month must be 1..12, got: ${month_raw}" >&2
  exit 1
fi

semver_minor="$(printf "%04d%02d" "${year}" "${month}")"
module_tag="v0.${semver_minor}.${patch}"
head_sha="$(git rev-parse HEAD)"

if git rev-parse -q --verify "refs/tags/${module_tag}" >/dev/null; then
  tag_sha="$(git rev-list -n 1 "${module_tag}")"
  if [[ "${tag_sha}" != "${head_sha}" ]]; then
    echo "refusing to retarget existing tag ${module_tag} (${tag_sha} != ${head_sha})" >&2
    exit 1
  fi
else
  git tag -a "${module_tag}" -m "Go module compatibility tag for ${release_tag}" "${head_sha}"
fi

echo "${module_tag}"
