#!/usr/bin/env bash
# AUR-469 acceptance: a configured MCP context source reaches the review
# with its origin and clear limits. Only the declared, redacted payload is
# sent; the answer enters the repository-context slot as mcp:<name>/<tool>;
# an absent, slow or malformed server is an omission warning and the review
# goes on; a hostile answer changes neither gate nor exit; only trusted
# configuration (policy, local config, PR base) declares a source.
#
# Selectors:
#   all        AC-001..AC-003, then MUT-001
#   AC-001     declared and redacted payload; answer in the prompt with origin
#   AC-002     absent, slow, malformed server: warning, review continues
#   AC-003     hostile answer leaves gate and exit; only trusted config
#   MUT-001    sending an argument unredacted turns AC-001 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-469'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum cmd internal pkg cmd/aurumcode/aur469_test.go internal/context/mcp/source.go internal/context/mcp/aur469_test.go internal/config/mcpcontext.go; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a469.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP
mkdir -p "$run_dir/gotmp"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1 -buildvcs=false'
: "${GOCACHE:=$run_dir/gocache}"
export GOCACHE GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

# stage copies the whole module (never enumerated packages) to a fresh root.
stage() {
  local root="$1" source
  mkdir -p "$root"
  for source in go.mod go.sum cmd internal pkg docs demo; do
    if [[ -e "$repo_root/$source" ]]; then cp -R "$repo_root/$source" "$root/$source"; fi
  done
  for source in tests/fixtures tests/e2e; do
    if [[ -d "$repo_root/$source" ]]; then mkdir -p "$root/tests"; cp -R "$repo_root/$source" "$root/$source"; fi
  done
  chmod -R u+w -- "$root"
}

# go_test root log pattern pkgs... runs the named tests; rc is go's.
go_test() {
  local root="$1" log="$2" pattern="$3"; shift 3
  ( cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "$@" ) >"$log" 2>&1
}

# require_pass log names... fails unless every named test passed.
require_pass() {
  local log="$1" name; shift
  for name in "$@"; do
    grep -Eq -- "^--- PASS: ${name} " "$log" || { cat "$log" >&2; fail "missing-pass:$name"; }
  done
}

# replace_once file anchor replacement: a literal, unique edit.
replace_once() {
  local file="$1" anchor="$2" replacement="$3" count
  count="$(grep -Fc -- "$anchor" "$file")" || infra "anchor-missing:${file##*/}"
  [[ "$count" == 1 ]] || infra "anchor-not-unique:${file##*/}"
  ANCHOR="$anchor" REPL="$replacement" awk '
    BEGIN { a = ENVIRON["ANCHOR"]; r = ENVIRON["REPL"] }
    { i = index($0, a); if (i > 0) { print substr($0, 1, i - 1) r substr($0, i + length(a)) } else { print } }
  ' "$file" >"$file.mut" && mv "$file.mut" "$file"
  grep -Fq -- "$replacement" "$file" || infra "mutation-not-applied:${file##*/}"
}

readonly ac001_tests=(TestAUR469DeclaredRedactedPayloadAndOrigin TestAUR469NothingUndeclaredIsSent TestAUR469AnswerInPromptWithOriginAndNoGateEffect)
readonly ac002_tests=(TestAUR469AbsentSlowAndMalformedServersAreErrors TestAUR469AbsentServerWarnsAndReviewContinues TestAUR480ProviderFailureWarnsAndContinues)
readonly ac003_tests=(TestAUR469AnswerInPromptWithOriginAndNoGateEffect TestAUR469OnlyTrustedConfigurationDeclaresASource)
readonly pkgs=(./cmd/aurumcode/ ./internal/context/mcp/ ./internal/config/)

pattern_of() { local IFS='|'; printf '^(%s)$' "$*"; }

run_ac() {
  local name="$1"; shift
  local root="$run_dir/root-$name" log="$run_dir/$name.log"
  stage "$root"
  go_test "$root" "$log" "$(pattern_of "$@")" "${pkgs[@]}" || { cat "$log" >&2; fail go-test-failed; }
  require_pass "$log" "$@"
  printf '%s/%s/pass\n' "$card" "$name"
}

# expect_red root log tests...: the mutated copy must compile and turn at
# least one named test RED by its assertion, never by a build error.
expect_red() {
  local root="$1" log="$2"; shift 2
  if go_test "$root" "$log" "$(pattern_of "$@")" "${pkgs[@]}"; then
    cat "$log" >&2; fail mutation-survived
  fi
  if grep -Eq 'build failed|cannot use|undefined:|syntax error|declared and not used' "$log"; then
    cat "$log" >&2; fail mutation-did-not-compile
  fi
  grep -Eq -- '^--- FAIL: ' "$log" || { cat "$log" >&2; fail mutation-not-behavioral; }
  grep -E -- '^--- FAIL: |_test\.go:[0-9]+:' "$log" | sed -n '1,4p' >&2
}

# MUT-001: a declared argument is sent without redaction.
run_mut001() {
  local root="$run_dir/root-mut1"
  stage "$root"
  replace_once "$root/internal/context/mcp/source.go" \
    'out[k] = s.Redact(v)' \
    'out[k] = v // MUT-001: unredacted payload'
  expect_red "$root" "$run_dir/mut1.log" "${ac001_tests[@]}"
  grep -Eq -- '^--- FAIL: TestAUR469DeclaredRedactedPayloadAndOrigin' "$run_dir/mut1.log" || fail mut001-wrong-test
  printf '%s/MUT-001/rejected\n' "$card"
}

case "$selector" in
  AC-001) run_ac AC-001 "${ac001_tests[@]}" ;;
  AC-002) run_ac AC-002 "${ac002_tests[@]}" ;;
  AC-003) run_ac AC-003 "${ac003_tests[@]}" ;;
  MUT-001) run_mut001 ;;
  all)
    run_ac AC-001 "${ac001_tests[@]}"
    run_ac AC-002 "${ac002_tests[@]}"
    run_ac AC-003 "${ac003_tests[@]}"
    run_mut001
    ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
