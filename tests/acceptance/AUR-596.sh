#!/usr/bin/env bash
# AUR-596 acceptance: known LLM providers work by configuration. A
# declarative catalog of provider profiles (internal/llm/provider/profiles)
# selected by LLM_PROVIDER drives the one OpenAI-compatible engine; without
# LLM_PROVIDER the request is byte for byte the previous one.
#
# Selectors:
#   all        AC-001..AC-006, then MUT-001..MUT-003
#   AC-001     no LLM_PROVIDER: same body, path and headers as before
#   AC-002     every catalog profile meets its documented dialect against a
#              fake endpoint and a real review concludes; docs cover each
#   AC-003     max_completion_tokens never comes with max_tokens; Azure sends
#              api-key and api-version, never Authorization
#   AC-004     an answer outside the schema is inconclusive, never approval
#   AC-005     provider error echoing the key: redacted, provider named
#   AC-006     unknown LLM_PROVIDER fails listing the valid profiles
#   MUT-001    ignoring the profile's token field turns AC-003 RED
#   MUT-002    accepting an out-of-schema answer as no findings turns AC-004 RED
#   MUT-003    sending Authorization with api-key turns AC-003 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-596'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-005|AC-006|MUT-001|MUT-002|MUT-003) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum cmd internal pkg docs/provedores.md internal/llm/provider/profiles/profiles.yml internal/llm/provider/litellm/dialect.go internal/prompt/parser.go cmd/aurumcode/aur596_test.go; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a596.XXXXXX")" || infra mktemp
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
  for source in go.mod go.sum cmd internal pkg; do
    if [[ -e "$repo_root/$source" ]]; then cp -R "$repo_root/$source" "$root/$source"; fi
  done
  if [[ -d "$repo_root/tests/fixtures" ]]; then mkdir -p "$root/tests"; cp -R "$repo_root/tests/fixtures" "$root/tests/fixtures"; fi
  chmod -R u+w -- "$root"
}

go_test() {
  local root="$1" log="$2" pattern="$3"; shift 3
  ( cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "$@" ) >"$log" 2>&1
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

readonly lite=./internal/llm/provider/litellm/
readonly prof=./internal/llm/provider/profiles/
readonly cmdp=./cmd/aurumcode/

pattern_of() { local IFS='|'; printf '^(%s)$' "$*"; }

# run_tests name pkg tests...: every named test must PASS (a SKIP fails).
run_tests() {
  local name="$1" pkg="$2"; shift 2
  local root="$run_dir/root-$name" log="$run_dir/$name-${pkg//\//_}.log"
  [[ -d "$root" ]] || stage "$root"
  go_test "$root" "$log" "$(pattern_of "$@")" "$pkg" || { cat "$log" >&2; fail "go-test-failed:$name"; }
  if grep -Eq -- '--- SKIP: ' "$log"; then cat "$log" >&2; fail "test-skipped:$name"; fi
  local t
  for t in "$@"; do grep -Eq -- "^--- PASS: ${t} " "$log" || { cat "$log" >&2; fail "missing-pass:$t"; }; done
}

# expect_red root log pkg tests...: the mutated copy compiles and turns a
# named test RED by its assertion, never by a build error.
expect_red() {
  local root="$1" log="$2" pkg="$3"; shift 3
  if go_test "$root" "$log" "$(pattern_of "$@")" "$pkg"; then
    cat "$log" >&2; fail mutation-survived
  fi
  if grep -Eq 'build failed|cannot use|undefined:|syntax error|declared and not used' "$log"; then
    cat "$log" >&2; fail mutation-did-not-compile
  fi
  grep -Eq -- '^--- FAIL: ' "$log" || { cat "$log" >&2; fail mutation-not-behavioral; }
  grep -E -- '^--- FAIL: |_test\.go:[0-9]+:' "$log" | sed -n '1,4p' >&2
}

run_ac001() {
  run_tests AC-001 "$lite" TestDefaultDialectRequestIsUnchanged
  run_tests AC-001 "$prof" TestUnsetProviderKeepsTheLegacySelection
  printf '%s/AC-001/pass\n' "$card"
}

run_ac002() {
  local name
  run_tests AC-002 "$prof" TestEveryProfileSpeaksItsDocumentedDialect
  run_tests AC-002 "$cmdp" TestAUR596EveryProfileConcludesAReview
  for name in $(awk '/^profiles:/{p=1;next} p&&/^  [a-z][a-z-]*:$/{sub(":","",$1);print $1}' "$repo_root/internal/llm/provider/profiles/profiles.yml"); do
    grep -Fq "LLM_PROVIDER=$name" "$repo_root/docs/provedores.md" || fail "profile-not-documented:$name"
  done
  printf '%s/AC-002/pass\n' "$card"
}

run_ac003() {
  run_tests AC-003 "$lite" TestDialectSendsOnlyItsTokenFieldAndAuth
  run_tests AC-003 "$prof" TestEveryProfileSpeaksItsDocumentedDialect
  printf '%s/AC-003/pass\n' "$card"
}

run_ac004() {
  run_tests AC-004 ./internal/prompt/ TestAnswerWithoutFindingsListIsInconclusive
  run_tests AC-004 "$lite" TestDialectWithoutStructuredOutputSendsNoResponseFormat
  run_tests AC-004 "$cmdp" TestAUR596OutOfSchemaAnswerIsInconclusive
  printf '%s/AC-004/pass\n' "$card"
}

run_ac005() {
  run_tests AC-005 "$lite" TestProviderErrorIsSummarisedRedactedAndNamed
  run_tests AC-005 "$cmdp" TestAUR596ProviderErrorIsRedactedAndNamed
  printf '%s/AC-005/pass\n' "$card"
}

run_ac006() {
  run_tests AC-006 "$prof" TestUnknownProfileListsTheValidOnes TestMissingKeyOrPlaceholderFailsClosed
  run_tests AC-006 "$cmdp" TestAUR596UnknownProfileFailsListingTheValidOnes
  printf '%s/AC-006/pass\n' "$card"
}

run_mut001() {
  local root="$run_dir/root-mut1"
  stage "$root"
  replace_once "$root/internal/llm/provider/litellm/dialect.go" \
    'if d.TokenField == TokenFieldMaxCompletionTokens {' \
    'if false && d.TokenField == TokenFieldMaxCompletionTokens { /* MUT-001 */'
  expect_red "$root" "$run_dir/mut1.log" "$lite" TestDialectSendsOnlyItsTokenFieldAndAuth
  expect_red "$root" "$run_dir/mut1-prof.log" "$prof" TestEveryProfileSpeaksItsDocumentedDialect
  printf '%s/MUT-001/rejected\n' "$card"
}

run_mut002() {
  local root="$run_dir/root-mut2"
  stage "$root"
  replace_once "$root/internal/prompt/parser.go" \
    'if !hasFindingsList(jsonContent) {' \
    'if false && !hasFindingsList(jsonContent) { /* MUT-002 */'
  expect_red "$root" "$run_dir/mut2.log" ./internal/prompt/ TestAnswerWithoutFindingsListIsInconclusive
  expect_red "$root" "$run_dir/mut2-cmd.log" "$cmdp" TestAUR596OutOfSchemaAnswerIsInconclusive
  printf '%s/MUT-002/rejected\n' "$card"
}

run_mut003() {
  local root="$run_dir/root-mut3"
  stage "$root"
  replace_once "$root/internal/llm/provider/litellm/dialect.go" \
    'req.Header.Set(d.AuthHeader, apiKey)' \
    'req.Header.Set(d.AuthHeader, apiKey); req.Header.Set("Authorization", "Bearer "+apiKey) /* MUT-003 */'
  expect_red "$root" "$run_dir/mut3.log" "$lite" TestDialectSendsOnlyItsTokenFieldAndAuth
  expect_red "$root" "$run_dir/mut3-prof.log" "$prof" TestEveryProfileSpeaksItsDocumentedDialect
  printf '%s/MUT-003/rejected\n' "$card"
}

case "$selector" in
  AC-001) run_ac001 ;;
  AC-002) run_ac002 ;;
  AC-003) run_ac003 ;;
  AC-004) run_ac004 ;;
  AC-005) run_ac005 ;;
  AC-006) run_ac006 ;;
  MUT-001) run_mut001 ;;
  MUT-002) run_mut002 ;;
  MUT-003) run_mut003 ;;
  all)
    run_ac001
    run_ac002
    run_ac003
    run_ac004
    run_ac005
    run_ac006
    run_mut001
    run_mut002
    run_mut003
    ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
