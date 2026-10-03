#!/usr/bin/env bash
#
# Acceptance program for card AUR-582: the embedded hardcoded-secret rule of
# internal/analysis reads the public gitleaks default rule base as pinned
# data (the JSON derivation of the TOML at the digest of the scanners lock;
# the TOML is not committed and credential-shaped allowlist literals live
# only as sha256), not a regex written in Go.
#
# Selectors:
#   all      AC-001, AC-002, AC-003, AC-004, AC-005, MUT-001, MUT-002
#   AC-001   AWS and GitHub token lines give two analysis/hardcoded-secret
#            findings naming the base rule id, never the value
#   AC-002   allowlist (stopword, EXAMPLE key, hashed GCP example key,
#            lock-file path) and entropy suppress; the long-standing
#            keyword-assignment cases still hold
#   AC-003   source digest recorded in the derived catalog == scanners lock;
#            one flipped byte in the catalog, or a different lock digest,
#            turns the tests red
#   AC-004   no secret-shaped regex compiled in Go (grep + go/ast test, and
#            the go/ast test goes red when one is planted)
#   AC-005   no tracked (or, without git, materialized) file carries a
#            vendor credential-shaped literal
#   MUT-001  entropy minimum ignored: AC-002 must go red
#   MUT-002  loader accepts a divergent digest: AC-003 must go red
#
# Exit codes: 0 holds, 1 behavioral RED, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-582'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-005|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

readonly lock_rel=.board/bootstrap/locks/scanners.yml
readonly pkg=internal/analysis
for input in go.mod go.sum cmd internal pkg "$lock_rel" "$pkg/rules/secrets.json" "$pkg/rules/secrets.json.sha256"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a582.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP
mkdir -p "$run_dir/gotmp"
: "${GOCACHE:=$run_dir/gocache}"
export GOCACHE
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1' GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

seed_root() { # seed_root DIR
  rm -rf "$1"; mkdir -p "$1/.board/bootstrap/locks"
  local s
  for s in go.mod go.sum cmd internal pkg; do cp -R "$repo_root/$s" "$1/$s"; done
  cp "$repo_root/$lock_rel" "$1/$lock_rel"
  chmod -R u+w -- "$1"
}

# run_tests DIR REGEX: runs the analysis package tests matching REGEX with
# the scanners lock wired in; returns go test's status, log in $run_dir/test.log.
run_tests() {
  (cd "$1" && AURUMCODE_SCANNERS_LOCK="$1/$lock_rel" go test -buildvcs=false -count=1 -run "$2" "./$pkg/") >"$run_dir/test.log" 2>&1
}

expect_green() { # expect_green DIR REGEX LABEL
  run_tests "$1" "$2" || { tail -n 30 "$run_dir/test.log" >&2; fail "$3"; }
  grep -E '^(ok|---)' "$run_dir/test.log" >&2 || true
}

expect_red() { # expect_red DIR REGEX LABEL WITNESS
  if run_tests "$1" "$2"; then fail "$3/survived"; fi
  grep -Fq -- "$4" "$run_dir/test.log" || { tail -n 30 "$run_dir/test.log" >&2; fail "$3/red-for-wrong-reason"; }
  grep -F -- "$4" "$run_dir/test.log" | sed -n '1p' >&2
}

mutate() { # mutate FILE ANCHOR REPLACEMENT
  [[ "$(grep -cF -- "$2" "$1")" == 1 ]] || infra "anchor-not-unique:${1##*/}"
  local content
  content="$(<"$1")"
  printf '%s\n' "${content/"$2"/"$3"}" >"$1"
  grep -qF -- "$3" "$1" || infra "mutation-not-applied:${1##*/}"
}

readonly ac002_tests='TestSecretAllowlistAndEntropy|TestHashedAllowlistSuppressesExampleKey|TestAnalyzeTable|TestAnalyzeDeterministic|TestNewRunnerZeroConfig|TestAUR4'
readonly ac003_tests='TestEmbeddedCatalogMatchesRecordedDigest|TestSecretCatalogMatchesScannersLock|TestSecretCatalogRejectsDivergentDigest|TestSkippedSecretRulesAreDeclared'

ac_001() {
  seed_root "$run_dir/root"
  expect_green "$run_dir/root" 'TestSecretVendorFormatsReportRuleIDWithoutValue' AC-001
}

ac_002() {
  seed_root "$run_dir/root"
  expect_green "$run_dir/root" "$ac002_tests" AC-002
}

ac_003() {
  seed_root "$run_dir/root"
  expect_green "$run_dir/root" "$ac003_tests" AC-003
  # The lock test must have run, not skipped.
  (cd "$run_dir/root" && AURUMCODE_SCANNERS_LOCK="$run_dir/root/$lock_rel" go test -buildvcs=false -count=1 -v -run TestSecretCatalogMatchesScannersLock "./$pkg/") >"$run_dir/lock.log" 2>&1 || fail AC-003/lock-test
  grep -q '^--- PASS: TestSecretCatalogMatchesScannersLock' "$run_dir/lock.log" || fail AC-003/lock-test-did-not-run
  # A lock pinning another rule base: the catalog no longer matches it.
  seed_root "$run_dir/root"
  mutate "$run_dir/root/$lock_rel" 'secrets_rulebase_sha256: sha256:e' 'secrets_rulebase_sha256: sha256:f'
  expect_red "$run_dir/root" "$ac003_tests" AC-003/lock 'catalog records'
  # One flipped byte in the derived catalog (a stopword of the
  # generic-api-key allowlist): the artifact digest refuses it.
  seed_root "$run_dir/root"
  mutate "$run_dir/root/$pkg/rules/secrets.json" '"6fe4476ee5a1832882e326b506d14126"' '"6fe4476ee5a1832882e326b506d14127"'
  expect_red "$run_dir/root" "$ac003_tests" AC-003/json-byte 'artifact digest'
}

ac_004() {
  local hits
  hits="$(grep -nE 'regexp\.(MustCompile|Compile)\(`[^`]*(passw|secret|token|api[_-]?key|credential|AKIA|ghp_)' "$repo_root/$pkg"/*.go | grep -v '_test\.go:' || true)"
  [[ -z "$hits" ]] || { printf '%s\n' "$hits" >&2; fail AC-004/grep; }
  seed_root "$run_dir/root"
  expect_green "$run_dir/root" 'TestNoSecretRegexCompiledInGo' AC-004
  # The detector itself must catch a planted Go-compiled secret regex.
  printf 'package analysis\n\nimport "regexp"\n\nvar plantedSecretRe = regexp.MustCompile(`(?i)password\\s*=`)\n' >"$run_dir/root/$pkg/planted.go"
  expect_red "$run_dir/root" 'TestNoSecretRegexCompiledInGo' AC-004/detector 'secret-shaped regex compiled in Go'
}

readonly credential_shapes='AIza[0-9A-Za-z_-]{20,}|ghp_[0-9A-Za-z]{20,}|xox[baprs]-[0-9A-Za-z-]{10,}|AKIA[0-9A-Z]{16}|sk_live_[0-9a-zA-Z]{10,}|glpat-[0-9A-Za-z_-]{10,}|BEGIN [A-Z ]*PRIVATE KEY'

ac_005() {
  local hits
  if command -v git >/dev/null 2>&1 && git -C "$repo_root" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    hits="$(cd "$repo_root" && git ls-files -z | xargs -0 grep -lE -- "$credential_shapes" || true)"
  else
    hits="$(grep -rlE -- "$credential_shapes" "$repo_root" || true)"
  fi
  [[ -z "$hits" ]] || { printf '%s\n' "$hits" >&2; fail AC-005/credential-shaped-literal; }
  printf 'AC-005: no credential-shaped literal\n' >&2
}

mut_001() {
  seed_root "$run_dir/root"
  mutate "$run_dir/root/$pkg/secretscan.go" 'if r.entropy != 0 && shannonEntropy(cand.secret) <= r.entropy {' 'if false && shannonEntropy(cand.secret) <= r.entropy {'
  expect_red "$run_dir/root" "$ac002_tests" MUT-001 'low-entropy'
}

mut_002() {
  seed_root "$run_dir/root"
  mutate "$run_dir/root/$pkg/secretcatalog.go" 'got != want {' 'false && got != want {'
  expect_red "$run_dir/root" "$ac003_tests" MUT-002 'was accepted'
}

case "$selector" in
  AC-001) ac_001 ;;
  AC-002) ac_002 ;;
  AC-003) ac_003 ;;
  AC-004) ac_004 ;;
  AC-005) ac_005 ;;
  MUT-001) mut_001 ;;
  MUT-002) mut_002 ;;
  all) ac_001; ac_002; ac_003; ac_004; ac_005; mut_001; mut_002 ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
