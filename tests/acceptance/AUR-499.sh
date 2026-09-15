#!/usr/bin/env bash
# AUR-499 acceptance: the review reads PR/local commit metadata, publishes the
# AUR-498 suggested version and changelog entry, exposes them as Action
# outputs, and treats commit/PR text as untrusted redacted data.
#
# Selectors: all | AC-001 | AC-002 | AC-003 | AC-004 | MUT-001 | MUT-002 | MUT-003
# Exit: 0 green; 1 behavioural failure; 64 unknown selector; 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C
umask 077
readonly card='AUR-499'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|MUT-001|MUT-002|MUT-003) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac
if [[ "$selector" == all ]]; then
  for scenario in AC-001 AC-002 AC-003 AC-004; do
    bash "${BASH_SOURCE[0]}" "$scenario"
  done
  exit 0
fi

case "$selector" in
  AC-001) test_pattern='^TestAUR499CommitSources$' ;;
  AC-002) test_pattern='^TestAUR499PublishedBody$' ;;
  AC-003) test_pattern='^TestAUR499ActionOutput$' ;;
  AC-004) test_pattern='^TestAUR499Redaction$' ;;
  MUT-*)  test_pattern='^TestAUR499' ;;
esac

command -v go >/dev/null 2>&1 || { printf '%s/%s/infrastructure/missing_go\n' "$card" "$selector" >&2; exit 79; }
script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || { printf '%s/%s/infrastructure/repo_root\n' "$card" "$selector" >&2; exit 79; }

scratch="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a499.XXXXXX")" || { printf '%s/%s/infrastructure/mktemp\n' "$card" "$selector" >&2; exit 79; }
cleanup() { chmod -R u+w -- "$scratch" >/dev/null 2>&1 || true; rm -rf -- "$scratch" >/dev/null 2>&1 || true; }
trap cleanup EXIT INT TERM HUP
mkdir -p "$scratch/root" "$scratch/cache" "$scratch/gotmp"
for source in go.mod go.sum cmd internal pkg action.yml scripts; do
  cp -R "$repo_root/$source" "$scratch/root/"
done
chmod -R u+w -- "$scratch/root"
export GOCACHE="${GOCACHE:-$scratch/cache}" GOTMPDIR="$scratch/gotmp"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOMEMLIMIT=2GiB GOMAXPROCS=2

case "$selector" in
  MUT-001)
    # Omit the section from the published body: the append becomes a no-op.
    sed -i 's@return strings.TrimRight(body, "\\n") + "\\n\\n" + strings.TrimLeft(section, "\\n")@return body@' "$scratch/root/cmd/aurumcode/passes.go"
    grep -q 'return body$' "$scratch/root/cmd/aurumcode/passes.go" || { printf '%s/%s/infrastructure/mutation-anchor-missing:MUT-001\n' "$card" "$selector" >&2; exit 79; }
    ;;
  MUT-002)
    # Drop the Action output declaration.
    sed -i '/^outputs:/,/^$/d' "$scratch/root/action.yml"
    if grep -q '^outputs:' "$scratch/root/action.yml" || grep -q '^  version:' "$scratch/root/action.yml"; then
      printf '%s/%s/infrastructure/mutation-anchor-missing:MUT-002\n' "$card" "$selector" >&2
      exit 79
    fi
    ;;
  MUT-003)
    # Echo raw commit text without redaction in the sink.
    sed -i 's@Subject: filter.Redact(c.Subject),@Subject: c.Subject,@' "$scratch/root/cmd/aurumcode/passes.go"
    grep -q 'Subject: c.Subject,' "$scratch/root/cmd/aurumcode/passes.go" || { printf '%s/%s/infrastructure/mutation-anchor-missing:MUT-003\n' "$card" "$selector" >&2; exit 79; }
    ;;
esac

rc=0
(cd "$scratch/root" && go test ./cmd/aurumcode -run "$test_pattern" -count=1 -timeout=180s -v) >"$scratch/test.log" 2>&1 || rc=$?
cat "$scratch/test.log" >&2

if [[ "$selector" == MUT-* ]]; then
  [[ "$rc" == 1 ]] && grep -q '^--- FAIL: TestAUR499' "$scratch/test.log" || { printf '%s/%s/mutation-not-detected\n' "$card" "$selector" >&2; exit 1; }
else
  [[ "$rc" == 0 ]] || exit "$rc"
  grep -q '^--- PASS: TestAUR499' "$scratch/test.log" || { printf '%s/%s/selector-not-executed\n' "$card" "$selector" >&2; exit 1; }
  if [[ "$selector" == all ]]; then
    for name in CommitSources PublishedBody ActionOutput Redaction; do
      grep -q "^--- PASS: TestAUR499$name " "$scratch/test.log" || { printf '%s/%s/missing-pass:%s\n' "$card" "$selector" "$name" >&2; exit 1; }
    done
  fi
fi
printf '%s/%s/pass\n' "$card" "$selector"
