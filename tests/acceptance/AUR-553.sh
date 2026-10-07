#!/usr/bin/env bash
# AUR-553 acceptance: every term of the gate-verdict key has a behavioral
# proof that fails when the term is removed, and every finding read back
# from the (untrusted, shareable) review cache passes the model-output
# redaction before any sink. See docs/specs/AUR-553.md.
#
# Selectors:
#   all      every behavior test, then every mutation below must go RED
#   AC-001   prompt version and model name invalidate reuse with the SAME
#            provider identity (alternating endpoint, no fixture swap);
#            mutations drop each term and must go RED
#   AC-002   a changed diff without GITHUB_SHA is reviewed fresh; dropping
#            the diff digest from the key must go RED
#   AC-003   repository identity, binary identity and raw storage on --pr;
#            dropping each must go RED
#   AC-004   a forged cache entry with secret canaries is published on
#            --pr (and printed on --base) redacted
#   MUT-001  publishing a cached finding without redaction must go RED
# Unknown selectors exit 64; infrastructure failures exit 79; behavioral
# failures exit 1. A mutation that does not compile is infrastructure,
# never a silently passing mutation.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-553'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum cmd internal pkg; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done
[[ -f "$repo_root/cmd/aurumcode/aur553_test.go" ]] || infra missing-behavior-test

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a553.XXXXXX")" || infra mktemp
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gotmp"
seed_root() {
  cleanup_root "$run_dir/root"
  mkdir -p "$run_dir/root"
  for source in go.mod go.sum cmd internal pkg; do
    cp -R "$repo_root/$source" "$run_dir/root/$source"
  done
  chmod -R u+w -- "$run_dir/root"
}

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1 -buildvcs=false'
: "${GOCACHE:=$run_dir/cache}"
export GOCACHE GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

run_go_test() {
  local pattern="$1" log="$2"
  set +e
  (cd "$run_dir/root" && go test -buildvcs=false -count=1 -timeout 300s -v ./cmd/aurumcode/ -run "$pattern") >"$log" 2>&1
  local status=$?
  set -e
  return $status
}

# mutate FILE OLD NEW: replaces the one literal occurrence of OLD.
mutate() {
  local target="$run_dir/root/$1"
  [[ -f "$target" ]] || infra "mutation-target-missing:$1"
  grep -Fq -- "$2" "$target" || infra "mutation-anchor-missing:$1"
  MUT_OLD="$2" MUT_NEW="$3" awk '
    { idx = index($0, ENVIRON["MUT_OLD"])
      if (idx > 0) { $0 = substr($0, 1, idx - 1) ENVIRON["MUT_NEW"] substr($0, idx + length(ENVIRON["MUT_OLD"])) }
      print
    }' "$target" > "$target.tmp" && mv "$target.tmp" "$target"
  grep -Fq -- "$3" "$target" || infra "mutation-not-applied:$1"
}

readonly key_values='{inner, in.PolicyDigest, in.PromptVersionDigest, in.BinaryIdentity, in.RepoIdentity, in.DiffDigest, in.ReviewedSHA}'
readonly verdict_go='internal/gate/verdict.go'

# Each mutation: name|test it must turn RED. apply_<name> edits the root.
apply_prompt_version() { mutate "$verdict_go" "$key_values" '{inner, in.PolicyDigest, "", in.BinaryIdentity, in.RepoIdentity, in.DiffDigest, in.ReviewedSHA}'; }
apply_binary_identity() { mutate "$verdict_go" "$key_values" '{inner, in.PolicyDigest, in.PromptVersionDigest, "", in.RepoIdentity, in.DiffDigest, in.ReviewedSHA}'; }
apply_repo_identity() { mutate "$verdict_go" "$key_values" '{inner, in.PolicyDigest, in.PromptVersionDigest, in.BinaryIdentity, "", in.DiffDigest, in.ReviewedSHA}'; }
apply_diff_digest() { mutate "$verdict_go" "$key_values" '{inner, in.PolicyDigest, in.PromptVersionDigest, in.BinaryIdentity, in.RepoIdentity, "", in.ReviewedSHA}'; }
apply_model_name() {
  mutate cmd/aurumcode/review_cache.go 'name := provider.Name()' 'name := "model"'
  mutate cmd/aurumcode/review_cache.go 'if key := resolver.ResolveModel(llm.Options{}); key != ""' 'if key := resolver.ResolveModel(llm.Options{}); key == "\x00"'
}
apply_raw_storage() {
  mutate cmd/aurumcode/review_evidence.go 's.rawIssues = append(append([]types.ReviewIssue(nil), s.result.Issues...), s.securityApart()...)' 's.rawIssues = append(config.ApplyRuleConfig(s.result.Issues, s.cfg), s.securityApart()...)'
}
apply_unredacted_verdict() {
  mutate cmd/aurumcode/review_gate.go 'redactedVerdictReuse{gate.VerdictReuseContributor{Key: in.VerdictKey, Raw: in.RawIssues, PromptDigest: in.PromptDigest}}' 'gate.VerdictReuseContributor{Key: in.VerdictKey, Raw: in.RawIssues, PromptDigest: in.PromptDigest}'
}
apply_unredacted_file_cache() {
  mutate cmd/aurumcode/review_cache.go 'statuses[i].issues = redactCachedIssues(filter, entry.Issues)' 'statuses[i].issues = entry.Issues'
}

declare -A mutation_test=(
  [prompt_version]=TestAUR524AC002PromptVersionChangeInvalidatesReuse
  [model_name]=TestAUR524AC002ModelChangeInvalidatesReuse
  [diff_digest]=TestAUR553AC002DiffChangeWithoutSHAInvalidatesReuse
  [repo_identity]=TestAUR553AC003RepoIdentityChangeInvalidatesReuse
  [binary_identity]=TestAUR553AC003BinaryIdentityChangeInvalidatesReuse
  [raw_storage]=TestAUR553AC003PRStoresRawIssues
  [unredacted_verdict]=TestAUR553AC004PRCachedFindingRedacted
  [unredacted_file_cache]=TestAUR553AC004BaseCachedFileFindingRedacted
)

# check_mutation NAME: applies the mutation to a fresh root and requires
# its test to FAIL behaviorally (the package compiled and the test ran).
check_mutation() {
  local name="$1" test="${mutation_test[$1]}" log="$run_dir/mut-$1.log"
  seed_root
  "apply_$name"
  run_go_test "^${test}\$" "$log" || true
  if grep -Eq '\[build failed\]|\[setup failed\]|undefined:|syntax error|declared and not used' "$log"; then
    cat "$log" >&2
    infra "mutation-did-not-compile:$name"
  fi
  if ! grep -q -- "^--- FAIL: $test " "$log"; then
    cat "$log" >&2
    fail "mutation-survived:$name"
  fi
  printf 'mutation %s -> RED: %s\n' "$name" "$(grep -m1 -E '^\s+aur5[0-9]+_test.go:[0-9]+:' "$log" | sed -e 's/^[[:space:]]*//' | cut -c1-160)"
}

# check_green PATTERN TEST...: the unmutated tree passes every named test.
check_green() {
  local pattern="$1"; shift
  local log="$run_dir/green.log"
  seed_root
  run_go_test "$pattern" "$log" || { cat "$log" >&2; fail go-test-failed; }
  local test
  for test in "$@"; do
    grep -q -- "^--- PASS: $test " "$log" || { cat "$log" >&2; fail "missing-pass:$test"; }
  done
}

ac001_tests=(TestAUR524AC002PromptVersionChangeInvalidatesReuse TestAUR524AC002ModelChangeInvalidatesReuse)
ac002_tests=(TestAUR553AC002DiffChangeWithoutSHAInvalidatesReuse)
ac003_tests=(TestAUR553AC003RepoIdentityChangeInvalidatesReuse TestAUR553AC003BinaryIdentityChangeInvalidatesReuse TestAUR553AC003PRStoresRawIssues)
ac004_tests=(TestAUR553AC004PRCachedFindingRedacted TestAUR553AC004BaseCachedFileFindingRedacted)

join_pattern() { local IFS='|'; printf '^(%s)$' "$*"; }

run_ac() {
  local -n tests="$1"; shift
  check_green "$(join_pattern "${tests[@]}")" "${tests[@]}"
  local m
  for m in "$@"; do check_mutation "$m"; done
}

case "$selector" in
  AC-001) run_ac ac001_tests prompt_version model_name ;;
  AC-002) run_ac ac002_tests diff_digest ;;
  AC-003) run_ac ac003_tests repo_identity binary_identity raw_storage ;;
  AC-004) run_ac ac004_tests ;;
  MUT-001)
    check_mutation unredacted_verdict
    check_mutation unredacted_file_cache
    ;;
  all)
    all_tests=("${ac001_tests[@]}" "${ac002_tests[@]}" "${ac003_tests[@]}" "${ac004_tests[@]}")
    check_green "$(join_pattern "${all_tests[@]}")" "${all_tests[@]}"
    for m in prompt_version model_name diff_digest repo_identity binary_identity raw_storage unredacted_verdict unredacted_file_cache; do
      check_mutation "$m"
    done
    ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
