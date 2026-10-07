#!/usr/bin/env bash
#
# Acceptance program for card AUR-545: a model finding outside the changed
# lines that carries full proof becomes a general comment (never inline,
# never counted by the policy gate); one without proof stays discarded.
# See docs/specs/AUR-545.md.
#
# SELECTORS
#   all      every selector below
#   AC-001   the spec records the decision and its reason; scope.go,
#            scope_test.go and tests/e2e/AUR-438.sh agree with it
#   AC-002   an outside finding without evidence is still discarded,
#            counted, and never published
#   AC-003   no outside finding counts for the gate: the real binary,
#            under --fail-on error, approves and exits 0 with an
#            error-severity proved finding outside the diff, which it
#            prints as a general comment, and SARIF stays empty
#   MUT-001  letting the outside finding into result.Issues (so it counts
#            for the gate) turns AC-003 red, in the Go test and in the
#            real binary
#
# EXIT CODES: 0 pass, 1 behavioral RED, 64 unknown selector, 79 infrastructure.
set -euo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-545'
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
command -v git >/dev/null 2>&1 || infra missing_git

for input in go.mod go.sum cmd internal pkg docs/specs/AUR-545.md tests/e2e/AUR-438.sh \
  tests/fixtures/repos/git-demo/repo.git tests/fixtures/scm/github; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a545.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
: "${GOCACHE:=$run_dir/gocache}"
: "${GOTMPDIR:=$run_dir/gotmp}"
export GOCACHE GOTMPDIR GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS=-mod=mod

# stage copies the module into a fresh root so a mutation never touches
# the checkout.
stage() {
  local root="$1" top
  mkdir -p "$root"
  for top in cmd internal pkg go.mod go.sum; do
    cp -R "$repo_root/$top" "$root/$top"
  done
  chmod -R u+w -- "$root"
}

# go_test runs the named tests of internal/review in root; rc and output
# land in gt_rc and $run_dir/gotest.out.
go_test() {
  local root="$1" pattern="$2"
  set +e
  (cd "$root" && go test -buildvcs=false -count=1 -p 1 -run "$pattern" ./internal/review) >"$run_dir/gotest.out" 2>&1
  gt_rc=$?
  set -e
}

# The proved, error-severity finding on src/greeter.py:5, a line the
# HEAD~1..HEAD diff of the git-demo fixture does not touch.
fixture="$run_dir/outside.json"
cat >"$fixture" <<'EOF'
{"issues":[{"file":"src/greeter.py","line":5,"severity":"error","rule_id":"quality/dead-code",
"message":"Achado sintetico fora do diff.",
"evidence":"A linha 5 de src/greeter.py nao foi alterada e e citada como prova.",
"impact":"Impacto sintetico para a prova de aceite.",
"verification":"Rodar o review e conferir que o achado vira comentario geral."}],
"summary":"Resposta sintetica do aceite AUR-545."}
EOF

# run_base builds root's binary (named by $2, inside run_dir, never in
# the checkout) and reviews the git-demo fixture with the
# outside-diff response under --fail-on error. Sets base_rc, base_out,
# base_sarif.
run_base() {
  local root="$1" bin="$run_dir/aurumcode-$2"
  (cd "$root" && go build -buildvcs=false -o "$bin" ./cmd/aurumcode) >"$run_dir/build.log" 2>&1 \
    || { cat "$run_dir/build.log" >&2; infra build_failed; }
  base_out="$run_dir/base.out"
  base_sarif="$run_dir/base.sarif"
  rm -f -- "$base_sarif"
  set +e
  (cd "$repo_root/tests/fixtures/repos/git-demo/repo.git" && env -u AURUMCODE_CACHE_DIR -u LLM_API_KEY -u LLM_BASE_URL \
    AURUMCODE_LLM_FIXTURE="$fixture" "$bin" review --base HEAD~1 --fail-on error --sarif "$base_sarif") >"$base_out" 2>&1
  base_rc=$?
  set -e
}

# base_gate_ignores_outside: 0 when the binary's run kept the finding out
# of every gate input and printed it as a general comment.
base_gate_ignores_outside() {
  [[ "$base_rc" -eq 0 ]] || { printf 'exit=%s\n' "$base_rc" >&2; return 1; }
  grep -Fq '**Verdict:** Approve' "$base_out" || { echo 'verdict-not-approve' >&2; return 1; }
  grep -Fq 'src/greeter.py:5: [error] Achado sintetico fora do diff.' "$base_out" || { echo 'general-line-missing' >&2; return 1; }
  grep -Fq 'fora das linhas alteradas (comentario geral; nao conta para o gate)' "$base_out" || { echo 'general-marker-missing' >&2; return 1; }
  [[ -s "$base_sarif" ]] || { echo 'sarif-missing' >&2; return 1; }
  if grep -Fq 'greeter' "$base_sarif"; then echo 'outside-finding-in-sarif' >&2; return 1; fi
  return 0
}

run_ac001() {
  local spec="$repo_root/docs/specs/AUR-545.md" part
  for part in '## Decisao' '## Motivo' 'comentario geral' 'nunca conta para o gate'; do
    grep -Fq "$part" "$spec" || fail "spec-missing:$part"
  done
  go_test "$repo_root" 'TestFilterModelIssuesRoutesProvedOutsideFindingsAndRejectsUnproved|TestDeletionRegressionUsesOldSideWithoutAdmittingUntouchedCode'
  [[ "$gt_rc" -eq 0 ]] || { cat "$run_dir/gotest.out" >&2; fail scope-tests; }
  set +e
  (cd "$repo_root" && bash tests/e2e/AUR-438.sh) >"$run_dir/e2e438.out" 2>&1
  local rc=$?
  set -e
  [[ "$rc" -eq 0 ]] || { cat "$run_dir/e2e438.out" >&2; fail "e2e-438:$rc"; }
  grep -Fq 'AUR-438/AC-001/E2EAUR438/ok' "$run_dir/e2e438.out" || fail e2e-438-no-ok
}

run_ac002() {
  go_test "$repo_root" 'TestOutsideDiffAC002UnprovedFindingStillDiscarded|TestFilterModelIssuesNeverRoutesOutsideFindingWithoutFullProof|TestOutsideDiffForgedMetadataRemoved'
  [[ "$gt_rc" -eq 0 ]] || { cat "$run_dir/gotest.out" >&2; fail unproved-tests; }
  grep -Fq 'unproved_outside_finding_published' "$repo_root/tests/e2e/AUR-438.sh" || fail e2e-438-lacks-unproved-check
}

run_ac003() {
  go_test "$repo_root" 'TestOutsideDiffAC003ProvedFindingNeverCountsForTheGate'
  [[ "$gt_rc" -eq 0 ]] || { cat "$run_dir/gotest.out" >&2; fail gate-test; }
  run_base "$repo_root" candidate
  base_gate_ignores_outside || { cat "$base_out" >&2; fail binary-gate; }
}

run_mut001() {
  local root="$run_dir/mut001" target
  stage "$root"
  target="$root/internal/review/reviewer.go"
  grep -Fq 'outcome.outsideDiff = r.citedOutsideDiff(rules, outside, &outcome)' "$target" || infra mut001-anchor
  awk '{ print } index($0, "outcome.outsideDiff = r.citedOutsideDiff(rules, outside, &outcome)") > 0 { print "\tresult.Issues = append(result.Issues, outcome.outsideDiff...)" }' \
    "$target" >"$target.new" && mv -- "$target.new" "$target"
  grep -Fq 'result.Issues = append(result.Issues, outcome.outsideDiff...)' "$target" || infra mut001-not-applied
  go_test "$root" 'TestOutsideDiffAC003ProvedFindingNeverCountsForTheGate'
  grep -Eq '^(FAIL|--- FAIL)' "$run_dir/gotest.out" || { cat "$run_dir/gotest.out" >&2; fail mutant-test-survived; }
  grep -Fq 'a finding outside the diff reached result.Issues' "$run_dir/gotest.out" || { cat "$run_dir/gotest.out" >&2; fail mutant-test-other-cause; }
  run_base "$root" mutant
  if base_gate_ignores_outside 2>/dev/null; then cat "$base_out" >&2; fail mutant-binary-survived; fi
  [[ "$base_rc" -ne 0 ]] || { cat "$base_out" >&2; fail mutant-binary-exit-zero; }
  printf 'MUT-001 red: go test fails and --fail-on error exits %s\n' "$base_rc"
}

case "$selector" in
  AC-001) run_ac001 ;;
  AC-002) run_ac002 ;;
  AC-003) run_ac003 ;;
  MUT-001) run_mut001 ;;
  all) run_ac001; run_ac002; run_ac003; run_mut001 ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
