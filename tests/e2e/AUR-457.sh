#!/usr/bin/env bash
# AUR-457 E2E: the finding, re-derived black-box and offline, WITHOUT reusing
# tests/acceptance/AUR-457.sh or the Go layers -- so a single bug in the
# acceptance cannot fake a pass in every layer.
#
# It re-establishes the causal chain independently:
#   1. cmd/regenerate-docs/main.go registers two constructors,
#   2. whose definitions live in two native.go files,
#   3. that AUR-002's read_paths does not enumerate  (the cause), while
#   4. the legacy characterization baseline is unchanged  (not the cause).
#
# Step 3 reads .board/cards/done/AUR-002.md, which is outside AUR-457's
# read_paths and therefore absent under the sealed profile. That step is
# SKIPPED as unavailable there and only asserted on a host run, where the card
# is present. Skipping is recorded in the receipt, never silently.
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-457'
readonly scenario='E2EAUR457'

note() { printf '%s/%s/%s: %s\n' "$card" "$scenario" "$1" "$2" >&2; }
fail() { note "$1" "$2"; exit 1; }
infra() { note infrastructure "$1"; exit 79; }

selector="${1:-E2EAUR457}"
case "$selector" in
  E2EAUR457|E2EAUR446) ;;
  *) note unknown-selector "$selector"; exit 64 ;;
esac

for tool in grep awk; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing tool: $tool"
done

root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." 2>/dev/null && pwd -P)" || infra 'repo root unresolved'
cd -- "$root" || infra 'repo root unreachable'

readonly subject='cmd/regenerate-docs/main.go'
readonly aur002_card='.board/cards/done/AUR-002.md'

# --- 1. the subject registers the native extractors -------------------------
[[ -f $subject && ! -L $subject && -s $subject ]] || fail subject-absent "$subject"
grep -Fq 'rustExtractor.NewNativeExtractor(' "$subject"   || fail registration-lost 'rust native extractor no longer registered'
grep -Fq 'csharpExtractor.NewNativeExtractor(' "$subject" || fail registration-lost 'csharp native extractor no longer registered'

# --- 2. their definitions live in the two native.go files -------------------
declare -a natives=(
  'internal/documentation/extractors/rust/native.go'
  'internal/documentation/extractors/csharp/native.go'
)
for src in "${natives[@]}"; do
  [[ -f $src && ! -L $src && -s $src ]] || fail native-source-absent "$src"
  grep -Eq '^func NewNativeExtractor\(\) \*NativeExtractor \{' "$src" ||
    fail native-definition-lost "$src no longer defines NewNativeExtractor"
done

# --- 3. AUR-002's read_paths omits them (host-only; card is out of profile) --
read_paths_checked=false
if [[ -f $aur002_card && ! -L $aur002_card && -r $aur002_card ]]; then
  read_paths_line="$(awk '/^read_paths: / { print; exit }' "$aur002_card")" ||
    infra 'AUR-002 read_paths unreadable'
  [[ -n $read_paths_line ]] || fail aur002-contract 'AUR-002 declares no read_paths'
  # The extractor packages are enumerated file by file -- that is the shape of
  # the defect. If AUR-002 ever lists the directory instead, this claim falls
  # and docs/specs/AUR-457.md is stale.
  printf '%s\n' "$read_paths_line" | grep -Fq 'internal/documentation/extractors/rust/extractor.go' ||
    fail aur002-contract 'AUR-002 no longer enumerates rust/extractor.go; the finding is stale'
  for src in "${natives[@]}"; do
    if printf '%s\n' "$read_paths_line" | grep -Fq "$src"; then
      fail aur002-contract "AUR-002 read_paths now includes $src; the finding is repaired and docs/specs/AUR-457.md is stale"
    fi
  done
  read_paths_checked=true
else
  note read-paths-unavailable "$aur002_card is outside this profile; step 3 asserted only on a host run"
fi

# --- 4. the characterization baseline is unchanged --------------------------
readonly baseline='tests/characterization/legacy-baseline'
[[ -d $baseline && ! -L $baseline ]] || fail baseline-absent "$baseline"
while IFS='|' read -r replay want; do
  [[ -n $replay ]] || continue
  file="$baseline/$replay"
  [[ -f $file && ! -L $file && -s $file ]] || fail baseline-replay-absent "$file"
  grep -Fqx -- "$want" "$file" || fail characterization-drift "$replay lost its exact summary line"
done <<'REPLAYS'
complete-success.stderr|aurumcode: result=ok docs=1 skipped=0 failed=0 languages_skipped=none output=/tmp/aurum-a002-output index_pages=1 index_pages_excluded=0 config=true
missing-extractor.stderr|aurumcode: result=partial docs=1 skipped=1 failed=0 languages_skipped=java output=/tmp/aurum-a002-output index_pages=1 index_pages_excluded=0 config=true
extractor-error.stderr|aurumcode: result=partial docs=1 skipped=0 failed=1 languages_skipped=none output=/tmp/aurum-a002-output index_pages=1 index_pages_excluded=0 config=true
REPLAYS

printf '{"card":"%s","scenario":"%s","read_paths_checked":%s,"result":"pass"}\n' \
  "$card" "$scenario" "$read_paths_checked"
