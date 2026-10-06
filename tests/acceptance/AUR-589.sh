#!/usr/bin/env bash
set -euo pipefail

# AUR-589 -- historical evidence without rot.
#
# Every acceptance of a card already `done` ends in exactly one measured
# state: green (exit 0) or explicitly retired (exit 69 printing
# `AUR-NNN/retired: <product reason>`). The expected state of each card is the
# table in docs/specs/AUR-589.md, the single source both this program and the
# nightly workflow read:
#
#   | AUR-NNN | 0    | <what it proves / what was fixed> |
#   | AUR-NNN | 69   | <why the product it proved is gone> |
#   | AUR-NNN | fora | <why it is out of this run> |
#
# `fora` is limited to the fixed set in `allowed_outside` below, so a red
# acceptance cannot be parked there by editing the table.
#
# SELECTORS
#   AC-001          runs every 0/69 row of the table and requires the exit the
#                   table states (AUR589_SHARD=i/n picks one shard,
#                   AUR589_JOBS=k runs k acceptances at a time; default 1/1, 1)
#   AC-003          no acceptance copies an enumerated list of internal/
#                   packages into its staging root (grep guard)
#   AC-001-MUT-001  (named after the card's MUT-002) a retired stub copied to a
#                   scratch dir and changed to exit 0 is rejected by AC-001
#   AC-003-MUT-001  (the card's MUT-001) an enumerated copy reintroduced in a
#                   scratch copy of a fixed acceptance is rejected by AC-003
#   MUT-001/MUT-002 aliases of AC-003-MUT-001 / AC-001-MUT-001 (the card's IDs)
#   coverage        table/script consistency (every retired script is a 69
#                   row; every 69 row's script says retired; with .board
#                   present, a done card without a row runs in AC-001
#                   expecting 0)
#   all             coverage, AC-003, both mutations and AC-001 over a fixed
#                   sample (fits the sealed 600 s budget)
#   full            coverage, AC-003 and AC-001 over the whole table
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md): 0 pass, 1 behavioral
# red, 64 unknown selector, 79 infrastructure.

export LC_ALL=C
readonly card='AUR-589'
selector="${1:-all}"

case "$selector" in
  all|full|AC-001|AC-003|AC-001-MUT-001|AC-003-MUT-001|MUT-001|MUT-002|coverage|one) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
self="$repo_root/tests/acceptance/AUR-589.sh"
spec="$repo_root/docs/specs/AUR-589.md"
acc_dir="$repo_root/tests/acceptance"
[[ -f "$spec" ]] || infra missing-spec
[[ -d "$acc_dir" ]] || infra missing-acceptance-dir

# Cards this run does not execute, each for a reason stated in the spec:
# AUR-448, 458, 491 and 547 belong to AUR-590 (measured defects and the sealed
# image repin); AUR-534's AC-004 only runs inside the sealed image, whose baked
# /opt/aurum-a006 files it compares (AUR-590 repins that image); AUR-436 and
# AUR-460 are open as their own cards (a product behavior shared with AUR-448,
# and a fake gateway response in tests/e2e outside this card's paths).
readonly -a allowed_outside=(AUR-436 AUR-448 AUR-458 AUR-460 AUR-491 AUR-534 AUR-547)

# Fixed sample for `all`: retired stubs of each retirement class plus light
# green acceptances that were repaired by this card.
readonly -a sample=(AUR-001 AUR-308 AUR-359 AUR-403 AUR-437 AUR-498)

# table_rows prints "CARD EXPECT" for each row of the spec table.
table_rows() {
  awk -F'|' '/^\| AUR-[0-9]+ +\| +(0|69|fora) +\|/ {
    c=$2; e=$3; gsub(/ /, "", c); gsub(/ /, "", e); print c, e }' "$spec"
}

expect_of() { table_rows | awk -v c="$1" '$1 == c { print $2 }'; }

# check_one DIR CARD EXPECT: runs DIR/CARD.sh (selector all, AC-001 when `all`
# is unknown to it) and prints one result line. Returns 0 only when the exit
# matches EXPECT; a 69 must also name its retirement reason.
check_one() {
  local dir="$1" c="$2" want="$3" rc=0 sel=all log
  log="$(mktemp "${TMPDIR:-/tmp}/aur589-$c.XXXXXX")"
  [[ -f "$dir/$c.sh" ]] || { printf '%s\t%s\tmissing\t-\tmissing-script\n' "$c" "$want"; rm -f "$log"; return 1; }
  (cd "$repo_root" && timeout 900 bash "$dir/$c.sh" all) >"$log" 2>&1 || rc=$?
  if (( rc == 64 )); then
    sel=AC-001; rc=0
    (cd "$repo_root" && timeout 900 bash "$dir/$c.sh" AC-001) >"$log" 2>&1 || rc=$?
  fi
  local last verdict=ok
  last="$(awk 'NF { l = $0 } END { print substr(l, 1, 140) }' "$log")"
  case "$want" in
    0)  (( rc == 0 )) || verdict=mismatch ;;
    69) if (( rc != 69 )); then verdict=mismatch
        elif ! grep -Eq "^$c/retired: .{20,}" "$log"; then verdict=retired-without-reason; fi ;;
    *)  verdict=bad-expectation ;;
  esac
  printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$c" "$want" "$rc" "$sel" "$verdict" "$last"
  rm -f "$log"
  [[ "$verdict" == ok ]]
}

# run_rows DIR < "CARD EXPECT" lines: checks each row, AUR589_JOBS at a time.
run_rows() {
  local dir="$1" out rc=0
  out="$(mktemp "${TMPDIR:-/tmp}/aur589-rows.XXXXXX")"
  xargs -r -n2 -P "${AUR589_JOBS:-1}" bash "$self" one "$dir" >"$out" || rc=$?
  sort "$out" >&2
  local bad
  bad="$(awk -F'\t' '$5 != "ok"' "$out" | wc -l)"
  printf '%s/%s/rows:%s bad:%s\n' "$card" "$selector" "$(wc -l <"$out")" "$bad" >&2
  rm -f "$out"
  (( rc == 0 && bad == 0 ))
}

shard_filter() {
  local shard="${AUR589_SHARD:-1/1}" i n
  i="${shard%/*}"; n="${shard#*/}"
  [[ "$i" =~ ^[0-9]+$ && "$n" =~ ^[0-9]+$ ]] && (( i >= 1 && i <= n )) || infra "bad-shard:$shard"
  awk -v i="$i" -v n="$n" '(NR - 1) % n == i - 1'
}

ac001_full() {
  local rows
  rows="$({ table_rows | awk '$2 != "fora"'; unlisted_done; } | shard_filter)"
  [[ -n "$rows" ]] || infra empty-table
  run_rows "$acc_dir" <<<"$rows" || fail rows-mismatch
}

ac001_sample() {
  local c want rows=''
  for c in "${sample[@]}"; do
    want="$(expect_of "$c")"
    [[ "$want" == 0 || "$want" == 69 ]] || infra "sample-not-in-table:$c"
    rows+="$c $want"$'\n'
  done
  run_rows "$acc_dir" <<<"${rows%$'\n'}" || fail rows-mismatch
}

coverage() {
  local c e f
  # The table only parks the fixed set outside the run.
  while read -r c e; do
    [[ "$e" == fora ]] || continue
    [[ " ${allowed_outside[*]} " == *" $c "* ]] || fail "outside-not-allowed:$c"
  done < <(table_rows)
  # A row must name an acceptance that exists, or AC-001 fails late on it.
  while read -r c e; do
    [[ "$e" == fora || -f "$acc_dir/$c.sh" ]] || fail "row-without-script:$c"
  done < <(table_rows)
  # A duplicated row would let one state hide another.
  [[ -z "$(table_rows | awk '{ print $1 }' | sort | uniq -d)" ]] || fail duplicate-row
  # Every retired stub (prints /retired: and ends in `exit 69`) is a 69 row,
  # and every 69 row's script is such a stub.
  for f in "$acc_dir"/AUR-*.sh; do
    c="$(basename "$f" .sh)"
    grep -Fq '/retired: ' "$f" && grep -Fxq 'exit 69' "$f" || continue
    [[ "$(expect_of "$c")" == 69 ]] || fail "retired-script-not-69-row:$c"
  done
  while read -r c e; do
    [[ "$e" == 69 ]] || continue
    { grep -Fq '/retired: ' "$acc_dir/$c.sh" && grep -Fxq 'exit 69' "$acc_dir/$c.sh"; } 2>/dev/null ||
      fail "69-row-script-not-retired:$c"
  done < <(table_rows)
  # Outside the sealed profile the board is present: a done card without a
  # row (one that reached done after this table) runs in AC-001 expecting 0.
  if [[ -d "$repo_root/.board/cards/done" ]]; then
    local extra; extra="$(unlisted_done | awk '{ print $1 }' | tr '\n' ' ')"
    [[ -z "$extra" ]] || printf '%s/%s/unlisted-done-expected-green: %s\n' "$card" "$selector" "$extra" >&2
  else
    printf '%s/%s/board-absent: done list cross-check skipped, table rows still checked\n' "$card" "$selector" >&2
  fi
}

# unlisted_done prints "CARD 0" for every done card on the board without a row.
unlisted_done() {
  [[ -d "$repo_root/.board/cards/done" ]] || return 0
  local f c
  for f in "$repo_root"/.board/cards/done/AUR-*.md; do
    c="$(basename "$f" .md)"
    [[ -n "$(expect_of "$c")" ]] || printf '%s 0\n' "$c"
  done
}

# enumerated_copies DIR prints file:line of every acceptance that copies a named
# internal/<pkg> into its staging root: a `copy`/`cp` call or a
# required_inputs array naming internal/<something>. Copying internal whole
# (`internal` with no sub-path) is the only accepted form.
enumerated_copies() {
  grep -HnE '^[[:space:]]*(copy[A-Za-z_]*|cp)[[:space:]][^#]*internal/[A-Za-z0-9_]|required_inputs=\([^)]*internal/[A-Za-z0-9_]' \
    "$1"/AUR-*.sh || true
}

ac003() {
  local hits
  hits="$(enumerated_copies "$acc_dir")"
  [[ -z "$hits" ]] || { printf '%s\n' "$hits" >&2; fail enumerated-internal-copy; }
}

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a589.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$run_dir"' EXIT INT TERM HUP

# MUT-002 of the card: a retired acceptance made to exit 0 must fail AC-001.
mut_retired_exit0() {
  local c=AUR-308 dir="$run_dir/acc"
  mkdir -p "$dir"
  cp "$acc_dir/$c.sh" "$dir/$c.sh"
  grep -Fxq 'exit 69' "$dir/$c.sh" || infra mutation-anchor-missing
  sed -i 's/^exit 69$/exit 0/' "$dir/$c.sh"
  grep -Fxq 'exit 0' "$dir/$c.sh" || infra mutation-not-applied
  [[ "$(expect_of "$c")" == 69 ]] || infra mutation-row-missing
  if run_rows "$dir" <<<"$c 69" 2>/dev/null; then fail mutation-survived; fi
  printf '%s/AC-001-MUT-001/rejected (retired stub exiting 0 fails AC-001)\n' "$card" >&2
}

# MUT-001 of the card: an enumerated internal/ copy reintroduced into a fixed
# acceptance must fail AC-003.
mut_enumerated_copy() {
  local c=AUR-437 dir="$run_dir/guard"
  mkdir -p "$dir"
  cp "$acc_dir/$c.sh" "$dir/$c.sh"
  local anchor='  copy "$root" cmd internal pkg tests/fixtures/scm/github'
  grep -Fxq "$anchor" "$dir/$c.sh" || infra mutation-anchor-missing
  sed -i 's|^  copy "$root" cmd internal pkg tests/fixtures/scm/github$|  copy "$root" internal/git/githubclient tests/fixtures/scm/github|' "$dir/$c.sh"
  grep -Fxq "$anchor" "$dir/$c.sh" && infra mutation-not-applied
  [[ -z "$(enumerated_copies "$dir")" ]] && fail mutation-survived
  printf '%s/AC-003-MUT-001/rejected (enumerated internal copy fails AC-003)\n' "$card" >&2
}

case "$selector" in
  one)            shift; check_one "$@"; exit $? ;;
  coverage)       coverage ;;
  AC-001)         coverage; ac001_full ;;
  AC-003)         ac003 ;;
  AC-001-MUT-001|MUT-002) mut_retired_exit0 ;;
  AC-003-MUT-001|MUT-001) mut_enumerated_copy ;;
  all)            coverage; ac003; mut_enumerated_copy; mut_retired_exit0; ac001_sample ;;
  full)           coverage; ac003; ac001_full ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
