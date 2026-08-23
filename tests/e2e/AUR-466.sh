#!/usr/bin/env bash
# E2E check for AUR-466: build (or reuse) the real aurumcode binary and run
# it, as a user would, against the committed Node fixture repository
# tests/fixtures/review/vuln/node-placeholder-vs-secret/repo.git. The
# sealed acceptance profile (bootstrap-readonly-v1) carries bash and a Go
# toolchain but no `git` binary, so the fixture is committed instead, built
# by tests/fixtures/repos/git-demo/build-fixture.sh -- the same git-less,
# deterministic, loose-object builder that produced every sibling fixture
# this card's own AC-003 regresses against. Nothing in this script shells
# out to `git`, at build time or run time.
#
# WHAT THIS PROVES, DISTINCT FROM tests/unit/AUR-466.go (package-boundary,
# synthetic types.Diff) AND tests/integration/AUR-466.go (CLI-boundary,
# stdout content assertions):
#
#   The 2026-08-14 measurement on a real Node repository found 15
#   security/`error` findings and every sampled one false: a doc
#   placeholder, a help-text string, two test-fixture assignments, and a
#   SQL query built from constants. This script proves, at the
#   full-process boundary: none of those five false-positive shapes ever
#   appear (AC-001); a real digit-bearing secret, a SQL query
#   concatenating a variable, and a shell command concatenating a variable
#   all three appear with exact citation counts (AC-002); the run is
#   deterministic; and, distinct from the integration program's own check,
#   that a repository carrying ONLY these three real findings closes the
#   `--fail-on high` gate with exit 3, while --fail-on high against the
#   AC-001-only surface (none of the false positives) would never close it.
set -euo pipefail
export LC_ALL=C

ulimit -v 8388608 2>/dev/null || true
export GOMEMLIMIT=2GiB

readonly card=AUR-466
selector="${1:-E2EAUR466}"
[[ "$selector" == "E2EAUR466" ]] || { printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64; }

fail() { printf '%s/AC-001/%s\n' "$card" "$1" >&2; exit 1; }
infra() { printf '%s/AC-001/infrastructure/%s\n' "$card" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

command -v go >/dev/null 2>&1 || infra missing_go

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e2e-a466.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP

mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
: "${GOCACHE:=$run_dir/gocache}"
: "${GOTMPDIR:=$run_dir/gotmp}"
export GOCACHE GOTMPDIR

unset LLM_API_KEY LLM_BASE_URL AURUMCODE_LLM_FIXTURE

if [[ -n "${AURUMCODE_BIN:-}" ]]; then
  bin="$AURUMCODE_BIN"
  test -x "$bin" || infra missing_prebuilt_binary
else
  bin="$run_dir/aurumcode"
  build_log="$run_dir/build.log"
  if ! (cd "$repo_root" && GOFLAGS='-mod=mod -p=1' go build -o "$bin" ./cmd/aurumcode) >"$build_log" 2>&1; then
    cat "$build_log" >&2
    fail build_failed
  fi
fi

node_repo="$repo_root/tests/fixtures/review/vuln/node-placeholder-vs-secret/repo.git"
test -d "$node_repo" || fail missing-node-fixture

header='Security findings (standards/security-review):'
secret_citation='(rule security/hardcoded-secret: Hardcoded Secrets)'
sql_citation='(rule security/sql-injection: SQL Injection Vulnerability)'
cmd_citation='(rule security/command-injection: Command Injection)'

out_sec="$(cd "$node_repo" && "$bin" review --base HEAD~1 --seguranca)" || fail behavior-missing
grep -Fq "$header" <<<"$out_sec" || fail security-section-missing

# AC-001: none of the five measured false-positive shapes ever appear.
grep -Fq 'README.md:' <<<"$out_sec" && fail doc-placeholder-false-positive
for ln in 6 10 11 16; do
  grep -Fq "src/app.js:$ln: [error]" <<<"$out_sec" && fail "false-positive-line-$ln"
done

# AC-002: the real secret, SQL variable concat, and shell variable concat
# all three appear.
grep -Fq 'src/app.js:21: [error]' <<<"$out_sec" || fail real-secret-not-found
grep -Fq 'src/app.js:24: [error]' <<<"$out_sec" || fail sql-variable-concat-not-found
grep -Fq 'src/app.js:29: [error]' <<<"$out_sec" || fail shell-variable-concat-not-found

after_header="${out_sec#*"$header"}"
[[ "$(grep -Fo "$secret_citation" <<<"$after_header" | wc -l)" -eq 1 ]] || fail unexpected-secret-count
[[ "$(grep -Fo "$sql_citation" <<<"$after_header" | wc -l)" -eq 1 ]] || fail unexpected-sql-count
[[ "$(grep -Fo "$cmd_citation" <<<"$after_header" | wc -l)" -eq 1 ]] || fail unexpected-cmd-count
[[ "$(grep -Fo '[error]' <<<"$after_header" | wc -l)" -eq 3 ]] || fail unexpected-total-count

# Determinism.
out_again="$(cd "$node_repo" && "$bin" review --base HEAD~1 --seguranca)" || fail rerun-failed
[[ "$out_sec" == "$out_again" ]] || fail non-deterministic

# The three real findings close --fail-on high.
set +e
(cd "$node_repo" && "$bin" review --base HEAD~1 --seguranca --fail-on high) >/dev/null 2>"$run_dir/failon.err"
rc=$?
set -e
[[ "$rc" -eq 3 ]] || fail "fail-on-gate-did-not-close:$rc"

# Without --seguranca: no security section leaks in.
out_base="$(cd "$node_repo" && "$bin" review --base HEAD~1 2>/dev/null || true)"
if grep -Fq "$header" <<<"$out_base"; then fail base-grew-a-security-section; fi

printf '%s/AC-001/e2e-ok\n' "$card"
