#!/usr/bin/env bash
set -Eeuo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-004'
readonly scenario='AC-001'
readonly max_deadline=30
selector="${1:-AC-001}"

fail() {
  printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2
  exit 1
}

infra() {
  printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2
  exit 69
}

case "$selector" in
  AC-001|TestAUR004|IntegrationAUR004) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

script_dir="${0%/*}"
[[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." >/dev/null 2>&1 && pwd -P)" || infra repo-root-unresolved
cd -- "$repo_root" || infra repo-root-unreachable

for tool in go grep mktemp rm sed sha256sum timeout wc; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing-tool:$tool"
done

for path in \
  go.mod \
  go.sum \
  internal/governance/dag/dag.go \
  tests/unit/AUR-004.go \
  tests/integration/AUR-004.go \
  tests/specs/AUR-004/cases.yaml \
  docs/specs/AUR-004.md; do
  [[ -f "$path" && ! -L "$path" && -r "$path" ]] || fail "declared-input-missing:$path"
done

[[ "$(go env GOMOD)" == "$repo_root/go.mod" ]] || infra canonical-module-not-selected
bytes="$(wc -c < tests/specs/AUR-004/cases.yaml)" || infra fixture-size-unavailable
(( bytes <= 4 * 1024 * 1024 )) || fail fixture-limit-exceeded

scratch="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a004.XXXXXX")" || infra mktemp-failed
trap 'rm -rf -- "$scratch"' EXIT INT TERM HUP
mkdir -p "$scratch/go-cache" "$scratch/go-tmp" || infra scratch-unavailable

build_selector() {
  local source="$1" binary="$2" function_name="$3"
  grep -Eq "^func[[:space:]]+${function_name}[[:space:]]*\\(" "$source" || fail "selector-not-defined:$source::$function_name"
  set +e
  timeout "$max_deadline" env GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOCACHE="$scratch/go-cache" GOTMPDIR="$scratch/go-tmp" \
    go build -mod=readonly -o "$binary" "$source" >"$scratch/build.out" 2>"$scratch/build.err"
  local build_exit=$?
  set -e
  if (( build_exit != 0 )); then
    sed -n '1,80p' "$scratch/build.err" >&2
    infra "go-build-failed:$function_name:$build_exit"
  fi
}

run_selector() {
  local binary="$1"
  local function_name="$2"
  local output="$scratch/$function_name.out"
  set +e
  timeout "$max_deadline" "$binary" "$function_name" >"$output" 2>&1
  local selector_exit=$?
  set -e
  if (( selector_exit != 0 )); then
    sed -n '1,100p' "$output" >&2
    grep -Fq 'AUR-004/AC-001/' "$output" || infra "selector-crashed:$function_name:$selector_exit"
    fail "selector-failed:$function_name:$selector_exit"
  fi
  grep -Eq '"assertions":[1-9][0-9]*' "$output" || fail "zero-assertions:$function_name"
  grep -Fq '"result":"pass"' "$output" || fail "selector-no-pass:$function_name"
  grep -Fq '[no test files]' "$output" && fail "no-test-files:$function_name"
  sed -n '1,20p' "$output"
}

build_selector tests/unit/AUR-004.go "$scratch/unit" TestAUR004
build_selector tests/integration/AUR-004.go "$scratch/integration" IntegrationAUR004

case "$selector" in
  AC-001)
    run_selector "$scratch/unit" TestAUR004
    run_selector "$scratch/integration" IntegrationAUR004
    ;;
  TestAUR004)
    run_selector "$scratch/unit" TestAUR004
    ;;
  IntegrationAUR004)
    run_selector "$scratch/integration" IntegrationAUR004
    ;;
esac

module_digest="sha256:$(sha256sum go.mod | sed 's/[[:space:]].*$//')" || infra module-digest-failed
sum_digest="sha256:$(sha256sum go.sum | sed 's/[[:space:]].*$//')" || infra sum-digest-failed
printf '{"card":"%s","scenario":"%s","selector":"%s","result":"pass","effects":0,"module_digest":"%s","sum_digest":"%s"}\n' \
  "$card" "$scenario" "$selector" "$module_digest" "$sum_digest"
