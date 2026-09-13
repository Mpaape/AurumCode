#!/usr/bin/env bash
set -euo pipefail
selector="${1:-all}"
case "$selector" in
  all) test_pattern='^TestAUR490' ;;
  AC-001|MUT-001) test_pattern='^TestAUR490LocalAnalysis$' ;;
  AC-002|MUT-002) test_pattern='^TestAUR490LocalRender$' ;;
  AC-003) test_pattern='^TestAUR490LocalMemory$' ;;
  AC-004) test_pattern='^TestAUR490SharedPasses$' ;;
  *) exit 64 ;;
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
case "$selector" in
  MUT-001) sed -i '/^[[:space:]]*mergeStaticAnalysis(diff, result)$/d' "$scratch/root/cmd/aurumcode/main.go" ;;
  MUT-002) sed -i '/fmt.Fprint(stdout, renderLocalReport(result, diff, reviewLanguage))/d' "$scratch/root/cmd/aurumcode/main.go" ;;
esac
rc=0
(cd "$scratch/root" && go test ./cmd/aurumcode -run "$test_pattern" -count=1 -timeout=120s -v) >"$scratch/test.log" 2>&1 || rc=$?
cat "$scratch/test.log"
if [[ "$selector" == MUT-* ]]; then
  [[ "$rc" == 1 ]] && grep -q '^--- FAIL: TestAUR490' "$scratch/test.log" || exit 1
else
  [[ "$rc" == 0 ]] || exit "$rc"
  grep -q '^--- PASS: TestAUR490' "$scratch/test.log" || exit 1
  if [[ "$selector" == all ]]; then
    for name in LocalAnalysis LocalRender LocalMemory SharedPasses; do
      grep -q "^--- PASS: TestAUR490$name " "$scratch/test.log" || exit 1
    done
  fi
fi
printf 'AUR-490/%s/pass\n' "$selector"
