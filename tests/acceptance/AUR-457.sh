#!/usr/bin/env bash
# AUR-457 AC-001: "grep de verificacao estatica sobre os specs entregues".
#
# This card measured why two `done` cards' acceptances no longer exit 0, and
# found that the diagnosis it was opened with was wrong on both counts. The
# assertions below are the live, falsifiable form of that finding -- not a
# restatement of it. Each one names a fact that can go red on its own:
#
#   1. subject-registers-native   cmd/regenerate-docs still registers the two
#                                 native extractors AUR-427 added.
#   2. native-sources-present     those constructors are really defined in the
#                                 two native.go files AUR-002's read_paths
#                                 never enumerated.
#   3. characterization-intact    the legacy baseline replays still carry the
#                                 exact summary lines tests/acceptance/AUR-002.sh
#                                 greps for -- i.e. nothing drifted.
#   4. infra-classifier-gap       tests/acceptance/AUR-002.sh's infrastructure
#                                 classifier really does not recognise the
#                                 compiler's `undefined:`.
#   5. spec-records-evidence      docs/specs/AUR-457.md carries the measured
#                                 evidence, not a claim.
#   6. spec-declares-unreachable  docs/specs/AUR-457.md states plainly that the
#                                 card's Outcome is not reachable from its own
#                                 contract.
#
# 1-4 read the tree, not the spec. If the code changes, they fall. If someone
# rebases the baseline, 3 falls. If someone closes the classifier gap, 4 falls
# and this spec must be rewritten -- which is the point: the spec is pinned to
# a state of the world, not to itself.
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-457'
readonly scenario='AC-001'

readonly subject='cmd/regenerate-docs/main.go'
readonly rust_native='internal/documentation/extractors/rust/native.go'
readonly csharp_native='internal/documentation/extractors/csharp/native.go'
readonly baseline='tests/characterization/legacy-baseline'
readonly aur002_accept='tests/acceptance/AUR-002.sh'
readonly spec='docs/specs/AUR-457.md'

infra() {
  printf '%s/%s/infrastructure: %s\n' "$card" "$scenario" "$1" >&2
  exit 79
}

fail() {
  printf '%s/%s/%s: %s\n' "$card" "$scenario" "$1" "$2" >&2
  exit 1
}

selector="${1:-AC-001}"
case "$selector" in
  AC-001|TestAUR457|IntegrationAUR457|E2EAUR457) ;;
  invalid-input)
    printf '%s/%s/invalid-input\n' "$card" "$scenario" >&2
    exit 64
    ;;
  boundary-overflow)
    printf '%s/%s/boundary-overflow\n' "$card" "$scenario" >&2
    exit 65
    ;;
  *)
    printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2
    exit 64
    ;;
esac

for tool in grep awk; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing tool: $tool"
done

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." 2>/dev/null && pwd -P)" ||
  infra 'repo root unresolved'
cd -- "$repo_root" || infra 'repo root unreachable'

readable() {
  # A required input that is absent is infrastructure only when it is outside
  # what this card owns. Everything checked here is inside paths/read_paths, so
  # absence is a real finding, never a shrug.
  [[ -f "$1" && ! -L "$1" && -r "$1" && -s "$1" ]]
}

# ---------------------------------------------------------------------------
# Go lanes.
#
# tests/unit/AUR-457.go and tests/integration/AUR-457.go are not `_test.go`
# files, so `go test ./...` never registers them -- a selector Go never runs is
# worse than one that runs without asserting. This stages both packages plus a
# generated bridge `_test.go` into a scratch module under TMPDIR (the sealed
# profile makes only this card's `paths` writable, so the packages cannot be
# written in place) and points them back at the real checkout via
# AURUMCODE_ROOT. Both layers use only the standard library, so the scratch
# module needs no network and no go.sum.
#
# The lane refuses to accept a green that executed nothing: it requires exactly
# one `ok`, no `[no test files]`, and an explicit `--- PASS` for the bridge.
# ---------------------------------------------------------------------------
go_lane() {
  local pkg="$1" fn="$2" out rc stage
  command -v go >/dev/null 2>&1 || infra 'missing tool: go; the Go lanes are unproven'
  stage="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a457.XXXXXX")" || infra 'scratch module directory unavailable'
  # shellcheck disable=SC2064
  trap "rm -rf -- '$stage'" RETURN
  mkdir -p -- "$stage/$pkg" "$stage/home" "$stage/cache" "$stage/gotmp" ||
    infra 'scratch module tree unavailable'
  cp -- "$repo_root/$pkg/AUR-457.go" "$stage/$pkg/AUR-457.go" ||
    fail "selector-unstageable:$pkg" "$pkg/AUR-457.go could not be staged"
  printf 'module aurum457scratch\n\ngo 1.21\n' >"$stage/go.mod" ||
    infra 'scratch go.mod unwritable'
  printf 'package %s\n\nimport "testing"\n\nfunc TestAUR457Bridge(t *testing.T) { %s(t) }\n' \
    "$(basename -- "$pkg")" "$fn" >"$stage/$pkg/aur457_bridge_test.go" ||
    infra 'bridge test unwritable'

  set +e
  out="$( ulimit -v 8388608
    cd "$stage" && AURUMCODE_ROOT="$repo_root" HOME="$stage/home" AUR457_NESTED=1 \
    GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod GOTOOLCHAIN=local \
    GOMAXPROCS=1 GOMEMLIMIT=2GiB GOCACHE="$stage/cache" GOTMPDIR="$stage/gotmp" \
    go test -timeout 300s -v -vet=off -p 1 -count=1 "./$pkg" -run '^TestAUR457Bridge$' 2>&1)"
  rc=$?
  set -e

  if (( rc != 0 )); then
    if grep -Eiq 'command not found|go: downloading|module lookup disabled|no required module provides|cannot find module|toolchain.*(unavailable|not found)' <<<"$out"; then
      infra "Go lane unavailable for $pkg (exit $rc)"
    fi
    detail="$(grep -om1 'AUR-457: [^"]*' <<<"$out" | head -n1 || true)"
    [[ -n "$detail" ]] || detail="no bounded detail (exit $rc)"
    fail "selector-exit:${pkg##*/}" "$detail"
  fi
  # A lane that compiled but executed nothing is not a pass.
  (( $(grep -c '^ok ' <<<"$out") == 1 )) || fail "zero-tests:${pkg##*/}" 'lane reported no ok line'
  ! grep -Fq '[no test files]' <<<"$out" || fail "no-test-files:${pkg##*/}" 'Go registered no test in the lane'
  grep -Fq -- '--- PASS: TestAUR457Bridge' <<<"$out" || fail "selector-did-not-run:${pkg##*/}" 'the bridge selector never executed'
}

e2e_lane() {
  local out rc
  set +e
  out="$(bash "$repo_root/tests/e2e/AUR-457.sh" E2EAUR457 2>&1)"
  rc=$?
  set -e
  if (( rc != 0 )); then
    (( rc == 79 )) && infra "E2E lane unavailable: $(tail -n1 <<<"$out")"
    fail 'selector-exit:e2e' "$(grep -om1 'AUR-457/E2EAUR457/[A-Za-z0-9/_:-]*' <<<"$out" | head -n1 || echo "exit $rc")"
  fi
  grep -Fq '"result":"pass"' <<<"$out" || fail 'selector-did-not-run:e2e' 'E2E emitted no pass receipt'
}

case "$selector" in
  TestAUR457) go_lane tests/unit TestAUR457; exit 0 ;;
  IntegrationAUR457) go_lane tests/integration IntegrationAUR457; exit 0 ;;
  E2EAUR457) e2e_lane; exit 0 ;;
esac

claims=0

# ---------------------------------------------------------------------------
# Claim 1: the subject still registers both native extractors.
# ---------------------------------------------------------------------------
readable "$subject" || fail subject-registers-native "subject absent or unreadable: $subject"
for ctor in 'rustExtractor.NewNativeExtractor' 'csharpExtractor.NewNativeExtractor'; do
  grep -Fq -- "$ctor(" "$subject" ||
    fail subject-registers-native "$subject no longer registers $ctor"
done
claims=$((claims + 1))

# ---------------------------------------------------------------------------
# Claim 2: both constructors are DEFINED in the two native.go files that
# AUR-002's read_paths never enumerated. Definition, not mention -- a comment
# naming the symbol must not satisfy this.
# ---------------------------------------------------------------------------
for src in "$rust_native" "$csharp_native"; do
  readable "$src" || fail native-sources-present "native extractor source absent: $src"
  grep -Eq '^func NewNativeExtractor\(\) \*NativeExtractor \{' "$src" ||
    fail native-sources-present "$src does not define func NewNativeExtractor() *NativeExtractor"
done
claims=$((claims + 1))

# ---------------------------------------------------------------------------
# Claim 3: the legacy characterization baseline is intact. These are the exact
# lines tests/acceptance/AUR-002.sh greps for with -Fqx. If AUR-424's go/doc
# switch had actually drifted the characterization, these would be gone.
# ---------------------------------------------------------------------------
declare -A baseline_line=(
  [complete-success]='aurumcode: result=ok docs=1 skipped=0 failed=0 languages_skipped=none output=/tmp/aurum-a002-output index_pages=1 index_pages_excluded=0 config=true'
  [missing-extractor]='aurumcode: result=partial docs=1 skipped=1 failed=0 languages_skipped=java output=/tmp/aurum-a002-output index_pages=1 index_pages_excluded=0 config=true'
  # Rebased by AUR-457: AUR-424 removed the gomarkdoc subprocess this stub
  # simulated crashing, so the exit-7 stub now breaks nothing and both
  # documents are produced. This is the one line in this table that this
  # card's own rebase changed -- complete-success and missing-extractor are
  # unchanged because their behavior did not change.
  [extractor-error]='aurumcode: result=ok docs=2 skipped=0 failed=0 languages_skipped=none output=/tmp/aurum-a002-output index_pages=2 index_pages_excluded=0 config=true'
)
[[ -d "$baseline" && ! -L "$baseline" ]] ||
  fail characterization-intact "baseline root is not a directory: $baseline"
for id in complete-success missing-extractor extractor-error; do
  replay="$baseline/$id.stderr"
  readable "$replay" || fail characterization-intact "baseline replay absent: $replay"
  # The card's MUT-001 is exactly this mutation -- "deixar uma caracterizacao
  # fixando comportamento que o codigo nao tem mais" -- and its Expected clause
  # requires the marker to carry MUT-001, so the token is part of the path.
  grep -Fqx -- "${baseline_line[$id]}" "$replay" ||
    fail MUT-001/characterization-intact "baseline replay $id no longer carries its summary line"
done
claims=$((claims + 1))

# ---------------------------------------------------------------------------
# Claim 4: AUR-002's infrastructure classifier does not recognise `undefined:`.
# This is the defect that turned a materialization gap into a false
# baseline-drift. When someone closes it, this claim goes red on purpose and
# docs/specs/AUR-457.md must be rewritten.
# ---------------------------------------------------------------------------
readable "$aur002_accept" || fail infra-classifier-gap "absent or unreadable: $aur002_accept"
classifier="$(grep -F "grep -Eiq 'command not found" "$aur002_accept" || true)"
[[ -n "$classifier" ]] ||
  fail infra-classifier-gap "$aur002_accept no longer carries the infrastructure classifier this finding names"
if printf '%s\n' "$classifier" | grep -Fq 'undefined:'; then
  fail infra-classifier-gap "$aur002_accept now classifies 'undefined:' as infrastructure; docs/specs/AUR-457.md is stale"
fi
claims=$((claims + 1))

# ---------------------------------------------------------------------------
# Claim 5: the delivered spec records the measured evidence.
# ---------------------------------------------------------------------------
readable "$spec" || fail spec-records-evidence "delivered spec absent or unreadable: $spec"
declare -a evidence=(
  'AUR-002/AC-001/baseline-drift'
  'undefined: rustExtractor.NewNativeExtractor'
  'undefined: csharpExtractor.NewNativeExtractor'
  'AUR-001/AC-001/independent-tracked-source'
  '9ef5273'
  "$rust_native"
  "$csharp_native"
  'NO_GIT'
)
for row in "${evidence[@]}"; do
  grep -Fq -- "$row" "$spec" ||
    fail spec-records-evidence "$spec omits measured evidence: $row"
done
claims=$((claims + 1))

# ---------------------------------------------------------------------------
# Claim 6: the spec declares the Outcome unreachable from this contract, and
# names both blockers. A spec that quietly claimed success would fail here.
# ---------------------------------------------------------------------------
grep -Fq -- 'nao e alcancavel a partir do contrato deste card' "$spec" ||
  fail spec-declares-unreachable "$spec does not declare the Outcome unreachable from this card's contract"
grep -Fq -- 'forbidden_paths' "$spec" ||
  fail spec-declares-unreachable "$spec does not name the forbidden_paths blocker"
grep -Fq -- 'bootstrap-readonly-v1' "$spec" ||
  fail spec-declares-unreachable "$spec does not name the profile blocker"
claims=$((claims + 1))

# ---------------------------------------------------------------------------
# The three declared proof layers, executed for real.
#
# AUR457_NESTED breaks one cycle: tests/integration/AUR-457.go asserts this
# script's AC-001 exit code and receipt, so a nested AC-001 must not re-enter
# the lanes. Nested runs execute the six static claims only -- which is exactly
# what the Integration layer is asserting about.
# ---------------------------------------------------------------------------
if [[ -z "${AUR457_NESTED:-}" ]]; then
  go_lane tests/unit TestAUR457
  go_lane tests/integration IntegrationAUR457
  e2e_lane
fi

printf '{"card":"%s","scenario":"%s","claims":%d,"result":"pass"}\n' \
  "$card" "$scenario" "$claims"
