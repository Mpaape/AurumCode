#!/usr/bin/env bash
# AUR-491 acceptance: the documentation lets a new developer install, configure
# and use the CURRENT product without guessing paths, commands or capabilities.
#
# Selectors:
#   all      AC-001..AC-004
#   AC-001   free product + install/minimum config + documented config path
#   AC-002   public option inventory vs the real CLI help + no internal hooks
#   AC-003   review suggestions -> `aurumcode fix` -> applicable unified diff
#   AC-004   each README capability points to a command/tutorial + limitation
#   MUT-001  corrupting the documented config path must make the checks fail
# Unknown selectors exit 64.
set -euo pipefail

selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|MUT-001) ;;
  *) exit 64 ;;
esac

if ! command -v go >/dev/null 2>&1; then
  echo "AUR-491: go runtime is unavailable in this profile" >&2
  exit 79
fi

repo_root="$(cd -- "${BASH_SOURCE[0]%/*}/../.." && pwd -P)"
scratch="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a491.XXXXXX")"
cleanup() {
  chmod -R u+w -- "$scratch" 2>/dev/null || true
  rm -rf -- "$scratch"
}
trap cleanup EXIT INT TERM HUP

fail() {
  echo "AUR-491: $*" >&2
  exit 1
}

cli_built=0
build_cli() {
  (( cli_built == 0 )) || return 0
  go build -o "$scratch/aurumcode" ./cmd/aurumcode >"$scratch/build.log" 2>&1 ||
    { cat "$scratch/build.log" >&2; fail "building the product CLI failed"; }
  cli_built=1
}

docroot="$repo_root"
if [[ "$selector" == MUT-001 ]]; then
  docroot="$scratch/mut"
  mkdir -p "$docroot/docs"
  cp -- "$repo_root/README.md" "$docroot/README.md"
  for f in "$repo_root"/docs/*.md; do cp -- "$f" "$docroot/docs/"; done
  sed -i 's#\.aurumcode/config\.yml#.aurumcode/config.yaml#g' \
    "$docroot/README.md" "$docroot/docs/configuration.md"
fi

# The documented config path must be exactly config.DefaultConfigPath. The
# product value is obtained by executing the real package, not by grepping it.
check_config_path() {
  local documented probe_module
  documented="$(grep -hoE -e '\.aurumcode/[A-Za-z0-9._-]+\.ya?ml' \
    "$docroot/README.md" "$docroot"/docs/*.md 2>/dev/null | sort -u || true)"
  [[ -n "$documented" ]] || fail "no documented .aurumcode YAML path was found"

  probe_module="$scratch/probe-module"
  mkdir -p "$probe_module/cmd/probe"
  cp -R "$repo_root/go.mod" "$repo_root/go.sum" "$repo_root/internal" "$repo_root/pkg" "$probe_module/"
  cat > "$probe_module/cmd/probe/main.go" <<'GO'
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
)

func main() {
	bad := false
	for _, want := range strings.Split(os.Getenv("AUR491_CONFIG_PATHS"), "\n") {
		if want == "" {
			continue
		}
		if want != config.DefaultConfigPath {
			fmt.Fprintf(os.Stderr, "documented config path %q != product path %q\n", want, config.DefaultConfigPath)
			bad = true
		}
	}
	if bad {
		os.Exit(1)
	}
	fmt.Print(config.DefaultConfigPath)
}
GO
  ( cd "$probe_module" && AUR491_CONFIG_PATHS="$documented" go run ./cmd/probe ) \
    >"$scratch/product-path" 2>"$scratch/probe.err" ||
    { cat "$scratch/probe.err" >&2; fail "a documented config path does not match config.DefaultConfigPath"; }
}

# Every flag documented in the consumer pages must exist in the real help text.
# Lines that invoke `docker run` are skipped so container flags such as --rm are
# never confused with product flags.
check_documented_flags() {
  build_cli
  "$scratch/aurumcode" --help >"$scratch/help-top" 2>&1 || fail "top-level --help failed"
  "$scratch/aurumcode" review --help >"$scratch/help-review" 2>&1 || fail "review --help failed"
  "$scratch/aurumcode" fix --help >"$scratch/help-fix" 2>&1 || fail "fix --help failed"
  # AUR-573: sbom/sign/xbom are product subcommands with their own flags and
  # the consumer docs document them; include their help (it widens the check).
  local sub
  for sub in sbom sign xbom; do
    "$scratch/aurumcode" "$sub" --help >>"$scratch/help-fix" 2>&1 || true
  done

  local documented flag single missing=""
  # AUR-573: fenced code blocks only count when they are `aurumcode ...`
  # invocations; a block for another tool (`cosign verify-blob --bundle ...`)
  # documents that tool's flags, not the product's. Prose outside fences is
  # still checked in full.
  documented="$(awk '
    /^[ \t]*```/ { if (infence) { if (block ~ /(^|[ \t])aurumcode([ \t]|$)/) printf "%s", block; infence=0; block="" } else { infence=1; block="" } ; next }
    infence { block = block $0 "\n"; next }
    { print }
  ' "$docroot/README.md" "$docroot/docs/getting-started.md" \
    "$docroot/docs/configuration.md" "$docroot/docs/review-quality.md" 2>/dev/null |
    grep -vE -e 'docker run' |
    grep -oE -e '--[a-z][a-z0-9-]+' | sort -u || true)"
  [[ -n "$documented" ]] || fail "no documented CLI flag was found"
  while IFS= read -r flag; do
    [[ -n "$flag" ]] || continue
    [[ "$flag" == "--help" ]] && continue
    # AUR-573: flags of third-party tools that the docs show in prose or in
    # non-aurumcode examples (cosign verify-blob, trivy, semgrep) are not
    # product flags.
    case "$flag" in
      --certificate-identity-regexp|--certificate-oidc-issuer|--bundle|--key|--format|--json|--output) continue ;;
    esac
    # Go's flag package documents its flags with a single dash ("-base"), while
    # the consumer documentation uses the double-dash spelling both parsers
    # accept. Accept either as long as the real help names the flag.
    single="-${flag#--}"
    grep -qF -e "$flag" "$scratch/help-top" "$scratch/help-review" "$scratch/help-fix" ||
      grep -qF -e "$single" "$scratch/help-top" "$scratch/help-review" "$scratch/help-fix" ||
      missing="$missing $flag"
  done <<<"$documented"
  [[ -z "$missing" ]] || fail "documented flags absent from the product help:$missing"
}

# Test-only hooks are not part of the product surface.
check_internal_hooks() {
  local hook
  for hook in AURUMCODE_LLM_FIXTURE AURUMCODE_PROMPT_CAPTURE AURUMCODE_SECRET_CANARY; do
    if grep -qF -e "$hook" \
      "$docroot/README.md" "$docroot/docs/getting-started.md" \
      "$docroot/docs/configuration.md" "$docroot/docs/review-quality.md"; then
      fail "internal test-only variable $hook leaked into consumer documentation"
    fi
  done
}

check_options_inventory() {
  grep -qF -e 'Opções públicas' "$docroot/docs/configuration.md" ||
    fail "configuration.md lacks the inventory of public options"
}

check_ac001() {
  grep -qiE -e 'gratuit' "$docroot/README.md" || fail "README does not state the product is free"
  grep -qF -e 'docs/getting-started.md' "$docroot/README.md" ||
    fail "README does not link the getting-started guide"
  grep -qF -e 'LLM_API_KEY' "$docroot/docs/getting-started.md" ||
    fail "getting-started lacks LLM_API_KEY"
  grep -qF -e 'LLM_BASE_URL' "$docroot/docs/getting-started.md" ||
    fail "getting-started lacks LLM_BASE_URL"
  grep -qiE -e 'hist[oó]rica' "$docroot/docs/getting-started.md" ||
    fail "getting-started does not distinguish historical tags"
  check_config_path
}

check_ac002() {
  check_options_inventory
  check_documented_flags
  check_internal_hooks
}

check_ac003() {
  build_cli
  local work="$scratch/fix-work"
  mkdir -p "$work"
  cat > "$work/app.go" <<'GO'
package main

func main() {
	dbPassword := "hunter2"
}
GO
  cat > "$work/good.json" <<'JSON'
[{"title":"Carregar a senha do loader","description":"Evita o segredo inline","kind":"code","file":"app.go","line":4,"current_code":"\tdbPassword := \"hunter2\"","proposed_code":"\tdbPassword := loadPassword()"}]
JSON
  cat > "$work/stale.json" <<'JSON'
[{"title":"Sugestao obsoleta","kind":"code","file":"app.go","line":2,"current_code":"return err","proposed_code":"return nil"}]
JSON

  local rc=0
  ( cd "$work" && "$scratch/aurumcode" fix --file good.json ) \
    >"$scratch/good.patch" 2>"$scratch/good.err" || rc=$?
  [[ "$rc" == 0 ]] || { cat "$scratch/good.err" >&2; fail "fix on an applicable suggestion exited $rc"; }
  grep -qF -e '--- a/app.go' "$scratch/good.patch" || fail "fix patch lacks the old-file header"
  grep -qF -e '+++ b/app.go' "$scratch/good.patch" || fail "fix patch lacks the new-file header"
  grep -qE -e '^@@ ' "$scratch/good.patch" || fail "fix patch lacks a hunk header"
  grep -qF -e '-	dbPassword := "hunter2"' "$scratch/good.patch" || fail "fix patch lacks the removed line"
  grep -qF -e '+	dbPassword := loadPassword()' "$scratch/good.patch" || fail "fix patch lacks the added line"

  rc=0
  ( cd "$work" && "$scratch/aurumcode" fix --file stale.json ) \
    >"$scratch/stale.patch" 2>"$scratch/stale.err" || rc=$?
  [[ "$rc" == 1 ]] || fail "a stale suggestion exited $rc instead of 1"
  [[ ! -s "$scratch/stale.patch" ]] || fail "a stale suggestion still printed a patch"
}

check_ac004() {
  grep -qF -e 'docs/getting-started.md' "$docroot/README.md" || fail "README does not link getting-started"
  grep -qF -e 'docs/configuration.md' "$docroot/README.md" || fail "README does not link configuration"
  grep -qF -e 'docs/review-quality.md' "$docroot/README.md" || fail "README does not link review-quality"
  grep -qF -e 'aurumcode review' "$docroot/README.md" || fail "README does not show the review command"
  grep -qF -e 'aurumcode fix' "$docroot/README.md" || fail "README does not show the fix command"
  grep -qiE -e 'fork' "$docroot/README.md" || fail "README does not state the fork limitation"
  grep -qiE -e 'mem[oó]ria|memory' "$docroot/README.md" || fail "README does not cover memory"
  grep -qE -e '(^|[^A-Za-z])CI([^A-Za-z]|$)' "$docroot/README.md" || fail "README does not cover CI"
  grep -qiE -e 'custo|--limite' "$docroot/README.md" || fail "README does not cover cost"
  grep -qiE -e 'OpenAI' "$docroot/README.md" || fail "README does not cover provider neutrality"
  grep -qiE -e 'capacidades' "$docroot/README.md" || fail "README lacks a capabilities section"
}

doc_checks() {
  check_ac001
  check_ac002
  check_ac004
}

case "$selector" in
  all)
    doc_checks
    check_ac003
    ;;
  AC-001) check_ac001 ;;
  AC-002) check_ac002 ;;
  AC-003) check_ac003 ;;
  AC-004) check_ac004 ;;
  MUT-001)
    # Corrupting the documented config path must break at least one real check.
    if ( doc_checks ); then
      fail "MUT-001 not detected: the documentation checks still passed"
    fi
    printf 'AUR-491/MUT-001/pass (mutation detected)\n'
    exit 0
    ;;
esac

printf 'AUR-491/%s/pass\n' "$selector"
