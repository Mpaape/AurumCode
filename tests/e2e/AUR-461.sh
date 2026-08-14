#!/usr/bin/env bash
#
# E2E program for card AUR-461, selector E2EAUR461.
#
# WHAT THIS PROVES, AND WHY IT IS NOT THE ACCEPTANCE AGAIN
#
#   tests/acceptance/AUR-461.sh::AC-001 asserts the PROMPT: the assembled
#   review prompt names every id of the embedded catalog. This program
#   asserts the USER-VISIBLE consequence, through the real binary, of the
#   two cases the 2026-08-14 measurement separated:
#
#     1. A finding citing security/command-injection -- the catalog's own
#        id for the defect the model reported as "shell-injection" -- is
#        printed with its catalog citation and closes the --fail-on gate.
#        This is the finding that was found and lost.
#     2. The invented id security/shell-injection is STILL discarded and
#        still announced on stderr. Handing the model the list must not
#        loosen the AUR-434 gate, and must not silently rewrite a wrong
#        citation into a plausible one.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0 = the promised property holds
#   1 = behavioral RED
#   64 = unknown selector
#   79 = inconclusive / infrastructure. Never valid red evidence.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-461'
readonly scenario='E2E'
selector="${1:-E2EAUR461}"
case "$selector" in
  E2EAUR461) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

for input in go.mod go.sum cmd/aurumcode internal/prompt internal/review \
             internal/prompt/templates/review.md tests/fixtures/repos/git-demo/repo.git; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e461.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"

# REGRAS INEGOCIAVEIS: bounded memory and offline module resolution on
# every go invocation.
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"

bin="$run_dir/aurumcode"
if ! (cd "$repo_root" && ulimit -v 8388608 && GOMEMLIMIT=2GiB go build -o "$bin" ./cmd/aurumcode) >"$run_dir/build.log" 2>&1; then
  cat "$run_dir/build.log" >&2
  infra build_failed
fi

mkdir -p "$run_dir/fixtures"
cat >"$run_dir/fixtures/catalog-id.json" <<'EOF'
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
cat >"$run_dir/fixtures/invented-id.json" <<'EOF'
{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "rule_id": "security/shell-injection",
      "message": "user input reaches a shell",
      "suggestion": "pass an argument vector"
    }
  ],
  "summary": "one finding"
}
EOF

repo_dir="$repo_root/tests/fixtures/repos/git-demo/repo.git"
run_bin() {
  local fixture="$1"; shift
  set +e
  (cd "$repo_dir" && env -u AURUM_SECRET_CANARY -u LLM_API_KEY -u LLM_BASE_URL -u AURUMCODE_CACHE_DIR \
    "AURUMCODE_LLM_FIXTURE=$fixture" "$bin" "$@") >"$run_dir/out" 2>"$run_dir/err"
  rc=$?
  set -e
}

# --- 1. The id the catalog defines reaches the user and closes the gate. ---
run_bin "$run_dir/fixtures/catalog-id.json" review --base HEAD~1 --fail-on error
[[ "$rc" -eq 3 ]] || { cat "$run_dir/out" "$run_dir/err" >&2; fail "catalog-id-gate-exit:$rc"; }
grep -Fq 'user input reaches a shell' "$run_dir/out" || fail catalog-id-not-shown
grep -Fq 'security/command-injection' "$run_dir/out" || fail catalog-id-citation-missing
grep -Fq 'discarded' "$run_dir/err" && fail catalog-id-discarded

# --- 2. The invented id is still discarded, still announced, gate open. ---
run_bin "$run_dir/fixtures/invented-id.json" review --base HEAD~1 --fail-on error
[[ "$rc" -eq 0 ]] || { cat "$run_dir/out" "$run_dir/err" >&2; fail "invented-id-gate-exit:$rc"; }
grep -Fq 'unknown rule_id' "$run_dir/err" || fail invented-id-not-announced
grep -Fq 'security/shell-injection' "$run_dir/err" || fail invented-id-not-named
# Never rewritten by similarity into the catalog's neighbouring id.
grep -Fq 'security/command-injection' "$run_dir/out" && fail invented-id-silently-remapped
grep -Fq 'user input reaches a shell' "$run_dir/out" && fail invented-id-shown-as-true

exit 0
