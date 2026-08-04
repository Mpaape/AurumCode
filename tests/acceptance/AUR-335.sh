#!/usr/bin/env bash
set -euo pipefail

readonly manifest='tests/characterization/legacy/config-yaml/manifest.tsv'
[[ -s "$manifest" && ! -L "$manifest" ]] || {
  printf 'AUR-335/AC-001: YAML config manifest absent\n' >&2
  exit 1
}
for path in configs/default-config.yml configs/.aurumcode/config.example.yml; do
  grep -Fq "$path" "$manifest" || {
    printf 'AUR-335/AC-001: config surface unmapped\n' >&2
    exit 1
  }
done
grep -Eq $'\tsecret_values=0\t' "$manifest" || {
  printf 'AUR-335/AC-001: secret-value proof absent\n' >&2
  exit 1
}
printf '{"card":"AUR-335","scenario":"AC-001","result":"pass"}\n'
