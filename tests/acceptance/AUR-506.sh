#!/usr/bin/env bash
# Static artifact contract for AUR-506. Browser interaction is covered by
# tests/docs/site.test.cjs in the browser-enabled CI lane.
set -eu
export LC_ALL=C

card=AUR-506
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003) ;;
  *) printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
script_dir=${0%/*}
repo_root=$(CDPATH='' cd -- "$script_dir/../.." && pwd -P) || fail repo-root
config_source=$repo_root/internal/config/config.go
site=$repo_root/docs/site/index.html
app=$repo_root/docs/site/app.js
browser_test=$repo_root/tests/docs/site.test.cjs
for file in "$config_source" "$site" "$app" "$browser_test"; do
  [ -f "$file" ] || fail "missing:$file"
done

# Read the canonical value from the product declaration, rather than keeping a
# second hard-coded authority in this check. Require exactly one declaration.
config_path=$(awk '
  /^const DefaultConfigPath = "/ {
    line = $0
    sub(/^const DefaultConfigPath = "/, "", line)
    sub(/".*$/, "", line)
    print line
    count++
  }
  END { if (count != 1) exit 1 }
' "$config_source") || fail default-config-path-declaration
[ -n "$config_path" ] || fail empty-default-config-path

grep -Fq "salve o YAML gerado em <code>$config_path</code>" "$site" || fail install-destination
grep -Fq "<span>$config_path</span>" "$site" || fail config-artifact-label
grep -Fq "$config_path" "$app" || fail javascript-default-fallback
if grep -Fq '.aurumcode.yaml' "$site" "$app"; then fail obsolete-config-path; fi

grep -Fq 'preset de exemplo' "$site" || fail preset-not-explained
grep -Fq 'não com os padrões' "$site" || fail preset-default-distinction
grep -Fq 'inglês, comentários na conversa e sem sugestões inline' "$site" || fail actual-defaults-not-explained
grep -Fq 'assert.equal((await configCode.textContent()).trim()' "$browser_test" || fail browser-config-selection-assertion
grep -Fq 'navigator.clipboard.readText()' "$browser_test" || fail browser-config-copy-assertion
grep -Fq 'javaScriptEnabled: false' "$browser_test" || fail browser-no-js-coverage

printf '{"card":"%s","scenario":"AC-001","selector":"%s","config_path":"%s","result":"pass"}\n' \
  "$card" "$selector" "$config_path"
