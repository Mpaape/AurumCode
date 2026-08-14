#!/usr/bin/env bash
# E2E check for AUR-458. DISTINCT ASSERTION: the CI-SHAPED invocation --
# the exact command a pipeline runs, `review --base X --seguranca
# --fail-on high`, where the two exit codes 1 ("did not review") and 3
# ("reviewed, findings at threshold") compete and the taxonomy's precedence
# decides. tests/unit/AUR-458.go asserts exit codes per situation and
# tests/integration/AUR-458.go asserts stdout composition; neither
# exercises the gate's precedence or the published demo path, which is what
# this program exists for.
set -euo pipefail
export LC_ALL=C
ulimit -v 8388608 2>/dev/null || true
export GOMEMLIMIT=2GiB

readonly card='AUR-458'
selector="${1:-E2EAUR458}"
case "$selector" in
  E2EAUR458) ;;
  *) printf '%s/E2E/unknown-selector\n' "$card" >&2; exit 64 ;;
esac

fail() { printf '%s/E2E/%s\n' "$card" "$1" >&2; exit 1; }
infra() { printf '%s/E2E/infrastructure/%s\n' "$card" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
root="${AURUMCODE_ROOT:-$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)}" || infra root
demo="$root/tests/fixtures/repos/git-demo/repo.git"
[[ -d "$demo" ]] || infra missing-demo-fixture

work="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e2e458.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$work" >/dev/null 2>&1 || true' EXIT INT TERM HUP

bin="${AURUMCODE_BIN:-}"
if [[ -z "$bin" ]]; then
  command -v go >/dev/null 2>&1 || infra missing-go
  bin="$work/aurumcode"
  (cd "$root" && go build -mod=mod -o "$bin" ./cmd/aurumcode) >"$work/build.log" 2>&1 || { cat "$work/build.log" >&2; infra build-failed; }
fi

# noprov strips every provider-selecting variable so "nothing configured"
# is never an accident of the harness's own environment.
noprov() { env -u AURUMCODE_LLM_FIXTURE -u LLM_API_KEY -u LLM_BASE_URL -u LLM_MODEL "$@"; }

run() {  # run <label> <env-prefix...> -- <args...>; sets rc/out/err
  local label="$1"; shift
  set +e
  ( cd "$demo" && "$@" ) >"$work/out.txt" 2>"$work/err.txt"
  rc=$?
  set -e
  out="$(cat "$work/out.txt")"
  err="$(cat "$work/err.txt")"
}

readonly sec_header='Security findings (standards/security-review):'

# --- 1. THE GATE'S PRECEDENCE ------------------------------------------
# A pipeline asks for both halves and a gate on top. The model half fails;
# the security half finds three error-severity secrets, which on their own
# would close the --fail-on high gate and exit 3. Exit 3 is a claim that
# the review RAN and its findings reached the threshold -- a claim half a
# review cannot make. So the answer must be 1, not 3.
run gate noprov env LLM_API_KEY=k LLM_BASE_URL=http://127.0.0.1:9/v1 "$bin" review --base HEAD~1 --seguranca --fail-on high
[[ "$rc" -eq 1 ]] || fail "gate-precedence:want-1-got-$rc"
grep -Fq "$sec_header" <<<"$out" || fail gate-lost-the-security-section
[[ "$rc" -ne 3 ]] || fail gate-claimed-a-complete-review

# --- 2. THE PUBLISHED DEMO PATH MUST STILL BE GREEN ---------------------
# `review --base X --seguranca` with no credential at all is what the
# public demo and the self-review action run. It reviewed exactly what it
# was asked to review, so it exits 0 -- unchanged by this card.
run demo noprov "$bin" review --base HEAD~1 --seguranca
[[ "$rc" -eq 0 ]] || fail "demo-path-broken:want-0-got-$rc"
grep -Fq "$sec_header" <<<"$out" || fail demo-path-lost-findings

# --- 3. ...UNTIL THE CALLER ASKS FOR THE QUALITY HALF -------------------
run demo_strict noprov "$bin" review --base HEAD~1 --seguranca --exigir-qualidade
[[ "$rc" -eq 1 ]] || fail "exigir-qualidade-ignored:want-1-got-$rc"
grep -Fq "$sec_header" <<<"$out" || fail strict-run-lost-the-security-section
grep -Fq 'did not run' <<<"$err" || fail strict-run-did-not-explain-itself

# --- 4. THE FLAG IS NEVER SILENTLY DROPPED ------------------------------
run pr noprov "$bin" review --pr 1 --repo a/b --publicar --na-linha --exigir-qualidade
[[ "$rc" -eq 2 ]] || fail "pr-path-must-refuse-the-flag:want-2-got-$rc"

# --- 5. DETERMINISM -----------------------------------------------------
run det1 noprov "$bin" review --base HEAD~1 --seguranca
first="$out"
run det2 noprov "$bin" review --base HEAD~1 --seguranca
[[ "$first" == "$out" ]] || fail nondeterministic-output

printf '%s/E2E/ok\n' "$card"
exit 0
