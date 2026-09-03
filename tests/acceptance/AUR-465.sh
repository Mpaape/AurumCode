#!/usr/bin/env bash
# AUR-465 AC-001: the generated index no longer teaches the reader to pin the
# AurumCode Action to a mutable branch, no _config.yml logo declaration
# outlives the asset it names, and every internal link on the index points
# somewhere the generator actually produced.
#
# SCOPE
#   This runs inside `bootstrap-readonly-v1`: no network. It never invokes
#   cmd/regenerate-docs end to end (internal/documentation/site internal/documentation/review, the package
#   that writes _config.yml and the deterministic page listing, is a
#   read_path here, not owned by this card). What this card owns and proves
#   is the deterministic guardrail internal/documentation/welcome internal/documentation/review/sanitize.go
#   adds around the LLM-generated welcome content that becomes the top of
#   the published index (internal/documentation/site internal/documentation/review/scaffold.go's own
#   comment: "prose already in the file ... is preserved above the generated
#   listing"), wired into Generator.Generate() -- the entry point
#   cmd/regenerate-docs actually calls.
#
# WHY THIS SCRIPT STAGES A SCRATCH COPY (same technique as AUR-422/424/428/440/445)
#   The sandbox rootfs is read-only; only this card's paths/read_paths are
#   materialized and /tmp is the only writable space. `go test` needs a
#   writable build cache, so the Go lanes copy their exact inputs into a
#   private module under /tmp and bridge the exported selector functions into
#   `_test.go` files there.
#
# MUT-001 (AC-001: mutable Action ref)
#   Neutering SanitizeActionRef so it stops rewriting a mutable ref must make
#   both the unit lane and the e2e lane fail, each reporting
#   AUR-465/AC-001/MUT-001. Restoring the guard reproduces the exact GREEN.
#
# MUT-002 (AC-002: declared-but-missing asset)
#   A _config.yml fixture that declares a logo the published tree never
#   received must make the e2e lane fail, reporting AUR-465/AC-001/MUT-002,
#   independent of any code change -- only the fixture data differs.
set -euo pipefail
export LC_ALL=C
readonly card=AUR-465 scenario=AC-001
selector="${1:-AC-001}"

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }

case "$selector" in
  AC-001|TestAUR465|IntegrationAUR465|E2EAUR465|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

inputs=(
  go.mod go.sum
  internal/documentation/welcome internal/documentation/review/generator.go
  internal/documentation/welcome internal/documentation/review/sanitize.go
  internal/documentation/welcome internal/documentation/review/templates/welcome-page.md
  tests/unit/AUR-465.go
  tests/integration/AUR-465.go
  tests/e2e/AUR-465.sh
  docs/specs/AUR-465.md
)
for p in "${inputs[@]}"; do
  [[ -f "$repo_root/$p" ]] || fail "entrypoint_missing:$p"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a465.XXXXXX")" || infra mktemp
# Staged inputs are copied read-only, so rm alone cannot remove them and the
# cleanup error would overwrite a green nominal result. Make the tree writable
# first, and never let cleanup decide the exit code (AUR-318, AUR-426).
cleanup() {
  chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true
  rm -rf -- "$run_dir" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM HUP
root="$run_dir/root"
mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$run_dir/home"

stage() {
  local dest="$1"
  local p
  for p in "${inputs[@]}"; do
    mkdir -p "$dest/$(dirname "$p")"
    cp "$repo_root/$p" "$dest/$p"
  done
  # internal/llm: read_path-adjacent dependency of generator.go (the
  # Orchestrator/Provider types the Integration lane's mock plugs into).
  # Staged as a whole directory because it is a multi-file package, not one
  # named path.
  mkdir -p "$dest/internal/llm"
  cp "$repo_root"/internal/llm/*.go "$dest/internal/llm/" 2>/dev/null || true
  mkdir -p "$dest/internal/llm/provider"
  cp -r "$repo_root"/internal/llm/provider/. "$dest/internal/llm/provider/" 2>/dev/null || true
  mkdir -p "$dest/internal/llm/cost"
  cp -r "$repo_root"/internal/llm/cost/. "$dest/internal/llm/cost/" 2>/dev/null || true
}

stage "$root"

printf 'package unit\nimport "testing"\nfunc TestAUR465Bridge(t *testing.T){ TestAUR465(t) }\n' \
  >"$root/tests/unit/aur465_bridge_test.go"
printf 'package integration\nimport "testing"\nfunc TestAUR465Bridge(t *testing.T){ IntegrationAUR465(t) }\n' \
  >"$root/tests/integration/aur465_bridge_test.go"

go_lane() {
  # One offline, bounded invocation compiles and executes the requested
  # package's real assertions against the staged artifacts. root_override
  # lets MUT-001 point the same lane at a different, mutated staging area.
  local pkg="$1" root_override="${2:-$root}" out rc
  set +e
  out="$( ulimit -v 8388608
    cd "$root_override" && HOME="$run_dir/home" GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod \
    GOTOOLCHAIN=local GOMAXPROCS=1 GOMEMLIMIT=2GiB \
    GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" \
    go test -timeout 300s -v -vet=off -p 1 -count=1 "$pkg" -run '^TestAUR465Bridge$' 2>&1)"
  rc=$?
  set -e
  aur465_last_output="$out"
  printf '%s\n' "$out"
  if (( rc != 0 )); then
    detail="$(grep -om1 "$card/$scenario/[A-Za-z0-9/_:-]*" <<<"$out" | head -n1 || true)"
    [[ -z "$detail" ]] || printf '%s\n' "$detail" >&2
    return "$rc"
  fi
  local ok_count
  ok_count="$(grep -c '^ok ' <<<"$out" || true)"
  (( ok_count == 1 )) || { fail "zero-tests:$pkg"; }
  ! grep -Fq '[no test files]' <<<"$out" || { fail "no-test-files:$pkg"; }
  grep -Fq -- '--- PASS: TestAUR465Bridge' <<<"$out" || { fail "selector-did-not-run:$pkg"; }
  return 0
}

e2e_case() {
  local sel="${1:-E2EAUR465}" out rc
  local e2e_selector="E2EAUR465"
  [[ "$sel" == E2EAUR465 || "$sel" == MUT-001 || "$sel" == MUT-002 ]] && e2e_selector="$sel"
  set +e
  out="$(bash "$repo_root/tests/e2e/AUR-465.sh" "$e2e_selector" 2>&1)"
  rc=$?
  set -e
  aur465_last_output="$out"
  printf '%s\n' "$out"
  return "$rc"
}

spec_case() {
  [[ -s "$repo_root/docs/specs/AUR-465.md" ]] || fail 'behavior-missing:docs/specs/AUR-465.md absent or empty'
  grep -Fq 'SanitizeActionRef' "$repo_root/docs/specs/AUR-465.md" \
    || fail 'behavior-missing:spec never names the SanitizeActionRef guardrail'
  grep -Fq 'AC-002' "$repo_root/docs/specs/AUR-465.md" \
    || fail 'behavior-missing:spec never records AC-002'
}

case "$selector" in
  AC-001)
    go_lane ./tests/unit "$root" || fail 'selector-exit:unit'
    go_lane ./tests/integration "$root" || fail 'selector-exit:integration'
    rc=0; e2e_case E2EAUR465 || rc=$?
    if (( rc != 0 )); then
      (( rc == 69 || rc == 64 )) && exit "$rc"
      fail "selector-exit:e2e:$rc"
    fi
    spec_case
    printf '{"card":"%s","scenario":"%s","result":"pass","checks":["AC-001","AC-002","AC-003"]}\n' "$card" "$scenario"
    ;;
  TestAUR465) go_lane ./tests/unit "$root" ;;
  IntegrationAUR465) go_lane ./tests/integration "$root" ;;
  E2EAUR465) e2e_case E2EAUR465 ;;
  MUT-001)
    mut_root="$run_dir/mut-root"
    stage "$mut_root"
    # Neuter the guardrail in the SCRATCH copy only: the regex substitution
    # becomes an identity function, as if a future edit reintroduced @main
    # capability by breaking the guard.
    sed -i.bak 's/^func SanitizeActionRef(content string) (string, bool) {$/func SanitizeActionRef(content string) (string, bool) { return content, false; _ = actionRefPattern; _ = pinnedTagPattern; _ = fullSHAPattern/' \
      "$mut_root/internal/documentation/welcome internal/documentation/review/sanitize.go"
    rm -f "$mut_root/internal/documentation/welcome internal/documentation/review/sanitize.go.bak"
    printf 'package unit\nimport "testing"\nfunc TestAUR465Bridge(t *testing.T){ TestAUR465(t) }\n' \
      >"$mut_root/tests/unit/aur465_bridge_test.go"

    unit_rc=0; go_lane ./tests/unit "$mut_root" || unit_rc=$?
    (( unit_rc != 0 )) || fail 'MUT-001:unit lane passed on a neutered guard, expected failure'
    grep -Fq '@main' <<<"$aur465_last_output" \
      || fail 'MUT-001:unit lane failed but not on the @main assertion'

    # tests/e2e/AUR-465.sh's own MUT-001 selector is self-contained: it
    # stages ITS OWN mutated copy, runs the driver against it, and returns 0
    # only when the neutered guard was correctly caught (mirroring this
    # script's own AC-001/TestAUR465/etc split). A nonzero exit here means
    # detection failed, not that it succeeded.
    e2e_rc=0; e2e_case MUT-001 || e2e_rc=$?
    (( e2e_rc == 0 )) || fail "MUT-001:e2e lane's own mutation-detection run failed:$e2e_rc"
    grep -Fq "$card/$scenario/MUT-001" <<<"$aur465_last_output" \
      || fail 'MUT-001:e2e lane never reported the MUT-001 label'
    grep -Fq '"result":"detected"' <<<"$aur465_last_output" \
      || fail 'MUT-001:e2e lane never confirmed detection'

    printf '%s/%s/MUT-001 confirmed: unit_rc=%d e2e_rc=%d, tracked sanitize.go untouched\n' "$card" "$scenario" "$unit_rc" "$e2e_rc"
    printf '{"card":"%s","scenario":"%s","mutation":"MUT-001","result":"detected"}\n' "$card" "$scenario"
    ;;
  MUT-002)
    # Same self-contained shape as MUT-001 above: tests/e2e/AUR-465.sh's own
    # MUT-002 selector runs its driver against the declared-but-missing-logo
    # fixture and returns 0 only when AC-002 correctly refused it.
    e2e_rc=0; e2e_case MUT-002 || e2e_rc=$?
    (( e2e_rc == 0 )) || fail "MUT-002:e2e lane's own mutation-detection run failed:$e2e_rc"
    grep -Fq "$card/$scenario/MUT-002" <<<"$aur465_last_output" \
      || fail 'MUT-002:e2e lane never reported the MUT-002 label'
    grep -Fq '"result":"detected"' <<<"$aur465_last_output" \
      || fail 'MUT-002:e2e lane never confirmed detection'

    printf '%s/%s/MUT-002 confirmed: e2e_rc=%d, tracked fixtures untouched\n' "$card" "$scenario" "$e2e_rc"
    printf '{"card":"%s","scenario":"%s","mutation":"MUT-002","result":"detected"}\n' "$card" "$scenario"
    ;;
esac
