#!/usr/bin/env bash
# E2E check for AUR-449: build (or reuse) the real aurumcode binary and run
# it, as a user would, against tests/fixtures/repos/git-demo (the project's
# own demo fixture, which AUR-442 already taught the deterministic security
# pass to find three planted secrets in) with NO LLM provider configured at
# all.
#
# Proves `aurumcode review --base HEAD~1 --seguranca` runs the security
# pass and reports its findings with exit 0 -- instead of refusing with "no
# LLM provider configured" -- because that pass is a regex matcher over the
# diff's added lines and calls no model; that the skip of the quality
# review is explained plainly on stderr, never silently; that the run is
# deterministic; that --fail-on still closes the gate off the security
# findings alone; that a configured provider keeps the AUR-442 contract
# byte for byte; that an explicit --modelo that cannot be served still
# fails loudly instead of being silently downgraded into the skip; that
# without --seguranca the pre-existing no-provider refusal is untouched;
# and that the secret canary never reaches a sink. See docs/specs/AUR-449.md.
set -euo pipefail
export LC_ALL=C

ulimit -v 8388608 2>/dev/null || true
export GOMEMLIMIT=2GiB

readonly card=AUR-449
selector="${1:-E2EAUR449}"
[[ "$selector" == "E2EAUR449" ]] || { printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64; }

fail() { printf '%s/AC-001/%s\n' "$card" "$1" >&2; exit 1; }
infra() { printf '%s/AC-001/infrastructure/%s\n' "$card" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

command -v go >/dev/null 2>&1 || infra missing_go

demo_repo="$repo_root/tests/fixtures/repos/git-demo/repo.git"
test -d "$demo_repo" || infra missing-git-demo-fixture

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e2e-a449.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP

mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
# Reuse an already-warm build cache and an already-built binary when a
# caller provides them (tests/acceptance/AUR-449.sh's e2e_case does).
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
standard_citation='standards/security-review SCR-003'

# No provider at all, --seguranca alone: the security pass runs and reports
# its findings, exit 0 -- the card's central proof.
out_noprov="$(cd "$demo_repo" && env -u AURUMCODE_LLM_FIXTURE -u LLM_API_KEY -u LLM_BASE_URL "$bin" review --base HEAD~1 --seguranca 2>"$run_dir/noprov.err")" || fail behavior-missing
err_noprov="$(cat "$run_dir/noprov.err")"
grep -Fq "$header" <<<"$out_noprov" || fail security-section-missing
for line in 4 5 6; do
  grep -Fq "config/demo-tokens.txt:${line}: [error]" <<<"$out_noprov" || fail "demo-secret-not-found:${line}"
done
if grep -Fq 'No issues found.' <<<"$out_noprov"; then fail quality-section-falsely-claimed; fi
grep -Fq 'no LLM provider configured' <<<"$err_noprov" || fail skip-note-missing
grep -Fq 'quality review skipped' <<<"$err_noprov" || fail skip-note-missing

# Determinism.
out_again="$(cd "$demo_repo" && env -u AURUMCODE_LLM_FIXTURE -u LLM_API_KEY -u LLM_BASE_URL "$bin" review --base HEAD~1 --seguranca)" || fail rerun-failed
[[ "$out_noprov" == "$out_again" ]] || fail non-deterministic

# Honest absence on the skip path: --base HEAD against itself is an empty
# diff (no new fixture needed) -- the pass still ran and found nothing,
# which must print as "No security findings.", not look like the pass
# silently never ran.
out_clean="$(cd "$demo_repo" && env -u AURUMCODE_LLM_FIXTURE -u LLM_API_KEY -u LLM_BASE_URL "$bin" review --base HEAD --seguranca)" || fail clean-run-failed
grep -Fq "$header" <<<"$out_clean" || fail honest-absence-header-missing
grep -Fq 'No security findings.' <<<"$out_clean" || fail honest-absence-missing

# --fail-on still closes the gate off the security findings alone.
set +e
(cd "$demo_repo" && env -u AURUMCODE_LLM_FIXTURE -u LLM_API_KEY -u LLM_BASE_URL "$bin" review --base HEAD~1 --seguranca --fail-on high) >/dev/null 2>"$run_dir/failon.err"
rc=$?
set -e
[[ "$rc" -eq 3 ]] || fail "fail-on-gate-did-not-close:$rc"

# With a provider configured, the AUR-442 contract is byte-for-byte
# unaffected by this card.
out_prov="$(cd "$demo_repo" && AURUMCODE_LLM_FIXTURE="$fixture" "$bin" review --base HEAD~1 --seguranca)" || fail provider-run-failed
grep -Fq "$header" <<<"$out_prov" || fail provider-security-section-missing
after_header="${out_prov#*"$header"}"
citation_count="$(grep -Fo "$citation" <<<"$after_header" | wc -l)"
[[ "$citation_count" -eq 3 ]] || fail "unexpected-citation-count:$citation_count"
grep -Fq "$standard_citation" <<<"$out_prov" || fail standard-citation-missing
for value in AURUM-FAKE-TOKEN-0000-0001 AURUM-FAKE-PASSWORD-0000-0002 AURUM-FAKE-WEBHOOK-0000-0003; do
  if grep -Fq "$value" <<<"$out_prov"; then fail "secret-value-leaked:$value"; fi
done

# An explicit --modelo that cannot be served still fails loudly: never
# silently downgraded into the skip just because --seguranca is present too.
set +e
out_modelo="$(cd "$demo_repo" && env -u AURUMCODE_LLM_FIXTURE -u LLM_API_KEY -u LLM_BASE_URL "$bin" review --base HEAD~1 --seguranca --modelo local 2>"$run_dir/modelo.err")"
rc=$?
set -e
err_modelo="$(cat "$run_dir/modelo.err")"
[[ "$rc" -eq 1 ]] || fail "explicit-modelo-must-still-fail:$rc"
grep -Fq 'model "local" is unavailable' <<<"$err_modelo" || fail explicit-modelo-error-missing
if grep -Fq "$header" <<<"$out_modelo"; then fail explicit-modelo-must-not-print-security-section; fi

# Without --seguranca, the pre-existing no-provider refusal is untouched.
set +e
out_plain="$(cd "$demo_repo" && env -u AURUMCODE_LLM_FIXTURE -u LLM_API_KEY -u LLM_BASE_URL "$bin" review --base HEAD~1 2>"$run_dir/plain.err")"
rc=$?
set -e
err_plain="$(cat "$run_dir/plain.err")"
[[ "$rc" -eq 1 ]] || fail "no-seguranca-no-provider-must-still-fail:$rc"
[[ -z "$out_plain" ]] || fail no-seguranca-unexpected-stdout
grep -Fq 'no LLM provider configured' <<<"$err_plain" || fail no-seguranca-error-missing
if grep -Fq 'quality review skipped' <<<"$err_plain"; then fail skip-note-must-not-appear-without-seguranca; fi

# The secret canary never reaches a sink on the skip path.
canary="aurum-canary-449-e2e-$$"
out_canary="$(cd "$demo_repo" && env -u AURUMCODE_LLM_FIXTURE -u LLM_API_KEY -u LLM_BASE_URL AURUM_SECRET_CANARY="$canary" "$bin" review --base HEAD~1 --seguranca 2>"$run_dir/canary.err")" || fail canary-run-failed
err_canary="$(cat "$run_dir/canary.err")"
if grep -Fq "$canary" <<<"$out_canary$err_canary"; then fail canary-leaked; fi

printf '%s/AC-001/e2e-ok\n' "$card"
