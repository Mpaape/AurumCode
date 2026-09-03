#!/usr/bin/env bash
# AUR-484 AC-001/AC-002/AC-003: the published landing page stops being ~80%
# raw enumeration of generated pages and gains, above that enumeration, a
# copy-pasteable no-credential command, one example per product feature, and
# a declared, verifiable limitation section. The enumeration itself moves to
# its own page (reference.md), referenced rather than embedded.
#
# SCOPE
#   Runs inside `bootstrap-readonly-v1`: no network, no git, no python3. This
#   card owns internal/documentation/site (the deterministic scaffold that
#   writes index.md/_config.yml/reference.md with no LLM involved) and
#   internal/documentation/welcome (the separate, optional model-enrichment
#   path, untouched by this card's own content). It does not own
#   internal/pipeline (AUR-483, in parallel) and does not edit
#   .aurumcode/index.md (outside this card's paths/read_paths); the fix lives
#   in the generator that writes that file, not in the checked-in copy.
#
# WHY THIS SCRIPT STAGES A SCRATCH COPY (same technique as AUR-422/424/428/440/465)
#   The sandbox rootfs is read-only; only this card's paths/read_paths are
#   materialized and /tmp is the only writable space. `go test`/`go run` need
#   a writable build cache, so the Go lanes copy their exact inputs into a
#   private module under /tmp.
#
# MUT-001 (AC-001: the copyable command is removed from the template)
#   Both the unit lane and the e2e lane must fail, each reporting
#   AUR-484/AC-001/MUT-001. Restoring the guard reproduces the exact GREEN.
#
# MUT-002 (AC-002: the full enumeration goes back into the index body)
#   The e2e lane must fail, reporting AUR-484/AC-001/MUT-002, from a code
#   change alone (writeIndex calling renderPageBlock instead of
#   renderPageSummaryBlock).
set -euo pipefail
export LC_ALL=C
readonly card=AUR-484 scenario=AC-001
selector="${1:-AC-001}"

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }

case "$selector" in
  AC-001|TestAUR484|IntegrationAUR484|E2EAUR484|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

inputs=(
  go.mod go.sum
  internal/documentation/site/scaffold.go
  internal/documentation/site/types.go
  internal/documentation/site/runner.go
  tests/unit/AUR-484.go
  tests/integration/AUR-484.go
  tests/e2e/AUR-484.sh
  docs/specs/AUR-484.md
)
for p in "${inputs[@]}"; do
  [[ -f "$repo_root/$p" ]] || fail "entrypoint_missing:$p"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a484.XXXXXX")" || infra mktemp
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
}

stage "$root"

printf 'package unit\nimport "testing"\nfunc TestAUR484Bridge(t *testing.T){ TestAUR484(t) }\n' \
  >"$root/tests/unit/aur484_bridge_test.go"
printf 'package integration\nimport "testing"\nfunc TestAUR484Bridge(t *testing.T){ IntegrationAUR484(t) }\n' \
  >"$root/tests/integration/aur484_bridge_test.go"

go_lane() {
  local pkg="$1" root_override="${2:-$root}" out rc
  set +e
  out="$( ulimit -v 8388608
    cd "$root_override" && HOME="$run_dir/home" GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod \
    GOTOOLCHAIN=local GOMAXPROCS=1 GOMEMLIMIT=2GiB \
    GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" \
    go test -timeout 300s -v -vet=off -p 1 -count=1 "$pkg" -run '^TestAUR484Bridge$' 2>&1)"
  rc=$?
  set -e
  aur484_last_output="$out"
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
  grep -Fq -- '--- PASS: TestAUR484Bridge' <<<"$out" || { fail "selector-did-not-run:$pkg"; }
  return 0
}

e2e_case() {
  local sel="${1:-E2EAUR484}" out rc
  set +e
  out="$(bash "$repo_root/tests/e2e/AUR-484.sh" "$sel" 2>&1)"
  rc=$?
  set -e
  aur484_last_output="$out"
  printf '%s\n' "$out"
  return "$rc"
}

spec_case() {
  [[ -s "$repo_root/docs/specs/AUR-484.md" ]] || fail 'behavior-missing:docs/specs/AUR-484.md absent or empty'
  grep -Fq 'AC-002' "$repo_root/docs/specs/AUR-484.md" \
    || fail 'behavior-missing:spec never records AC-002'
  grep -Fq '4 das 8' "$repo_root/docs/specs/AUR-484.md" \
    || fail 'behavior-missing:spec never records the 4-of-8 security rule limitation'
}

case "$selector" in
  AC-001)
    go_lane ./tests/unit "$root" || fail 'selector-exit:unit'
    go_lane ./tests/integration "$root" || fail 'selector-exit:integration'
    rc=0; e2e_case E2EAUR484 || rc=$?
    if (( rc != 0 )); then
      (( rc == 69 || rc == 64 )) && exit "$rc"
      fail "selector-exit:e2e:$rc"
    fi
    spec_case
    printf '{"card":"%s","scenario":"%s","result":"pass","checks":["AC-001","AC-002","AC-003"]}\n' "$card" "$scenario"
    ;;
  TestAUR484) go_lane ./tests/unit "$root" ;;
  IntegrationAUR484) go_lane ./tests/integration "$root" ;;
  E2EAUR484) e2e_case E2EAUR484 ;;
  MUT-001)
    mut_root="$run_dir/mut-root"
    stage "$mut_root"
    # Neuter the guardrail in the SCRATCH copy only: the no-credential
    # command is dropped from the quickstart block, as if a future edit
    # regressed the landing page back toward empty prose above the listing.
    sed -i.bak "s#./aurumcode review --base HEAD~1 --seguranca#REMOVED#" \
      "$mut_root/internal/documentation/site/scaffold.go"
    rm -f "$mut_root/internal/documentation/site/scaffold.go.bak"
    printf 'package unit\nimport "testing"\nfunc TestAUR484Bridge(t *testing.T){ TestAUR484(t) }\n' \
      >"$mut_root/tests/unit/aur484_bridge_test.go"

    unit_rc=0; go_lane ./tests/unit "$mut_root" || unit_rc=$?
    (( unit_rc != 0 )) || fail 'MUT-001:unit lane passed on a neutered command, expected failure'

    e2e_rc=0; e2e_case MUT-001 || e2e_rc=$?
    (( e2e_rc == 0 )) || fail "MUT-001:e2e lane's own mutation-detection run failed:$e2e_rc"
    grep -Fq "$card/$scenario/MUT-001" <<<"$aur484_last_output" \
      || fail 'MUT-001:e2e lane never reported the MUT-001 label'
    grep -Fq '"result":"detected"' <<<"$aur484_last_output" \
      || fail 'MUT-001:e2e lane never confirmed detection'

    printf '%s/%s/MUT-001 confirmed: unit_rc=%d e2e_rc=%d, tracked scaffold.go untouched\n' "$card" "$scenario" "$unit_rc" "$e2e_rc"
    printf '{"card":"%s","scenario":"%s","mutation":"MUT-001","result":"detected"}\n' "$card" "$scenario"
    ;;
  MUT-002)
    # Self-contained, same shape as MUT-001: tests/e2e/AUR-484.sh's own
    # MUT-002 selector stages its own mutated copy (the full listing put
    # back into index.md) and proves detection.
    e2e_rc=0; e2e_case MUT-002 || e2e_rc=$?
    (( e2e_rc == 0 )) || fail "MUT-002:e2e lane's own mutation-detection run failed:$e2e_rc"
    grep -Fq "$card/$scenario/MUT-002" <<<"$aur484_last_output" \
      || fail 'MUT-002:e2e lane never reported the MUT-002 label'
    grep -Fq '"result":"detected"' <<<"$aur484_last_output" \
      || fail 'MUT-002:e2e lane never confirmed detection'

    printf '%s/%s/MUT-002 confirmed: e2e_rc=%d, tracked scaffold.go untouched\n' "$card" "$scenario" "$e2e_rc"
    printf '{"card":"%s","scenario":"%s","mutation":"MUT-002","result":"detected"}\n' "$card" "$scenario"
    ;;
esac
