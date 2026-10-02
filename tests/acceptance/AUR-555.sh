#!/usr/bin/env bash
# AUR-555 acceptance: the reusable workflow generates the SBOM (AUR-549)
# BEFORE the review step so the Dependency-Track gate (AUR-550) has a file
# to upload, declares the Dependency-Track secrets as optional, exposes
# them only to the review step, and signs (AUR-551) only after a
# successful review. See docs/specs/AUR-555.md.
#
# Selectors:
#   all             every test below plus the mutation
#   AC-001          SBOM step precedes the review step and both name the
#                   same checkout/path
#   AC-002          secrets are required:false and only the review step
#                   receives them
#   AC-003          ssor_dtrack on + secret absent: inconclusive per policy
#                   mode (block fails, warn never approves), no upload
#   AC-004          workflow valid without the new secrets; signing runs
#                   only after a successful review
#   AC-001-MUT-001  copy of the workflow with the SBOM step moved after
#                   the review step; AC-001's test must go RED
# Unknown selectors exit 64; infrastructure failures 79; behavior 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-555'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

for input in go.mod go.sum cmd internal pkg .github/workflows/review.yml; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done
[[ -f "$repo_root/cmd/aurumcode/aur555_test.go" ]] || infra missing-behavior-test

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a555.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP
mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$run_dir/root/.github"
for source in go.mod go.sum cmd internal pkg; do
  cp -R "$repo_root/$source" "$run_dir/root/$source"
done
cp -R "$repo_root/.github/workflows" "$run_dir/root/.github/workflows"
chmod -R u+w -- "$run_dir/root"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

run_go_test() {
  local pattern="$1" log="$2" status
  set +e
  (cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 300s -v ./cmd/aurumcode/... -run "$pattern") >"$log" 2>&1
  status=$?
  set -e
  cat "$log" >&2
  return $status
}

expect_pass() {
  local pattern="$1"; shift
  local log="$run_dir/test.log" status=0 name
  run_go_test "$pattern" "$log" || status=$?
  (( status == 0 )) || fail "go-test-exit:$status"
  for name in "$@"; do
    grep -q "^--- PASS: $name " "$log" || fail "missing-pass:$name"
  done
}

# MUT-001: move the SBOM step block (from its AUR-555 comment up to the
# review step) to just after the review step, in a COPY of the workflow.
mutate_move_sbom_after_review() {
  local wf="$run_dir/mutated-review.yml"
  awk '
    { lines[NR] = $0 }
    END {
      for (i = 1; i <= NR; i++) {
        if (!s && lines[i] ~ /^      # AUR-555: this step now runs BEFORE/) s = i
        if (!r && lines[i] ~ /^      - name: Run and publish code review$/) r = i
        if (!g && lines[i] ~ /^      # AUR-555: signing runs only after/) g = i
      }
      if (!s || !r || !g || !(s < r && r < g)) exit 3
      for (i = 1; i < s; i++) print lines[i]
      for (i = r; i < g; i++) print lines[i]
      for (i = s; i < r; i++) print lines[i]
      for (i = g; i <= NR; i++) print lines[i]
    }' "$run_dir/root/.github/workflows/review.yml" >"$wf" || infra mutation-anchor-missing
  cmp -s "$wf" "$run_dir/root/.github/workflows/review.yml" && infra mutation-not-applied
  printf '%s\n' "$wf"
}

p1='^TestAUR555SBOMStepPrecedesReview$'
p2='^TestAUR555SecretsOptionalAndScopedToReview$'
p3='^TestAUR555MissingSecretIsInconclusiveNeverApproved$'
p4='^TestAUR555WorkflowValidWithoutDTrackAndSignsOnlyAfterReview$'

run_mutation() {
  local wf log="$run_dir/mutation.log"
  wf="$(mutate_move_sbom_after_review)"
  AUR555_WORKFLOW="$wf" run_go_test "$p1" "$log" || true
  grep -Eq -- '^--- FAIL: TestAUR555SBOMStepPrecedesReview' "$log" || fail 'mutation-survived'
  if grep -Eq 'build failed|cannot use|undefined:|syntax error' "$log"; then
    fail 'mutation-build-failure-not-behavioral'
  fi
}

case "$selector" in
  AC-001) expect_pass "$p1" TestAUR555SBOMStepPrecedesReview ;;
  AC-002) expect_pass "$p2" TestAUR555SecretsOptionalAndScopedToReview ;;
  AC-003) expect_pass "$p3" TestAUR555MissingSecretIsInconclusiveNeverApproved ;;
  AC-004) expect_pass "$p4" TestAUR555WorkflowValidWithoutDTrackAndSignsOnlyAfterReview ;;
  AC-001-MUT-001) run_mutation; printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"; exit 0 ;;
  all)
    expect_pass '^TestAUR555' TestAUR555SBOMStepPrecedesReview TestAUR555SecretsOptionalAndScopedToReview \
      TestAUR555MissingSecretIsInconclusiveNeverApproved TestAUR555WorkflowValidWithoutDTrackAndSignsOnlyAfterReview
    run_mutation
    ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
