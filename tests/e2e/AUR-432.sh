#!/usr/bin/env bash
# E2E check for AUR-432: plant runtime-only synthetic secrets inside a
# reviewed git repository (rebuilt at runtime with the git-less AUR-437
# fixture builder), run the real `aurumcode review --base HEAD~1` as a
# user would, and prove that no planted value reaches the model prompt,
# stdout, or stderr, while the finding citing the planted line still
# arrives.
#
# The planted values are a structural key/value secret and a registered
# canary. A credential-SHAPED value (sk-...) deliberately cannot ride this
# path: the AUR-437 builder refuses any credential shape in fixture
# content, mirroring the sealed runner's input gate -- this script proves
# that refusal instead of bypassing it, and the shape rule itself is
# proved at the model boundary by tests/unit/AUR-432.go, where the diff is
# built in process. See docs/specs/AUR-432.md.
set -euo pipefail
export LC_ALL=C

readonly card=AUR-432
selector="${1:-E2EAUR432}"
[[ "$selector" == "E2EAUR432" ]] || { printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64; }

fail() { printf '%s/AC-001/%s\n' "$card" "$1" >&2; exit 1; }
infra() { printf '%s/AC-001/infrastructure/%s\n' "$card" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

fixture_src="$repo_root/tests/fixtures/repos/git-demo"
test -f "$fixture_src/build-fixture.sh" || infra missing_fixture_builder
test -f "$fixture_src/history.spec" || infra missing_fixture_spec
test -d "$fixture_src/content" || infra missing_fixture_content

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e2e-a432.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP

mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
: "${GOCACHE:=$run_dir/gocache}"
: "${GOTMPDIR:=$run_dir/gotmp}"
export GOCACHE GOTMPDIR

if [[ -n "${AURUMCODE_BIN:-}" ]]; then
  bin="$AURUMCODE_BIN"
  test -x "$bin" || infra missing_prebuilt_binary
else
  command -v go >/dev/null 2>&1 || infra missing_go
  bin="$run_dir/aurumcode"
  build_log="$run_dir/build.log"
  if ! (cd "$repo_root" && GOFLAGS=-mod=mod go build -o "$bin" ./cmd/aurumcode) >"$build_log" 2>&1; then
    cat "$build_log" >&2
    fail build_failed
  fi
fi

# Runtime-only synthetic secrets. The credential-shaped value is
# assembled from split literals (tests/fixtures/review/secret/
# assemble-credential.sh) so no tracked file carries the shape; the
# key/value secret and the canary are structural-and-registered values the
# fixture builder accepts. The canary is registered through
# AURUM_SECRET_CANARY, exercising the registered-value rule independently
# of any structural key/value context.
cred="$(bash "$repo_root/tests/fixtures/review/secret/assemble-credential.sh")" || infra assemble_credential
[[ -n "$cred" ]] || infra assemble_credential_empty
planted_kv='AURUM-E2E-KV-VALUE-0117'
canary='AURUM-E2E-REGISTERED-0424'
# The anchored-header vector the AUR-432 review proved leaking: a diff
# line "+Authorization: Bearer <value>" defeats the filter's line-anchored
# header rule unless the composition strips the diff marker first. The
# value is assembled at runtime; it exists in no tracked file.
bearer="AURUM-E2E-BEARER-$(printf '%s' '3117')"

stage_src="$run_dir/fixture-src"
mkdir -p "$stage_src"
cp -R "$fixture_src/build-fixture.sh" "$fixture_src/history.spec" "$fixture_src/content" "$stage_src/"
chmod -R u+w -- "$stage_src"
mkdir -p "$stage_src/content/03/config"
# shellcheck disable=SC2016
awk '{print} $0 == "add 03 config/demo-tokens.txt" {print "add 03 config/secrets.env"}' \
  "$stage_src/history.spec" >"$stage_src/history-planted.spec"
grep -Fq 'add 03 config/secrets.env' "$stage_src/history-planted.spec" || infra spec_patch_failed

build_planted() {
  local out="$1"
  local log="$2"
  set +e
  bash "$stage_src/build-fixture.sh" "$stage_src/history-planted.spec" "$out" \
    >"$log" 2>&1
  fixture_rc=$?
  set -e
}

# First, the negative half: the AUR-437 builder must REFUSE a
# credential-shaped value in fixture content -- the same refusal the
# sealed runner applies to tracked inputs. If this ever stops refusing,
# the shape has found a path into a buildable fixture and this card's
# threat model changed.
{
  printf '# runtime-planted synthetic credentials (AUR-432 e2e); never real\n'
  printf 'service_key=%s\n' "$cred"
} >"$stage_src/content/03/config/secrets.env"
build_planted "$run_dir/fixture-refused" "$run_dir/fixture-refused.log"
if (( fixture_rc == 3 )); then
  infra "fixture_builder_environment:$fixture_rc"
elif (( fixture_rc == 0 )); then
  fail credential_shape_not_refused
fi
grep -Fq 'non-synthetic-secret' "$run_dir/fixture-refused.log" || fail credential_shape_refusal_untyped

# Then the positive half: plant the structural key/value secret and the
# canary, which the builder accepts, and rebuild the repository for real.
{
  printf '# runtime-planted synthetic credentials (AUR-432 e2e); never real\n'
  printf 'service_password=%s\n' "$planted_kv"
  printf 'incident marker %s\n' "$canary"
  printf 'Authorization: Bearer %s\n' "$bearer"
} >"$stage_src/content/03/config/secrets.env"
fixture_out="$run_dir/fixture-out"
build_fixture_log="$run_dir/fixture-build.log"
build_planted "$fixture_out" "$build_fixture_log"
if (( fixture_rc == 3 )); then
  infra "fixture_builder_environment:$fixture_rc"
elif (( fixture_rc != 0 )); then
  cat "$build_fixture_log" >&2
  fail "fixture_build_failed:exit:$fixture_rc"
fi
repo_dir="$fixture_out/repo.git"
test -d "$repo_dir" || fail fixture_build_no_repo

# Deterministic offline model response citing the planted line. Its
# message deliberately quotes the offending header diff line, marker and
# all, exactly the way a model echoes what it saw -- the output boundary
# must redact it before it can reach stdout.
response="$run_dir/response.json"
cat >"$response" <<EOF
{
  "issues": [
    {
      "file": "config/secrets.env",
      "line": 2,
      "severity": "error",
      "rule_id": "security/hardcoded-secret",
      "message": "A hardcoded credential is committed in plain text at this line, together with:\n+Authorization: Bearer ${bearer}\nRotate both.",
      "suggestion": "Remove the value, rotate it, and load it from the environment."
    }
  ],
  "summary": "The change commits a plaintext credential in config/secrets.env."
}
EOF
grep -Fq -- "$bearer" "$response" || infra response_interpolation_failed

capture="$run_dir/prompt.txt"
run_review() {
  set +e
  (cd "$repo_dir" && \
    AURUMCODE_LLM_FIXTURE="$response" \
    AURUMCODE_PROMPT_CAPTURE="$capture" \
    AURUM_SECRET_CANARY="$canary" \
    "$bin" review --base HEAD~1) \
    >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  rc=$?
  set -e
}

run_review
[[ "$rc" -eq 0 ]] || fail "review_failed:exit:$rc"
test -s "$capture" || fail prompt_capture_missing

# Neither the credential-shaped value nor the registered canary may exist
# in anything that left the process. Typed markers only: this script never
# echoes a planted value.
for sink in "$capture" "$run_dir/out.stdout" "$run_dir/out.stderr"; do
  if grep -Fq -- "$planted_kv" "$sink"; then fail "secret_leaked:${sink##*/}"; fi
  if grep -Fq -- "$canary" "$sink"; then fail "canary_leaked:${sink##*/}"; fi
  if grep -Fq -- "$bearer" "$sink"; then fail "header_credential_leaked:${sink##*/}"; fi
done

# The redaction replaced values, not context: the model still saw the
# file, the keys, the header name, and the marker where each secret was,
# and the echoed header line reached stdout redacted the same way.
grep -Fq '[REDACTED]' "$capture" || fail redaction_marker_missing
grep -Fq 'config/secrets.env' "$capture" || fail review_context_destroyed
grep -Fq 'service_password=' "$capture" || fail review_context_destroyed
grep -Fq 'Authorization: [REDACTED]' "$capture" || fail header_not_redacted_in_prompt
grep -Fq 'Authorization: [REDACTED]' "$run_dir/out.stdout" || fail header_not_redacted_on_stdout

# The finding citing the planted line still reaches the user.
grep -Fq 'config/secrets.env:2: [error]' "$run_dir/out.stdout" || fail finding_lost

# Determinism: same input, same redacted prompt, same output.
first_stdout="$(cat "$run_dir/out.stdout")"
first_capture="$(cat "$capture")"
run_review
[[ "$rc" -eq 0 ]] || fail non_deterministic_exit
[[ "$(cat "$run_dir/out.stdout")" == "$first_stdout" ]] || fail non_deterministic_stdout
[[ "$(cat "$capture")" == "$first_capture" ]] || fail non_deterministic_prompt

printf '%s/AC-001/e2e-ok\n' "$card"
