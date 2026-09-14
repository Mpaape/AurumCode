#!/usr/bin/env bash
# AUR-496 acceptance: the deterministic scanner only flags additions, file
# permission findings require an actual write-for-others mode on the call's
# real last argument (not any nearby digit), comments and documentation
# strings are never mistaken for credential assignments, and the reusable
# review workflow checks out the reviewed PR into its own read-only mount
# instead of reusing AurumCode's own checkout.
#
# Exit: 0 green; 1 behavioural failure; 64 unknown selector; 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C
umask 077
readonly card='AUR-496'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac
if [[ "$selector" == all ]]; then
  for scenario in AC-001 AC-002 AC-003 AC-004 MUT-001 MUT-002; do
    bash "${BASH_SOURCE[0]}" "$scenario"
  done
  exit 0
fi
scenario="$selector"
fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum internal/analysis pkg/types .github/workflows/review.yml docs/review-quality.md; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a496.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="${GOCACHE:-$run_dir/gocache}" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir" GOMAXPROCS=1
export XDG_CACHE_HOME="$run_dir/xdg" HOME="$run_dir/home"

# Whole top-level trees, never a per-package list: in a sealed run only the
# declared inputs exist, so this copies exactly what the card materializes.
root="$run_dir/root"; mkdir -p "$root"
for top in go.mod go.sum internal pkg; do
  [[ -e "$repo_root/$top" ]] || continue
  cp -R "$repo_root/$top" "$root/$top"
done
chmod -R u+w -- "$root"

# gotest PKG SELECTOR -> 0 when the named test PASSES, 1 when it FAILS,
# 79 when the package does not build.
gotest() {
  local pkg="$1" sel="$2"
  local log="$run_dir/$sel.log" rc=0
  (cd "$root" && go test "./$pkg/" -run "^${sel}\$" -count=1 -timeout=60s -v) >"$log" 2>&1 || rc=$?
  grep -q 'build failed\|cannot find package\|setup failed' "$log" && { cat "$log" >&2; return 79; }
  if ((rc == 0)) && grep -q -- "--- PASS: $sel" "$log"; then return 0; fi
  cat "$log" >&2
  return 1
}
expect_pass() { local rc=0; gotest "$1" "$2" || rc=$?; ((rc == 79)) && infra "build_failed:$2"; ((rc == 0)) || fail "selector-failed:$2"; }
expect_fail() { local rc=0; gotest "$1" "$2" || rc=$?; ((rc == 79)) && infra "build_failed:$2"; ((rc == 1)) || fail "mutation-not-detected:$2"; }

# workflow_step_block WORKFLOW STEP_NAME -> prints the lines of that named
# step ("- name: STEP_NAME" up to, but excluding, the next top-level "- name:"
# line or EOF), so a check binds to the exact step it means instead of
# matching a substring anywhere else in the file (e.g. another step that
# also happens to set "persist-credentials").
workflow_step_block() {
  local workflow="$1" step="$2"
  awk -v step="      - name: $step" '
    $0 == step { found=1; print; next }
    found && /^      - name:/ { exit }
    found { print }
  ' "$workflow"
}

# check_workflow_contract WORKFLOW -> 0 if every AC-003 property holds on
# that file, 1 (with reasons on stdout) otherwise. This is the single
# reusable check: the nominal case below calls it on the real file, and
# MUT-002 calls the SAME function on a staged, mutated copy and requires it
# to fail. It never calls exit itself, so both callers can capture its
# return code.
check_workflow_contract() {
  local workflow="$1" bad=0
  [[ -f "$workflow" ]] || { echo "workflow-missing"; return 1; }

  local checkout_block
  checkout_block="$(workflow_step_block "$workflow" 'Checkout pull request')"
  if [[ -z "$checkout_block" ]]; then echo "missing-checkout-step"; bad=1; fi
  grep -Fq 'repository: ${{ github.repository }}' <<<"$checkout_block" || { echo "wrong-target-repository"; bad=1; }
  grep -Fq 'ref: ${{ github.event.pull_request.head.sha }}' <<<"$checkout_block" || { echo "wrong-target-ref"; bad=1; }
  grep -Fq 'path: .aurumcode-target' <<<"$checkout_block" || { echo "missing-separate-checkout-path"; bad=1; }
  grep -Fq 'persist-credentials: false' <<<"$checkout_block" || { echo "credentials-persisted"; bad=1; }

  local run_block
  run_block="$(workflow_step_block "$workflow" 'Run and publish code review')"
  if [[ -z "$run_block" ]]; then echo "missing-run-step"; bad=1; fi
  grep -Fq -- '-v "${GITHUB_WORKSPACE}/.aurumcode-target:/github/workspace:ro"' <<<"$run_block" || { echo "workspace-not-readonly"; bad=1; }
  grep -Fq -- '-w /github/workspace' <<<"$run_block" || { echo "workdir-not-set"; bad=1; }

  # AC-003: the reviewed PR tree is mounted read-only and its code is never
  # executed. The workflow legitimately names the tree on exactly two lines:
  # the read-only mount source and the checkout's `path: .aurumcode-target`.
  # Drop those two lines, then reject ANY remaining mention of the tree, so a
  # step that cd's into it or runs it with `sh -c` / `bash "$PWD/..."` is
  # caught, not just `./.aurumcode-target/...` and `bash .aurumcode-target/...`.
  # The package-manager and build command patterns stay as a backstop.
  local without_mount
  without_mount="$(grep -Fv -- '-v "${GITHUB_WORKSPACE}/.aurumcode-target:/github/workspace:ro"' "$workflow" \
    | grep -Fv -- 'path: .aurumcode-target' || true)"
  if grep -Fq -- '.aurumcode-target' <<<"$without_mount"; then echo "runs-pr-code"; bad=1; fi
  if grep -Eq 'npm (ci|install)|go (build|test)|make ' <<<"$without_mount"; then echo "runs-pr-code"; bad=1; fi

  return "$bad"
}

workflow="$repo_root/.github/workflows/review.yml"

case "$selector" in
  AC-001)
    expect_pass internal/analysis TestAnalyzeTable
    ;;
  AC-002)
    expect_pass internal/analysis TestAUR496FilePermissions
    expect_pass internal/analysis TestAUR496CommentsNotCredentials
    expect_pass internal/analysis TestAUR496DocumentationStringsNotCredentials
    expect_pass internal/analysis TestAUR489SecretNaming
    ;;
  AC-003)
    reasons="$(check_workflow_contract "$workflow")" && rc=0 || rc=$?
    ((rc == 0)) || fail "${reasons//$'\n'/;}"
    ;;
  AC-004)
    doc="$repo_root/docs/review-quality.md"
    grep -qi 'heur' "$doc" || fail 'docs-missing:heuristic-language'
    grep -Fq 'https://pkg.go.dev/cmd/vet' "$doc" || fail 'docs-missing:govet-primary-source'
    grep -Fq 'https://docs.coderabbit.ai/reference/review-commands' "$doc" || fail 'docs-missing:coderabbit-primary-source'
    grep -Fq 'https://docs.github.com/en/copilot/concepts/agents/code-review' "$doc" || fail 'docs-missing:copilot-primary-source'
    grep -qi 'nao afirma superioridade\|não afirma superioridade' "$doc" || fail 'docs-missing:no-superiority-claim'
    ;;
  MUT-001)
    # Restoring the LEFT match must make the removed-secret AC fail again.
    sed -i 's|case "-":|case "-":\n\t\t\t\t\tfindings = append(findings, r.match(file.Path, oldLine, SideLeft, body)...)|' "$root/internal/analysis/analysis.go"
    grep -q 'SideLeft, body' "$root/internal/analysis/analysis.go" || infra 'mutation-anchor-missing:MUT-001'
    expect_fail internal/analysis TestAnalyzeTable
    ;;
  MUT-002)
    staged_dir="$run_dir/workflow-mutations"; mkdir -p "$staged_dir"

    # Baseline: the SAME function must accept the real, unmutated file.
    reasons="$(check_workflow_contract "$workflow")" && rc=0 || rc=$?
    ((rc == 0)) || infra "baseline-contract-failed:${reasons//$'\n'/;}"

    # Mutation A: drop the read-only mount flag.
    mut_a="$staged_dir/readwrite.yml"
    sed 's#-v "\${GITHUB_WORKSPACE}/.aurumcode-target:/github/workspace:ro"#-v "${GITHUB_WORKSPACE}/.aurumcode-target:/github/workspace"#' "$workflow" > "$mut_a"
    grep -Fq -- '-v "${GITHUB_WORKSPACE}/.aurumcode-target:/github/workspace"' "$mut_a" || infra 'mutation-anchor-missing:MUT-002a'
    if check_workflow_contract "$mut_a" >/dev/null; then fail 'readwrite-mount-not-detected'; fi

    # Mutation B: point the target checkout at a different repository/SHA.
    mut_b="$staged_dir/wrong-target.yml"
    sed "s#repository: \${{ github.repository }}#repository: some-other/repo#; s#ref: \${{ github.event.pull_request.head.sha }}#ref: main#" "$workflow" > "$mut_b"
    grep -Fq 'repository: some-other/repo' "$mut_b" || infra 'mutation-anchor-missing:MUT-002b'
    if check_workflow_contract "$mut_b" >/dev/null; then fail 'wrong-checkout-target-not-detected'; fi

    # Mutation C: persist credentials on the target checkout.
    mut_c="$staged_dir/persist-creds.yml"
    sed 's#persist-credentials: false#persist-credentials: true#' "$workflow" > "$mut_c"
    grep -Fq 'persist-credentials: true' "$mut_c" || infra 'mutation-anchor-missing:MUT-002c'
    if check_workflow_contract "$mut_c" >/dev/null; then fail 'persisted-credentials-not-detected'; fi

    # Mutation D: execute a script from the checked-out PR tree.
    mut_d="$staged_dir/runs-pr-code.yml"
    { cat "$workflow"; printf '      - name: Leak PR tree\n        run: bash .aurumcode-target/build.sh\n'; } > "$mut_d"
    grep -Fq 'run: bash .aurumcode-target/build.sh' "$mut_d" || infra 'mutation-anchor-missing:MUT-002d'
    if check_workflow_contract "$mut_d" >/dev/null; then fail 'pr-code-execution-not-detected'; fi

    # Mutation E: reach the PR tree by cd'ing into it first. This form has no
    # ".aurumcode-target/" slash token, so the earlier path pattern alone
    # missed it; the any-remaining-mention check must catch it.
    mut_e="$staged_dir/cd-pr-tree.yml"
    { cat "$workflow"; printf '      - name: Leak\n        run: cd .aurumcode-target && bash build.sh\n'; } > "$mut_e"
    grep -Fq 'run: cd .aurumcode-target && bash build.sh' "$mut_e" || infra 'mutation-anchor-missing:MUT-002e'
    if check_workflow_contract "$mut_e" >/dev/null; then fail 'cd-pr-code-execution-not-detected'; fi

    # Restore: the unmutated file must pass again (green after red).
    reasons="$(check_workflow_contract "$workflow")" && rc=0 || rc=$?
    ((rc == 0)) || fail "not-restored-green:${reasons//$'\n'/;}"
    ;;
esac
printf '%s/%s/pass\n' "$card" "$scenario"
