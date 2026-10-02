#!/usr/bin/env bash
# AUR-525 acceptance: the OpenAI-compatible provider sends tool definitions,
# returns the model's tool calls intact, serializes a multi-message tool
# conversation with its ids, rejects malformed arguments with a typed error,
# lets callers detect tool support before a request, and leaves the plain
# Complete request byte-identical.
#
# Selectors:
#   all             run every behavior test below
#   AC-001 .. AC-005
#   AC-002-MUT-001  drop tool_call_id when serializing a tool result; AC-002
#                   must fail (RED)
# Unknown selectors exit 64; infrastructure failures exit 79; behavioral
# failures exit 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-525'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-005|AC-002-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

for input in go.mod go.sum internal pkg; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done
[[ -f "$repo_root/internal/llm/provider/litellm/aur525_test.go" ]] || infra missing-behavior-test

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a525.XXXXXX")" || infra mktemp
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/root" "$run_dir/cache" "$run_dir/gotmp"
for source in go.mod go.sum internal pkg; do
  cp -R "$repo_root/$source" "$run_dir/root/$source"
done
chmod -R u+w -- "$run_dir/root"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

# MUT-001: the tool result stops naming the call it answers. Anchored on the
# stable field assignment; the token is split so this file cannot match its
# own edit, and a missing anchor is infrastructure, never a silent no-op.
apply_mutation() {
  local target="$run_dir/root/internal/llm/provider/litellm/tools.go"
  local anchor='ToolCallID: m.'"ToolCallID}"
  grep -Fq "$anchor" "$target" || infra mutation-anchor-missing
  sed -i "s|${anchor}|ToolCallID: \"\"}|" "$target"
  grep -Fq "$anchor" "$target" && infra mutation-not-applied
  return 0
}

test_pattern=''
expect_fail=''
case "$selector" in
  all)            test_pattern='^TestAUR525' ;;
  AC-001)         test_pattern='^TestAUR525RequestCarriesToolsAndReturnsCalls$' ;;
  AC-002)         test_pattern='^TestAUR525ConversationSerializesInOrderWithIDs$' ;;
  AC-003)         test_pattern='^TestAUR525MalformedArgumentsIsTypedError$' ;;
  AC-004)         test_pattern='^TestAUR525ToolSupportDetectableBeforeCall$' ;;
  AC-005)         test_pattern='^TestAUR525CompleteRequestUnchanged$' ;;
  AC-002-MUT-001) test_pattern='^TestAUR525ConversationSerializesInOrderWithIDs$'; expect_fail=1; apply_mutation ;;
esac

log="$run_dir/test.log"
set +e
(cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 300s -v ./internal/llm/... -run "$test_pattern") >"$log" 2>&1
status=$?
set -e
cat "$log" >&2

if [[ -n "$expect_fail" ]]; then
  grep -Eq -- '^--- FAIL: TestAUR525' "$log" || fail 'mutation-survived'
  (( status != 0 )) || fail 'mutation-survived-exit-zero'
  if grep -Eq 'build failed|cannot use|undefined:|syntax error' "$log"; then
    fail 'mutation-build-failure-not-behavioral'
  fi
  printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
  exit 0
fi

(( status == 0 )) || fail "go-test-exit:$status"
grep -Eq -- '^--- PASS: TestAUR525' "$log" || fail 'no-test-executed'

if [[ "$selector" == all ]]; then
  for name in RequestCarriesToolsAndReturnsCalls ConversationSerializesInOrderWithIDs MalformedArgumentsIsTypedError ToolSupportDetectableBeforeCall CompleteRequestUnchanged; do
    grep -q "^--- PASS: TestAUR525$name " "$log" || fail "missing-pass:$name"
  done
fi
printf '%s/%s/pass\n' "$card" "$selector"
