#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

board_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd "$board_dir/.." && pwd -P)"
states=(backlog ready doing review done blocked-on-owner cancelled)
required_sections=(
  "## Outcome"
  "## Non-goals"
  "## Preconditions"
  "## Postconditions"
  "## Acceptance scenarios"
  "## Public contract"
  "## TDD proof"
  "## Acceptance"
  "## Skeptical mutations"
  "## Security and privacy"
  "## Documentation"
  "## Compatibility, migration, rollback"
  "## Review"
  "## Evidence"
)
failed=0
failure_count=0
max_reported_failures=200
declare -A ids=()
declare -A files=()
declare -A card_states=()
declare -A card_dependencies=()
declare -A card_offices=()
declare -A card_risks=()
declare -A card_titles=()
declare -A card_base_shas=()
declare -A card_spec_digests=()
# The dependency-on-cancelled gate needs, for every cancelled card, the
# successor its cancellation.json declares (or the literal string "null"):
# empty means the cancellation evidence itself failed to validate and no
# override can be honored.
declare -A card_superseded_by=()
# Recomputable card facts the `done` evidence gate re-derives instead of
# trusting: the exact locked acceptance command, the artifact path the card
# promises, the acceptance scenarios that must be observed, and how many
# skeptical mutations exist to choose from.
declare -A card_accept_commands=()
declare -A card_expected_artifacts=()
declare -A card_scenario_ids=()
declare -A card_mutation_counts=()
declare -A visit_state=()
declare -A path_owners=()
declare -A reachability=()
declare -A referenced_requirements=()
declare -A referenced_controls=()
# `read_paths` widens a card's read surface beyond what it owns. Each declared
# entry is recorded as a (card, path) claim during the card pass and adjudicated
# after the loop, when every owned path in the board is finally known.
declare -a read_path_claim_cards=()
declare -a read_path_claim_paths=()
declare -A owned_path_ancestors=()
declare -A read_path_resolution=()
# Role nonces are compared inside a single evidence bundle: a nonce repeated
# across reviewer-a, reviewer-b or the skeptic is one sealed run relabelled.
declare -A bundle_role_nonces=()
# Specification-by-slogan detectors. A card that reuses another card's Given,
# When, Then, Green or Non-goal verbatim cannot distinguish its own promised
# behavior from its neighbour's, so the acceptance proof stops being falsifiable.
# Owners are accumulated per normalized value and adjudicated after the card loop.
declare -A scenario_given_owners=()
declare -A scenario_when_owners=()
declare -A scenario_then_owners=()
declare -A tdd_green_owners=()
declare -A non_goal_owners=()
# Ratcheted (ratio: ok to already collide, never ok to collide *more*): the
# `And` bullet of each scenario, and Non-goal bullets beyond the first.
declare -A scenario_and_owners=()
declare -A non_goal_extra_owners=()
# --- second reader ------------------------------------------------------------
# `.board/bin/oci-run` runs exactly one program -- the card's own acceptance
# script -- inside a pinned Alpine/BusyBox image that has no language toolchain,
# and it materializes only that card's `paths`/`read_paths`. A `## TDD proof`
# line that cites `tests/integration/AUR-0NN_test.go::TestX` was therefore never
# executed by any gate: the citation was prose. Three independent reviewers
# reproduced the consequence on three cards. Everything below makes the cited
# test a thing that ran, and makes the record of that run recomputable rather
# than asserted: every fact the observation JSON states is re-derived here from
# the raw captured bytes, and a fact that disagrees with the bytes loses.
declare -A card_layer_specs=()
declare -A second_reader_legacy=()
declare -A second_reader_exempt=()
declare -a second_reader_debt=()
declare -a second_reader_exempt_debt=()
declare -a second_reader_skipped=()
second_reader_executed=0
second_reader_matched=0
readonly second_reader_control_selector='AURUM-SECOND-READER-CONTROL'
readonly second_reader_legacy_file='.board/second-reader-legacy.tsv'
readonly second_reader_exempt_file='.board/second-reader-exempt.tsv'
readonly second_reader_legacy_dir='.board/tests/second-reader-legacy'
readonly second_reader_log_schema='=== second-reader-log v1'
readonly second_reader_readme='.board/README.md'
readonly second_reader_go_lock='.board/locks/oci/second-reader-go-v1.lock.json'
readonly second_reader_shell_lock='.board/locks/oci/second-reader-shell-v1.lock.json'
# The cutover, frozen in the validator rather than in the data it judges. The
# ratchet rules below refuse an entry that went stale; this list refuses an
# entry that was never part of the migration in the first place, which is the
# one thing a card could otherwise do in the same commit that moves it to
# `done`. Growing either registry is a change to THIS file -- a reviewed code
# change with its own mutant -- and can never be a data-only edit.
readonly second_reader_legacy_frozen='
AUR-020/Integration
AUR-021/Integration
AUR-359/Contract
AUR-359/E2E
AUR-359/Integration
AUR-360/Contract
AUR-360/E2E
AUR-360/Integration
AUR-362/Contract
AUR-362/E2E
AUR-362/Integration
AUR-363/Contract
AUR-363/E2E
AUR-363/Integration
'
readonly second_reader_exempt_frozen='
AUR-016
AUR-017
'
# `recompute` is the floor and is never bypassable: the structural and raw-log
# recomputation gates below always run. `exec` is the DEFAULT and additionally
# re-runs the second reader through `.board/bin/second-reader --verify`.
#
# Law 4 and law 11 together decide what happens when `exec` cannot run: a
# missing OCI engine is infrastructure, so it is never behavioral red -- and it
# is never a pass either. Every layer that was not re-executed is named on
# stderr as it is skipped, counted, and summarized, and the run ends
# INCONCLUSIVE (exit 3). A `done` transition may only be authorized by a run
# that exited 0, which is exactly the run in which every layer was re-executed.
second_reader_mode="${AURUM_SECOND_READER:-exec}"
[[ "$second_reader_mode" =~ ^(recompute|exec)$ ]] ||
  second_reader_mode='exec'

fail() {
  failed=1
  failure_count=$((failure_count + 1))
  if (( failure_count <= max_reported_failures )); then
    printf 'board error: %s\n' "$*" >&2
  elif (( failure_count == max_reported_failures + 1 )); then
    printf 'board error: further errors suppressed (limit=%d)\n' "$max_reported_failures" >&2
  fi
}

finish_failures() {
  if (( failed != 0 )); then
    printf 'board invalid: %d error(s)\n' "$failure_count" >&2
    exit 1
  fi
}

trim() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

# The four card parsers below ran one `awk` subprocess PER CALL, which is the
# hot path of the whole validator: the 423-card loop previously forked tens of
# thousands of `awk`/`grep`/`sed` processes (~157s). They are now pure-bash
# against an in-memory copy of the current card, loaded exactly once per card.
# Semantics are preserved byte-for-byte: frontmatter is parsed only between the
# first two `---` delimiters (a body line cannot spoof a required field), a
# section ends at the next `## ` or `### ` heading, and counts/exit codes
# match the awk originals.
card_lines=()

card_cache_load() {
  local file="$1"
  mapfile -t card_lines < "$file"
}

# Front matter is deliberately parsed only between the first two delimiters.
# A body line cannot spoof a required field. Pure-bash mirror of the original
# awk (which exited 2 for a missing opening delimiter and 3 for a field that
# did not occur exactly once).
frontmatter_value() {
  local card="$1"
  local key="$2"
  local line in_fm=0 value count=0
  if [[ "${card_cache_file:-}" != "$card" ]]; then
    card_cache_load "$card"
    card_cache_file="$card"
  fi
  if [[ "${#card_lines[@]}" -eq 0 || "${card_lines[0]}" != "---" ]]; then
    return 2
  fi
  for line in "${card_lines[@]}"; do
    if (( in_fm == 0 )); then
      [[ "$line" == "---" ]] && in_fm=1
      continue
    fi
    [[ "$line" == "---" ]] && break
    if [[ "$line" == "$key:"* ]]; then
      value="${line:${#key} + 1}"
      value="${value#"${value%%[![:space:]]*}"}"
      count=$((count + 1))
    fi
  done
  (( count == 1 )) || return 3
  printf '%s' "$value"
}

frontmatter_count() {
  local card="$1"
  local key="$2"
  local line in_fm=0 count=0
  if [[ "${card_cache_file:-}" != "$card" ]]; then
    card_cache_load "$card"
    card_cache_file="$card"
  fi
  if [[ "${#card_lines[@]}" -eq 0 || "${card_lines[0]}" != "---" ]]; then
    return 2
  fi
  for line in "${card_lines[@]}"; do
    if (( in_fm == 0 )); then
      [[ "$line" == "---" ]] && in_fm=1
      continue
    fi
    [[ "$line" == "---" ]] && break
    [[ "$line" == "$key:"* ]] && count=$((count + 1))
  done
  printf '%s' "$count"
}

section_body() {
  local card="$1"
  local section="$2"
  local line found=0
  local -a out=()
  if [[ "${card_cache_file:-}" != "$card" ]]; then
    card_cache_load "$card"
    card_cache_file="$card"
  fi
  for line in "${card_lines[@]}"; do
    if (( found )); then
      [[ "$line" == '## '* ]] && break
      out+=("$line")
    else
      [[ "$line" == "$section" ]] && found=1
    fi
  done
  printf '%s\n' "${out[@]}"
}

subsection_body() {
  local card="$1"
  local prefix="$2"
  local line found=0
  local -a out=()
  if [[ "${card_cache_file:-}" != "$card" ]]; then
    card_cache_load "$card"
    card_cache_file="$card"
  fi
  for line in "${card_lines[@]}"; do
    if (( found )); then
      [[ "$line" == '### '* ]] && break
      [[ "$line" == '## '* ]] && break
      out+=("$line")
    else
      [[ "$line" == "$prefix"* ]] && found=1
    fi
  done
  printf '%s\n' "${out[@]}"
}

# --- Pure-bash line scanners (no subprocess) --------------------------------
# The card loop used to run ~40 `grep`/`sed` on `<<<` here-strings per card,
# each a fork+exec (~5-8ms), which dominated the ~140s loop. These helpers do
# the same scans against an in-memory string with bash builtins only. Semantics
# mirror the GNU tools they replace: `-E` regexes (ERE), `-F` fixed strings,
# `-c` line counts, and first-match extraction with head-like behavior.

# Count lines in "$text" matching an ERE (grep -Ec ... || true).
count_matching_lines() {
  local re="$1"
  local text="$2"
  local line n=0
  while IFS= read -r line; do
    [[ "$line" =~ $re ]] && n=$((n + 1))
  done <<< "$text"
  printf '%s' "$n"
}

# Return 0 if any line matches (grep -Eq), 1 otherwise.
has_matching_line() {
  local re="$1"
  local text="$2"
  local line
  while IFS= read -r line; do
    [[ "$line" =~ $re ]] && return 0
  done <<< "$text"
  return 1
}

# Count lines equal to a fixed string (grep -Fxc "$needle").
count_fixed_lines() {
  local needle="$1"
  local text="$2"
  local line n=0
  while IFS= read -r line; do
    [[ "$line" == "$needle" ]] && n=$((n + 1))
  done <<< "$text"
  printf '%s' "$n"
}

# Extract first line matching `prefix RE` and strip the prefix (sed -n ... p |
# head -1). Emits the match group captured by the caller-provided ERE in $3.
extract_first() {
  local prefix="$1"
  local text="$2"
  local re="$3"
  local line
  while IFS= read -r line; do
    [[ "$line" == "$prefix"* ]] || continue
    if [[ "$line" =~ $re ]]; then
      printf '%s' "${BASH_REMATCH[1]}"
      return 0
    fi
  done <<< "$text"
  return 1
}

# Collect every captured group for lines matching `prefix RE` into the global
# array _extracted (mapfile -t < <(sed -n ...)).
declare -a _extracted=()
extract_all() {
  local prefix="$1"
  local text="$2"
  local re="$3"
  local line
  _extracted=()
  while IFS= read -r line; do
    [[ "$line" == "$prefix"* ]] || continue
    if [[ "$line" =~ $re ]]; then
      _extracted+=("${BASH_REMATCH[1]}")
    fi
  done <<< "$text"
}

# First line matching ERE, whole line emitted (grep -E ... | head -1).
first_matching_line() {
  local re="$1"
  local text="$2"
  local line
  while IFS= read -r line; do
    [[ "$line" =~ $re ]] && { printf '%s' "$line"; return 0; }
  done <<< "$text"
  return 1
}

# $2 (self_id, optional) is the AUR-NNN id of the card the text came from.
# When given, only occurrences of THAT card's own id are folded to CARDREF --
# e.g. `tests/specs/AUR-015/cases.yaml` in AUR-015's own Given -- so that two
# cards which differ only by mechanically citing themselves still collide.
# A card's text citing a *different* card's id (a dependency manifest, a
# sibling path) is real distinguishing content and must not be blanked: doing
# that indiscriminately for every `AUR-[0-9]{3}` in the text, regardless of
# whose id it is, folds genuinely different preconditions (different
# dependency sets) into a false collision across unrelated cards.
normalize_spec_text() {
  local text="$1"
  local self_id="${2:-}"
  # The self-id substitution must run while the id is still upper-case (the
  # only case it ever appears in card prose), *before* the lowercasing pass
  # below. Reordering these silently turns the substitution into dead code:
  # the pattern can never match already-lowered text, so a card whose only
  # textual delta from a sibling is its own cited id stops colliding --
  # exactly the failure this substitution exists to catch.
  if [[ -n "$self_id" ]]; then
    text="${text//$self_id/CARDREF}"
  fi
  text="${text,,}"
  text="${text//\`/}"
  text="${text//\*/}"
  text="${text//_/}"
  local -a spec_words=()
  read -ra spec_words <<< "$text" || true
  text="${spec_words[*]}"
  while [[ -n "$text" ]]; do
    c="${text: -1}"
    case "$c" in
      [.]|[,]) ;;
      ";") ;;
      *) break ;;
    esac
    text="${text%?}"
  done
  printf '%s' "$text"
}

# $4 (key_prefix, optional) scopes the collision bucket -- e.g. by bullet
# position -- without affecting the length gate below, which is judged on the
# actual content, not on a synthetic prefix. $5 (self_id, optional) is passed
# through to normalize_spec_text unchanged.
record_spec_owner() {
  local -n owners="$1"
  local value="$2"
  local id="$3"
  local key_prefix="${4:-}"
  local self_id="${5:-}"
  local key
  [[ -n "$value" ]] || return 0
  key="$(normalize_spec_text "$value" "$self_id")"
  # Too short to carry a card-specific claim; length is already policed elsewhere.
  (( ${#key} >= 16 )) || return 0
  key="${key_prefix}${key}"
  if [[ -n "${owners[$key]+x}" ]]; then
    owners[$key]="${owners[$key]} $id"
  else
    owners[$key]="$id"
  fi
}

# --- Spec-collision ratchet -------------------------------------------------
# Every checked field (Given, When, Then, Green, Non-goal bullet 1, And,
# Non-goal bullets 2+) is judged the same way: a pairwise verbatim collision,
# after normalize_spec_text (self-id folded to CARDREF, case/markup/whitespace
# collapsed), against a committed baseline of already-known collisions.
#
# For Given/When/Then/Green/Non-goal#1 that baseline is currently EMPTY, so in
# practice they stay zero-tolerance exactly as before: any collision at all is
# a NEW collision and fails. Fixing the normalize_spec_text ordering bug (the
# self-id substitution was dead code -- it ran after lowercasing, so it could
# never match) makes the Given check live for the first time, and that
# surfaces real pre-existing debt: several card families whose Given differs
# from a sibling's only by each card mechanically citing its own id in a file
# path (e.g. `tests/characterization/legacy/script-files/AUR-365/cases.tsv`
# vs the identical path for AUR-366..376). That debt is recorded in the
# baseline exactly like the And/Non-goal debt below, not hard-coded around.
#
# `And` and Non-goal bullets 2+ were never checked at all before this patch --
# 204 and 266 pre-existing collisions respectively (see
# .board/tests/spec-collision-baseline.txt for the measured set, and BUG 2 in
# /tmp/aurum-reviews/GATE-SPEC-COLLISION.md for how they were found). Turning
# those on as hard failures today would redden the whole board, so they enter
# through the same ratchet as pre-existing debt.
#
# In every field the ratchet only shrinks: a NEW collision not already in the
# baseline fails the build, and a baseline entry that no longer collides
# (because a card's text changed) also fails the build, forcing the stale
# entry to be removed rather than left to rot. The baseline can never grow by
# editing it forward; it only shrinks as collisions get resolved. Regenerate
# it with `BOARD_SPEC_BASELINE_DUMP=1 ./.board/validate.sh` after resolving or
# introducing debt -- never by hand-editing the file.
spec_collision_baseline="$board_dir/tests/spec-collision-baseline.txt"

# Emits one `id1 id2` line (id1 < id2 byte-wise) per pairwise collision found
# in the given owners map, decomposing any >2-way collision into all its pairs.
compute_ratchet_pairs() {
  local -n owners="$1"
  local key list
  local -a members
  local i j a b
  for key in "${!owners[@]}"; do
    list="${owners[$key]}"
    read -ra members <<< "$list"
    (( ${#members[@]} > 1 )) || continue
    for (( i = 0; i < ${#members[@]}; i++ )); do
      for (( j = i + 1; j < ${#members[@]}; j++ )); do
        a="${members[$i]}"
        b="${members[$j]}"
        if [[ "$a" < "$b" ]]; then
          printf '%s %s\n' "$a" "$b"
        else
          printf '%s %s\n' "$b" "$a"
        fi
      done
    done
  done
}

check_ratcheted_collisions() {
  local owners_name="$1"
  local field_label="$2"
  local baseline_file="$3"
  local current_pairs baseline_pairs line
  current_pairs="$(compute_ratchet_pairs "$owners_name" | sort -u)"
  if [[ -f "$baseline_file" ]]; then
    baseline_pairs="$(awk -v f="$field_label" '$1 == f { print $2, $3 }' "$baseline_file" | sort -u)"
  else
    baseline_pairs=""
  fi
  # Both streams are already sorted -u under the script-wide LC_ALL=C, so a
  # linear comm merge replaces what would otherwise be an O(current *
  # baseline) grep-per-line scan -- material at the baseline's current size
  # (13k+ tolerated pairs and growing with every ratcheted field).
  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    fail "spec-collision ratchet: new $field_label collision not recorded in the committed baseline ($spec_collision_baseline): $line"
  done < <(comm -23 <(printf '%s\n' "$current_pairs") <(printf '%s\n' "$baseline_pairs"))
  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    fail "spec-collision ratchet: baseline entry for $field_label no longer collides; the ratchet only shrinks, remove the stale entry: $line"
  done < <(comm -13 <(printf '%s\n' "$current_pairs") <(printf '%s\n' "$baseline_pairs"))
}

is_generic_text() {
  local original="$1"
  local text
  if [[ "$original" =~ (^|[^A-Za-z])(TBD|TODO:|FIXME)([^A-Za-z]|$) ]]; then
    return 0
  fi
  text="${original,,}"
  text="${text//$'\n'/ }"
  [[ "$text" =~ (placeholder|lorem[[:space:]]+ipsum|(^|[^a-z])noop([^a-z]|$)|replace[[:space:]]+every|one[[:space:]]+externally[[:space:]]+observable[[:space:]]+outcome|implementar[[:space:]]+o[[:space:]]+m[ií]nimo|ainda[[:space:]]+n[aã]o[[:space:]]+existe[[:space:]]+ou[[:space:]]+n[aã]o[[:space:]]+[eé][[:space:]]+imposto|unit[[:space:]]+e[[:space:]]+contract[[:space:]]+sempre|state[[:space:]]+which[[:space:]]+layers[[:space:]]+apply|exact[[:space:]]+failing[[:space:]]+test|exact[[:space:]]+reversible[[:space:]]+change|path,[[:space:]]+schema,[[:space:]]+endpoint|fazer[[:space:]]+funcionar|comportamento[[:space:]]+desejado|delet(e|ar)[[:space:]]+(the[[:space:]]+)?test|quebrar[[:space:]]+(a[[:space:]]+)?compila|break(ing)?[[:space:]]+compil|infrastructure[[:space:]]+failure|falha[[:space:]]+de[[:space:]]+infra) ]]
}

require_meaningful_section() {
  local card="$1"
  local section="$2"
  local body compact
  body="$(section_body "$card" "$section")"
  compact="${body//[[:space:]#*\`_-]/}"
  if (( ${#compact} < 24 )); then
    fail "$card: $section must contain a specific, falsifiable statement"
  elif is_generic_text "$body"; then
    fail "$card: $section contains generic or placeholder language"
  fi
}

parse_list() {
  local value="$1"
  [[ "$value" =~ ^\[(.*)\]$ ]] || return 1
  value="${BASH_REMATCH[1]}"
  [[ -n "$value" ]] || return 0
  local old_ifs="$IFS"
  IFS=','
  read -ra parsed_list <<< "$value"
  IFS="$old_ifs"
  local item
  for item in "${parsed_list[@]}"; do
    item="$(trim "$item")"
    [[ -n "$item" ]] || return 1
    printf '%s\n' "$item"
  done
}

safe_repo_path() {
  local path="$1"
  local without_spaces
  [[ -n "$path" && "$path" != "." && "$path" != "/" ]] || return 1
  [[ "$path" != ' '* && "$path" != *' ' ]] || return 1
  [[ "$path" != *$'\t'* && "$path" != *$'\r'* && "$path" != *$'\n'* ]] || return 1
  [[ "$path" != /* && "$path" != ~* && "$path" != *\\* ]] || return 1
  without_spaces="${path// /}"
  [[ "$without_spaces" =~ ^[A-Za-z0-9._][A-Za-z0-9._/-]*$ ]] || return 1
  [[ "$path" != */ && "$path" != *//* ]] || return 1
  [[ "/$path/" != */../* && "/$path/" != */./* ]] || return 1
  [[ "$path" != *'$'* && "$path" != *'`'* && "$path" != *'*'* && "$path" != *'?'* && "$path" != *'['* ]] || return 1
}

canonical_repo_path_list() {
  local value="$1"
  local allow_empty="${2:-false}"
  local body remainder
  [[ "$value" =~ ^\[(.*)\]$ ]] || return 1
  body="${BASH_REMATCH[1]}"
  if [[ -z "$body" ]]; then
    [[ "$allow_empty" == true ]]
    return
  fi
  [[ "$body" != ' '* && "$body" != *' ' ]] || return 1
  [[ "$body" != *' ,'* && "$body" != *',  '* ]] || return 1
  # Every comma is the list delimiter and is followed by exactly one space.
  remainder="$body"
  while [[ "$remainder" == *,* ]]; do
    remainder="${remainder#*,}"
    [[ "$remainder" == ' '* && "$remainder" != '  '* ]] || return 1
    remainder="${remainder# }"
  done
  [[ "$body" != *$'\t'* && "$body" != *$'\r'* && "$body" != *$'\n'* ]]
}

paths_overlap() {
  local first="$1"
  local second="$2"
  [[ "$first" == "$second" || "$first" == "$second/"* || "$second" == "$first/"* ]]
}

# Ownership is directional. A card owning `tests/acceptance` owns a file below
# it; a card owning `tests/acceptance/AUR-001.sh/subdir` does not own the
# ancestor `tests/acceptance/AUR-001.sh`.
path_is_within() {
  local child="$1"
  local owner="$2"
  [[ "$child" == "$owner" || "$child" == "$owner/"* ]]
}

validate_owned_test_path() {
  local card="$1"
  local label="$2"
  local test_path="$3"
  shift 3
  local owner owned=0

  safe_repo_path "$test_path" || {
    fail "$card: $label test path is unsafe or not repository-relative: $test_path"
    return
  }
  [[ "$test_path" == tests/* || "$test_path" == *_test.go ]] ||
    fail "$card: $label test artifact must be under tests/ or name an owned _test.go file: $test_path"
  # RULE:second-reader-test-owned
  for owner in "$@"; do
    if path_is_within "$test_path" "$owner"; then
      owned=1
      break
    fi
  done
  (( owned == 1 )) || fail "$card: $label TDD test is outside owned paths: $test_path"
  # /RULE:second-reader-test-owned
}

# The engine that can execute a citation, derived from the citation itself. A
# `_test.go` file is a Go test binary; a `.sh` file under `tests/` is an
# acceptance program dispatched by selector argument. Anything else names an
# artifact no second reader can run, and a card cannot promise a test nobody
# can execute.
second_reader_engine() {
  case "$1" in
    *_test.go) printf 'go' ;;
    *.sh) printf 'shell' ;;
    *) printf '' ;;
  esac
}

# A selector is card-supplied text and its charset allows `.`, `-` and `/`. Every
# place a selector reaches a regular expression it goes through here first, so
# `TestA.B` can never match `TestAxB`: a metacharacter that is not escaped is a
# narrow fail-open, and fail-open is the one direction this gate may not take.
ere_escape() {
  printf '%s' "$1" | sed -e 's/[][\.^$*+?(){}|\/-]/\\&/g'
}

# The digest-pinned image a second-reader run of this engine must have used. The
# lock is the authority; the `=== image:` frame the runner wrote is data, and
# data that disagrees with the lock does not describe a run this board pins.
second_reader_lock_image() {
  local engine="$1" lock_rel value
  case "$engine" in
    go) lock_rel="$second_reader_go_lock" ;;
    shell) lock_rel="$second_reader_shell_lock" ;;
    *) printf ''; return 1 ;;
  esac
  [[ -f "$repo_root/$lock_rel" && ! -L "$repo_root/$lock_rel" ]] || { printf ''; return 1; }
  value="$(sed -n 's/.*"image"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$repo_root/$lock_rel" | head -n 1)"
  [[ "$value" =~ ^[a-z0-9][a-z0-9._/-]*@sha256:[0-9a-f]{64}$ ]] || { printf ''; return 1; }
  printf '%s' "$value"
}

# The canonical command for a layer citation, rebuilt here from the card alone.
# `.board/bin/second-reader` builds the identical string, and the raw log must
# record it byte for byte, so a recorded run can never have executed a selector
# other than the one the card declares.
second_reader_command() {
  local engine="$1" test_path="$2" selector="$3"
  case "$engine" in
    go) printf 'go test ./%s/... -run ^%s$ -v -count=1' "$(dirname "$test_path")" "$selector" ;;
    shell) printf 'bash %s %s' "$test_path" "$selector" ;;
    *) printf '' ;;
  esac
}

# Does the cited artifact itself define the cited selector? For Go the selector
# may name a subtest (`TestX/case`), and only the top function is declared in
# the file. For a shell acceptance program the selector is a dispatcher arm, so
# the literal must appear in the script.
second_reader_file_defines_selector() {
  local engine="$1" test_path="$2" selector="$3" top
  [[ -f "$repo_root/$test_path" && ! -L "$repo_root/$test_path" ]] || return 1
  case "$engine" in
    go)
      top="$(ere_escape "${selector%%/*}")"
      grep -Eq "^func[[:space:]]+${top}\(" "$repo_root/$test_path"
      ;;
    shell)
      grep -Fq -- "$selector" "$repo_root/$test_path"
      ;;
    *) return 1 ;;
  esac
}

validate_tdd_layer() {
  local card="$1"
  local id="$2"
  local state="$3"
  local layer="$4"
  local tdd="$5"
  shift 5
  local line count test_path selector reason engine

  count="$(count_matching_lines "^- $layer: .+" "$tdd" || true)"
  [[ "$count" -eq 1 ]] || {
    fail "$card: TDD proof must specify $layer exactly once"
    return
  }
  line="$(first_matching_line "^- $layer: .+" "$tdd" || true)"
  if [[ "$line" =~ ^-[[:space:]]$layer:[[:space:]]not-applicable:[[:space:]](.{16,})$ ]]; then
    reason="${BASH_REMATCH[1]}"
    [[ "$reason" != *'`'* ]] || fail "$card: $layer not-applicable must not cite a test artifact"
    return
  fi
  if [[ "$line" =~ ^-[[:space:]]$layer:[[:space:]]\`(.+)::([A-Za-z0-9_./:-]+)\`\.$ ]]; then
    test_path="${BASH_REMATCH[1]}"
    selector="${BASH_REMATCH[2]}"
    [[ -n "$selector" ]] || fail "$card: $layer test selector is empty"
    validate_owned_test_path "$card" "$layer" "$test_path" "$@"
    engine="$(second_reader_engine "$test_path")"
    # RULE:second-reader-runnable-kind
    # A citation nobody can execute is prose with backticks around it. The rule
    # binds at the boundary a card crosses on the way to evidence, so planned
    # `backlog` work may still describe an artifact that does not exist yet.
    if [[ -z "$engine" && "$state" =~ ^(review|done)$ ]]; then
      fail "$card: $layer cites an artifact no second-reader engine can execute (expected *_test.go or *.sh): $test_path"
    fi
    # /RULE:second-reader-runnable-kind
    # RULE:second-reader-file-exists
    # `doing` is exempt on purpose and the asymmetry is the TDD protocol, not an
    # oversight: the acceptance `Test:` is sealed by an isolated test designer
    # BEFORE a builder starts, which is why it must already exist at `doing`,
    # while the Unit/Contract/Integration/E2E artifacts are the builder's own
    # output. From `review` on, a cited layer test is a delivered artifact.
    if [[ "$state" =~ ^(review|done)$ ]]; then
      if [[ ! -e "$repo_root/$test_path" && ! -L "$repo_root/$test_path" ]]; then
        fail "$card: $state card cites $layer test $test_path, which does not exist"
      elif [[ ! -f "$repo_root/$test_path" || -L "$repo_root/$test_path" ]]; then
        fail "$card: $layer test must be a regular non-symlink file: $test_path"
      elif [[ ! -s "$repo_root/$test_path" ]]; then
        fail "$card: $layer test exists but is empty: $test_path"
      fi
    fi
    # /RULE:second-reader-file-exists
    # RULE:second-reader-selector-defined
    # Ownership, existence, non-emptiness and the sha256 of the cited file say
    # nothing about whether that file DEFINES the selector. The Go command is
    # package-scoped (`go test ./<dir>/... -run ^SEL$`), so an owned `_test.go`
    # containing no test at all, citing a NEIGHBOURING card's selector from the
    # same package, satisfied every other rule and produced a green log with a
    # real `--- PASS:` line in it. The binding the log cannot supply is
    # recomputed here, from the cited bytes: the file must define the selector.
    if [[ "$state" =~ ^(review|done)$ && -n "$engine" && -s "$repo_root/$test_path" && ! -L "$repo_root/$test_path" ]] &&
      ! second_reader_file_defines_selector "$engine" "$test_path" "$selector"; then
      fail "$card: $layer cites $test_path, which does not define the selector $selector; the log would bind a test this file never carried"
    fi
    # /RULE:second-reader-selector-defined
    card_layer_specs[$id]+="$layer"$'\t'"$test_path"$'\t'"$selector"$'\t'"$engine"$'\t'"$line"$'\n'
    return
  fi
  fail "$card: $layer must be exactly a backtick-delimited path::selector or not-applicable with a concrete reason"
}

# Re-derives, from the raw captured bytes alone, every fact a second-reader
# observation claims. Nothing here reads the observation JSON: the log is the
# record, the JSON is a summary of it, and a summary that disagrees with the
# record loses. `second_reader_matched` carries the recomputed match count back
# to the caller.
#
# The capture regions are written with a `| ` prefix on every line by
# `.board/bin/second-reader`. That prefix is what makes the frames unforgeable:
# a test that prints `=== exit: 0` lands inside the region as `| === exit: 0`
# and can never become a frame line, and any unprefixed line inside a region is
# rejected outright.
second_reader_verify_log() {
  local card="$1" layer="$2" test_path="$3" selector="$4" engine="$5"
  local log_file="$6" label="$7"
  local frame expected_command expected_file_digest region exit_line
  local control_command control_exit_line matched expected_image selector_ere

  second_reader_matched=0
  [[ -f "$log_file" && ! -L "$log_file" ]] || {
    fail "$label: the raw second-reader log is missing or not a regular file"
    return 1
  }
  [[ "$(head -n 1 "$log_file")" == "$second_reader_log_schema" ]] || {
    fail "$label: the raw second-reader log does not carry the canonical schema header"
    return 1
  }
  expected_file_digest="sha256:$(sha256sum "$repo_root/$test_path" | awk '{print $1}')"
  expected_command="$(second_reader_command "$engine" "$test_path" "$selector")"
  [[ -n "$expected_command" ]] || {
    fail "$label: no second-reader engine can execute $test_path"
    return 1
  }
  # RULE:second-reader-image-pinned
  # `=== image:` is written by the runner, so on its own it is a claim about
  # which image ran. Recomputing it from the lock is what turns it into a
  # binding: a record produced in some other image no longer describes a run
  # this board pinned, whoever wrote the frame.
  expected_image="$(second_reader_lock_image "$engine" || true)"
  [[ -n "$expected_image" ]] || {
    fail "$label: no digest-pinned second-reader image lock is readable for the $engine engine"
    return 1
  }
  grep -Fqx -- "=== image: $expected_image" "$log_file" || {
    fail "$label: the recorded run does not name the locked $engine image; missing frame: === image: $expected_image"
    return 1
  }
  # /RULE:second-reader-image-pinned
  # RULE:second-reader-log-binds-declaration
  for frame in \
    "=== card: $card" \
    "=== layer: $layer" \
    "=== engine: $engine" \
    "=== test-path: $test_path" \
    "=== selector: $selector" \
    "=== command: $expected_command"; do
    grep -Fqx -- "$frame" "$log_file" || {
      fail "$label: the recorded run does not bind the card's declaration; missing frame: $frame"
      return 1
    }
  done
  # A recorded run expires when the artifact it read changes. This is what stops
  # a pasted or stale observation from covering new bytes.
  grep -Fqx -- "=== test-file-sha256: $expected_file_digest" "$log_file" || {
    fail "$label: the recorded run read a different $test_path than the tree now carries"
    return 1
  }
  # /RULE:second-reader-log-binds-declaration

  region="$(awk '/^=== output-begin$/ { inside=1; next } /^=== output-end$/ { exit } inside { print }' "$log_file")"
  # RULE:second-reader-frame-integrity
  # Two halves of one property. The prefix check refuses an unprefixed line
  # inside a region; the cardinality check refuses a SECOND region boundary or
  # exit frame, which is how a program that prints `=== output-end` would
  # otherwise truncate the region early and append a friendlier exit code after
  # its own real one.
  if [[ -n "$region" ]] && grep -qvE '^\| ' <<< "$region"; then
    fail "$label: the captured output region contains an unprefixed line and cannot be trusted as a framed capture"
    return 1
  fi
  for frame in '=== output-begin' '=== output-end' '=== exit: '; do
    if [[ "$(grep -Fc -- "$frame" "$log_file" || true)" != 1 ]]; then
      fail "$label: the raw second-reader log does not contain exactly one '$frame' frame; the capture boundaries are forgeable"
      return 1
    fi
  done
  if [[ "$engine" == shell ]]; then
    for frame in '=== control-output-begin' '=== control-output-end' '=== control-exit: '; do
      if [[ "$(grep -Fc -- "$frame" "$log_file" || true)" != 1 ]]; then
        fail "$label: the raw second-reader log does not contain exactly one '$frame' frame; the capture boundaries are forgeable"
        return 1
      fi
    done
  fi
  # /RULE:second-reader-frame-integrity
  exit_line="$(awk '/^=== output-end$/ { getline value; print value; exit }' "$log_file")"

  # RULE:second-reader-run-passed
  [[ "$exit_line" == '=== exit: 0' ]] || {
    fail "$label: the recorded second-reader run did not exit zero (${exit_line:-no exit frame})"
    return 1
  }
  # /RULE:second-reader-run-passed

  # The default is deliberately the permissive one: with the rule below removed
  # the validator behaves exactly as it did before this gate existed, which is
  # what makes the rule's mutant survive when the rule is disabled instead of
  # being caught by a neighbouring check.
  matched=1
  # RULE:second-reader-selector-matches
  # `go test -run` that matches nothing prints a warning and exits ZERO. That is
  # the classic success-without-work and it has already vetoed a card in this
  # office, so it is refused explicitly rather than inferred from the exit code.
  if [[ -n "$region" ]] && grep -Eq '(no tests to run|no test files|matched no tests)' <<< "$region"; then
    fail "$label: the declared selector $selector matched no test; a run that did no work is not a pass"
    return 1
  fi
  if [[ "$engine" == go ]]; then
    selector_ere="$(ere_escape "$selector")"
    matched="$(grep -Ec "^\| *--- PASS: $selector_ere( |\(|/|\$)" <<< "$region" || true)"
    if (( matched < 1 )); then
      fail "$label: no passing test carried the declared selector $selector"
      return 1
    fi
    if grep -Eq '^\| *(--- FAIL:|FAIL[[:space:]])' <<< "$region"; then
      fail "$label: the recorded second-reader run contains a failing test"
      return 1
    fi
  else
    # A bash acceptance program selects by argument. The observable form of
    # "this selector selects something" is that a selector the program does NOT
    # know is refused: a dispatcher that ignores its argument answers the same
    # way to every name, so the cited layer name proves nothing about the layer.
    control_command="bash $test_path $second_reader_control_selector"
    grep -Fqx -- "=== control-command: $control_command" "$log_file" || {
      fail "$label: the recorded run carries no unknown-selector control, so the declared selector is not shown to select anything"
      return 1
    }
    control_exit_line="$(awk '/^=== control-output-end$/ { getline value; print value; exit }' "$log_file")"
    [[ "$control_exit_line" == '=== control-exit: 64' ]] || {
      fail "$label: the acceptance program answered ${control_exit_line:-nothing} to an unknown selector; $selector selects nothing it does not already do"
      return 1
    }
    matched=1
  fi
  # /RULE:second-reader-selector-matches
  second_reader_matched="$matched"
  return 0
}

second_reader_executor_available() {
  [[ -x "$board_dir/bin/second-reader" ]] || return 1
  command -v docker >/dev/null 2>&1 || command -v podman >/dev/null 2>&1
}

# The gate's optional third tier: re-run the second reader now and require it to
# agree with the record. Inconclusive is never a pass (law 11), so an `exec`
# request that cannot run is a failure rather than a downgrade.
second_reader_reexecute() {
  local card="$1" layer="$2" label="$3"
  local output status skip_reason=''
  case "$second_reader_mode" in
    recompute) skip_reason='AURUM_SECOND_READER=recompute was requested' ;;
    exec)
      second_reader_executor_available ||
        skip_reason='no second-reader executor or OCI engine is available on this host'
      ;;
  esac
  if [[ -n "$skip_reason" ]]; then
    # RULE:second-reader-inconclusive
    # The downgrade this replaces was invisible: the old default skipped the
    # re-execution in silence, and the stderr of a run that proved nothing was
    # byte-identical to the stderr of a run that proved everything. Skipping is
    # now named per layer, counted, and escalated to a distinct terminal verdict
    # below, so the two runs can never be confused for each other -- not by an
    # operator reading stderr, and not by a caller reading the exit status.
    second_reader_skipped+=("$card/$layer: $skip_reason; the cited test was NOT re-executed")
    printf 'board note: second reader NOT re-executed for %s/%s: %s\n' "$card" "$layer" "$skip_reason" >&2
    # /RULE:second-reader-inconclusive
    return 0
  fi
  set +e
  output="$("$board_dir/bin/second-reader" --verify --card "$card" --layer "$layer" 2>&1)"
  status=$?
  set -e
  # RULE:second-reader-reexecute
  if (( status != 0 )); then
    fail "$label: re-executing the second reader disagreed with the record (exit $status): $(head -n 3 <<< "$output" | tr '\n' ' ')"
    return 1
  fi
  # /RULE:second-reader-reexecute
  second_reader_executed=$((second_reader_executed + 1))
  return 0
}

validate_compose_file() {
  local card="$1"
  local compose_path="$2"
  local compose_file="$repo_root/$compose_path"
  local service_count network_none_count read_only_count pids_count memory_count cpu_count user_count
  local cap_drop_count cap_all_count nnp_count

  safe_repo_path "$compose_path" || {
    fail "$card: compose file path is not repository-relative and bounded: $compose_path"
    return
  }
  [[ -f "$compose_file" && ! -L "$compose_file" ]] || {
    fail "$card: compose file is missing or a symlink: $compose_path"
    return
  }
  if grep -Eiq '^[[:space:]]*(privileged|pid|ipc|cap_add|devices|build|extends|include|env_file|secrets|configs)[[:space:]]*:' "$compose_file"; then
    fail "$card: compose acceptance contains a forbidden privilege, namespace, device, or local build directive"
  fi
  if grep -Eiq '^[[:space:]]*network_mode[[:space:]]*:[[:space:]]*(host|service:|container:)' "$compose_file" || grep -Eiq '^[[:space:]]*networks?[[:space:]]*:' "$compose_file"; then
    fail "$card: compose acceptance contains a host/shared/custom network"
  fi
  if grep -Eiq '(docker\.sock|podman\.sock|containerd\.sock|^[[:space:]]*-[[:space:]]*(/|\.|~|\$\{?PWD)|type[[:space:]]*:[[:space:]]*bind|source[[:space:]]*:[[:space:]]*(/|\.|~|\$))' "$compose_file"; then
    fail "$card: compose acceptance contains a host bind/socket mount"
  fi
  service_count="$(grep -Ec '^[[:space:]]*image[[:space:]]*:' "$compose_file" || true)"
  (( service_count > 0 )) || fail "$card: compose acceptance must declare at least one pinned image"
  if grep -Eq '^[[:space:]]*image[[:space:]]*:' "$compose_file" && grep -Ev '^[[:space:]]*(#|$)' "$compose_file" | grep -E '^[[:space:]]*image[[:space:]]*:' | grep -Evq '@sha256:[0-9a-f]{64}[[:space:]]*$'; then
    fail "$card: every compose image must be digest-pinned"
  fi
  network_none_count="$(grep -Eic "^[[:space:]]*network_mode[[:space:]]*:[[:space:]]*['\"]?none['\"]?[[:space:]]*$" "$compose_file" || true)"
  read_only_count="$(grep -Eic '^[[:space:]]*read_only[[:space:]]*:[[:space:]]*true[[:space:]]*$' "$compose_file" || true)"
  pids_count="$(grep -Eic '^[[:space:]]*pids_limit[[:space:]]*:[[:space:]]*[0-9]+[[:space:]]*$' "$compose_file" || true)"
  memory_count="$(grep -Eic '^[[:space:]]*mem_limit[[:space:]]*:[[:space:]]*[^[:space:]]+[[:space:]]*$' "$compose_file" || true)"
  cpu_count="$(grep -Eic '^[[:space:]]*cpus[[:space:]]*:[[:space:]]*[^[:space:]]+[[:space:]]*$' "$compose_file" || true)"
  user_count="$(grep -Eic "^[[:space:]]*user[[:space:]]*:[[:space:]]*['\"]?[1-9][0-9]*(:[1-9][0-9]*)?['\"]?[[:space:]]*$" "$compose_file" || true)"
  (( network_none_count >= service_count )) || fail "$card: every compose service must deny network"
  (( read_only_count >= service_count )) || fail "$card: every compose service must use a read-only rootfs"
  (( pids_count >= service_count )) || fail "$card: every compose service must bound PIDs"
  (( memory_count >= service_count )) || fail "$card: every compose service must bound memory"
  (( cpu_count >= service_count )) || fail "$card: every compose service must bound CPUs"
  (( user_count >= service_count )) || fail "$card: every compose service must select a non-root numeric user"
  cap_drop_count="$(grep -Eic '^[[:space:]]*cap_drop[[:space:]]*:' "$compose_file" || true)"
  cap_all_count="$(grep -Eic "^[[:space:]]*-[[:space:]]*['\"]?ALL['\"]?[[:space:]]*$" "$compose_file" || true)"
  nnp_count="$(grep -Eic 'no-new-privileges' "$compose_file" || true)"
  (( cap_drop_count >= service_count && cap_all_count >= service_count )) || fail "$card: every compose service must drop ALL capabilities"
  (( nnp_count >= service_count )) || fail "$card: every compose service must deny new privileges"
}

validate_acceptance() {
  local card="$1"
  local id="$2"
  local count accept_line command compose_path profile declared_profile volume_count mount_count compose_file_count
  count="$(grep -Ec '^accept: `[^`]+`$' "$card" || true)"
  [[ "$count" -eq 1 ]] || {
    fail "$card: acceptance must be exactly one backtick-delimited command"
    return
  }
  accept_line="$(grep -E '^accept: `[^`]+`$' "$card")"
  command="${accept_line#accept: \`}"
  command="${command%\`}"

  [[ "$command" == *"$id"* ]] || fail "$card: acceptance does not bind to its card id"
  [[ "$command" != *':latest'* ]] || fail "$card: acceptance uses a mutable latest image"
  if [[ "$command" =~ (\||\;|\&\&|\|\||\>\>?|\<\<|\$\(|\`|\\|[[:space:]]--privileged([[:space:]]|$)|--network(=|[[:space:]])host|--pid(=|[[:space:]])host|--ipc(=|[[:space:]])host|--userns(=|[[:space:]])host|--cap-add|--device|docker\.sock|podman\.sock|containerd\.sock|--env-file|--project-directory|(^|[[:space:]])(-e|--env)(=|[[:space:]])) ]]; then
    fail "$card: acceptance contains shell composition, privilege escalation, host namespace/socket, or environment injection"
  fi

  declared_profile="$(grep -E '^container_profile: `[^`]+`$' "$card" | sed -E 's/^container_profile: `([^`]+)`$/\1/' || true)"
  [[ "$declared_profile" =~ ^[a-z0-9][a-z0-9._-]{2,63}$ ]] || fail "$card: container_profile must name one versioned bounded profile"

  if [[ "$command" =~ ^\./\.board/bin/oci-run[[:space:]]+--profile[[:space:]]+([a-z0-9][a-z0-9._-]{2,63})[[:space:]]+--card[[:space:]]+$id$ ]]; then
    profile="${BASH_REMATCH[1]}"
    [[ "$profile" == "$declared_profile" ]] || fail "$card: acceptance wrapper profile differs from container_profile"
    return
  fi

  if [[ "$command" =~ ^(docker|podman)[[:space:]]+run[[:space:]] ]]; then
    [[ "$command" =~ ^(docker|podman)[[:space:]]+run[[:space:]]+--rm([[:space:]]|$) || "$command" == *' --rm '* ]] || fail "$card: direct OCI run must be ephemeral (--rm)"
    [[ "$command" =~ (^|[[:space:]])[^[:space:]]+@sha256:[0-9a-f]{64}([[:space:]]|$) ]] || fail "$card: direct OCI image is not digest-pinned"
    [[ "$command" =~ (^|[[:space:]])--network=none([[:space:]]|$) ]] || fail "$card: direct OCI run must deny network"
    [[ "$command" =~ (^|[[:space:]])--read-only([[:space:]]|$) ]] || fail "$card: direct OCI run must use a read-only rootfs"
    [[ "$command" =~ (^|[[:space:]])--cap-drop=ALL([[:space:]]|$) ]] || fail "$card: direct OCI run must drop all capabilities"
    [[ "$command" =~ (^|[[:space:]])--security-opt=no-new-privileges([[:space:]]|$) ]] || fail "$card: direct OCI run must deny new privileges"
    [[ "$command" =~ --pids-limit(=|[[:space:]])[0-9]+ ]] || fail "$card: direct OCI run must bound PIDs"
    [[ "$command" =~ --memory(=|[[:space:]])[^[:space:]]+ ]] || fail "$card: direct OCI run must bound memory"
    [[ "$command" =~ --cpus(=|[[:space:]])[^[:space:]]+ ]] || fail "$card: direct OCI run must bound CPUs"
    [[ "$command" =~ --user(=|[[:space:]])[1-9][0-9]*(:[1-9][0-9]*)? ]] || fail "$card: direct OCI run must select a non-root numeric user"
    [[ "$command" != *'seccomp=unconfined'* && "$command" != *'apparmor=unconfined'* && "$command" != *'label=disable'* ]] || fail "$card: direct OCI run disables a mandatory security policy"
    if [[ "$command" == *' -v '* || "$command" == *' --volume '* || "$command" == *'--volume='* ]]; then
      [[ "$command" =~ -v[[:space:]]+\"?\$\{?PWD\}?\"?:/src:ro([[:space:]]|$) ]] || fail "$card: direct OCI run may bind only PWD to /src read-only"
      volume_count="$(grep -oE '(^|[[:space:]])(-v|--volume(=|[[:space:]]))' <<< "$command" | wc -l | tr -d ' ')"
      [[ "$volume_count" -eq 1 ]] || fail "$card: direct OCI run must not contain additional volume mounts"
    fi
    if [[ "$command" == *'--mount'* ]]; then
      [[ "$command" =~ --mount[[:space:]]+type=bind,src=\"?\$\{?PWD\}?\"?,dst=/src,readonly([[:space:]]|$) ]] || fail "$card: direct OCI --mount may bind only PWD to /src read-only"
      mount_count="$(grep -oE '(^|[[:space:]])--mount(=|[[:space:]])' <<< "$command" | wc -l | tr -d ' ')"
      [[ "$mount_count" -eq 1 ]] || fail "$card: direct OCI run must not contain additional mounts"
    fi
    return
  fi

  if [[ "$command" =~ ^(docker|podman)[[:space:]]+compose[[:space:]] ]]; then
    [[ "$command" != *' -v '* && "$command" != *'--volume'* ]] || fail "$card: compose command must not add host volumes"
    [[ "$command" =~ [[:space:]]run[[:space:]]+--rm([[:space:]]|$) || "$command" =~ [[:space:]]up[[:space:]].*--abort-on-container-exit.*--exit-code-from ]] || fail "$card: compose acceptance must use an ephemeral run or a bounded up with decisive exit code"
    compose_file_count="$(grep -oE '(^|[[:space:]])(-f([[:space:]]|=)|--file(=|[[:space:]]))' <<< "$command" | wc -l | tr -d ' ')"
    [[ "$compose_file_count" -eq 1 ]] || fail "$card: compose acceptance must use exactly one explicit file"
    if [[ "$command" =~ (^|[[:space:]])--file=([A-Za-z0-9._/-]+)([[:space:]]|$) ]]; then
      compose_path="${BASH_REMATCH[2]}"
    elif [[ "$command" =~ (^|[[:space:]])-f[[:space:]]+([A-Za-z0-9._/-]+)([[:space:]]|$) ]]; then
      compose_path="${BASH_REMATCH[2]}"
    else
      fail "$card: compose acceptance must name one explicit repository-relative file"
      return
    fi
    validate_compose_file "$card" "$compose_path"
    return
  fi

  fail "$card: acceptance must use the repository acceptance wrapper or a hardened Docker/Podman command"
}

# Minimal dependency-free JSON parser. It validates syntax, rejects duplicate
# object keys, and emits path/type/value triples for schema checks below.
json_flatten() {
  local file="$1"
  awk '
    function die(message) { if (!bad) print "json error at byte " pos ": " message > "/dev/stderr"; bad=1 }
    function ws(   c) { while (pos <= length(src)) { c=substr(src,pos,1); if (c ~ /[ \t\r\n]/) pos++; else break } }
    function string(   out,c,e,h) {
      if (substr(src,pos,1) != "\"") { die("expected string"); return "" }
      pos++
      while (pos <= length(src)) {
        c=substr(src,pos++,1)
        if (c == "\"") return out
        if (c == "\\") {
          if (pos > length(src)) { die("unfinished escape"); return "" }
          e=substr(src,pos++,1)
          if (e == "u") {
            h=substr(src,pos,4)
            if (length(h) != 4 || h !~ /^[0-9A-Fa-f]{4}$/) { die("invalid unicode escape"); return "" }
            pos += 4; out=out "?"
          } else if (e ~ /^["\\\/bfnrt]$/) {
            out=out "?"
          } else { die("invalid escape"); return "" }
        } else {
          if (c ~ /[\001-\037]/) { die("control character in string"); return "" }
          out=out c
        }
      }
      die("unterminated string"); return ""
    }
    function value(path,   c,v,start) {
      ws(); c=substr(src,pos,1)
      if (c == "{") { print path "\tobject\t"; object(path); return }
      if (c == "[") { print path "\tarray\t"; array(path); return }
      if (c == "\"") { v=string(); print path "\tstring\t" v; return }
      if (substr(src,pos,4) == "true") { pos+=4; print path "\tbool\ttrue"; return }
      if (substr(src,pos,5) == "false") { pos+=5; print path "\tbool\tfalse"; return }
      if (substr(src,pos,4) == "null") { pos+=4; print path "\tnull\tnull"; return }
      start=pos
      while (substr(src,pos,1) ~ /[0-9eE+.-]/) pos++
      v=substr(src,start,pos-start)
      if (v ~ /^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$/) { print path "\tnumber\t" v; return }
      die("invalid value")
    }
    function object(path,   key,child,c) {
      pos++; ws()
      if (substr(src,pos,1) == "}") { pos++; return }
      while (!bad) {
        ws(); key=string(); ws()
        if (substr(src,pos,1) != ":") { die("expected colon"); return }
        pos++; child=(path == "" ? key : path "." key)
        if (seen[child]++) { die("duplicate object key " child); return }
        value(child); ws(); c=substr(src,pos,1)
        if (c == "}") { pos++; return }
        if (c != ",") { die("expected comma or object end"); return }
        pos++
      }
    }
    function array(path,   i,c) {
      pos++; ws()
      if (substr(src,pos,1) == "]") { pos++; return }
      i=0
      while (!bad) {
        value(path "[" i "]"); i++; ws(); c=substr(src,pos,1)
        if (c == "]") { pos++; return }
        if (c != ",") { die("expected comma or array end"); return }
        pos++
      }
    }
    { src=src $0 "\n" }
    END {
      pos=1; ws(); value(""); ws()
      if (!bad && pos <= length(src)) die("trailing content")
      if (bad) exit 2
    }
  ' "$file"
}

json_get() {
  local flattened="$1"
  local path="$2"
  awk -F '\t' -v path="$path" '$1 == path { print $3; found=1 } END { if (!found) exit 1 }' <<< "$flattened"
}

json_type() {
  local flattened="$1"
  local path="$2"
  awk -F '\t' -v path="$path" '$1 == path { print $2; found=1 } END { if (!found) exit 1 }' <<< "$flattened"
}

require_json_value() {
  local evidence_file="$1"
  local flattened="$2"
  local path="$3"
  local expected="$4"
  local actual
  actual="$(json_get "$flattened" "$path" 2>/dev/null || true)"
  [[ "$actual" == "$expected" ]] || fail "$evidence_file: $path must equal $expected"
}

require_json_pattern() {
  local evidence_file="$1"
  local flattened="$2"
  local path="$3"
  local pattern="$4"
  local actual
  actual="$(json_get "$flattened" "$path" 2>/dev/null || true)"
  [[ "$actual" =~ $pattern ]] || fail "$evidence_file: $path is missing or malformed"
}

validate_evidence_artifact() {
  local card="$1"
  local artifact_path="$2"
  local expected_sha="$3"
  local expected_identity="$4"
  local expected_schema="$5"
  local expected_role="$6"
  local expected_context="$7"
  local expected_backend="$8"
  local expected_independence="${9:-}"
  local expected_profile="${10:-}"
  local evidence_root="$board_dir/evidence/$card"
  local artifact_file flattened actual_sha resolved verdict dimension_id dimension_status dimension_index role_nonce
  local expected_command_digest mutation_id mutation_index mutation_count scenario_index
  local -a dimension_ids=(contract design compatibility tests security concurrency errors-operations documentation scope-simplicity hunks)
  local -a expected_scenarios=()

  safe_repo_path "$artifact_path" || {
    fail "$evidence_root/manifest.json: unsafe evidence path $artifact_path"
    return
  }
  [[ "$artifact_path" == ".board/evidence/$card/"* ]] || {
    fail "$evidence_root/manifest.json: evidence path escapes the card directory: $artifact_path"
    return
  }
  artifact_file="$repo_root/$artifact_path"
  [[ -f "$artifact_file" && ! -L "$artifact_file" ]] || {
    fail "$evidence_root/manifest.json: referenced artifact is missing or a symlink: $artifact_path"
    return
  }
  if grep -Eiq '"(api[_-]?key|access[_-]?token|authorization|credential|raw[_-]?prompt|model[_-]?response|chain[_-]?of[_-]?thought)"[[:space:]]*:' "$artifact_file"; then
    fail "$artifact_file: forbidden sensitive/raw model field"
  fi
  resolved="$(cd "$(dirname "$artifact_file")" && pwd -P)/$(basename "$artifact_file")"
  [[ "$resolved" == "$evidence_root/"* ]] || {
    fail "$evidence_root/manifest.json: referenced artifact resolves outside the card directory"
    return
  }
  actual_sha="sha256:$(sha256sum "$artifact_file" | awk '{print $1}')"
  [[ "$actual_sha" == "$expected_sha" ]] || fail "$evidence_root/manifest.json: digest mismatch for $artifact_path"
  if ! flattened="$(json_flatten "$artifact_file" 2>/dev/null)"; then
    fail "$artifact_file: invalid JSON"
    return
  fi
  require_json_value "$artifact_file" "$flattened" schema "$expected_schema"
  require_json_value "$artifact_file" "$flattened" version 1
  require_json_value "$artifact_file" "$flattened" card_id "$card"
  require_json_value "$artifact_file" "$flattened" candidate_identity_digest "$expected_identity"
  require_json_value "$artifact_file" "$flattened" role "$expected_role"
  require_json_value "$artifact_file" "$flattened" sealed true
  require_json_pattern "$artifact_file" "$flattened" role_nonce '^[A-Za-z0-9._:-]{16,128}$'
  # The nonce is the per-role draw that makes a seal unforgeable by its peers.
  # A well-formed nonce that another role in the same bundle already sealed
  # proves a single run was replayed under two role labels, so reviewer-a,
  # reviewer-b and the skeptic stop being independent observers of the change.
  role_nonce="$(json_get "$flattened" role_nonce 2>/dev/null || true)"
  if [[ -n "$role_nonce" ]]; then
    if [[ -n "${bundle_role_nonces[$role_nonce]+x}" ]]; then
      fail "$artifact_file: role_nonce is already sealed by ${bundle_role_nonces[$role_nonce]} in this bundle; $expected_role must seal an independent nonce"
    else
      bundle_role_nonces[$role_nonce]="$expected_role"
    fi
  fi
  require_json_value "$artifact_file" "$flattened" context_digest "$expected_context"
  require_json_value "$artifact_file" "$flattened" backend_family_digest "$expected_backend"

  if [[ "$expected_schema" == "aurum.review-report" ]]; then
    verdict="$(json_get "$flattened" verdict 2>/dev/null || true)"
    [[ "$verdict" =~ ^(pass|pass_with_nits)$ ]] || fail "$artifact_file: a review/done card requires a non-blocking sealed verdict"
    require_json_value "$artifact_file" "$flattened" coverage.all_hunks true
    require_json_value "$artifact_file" "$flattened" coverage.all_dimensions true
    require_json_pattern "$artifact_file" "$flattened" coverage.manifest_digest '^sha256:[0-9a-f]{64}$'
    require_json_value "$artifact_file" "$flattened" independence_level "$expected_independence"
    require_json_value "$artifact_file" "$flattened" isolation.peer_report_visible_before_seal false
    require_json_value "$artifact_file" "$flattened" isolation.builder_trace_received false
    require_json_value "$artifact_file" "$flattened" isolation.shared_memory false
    require_json_value "$artifact_file" "$flattened" coverage.uncovered_hunks 0
    [[ "$(json_type "$flattened" coverage.dimensions 2>/dev/null || true)" == "array" ]] || fail "$artifact_file: coverage.dimensions must be an array"
    for (( dimension_index = 0; dimension_index < ${#dimension_ids[@]}; dimension_index++ )); do
      dimension_id="${dimension_ids[$dimension_index]}"
      require_json_value "$artifact_file" "$flattened" "coverage.dimensions[$dimension_index].id" "$dimension_id"
      dimension_status="$(json_get "$flattened" "coverage.dimensions[$dimension_index].status" 2>/dev/null || true)"
      [[ "$dimension_status" =~ ^(covered|not_applicable)$ ]] || fail "$artifact_file: dimension $dimension_id is not covered or justified not_applicable"
      if [[ "$dimension_status" == "covered" ]]; then
        require_json_pattern "$artifact_file" "$flattened" "coverage.dimensions[$dimension_index].evidence_digest" '^sha256:[0-9a-f]{64}$'
      else
        require_json_pattern "$artifact_file" "$flattened" "coverage.dimensions[$dimension_index].justification" '^.{16,512}$'
      fi
    done
    [[ -z "$(json_type "$flattened" "coverage.dimensions[${#dimension_ids[@]}]" 2>/dev/null || true)" ]] || fail "$artifact_file: coverage.dimensions must contain exactly ten entries"
  elif [[ "$expected_schema" == "aurum.acceptance-observation" ]]; then
    # This artifact is the run itself, not an opinion about it. It carries the
    # exit code the card's locked command actually produced: non-zero before the
    # change, zero after it, non-zero again under the skeptical mutation, and
    # zero on the restored clean replay. The command it claims to have run is
    # recomputed from the card, so a bundle cannot observe a different command.
    expected_command_digest="sha256:$(printf '%s' "${card_accept_commands[$card]:-}" | sha256sum | awk '{print $1}')"
    require_json_value "$artifact_file" "$flattened" verdict pass
    require_json_value "$artifact_file" "$flattened" command_digest "$expected_command_digest"
    [[ "$expected_profile" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "$artifact_file: the manifest declares no container profile digest to bind"
    require_json_value "$artifact_file" "$flattened" container_profile_digest "$expected_profile"
    require_json_pattern "$artifact_file" "$flattened" red.exit_code '^([1-9]|[1-9][0-9]|1[0-9][0-9]|2[0-4][0-9]|25[0-5])$'
    require_json_value "$artifact_file" "$flattened" green.exit_code 0
    require_json_value "$artifact_file" "$flattened" mutation.detected true
    require_json_pattern "$artifact_file" "$flattened" mutation.exit_code '^([1-9]|[1-9][0-9]|1[0-9][0-9]|2[0-4][0-9]|25[0-5])$'
    require_json_value "$artifact_file" "$flattened" mutation.restored_exit_code 0
    require_json_value "$artifact_file" "$flattened" clean_replay.exit_code 0
    require_json_value "$artifact_file" "$flattened" secret_canaries.pass true
    require_json_value "$artifact_file" "$flattened" secret_canaries.leaked 0
    mutation_count="${card_mutation_counts[$card]:-0}"
    mutation_id="$(json_get "$flattened" mutation.mutation_id 2>/dev/null || true)"
    if [[ "$mutation_id" =~ ^MUT-([0-9]{3})$ ]]; then
      mutation_index="$((10#${BASH_REMATCH[1]}))"
      (( mutation_index >= 1 && mutation_index <= mutation_count )) ||
        fail "$artifact_file: mutation.mutation_id is not a mutation the card declares"
    else
      fail "$artifact_file: mutation.mutation_id must name one MUT-NNN hypothesis from the card"
    fi
    read -ra expected_scenarios <<< "${card_scenario_ids[$card]:-}"
    (( ${#expected_scenarios[@]} > 0 )) || fail "$artifact_file: the card declares no acceptance scenario to observe"
    [[ "$(json_type "$flattened" scenarios 2>/dev/null || true)" == "array" ]] || fail "$artifact_file: scenarios must be an array"
    for (( scenario_index = 0; scenario_index < ${#expected_scenarios[@]}; scenario_index++ )); do
      require_json_value "$artifact_file" "$flattened" "scenarios[$scenario_index].id" "${expected_scenarios[$scenario_index]}"
      require_json_value "$artifact_file" "$flattened" "scenarios[$scenario_index].exit_code" 0
    done
    [[ -z "$(json_type "$flattened" "scenarios[${#expected_scenarios[@]}]" 2>/dev/null || true)" ]] ||
      fail "$artifact_file: scenarios must observe exactly the acceptance scenarios the card declares"
  else
    require_json_value "$artifact_file" "$flattened" verdict pass
    require_json_value "$artifact_file" "$flattened" challenge_plan.sealed_before_reviews true
    require_json_pattern "$artifact_file" "$flattened" challenge_plan.digest '^sha256:[0-9a-f]{64}$'
    require_json_value "$artifact_file" "$flattened" challenge_plan.all_acceptance_criteria true
    require_json_value "$artifact_file" "$flattened" challenge_plan.all_trust_boundaries true
    require_json_value "$artifact_file" "$flattened" isolation.reviews_visible_before_challenge_seal false
    require_json_value "$artifact_file" "$flattened" mutation.detected true
    require_json_value "$artifact_file" "$flattened" clean_replay.pass true
    require_json_value "$artifact_file" "$flattened" secret_canaries.pass true
  fi
}

validate_evidence_hashes() {
  local card="$1"
  local flattened="$2"
  local manifest="$board_dir/evidence/$card/manifest.json"
  local evidence_root="$board_dir/evidence/$card"
  local index path expected_sha artifact_file resolved actual_sha path_key actual_relative count=0
  local expected_index=0 previous_path='' chain_input='' identity computed_chain declared_chain
  local -a evidence_indices=()
  declare -A hashed_paths=()

  mapfile -t evidence_indices < <(
    awk -F '\t' '$1 ~ /^evidence_hashes\[[0-9]+\]\.path$/ {
      value=$1
      sub(/^evidence_hashes\[/, "", value)
      sub(/\]\.path$/, "", value)
      print value
    }' <<< "$flattened" | sort -n -u
  )
  (( ${#evidence_indices[@]} >= 3 )) || fail "$manifest: evidence_hashes must include both reviews and the skeptic report"
  for index in "${evidence_indices[@]}"; do
    [[ "$index" -eq "$expected_index" ]] || fail "$manifest: evidence_hashes indices must be contiguous from zero"
    expected_index=$((expected_index + 1))
    path="$(json_get "$flattened" "evidence_hashes[$index].path" 2>/dev/null || true)"
    expected_sha="$(json_get "$flattened" "evidence_hashes[$index].sha256" 2>/dev/null || true)"
    safe_repo_path "$path" || {
      fail "$manifest: evidence_hashes[$index].path is unsafe"
      continue
    }
    [[ "$path" == ".board/evidence/$card/"* ]] || {
      fail "$manifest: evidence_hashes[$index].path escapes the card directory"
      continue
    }
    [[ -z "${hashed_paths[$path]+x}" ]] || fail "$manifest: duplicate evidence hash path $path"
    hashed_paths[$path]=1
    if [[ -n "$previous_path" && "$previous_path" > "$path" ]]; then
      fail "$manifest: evidence_hashes paths must be strictly byte-sorted"
    fi
    previous_path="$path"
    [[ "$expected_sha" =~ ^sha256:[0-9a-f]{64}$ ]] || {
      fail "$manifest: evidence_hashes[$index].sha256 is malformed"
      continue
    }
    artifact_file="$repo_root/$path"
    [[ -f "$artifact_file" && ! -L "$artifact_file" ]] || {
      fail "$manifest: hashed evidence is missing or a symlink: $path"
      continue
    }
    resolved="$(cd "$(dirname "$artifact_file")" && pwd -P)/$(basename "$artifact_file")"
    [[ "$resolved" == "$evidence_root/"* ]] || {
      fail "$manifest: hashed evidence resolves outside the card directory: $path"
      continue
    }
    actual_sha="sha256:$(sha256sum "$artifact_file" | awk '{print $1}')"
    [[ "$actual_sha" == "$expected_sha" ]] || fail "$manifest: evidence hash mismatch for $path"
    printf -v chain_input '%sartifact.path=%s\nartifact.sha256=%s\n' "$chain_input" "$path" "$expected_sha"
    count=$((count + 1))
  done
  [[ "$count" -eq "${#evidence_indices[@]}" ]] || fail "$manifest: evidence_hashes is incomplete or invalid"

  if find "$evidence_root" -type l -print -quit | grep -q .; then
    fail "$manifest: evidence directory contains a symlink"
  fi
  while IFS= read -r -d '' artifact_file; do
    [[ "$artifact_file" != "$manifest" ]] || continue
    actual_relative=".board/evidence/$card/${artifact_file#"$evidence_root/"}"
    [[ -n "${hashed_paths[$actual_relative]+x}" ]] || fail "$manifest: unlisted evidence artifact $actual_relative"
    if grep -Eiq -- '(-----BEGIN[[:space:]][A-Z ]*PRIVATE KEY-----|AKIA[0-9A-Z]{16}|sk-[A-Za-z0-9_-]{20,}|gh[pousr]_[A-Za-z0-9]{20,})' "$artifact_file"; then
      fail "$manifest: evidence artifact contains a credential-like value: $actual_relative"
    fi
  done < <(find "$evidence_root" -type f -print0 | sort -z)

  for path_key in reviews.a.path reviews.b.path skeptic.path; do
    path="$(json_get "$flattened" "$path_key" 2>/dev/null || true)"
    if [[ -z "$path" || -z "${hashed_paths[$path]+x}" ]]; then
      fail "$manifest: $path_key is absent from evidence_hashes"
    fi
  done
  path="$(json_get "$flattened" human_approval.path 2>/dev/null || true)"
  if [[ -n "$path" && -z "${hashed_paths[$path]+x}" ]]; then
    fail "$manifest: human_approval.path is absent from evidence_hashes"
  fi
  path="$(json_get "$flattened" acceptance.path 2>/dev/null || true)"
  if [[ -n "$path" && -z "${hashed_paths[$path]+x}" ]]; then
    fail "$manifest: acceptance.path is absent from evidence_hashes"
  fi

  identity="$(json_get "$flattened" candidate_identity_digest 2>/dev/null || true)"
  declared_chain="$(json_get "$flattened" evidence_chain_digest 2>/dev/null || true)"
  computed_chain="sha256:$(printf 'candidate_identity_digest=%s\n%s' "$identity" "$chain_input" | sha256sum | awk '{print $1}')"
  [[ "$declared_chain" == "$computed_chain" ]] ||
    fail "$manifest: evidence_chain_digest does not match the canonical CandidateIdentity/path/hash chain"
}

manifest_lists_evidence_path() {
  local flattened="$1" wanted="$2"
  awk -F '\t' -v wanted="$wanted" '
    $1 ~ /^evidence_hashes\[[0-9]+\]\.path$/ && $3 == wanted { found=1 }
    END { exit(found ? 0 : 1) }
  ' <<< "$flattened"
}

# A `done` card whose evidence bundle predates this gate cannot have its bundle
# reopened by a builder: re-sealing CandidateIdentityV1 is a coordinator act
# performed by three independent roles. The run itself is still recorded, under
# `.board/tests/second-reader-legacy/`, and every fact in it is recomputed here
# exactly as it is for a bundled observation. The only thing the legacy record
# does NOT carry is the binding to the sealed candidate identity, and the
# registry names that missing binding per card instead of hiding it.
validate_second_reader_legacy_proof() {
  local card="$1" layer="$2" test_path="$3" selector="$4" engine="$5"
  local log_rel="$second_reader_legacy_dir/$card.$layer.raw.txt"
  local label="$repo_root/$log_rel"

  [[ -f "$repo_root/$log_rel" && ! -L "$repo_root/$log_rel" ]] || {
    fail "${files[$card]}: $card/$layer is recorded in $second_reader_legacy_file but $log_rel carries no raw second-reader run"
    return 1
  }
  second_reader_verify_log "$card" "$layer" "$test_path" "$selector" "$engine" \
    "$repo_root/$log_rel" "$label" || return 1
  second_reader_reexecute "$card" "$layer" "$label" || return 1
  return 0
}

# The `done` execution gate. Every concrete Unit/Contract/Integration/E2E
# citation of a `done` card must point at a run that happened and passed, and
# every number the observation states is re-derived here.
validate_second_reader_bundle() {
  local card="$1" identity="$2" flattened="$3"
  shift 3
  local -a peer_contexts=("$@")
  local layer test_path selector engine declaration_line
  local observation_rel log_rel observation_file label obs_flat
  local declared_command declared_declaration declared_file_digest declared_log_digest
  local nonce peer_context declared_matched

  # RULE:second-reader-done-requires-execution
  [[ -n "${card_layer_specs[$card]:-}" ]] || return 0
  while IFS=$'\t' read -r layer test_path selector engine declaration_line; do
    [[ -n "$layer" ]] || continue
    observation_rel=".board/evidence/$card/second-reader/$layer.json"
    log_rel=".board/evidence/$card/second-reader/$layer.raw.txt"
    observation_file="$repo_root/$observation_rel"
    label="$observation_file"

    # RULE:second-reader-done-requires-execution
    if [[ ! -e "$observation_file" && ! -L "$observation_file" ]]; then
      if [[ -n "${second_reader_legacy[$card/$layer]+x}" ]]; then
        validate_second_reader_legacy_proof "$card" "$layer" "$test_path" "$selector" "$engine" || true
      else
        fail "${files[$card]}: done card cites $layer test \`$test_path::$selector\` but no second reader ever executed it: $observation_rel is absent and $card/$layer is not recorded in $second_reader_legacy_file"
      fi
      continue
    fi
    [[ -f "$observation_file" && ! -L "$observation_file" ]] || {
      fail "$label: the second-reader observation must be a regular non-symlink file"
      continue
    }
    manifest_is_sealed=0
    if awk -F '\t' -v wanted="$observation_rel" '
        BEGIN { sealed=0 }
        $1 ~ /^evidence_hashes\[[0-9]+\]\.path$/ { sealed=1 }
        $1 ~ /^evidence_hashes\[[0-9]+\]\.path$/ && $3 == wanted { found=1 }
        END { exit(sealed ? 0 : 1) }' <<< "$flattened"; then
      manifest_is_sealed=1
    else
      manifest_is_sealed=0
    fi
    if (( manifest_is_sealed == 1 )); then
      manifest_lists_evidence_path "$flattened" "$observation_rel" ||
        fail "$label: the second-reader observation is absent from evidence_hashes and therefore outside the sealed chain"
      manifest_lists_evidence_path "$flattened" "$log_rel" ||
        fail "$label: the raw second-reader log $log_rel is absent from evidence_hashes and therefore outside the sealed chain"
    else
      # Execution-proof cards carry no hand-written manifest, so there is no
      # evidence_hashes chain to join; the observation is bound by the same
      # recomputed identity and raw-log digest the manifest path enforces below.
      [[ -n "$flattened" ]] &&
        printf 'board note: %s sits outside a hand-written evidence chain by design (execution-proof card)\n' "$label" >&2
    fi

    if ! obs_flat="$(json_flatten "$observation_file" 2>/dev/null)"; then
      fail "$label: invalid JSON"
      continue
    fi
    require_json_value "$observation_file" "$obs_flat" schema aurum.second-reader-observation
    require_json_value "$observation_file" "$obs_flat" version 1
    require_json_value "$observation_file" "$obs_flat" card_id "$card"
    require_json_value "$observation_file" "$obs_flat" candidate_identity_digest "$identity"
    require_json_value "$observation_file" "$obs_flat" role second-reader
    require_json_value "$observation_file" "$obs_flat" sealed true
    # Law 7: captured program output is data. The observation says so about
    # itself, and this validator behaves accordingly by re-deriving every fact.
    require_json_value "$observation_file" "$obs_flat" observation_trusted false
    require_json_value "$observation_file" "$obs_flat" verdict pass
    require_json_value "$observation_file" "$obs_flat" layer "$layer"
    require_json_value "$observation_file" "$obs_flat" engine "$engine"
    require_json_value "$observation_file" "$obs_flat" test_path "$test_path"
    require_json_value "$observation_file" "$obs_flat" selector "$selector"
    require_json_value "$observation_file" "$obs_flat" raw_output_path "$log_rel"
    require_json_value "$observation_file" "$obs_flat" exit_code 0
    require_json_pattern "$observation_file" "$obs_flat" role_nonce '^[A-Za-z0-9._:-]{16,128}$'
    require_json_pattern "$observation_file" "$obs_flat" context_digest '^sha256:[0-9a-f]{64}$'
    require_json_pattern "$observation_file" "$obs_flat" backend_family_digest '^sha256:[0-9a-f]{64}$'

    nonce="$(json_get "$obs_flat" role_nonce 2>/dev/null || true)"
    if [[ -n "$nonce" ]]; then
      if [[ -n "${bundle_role_nonces[$nonce]+x}" ]]; then
        fail "$label: role_nonce is already sealed by ${bundle_role_nonces[$nonce]} in this bundle; the second reader must seal an independent nonce"
      else
        bundle_role_nonces[$nonce]="second-reader/$layer"
      fi
    fi
    for peer_context in "${peer_contexts[@]}"; do
      [[ -n "$peer_context" ]] || continue
      [[ "$(json_get "$obs_flat" context_digest 2>/dev/null || true)" != "$peer_context" ]] ||
        fail "$label: the second reader reuses a review or acceptance context and is therefore not an independent reading"
    done

    # RULE:second-reader-observation-recomputed
    # Each of these is recomputed from the card and the tree, never read from
    # the artifact that is being judged.
    declared_declaration="sha256:$(printf '%s' "$declaration_line" | sha256sum | awk '{print $1}')"
    require_json_value "$observation_file" "$obs_flat" declaration_digest "$declared_declaration"
    declared_command="sha256:$(second_reader_command "$engine" "$test_path" "$selector" | sha256sum | awk '{print $1}')"
    require_json_value "$observation_file" "$obs_flat" command_digest "$declared_command"
    if [[ -f "$repo_root/$test_path" && ! -L "$repo_root/$test_path" ]]; then
      declared_file_digest="sha256:$(sha256sum "$repo_root/$test_path" | awk '{print $1}')"
      require_json_value "$observation_file" "$obs_flat" test_file_digest "$declared_file_digest"
    fi
    if [[ -f "$repo_root/$log_rel" && ! -L "$repo_root/$log_rel" ]]; then
      declared_log_digest="sha256:$(sha256sum "$repo_root/$log_rel" | awk '{print $1}')"
      require_json_value "$observation_file" "$obs_flat" raw_output_sha256 "$declared_log_digest"
    fi
    # /RULE:second-reader-observation-recomputed

    if second_reader_verify_log "$card" "$layer" "$test_path" "$selector" "$engine" \
      "$repo_root/$log_rel" "$repo_root/$log_rel"; then
      # RULE:second-reader-matched-recomputed
      declared_matched="$(json_get "$obs_flat" matched_tests 2>/dev/null || true)"
      [[ "$declared_matched" == "$second_reader_matched" ]] ||
        fail "$label: matched_tests claims $declared_matched but the raw log shows $second_reader_matched passing selector match(es)"
      # /RULE:second-reader-matched-recomputed
      second_reader_reexecute "$card" "$layer" "$label" || true
    fi
  done <<< "${card_layer_specs[$card]}"
  # /RULE:second-reader-done-requires-execution
  return 0
}

# The registry is a ratchet, not an escape hatch: it may only shrink. An entry
# for a card that is not `done`, does not declare that layer, or already carries
# a bundled observation is dead and is refused, so the list can never
# pre-authorize future work or outlive the debt it records.
validate_second_reader_legacy_registry() {
  local registry="$repo_root/$second_reader_legacy_file"
  local line_number=0 line entry_card entry_layer entry_reason previous_key=''
  local key spec_found layer test_path selector engine declaration_line
  local entry_count=0 card_count=0 distinct_count
  local -A seen_cards=()

  [[ -e "$registry" || -L "$registry" ]] || return 0
  [[ -f "$registry" && ! -L "$registry" ]] || {
    fail "$second_reader_legacy_file: the second-reader legacy registry must be a regular non-symlink file"
    return
  }
  while IFS= read -r line || [[ -n "$line" ]]; do
    line_number=$((line_number + 1))
    [[ -n "$line" ]] || continue
    [[ "$line" != '#'* ]] || continue
    IFS=$'\t' read -r entry_card entry_layer entry_reason <<< "$line"
    [[ "$entry_card" =~ ^AUR-[0-9]{3}$ ]] || {
      fail "$second_reader_legacy_file:$line_number: entry does not begin with a card id"
      continue
    }
    [[ "$entry_layer" =~ ^(Unit|Contract|Integration|E2E)$ ]] || {
      fail "$second_reader_legacy_file:$line_number: $entry_card names no TDD layer"
      continue
    }
    key="$entry_card/$entry_layer"
    [[ -z "$previous_key" || "$previous_key" < "$key" ]] ||
      fail "$second_reader_legacy_file:$line_number: entries must be strictly sorted and unique ($previous_key then $key)"
    previous_key="$key"
    if (( ${#entry_reason} < 40 )) || is_generic_text "$entry_reason"; then
      fail "$second_reader_legacy_file:$line_number: $key must state a specific, non-generic reason of at least 40 characters"
      continue
    fi
    # RULE:second-reader-legacy-ratchet
    [[ "${card_states[$entry_card]:-}" == done ]] || {
      fail "$second_reader_legacy_file:$line_number: $entry_card is not in done; the legacy registry can never pre-authorize a future transition"
      continue
    }
    spec_found=0
    while IFS=$'\t' read -r layer test_path selector engine declaration_line; do
      [[ "$layer" == "$entry_layer" ]] || continue
      spec_found=1
    done <<< "${card_layer_specs[$entry_card]:-}"
    (( spec_found == 1 )) || {
      fail "$second_reader_legacy_file:$line_number: $key no longer cites a concrete $entry_layer test; the ratchet only shrinks, remove the stale entry"
      continue
    }
    if [[ -f "$repo_root/.board/evidence/$entry_card/second-reader/$entry_layer.json" ]]; then
      fail "$second_reader_legacy_file:$line_number: $key already carries a sealed second-reader observation; the ratchet only shrinks, remove the stale entry"
      continue
    fi
    # /RULE:second-reader-legacy-ratchet
    # RULE:second-reader-legacy-frozen
    [[ $'\n'"$second_reader_legacy_frozen"$'\n' == *$'\n'"$key"$'\n'* ]] || {
      fail "$second_reader_legacy_file:$line_number: $key is not in the frozen cutover set compiled into .board/validate.sh; a card cannot join the legacy registry in the same commit that moves it to done"
      continue
    }
    # /RULE:second-reader-legacy-frozen
    second_reader_legacy[$key]="$entry_reason"
    second_reader_debt+=("$key: $entry_reason")
    entry_count=$((entry_count + 1))
    [[ -n "${seen_cards[$entry_card]+x}" ]] || { seen_cards[$entry_card]=1; card_count=$((card_count + 1)); }
  done < "$registry"

  distinct_count="$(second_reader_distinct_programs)"
  validate_registry_counts "$second_reader_legacy_file" "$entry_count" "$card_count" "$distinct_count"
}

# F5, stated instead of implied. `tests/acceptance/AUR-359.sh` dispatches four
# selector names onto ONE body, so three of that card's recorded "layers" are
# the same program three times and their captured regions differ only in the
# selector they echo. A citation is therefore not an execution, and the number
# this board may claim is the number of DISTINCT captured programs -- computed
# here from the recorded bytes, declared in the registry header and the README,
# and required to agree with both.
second_reader_distinct_programs() {
  local log_file selector normalized
  local -A seen=()
  local total=0
  [[ -d "$repo_root/$second_reader_legacy_dir" ]] || { printf '0'; return 0; }
  while IFS= read -r log_file; do
    [[ -f "$log_file" && ! -L "$log_file" ]] || continue
    selector="$(sed -n 's/^=== selector: //p' "$log_file" | head -n 1)"
    normalized="$(
      awk '/^=== output-begin$/ { inside=1; next } /^=== output-end$/ { exit } inside { print }' "$log_file" |
        { [[ -n "$selector" ]] && sed -e "s/$(ere_escape "$selector")/SELECTOR/g" || cat; } |
        sha256sum | awk '{print $1}'
    )"
    [[ -n "${seen[$normalized]+x}" ]] || { seen[$normalized]=1; total=$((total + 1)); }
  done < <(find "$repo_root/$second_reader_legacy_dir" -mindepth 1 -maxdepth 1 -type f -name '*.raw.txt' | sort)
  printf '%s' "$total"
}

# Three texts stated three different numbers for one list: the registry header
# said sixteen, the README said fifteen, the file carried fifteen. Prose that
# counts is prose that drifts, so the numbers are derived from the file here and
# BOTH texts must carry the derived line verbatim. Absence of the line is a
# failure, not a pass (law 12).
validate_registry_counts() {
  local registry_rel="$1" entries="$2" cards="$3" distinct="${4:-}"
  local canonical readme_line
  # RULE:second-reader-registry-counts
  if [[ -n "$distinct" ]]; then
    canonical="$entries entry(ies) across $cards card(s), $distinct distinct captured program(s)"
  else
    canonical="$entries entry(ies) across $cards card(s)"
  fi
  grep -Fqx -- "# count: $canonical" "$repo_root/$registry_rel" ||
    fail "$registry_rel: the header must declare exactly '# count: $canonical'; a registry whose stated size disagrees with its contents counts nothing"
  if [[ -f "$repo_root/$second_reader_readme" && ! -L "$repo_root/$second_reader_readme" ]]; then
    readme_line="- \`$registry_rel\` records $canonical."
    grep -Fqx -- "$readme_line" "$repo_root/$second_reader_readme" ||
      fail "$second_reader_readme: must carry exactly the line '$readme_line'; the README and the registry may not state different sizes for the same list"
  fi
  # /RULE:second-reader-registry-counts
  return 0
}

# Evidence-shaped text under the legacy directory that no entry names is
# unpoliced: nothing recomputes it, nothing prints it, and it survives the
# removal of the debt it once documented. It is refused instead.
validate_second_reader_legacy_orphans() {
  local log_file base key
  [[ -d "$repo_root/$second_reader_legacy_dir" ]] || return 0
  # RULE:second-reader-legacy-orphan
  while IFS= read -r log_file; do
    base="$(basename "$log_file")"
    if [[ ! "$base" =~ ^(AUR-[0-9]{3})\.(Unit|Contract|Integration|E2E)\.raw\.txt$ ]]; then
      fail "$second_reader_legacy_dir/$base: the legacy directory may only hold <card>.<layer>.raw.txt records"
      continue
    fi
    key="${BASH_REMATCH[1]}/${BASH_REMATCH[2]}"
    [[ -n "${second_reader_legacy[$key]+x}" ]] ||
      fail "$second_reader_legacy_dir/$base: no accepted entry in $second_reader_legacy_file names $key; an unreferenced record is evidence-shaped text nothing recomputes"
  done < <(find "$repo_root/$second_reader_legacy_dir" -mindepth 1 -maxdepth 1 | sort)
  # /RULE:second-reader-legacy-orphan
  return 0
}

# The exemption registry. `validate_second_reader_bundle` is only reachable
# through a concrete citation, so a card whose four layers all read
# `not-applicable: <reason>` never touched the second reader at all -- and two
# cards sat in `done` in exactly that shape, one of them justifying itself with
# "covered by the acceptance", which is the single engine this gate exists to
# distrust. Silence is now refused: such a card is either outside `done`, or
# named here under the same ratchet the legacy registry carries.
validate_second_reader_exempt_registry() {
  local registry="$repo_root/$second_reader_exempt_file"
  local line_number=0 line entry_card entry_reason previous_key=''
  local entry_count=0

  [[ -e "$registry" || -L "$registry" ]] || return 0
  [[ -f "$registry" && ! -L "$registry" ]] || {
    fail "$second_reader_exempt_file: the second-reader exemption registry must be a regular non-symlink file"
    return
  }
  while IFS= read -r line || [[ -n "$line" ]]; do
    line_number=$((line_number + 1))
    [[ -n "$line" ]] || continue
    [[ "$line" != '#'* ]] || continue
    IFS=$'\t' read -r entry_card entry_reason <<< "$line"
    [[ "$entry_card" =~ ^AUR-[0-9]{3}$ ]] || {
      fail "$second_reader_exempt_file:$line_number: entry does not begin with a card id"
      continue
    }
    [[ -z "$previous_key" || "$previous_key" < "$entry_card" ]] ||
      fail "$second_reader_exempt_file:$line_number: entries must be strictly sorted and unique ($previous_key then $entry_card)"
    previous_key="$entry_card"
    if (( ${#entry_reason} < 40 )) || is_generic_text "$entry_reason"; then
      fail "$second_reader_exempt_file:$line_number: $entry_card must state a specific, non-generic reason of at least 40 characters"
      continue
    fi
    # RULE:second-reader-exempt-ratchet
    [[ "${card_states[$entry_card]:-}" == done ]] || {
      fail "$second_reader_exempt_file:$line_number: $entry_card is not in done; the exemption registry can never pre-authorize a future transition"
      continue
    }
    [[ -z "${card_layer_specs[$entry_card]:-}" ]] || {
      fail "$second_reader_exempt_file:$line_number: $entry_card now cites a concrete TDD layer test; the ratchet only shrinks, remove the stale entry"
      continue
    }
    # /RULE:second-reader-exempt-ratchet
    # RULE:second-reader-exempt-frozen
    [[ $'\n'"$second_reader_exempt_frozen"$'\n' == *$'\n'"$entry_card"$'\n'* ]] || {
      fail "$second_reader_exempt_file:$line_number: $entry_card is not in the frozen exemption set compiled into .board/validate.sh; a card cannot join the exemption registry in the same commit that moves it to done"
      continue
    }
    # /RULE:second-reader-exempt-frozen
    second_reader_exempt[$entry_card]="$entry_reason"
    second_reader_exempt_debt+=("$entry_card: $entry_reason")
    entry_count=$((entry_count + 1))
  done < "$registry"

  validate_registry_counts "$second_reader_exempt_file" "$entry_count" "$entry_count"
}

# The gate used to be opt-in: it started at a concrete citation, so a card with
# four `not-applicable` layers walked past it without leaving a trace. A card
# crossing into `review` or `done` must therefore either hand the second reader
# something it can execute, or be named in the exemption registry above. An
# unrecorded exemption is an absence, and an absence never passes (law 12).
validate_second_reader_coverage() {
  local id
  # RULE:second-reader-coverage-required
  for id in "${!ids[@]}"; do
    [[ "${card_states[$id]}" =~ ^(review|done)$ ]] || continue
    [[ -z "${card_layer_specs[$id]:-}" ]] || continue
    [[ -z "${second_reader_exempt[$id]+x}" ]] || continue
    fail "${files[$id]}: ${card_states[$id]} card hands the second reader nothing to execute -- all four TDD layers are not-applicable -- and it is not recorded in $second_reader_exempt_file"
  done
  # /RULE:second-reader-coverage-required
  return 0
}

# Execution-derived evidence. The card is proved by command output the
# validator recomputes, never by a hand-written declaration of approval. For a
# `review`/`done` card this is the ONLY accepted proof shape: the acceptance
# run (with its raw program output), one failing run per locked skeptical
# mutation and a byte-identical restore, the two blind review reports, and —
# for `done` — the second reader re-executed over the same candidate.
validate_execution_proof() {
  local card="$1"
  local state="$2"
  local evidence_root="$board_dir/evidence/$card"
  local acc_json acc_stdout flattened computed_sha recorded_sha
  local mut mut_restore mut_flat restore_flat restore_sha record
  local mut_count n=0 review review_a review_b
  local accept_profile locked_image expected_lock_digest actual_lock_digest
  local mut_red_stderr mut_restore_stdout computed_stderr_sha recorded_stderr_sha computed_restore_sha

  # The acceptance observation is the decisive artifact the card locks. Its
  # location is recomputed from the card, never read from the evidence.
  acc_json="${card_expected_artifacts[$card]:-}"
  if [[ -z "$acc_json" || ! -f "$repo_root/$acc_json" || -L "$repo_root/$acc_json" ]]; then
    return 0
  fi
  acc_json="$repo_root/$acc_json"

  if grep -Eiq -- '(-----BEGIN[[:space:]][A-Z ]*PRIVATE? KEY-----|AKIA[0-9A-Z]{16}|sk-[A-Za-z0-9_-]{20,}|gh[pousr]_[A-Za-z0-9]{20,})' "$acc_json"; then
    fail "$acc_json: acceptance evidence contains a credential-like value"
  fi
  if ! flattened="$(json_flatten "$acc_json" 2>/dev/null)"; then
    fail "$acc_json: invalid acceptance JSON"
    return 0
  fi
  require_json_value "$acc_json" "$flattened" schema aurum.acceptance-execution
  require_json_value "$acc_json" "$flattened" card_id "$card"
  require_json_value "$acc_json" "$flattened" exit_code 0
  require_json_value "$acc_json" "$flattened" secret_detected false
  require_json_value "$acc_json" "$flattened" observation_trusted false
  # The card locks an accept command; the observation must be a digest of the
  # pinned lock file that command names. The lock digest is recomputed from the
  # profile the command used, never read from the evidence.
  accept_profile="$(sed -n 's/^accept: `[^`]*--profile \([^ `]*\).*/\1/p' "${files[$card]:-$board_dir/cards/$card.md}" | head -n 1)"
  locked_image="$(sha256sum "$board_dir/locks/oci/$accept_profile.lock.json" 2>/dev/null | awk '{print $1}' || true)"
  expected_lock_digest="sha256:${locked_image:-missing}"
  actual_lock_digest="$(json_get "$flattened" lock_digest 2>/dev/null || true)"
  if [[ -n "$locked_image" && "$actual_lock_digest" != "$expected_lock_digest" ]]; then
    fail "$acc_json: lock_digest must be the recomputed digest of locks/oci/$accept_profile.lock.json"
  fi
  # Raw program stdout must corroborate the recorded digest: recompute and
  # compare, because a digest that disagrees with the bytes on disk loses.
  acc_stdout="$(dirname "$acc_json")/$(basename "$acc_json" .json).stdout.txt"
  if [[ -f "$acc_stdout" && ! -L "$acc_stdout" ]]; then
    computed_sha="sha256:$(sha256sum "$acc_stdout" | awk '{print $1}')"
    recorded_sha="$(json_get "$flattened" stdout_sha256 2>/dev/null || true)"
    if [[ -z "$recorded_sha" ]]; then
      fail "$acc_json: acceptance observation carries no stdout_sha256"
    elif [[ "$computed_sha" != "$recorded_sha" ]]; then
      fail "$acc_json: recorded stdout_sha256 disagrees with the raw program stdout"
    fi
  else
    fail "${files[$card]}: acceptance raw program stdout is missing: $acc_stdout"
  fi

  # Skeptical mutations: every locked MUT-NNN must fail the same acceptance and
  # a byte-identical restore must reproduce the exact GREEN.
  mut_count="${card_mutation_counts[$card]:-0}"
  while (( n < mut_count )); do
    n=$((n + 1))
    record=".board/evidence/$card/mutations/MUT-$(printf '%03d' "$n")-RED.json"
    mut_restore=".board/evidence/$card/mutations/MUT-$(printf '%03d' "$n")-restore.json"
    if [[ ! -f "$repo_root/$record" || -L "$repo_root/$record" ]]; then
      fail "${files[$card]}: skeptical mutation $n has no RED evidence at $record"
      continue
    fi
    if ! mut_flat="$(json_flatten "$repo_root/$record" 2>/dev/null)"; then
      fail "$repo_root/$record: invalid mutation JSON"
      continue
    fi
    require_json_value "$repo_root/$record" "$mut_flat" schema aurum.acceptance-execution
    require_json_value "$repo_root/$record" "$mut_flat" card_id "$card"
    require_json_value "$repo_root/$record" "$mut_flat" exit_code 1
    require_json_value "$repo_root/$record" "$mut_flat" observation_trusted false
    mut_red_stderr=".board/evidence/$card/mutations/MUT-$(printf '%03d' "$n")-RED.stderr.txt"
    if [[ -f "$repo_root/$mut_red_stderr" && ! -L "$repo_root/$mut_red_stderr" ]]; then
      computed_stderr_sha="sha256:$(sha256sum "$repo_root/$mut_red_stderr" | awk '{print $1}')"
      recorded_stderr_sha="$(json_get "$mut_flat" stderr_sha256 2>/dev/null || true)"
      [[ "$computed_stderr_sha" == "$recorded_stderr_sha" ]] ||
        fail "$repo_root/$record: recorded stderr_sha256 disagrees with the raw mutation stderr"
    else
      fail "${files[$card]}: mutation $n raw stderr is missing: $mut_red_stderr"
    fi
    [[ -f "$repo_root/$mut_restore" || -L "$repo_root/$mut_restore" ]] || {
      fail "${files[$card]}: mutation restore is missing: $mut_restore"
      continue
    }
    [[ -f "$repo_root/$mut_restore" && ! -L "$repo_root/$mut_restore" ]] || {
      fail "$repo_root/$mut_restore: mutation restore must be a regular non-symlink"
      continue
    }
    if ! restore_flat="$(json_flatten "$repo_root/$mut_restore" 2>/dev/null)"; then
      fail "$repo_root/$mut_restore: invalid mutation restore JSON"
      continue
    fi
    require_json_value "$repo_root/$mut_restore" "$restore_flat" schema aurum.acceptance-execution
    require_json_value "$repo_root/$mut_restore" "$restore_flat" card_id "$card"
    require_json_value "$repo_root/$mut_restore" "$restore_flat" exit_code 0
    require_json_value "$repo_root/$mut_restore" "$restore_flat" observation_trusted false
    restore_sha="$(json_get "$restore_flat" stdout_sha256 2>/dev/null || true)"
    [[ "$restore_sha" == "$recorded_sha" ]] ||
      fail "$repo_root/$mut_restore: restoration must reproduce the identical acceptance stdout (got $restore_sha, want $recorded_sha)"
    mut_restore_stdout=".board/evidence/$card/mutations/MUT-$(printf '%03d' "$n")-restore.stdout.txt"
    if [[ -f "$repo_root/$mut_restore_stdout" && ! -L "$repo_root/$mut_restore_stdout" ]]; then
      computed_restore_sha="sha256:$(sha256sum "$repo_root/$mut_restore_stdout" | awk '{print $1}')"
      [[ "$computed_restore_sha" == "$restore_sha" ]] ||
        fail "$repo_root/$mut_restore: restore stdout file disagrees with its recorded digest"
    else
      fail "${files[$card]}: mutation $n restore raw stdout is missing: $mut_restore_stdout"
    fi
  done

  # Two blind reports: the code reviewer (correctness/design) and the delivery
  # validator (adversarial security/resilience). Both must exist and pass.
  review_a="$evidence_root/reviews/reviewer-a.md"
  review_b="$evidence_root/reviews/reviewer-b.md"
  for review in "$review_a" "$review_b"; do
    [[ -f "$review" && ! -L "$review" && -s "$review" ]] ||
      fail "${files[$card]}: a blind review report is missing or empty: $review"
  done
  grep -Eiq 'APPROVE|approve|^.*Verdict[[:space:]]*:[[:space:]]*pass' "$review_a" ||
    fail "${files[$card]}: reviewer-a report has no clear passing verdict"
  grep -Eiq 'APPROVE|approve|^.*Verdict[[:space:]]*:[[:space:]]*pass' "$review_b" ||
    fail "${files[$card]}: reviewer-b report has no clear passing verdict"

  # `done` additionally crosses the second reader over the same candidate.
  if [[ "$state" == "done" ]]; then
    validate_second_reader_bundle "$card" "sha256:${card_spec_digests[$card]#sha256:}" "$flattened" "" || true
  fi
  return 0
}

validate_manifest() {
  local card="$1"
  local state="$2"
  local manifest="$board_dir/evidence/$card/manifest.json"
  local flattened identity candidate_path value report_path report_sha report_identity
  local candidate_digest_input computed_identity manifest_field
  local review_a_context review_b_context skeptic_context
  local review_a_session review_b_session skeptic_session
  local review_a_backend review_b_backend skeptic_backend independence_level
  local acceptance_context acceptance_session acceptance_backend expected_command_digest
  local -a allowed_manifest_fields=(
    schema version card_id clean_tree candidate_identity_digest candidate_identity
    provenance gates tdd reviews skeptic acceptance approval
    evidence_hashes_complete evidence_chain_digest evidence_hashes
  )

  [[ -f "$manifest" && ! -L "$manifest" ]] || {
    fail "${files[$card]}: $state card lacks a regular evidence/$card/manifest.json"
    return
  }
  if grep -Eiq '"(api[_-]?key|access[_-]?token|authorization|credential|raw[_-]?prompt|model[_-]?response|chain[_-]?of[_-]?thought)"[[:space:]]*:' "$manifest"; then
    fail "$manifest: forbidden sensitive/raw model field"
  fi
  if grep -Eiq -- '(-----BEGIN[[:space:]][A-Z ]*PRIVATE KEY-----|AKIA[0-9A-Z]{16}|sk-[A-Za-z0-9_-]{20,}|gh[pousr]_[A-Za-z0-9]{20,})' "$manifest"; then
    fail "$manifest: contains a credential-like value"
  fi
  if ! flattened="$(json_flatten "$manifest" 2>/dev/null)"; then
    fail "$manifest: invalid JSON or duplicate key"
    return
  fi

  # A human gate is not part of this board. Every card is proved by agent-driven
  # functional validation plus a skeptical mutation, and `done` is judged on the
  # recomputable evidence bundle alone. Only account creation and budget go to a
  # human, and those never travel as a manifest field — they belong in
  # `cards/blocked-on-owner`. A `human_approval` object is therefore an
  # unverifiable claim wherever it appears.
  if [[ -n "$(json_type "$flattened" human_approval 2>/dev/null || true)" ]]; then
    fail "$manifest: human_approval is not an accepted gate; prove the behavior and its skeptical mutation instead"
  fi
  # Every manifest field is recomputed or cross-checked below. An unrecognized
  # top-level field is a self-declared claim with no verifier behind it, so it
  # is refused instead of silently ignored.
  while IFS= read -r manifest_field; do
    [[ -n "$manifest_field" ]] || continue
    # `human_approval` is adjudicated by the dedicated check above, which states
    # the missing verifier instead of the missing recomputation.
    case " ${allowed_manifest_fields[*]} human_approval " in
      *" $manifest_field "*) ;;
      *) fail "$manifest: $manifest_field is a self-declared field the coordinator cannot recompute" ;;
    esac
  done < <(awk -F '\t' '$1 != "" && $1 !~ /\./ && $1 !~ /\[/ { print $1 }' <<< "$flattened" | sort -u)

  require_json_value "$manifest" "$flattened" schema aurum.evidence-manifest
  require_json_value "$manifest" "$flattened" version 1
  require_json_value "$manifest" "$flattened" card_id "$card"
  require_json_value "$manifest" "$flattened" clean_tree true
  require_json_value "$manifest" "$flattened" gates.status pass
  require_json_value "$manifest" "$flattened" gates.fail_closed true
  require_json_value "$manifest" "$flattened" gates.secret_canary pass
  require_json_value "$manifest" "$flattened" gates.supply_chain pass
  require_json_pattern "$manifest" "$flattened" gates.coverage_manifest_digest '^sha256:[0-9a-f]{64}$'
  require_json_pattern "$manifest" "$flattened" gates.container_profile_digest '^sha256:[0-9a-f]{64}$'
  require_json_value "$manifest" "$flattened" tdd.acceptance_tests_locked true
  require_json_value "$manifest" "$flattened" tdd.red.behavioral_failure_verified true
  require_json_value "$manifest" "$flattened" tdd.green.status pass
  require_json_value "$manifest" "$flattened" tdd.refactor.status pass
  require_json_value "$manifest" "$flattened" reviews.barrier_sealed true
  require_json_value "$manifest" "$flattened" reviews.sealed_before_reconciliation true
  require_json_pattern "$manifest" "$flattened" reviews.independence_level '^I[1-3]$'
  require_json_value "$manifest" "$flattened" skeptic.challenge_presealed true
  require_json_value "$manifest" "$flattened" approval.unresolved_blockers 0
  require_json_value "$manifest" "$flattened" approval.verdict candidate_approved
  require_json_value "$manifest" "$flattened" evidence_hashes_complete true
  require_json_pattern "$manifest" "$flattened" evidence_chain_digest '^sha256:[0-9a-f]{64}$'
  [[ "$(json_type "$flattened" evidence_hashes 2>/dev/null || true)" == "array" ]] || fail "$manifest: evidence_hashes must be an array"
  validate_evidence_hashes "$card" "$flattened"

  identity="$(json_get "$flattened" candidate_identity_digest 2>/dev/null || true)"
  [[ "$identity" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "$manifest: candidate_identity_digest is missing or malformed"
  require_json_value "$manifest" "$flattened" candidate_identity.schema CandidateIdentityV1
  require_json_value "$manifest" "$flattened" candidate_identity.version 1
  for candidate_path in repository_identity base_tree_digest head_tree_digest change_digest task_spec_digest configuration_digest policy_digest prompt_and_rubric_digest skill_set_digest provider_model_backend_identity_digest toolchain_and_tool_set_digest dependency_lock_digest container_image_set_digest test_manifest_digest role_context_manifest_digest; do
    require_json_pattern "$manifest" "$flattened" "candidate_identity.$candidate_path" '^sha256:[0-9a-f]{64}$'
  done
  require_json_value "$manifest" "$flattened" candidate_identity.task_spec_digest "${card_spec_digests[$card]}"
  require_json_value "$manifest" "$flattened" provenance.base_sha "${card_base_shas[$card]}"
  require_json_pattern "$manifest" "$flattened" provenance.head_sha '^[0-9a-f]{40}([0-9a-f]{24})?$'
  require_json_value "$manifest" "$flattened" gates.test_manifest_digest "$(json_get "$flattened" candidate_identity.test_manifest_digest 2>/dev/null || true)"

  # CandidateIdentity/v1 is canonicalized in this exact field order. Reports
  # bind only to its aggregate digest, preventing identity-formula drift.
  candidate_digest_input="$(
    for candidate_path in repository_identity base_tree_digest head_tree_digest change_digest task_spec_digest configuration_digest policy_digest prompt_and_rubric_digest skill_set_digest provider_model_backend_identity_digest toolchain_and_tool_set_digest dependency_lock_digest container_image_set_digest test_manifest_digest role_context_manifest_digest; do
      value="$(json_get "$flattened" "candidate_identity.$candidate_path" 2>/dev/null || true)"
      printf '%s=%s\n' "$candidate_path" "$value"
    done
  )"
  computed_identity="sha256:$(printf '%s\n' "$candidate_digest_input" | sha256sum | awk '{print $1}')"
  [[ "$identity" == "$computed_identity" ]] || fail "$manifest: candidate_identity_digest does not match canonical CandidateIdentity/v1"

  # Nonce independence is scoped to one bundle, so the map is emptied before the
  # three roles of this card are read and a collision is always reported against
  # the manifest that contains it.
  bundle_role_nonces=()
  for role in a b; do
    report_path="$(json_get "$flattened" "reviews.$role.path" 2>/dev/null || true)"
    report_sha="$(json_get "$flattened" "reviews.$role.sha256" 2>/dev/null || true)"
    report_identity="$(json_get "$flattened" "reviews.$role.candidate_identity_digest" 2>/dev/null || true)"
    [[ "$report_sha" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "$manifest: reviews.$role.sha256 is missing or malformed"
    [[ "$report_identity" == "$identity" ]] || fail "$manifest: reviews.$role is bound to a different CandidateIdentity"
    require_json_value "$manifest" "$flattened" "reviews.$role.sealed" true
    require_json_pattern "$manifest" "$flattened" "reviews.$role.context_digest" '^sha256:[0-9a-f]{64}$'
    require_json_pattern "$manifest" "$flattened" "reviews.$role.session_digest" '^sha256:[0-9a-f]{64}$'
    require_json_pattern "$manifest" "$flattened" "reviews.$role.backend_family_digest" '^sha256:[0-9a-f]{64}$'
    validate_evidence_artifact "$card" "$report_path" "$report_sha" "$identity" aurum.review-report "reviewer-$role" "$(json_get "$flattened" "reviews.$role.context_digest" 2>/dev/null || true)" "$(json_get "$flattened" "reviews.$role.backend_family_digest" 2>/dev/null || true)" "$(json_get "$flattened" reviews.independence_level 2>/dev/null || true)"
  done
  review_a_context="$(json_get "$flattened" reviews.a.context_digest 2>/dev/null || true)"
  review_b_context="$(json_get "$flattened" reviews.b.context_digest 2>/dev/null || true)"
  review_a_session="$(json_get "$flattened" reviews.a.session_digest 2>/dev/null || true)"
  review_b_session="$(json_get "$flattened" reviews.b.session_digest 2>/dev/null || true)"
  review_a_backend="$(json_get "$flattened" reviews.a.backend_family_digest 2>/dev/null || true)"
  review_b_backend="$(json_get "$flattened" reviews.b.backend_family_digest 2>/dev/null || true)"
  independence_level="$(json_get "$flattened" reviews.independence_level 2>/dev/null || true)"
  [[ "$review_a_context" != "$review_b_context" ]] || fail "$manifest: reviewers A and B reuse the same context"
  [[ "$review_a_session" != "$review_b_session" ]] || fail "$manifest: reviewers A and B reuse the same session"
  if [[ "$independence_level" =~ ^I[23]$ && "$review_a_backend" == "$review_b_backend" ]]; then
    fail "$manifest: I2/I3 claimed with the same backend family"
  fi
  # I3 is the level whose independence comes from an authenticated human. No
  # verifier for that exists, so the level stays unclaimable in every state.
  if [[ "$independence_level" == "I3" ]]; then
    fail "$manifest: I3 cannot be claimed before authenticated human approval"
  fi
  report_path="$(json_get "$flattened" skeptic.path 2>/dev/null || true)"
  report_sha="$(json_get "$flattened" skeptic.sha256 2>/dev/null || true)"
  report_identity="$(json_get "$flattened" skeptic.candidate_identity_digest 2>/dev/null || true)"
  [[ "$report_sha" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "$manifest: skeptic.sha256 is missing or malformed"
  [[ "$report_identity" == "$identity" ]] || fail "$manifest: skeptic is bound to a different CandidateIdentity"
  require_json_value "$manifest" "$flattened" skeptic.sealed true
  require_json_pattern "$manifest" "$flattened" skeptic.context_digest '^sha256:[0-9a-f]{64}$'
  require_json_pattern "$manifest" "$flattened" skeptic.session_digest '^sha256:[0-9a-f]{64}$'
  require_json_pattern "$manifest" "$flattened" skeptic.backend_family_digest '^sha256:[0-9a-f]{64}$'
  skeptic_context="$(json_get "$flattened" skeptic.context_digest 2>/dev/null || true)"
  skeptic_session="$(json_get "$flattened" skeptic.session_digest 2>/dev/null || true)"
  skeptic_backend="$(json_get "$flattened" skeptic.backend_family_digest 2>/dev/null || true)"
  [[ "$skeptic_context" != "$review_a_context" && "$skeptic_context" != "$review_b_context" ]] || fail "$manifest: skeptic reuses a reviewer context"
  [[ "$skeptic_session" != "$review_a_session" && "$skeptic_session" != "$review_b_session" ]] || fail "$manifest: skeptic reuses a reviewer session"
  validate_evidence_artifact "$card" "$report_path" "$report_sha" "$identity" aurum.skeptic-report skeptic "$skeptic_context" "$skeptic_backend"

  # The `done` boundary is crossed on evidence, never on a declaration. On top
  # of everything a `review` bundle already proves, the bundle must contain the
  # acceptance run itself: the observed exit codes of the exact command the card
  # locks, that command failing under the skeptical mutation, and a restored
  # clean replay, sealed under the same CandidateIdentityV1 as the three reports
  # and under a fourth role and nonce that none of them used.
  if [[ "$state" == "done" ]]; then
    expected_command_digest="sha256:$(printf '%s' "${card_accept_commands[$card]:-}" | sha256sum | awk '{print $1}')"
    report_path="$(json_get "$flattened" acceptance.path 2>/dev/null || true)"
    report_sha="$(json_get "$flattened" acceptance.sha256 2>/dev/null || true)"
    report_identity="$(json_get "$flattened" acceptance.candidate_identity_digest 2>/dev/null || true)"
    acceptance_context="$(json_get "$flattened" acceptance.context_digest 2>/dev/null || true)"
    acceptance_session="$(json_get "$flattened" acceptance.session_digest 2>/dev/null || true)"
    acceptance_backend="$(json_get "$flattened" acceptance.backend_family_digest 2>/dev/null || true)"
    [[ "$report_sha" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "$manifest: acceptance.sha256 is missing or malformed"
    [[ "$report_identity" == "$identity" ]] || fail "$manifest: acceptance is bound to a different CandidateIdentity"
    [[ -n "${card_expected_artifacts[$card]:-}" && "$report_path" == "${card_expected_artifacts[$card]}" ]] ||
      fail "$manifest: acceptance.path is not the expected artifact the card locks"
    require_json_value "$manifest" "$flattened" acceptance.sealed true
    require_json_value "$manifest" "$flattened" acceptance.command_digest "$expected_command_digest"
    require_json_pattern "$manifest" "$flattened" acceptance.context_digest '^sha256:[0-9a-f]{64}$'
    require_json_pattern "$manifest" "$flattened" acceptance.session_digest '^sha256:[0-9a-f]{64}$'
    require_json_pattern "$manifest" "$flattened" acceptance.backend_family_digest '^sha256:[0-9a-f]{64}$'
    [[ "$acceptance_context" != "$review_a_context" && "$acceptance_context" != "$review_b_context" && "$acceptance_context" != "$skeptic_context" ]] ||
      fail "$manifest: the acceptance run reuses a review context"
    [[ "$acceptance_session" != "$review_a_session" && "$acceptance_session" != "$review_b_session" && "$acceptance_session" != "$skeptic_session" ]] ||
      fail "$manifest: the acceptance run reuses a review session"
    validate_evidence_artifact "$card" "$report_path" "$report_sha" "$identity" \
      aurum.acceptance-observation acceptance "$acceptance_context" "$acceptance_backend" '' \
      "$(json_get "$flattened" gates.container_profile_digest 2>/dev/null || true)"

    # The acceptance observation above records ONE engine: the card's own
    # program inside the pinned sandbox. The second reader is the other one, and
    # without it `done` was decided by a single reading.
    validate_second_reader_bundle "$card" "$identity" "$flattened" \
      "$review_a_context" "$review_b_context" "$skeptic_context" "$acceptance_context"
  fi
}

# A card moves to `cancelled` only under evidence, never a rename. The owner's
# rule ("you don't delete a card, you send it to cancelled") is enforced here:
# the manager's approval, a non-generic reason, an explicit supersession
# decision, and a digest binding that decision to the exact cancelled card
# text (the same self-referential recomputation `spec_digest` already uses)
# must all be present under .board/evidence/<id>/cancellation.json.
cancellation_reason_min_len=40

is_generic_cancellation_reason() {
  local reason_lower="$1"
  [[ "$reason_lower" =~ ^(it[[:space:]]+is[[:space:]]+|this[[:space:]]+is[[:space:]]+|this[[:space:]]+card[[:space:]]+is[[:space:]]+|the[[:space:]]+card[[:space:]]+is[[:space:]]+)?(not[[:space:]]+needed|no[[:space:]]+longer[[:space:]]+needed|not[[:space:]]+necessary|unnecessary|obsolete|no[[:space:]]+longer[[:space:]]+required|deprecated|duplicate|superseded|replaced|cancell?ed|wont[[:space:]]+fix|won.t[[:space:]]+fix)([[:space:]]+(now|anymore|any[[:space:]]+longer))?$ ]]
}

validate_cancellation() {
  local card="$1"
  local id="$2"
  local evidence_root="$board_dir/evidence/$id"
  local cancellation_file="$evidence_root/cancellation.json"
  local flattened reason reason_compact reason_lower digest_value successor successor_type

  if [[ -e "$board_dir/evidence/$id/manifest.json" ]]; then
    fail "$card: cancelled card must not carry a done evidence bundle (.board/evidence/$id/manifest.json); cancellation is not completion"
  fi

  [[ -f "$cancellation_file" && ! -L "$cancellation_file" ]] || {
    fail "$card: cancelled card lacks a regular .board/evidence/$id/cancellation.json"
    return
  }
  if grep -Eiq '"(api[_-]?key|access[_-]?token|authorization|credential|raw[_-]?prompt|model[_-]?response|chain[_-]?of[_-]?thought)"[[:space:]]*:' "$cancellation_file"; then
    fail "$cancellation_file: forbidden sensitive/raw model field"
  fi
  if grep -Eiq -- '(-----BEGIN[[:space:]][A-Z ]*PRIVATE KEY-----|AKIA[0-9A-Z]{16}|sk-[A-Za-z0-9_-]{20,}|gh[pousr]_[A-Za-z0-9]{20,})' "$cancellation_file"; then
    fail "$cancellation_file: contains a credential-like value"
  fi
  if ! flattened="$(json_flatten "$cancellation_file" 2>/dev/null)"; then
    fail "$cancellation_file: invalid JSON or duplicate key"
    return
  fi

  require_json_value "$cancellation_file" "$flattened" schema aurum.cancellation
  require_json_value "$cancellation_file" "$flattened" version 1
  require_json_value "$cancellation_file" "$flattened" card_id "$id"
  require_json_value "$cancellation_file" "$flattened" approved_by_role manager
  require_json_pattern "$cancellation_file" "$flattened" card_digest '^sha256:[0-9a-f]{64}$'
  digest_value="$(json_get "$flattened" card_digest 2>/dev/null || true)"
  [[ -n "${card_spec_digests[$id]:-}" && "$digest_value" == "${card_spec_digests[$id]}" ]] ||
    fail "$cancellation_file: card_digest does not match the canonical recomputed digest of the cancelled card text"

  reason="$(json_get "$flattened" reason 2>/dev/null || true)"
  reason_compact="$(printf '%s' "$reason" | tr -d '[:space:]')"
  reason_lower="$(printf '%s' "$reason" | tr '[:upper:]' '[:lower:]' | sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//; s/[.;,!]+$//; s/[[:space:]]+/ /g')"
  if (( ${#reason_compact} < cancellation_reason_min_len )); then
    fail "$cancellation_file: reason must be a specific, non-generic explanation of at least $cancellation_reason_min_len characters"
  elif is_generic_text "$reason"; then
    fail "$cancellation_file: reason contains generic or placeholder language"
  elif is_generic_cancellation_reason "$reason_lower"; then
    fail "$cancellation_file: reason is a generic filler phrase alone, not a specific explanation"
  fi

  successor_type="$(json_type "$flattened" superseded_by 2>/dev/null || true)"
  if [[ "$successor_type" == "null" ]]; then
    card_superseded_by[$id]="null"
  else
    successor="$(json_get "$flattened" superseded_by 2>/dev/null || true)"
    if [[ "$successor" =~ ^AUR-[0-9]{3}$ ]]; then
      [[ "$successor" != "$id" ]] || fail "$cancellation_file: superseded_by must not name the cancelled card itself"
      card_superseded_by[$id]="$successor"
    else
      fail "$cancellation_file: superseded_by must be JSON null or one AUR-NNN card id"
    fi
  fi
}

for state in "${states[@]}"; do
  directory="$board_dir/cards/$state"
  [[ -d "$directory" ]] || fail "missing state directory: cards/$state"
done
while IFS= read -r -d '' directory; do
  state="$(basename "$directory")"
  case " ${states[*]} " in
    *" $state "*) ;;
    *) fail "unknown card state directory: cards/$state" ;;
  esac
done < <(find "$board_dir/cards" -mindepth 1 -maxdepth 1 -type d -print0 | sort -z)
while IFS= read -r -d '' unexpected_file; do
  [[ "$(basename "$unexpected_file")" == ".gitkeep" ]] || fail "unexpected file in card state directory: $unexpected_file"
done < <(find "$board_dir/cards" -mindepth 2 -maxdepth 2 -type f ! -name 'AUR-*.md' -print0 | sort -z)
if find "$board_dir/cards" -type l -print -quit | grep -q .; then
  fail "cards tree must not contain symlinks"
fi

while IFS= read -r -d '' card; do
  filename="$(basename "$card")"
  state="$(basename "$(dirname "$card")")"
  id="${filename%.md}"

  [[ "$id" =~ ^AUR-[0-9]{3}$ ]] || fail "$card: filename must be AUR-NNN.md"
  if grep -Eiq '(tokens truncated|warning:[[:space:]]+truncated output|original token count)' "$card"; then
    fail "$card: contains a tool-output truncation marker"
  fi
  if [[ -n "${ids[$id]+x}" ]]; then
    fail "$card: duplicate id also found at ${files[$id]}"
  fi
  ids[$id]=1
  files[$id]="$card"
  card_states[$id]="$state"
  # Preload the card text once per card. Every frontmatter/section helper is
  # invoked inside a $(...) subshell, so the pure-bash cache cannot persist
  # across calls by itself -- but a subshell *inherits* the parent's globals,
  # so populating the cache here (one read, one mapfile) makes all ~40 helper
  # calls per card reuse it instead of re-reading the file.
  mapfile -t card_lines < "$card"
  card_cache_file="$card"

  for field in id version title status office depends_on requirements controls paths forbidden_paths base_sha spec_digest risk data_class trust_boundaries; do
    count="$(frontmatter_count "$card" "$field" 2>/dev/null || true)"
    [[ "$count" == 1 ]] || fail "$card: front matter must contain $field exactly once"
  done
  count="$(frontmatter_count "$card" read_paths 2>/dev/null || true)"
  [[ "$count" -le 1 ]] || fail "$card: optional front matter read_paths may occur at most once"
  field_id="$(frontmatter_value "$card" id 2>/dev/null || true)"
  title="$(frontmatter_value "$card" title 2>/dev/null || true)"
  field_status="$(frontmatter_value "$card" status 2>/dev/null || true)"
  office="$(frontmatter_value "$card" office 2>/dev/null || true)"
  dependencies="$(frontmatter_value "$card" depends_on 2>/dev/null || true)"
  requirements="$(frontmatter_value "$card" requirements 2>/dev/null || true)"
  controls="$(frontmatter_value "$card" controls 2>/dev/null || true)"
  paths="$(frontmatter_value "$card" paths 2>/dev/null || true)"
  if [[ "$count" -eq 1 ]]; then
    read_paths="$(frontmatter_value "$card" read_paths 2>/dev/null || true)"
  else
    read_paths='[]'
  fi
  forbidden_paths="$(frontmatter_value "$card" forbidden_paths 2>/dev/null || true)"
  risk="$(frontmatter_value "$card" risk 2>/dev/null || true)"
  data_class="$(frontmatter_value "$card" data_class 2>/dev/null || true)"
  trust_boundaries="$(frontmatter_value "$card" trust_boundaries 2>/dev/null || true)"
  spec_version="$(frontmatter_value "$card" version 2>/dev/null || true)"
  base_sha="$(frontmatter_value "$card" base_sha 2>/dev/null || true)"
  spec_digest="$(frontmatter_value "$card" spec_digest 2>/dev/null || true)"

  [[ "$field_id" == "$id" ]] || fail "$card: front matter id differs from filename"
  [[ "$title" =~ ^[^[:space:]].{7,}$ ]] && ! is_generic_text "$title" || fail "$card: title is empty or generic"
  [[ "$field_status" == "$state" ]] || fail "$card: status must match containing directory"
  [[ "$office" =~ ^O[0-9]{2}-[a-z0-9-]+$ ]] || fail "$card: invalid office"
  [[ "$dependencies" =~ ^\[(|AUR-[0-9]{3}(,[[:space:]]AUR-[0-9]{3})*)\]$ ]] || fail "$card: invalid depends_on list"
  [[ "$risk" =~ ^(low|medium|high|critical)$ ]] || fail "$card: invalid risk"
  [[ "$data_class" =~ ^(public|internal|confidential|restricted|mixed)$ ]] || fail "$card: invalid data_class"
  [[ "$spec_version" =~ ^[1-9][0-9]*$ ]] || fail "$card: spec_version must be a positive integer"
  if [[ "$state" =~ ^(backlog|ready|blocked-on-owner)$ ]]; then
    [[ "$base_sha" == "lock-at-execution" ]] || fail "$card: unlocked card base_sha must be lock-at-execution"
    [[ "$spec_digest" == "lock-at-execution" ]] || fail "$card: unlocked card spec_digest must be lock-at-execution"
  else
    [[ "$base_sha" =~ ^[0-9a-f]{40}([0-9a-f]{24})?$ ]] || fail "$card: locked base_sha must be an immutable Git object id"
    [[ "$spec_digest" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "$card: locked spec_digest must be a SHA-256 digest"
  fi
  card_titles[$id]="$title"
  card_offices[$id]="$office"
  card_risks[$id]="$risk"
  card_spec_digests[$id]="$spec_digest"

  if [[ "$state" =~ ^(doing|review|done|cancelled)$ ]]; then
    computed_spec_digest="sha256:$(sed -E 's/^status: .+$/status: STATE/; s/^spec_digest: sha256:[0-9a-f]{64}$/spec_digest: sha256:SELF/' "$card" | sha256sum | awk '{print $1}')"
    [[ "$spec_digest" == "$computed_spec_digest" ]] || fail "$card: spec_digest does not match canonical locked card content"
    card_base_shas[$id]="$base_sha"
  else
    card_base_shas[$id]=""
  fi

  if [[ "$state" == "cancelled" ]]; then
    validate_cancellation "$card" "$id"
  fi

  if ! mapfile -t requirement_list < <(parse_list "$requirements"); then
    fail "$card: requirements must be a non-empty canonical list"
    requirement_list=()
  fi
  [[ "$requirements" =~ ^\[PR-[A-Z]+-[0-9]{3}(,[[:space:]]PR-[A-Z]+-[0-9]{3})*\]$ ]] || fail "$card: requirements list is not canonical"
  (( ${#requirement_list[@]} > 0 )) || fail "$card: requirements must bind at least one product requirement"
  unset seen_requirements
  declare -A seen_requirements=()
  for requirement in "${requirement_list[@]}"; do
    [[ -z "${seen_requirements[$requirement]+x}" ]] || fail "$card: duplicate requirement $requirement"
    seen_requirements[$requirement]=1
    [[ "$requirement" =~ ^PR-[A-Z]+-[0-9]{3}$ ]] || {
      fail "$card: malformed requirement id $requirement"
      continue
    }
    grep -Fq "| $requirement |" "$board_dir/requirements/REQUIREMENTS.md" || fail "$card: unknown product requirement $requirement"
    referenced_requirements[$requirement]=1
  done
  if ! mapfile -t control_list < <(parse_list "$controls"); then
    fail "$card: controls must be a non-empty canonical list"
    control_list=()
  fi
  [[ "$controls" =~ ^\[CR-[A-Z]+-[0-9]{3}(,[[:space:]]CR-[A-Z]+-[0-9]{3})*\]$ ]] || fail "$card: controls list is not canonical"
  (( ${#control_list[@]} > 0 )) || fail "$card: controls must bind at least one CR-* rule"
  unset seen_controls
  declare -A seen_controls=()
  for control in "${control_list[@]}"; do
    [[ -z "${seen_controls[$control]+x}" ]] || fail "$card: duplicate control $control"
    seen_controls[$control]=1
    [[ "$control" =~ ^CR-[A-Z]+-[0-9]{3}$ ]] || {
      fail "$card: malformed control id $control"
      continue
    }
    grep -Fq "\`$control\`" "$board_dir/research/code-review-standards.md" || fail "$card: unknown code-review control $control"
    referenced_controls[$control]=1
  done

  if ! mapfile -t declared_paths < <(parse_list "$paths"); then
    fail "$card: paths must be a non-empty canonical list"
    declared_paths=()
  fi
  canonical_repo_path_list "$paths" false || fail "$card: paths list is not canonical"
  (( ${#declared_paths[@]} > 0 )) || fail "$card: paths must be non-empty"
  if ! mapfile -t readable_paths < <(parse_list "$read_paths"); then
    fail "$card: read_paths must be a canonical list when present"
    readable_paths=()
  fi
  canonical_repo_path_list "$read_paths" true || fail "$card: read_paths list is not canonical"
  if ! mapfile -t denied_paths < <(parse_list "$forbidden_paths"); then
    fail "$card: forbidden_paths must be a non-empty canonical list"
    denied_paths=()
  fi
  canonical_repo_path_list "$forbidden_paths" false || fail "$card: forbidden_paths list is not canonical"
  (( ${#denied_paths[@]} > 0 )) || fail "$card: forbidden_paths must explicitly deny at least one path"

  unset seen_declared_paths seen_denied_paths seen_readable_paths
  declare -A seen_declared_paths=()
  declare -A seen_denied_paths=()
  declare -A seen_readable_paths=()
  for declared_path in "${declared_paths[@]}"; do
    [[ -z "${seen_declared_paths[$declared_path]+x}" ]] || fail "$card: duplicate owned path $declared_path"
    seen_declared_paths[$declared_path]=1
    if ! safe_repo_path "$declared_path"; then
      fail "$card: unsafe or non-repository-relative owned path: $declared_path"
      continue
    fi
    path_owners[$declared_path]="${path_owners[$declared_path]:-} $id"
  done
  for denied_path in "${denied_paths[@]}"; do
    [[ -z "${seen_denied_paths[$denied_path]+x}" ]] || fail "$card: duplicate forbidden path $denied_path"
    seen_denied_paths[$denied_path]=1
    safe_repo_path "$denied_path" || fail "$card: unsafe or non-repository-relative forbidden path: $denied_path"
    for declared_path in "${declared_paths[@]}"; do
      paths_overlap "$declared_path" "$denied_path" && fail "$card: owned and forbidden paths overlap: $declared_path <> $denied_path"
    done
  done
  for readable_path in "${readable_paths[@]}"; do
    [[ -z "${seen_readable_paths[$readable_path]+x}" ]] || fail "$card: duplicate read path $readable_path"
    seen_readable_paths[$readable_path]=1
    safe_repo_path "$readable_path" || {
      fail "$card: unsafe or non-repository-relative read path: $readable_path"
      continue
    }
    read_path_claim_cards+=("$id")
    read_path_claim_paths+=("$readable_path")
    for denied_path in "${denied_paths[@]}"; do
      paths_overlap "$readable_path" "$denied_path" && fail "$card: readable and forbidden paths overlap: $readable_path <> $denied_path"
    done
  done

  if ! mapfile -t boundary_list < <(parse_list "$trust_boundaries"); then
    fail "$card: trust_boundaries must be a non-empty canonical list"
    boundary_list=()
  fi
  [[ "$trust_boundaries" =~ ^\[[^][,]+(,[[:space:]][^][,]+)*\]$ ]] || fail "$card: trust_boundaries list is not canonical"
  (( ${#boundary_list[@]} > 0 )) || fail "$card: trust_boundaries must be explicit"
  unset seen_boundaries
  declare -A seen_boundaries=()
  for boundary in "${boundary_list[@]}"; do
    [[ -z "${seen_boundaries[$boundary]+x}" ]] || fail "$card: duplicate trust boundary $boundary"
    seen_boundaries[$boundary]=1
    [[ "$boundary" =~ ^[A-Za-z0-9][A-Za-z0-9._/-]{2,63}$ ]] || fail "$card: malformed trust boundary $boundary"
  done

  for section in "${required_sections[@]}"; do
section_count="$(count_fixed_lines "$section" "$(<"$card")" || true)"
    [[ "$section_count" -eq 1 ]] || fail "$card: section must occur exactly once: $section"
  done
  for section in "## Outcome" "## Non-goals" "## Preconditions" "## Postconditions" "## Public contract" "## Security and privacy" "## Documentation" "## Compatibility, migration, rollback"; do
    require_meaningful_section "$card" "$section"
  done

  non_goals="$(section_body "$card" "## Non-goals")"
  has_matching_line '^- .+' "$non_goals" || fail "$card: Non-goals must list an explicit exclusion"
  extract_all '- ' "$non_goals" '^- (.*)$'
  non_goal_bullets=("${_extracted[@]}")
  if (( ${#non_goal_bullets[@]} > 0 )); then
    # Bullet 1 is held to zero tolerance (matches current board reality).
    record_spec_owner non_goal_owners "${non_goal_bullets[0]}" "$id" "" "$id"
    # Bullets 2+ were never checked at all; they carry pre-existing debt, so
    # they ratchet against the committed baseline instead of hard-failing.
    for (( ng_index = 1; ng_index < ${#non_goal_bullets[@]}; ng_index++ )); do
      record_spec_owner non_goal_extra_owners "${non_goal_bullets[$ng_index]}" "$id" "ng$((ng_index + 1)):" "$id"
    done
  fi
  scenarios="$(section_body "$card" "## Acceptance scenarios")"
  extract_all '### AC-' "$scenarios" '^### (AC-[0-9]{3}): .*$'
  acceptance_ids=("${_extracted[@]}")
  (( ${#acceptance_ids[@]} > 0 )) || fail "$card: missing named AC-NNN Given/When/Then scenario"
  all_scenario_heading_count="$(count_matching_lines '^### AC-' "$scenarios" || true)"
  [[ "$all_scenario_heading_count" -eq "${#acceptance_ids[@]}" ]] || fail "$card: malformed acceptance scenario heading"
  covers_enabled=0
  has_matching_line '^- Covers (requirements|controls):' "$scenarios" && covers_enabled=1
  unset covered_requirement_ids covered_control_ids
  declare -A covered_requirement_ids=()
  declare -A covered_control_ids=()
  for (( scenario_index = 0; scenario_index < ${#acceptance_ids[@]}; scenario_index++ )); do
    printf -v expected_scenario_id 'AC-%03d' "$((scenario_index + 1))"
    scenario_id="${acceptance_ids[$scenario_index]}"
    [[ "$scenario_id" == "$expected_scenario_id" ]] || fail "$card: acceptance scenarios must be unique and sequential from AC-001"
    scenario_block="$(subsection_body "$card" "### $scenario_id:")"
    [[ "$(count_matching_lines '^- Given: .+' "$scenario_block" || true)" -eq 1 ]] || fail "$card: $scenario_id must contain exactly one Given"
    [[ "$(count_matching_lines '^- When: .+' "$scenario_block" || true)" -eq 1 ]] || fail "$card: $scenario_id must contain exactly one When"
    [[ "$(count_matching_lines '^- Then: .+' "$scenario_block" || true)" -eq 1 ]] || fail "$card: $scenario_id must contain exactly one Then"

    scenario_given_text="$(extract_first '- Given: ' "$scenario_block" '^- Given: (.*)$' || true)"
    scenario_when_text="$(extract_first '- When: ' "$scenario_block" '^- When: (.*)$' || true)"
    scenario_then_text="$(extract_first '- Then: ' "$scenario_block" '^- Then: (.*)$' || true)"
    # A Then that restates the Outcome proves nothing the Outcome did not already
    # assert, so the scenario cannot fail for a behavioral reason.
    if [[ -n "$scenario_then_text" ]] &&
       [[ "$(normalize_spec_text "$scenario_then_text")" == "$(normalize_spec_text "$(section_body "$card" '## Outcome')")" ]]; then
      fail "$card: $scenario_id Then restates the Outcome instead of naming an observable result"
    fi
    record_spec_owner scenario_given_owners "$scenario_given_text" "$id/$scenario_id" "" "$id"
    record_spec_owner scenario_when_owners "$scenario_when_text" "$id/$scenario_id" "" "$id"
    record_spec_owner scenario_then_owners "$scenario_then_text" "$id/$scenario_id" "" "$id"
    # `And` was never checked at all; it carries pre-existing debt (the field
    # a `Green` collision migrates to once `Green` itself gets fixed), so it
    # ratchets against the committed baseline instead of hard-failing. The
    # bullet position is folded into the key so And#1 only ever collides with
    # another card's And#1, not with a differently-positioned And.
    extract_all '- And: ' "$scenario_block" '^- And: (.*)$'
    scenario_and_texts=("${_extracted[@]}")
    for (( and_index = 0; and_index < ${#scenario_and_texts[@]}; and_index++ )); do
      record_spec_owner scenario_and_owners "${scenario_and_texts[$and_index]}" "$id/$scenario_id" "and$((and_index + 1)):" "$id"
    done
    if (( covers_enabled == 1 )); then
      requirement_covers_count="$(count_matching_lines '^- Covers requirements: .+' "$scenario_block" || true)"
      control_covers_count="$(count_matching_lines '^- Covers controls: .+' "$scenario_block" || true)"
      [[ "$requirement_covers_count" -eq 1 ]] || fail "$card: $scenario_id must contain exactly one Covers requirements list"
      [[ "$control_covers_count" -eq 1 ]] || fail "$card: $scenario_id must contain exactly one Covers controls list"
      scenario_requirements="$(extract_first '- Covers requirements: ' "$scenario_block" '^- Covers requirements: (.*)$' || true)"
      scenario_controls="$(extract_first '- Covers controls: ' "$scenario_block" '^- Covers controls: (.*)$' || true)"
      [[ "$scenario_requirements" =~ ^\[PR-[A-Z]+-[0-9]{3}(,[[:space:]]PR-[A-Z]+-[0-9]{3})*\]$ ]] ||
        fail "$card: $scenario_id Covers requirements list is not canonical"
      [[ "$scenario_controls" =~ ^\[CR-[A-Z]+-[0-9]{3}(,[[:space:]]CR-[A-Z]+-[0-9]{3})*\]$ ]] ||
        fail "$card: $scenario_id Covers controls list is not canonical"
      mapfile -t scenario_requirement_list < <(parse_list "$scenario_requirements" 2>/dev/null || true)
      mapfile -t scenario_control_list < <(parse_list "$scenario_controls" 2>/dev/null || true)
      for requirement in "${scenario_requirement_list[@]}"; do
        [[ -n "${seen_requirements[$requirement]+x}" ]] || fail "$card: $scenario_id covers undeclared requirement $requirement"
        covered_requirement_ids[$requirement]=1
      done
      for control in "${scenario_control_list[@]}"; do
        [[ -n "${seen_controls[$control]+x}" ]] || fail "$card: $scenario_id covers undeclared control $control"
        covered_control_ids[$control]=1
      done
    fi
  done
  if (( covers_enabled == 1 )); then
    for requirement in "${requirement_list[@]}"; do
      [[ -n "${covered_requirement_ids[$requirement]+x}" ]] || fail "$card: declared requirement $requirement is not covered by any AC"
    done
    for control in "${control_list[@]}"; do
      [[ -n "${covered_control_ids[$control]+x}" ]] || fail "$card: declared control $control is not covered by any AC"
    done
  fi

  tdd="$(section_body "$card" "## TDD proof")"
  red="$(first_matching_line '^- Red: .+' "$tdd" || true)"
  green="$(first_matching_line '^- Green: .+' "$tdd" || true)"
  refactor="$(first_matching_line '^- Refactor: .+' "$tdd" || true)"
  test_count="$(count_matching_lines '^- Test: .+' "$tdd" || true)"
  test_line="$(first_matching_line '^- Test: .+' "$tdd" || true)"
  [[ -n "$red" ]] || fail "$card: missing Red proof definition"
  [[ -n "$green" ]] || fail "$card: missing Green proof definition"
  [[ -n "$refactor" ]] || fail "$card: missing Refactor invariant"
  [[ "$test_count" -eq 1 ]] || fail "$card: TDD proof must contain exactly one Test reference"
  is_generic_text "$red" && fail "$card: Red proof is generic rather than a behavioral failure"
  is_generic_text "$green" && fail "$card: Green proof is generic rather than minimum observable behavior"
  record_spec_owner tdd_green_owners "${green#- Green: }" "$id" "" "$id"
  is_generic_text "$refactor" && fail "$card: Refactor proof is generic rather than an invariant"
  if [[ "$test_line" =~ ^-[[:space:]]Test:[[:space:]]\`(.+)::(AC-[0-9]{3}(,AC-[0-9]{3})*)\`\.$ ]]; then
    test_path="${BASH_REMATCH[1]}"
    test_selectors="${BASH_REMATCH[2]}"
    validate_owned_test_path "$card" Acceptance "$test_path" "${declared_paths[@]}"
    unset seen_test_scenarios
    declare -A seen_test_scenarios=()
    old_ifs="$IFS"
    IFS=','
    read -ra test_scenario_list <<< "$test_selectors"
    IFS="$old_ifs"
    for scenario_id in "${test_scenario_list[@]}"; do
      [[ -z "${seen_test_scenarios[$scenario_id]+x}" ]] || fail "$card: duplicate Test selector $scenario_id"
      seen_test_scenarios[$scenario_id]=1
      [[ " ${acceptance_ids[*]} " == *" $scenario_id "* ]] || fail "$card: Test binds unknown acceptance scenario $scenario_id"
    done
    for scenario_id in "${acceptance_ids[@]}"; do
      [[ -n "${seen_test_scenarios[$scenario_id]+x}" ]] || fail "$card: Test does not bind acceptance scenario $scenario_id"
    done
    if [[ -e "$repo_root/$test_path" || -L "$repo_root/$test_path" ]]; then
      [[ -f "$repo_root/$test_path" && ! -L "$repo_root/$test_path" ]] || fail "$card: acceptance Test must be a regular non-symlink: $test_path"
      [[ -x "$repo_root/$test_path" ]] || fail "$card: existing acceptance Test is not executable: $test_path"
    elif [[ "$state" =~ ^(doing|review|done)$ ]]; then
      fail "$card: active card acceptance Test is missing: $test_path"
    fi
  else
    fail "$card: Test must use the exact backtick-delimited tests/path::AC-NNN[,AC-NNN] grammar"
  fi
  for layer in Unit Contract Integration E2E; do
    validate_tdd_layer "$card" "$id" "$state" "$layer" "$tdd" "${declared_paths[@]}"
  done

  validate_acceptance "$card" "$id"
  grep -Eq '^Expected artifact: .+' "$card" || fail "$card: acceptance lacks a concrete expected artifact"
  # Recorded, not trusted: the `done` gate rebuilds the acceptance command
  # digest and the expected artifact path from the locked card itself, so an
  # evidence bundle cannot observe a command or write an artifact the card
  # never promised.
accept_line="$(first_matching_line '^accept: `[^`]+`$' "$(<"$card")" || true)"
  accept_command="${accept_line#accept: \`}"
  card_accept_commands[$id]="${accept_command%\`}"
  expected_artifact_line="$(first_matching_line '^Expected artifact: .+' "$(<"$card")" || true)"
  if [[ "$expected_artifact_line" =~ \`(\.board/evidence/$id/[A-Za-z0-9._/-]+)\` ]]; then
    card_expected_artifacts[$id]="${BASH_REMATCH[1]}"
  else
    card_expected_artifacts[$id]=""
  fi
  card_scenario_ids[$id]="${acceptance_ids[*]}"
  acceptance_body="$(section_body "$card" "## Acceptance")"
  for scenario_id in "${acceptance_ids[@]}"; do
    has_matching_line ".*$scenario_id.*" "$acceptance_body" || fail "$card: Acceptance evidence does not bind $scenario_id"
  done
  skeptical="$(section_body "$card" "## Skeptical mutations")"
  extract_all '### MUT-' "$skeptical" '^(### MUT-[0-9]{3}: AC-[0-9]{3} / [A-Za-z0-9._/-]+)$'
  mutation_headings=("${_extracted[@]}")
  (( ${#mutation_headings[@]} > 0 )) || fail "$card: skeptical mutation lacks a named MUT-NNN hypothesis mapped to AC and trust boundary"
  all_mutation_heading_count="$(count_matching_lines '^### MUT-' "$skeptical" || true)"
  [[ "$all_mutation_heading_count" -eq "${#mutation_headings[@]}" ]] || fail "$card: malformed skeptical mutation heading"
  card_mutation_counts[$id]="${#mutation_headings[@]}"
  unset covered_scenarios covered_boundaries
  declare -A covered_scenarios=()
  declare -A covered_boundaries=()
  for (( mutation_index = 0; mutation_index < ${#mutation_headings[@]}; mutation_index++ )); do
    printf -v mutation_id 'MUT-%03d' "$((mutation_index + 1))"
    mutation_heading="${mutation_headings[$mutation_index]}"
    [[ "$mutation_heading" == "### $mutation_id:"* ]] || fail "$card: skeptical mutations must be unique and sequential from MUT-001"
    mapped_scenario="$(extract_first '### MUT-' "$mutation_heading" '^### MUT-[0-9]{3}: (AC-[0-9]{3}) / .*$' || true)"
    mapped_boundary="$(extract_first '### MUT-' "$mutation_heading" '^### MUT-[0-9]{3}: AC-[0-9]{3} / (.*)$' || true)"
    [[ " ${acceptance_ids[*]} " == *" $mapped_scenario "* ]] || fail "$card: $mutation_id maps unknown scenario $mapped_scenario"
    [[ " ${boundary_list[*]} " == *" $mapped_boundary "* ]] || fail "$card: $mutation_id maps unknown trust boundary $mapped_boundary"
    covered_scenarios[$mapped_scenario]=1
    covered_boundaries[$mapped_boundary]=1
    mutation_block="$(subsection_body "$card" "### $mutation_id:")"
    [[ "$(count_matching_lines '^- Change: .+' "$mutation_block" || true)" -eq 1 ]] || fail "$card: $mutation_id must contain exactly one reversible Change"
    [[ "$(count_matching_lines '^- Expected: .+' "$mutation_block" || true)" -eq 1 ]] || fail "$card: $mutation_id must contain exactly one falsifiable Expected result"
    mutation_change="$(first_matching_line '^- Change: .+' "$mutation_block" || true)"
    mutation_expected="$(first_matching_line '^- Expected: .+' "$mutation_block" || true)"
    is_generic_text "$mutation_change" && fail "$card: $mutation_id Change is generic or invalid"
    is_generic_text "$mutation_expected" && fail "$card: $mutation_id Expected result accepts an invalid failure mode"
  done
  for scenario_id in "${acceptance_ids[@]}"; do
    [[ -n "${covered_scenarios[$scenario_id]+x}" ]] || fail "$card: no skeptical mutation covers $scenario_id"
  done
  for boundary in "${boundary_list[@]}"; do
    [[ -n "${covered_boundaries[$boundary]+x}" ]] || fail "$card: no skeptical mutation covers trust boundary $boundary"
  done

  review="$(section_body "$card" "## Review")"
  reviewer_a="$(first_matching_line '^- Reviewer A: .+' "$review" || true)"
  reviewer_b="$(first_matching_line '^- Reviewer B: .+' "$review" || true)"
  [[ -n "$reviewer_a" ]] || fail "$card: missing reviewer A lens"
  [[ -n "$reviewer_b" ]] || fail "$card: missing reviewer B lens"
  has_matching_line '^- Skeptical approver: .+' "$review" || fail "$card: missing skeptical approver lens"
  has_matching_line '^- Independence: .+' "$review" || fail "$card: missing explicit reviewer independence contract"
  [[ "$reviewer_a" =~ (every-hunk|all[[:space:]]+hunks|todos[[:space:]]+os[[:space:]]+hunks|cada[[:space:]]+hunk) ]] || fail "$card: Reviewer A must cover every hunk, not a partial specialty review"
  [[ "$reviewer_b" =~ (every-hunk|all[[:space:]]+hunks|todos[[:space:]]+os[[:space:]]+hunks|cada[[:space:]]+hunk) ]] || fail "$card: Reviewer B must cover every hunk, not a partial specialty review"
  [[ "$reviewer_a" =~ ((10|ten)[[:space:]-]+dimension|(10|dez)[[:space:]]+dimens) ]] || fail "$card: Reviewer A must cover all ten protocol dimensions"
  [[ "$reviewer_b" =~ ((10|ten)[[:space:]-]+dimension|(10|dez)[[:space:]]+dimens) ]] || fail "$card: Reviewer B must cover all ten protocol dimensions"
  has_matching_line "\.board/evidence/$id/" "$(<"$card")" || fail "$card: expected artifact is not card-scoped"
done < <(find "$board_dir/cards" -mindepth 2 -maxdepth 2 -type f -name 'AUR-*.md' -print0 | sort -z)

(( ${#ids[@]} > 0 )) || fail "board contains no cards"

# `superseded_by` is only safe to resolve once every card id on the board is
# known, which is not true mid-way through the card pass above (state
# directories are walked in sorted order, so `cancelled` cards can be visited
# before a later-sorted state that holds their declared successor).
for cancelled_id in "${!ids[@]}"; do
  [[ "${card_states[$cancelled_id]}" == "cancelled" ]] || continue
  successor_id="${card_superseded_by[$cancelled_id]:-}"
  [[ -n "$successor_id" && "$successor_id" != "null" ]] || continue
  [[ -n "${ids[$successor_id]+x}" ]] ||
    fail "${files[$cancelled_id]}: cancellation superseded_by references unknown card $successor_id"
done

# Reverse traceability is as important as rejecting unknown references. A
# registry row with no card is an unimplemented product/control claim.
while IFS= read -r requirement; do
  [[ -n "$requirement" ]] || continue
  [[ -n "${referenced_requirements[$requirement]+x}" ]] ||
    fail "requirements registry contains orphan product requirement $requirement"
done < <(
  awk -F '|' '
    {
      value=$2
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
      if (value ~ /^PR-[A-Z]+-[0-9][0-9][0-9]$/) print value
    }
  ' "$board_dir/requirements/REQUIREMENTS.md" | sort -u
)
while IFS= read -r control; do
  [[ -n "$control" ]] || continue
  [[ -n "${referenced_controls[$control]+x}" ]] ||
    fail "code-review standards contain orphan control $control"
done < <(sed -n 's/^`\(CR-[A-Z][A-Z]*-[0-9][0-9][0-9]\)`.*/\1/p' "$board_dir/research/code-review-standards.md" | sort -u)

while IFS= read -r -d '' card; do
  id="$(basename "$card" .md)"
  dependency_field="$(frontmatter_value "$card" depends_on 2>/dev/null || true)"
  if [[ ! "$dependency_field" =~ ^\[(|AUR-[0-9]{3}(,[[:space:]]AUR-[0-9]{3})*)\]$ ]]; then
    card_dependencies[$id]=""
    continue
  fi
  dependencies="$dependency_field"
  dependencies="${dependencies#[}"
  dependencies="${dependencies%]}"
  card_dependencies[$id]="$dependencies"
  if [[ "${card_states[$id]}" == "backlog" && -z "$dependencies" ]]; then
    fail "$card: an unblocked card belongs in ready, not backlog"
  fi
  [[ -z "$dependencies" ]] && continue
  old_ifs="$IFS"
  IFS=','
  read -ra dependency_list <<< "$dependencies"
  IFS="$old_ifs"
  for dependency in "${dependency_list[@]}"; do
    dependency="${dependency// /}"
    [[ -n "${ids[$dependency]+x}" ]] || fail "$card: unknown dependency $dependency"
    [[ "$dependency" != "$id" ]] || fail "$card: self dependency"
    dependency_state="${card_states[$dependency]:-missing}"
    # A cancelled dependency is never satisfiable by waiting -- it will never
    # reach `done` -- so it is an error in every active state, including
    # backlog, where an ordinary not-yet-done dependency is normal. The only
    # escape is the cancelled card declaring a successor AND this card also
    # depending on that successor, so the DAG is never silently orphaned.
    if [[ "$dependency_state" == "cancelled" ]]; then
      if [[ "${card_states[$id]}" =~ ^(backlog|ready|doing|review|done)$ ]]; then
        successor="${card_superseded_by[$dependency]:-}"
        if [[ -z "$successor" || "$successor" == "null" ]]; then
          fail "$card: depends on cancelled $dependency, which declares no accepted successor"
        else
          successor_listed=0
          for check_dependency in "${dependency_list[@]}"; do
            [[ "${check_dependency// /}" != "$successor" ]] || { successor_listed=1; break; }
          done
          (( successor_listed == 1 )) ||
            fail "$card: depends on cancelled $dependency but does not also depend on its declared successor $successor"
        fi
      fi
    elif [[ "${card_states[$id]}" =~ ^(ready|doing|review|done)$ && "$dependency_state" != "done" ]]; then
      fail "$card: active card depends on non-done $dependency"
    fi
  done
done < <(find "$board_dir/cards" -mindepth 2 -maxdepth 2 -type f -name 'AUR-*.md' -print0 | sort -z)

visit() {
  local id="$1"
  local dependency dependencies old_ifs
  case "${visit_state[$id]:-new}" in
    visiting)
      fail "dependency cycle reaches $id"
      return
      ;;
    done)
      return
      ;;
  esac

  visit_state[$id]="visiting"
  dependencies="${card_dependencies[$id]:-}"
  if [[ -n "$dependencies" ]]; then
    old_ifs="$IFS"
    IFS=','
    read -ra dependency_list <<< "$dependencies"
    IFS="$old_ifs"
    for dependency in "${dependency_list[@]}"; do
      dependency="${dependency// /}"
      [[ -n "${ids[$dependency]+x}" ]] && visit "$dependency"
    done
  fi
  visit_state[$id]="done"
}

for id in "${!ids[@]}"; do
  visit "$id"
done

# Unknown dependencies make reachability unsafe; stop rather than attempting to
# prove that overlapping ownership is ordered.
finish_failures

depends_on_transitively() {
  local from="$1"
  local target="$2"
  local key="$from|$target"
  local dependency dependencies old_ifs

  if [[ -n "${reachability[$key]+x}" ]]; then
    [[ "${reachability[$key]}" == 1 ]]
    return
  fi
  if [[ "$from" == "$target" ]]; then
    reachability[$key]=1
    return 0
  fi
  dependencies="${card_dependencies[$from]:-}"
  if [[ -z "$dependencies" ]]; then
    reachability[$key]=0
    return 1
  fi
  old_ifs="$IFS"
  IFS=','
  read -ra dependency_list <<< "$dependencies"
  IFS="$old_ifs"
  for dependency in "${dependency_list[@]}"; do
    dependency="${dependency// /}"
    if depends_on_transitively "$dependency" "$target"; then
      reachability[$key]=1
      return 0
    fi
  done
  reachability[$key]=0
  return 1
}

mapfile -t owned_path_list < <(printf '%s\n' "${!path_owners[@]}" | sort)
for (( path_left = 0; path_left < ${#owned_path_list[@]}; path_left++ )); do
  for (( path_right = path_left; path_right < ${#owned_path_list[@]}; path_right++ )); do
    first_path="${owned_path_list[$path_left]}"
    second_path="${owned_path_list[$path_right]}"
    # The list is byte-sorted. A path precedes its descendants, so once the
    # current right-hand value is neither equal nor under `first_path`, no
    # later value can overlap it. This preserves the fail-closed prefix check
    # without the quadratic scan that made a 300+ card board impractical.
    if [[ "$first_path" != "$second_path" && "$second_path" != "$first_path/"* ]]; then
      break
    fi
    read -ra first_owners <<< "${path_owners[$first_path]}"
    read -ra second_owners <<< "${path_owners[$second_path]}"
    for first in "${first_owners[@]}"; do
      for second in "${second_owners[@]}"; do
        [[ "$first" != "$second" ]] || continue
        # For the same exact key, test a pair only once.
        [[ "$first_path" != "$second_path" || "$first" < "$second" ]] || continue
        if ! depends_on_transitively "$first" "$second" && ! depends_on_transitively "$second" "$first"; then
          fail "overlapping paths $first_path ($first) and $second_path ($second) are concurrently owned by unordered cards"
        fi
      done
    done
  done
done

# A read path is only reviewable material if some card produces it or the tree
# already carries it. Ownership is checked in both directions: a read path may
# be an owned path, sit under one, or be the directory that contains one.
for (( ancestor_index = 0; ancestor_index < ${#owned_path_list[@]}; ancestor_index++ )); do
  ancestor_path="${owned_path_list[$ancestor_index]}"
  while [[ "$ancestor_path" == */* ]]; do
    ancestor_path="${ancestor_path%/*}"
    owned_path_ancestors[$ancestor_path]=1
  done
done

read_path_has_producer() {
  local path="$1"
  local prefix="$path"
  [[ -z "${path_owners[$path]+x}" ]] || return 0
  [[ -z "${owned_path_ancestors[$path]+x}" ]] || return 0
  while [[ "$prefix" == */* ]]; do
    prefix="${prefix%/*}"
    if [[ -n "${path_owners[$prefix]+x}" ]]; then
      # An owned ancestor only produces this path if it can hold children. A
      # prefix the tree already carries as a non-directory never will, so the
      # read path is fabricated rather than pending creation, and claiming it
      # would let any invented path ride in under an owned regular file.
      [[ ! -e "$repo_root/$prefix" || -d "$repo_root/$prefix" ]] || return 1
      return 0
    fi
  done
  return 1
}

for (( claim_index = 0; claim_index < ${#read_path_claim_paths[@]}; claim_index++ )); do
  claimed_path="${read_path_claim_paths[$claim_index]}"
  claim_card="${read_path_claim_cards[$claim_index]}"
  if [[ -z "${read_path_resolution[$claimed_path]+x}" ]]; then
    if read_path_has_producer "$claimed_path"; then
      read_path_resolution[$claimed_path]=owned
    elif [[ -L "$repo_root/$claimed_path" ]]; then
      read_path_resolution[$claimed_path]=symlink
    elif [[ -f "$repo_root/$claimed_path" || -d "$repo_root/$claimed_path" ]]; then
      read_path_resolution[$claimed_path]=present
    else
      read_path_resolution[$claimed_path]=absent
    fi
  fi
  case "${read_path_resolution[$claimed_path]}" in
    absent)
      fail "${files[$claim_card]}: read path is owned by no card and absent from the tree: $claimed_path"
      ;;
    symlink)
      fail "${files[$claim_card]}: read path is owned by no card and resolves through a symlink: $claimed_path"
      ;;
  esac
done

# `paths` and `read_paths` are claims over the source tree, and the board's
# entire disjoint-ownership guarantee is written in their vocabulary. A claim on
# a path the tree does not carry is legitimate in exactly one situation: the
# card has not been executed yet and that artifact is the work still to be done.
# A `backlog` or `ready` card naming a file that does not exist yet is normal and
# must stay silent here.
#
# Two situations are never legitimate, and neither was visible to this gate:
#
#   1. the repository still tracks the path while the working tree no longer
#      carries it. The tree lost an artifact a card owns, which is one lane
#      silently deleting another card's property, and nothing else in this file
#      notices;
#   2. the card sits in `review` or `done`. README.md defines those states as
#      "immutable candidate awaiting independent review" and "fully accepted
#      work", so the artifact the card claims must already be materialized. A
#      review of an absent file reviews nothing.
#
# `doing` is deliberately excluded from case 2. README.md admits a card to
# `doing` once its acceptance test demonstrably fails, and only then does "one
# builder own the card and may change its declared paths": in `doing` the
# declared artifacts are by definition the work that does not exist yet, so
# demanding their presence would paint the gate red for the whole duration of
# every card. The one artifact the ready->doing transition does guarantee is the
# acceptance `Test:` path, and that single path is already required to exist in
# `doing` by the rule above.
#
# Case 1 needs a ground truth for "this path existed", and the only one that is
# not self-declared prose is what Git records as tracked. Two records answer
# that question and each alone is blind to a real deletion: the index forgets
# the path on `git rm`, which drops it from the index and the working tree in
# one step and is exactly how a lane that produces a patch removes a file; the
# commit at `HEAD` never saw a path that was added but not committed yet. A path
# either record still names is property the repository is accountable for, so
# the two are unioned. Git is consulted, never trusted. When a declared path is
# absent and neither record can be read, planned work and a deleted artifact are
# indistinguishable, so the gate reports exactly that and fails instead of
# choosing the friendly reading.
#
# Presence means the tree carries an entry at the path, a symlink entry
# included: this repository tracks legitimate symlinks (`AGENTS.md`), and
# judging a link target is a different rule that needs different evidence.
#
# `forbidden_paths` is deliberately excluded. It denies authority over a path
# instead of claiming authorship of one, and a denied path such as `.env` or
# `secrets` is expected not to exist; demanding its existence would invert the
# field's meaning.
declare -A declared_path_presence=()
declare -A absent_path_claimants=()
declare -A tracked_entries=()
declare -a absent_declared_paths=()

mapfile -t materialization_claim_rows < <(
  {
    for declared_path in "${!path_owners[@]}"; do
      read -ra path_claimants <<< "${path_owners[$declared_path]}"
      for claim_card in "${path_claimants[@]}"; do
        printf '%s\t%s\towned\n' "$declared_path" "$claim_card"
      done
    done
    for (( claim_index = 0; claim_index < ${#read_path_claim_paths[@]}; claim_index++ )); do
      printf '%s\t%s\tread\n' "${read_path_claim_paths[$claim_index]}" "${read_path_claim_cards[$claim_index]}"
    done
  } | sort -u
)

for claim_row in "${materialization_claim_rows[@]}"; do
  IFS=$'\t' read -r claimed_path claim_card claim_kind <<< "$claim_row"
  if [[ -z "${declared_path_presence[$claimed_path]+x}" ]]; then
    if [[ -e "$repo_root/$claimed_path" || -L "$repo_root/$claimed_path" ]]; then
      declared_path_presence[$claimed_path]=present
    else
      declared_path_presence[$claimed_path]=absent
      absent_declared_paths+=("$claimed_path")
    fi
  fi
  [[ "${declared_path_presence[$claimed_path]}" == absent ]] || continue
  absent_path_claimants[$claimed_path]="${absent_path_claimants[$claimed_path]:-}${absent_path_claimants[$claimed_path]:+, }$claim_card"
  [[ "${card_states[$claim_card]:-}" =~ ^(review|done)$ ]] || continue
  fail "${files[$claim_card]}: ${card_states[$claim_card]} card declares $claim_kind path $claimed_path, which is absent from the tree"
done

# A tracked entry makes every directory above it tracked too, because a card may
# declare a directory and the records only ever name files inside it.
record_tracked_entry() {
  local tracked_entry="$1"
  tracked_entries[$tracked_entry]=1
  while [[ "$tracked_entry" == */* ]]; do
    tracked_entry="${tracked_entry%/*}"
    tracked_entries[$tracked_entry]=1
  done
}

if (( ${#absent_declared_paths[@]} > 0 )); then
  if ! git -C "$repo_root" rev-parse --git-dir >/dev/null 2>&1; then
    fail "declared-path materialization is unverifiable: ${#absent_declared_paths[@]} declared path(s) are absent and $repo_root exposes no readable Git index to separate planned work from a deleted artifact"
  else
    index_entry_count=0
    while IFS= read -r -d '' tracked_entry; do
      index_entry_count=$(( index_entry_count + 1 ))
      record_tracked_entry "$tracked_entry"
    done < <(git -C "$repo_root" ls-files -z 2>/dev/null)
    if (( index_entry_count == 0 )); then
      fail "declared-path materialization is unverifiable: the Git index at $repo_root lists no tracked path"
    fi
    # A repository whose `HEAD` is unborn has no committed record to union in,
    # and nothing can have been removed from a commit that does not exist, so
    # the index is then the whole truth. Once `HEAD` resolves to a commit, its
    # tree must be readable: an empty listing there means the record that sees
    # `git rm` is unavailable, which is the same blindness as an unreadable
    # index and is reported rather than assumed away.
    if git -C "$repo_root" rev-parse --verify --quiet HEAD >/dev/null 2>&1; then
      head_tree_entry_count=0
      while IFS= read -r -d '' tracked_entry; do
        head_tree_entry_count=$(( head_tree_entry_count + 1 ))
        record_tracked_entry "$tracked_entry"
      done < <(git -C "$repo_root" ls-tree -r -z --name-only HEAD 2>/dev/null)
      if (( head_tree_entry_count == 0 )); then
        fail "declared-path materialization is unverifiable: the commit at HEAD in $repo_root lists no tracked path"
      fi
    fi
    for declared_path in "${absent_declared_paths[@]}"; do
      [[ -n "${tracked_entries[$declared_path]+x}" ]] || continue
      claim_card="${absent_path_claimants[$declared_path]%%,*}"
      fail "${files[$claim_card]}: declared path is tracked by the repository but missing from the tree: $declared_path (claimed by ${absent_path_claimants[$declared_path]})"
    done
  fi
fi

# BOARD_SPEC_BASELINE_DUMP=1 regenerates the ratchet baseline from the current
# tree instead of enforcing it: maintenance-only, never run as part of the
# normal gate. It intentionally exits before the rest of validate.sh so the
# emitted set is exactly the current pairwise collisions in every field,
# nothing else.
if [[ "${BOARD_SPEC_BASELINE_DUMP:-0}" == "1" ]]; then
  {
    compute_ratchet_pairs scenario_given_owners | sed 's/^/given /'
    compute_ratchet_pairs scenario_when_owners | sed 's/^/when /'
    compute_ratchet_pairs scenario_then_owners | sed 's/^/then /'
    compute_ratchet_pairs tdd_green_owners | sed 's/^/green /'
    compute_ratchet_pairs non_goal_owners | sed 's/^/non_goal /'
    compute_ratchet_pairs scenario_and_owners | sed 's/^/and /'
    compute_ratchet_pairs non_goal_extra_owners | sed 's/^/non_goal_extra /'
  } | sort -u
  exit 0
fi
check_ratcheted_collisions scenario_given_owners "given" "$spec_collision_baseline"
check_ratcheted_collisions scenario_when_owners "when" "$spec_collision_baseline"
check_ratcheted_collisions scenario_then_owners "then" "$spec_collision_baseline"
check_ratcheted_collisions tdd_green_owners "green" "$spec_collision_baseline"
check_ratcheted_collisions non_goal_owners "non_goal" "$spec_collision_baseline"
check_ratcheted_collisions scenario_and_owners "and" "$spec_collision_baseline"
check_ratcheted_collisions non_goal_extra_owners "non_goal_extra" "$spec_collision_baseline"

card_count="${#ids[@]}"
for (( sequence = 1; sequence <= card_count; sequence++ )); do
  printf -v expected_id 'AUR-%03d' "$sequence"
  [[ -n "${ids[$expected_id]+x}" ]] || fail "card sequence has a gap at $expected_id"
done

if [[ -f "$board_dir/INDEX.md" ]]; then
  for id in "${!ids[@]}"; do
    relative="cards/${card_states[$id]}/$id.md"
    index_row="| [$id]($relative) | ${card_states[$id]} | ${card_offices[$id]} | ${card_risks[$id]} | \`[${card_dependencies[$id]}]\` | ${card_titles[$id]} |"
    grep -Fqx "$index_row" "$board_dir/INDEX.md" || fail "INDEX.md lacks the canonical row for $id"
  done
  index_rows="$(grep -Ec '^\| \[AUR-[0-9]{3}\]\(cards/' "$board_dir/INDEX.md" || true)"
  [[ "$index_rows" -eq "$card_count" ]] || fail "INDEX.md has $index_rows card rows, expected $card_count"
else
  fail "missing INDEX.md"
fi

validate_second_reader_legacy_registry
validate_second_reader_legacy_orphans
validate_second_reader_exempt_registry
validate_second_reader_coverage

for id in "${!ids[@]}"; do
  if [[ "${card_states[$id]}" =~ ^(review|done)$ ]]; then
    # Execution-proof is the proof shape for cards validated since the
    # redesign: evidence is recomputed from locked commands and pinned images,
    # never declared in a hand-written manifest. A card that owns a real
    # manifest (legacy sealed bundle) keeps the older gate; its absence selects
    # the execution-proof gate, which re-derives every fact from checked-in
    # programs and raw runs.
    if [[ -f "$repo_root/.board/evidence/$id/manifest.json" && ! -L "$repo_root/.board/evidence/$id/manifest.json" ]]; then
      validate_manifest "$id" "${card_states[$id]}"
    else
      validate_execution_proof "$id" "${card_states[$id]}"
    fi
  fi
done

# The migration is never silent. Every card/layer still covered by a recorded
# legacy run rather than a bundle-sealed one is named on every run, with the
# reason it carries, so the debt cannot decay into an unread file.
if (( ${#second_reader_debt[@]} > 0 )); then
  printf 'board note: second-reader legacy debt: %d card/layer pair(s) proved by a recorded run outside the sealed bundle\n' "${#second_reader_debt[@]}" >&2
  for debt_entry in "${second_reader_debt[@]}"; do
    printf 'board note:   %s\n' "$debt_entry" >&2
  done
fi
if (( ${#second_reader_exempt_debt[@]} > 0 )); then
  printf 'board note: second-reader exemption debt: %d done card(s) hand the second reader nothing to execute\n' "${#second_reader_exempt_debt[@]}" >&2
  for debt_entry in "${second_reader_exempt_debt[@]}"; do
    printf 'board note:   %s\n' "$debt_entry" >&2
  done
fi

finish_failures

# RULE:second-reader-inconclusive
# Law 4 and law 11: a missing engine is neither behavioral red nor a pass. It
# gets its own terminal verdict and its own exit status, so a run that proved
# nothing can never be mistaken -- by an operator or by a caller -- for the run
# that proved everything. `done` is authorized only by exit 0.
if (( ${#second_reader_skipped[@]} > 0 )); then
  printf 'board inconclusive: %d second-reader layer(s) were NOT re-executed; %d were\n' \
    "${#second_reader_skipped[@]}" "$second_reader_executed" >&2
  for debt_entry in "${second_reader_skipped[@]}"; do
    printf 'board inconclusive:   %s\n' "$debt_entry" >&2
  done
  printf 'board inconclusive: structure and raw-log recomputation passed on %d atomic cards, but this run does NOT authorize a transition to done\n' "${#ids[@]}" >&2
  exit 3
fi
# /RULE:second-reader-inconclusive
printf 'board note: second reader re-executed %d recorded layer(s) through .board/bin/second-reader\n' "$second_reader_executed" >&2
printf 'board valid: %d atomic cards\n' "${#ids[@]}"
