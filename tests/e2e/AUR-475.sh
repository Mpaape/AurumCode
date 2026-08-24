#!/usr/bin/env bash
#
# E2E program for card AUR-475, selector E2EAUR475.
#
# WHAT THIS PROVES, AND WHY IT IS NOT THE ACCEPTANCE AGAIN
#
#   AC-001/002/003 live entirely at PROMPT ASSEMBLY, inside internal/prompt
#   -- the package this card owns. tests/acceptance/AUR-475.sh::AC-001 and
#   tests/unit/AUR-475.go already prove them directly against
#   PromptBuilder.BuildPrompt. A canned AURUMCODE_LLM_FIXTURE response (the
#   only LLM double this repo has for the real binary, see
#   tests/e2e/AUR-461.sh) returns the SAME JSON regardless of what the
#   prompt contains, so driving the real `aurumcode review` binary cannot
#   demonstrate the exclusion or the coverage declaration either way --
#   only that the binary still runs.
#
#   This program is therefore a NO-REGRESSION control, not an AC proof: it
#   builds the real binary, runs `review` against the existing
#   tests/fixtures/repos/git-demo/repo.git fixture (whose HEAD~1 diff
#   touches NOTES.txt and config/demo-tokens.txt -- both classified as code
#   by this card's fail-closed prose detector, since neither has a
#   recognized documentation extension), and requires the finding pipeline
#   through cmd/aurumcode -> internal/review -> internal/prompt to still
#   report the fixture's finding and still close --fail-on. If this card's
#   change to internal/prompt broke prompt assembly for an ordinary
#   all-code diff, this goes RED.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0 = the promised property holds
#   1 = behavioral RED
#   64 = unknown selector
#   79 = inconclusive / infrastructure. Never valid red evidence.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-475'
readonly scenario='E2E'
selector="${1:-E2EAUR475}"
case "$selector" in
  E2EAUR475) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

for input in go.mod go.sum cmd/aurumcode internal/prompt internal/review \
             tests/fixtures/repos/git-demo/repo.git; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e475.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"

bin="$run_dir/aurumcode"
if ! (cd "$repo_root" && ulimit -v 8388608 && GOMEMLIMIT=2GiB go build -o "$bin" ./cmd/aurumcode) >"$run_dir/build.log" 2>&1; then
  cat "$run_dir/build.log" >&2
  infra build_failed
fi

mkdir -p "$run_dir/fixtures"
cat >"$run_dir/fixtures/finding.json" <<'EOF'
{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "rule_id": "security/command-injection",
      "message": "user input reaches a shell",
      "suggestion": "pass an argument vector"
    }
  ],
  "summary": "one finding"
}
EOF

repo_dir="$repo_root/tests/fixtures/repos/git-demo/repo.git"
set +e
(cd "$repo_dir" && env -u AURUM_SECRET_CANARY -u LLM_API_KEY -u LLM_BASE_URL -u AURUMCODE_CACHE_DIR \
  "AURUMCODE_LLM_FIXTURE=$run_dir/fixtures/finding.json" "$bin" review --base HEAD~1 --fail-on error) \
  >"$run_dir/out" 2>"$run_dir/err"
rc=$?
set -e

[[ "$rc" -eq 3 ]] || { cat "$run_dir/out" "$run_dir/err" >&2; fail "no-regression-gate-exit:$rc"; }
grep -Fq 'user input reaches a shell' "$run_dir/out" || fail finding-not-shown
grep -Fq 'security/command-injection' "$run_dir/out" || fail citation-missing

exit 0
