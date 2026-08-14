#!/usr/bin/env bash
#
# E2E program for card AUR-459, selector E2EAUR459.
#
# WHAT THIS PROVES, AND WHY IT IS NOT THE ACCEPTANCE AGAIN
#
#   tests/acceptance/AUR-459.sh::AC-001 asserts the review OUTPUT: a
#   finding the model reported only under "line_comments" prints on stdout,
#   and one without a rule_id produces the AUR-448 stderr warning instead
#   of silence. This program asserts the CI DECISION that output feeds --
#   the --fail-on gate (AUR-431), end to end through the real binary:
#
#     1. A converted finding at severity error CLOSES the gate: exit 3.
#        Before this card that finding did not exist, so a pipeline
#        reviewing a diff with a planted secret went green.
#     2. A converted finding the rule gate discarded does NOT close it:
#        exit 0, with the discard named on stderr. A gate that failed a
#        build over a finding the user was never shown would be as wrong
#        as one that passed a secret.
#     3. --fail-on low does not close the gate on the converted finding's
#        own text appearing nowhere: exit codes stay in AUR-431's
#        published set (0 or 3), never 1 or 2.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0 = the promised property holds
#   1 = behavioral RED
#   64 = unknown selector
#   79 = inconclusive / infrastructure
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-459'
readonly scenario='E2E'
selector="${1:-E2EAUR459}"
case "$selector" in
  E2EAUR459) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

for input in go.mod go.sum cmd/aurumcode internal/prompt internal/review tests/fixtures/repos/git-demo/repo.git; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e459.XXXXXX")" || infra mktemp
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp"
export TMPDIR="$run_dir"

root="$run_dir/root"
copy() {
  local p
  for p in "$@"; do
    [[ -e "$repo_root/$p" ]] || infra "missing_input:$p"
    mkdir -p "$root/$(dirname "$p")"
    cp -R "$repo_root/$p" "$root/$p"
  done
}
mkdir -p "$root"
copy go.mod go.sum
copy cmd/aurumcode cmd/regenerate-docs
copy internal/analyzer internal/prompt internal/review internal/security internal/llm internal/git internal/pipeline
copy internal/documentation/extractors internal/documentation/incremental internal/documentation/normalizer internal/documentation/site internal/documentation/welcome
copy pkg/types tests/fixtures/repos/git-demo tests/fixtures/review
chmod -R u+w -- "$root"

bin="$run_dir/aurumcode"
if ! ( cd "$root" && ulimit -v 8388608 && GOMEMLIMIT=2GiB go build -o "$bin" ./cmd/aurumcode ) >"$run_dir/build.log" 2>&1; then
  cat "$run_dir/build.log" >&2
  infra build_failed
fi

mkdir -p "$run_dir/fixtures"
cat >"$run_dir/fixtures/grounded.json" <<'EOF'
{"line_comments":[{"path":"config/demo-tokens.txt","line":4,"severity":"error","rule_id":"security/hardcoded-secret","body":"credential-shaped value committed"}]}
EOF
cat >"$run_dir/fixtures/ungrounded.json" <<'EOF'
{"line_comments":[{"path":"config/demo-tokens.txt","line":4,"severity":"error","body":"credential-shaped value committed"}]}
EOF

repo_dir="$root/tests/fixtures/repos/git-demo/repo.git"
run_bin() {
  local fixture="$1"; shift
  set +e
  ( cd "$repo_dir" && env -u AURUM_SECRET_CANARY -u LLM_API_KEY -u LLM_BASE_URL -u AURUMCODE_CACHE_DIR \
    "AURUMCODE_LLM_FIXTURE=$fixture" "$bin" "$@" ) >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  rc=$?
  set -e
}

# 1. The converted finding closes the CI gate.
run_bin "$run_dir/fixtures/grounded.json" review --base HEAD~1 --fail-on high
[[ "$rc" -eq 3 ]] || fail "gate-open-on-converted-finding:exit:$rc"
grep -Fq 'config/demo-tokens.txt:4' "$run_dir/out.stdout" || fail behavior-missing
grep -Fq 'severity error or above' "$run_dir/out.stderr" || fail behavior-missing

# 2. A discarded finding must not close the gate, and must still be named.
run_bin "$run_dir/fixtures/ungrounded.json" review --base HEAD~1 --fail-on high
[[ "$rc" -eq 0 ]] || fail "gate-closed-on-finding-never-shown:exit:$rc"
[[ "$(cat "$run_dir/out.stdout")" == 'No issues found.' ]] || fail behavior-missing
grep -Fq '1 finding(s) discarded: 1 with no rule_id' "$run_dir/out.stderr" || fail behavior-missing

# 3. Determinism, and exit codes stay inside AUR-431's published set.
first_rc=0
run_bin "$run_dir/fixtures/grounded.json" review --base HEAD~1 --fail-on low
first_rc="$rc"
case "$first_rc" in 0|3) ;; *) fail "unpublished-exit:$first_rc" ;; esac
first_out="$(cat "$run_dir/out.stdout")"
run_bin "$run_dir/fixtures/grounded.json" review --base HEAD~1 --fail-on low
[[ "$rc" -eq "$first_rc" ]] || fail non-deterministic
[[ "$first_out" == "$(cat "$run_dir/out.stdout")" ]] || fail non-deterministic

exit 0
