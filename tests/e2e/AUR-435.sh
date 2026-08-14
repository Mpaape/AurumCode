#!/usr/bin/env bash
# E2E check for AUR-435: build (or reuse) the real aurumcode binary and run
# it, as a user would, against the tests/fixtures/review/vuln bare
# repository whose HEAD~1..HEAD diff plants a synthetic SQL injection, with
# the LLM call pinned to a deterministic offline fixture. Proves that
# `review --base HEAD~1 --seguranca` reports the vulnerability in a security
# section separate from the quality findings, citing the security-category
# rule and the project security standard, and that without the flag the
# published output is untouched. See docs/specs/AUR-435.md.
set -euo pipefail
export LC_ALL=C

ulimit -v 8388608 2>/dev/null || true
export GOMEMLIMIT=2GiB

readonly card=AUR-435
selector="${1:-E2EAUR435}"
[[ "$selector" == "E2EAUR435" ]] || { printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64; }

fail() { printf '%s/AC-001/%s\n' "$card" "$1" >&2; exit 1; }
infra() { printf '%s/AC-001/infrastructure/%s\n' "$card" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

command -v go >/dev/null 2>&1 || infra missing_go

vuln_repo="$repo_root/tests/fixtures/review/vuln/repo.git"
test -d "$vuln_repo" || fail missing-vuln-fixture

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e2e-a435.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP

mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
# Reuse an already-warm build cache and an already-built binary when a
# caller provides them (tests/acceptance/AUR-435.sh's e2e_case does).
: "${GOCACHE:=$run_dir/gocache}"
: "${GOTMPDIR:=$run_dir/gotmp}"
export GOCACHE GOTMPDIR

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

# A deterministic model response with exactly one QUALITY finding, so the
# quality block and the security section are distinguishable.
fixture="$run_dir/quality.json"
cat >"$fixture" <<'EOF'
{
  "issues": [
    {
      "file": "src/db.py",
      "line": 4,
      "severity": "info",
      "rule_id": "quality/poor-naming",
      "message": "PLANTED-QUALITY-435 finding for the separation proof."
    }
  ],
  "summary": "Quality-only fixture for AUR-435."
}
EOF

header='Security findings (standards/security-review):'
citation='(rule security/sql-injection: SQL Injection Vulnerability)'

# Without the flag: the published contract, no security section.
out_base="$(cd "$vuln_repo" && AURUMCODE_LLM_FIXTURE="$fixture" "$bin" review --base HEAD~1)" || fail base-run-failed
grep -Fq 'PLANTED-QUALITY-435' <<<"$out_base" || fail base-quality-missing
if grep -Fq "$header" <<<"$out_base"; then fail base-grew-a-security-section; fi

# With the flag: the security pass finds the planted SQL injection in its
# own section, after the byte-identical base output.
out_sec="$(cd "$vuln_repo" && AURUMCODE_LLM_FIXTURE="$fixture" "$bin" review --base HEAD~1 --seguranca)" || fail behavior-missing
[[ "${out_sec:0:${#out_base}}" == "$out_base" ]] || fail base-bytes-rewritten
grep -Fq "$header" <<<"$out_sec" || fail security-section-missing
grep -Fq "src/db.py:8: [error]" <<<"$out_sec" || fail vulnerability-not-found
grep -Fq "$citation" <<<"$out_sec" || fail rule-citation-missing
grep -Fq 'standards/security-review SCR-001' <<<"$out_sec" || fail standard-citation-missing
# Separation both ways: the security citation only after the header, the
# quality marker only before it.
before_header="${out_sec%%"$header"*}"
after_header="${out_sec#*"$header"}"
if grep -Fq "$citation" <<<"$before_header"; then fail security-leaked-into-quality; fi
if grep -Fq 'PLANTED-QUALITY-435' <<<"$after_header"; then fail quality-leaked-into-security; fi

# Determinism: same input, same bytes.
out_again="$(cd "$vuln_repo" && AURUMCODE_LLM_FIXTURE="$fixture" "$bin" review --base HEAD~1 --seguranca)" || fail rerun-failed
[[ "$out_sec" == "$out_again" ]] || fail non-deterministic

printf '%s/AC-001/e2e-ok\n' "$card"
