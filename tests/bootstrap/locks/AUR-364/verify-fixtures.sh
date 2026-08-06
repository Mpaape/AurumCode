#!/usr/bin/env bash
# tests/bootstrap/locks/AUR-364/verify-fixtures.sh
#
# Executes every case in cases.tsv against a throwaway copy of the real accept
# surface (tests/acceptance/AUR-364.sh + the committed
# .board/bootstrap/locks/docs.yml + the card's read-paths.attested seal + the
# card's resolved.lock resolution artifact + the four real read_paths), applies
# that case's mutation from mutations.sh, and asserts the exact exit code and
# `AUR-364/AC-001/<code>` (or pass JSON) the case promises. Never mutates the
# real repository tree: every case runs against a `cp -r` throwaway copy
# created and destroyed per case under a private mktemp directory.
#
# The `extra` column adds a second, independent assertion:
#   +<literal>   the case's resulting lock must CONTAIN <literal>
#   !baseline    the case's resulting lock must DIFFER (by sha256) from the
#                committed baseline lock — i.e. the change is visible
#   -            no extra assertion
#
# After the case table come five ORACLES, each of which asserts a property of
# the delivered artifacts rather than replaying a case:
#
#   independent-reference-truth-identity
#       Re-derives the expected (manifest, name) tool identity set from the two
#       Gemfiles with a DIFFERENT tool chain than tests/acceptance/AUR-364.sh
#       uses (sed + sort instead of the bash grammar parser), and compares it
#       to the committed lock as a SET, marking each item seen by name in both
#       directions. It never reads the lock while building itself, so it is not
#       a copy of what it verifies (Regra dos Seis, item 6).
#
#   independent-reference-truth-resolution
#       The same discipline for the resolved facts: every lock entry's
#       resolved_version and artifact_digest are re-read straight out of
#       resolved.lock by awk, and matched pair by pair.
#
#   set-not-count-audit
#       Round 4's own gate. It re-greps tests/acceptance/AUR-364.sh and fails
#       if ANY arithmetic comparison there is untagged, or if any single
#       comparison has a set cardinality on BOTH sides — the exact shape that
#       let blocker B4 through. The detector is itself validated against a
#       synthetic positive control, so an empty result can never be mistaken
#       for a clean result (Lei 12).
#
#   determinism-5x
#       Five --generate runs must produce the same sha256, and that sha256 must
#       equal the committed lock.
#
#   no-enumeration-in-source
#       No derived tool name and no install verb pattern may appear in the
#       accept script's executable code.
#
# Usage: tests/bootstrap/locks/AUR-364/verify-fixtures.sh
set -uo pipefail

self_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)"
repo_root="$(CDPATH= cd -- "$self_dir/../../../.." && pwd -P)"
cases_tsv="$self_dir/cases.tsv"
mutations_sh="$self_dir/mutations.sh"
accept_script="$repo_root/tests/acceptance/AUR-364.sh"
lock_file="$repo_root/.board/bootstrap/locks/docs.yml"
attest_file="$self_dir/read-paths.attested"
resolved_file="$self_dir/resolved.lock"
TAB=$'\t'

# shellcheck disable=SC1090
. "$mutations_sh"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

spec_file="$repo_root/docs/specs/AUR-364.md"

mirror="$work/mirror_base"
mkdir -p "$mirror/tests/acceptance" "$mirror/tests/bootstrap/locks/AUR-364" \
  "$mirror/.board/bootstrap/locks" "$mirror/.aurumcode" "$mirror/.docker" \
  "$mirror/.github/workflows" "$mirror/docs/specs"
cp "$accept_script" "$mirror/tests/acceptance/AUR-364.sh"
chmod +x "$mirror/tests/acceptance/AUR-364.sh"
cp "$lock_file" "$mirror/.board/bootstrap/locks/docs.yml"
cp "$attest_file" "$mirror/tests/bootstrap/locks/AUR-364/read-paths.attested"
cp "$resolved_file" "$mirror/tests/bootstrap/locks/AUR-364/resolved.lock"
# Round 5, Lei 20: the accept now seals every file of the card's declared
# fixture DIRECTORY, so the mirror must be a faithful copy of that directory or
# every case would fail for the wrong reason.
cp "$cases_tsv" "$mirror/tests/bootstrap/locks/AUR-364/cases.tsv"
cp "$mutations_sh" "$mirror/tests/bootstrap/locks/AUR-364/mutations.sh"
cp "$self_dir/verify-fixtures.sh" "$mirror/tests/bootstrap/locks/AUR-364/verify-fixtures.sh"
cp "$repo_root/Gemfile" "$mirror/Gemfile"
cp "$repo_root/.aurumcode/Gemfile" "$mirror/.aurumcode/Gemfile"
cp "$repo_root/.docker/docs.Dockerfile" "$mirror/.docker/docs.Dockerfile"
cp "$repo_root/.github/workflows/documentation.yml" "$mirror/.github/workflows/documentation.yml"
cp "$spec_file" "$mirror/docs/specs/AUR-364.md"

baseline_lock_sha="$(sha256sum -- "$lock_file" | awk '{print $1}')"

total=0
failed=0
mkdir -p "$work/cases"

while IFS=$'\t' read -r name class expected extra; do
  [[ "$name" != \#* && -n "$name" ]] || continue
  total=$((total + 1))
  fn="mut_${name}"
  if ! declare -F "$fn" >/dev/null; then
    echo "FAIL  $name  no mutation function $fn defined in mutations.sh"
    failed=$((failed + 1))
    continue
  fi
  case_dir="$work/cases/$name"
  rm -rf "$case_dir"
  cp -r "$mirror" "$case_dir"
  "$fn" "$case_dir"
  out="$("$case_dir/tests/acceptance/AUR-364.sh" AC-001 2>&1)"
  code=$?
  ok=0
  if [[ "$expected" == 'pass' ]]; then
    [[ $code -eq 0 ]] && printf '%s' "$out" | grep -q '"result":"pass"' && ok=1
  else
    [[ $code -eq 1 && "$out" == "AUR-364/AC-001/$expected" ]] && ok=1
  fi
  extra_note=''
  case "${extra:--}" in
    -) ;;
    '!baseline')
      case_lock_sha='<absent>'
      [[ -f "$case_dir/.board/bootstrap/locks/docs.yml" ]] &&
        case_lock_sha="$(sha256sum -- "$case_dir/.board/bootstrap/locks/docs.yml" | awk '{print $1}')"
      if [[ "$case_lock_sha" == "$baseline_lock_sha" ]]; then
        ok=0; extra_note="  [extra FAILED: lock sha identical to baseline, change was invisible]"
      else
        extra_note="  [extra ok: lock sha moved $baseline_lock_sha -> $case_lock_sha]"
      fi
      ;;
    +*)
      needle="${extra#+}"
      if grep -Fq -- "$needle" "$case_dir/.board/bootstrap/locks/docs.yml" 2>/dev/null; then
        extra_note="  [extra ok: '$needle' present in regenerated lock]"
      else
        ok=0; extra_note="  [extra FAILED: '$needle' absent from regenerated lock]"
      fi
      ;;
  esac
  if [[ $ok -eq 1 ]]; then
    echo "PASS  $name  [$class]  exit=$code  $out$extra_note"
  else
    echo "FAIL  $name  [$class]  expected exit/[$expected], got exit=$code: $out$extra_note"
    failed=$((failed + 1))
  fi
done < "$cases_tsv"

# --- restoration proof ------------------------------------------------------
restore_sha_before="$(sha256sum -- "$lock_file" | awk '{print $1}')"
restore_out="$("$mirror/tests/acceptance/AUR-364.sh" AC-001 2>&1)"
restore_code=$?
restore_sha_after="$(sha256sum -- "$lock_file" | awk '{print $1}')"
if [[ $restore_code -eq 0 ]] && printf '%s' "$restore_out" | grep -q '"result":"pass"' \
   && [[ "$restore_sha_before" == "$restore_sha_after" ]]; then
  echo "PASS  restore-replay  [restore]  exit=$restore_code  $restore_out"
else
  echo "FAIL  restore-replay  [restore]  exit=$restore_code  $restore_out  (lock sha $restore_sha_before -> $restore_sha_after)"
  failed=$((failed + 1))
fi
total=$((total + 1))

# --- oracle 1: independent reference truth, tool IDENTITY -------------------
# Pairs, not names: keying on the name alone was blocker B2.
independent_pairs="$(
  { sed -n "s/^[[:space:]]*gem[[:space:]]\{1,\}['\"]\([^'\"]*\)['\"].*/Gemfile\t\1/p" "$repo_root/Gemfile"
    sed -n "s/^[[:space:]]*gem[[:space:]]\{1,\}['\"]\([^'\"]*\)['\"].*/.aurumcode\/Gemfile\t\1/p" "$repo_root/.aurumcode/Gemfile"
  } | LC_ALL=C sort
)"
lock_pairs="$(
  awk -F': ' '
    /^tool\[[0-9]+\]\.name: /        { idx = $1; sub(/^tool\[/, "", idx); sub(/\].name$/, "", idx); nm[idx] = $2 }
    /^tool\[[0-9]+\]\.declared_in: / { idx = $1; sub(/^tool\[/, "", idx); sub(/\].declared_in$/, "", idx); mf[idx] = $2 }
    END { for (i in nm) printf "%s\t%s\n", mf[i], nm[i] }
  ' "$lock_file" | LC_ALL=C sort
)"
total=$((total + 1))
identity_ok=1
identity_note=''
declare -A IND_SEEN=() LOCKPAIR_SEEN=()
while IFS= read -r p; do
  [[ -n "$p" ]] || continue
  if [[ -n "${IND_SEEN[$p]:-}" ]]; then
    identity_ok=0; identity_note="duplicate pair in independent derivation: $p"
  fi
  IND_SEEN[$p]=1
done <<< "$independent_pairs"
while IFS= read -r p; do
  [[ -n "$p" ]] || continue
  if [[ -n "${LOCKPAIR_SEEN[$p]:-}" ]]; then
    identity_ok=0; identity_note="duplicate pair in lock: $p"
  fi
  LOCKPAIR_SEEN[$p]=1
done <<< "$lock_pairs"
# Lei 12: an empty derived set is a FAILURE, not agreement.
if [[ "${#IND_SEEN[@]}" == '0' || "${#LOCKPAIR_SEEN[@]}" == '0' ]]; then
  identity_ok=0; identity_note="empty derived set (independent=${#IND_SEEN[@]} lock=${#LOCKPAIR_SEEN[@]})"
fi
for p in "${!IND_SEEN[@]}"; do
  if [[ -z "${LOCKPAIR_SEEN[$p]:-}" ]]; then
    identity_ok=0; identity_note="independently derived pair missing from lock: $p"
  fi
done
for p in "${!LOCKPAIR_SEEN[@]}"; do
  if [[ -z "${IND_SEEN[$p]:-}" ]]; then
    identity_ok=0; identity_note="lock pair not independently derivable: $p"
  fi
done
if (( identity_ok == 1 )); then
  echo "PASS  independent-reference-truth-identity  [oracle]  ${#IND_SEEN[@]} (manifest,name) pairs re-derived by a different tool chain; both set inclusions hold by name"
else
  echo "FAIL  independent-reference-truth-identity  [oracle]  $identity_note"
  printf 'independent:\n%s\nlock:\n%s\n' "$independent_pairs" "$lock_pairs"
  failed=$((failed + 1))
fi

# --- oracle 2: independent reference truth, RESOLVED facts ------------------
# Every lock entry's resolved_version and artifact_digest are read back out of
# resolved.lock by awk — a different tool chain from the accept script's bash
# parser — and matched pair by pair, both inclusions, by name.
total=$((total + 1))
resolution_ok=1
resolution_note=''
declare -A RESV=() RESD=() RESSEEN=()
while IFS=$'\t' read -r rk rm rn rv rd; do
  [[ "$rk" == 'gem' ]] || continue
  key="$rm$TAB$rn"
  if [[ -n "${RESV[$key]:-}" ]]; then
    resolution_ok=0; resolution_note="duplicate resolution record: $key"
  fi
  RESV[$key]="$rv"; RESD[$key]="$rd"
done < <(awk -F'\t' '$1 == "gem" { print }' "$resolved_file")
if [[ "${#RESV[@]}" == '0' ]]; then
  resolution_ok=0; resolution_note='resolution artifact yielded no records'
fi
while IFS= read -r p; do
  [[ -n "$p" ]] || continue
  idx="$(awk -F': ' -v want="$p" '
    /^tool\[[0-9]+\]\.name: /        { i = $1; sub(/^tool\[/, "", i); sub(/\].name$/, "", i); nm[i] = $2 }
    /^tool\[[0-9]+\]\.declared_in: / { i = $1; sub(/^tool\[/, "", i); sub(/\].declared_in$/, "", i); mf[i] = $2 }
    END { for (i in nm) if (mf[i] "\t" nm[i] == want) print i }
  ' "$lock_file")"
  if [[ -z "$idx" ]]; then
    resolution_ok=0; resolution_note="no lock entry for $p"
    continue
  fi
  lv="$(grep -F "tool[$idx].resolved_version: " "$lock_file" | sed -e "s/^tool\[$idx\]\.resolved_version: //")"
  ld="$(grep -F "tool[$idx].artifact_digest: " "$lock_file" | sed -e "s/^tool\[$idx\]\.artifact_digest: //")"
  if [[ "$lv" != "${RESV[$p]:-<none>}" || "$ld" != "${RESD[$p]:-<none>}" ]]; then
    resolution_ok=0; resolution_note="lock/resolution divergence at $p: lock=($lv,$ld) resolution=(${RESV[$p]:-<none>},${RESD[$p]:-<none>})"
  fi
  RESSEEN[$p]=1
done <<< "$lock_pairs"
for key in "${!RESV[@]}"; do
  if [[ -z "${RESSEEN[$key]:-}" ]]; then
    resolution_ok=0; resolution_note="resolution record nothing in the lock consumes: $key"
  fi
done
if (( resolution_ok == 1 )); then
  echo "PASS  independent-reference-truth-resolution  [oracle]  ${#RESV[@]} resolved records re-read by awk; every lock entry matches its record and no record is unused"
else
  echo "FAIL  independent-reference-truth-resolution  [oracle]  $resolution_note"
  failed=$((failed + 1))
fi

# --- oracle 3: set-not-count audit of the accept script ---------------------
# The class that survived three rounds is "a comparison of CARDINALITY standing
# in for a comparison of SETS". This oracle proves mechanically that no such
# comparison is left: every arithmetic comparison in the accept script must
# carry one of the three declared tags, and none may have a set size on both
# sides.
scan_arith() {
  awk '
    {
      line = $0
      if (line ~ /^[[:space:]]*#/) next
      if (line ~ /for[[:space:]]*\(\(/) next
      if (match(line, /\(\([^()]*\)\)/) == 0) next
      expr = substr(line, RSTART + 2, RLENGTH - 4)
      if (expr !~ /(==|!=|<=|>=|<|>)/) next
      tag = (line ~ /#[[:space:]]*(NONEMPTY|BOUND|ACCOUNTING)/) ? "TAGGED" : "UNTAGGED"
      n = gsub(/\$\{#/, "&", expr)
      if (n >= 2) tag = "DOUBLECARD"
      printf "%s\t%d\t%s\n", tag, FNR, line
    }
  ' "$1"
}
total=$((total + 1))
arith_out="$work/arith.txt"
scan_arith "$accept_script" > "$arith_out"
tagged="$(grep -c '^TAGGED' "$arith_out")"
untagged="$(grep -c '^UNTAGGED' "$arith_out")"
doublecard="$(grep -c '^DOUBLECARD' "$arith_out")"

# POSITIVE CONTROL for the detector itself (Lei 12): if the scanner cannot see
# a planted violation, its silence on the real file means nothing.
control="$work/arith-control.sh"
{
  printf 'if (( derived_a == derived_b )); then :; fi\n'
  printf '(( x > 0 )) || fail y          # NONEMPTY\n'
  printf '(( ${#p[@]} == ${#q[@]} )) || fail z   # BOUND\n'
} > "$control"
control_out="$work/arith-control.txt"
scan_arith "$control" > "$control_out"
ctl_tagged="$(grep -c '^TAGGED' "$control_out")"
ctl_untagged="$(grep -c '^UNTAGGED' "$control_out")"
ctl_double="$(grep -c '^DOUBLECARD' "$control_out")"
if (( tagged > 0 )) && (( untagged == 0 )) && (( doublecard == 0 )) \
   && (( ctl_tagged == 1 )) && (( ctl_untagged == 1 )) && (( ctl_double == 1 )); then
  echo "PASS  set-not-count-audit  [oracle]  ${tagged} arithmetic comparisons, all tagged NONEMPTY/BOUND/ACCOUNTING; 0 untagged; 0 with a set cardinality on both sides; detector positive control fired 1/1/1"
else
  echo "FAIL  set-not-count-audit  [oracle]  tagged=$tagged untagged=$untagged doublecard=$doublecard control(tagged/untagged/double)=$ctl_tagged/$ctl_untagged/$ctl_double"
  grep -E '^(UNTAGGED|DOUBLECARD)' "$arith_out"
  failed=$((failed + 1))
fi

# --- oracle 3b: no pipefail/SIGPIPE race in the accept script ---------------
# `set -o pipefail` plus a consumer that exits as soon as it has an answer
# (`grep -q`, `head`) makes the pipeline's status depend on a RACE: the
# producer is killed by SIGPIPE, the pipeline reports 141, and an `if` reads
# that as "no match" — a check that intermittently does not happen. Measured at
# 7/200 nominal runs before it was removed. This oracle forbids the shape.
scan_pipe_race() {
  awk '
    {
      line = $0
      if (line ~ /^[[:space:]]*#/) next
      if (line ~ /\|[[:space:]]*grep[^|]*[[:space:]]-[A-Za-z]*q/) { printf "RACE\t%d\t%s\n", FNR, line; next }
      if (line ~ /\|[[:space:]]*head([[:space:]]|$)/)             { printf "RACE\t%d\t%s\n", FNR, line; next }
    }
  ' "$1"
}
total=$((total + 1))
race_out="$work/piperace.txt"
scan_pipe_race "$accept_script" > "$race_out"
race_hits="$(grep -c '^RACE' "$race_out")"
race_control="$work/piperace-control.sh"
{
  printf 'printf x | grep -q y || fail z\n'
  printf 'cat "$f" | head -1\n'
  printf 'grep -Fqx a <<< "$b" || fail c\n'
} > "$race_control"
race_ctl="$(scan_pipe_race "$race_control" | grep -c '^RACE')"
if (( race_hits == 0 )) && (( race_ctl == 2 )); then
  echo "PASS  no-pipefail-race  [oracle]  0 early-exit consumers on the right of a pipe; detector positive control fired 2/2"
else
  echo "FAIL  no-pipefail-race  [oracle]  hits=$race_hits control=$race_ctl (expected 0 and 2)"
  cat "$race_out"
  failed=$((failed + 1))
fi

# --- oracle 4: determinism (5 runs, same bytes) -----------------------------
total=$((total + 1))
det_first=''
det_ok=1
for _ in 1 2 3 4 5; do
  det="$("$accept_script" --generate | sha256sum | awk '{print $1}')"
  [[ -n "$det_first" ]] || det_first="$det"
  [[ "$det" == "$det_first" ]] || det_ok=0
done
if (( det_ok == 1 )) && [[ "$det_first" == "$baseline_lock_sha" ]]; then
  echo "PASS  determinism-5x  [oracle]  sha256=$det_first equals the committed lock"
else
  echo "FAIL  determinism-5x  [oracle]  first=$det_first committed=$baseline_lock_sha"
  failed=$((failed + 1))
fi

# --- oracle 5: no verb list, no tool-name list, anywhere in the source -------
total=$((total + 1))
code_only="$work/accept.code"
grep -vE '^[[:space:]]*#' "$accept_script" > "$code_only"
code_lines="$(grep -c . "$code_only")"
independent_names="$(printf '%s\n' "$independent_pairs" | cut -f2 | LC_ALL=C sort -u)"
independent_count="$(printf '%s\n' "$independent_names" | grep -c .)"
name_hits=0
while IFS= read -r n; do
  [[ -n "$n" ]] || continue
  if grep -Fq -- "$n" "$code_only"; then
    name_hits=$((name_hits + 1))
    echo "      tool name '$n' appears literally in the accept script's code"
  fi
done <<< "$independent_names"
verb_hits="$(grep -cE '(^|[^a-z])(gem|npm|pip|pipx|apt-get|apk|brew|cargo|dpkg|snap|rustup|dotnet|go|yum|pacman|choco|winget|nix|conda|yarn|pnpm)[[:space:]]+(install|add|tool|get|-i)' "$code_only")"
if (( code_lines > 0 )) && (( independent_count > 0 )) && (( name_hits == 0 )) && (( verb_hits == 0 )); then
  echo "PASS  no-enumeration-in-source  [oracle]  ${code_lines} code lines scanned; 0/${independent_count} derived tool names and 0 install-verb patterns present"
else
  echo "FAIL  no-enumeration-in-source  [oracle]  code_lines=$code_lines names=$independent_count tool-name hits=$name_hits install-verb hits=$verb_hits"
  failed=$((failed + 1))
fi

echo "---"
echo "$total checks, $((total - failed)) passed, $failed failed"
[[ $failed -eq 0 ]]
