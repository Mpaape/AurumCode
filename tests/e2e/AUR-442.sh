#!/usr/bin/env bash
# E2E check for AUR-442: build (or reuse) the real aurumcode binary and run
# it, as a user would, against two bare repositories with the LLM call
# pinned to a deterministic offline fixture:
#
#   - tests/fixtures/review/vuln/hardcoded-secret: a fixture new under this
#     card (sibling to, and never touching, AUR-435's own
#     tests/fixtures/review/vuln/repo.git) whose HEAD~1..HEAD diff plants
#     two synthetic hardcoded secrets in two shapes -- unquoted env-style
#     and quoted Python -- beside one deliberately benign line each.
#   - tests/fixtures/repos/git-demo: the project's own demo fixture, whose
#     sole purpose is to plant a plaintext secret. Before this card its
#     `--seguranca` run reported "No security findings." -- the defect
#     AUR-442's dogfooding measured. This proves it now reports the secret.
#
# Proves `review --base HEAD~1 --seguranca` reports security/hardcoded-secret
# findings in the security section, citing the rule and the project
# standard SCR-003, without echoing the matched secret value; that without
# the flag the published output is untouched; and that the secret canary
# never reaches a sink. See docs/specs/AUR-442.md.
set -euo pipefail
export LC_ALL=C

ulimit -v 8388608 2>/dev/null || true
export GOMEMLIMIT=2GiB

readonly card=AUR-442
selector="${1:-E2EAUR442}"
[[ "$selector" == "E2EAUR442" ]] || { printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64; }

fail() { printf '%s/AC-001/%s\n' "$card" "$1" >&2; exit 1; }
infra() { printf '%s/AC-001/infrastructure/%s\n' "$card" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

command -v go >/dev/null 2>&1 || infra missing_go

secret_repo="$repo_root/tests/fixtures/review/vuln/hardcoded-secret/repo.git"
demo_repo="$repo_root/tests/fixtures/repos/git-demo/repo.git"
test -d "$secret_repo" || fail missing-hardcoded-secret-fixture
test -d "$demo_repo" || infra missing-git-demo-fixture

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e2e-a442.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP

mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
# Reuse an already-warm build cache and an already-built binary when a
# caller provides them (tests/acceptance/AUR-442.sh's e2e_case does).
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

# Without the flag: the published contract, no security section, on either
# repository.
for repo in "$secret_repo" "$demo_repo"; do
  out_base="$(cd "$repo" && AURUMCODE_LLM_FIXTURE="$fixture" "$bin" review --base HEAD~1)" || fail "base-run-failed:$repo"
  if grep -Fq "$header" <<<"$out_base"; then fail "base-grew-a-security-section:$repo"; fi
done

# The dedicated fixture: both planted shapes are found, in their own
# section, with the standard citation and without echoing the value.
out_sec="$(cd "$secret_repo" && AURUMCODE_LLM_FIXTURE="$fixture" "$bin" review --base HEAD~1 --seguranca)" || fail behavior-missing
grep -Fq "$header" <<<"$out_sec" || fail security-section-missing
grep -Fq 'config/secrets.env:4: [error]' <<<"$out_sec" || fail unquoted-shape-not-found
grep -Fq 'src/config.py:4: [error]' <<<"$out_sec" || fail quoted-shape-not-found
grep -Fq "$standard_citation" <<<"$out_sec" || fail standard-citation-missing
after_header="${out_sec#*"$header"}"
citation_count="$(grep -Fo "$citation" <<<"$after_header" | wc -l)"
[[ "$citation_count" -eq 2 ]] || fail "unexpected-citation-count:$citation_count"
if grep -Fq 'AURUM-FAKE-KEY-9000-2222' <<<"$out_sec"; then fail secret-value-leaked; fi
if grep -Fq 'AURUM-FAKE-PASSWORD-9000-1111' <<<"$out_sec"; then fail secret-value-leaked; fi

# Determinism: same input, same bytes.
out_again="$(cd "$secret_repo" && AURUMCODE_LLM_FIXTURE="$fixture" "$bin" review --base HEAD~1 --seguranca)" || fail rerun-failed
[[ "$out_sec" == "$out_again" ]] || fail non-deterministic

# The card's central proof: git-demo's own planted plaintext secret is now
# reported instead of the honest-looking but wrong "No security findings."
# Checks below are scoped to AFTER the header on purpose:
# known-problem-response.json's own QUALITY finding (the model pass)
# independently cites "config/demo-tokens.txt:4: [error] ..." for the same
# planted line, so an unscoped check could pass on the model's citation
# alone without the deterministic security pass having matched anything.
out_demo="$(cd "$demo_repo" && AURUMCODE_LLM_FIXTURE="$fixture" "$bin" review --base HEAD~1 --seguranca)" || fail demo-run-failed
grep -Fq "$header" <<<"$out_demo" || fail demo-security-section-missing
demo_section="${out_demo#*"$header"}"
if grep -Fq 'No security findings.' <<<"$demo_section"; then fail demo-still-reports-absence; fi
for line in 4 5 6; do
  grep -Fq "config/demo-tokens.txt:${line}: [error]" <<<"$demo_section" || fail "demo-secret-not-found:${line}"
done
for value in AURUM-FAKE-TOKEN-0000-0001 AURUM-FAKE-PASSWORD-0000-0002 AURUM-FAKE-WEBHOOK-0000-0003; do
  if grep -Fq "$value" <<<"$out_demo"; then fail "demo-secret-value-leaked:$value"; fi
done

# Composed with --fail-on: the matched secret (severity error) now closes
# the gate on git-demo.
set +e
(cd "$demo_repo" && AURUMCODE_LLM_FIXTURE="$fixture" "$bin" review --base HEAD~1 --seguranca --fail-on high) >/dev/null 2>"$run_dir/failon.err"
rc=$?
set -e
[[ "$rc" -eq 3 ]] || fail "fail-on-gate-did-not-close:$rc"

# The secret canary never reaches a sink.
canary="aurum-canary-442-e2e-$$"
out_canary="$(cd "$demo_repo" && AURUM_SECRET_CANARY="$canary" AURUMCODE_LLM_FIXTURE="$fixture" "$bin" review --base HEAD~1 --seguranca 2>"$run_dir/canary.err")" || fail canary-run-failed
err_canary="$(cat "$run_dir/canary.err")"
if grep -Fq "$canary" <<<"$out_canary$err_canary"; then fail canary-leaked; fi

printf '%s/AC-001/e2e-ok\n' "$card"
