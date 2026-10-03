#!/usr/bin/env bash
# AUR-533: build the analysis-data artifact and test it. Publishing is a
# separate step (publish.sh) that refuses to run unless this one finished.
#
#   build.sh [dist-dir]
#
# 1. build   - the ecosystem list is read from the public source itself
# 2. verify  - every file digest and the set digest are re-read from disk
# 3. test    - the package tests run against THIS artifact
#              (AURUM_ARTIFACT_DIR); any failure aborts, nothing is marked
# Only after all three does it write "<dist>.tests-passed" (set digest and
# tag), the marker publish.sh requires.
set -Eeuo pipefail
export LC_ALL=C

dist="${1:-dist}"
osv_base="${AURUM_OSV_BASE:-https://osv-vulnerabilities.storage.googleapis.com}"
scanners="${AURUM_SCANNERS_FILE:-.board/bootstrap/locks/scanners.yml}"
tool="${AURUM_ARTIFACTS_BIN:-go run ./cmd/analysis-data}"

rm -rf -- "$dist" "$dist.tests-passed"

# shellcheck disable=SC2086
$tool build --dir "$dist" --osv-base "$osv_base" --scanners "$scanners"
# shellcheck disable=SC2086
$tool verify --dir "$dist"

abs_dist="$(cd "$dist" && pwd -P)"
if [[ -n "${AURUM_ARTIFACT_TEST_CMD:-}" ]]; then
  AURUM_ARTIFACT_DIR="$abs_dist" bash -c "$AURUM_ARTIFACT_TEST_CMD"
else
  AURUM_ARTIFACT_DIR="$abs_dist" go test ./internal/artifacts/... -count=1
fi

# shellcheck disable=SC2086
tag="$($tool tag --dir "$dist")"
# shellcheck disable=SC2086
digest="$($tool digest --dir "$dist")"
[[ -n "$digest" && -n "$tag" ]] || { echo "build.sh: cannot read digest/tag" >&2; exit 1; }
printf '%s\n%s\n' "$digest" "$tag" >"$dist.tests-passed"
echo "analysis-data tests passed: $tag $digest"
