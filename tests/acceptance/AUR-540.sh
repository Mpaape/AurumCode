#!/usr/bin/env bash
#
# Acceptance program for card AUR-540.
#
# WHAT THIS PROVES
#
#   tests/e2e/AUR-467.sh::E2EAUR467, a selector belonging to a card already
#   in done, was exiting 1 on an unmodified main
#   (AUR-467/E2E/no-regression-gate-exit:0) for a reason unrelated to
#   AUR-467's own promise: internal/review/scope.go's scope-and-evidence
#   gate (filterModelIssues) now discards any model finding missing
#   evidence/impact/verification, and the e2e fixture predated that
#   contract, so its lone finding was silently dropped and `review`
#   exited 0 (clean) instead of 3 (exitFindings). The fix adds the three
#   fields to the fixture in tests/e2e/AUR-467.sh; it touches no product
#   path this card owns and does not change what AUR-467 promised
#   (prose-vs-code classification and declared coverage are untouched).
#   See docs/specs/AUR-540.md for the full measured cause.
#
# SELECTORS
#   all             run every scenario below
#   AC-001          tests/e2e/AUR-467.sh E2EAUR467 exits 0 on the worktree
#                   as committed
#   AC-001-MUT-001  staged copy with the evidence/impact/verification
#                   fields stripped back out of the fixture (AUR-467's
#                   pre-fix shape) -- E2EAUR467 must go behaviorally RED
#                   on that staged copy
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving mutation)
#   64 = unknown selector
#   79 = inconclusive / infrastructure. Never valid red evidence.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-540'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

required_inputs=(
  go.mod go.sum cmd/aurumcode internal/prompt internal/review
  tests/e2e/AUR-467.sh tests/fixtures/repos/git-demo/repo.git
)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a540.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP

# AC-001: the real selector, run as-is from the worktree.
run_ac001() {
  local out="$run_dir/ac001.out"
  set +e
  ( cd "$repo_root" && bash tests/e2e/AUR-467.sh E2EAUR467 ) >"$out" 2>&1
  local rc=$?
  set -e
  if (( rc == 79 )); then cat "$out" >&2; infra 'AC-001/e2e-infra'; fi
  if (( rc != 0 )); then cat "$out" >&2; return 1; fi
  return 0
}

# AC-001-MUT-001: strip the evidence/impact/verification fields this card
# added to the fixture back out, in a staged copy, reproducing AUR-467's
# pre-fix fixture exactly. The finding then has no evidence again, the
# scope-and-evidence gate discards it again, and `review` must exit 0
# (clean) instead of 3 -- so the staged E2EAUR467 must go RED.
run_mut001() {
  local root="$run_dir/root-mut001"
  mkdir -p "$root"
  local top
  for top in go.mod go.sum cmd internal pkg tests; do
    [[ -e "$repo_root/$top" ]] || continue
    cp -R "$repo_root/$top" "$root/$top"
  done
  chmod -R u+w -- "$root"

  local target="$root/tests/e2e/AUR-467.sh"
  [[ -f "$target" ]] || infra 'MUT-001/stage-missing'
  local before after
  before="$(sha256sum "$target" | awk '{print $1}')"

  local tmp="$root/AUR-467.mutated.sh"
  sed \
    -e '/"evidence":[[:space:]]*"/d' \
    -e '/"impact":[[:space:]]*"/d' \
    -e '/"verification":[[:space:]]*"/d' \
    -e 's/"suggestion": "pass an argument vector",/"suggestion": "pass an argument vector"/' \
    "$target" >"$tmp" || infra 'MUT-001/rewrite'
  mv "$tmp" "$target"

  after="$(sha256sum "$target" | awk '{print $1}')"
  [[ "$before" != "$after" ]] || infra 'MUT-001/no-change'
  grep -Fq '"evidence"' "$target" && infra 'MUT-001/evidence-still-present'

  local out="$root/mut001.out"
  set +e
  ( cd "$root" && bash tests/e2e/AUR-467.sh E2EAUR467 ) >"$out" 2>&1
  local rc=$?
  set -e

  if (( rc == 79 )); then cat "$out" >&2; infra 'MUT-001/e2e-infra'; fi
  if (( rc == 0 )); then
    cat "$out" >&2
    return 1 # mutation survived: pre-fix fixture should have gone RED again
  fi
  (( rc == 1 )) || { cat "$out" >&2; infra 'MUT-001/unexpected-exit'; }
  grep -Fq 'AUR-467/E2E/no-regression-gate-exit:0' "$out" || {
    cat "$out" >&2; infra 'MUT-001/wrong-failure-shape'
  }
  return 0
}

case "$selector" in
  AC-001)
    run_ac001 || fail AC-001
    ;;
  AC-001-MUT-001)
    run_mut001 || fail AC-001-MUT-001
    ;;
  all)
    run_ac001 || fail AC-001
    run_mut001 || fail AC-001-MUT-001
    ;;
esac

printf '%s/%s/pass\n' "$card" "$selector"
exit 0
