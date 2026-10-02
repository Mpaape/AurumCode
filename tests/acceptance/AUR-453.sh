#!/usr/bin/env bash
# AUR-453 acceptance: summary and suggestions reach the terminal and the PR.
#
# Runs the real Go behavior tests for the --base terminal path and the --pr
# formal-review path inside the sealed, offline Go profile. AC-001-MUT-001
# removes the terminal suggestion render call from a writable copy of the
# candidate and requires the AC-001 terminal assertion to fail (RED); the
# versioned source is never touched.
set -euo pipefail
selector="${1:-all}"
case "$selector" in
  all) test_pattern='^TestAUR453' ;;
  TestAUR453) test_pattern='^TestAUR453' ;;
  IntegrationAUR453) test_pattern='^TestAUR453Formal' ;;
  E2EAUR453) test_pattern='^TestAUR453Terminal' ;;
  AC-001-MUT-001) test_pattern='^TestAUR453TerminalSummaryAndSuggestion$' ;;
  *) printf 'AUR-453/unknown-selector\n' >&2; exit 64 ;;
esac

command -v go >/dev/null || exit 79
repo_root="$(cd -- "${BASH_SOURCE[0]%/*}/../.." && pwd -P)"
scratch="$(mktemp -d)"
trap 'chmod -R u+w -- "$scratch" 2>/dev/null || true; rm -rf -- "$scratch"' EXIT
mkdir -p "$scratch/root" "$scratch/cache" "$scratch/gotmp"
for source in go.mod go.sum cmd internal pkg; do cp -R "$repo_root/$source" "$scratch/root/"; done
chmod -R u+w "$scratch/root"
export GOCACHE="${GOCACHE:-$scratch/cache}" GOTMPDIR="$scratch/gotmp"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOMEMLIMIT=2GiB GOMAXPROCS=2

if [[ "$selector" == 'AC-001-MUT-001' ]]; then
  # MUT-001: suppress the eligible suggestion before it can reach the terminal.
  # The call is replaced by an empty literal so the candidate still COMPILES --
  # a build failure is never valid red evidence -- while the promised behavior
  # (a valid suggestion reaching stdout) is gone. The call identifier is split
  # so this acceptance file cannot match its own edit.
  marker="render""Suggestions(result, diff, reviewLanguage)"
  sed -i "s|${marker}|\"\"|" "$scratch/root/cmd/aurumcode/main.go"
  if grep -Fq "$marker" "$scratch/root/cmd/aurumcode/main.go"; then
    printf 'AUR-453/AC-001-MUT-001/mutation-not-applied\n' >&2
    exit 79
  fi
fi

rc=0
(cd "$scratch/root" && go test ./cmd/aurumcode -run "$test_pattern" -count=1 -timeout=180s -v) >"$scratch/test.log" 2>&1 || rc=$?
cat "$scratch/test.log"
if [[ "$selector" == 'AC-001-MUT-001' ]]; then
  [[ "$rc" == 1 ]] && grep -q '^--- FAIL: TestAUR453' "$scratch/test.log" || exit 1
else
  [[ "$rc" == 0 ]] || exit "$rc"
  grep -q '^--- PASS: TestAUR453' "$scratch/test.log" || exit 1
  if [[ "$selector" == all ]]; then
    for name in TerminalSummaryAndSuggestion TerminalIneligibleSuggestion FormalReviewCarriesSuggestion RedactionAndConsumerChoice NoSuggestionIsUnchanged; do
      grep -q "^--- PASS: TestAUR453$name " "$scratch/test.log" || exit 1
    done
  fi
fi
printf 'AUR-453/%s/pass\n' "$selector"
