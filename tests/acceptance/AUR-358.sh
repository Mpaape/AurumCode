#!/usr/bin/env bash
set -euo pipefail
manifest='tests/characterization/legacy/env-examples/manifest.tsv'
[[ -s "$manifest" && ! -L "$manifest" ]] || { printf 'AUR-358/AC-001: env-example manifest absent\n' >&2; exit 1; }
for path in .env.example '.env copy.example'; do grep -Fq "$path" "$manifest" || { printf 'AUR-358/AC-001: env example unmapped\n' >&2; exit 1; }; done
grep -Eq $'\tsecret_values=0\t' "$manifest" || { printf 'AUR-358/AC-001: secret-value proof absent\n' >&2; exit 1; }
grep -Fq $'.env copy.example\tquarantine-read-only' "$manifest" || { printf 'AUR-358/AC-001: spaced alias not quarantined\n' >&2; exit 1; }
printf '{"card":"AUR-358","scenario":"AC-001","result":"pass"}\n'
