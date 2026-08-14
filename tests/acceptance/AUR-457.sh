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
  AC-001) ;;
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
  [extractor-error]='aurumcode: result=partial docs=1 skipped=0 failed=1 languages_skipped=none output=/tmp/aurum-a002-output index_pages=1 index_pages_excluded=0 config=true'
)
[[ -d "$baseline" && ! -L "$baseline" ]] ||
  fail characterization-intact "baseline root is not a directory: $baseline"
for id in complete-success missing-extractor extractor-error; do
  replay="$baseline/$id.stderr"
  readable "$replay" || fail characterization-intact "baseline replay absent: $replay"
  grep -Fqx -- "${baseline_line[$id]}" "$replay" ||
    fail characterization-intact "baseline replay $id no longer carries its summary line"
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

printf '{"card":"%s","scenario":"%s","claims":%d,"result":"pass"}\n' \
  "$card" "$scenario" "$claims"
