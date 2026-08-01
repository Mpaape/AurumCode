#!/usr/bin/env bash
set -euo pipefail

readonly manifest='tests/characterization/legacy/docker-docs/manifest.tsv'
[[ -s "$manifest" && ! -L "$manifest" ]] || {
  printf 'AUR-338/AC-001: docker documentation manifest absent\n' >&2
  exit 1
}
for path in .docker/README.md .docker/docs.Dockerfile; do
  grep -Eq "^${path//./\\.}[[:space:]]" "$manifest" || {
    printf 'AUR-338/AC-001: docker surface unmapped\n' >&2
    exit 1
  }
done
grep -Fq $'.docker/docs.Dockerfile\tquarantine-no-build\tstatic_audit=pass' "$manifest" || {
  printf 'AUR-338/AC-001: Dockerfile quarantine proof absent\n' >&2
  exit 1
}
printf '{"card":"AUR-338","scenario":"AC-001","result":"pass"}\n'
