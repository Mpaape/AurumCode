#!/usr/bin/env bash
# AUR-571 acceptance (offline): the analysis_data API address comes from
# AURUMCODE_GITHUB_API_URL (same variable and validation as the PR client),
# default https://api.github.com; https only, http only for a loopback IP.
#
# Selectors:
#   all              AC-001..AC-003, then the mutation
#   AC-001           loopback fake via the variable: the review resolves the
#                    artifact from there; unset variable: the default
#   AC-002           http:// to a non-loopback host is a load error naming the
#                    variable (resolver and PR client)
#   AC-003           the tutorial runs without a demonstration CA and its
#                    recorded cases pass `run.sh --check`
#   AC-001-MUT-001   ignoring the variable (applied to a copy) turns AC-001 RED
# Unknown selectors exit 64; infrastructure failures 79; failures 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-571'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
demo="$repo_root/demo/tutoriais/dados-de-analise"
doc="$repo_root/docs/tutorials/dados-de-analise.md"

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a571.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP

seed_root() {
  command -v go >/dev/null 2>&1 || infra missing_go
  local input
  for input in go.mod go.sum cmd internal pkg; do
    [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
  done
  mkdir -p "$run_dir/cache" "$run_dir/gotmp"
  rm -rf "$run_dir/root"; mkdir -p "$run_dir/root"
  cp "$repo_root/go.mod" "$repo_root/go.sum" "$run_dir/root/"
  cp -R "$repo_root/cmd" "$repo_root/internal" "$repo_root/pkg" "$run_dir/root/"
  chmod -R u+w -- "$run_dir/root"
  export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
  export GOFLAGS='-mod=mod -p=1'
  export GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
  export GOMEMLIMIT=2GiB GOMAXPROCS=1
}

run_tests() {
  set +e
  (cd "$run_dir/root" && go vet ./internal/artifacts/... ./internal/config/... ./cmd/... && go test -mod=mod -p 1 -count=1 -timeout 600s -v ./internal/artifacts/... ./internal/config/... ./cmd/aurumcode/... -run "$1") >"$2" 2>&1
  local status=$?
  set -e
  cat "$2" >&2
  return $status
}

require_pass() {
  local log="$1"; shift
  local name
  for name in "$@"; do
    grep -q "^--- PASS: $name " "$log" || fail "missing-pass:$name"
  done
}

ac1_tests=(TestAUR571ReviewResolvesArtifactFromConfiguredAddress TestAUR571UnsetVariableUsesDefaultAddress TestAUR571GitHubAPIURLDefault)
ac2_tests=(TestAUR571InsecureAddressIsRefused TestAUR571PRClientRefusesInsecureAddress TestAUR571GitHubAPIURLValidation)

check_go() { # AC names...
  local log="$run_dir/$selector.log" pat
  pat="^($(IFS='|'; echo "$*"))\$"
  seed_root
  run_tests "$pat" "$log" || fail "go-test-exit"
  require_pass "$log" "$@"
}

ac003() {
  [[ -f "$demo/run.sh" && -f "$doc" ]] || infra missing-tutorial
  [[ ! -e "$demo/servidor-falso.py" ]] || fail 'AC-003/servidor-https-falso-ainda-existe'
  if grep -rIlE 'SSL_CERT_FILE|ca\.pem|leaf\.pem|add-host|CA de demonstra' "$demo/run.sh" "$demo/dentro.sh" "$demo"/*.py "$doc" 2>/dev/null | grep -q .; then
    fail 'AC-003/ca-de-demonstracao-ou-add-host-ainda-citados'
  fi
  grep -q 'AURUMCODE_GITHUB_API_URL' "$demo/run.sh" || fail 'AC-003/run-sh-nao-usa-a-variavel'
  grep -q 'AURUMCODE_GITHUB_API_URL' "$doc" || fail 'AC-003/doc-nao-cita-a-variavel'
  bash "$demo/run.sh" --check >"$run_dir/check.log" 2>&1 || { cat "$run_dir/check.log" >&2; fail 'AC-003/check-diverge'; }
}

mutation_red() {
  seed_root
  local target="$run_dir/root/internal/artifacts/client.go"
  local anchor='base, err := config.GitHubAPIURL(os.Getenv)'
  [[ "$(grep -Fc "$anchor" "$target")" == "1" ]] || infra mutation-anchor-not-unique
  sed -i "s|$anchor|base, err := config.DefaultGitHubAPIURL, error(nil) // MUT-001: variable ignored|" "$target"
  grep -Fq 'MUT-001: variable ignored' "$target" || infra mutation-not-applied
  local log="$run_dir/mutation.log"
  run_tests '^TestAUR571ReviewResolvesArtifactFromConfiguredAddress$' "$log" || true
  if grep -Eq 'build failed|cannot use|undefined:|syntax error|declared and not used' "$log"; then
    fail 'mutation-build-failure-not-behavioral'
  fi
  grep -Eq -- '^--- FAIL: TestAUR571ReviewResolvesArtifactFromConfiguredAddress' "$log" || fail 'mutation-survived'
}

case "$selector" in
  AC-001) check_go "${ac1_tests[@]}"; echo "$card/AC-001/pass" ;;
  AC-002) check_go "${ac2_tests[@]}"; echo "$card/AC-002/pass" ;;
  AC-003) ac003; echo "$card/AC-003/pass" ;;
  AC-001-MUT-001) mutation_red; echo "$card/AC-001-MUT-001/pass (mutation produced RED)" ;;
  all)
    check_go "${ac1_tests[@]}" "${ac2_tests[@]}"
    ac003
    mutation_red
    echo "$card/all/pass"
    ;;
esac
