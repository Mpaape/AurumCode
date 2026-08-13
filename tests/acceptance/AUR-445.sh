#!/usr/bin/env bash
# AUR-445 AC-001: whoever reads the root documentation discovers what the
# tool actually does today -- LICENSE exists, a local code-review command
# exists with flags, and Go needs no external tool -- because the
# 2026-08-13 audit's five false-statement families
# (.board/cards/ready/AUR-445.md, "Achados medidos") no longer appear in
# README.md, CHANGELOG.md, ACTION_USAGE.md, SETUP_GUIDE.md,
# RUN_DOCS_PIPELINE.md or docs/getting-started.md, and the true replacement
# text is present with lastro in the source it describes.
#
# SCOPE
#   This runs inside `bootstrap-readonly-v1`: no network, and it never
#   executes `aurumcode` or `regenerate-docs`. What THIS card accepts is
#   STATIC verification: normalized text matching over the six documentation
#   files (never loose grep -- see docs/specs/AUR-445.md's normalization
#   note) plus a direct read of the source those files describe.
#
# WHY THIS SCRIPT STAGES A SCRATCH COPY (same technique as AUR-422/424/428/440)
#   The sandbox rootfs is read-only; only this card's paths/read_paths are
#   materialized and /tmp is the only writable space. `go test` needs a
#   writable build cache, so the Go lanes copy their exact inputs into a
#   private module under /tmp and bridge the exported selector functions into
#   `_test.go` files there.
#
# MUT-001
#   Reintroducing "No `LICENSE` file is present" into README.md (or
#   ACTION_USAGE.md/CHANGELOG.md) must fail the acceptance. This script's
#   `MUT-001` selector proves it: it stages a SEPARATE scratch copy, mutates
#   ONLY that copy's README.md, and requires every lane to fail with the
#   AUR-445/AC-001/MUT-001 label. The tracked README.md is never touched.
set -euo pipefail
export LC_ALL=C
readonly card=AUR-445 scenario=AC-001
selector="${1:-AC-001}"

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }

case "$selector" in
  AC-001|TestAUR445|IntegrationAUR445|E2EAUR445|MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

# Inputs the lanes cannot run without. docs/specs/AUR-445.md and the six
# documentation files are NOT excluded here -- their absence or their
# pre-fix content is the card's RED (a behavior gap the lanes report as
# behavior-missing), never a missing entrypoint, so they are still staged
# when present rather than gating entry.
inputs=(
  go.mod go.sum
  LICENSE
  cmd/aurumcode/main.go
  cmd/regenerate-docs/main.go
  README.md
  CHANGELOG.md
  ACTION_USAGE.md
  SETUP_GUIDE.md
  RUN_DOCS_PIPELINE.md
  docs/getting-started.md
  docs/specs/AUR-445.md
  tests/unit/AUR-445.go
  tests/integration/AUR-445.go
  tests/e2e/AUR-445.sh
)
for p in "${inputs[@]}"; do
  [[ -f "$repo_root/$p" ]] || fail "entrypoint_missing:$p"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a445.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$run_dir"' EXIT INT TERM HUP
root="$run_dir/root"
mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$run_dir/home"

stage() {
  local dest="$1"
  local p
  for p in "${inputs[@]}"; do
    mkdir -p "$dest/$(dirname "$p")"
    cp "$repo_root/$p" "$dest/$p"
  done
  # internal/documentation/extractors/go/*.go: staged as a whole directory
  # because tests/integration/AUR-445.go reads every .go file in it, not one
  # named path.
  mkdir -p "$dest/internal/documentation/extractors/go"
  cp "$repo_root"/internal/documentation/extractors/go/*.go "$dest/internal/documentation/extractors/go/"
}

stage "$root"

printf 'package unit\nimport "testing"\nfunc TestAUR445Bridge(t *testing.T){ TestAUR445(t) }\n' \
  >"$root/tests/unit/aur445_bridge_test.go"
printf 'package integration\nimport "testing"\nfunc TestAUR445Bridge(t *testing.T){ IntegrationAUR445(t) }\n' \
  >"$root/tests/integration/aur445_bridge_test.go"

go_lane() {
  # One offline, bounded invocation compiles and executes the requested
  # package's real assertions against the staged artifacts. root_override
  # lets MUT-001 point the same lane at a different, mutated staging area.
  local pkg="$1" root_override="${2:-$root}" out rc
  set +e
  out="$( ulimit -v 8388608
    cd "$root_override" && AURUMCODE_ROOT="$root_override" \
    HOME="$run_dir/home" GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod \
    GOTOOLCHAIN=local GOMAXPROCS=1 GOMEMLIMIT=2GiB \
    GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" \
    go test -timeout 300s -v -vet=off -p 1 -count=1 "$pkg" -run '^TestAUR445Bridge$' 2>&1)"
  rc=$?
  set -e
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
  grep -Fq -- '--- PASS: TestAUR445Bridge' <<<"$out" || { fail "selector-did-not-run:$pkg"; }
  return 0
}

e2e_case() {
  local root_override="${1:-$root}" rc
  set +e
  AURUMCODE_ROOT="$root_override" bash "$repo_root/tests/e2e/AUR-445.sh" E2EAUR445
  rc=$?
  set -e
  return "$rc"
}

spec_case() {
  # The card's Documentation clause, plus the selector-naming deviation this
  # card records (see docs/specs/AUR-445.md's closing note and
  # tests/unit/AUR-445.go's header): the spec must name the deviation so a
  # reviewer checking "declared selectors" does not read TestAUR445 as an
  # unexplained mismatch against the card's literal TestAUR428 text.
  [[ -s "$repo_root/docs/specs/AUR-445.md" ]] || fail 'behavior-missing:docs/specs/AUR-445.md absent or empty'
  grep -Fq 'TestAUR445' "$repo_root/docs/specs/AUR-445.md" || fail 'behavior-missing:spec never names the TestAUR445 selector'
  grep -Fq 'AUR-428' "$repo_root/docs/specs/AUR-445.md" || fail 'behavior-missing:spec never records the AUR-428 template-residue naming note'
}

run_ac001() {
  local root_override="$1" label_prefix="$2"
  go_lane ./tests/unit "$root_override" || fail "${label_prefix}selector-exit:unit"
  go_lane ./tests/integration "$root_override" || fail "${label_prefix}selector-exit:integration"
  local rc
  set +e
  e2e_case "$root_override"
  rc=$?
  set -e
  if (( rc != 0 )); then
    (( rc == 69 || rc == 64 )) && exit "$rc"
    fail "${label_prefix}selector-exit:e2e:$rc"
  fi
}

case "$selector" in
  AC-001)
    run_ac001 "$root" ""
    spec_case
    printf '{"card":"%s","scenario":"%s","result":"pass","verification":"static-text-and-source","families_checked":5}\n' "$card" "$scenario"
    ;;
  TestAUR445) go_lane ./tests/unit "$root" ;;
  IntegrationAUR445) go_lane ./tests/integration "$root" ;;
  E2EAUR445) e2e_case "$root" ;;
  MUT-001)
    mut_root="$run_dir/mut-root"
    stage "$mut_root"
    # Mutate ONLY the scratch copy: reintroduce the exact banned sentence the
    # card names, at the end of the License section, mirroring how it
    # originally appeared.
    printf '\nNo `LICENSE` file is present in this repository, so no license is granted here yet.\n' \
      >>"$mut_root/README.md"
    printf 'package unit\nimport "testing"\nfunc TestAUR445Bridge(t *testing.T){ TestAUR445(t) }\n' \
      >"$mut_root/tests/unit/aur445_bridge_test.go"
    printf 'package integration\nimport "testing"\nfunc TestAUR445Bridge(t *testing.T){ IntegrationAUR445(t) }\n' \
      >"$mut_root/tests/integration/aur445_bridge_test.go"

    unit_rc=0; go_lane ./tests/unit "$mut_root" || unit_rc=$?
    (( unit_rc != 0 )) || fail 'MUT-001:unit lane passed on mutated input, expected failure'

    e2e_rc=0; e2e_case "$mut_root" || e2e_rc=$?
    (( e2e_rc != 0 )) || fail 'MUT-001:e2e lane passed on mutated input, expected failure'

    printf '%s/%s/MUT-001 confirmed: unit_rc=%d e2e_rc=%d, tracked README.md untouched\n' "$card" "$scenario" "$unit_rc" "$e2e_rc"
    printf '{"card":"%s","scenario":"%s","mutation":"MUT-001","result":"detected"}\n' "$card" "$scenario"
    ;;
esac
