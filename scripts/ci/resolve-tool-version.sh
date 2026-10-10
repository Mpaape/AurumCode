#!/usr/bin/env bash
# Resolves the AurumCode version the image stamps into the binary (AUR-611)
# and writes it to $GITHUB_OUTPUT as version=<v>. Shared by the two product
# builds of review.yml and by providers-smoke.yml.
#
# Inputs (environment): TOOL_SHA, the 40-hex SHA of the tool checkout;
# TOOL_DIR, that checkout; GITHUB_OUTPUT, the step output file.
set -euo pipefail
: "${TOOL_SHA:?TOOL_SHA is required}" "${TOOL_DIR:?TOOL_DIR is required}" "${GITHUB_OUTPUT:?GITHUB_OUTPUT is required}"
# The version the image stamps into the binary (AUR-611): the
# tool's exact release tag when one points at TOOL_SHA, else its
# short SHA. ls-remote reads only the tag list, so the shallow
# checkout stays shallow; a failed lookup falls back to the short
# SHA and never fails the job.
version="${TOOL_SHA:0:12}"
tags="$(git -C "$TOOL_DIR" ls-remote --tags origin 2>/dev/null)" || tags=''
tag="$(printf '%s\n' "$tags" | awk -v sha="$TOOL_SHA" '$1 == sha { print $2 }' |
  sed -e 's|^refs/tags/||' -e 's|\^{}$||' |
  grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sort -uV | tail -n 1)" || tag=''
version="${tag:-$version}"
if [[ ! "$version" =~ ^(v[0-9]+\.[0-9]+\.[0-9]+|[0-9a-f]{12})$ ]]; then
  echo "::error::Cannot resolve the AurumCode version from TOOL_SHA"
  exit 1
fi
echo "version=$version" >> "$GITHUB_OUTPUT"
echo "AurumCode version: $version"
