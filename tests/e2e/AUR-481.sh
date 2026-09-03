#!/usr/bin/env bash
# E2E check for AUR-481: build (or reuse) the real aurumcode binary and run
# it, as a user would, against the committed Rust fixture repository
# tests/fixtures/review/vuln/rust-secret-sql-injection/repo.git. The
# sealed acceptance profile (bootstrap-readonly-v1) carries bash and a Go
# toolchain but no `git` binary, so the fixture is committed instead, built
# by tests/fixtures/repos/git-demo/build-fixture.sh -- the same git-less,
# deterministic, loose-object builder that produced every sibling fixture
# this card's own AC-003 regresses against. Nothing in this script shells
# out to `git`, at build time or run time.
#
# WHAT THIS PROVES, DISTINCT FROM tests/unit/AUR-481.go (package-boundary,
# synthetic types.Diff) AND tests/integration/AUR-481.go (CLI-boundary,
# stdout content assertions):
#
#   The 2026-08-26 measurement found Rust at zero for all three
#   deterministic security rules, because the catalog's patterns were
#   written to the C/Python/Node spelling of each defect and Rust's own
#   idiomatic spelling (a typed const/static declaration, a
#   String::from-wrapped literal, .to_owned()/.to_string() concatenation,
#   a format! macro query, and Command::new("sh").arg("-c")) never
#   matched. This script proves, at the full-process boundary: the six
#   Rust true-positive shapes this card restores all appear with exact
#   citation counts (AC-001); none of the card-named safe forms (a
#   numeric constant, a digit-free constant, a $1-parametrized query, an
#   argv-form Command with no shell) appear (AC-002); the run is
#   deterministic; and that a repository carrying only these six real
#   findings closes the `--fail-on high` gate with exit 3.
set -euo pipefail
export LC_ALL=C

ulimit -v 8388608 2>/dev/null || true
export GOMEMLIMIT=2GiB

readonly card=AUR-481
selector="${1:-E2EAUR481}"
[[ "$selector" == "E2EAUR481" ]] || { printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64; }

fail() { printf '%s/AC-001/%s\n' "$card" "$1" >&2; exit 1; }
infra() { printf '%s/AC-001/infrastructure/%s\n' "$card" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

command -v go >/dev/null 2>&1 || infra missing_go

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e2e-a481.XXXXXX")" || infra mktemp
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

rust_repo="$repo_root/tests/fixtures/review/vuln/rust-secret-sql-injection/repo.git"
test -d "$rust_repo" || fail missing-rust-fixture

header='Security findings (standards/security-review):'
secret_citation='(rule security/hardcoded-secret: Hardcoded Secrets)'
sql_citation='(rule security/sql-injection: SQL Injection Vulnerability)'
cmd_citation='(rule security/command-injection: Command Injection)'

out_sec="$(cd "$rust_repo" && "$bin" review --base HEAD~1 --seguranca)" || fail behavior-missing
grep -Fq "$header" <<<"$out_sec" || fail security-section-missing

# AC-001: the six Rust true-positive shapes all appear.
for ln in 1 6 11 15 24 28; do
  grep -Fq "src/main.rs:$ln: [error]" <<<"$out_sec" || fail "true-positive-line-$ln-not-found"
done

# AC-002: no other line in src/main.rs produces a finding (the fixture
# carries exactly 6 unsafe and 4 safe lines).
after_header="${out_sec#*"$header"}"
[[ "$(grep -Fo "$secret_citation" <<<"$after_header" | wc -l)" -eq 2 ]] || fail unexpected-secret-count
[[ "$(grep -Fo "$sql_citation" <<<"$after_header" | wc -l)" -eq 2 ]] || fail unexpected-sql-count
[[ "$(grep -Fo "$cmd_citation" <<<"$after_header" | wc -l)" -eq 2 ]] || fail unexpected-cmd-count
[[ "$(grep -Fo '[error]' <<<"$after_header" | wc -l)" -eq 6 ]] || fail unexpected-total-count

# Determinism.
out_again="$(cd "$rust_repo" && "$bin" review --base HEAD~1 --seguranca)" || fail rerun-failed
[[ "$out_sec" == "$out_again" ]] || fail non-deterministic

# The four real findings close --fail-on high.
set +e
(cd "$rust_repo" && "$bin" review --base HEAD~1 --seguranca --fail-on high) >/dev/null 2>"$run_dir/failon.err"
rc=$?
set -e
[[ "$rc" -eq 3 ]] || fail "fail-on-gate-did-not-close:$rc"

# Without --seguranca: no security section leaks in.
out_base="$(cd "$rust_repo" && "$bin" review --base HEAD~1 2>/dev/null || true)"
if grep -Fq "$header" <<<"$out_base"; then fail base-grew-a-security-section; fi

printf '%s/AC-001/e2e-ok\n' "$card"
