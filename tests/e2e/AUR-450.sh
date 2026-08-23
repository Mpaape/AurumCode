#!/usr/bin/env bash
# E2E check for AUR-450: build (or reuse) the real aurumcode binary and run
# it, as a user would, against tests/fixtures/repos/git-demo (the project's
# own demo fixture, which already contains three planted secrets AUR-442
# taught the deterministic security pass to find) and against an empty diff
# (--base HEAD against itself, no findings possible).
#
# Proves `aurumcode review --base <ref> --seguranca` now names, on stderr,
# how many and which security-category rules of the embedded catalog it
# actually applied (carry a matcher) against how many the category
# declares in total -- identically whether the pass found something or
# found nothing -- so "No security findings." is never misread as "the
# code was scanned by the full catalog." Also proves stdout (the findings
# themselves, their content and order, and the AUR-442/AUR-449 byte
# contract with a provider configured) is completely untouched, and that
# --fail-on/--modelo compose exactly as before. See docs/specs/AUR-450.md.
set -euo pipefail
export LC_ALL=C

ulimit -v 8388608 2>/dev/null || true
export GOMEMLIMIT=2GiB

readonly card=AUR-450
selector="${1:-E2EAUR450}"
[[ "$selector" == "E2EAUR450" ]] || { printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64; }

fail() { printf '%s/AC-001/%s\n' "$card" "$1" >&2; exit 1; }
infra() { printf '%s/AC-001/infrastructure/%s\n' "$card" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

command -v go >/dev/null 2>&1 || infra missing_go

demo_repo="$repo_root/tests/fixtures/repos/git-demo/repo.git"
test -d "$demo_repo" || infra missing-git-demo-fixture

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e2e-a450.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP

mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
# Reuse an already-warm build cache and an already-built binary when a
# caller provides them (tests/acceptance/AUR-450.sh's e2e_case does).
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

fixture="$repo_root/tests/fixtures/review/known-problem-response.json"
test -f "$fixture" || infra missing-known-problem-fixture

header='Security findings (standards/security-review):'
citation='(rule security/hardcoded-secret: Hardcoded Secrets)'

# The AUR-450 coverage note: catalog-wide, so it is identical no matter
# what the diff contains -- proved below on both the findings case and the
# empty case.
coverage_prefix='aurumcode review: security pass applied 4 of 8 security rules ('
coverage_rules=(security/command-injection security/hardcoded-secret security/sql-injection)
coverage_pointer='internal/review/rules/security.yml'

noprov_env() {
  env -u AURUMCODE_LLM_FIXTURE -u LLM_API_KEY -u LLM_BASE_URL "$@"
}

# 1. The security pass finds something (git-demo, no provider needed at
# all -- AUR-449's skip path): the coverage note is on stderr, names all
# three matcher-backed rule ids and the total, and points at the catalog
# file. stdout is completely unaffected: the findings and their order are
# exactly what AUR-442/AUR-449 already publish.
out_sec="$(cd "$demo_repo" && noprov_env "$bin" review --base HEAD~1 --seguranca 2>"$run_dir/sec.err")" || fail behavior-missing
err_sec="$(cat "$run_dir/sec.err")"
grep -Fq "$header" <<<"$out_sec" || fail security-section-missing
for line in 4 5 6; do
  grep -Fq "config/demo-tokens.txt:${line}: [error]" <<<"$out_sec" || fail "demo-secret-not-found:${line}"
done
grep -Fq "$coverage_prefix" <<<"$err_sec" || fail coverage-note-missing
for rule in "${coverage_rules[@]}"; do
  grep -Fq "$rule" <<<"$err_sec" || fail "coverage-note-missing-rule:$rule"
done
grep -Fq "$coverage_pointer" <<<"$err_sec" || fail coverage-note-missing-pointer

# 2. Honest absence, but the coverage note is IDENTICAL: --base HEAD
# against itself is an empty diff, so the pass finds nothing -- this is
# the exact case the card's Outcome exists to fix: "No security findings."
# must not be misread as "the code was scanned in full."
out_clean="$(cd "$demo_repo" && noprov_env "$bin" review --base HEAD --seguranca 2>"$run_dir/clean.err")" || fail clean-run-failed
err_clean="$(cat "$run_dir/clean.err")"
grep -Fq "$header" <<<"$out_clean" || fail honest-absence-header-missing
grep -Fq 'No security findings.' <<<"$out_clean" || fail honest-absence-missing
[[ "$err_clean" == "$err_sec" ]] || fail "coverage-note-differs-between-empty-and-nonempty:$err_clean"

# 3. Determinism.
out_again="$(cd "$demo_repo" && noprov_env "$bin" review --base HEAD~1 --seguranca 2>"$run_dir/again.err")" || fail rerun-failed
[[ "$out_sec" == "$out_again" ]] || fail non-deterministic-stdout
[[ "$err_sec" == "$(cat "$run_dir/again.err")" ]] || fail non-deterministic-stderr

# 4. With a provider configured, stdout stays exactly the AUR-442/AUR-449
# contract (this card owns no byte of it), and the SAME coverage note
# appears on stderr.
out_prov="$(cd "$demo_repo" && AURUMCODE_LLM_FIXTURE="$fixture" "$bin" review --base HEAD~1 --seguranca 2>"$run_dir/prov.err")" || fail provider-run-failed
err_prov="$(cat "$run_dir/prov.err")"
grep -Fq "$header" <<<"$out_prov" || fail provider-security-section-missing
after_header="${out_prov#*"$header"}"
citation_count="$(grep -Fo "$citation" <<<"$after_header" | wc -l)"
[[ "$citation_count" -eq 3 ]] || fail "unexpected-citation-count:$citation_count"
grep -Fq "$coverage_prefix" <<<"$err_prov" || fail provider-coverage-note-missing
for value in AURUM-FAKE-TOKEN-0000-0001 AURUM-FAKE-PASSWORD-0000-0002 AURUM-FAKE-WEBHOOK-0000-0003; do
  if grep -Fq "$value" <<<"$out_prov$err_prov"; then fail "secret-value-leaked:$value"; fi
done

# 5. --fail-on still closes the gate off the security findings, unaffected
# by the new note.
set +e
(cd "$demo_repo" && noprov_env "$bin" review --base HEAD~1 --seguranca --fail-on high) >/dev/null 2>"$run_dir/failon.err"
rc=$?
set -e
[[ "$rc" -eq 3 ]] || fail "fail-on-gate-did-not-close:$rc"

# 6. --modelo unavailable still fails loudly, before any --seguranca
# output (coverage note included) ever prints.
set +e
out_modelo="$(cd "$demo_repo" && noprov_env "$bin" review --base HEAD~1 --seguranca --modelo local 2>"$run_dir/modelo.err")"
rc=$?
set -e
err_modelo="$(cat "$run_dir/modelo.err")"
[[ "$rc" -eq 1 ]] || fail "explicit-modelo-must-still-fail:$rc"
grep -Fq 'model "local" is unavailable' <<<"$err_modelo" || fail explicit-modelo-error-missing
if grep -Fq "$header" <<<"$out_modelo"; then fail explicit-modelo-must-not-print-security-section; fi
if grep -Fq "$coverage_prefix" <<<"$err_modelo"; then fail explicit-modelo-must-not-print-coverage-note; fi

# 7. Without --seguranca, no coverage note anywhere -- it is scoped to the
# security pass, not printed on every review.
out_plain="$(cd "$demo_repo" && AURUMCODE_LLM_FIXTURE="$fixture" "$bin" review --base HEAD~1 2>"$run_dir/plain.err")" || fail plain-run-failed
err_plain="$(cat "$run_dir/plain.err")"
if grep -Fq "$coverage_prefix" <<<"$out_plain$err_plain"; then fail coverage-note-must-not-appear-without-seguranca; fi

# 8. The secret canary never reaches a sink alongside the new note.
canary="aurum-canary-450-e2e-$$"
out_canary="$(cd "$demo_repo" && noprov_env env AURUM_SECRET_CANARY="$canary" "$bin" review --base HEAD~1 --seguranca 2>"$run_dir/canary.err")" || fail canary-run-failed
err_canary="$(cat "$run_dir/canary.err")"
if grep -Fq "$canary" <<<"$out_canary$err_canary"; then fail canary-leaked; fi

printf '%s/AC-001/e2e-ok\n' "$card"
