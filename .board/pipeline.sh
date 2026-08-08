#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C

board_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd "$board_dir/.." && pwd -P)"

states=(backlog ready doing review validating done blocked-on-owner cancelled)
validation_kinds=(none tested skeptical)

failed=0
failure_count=0

declare -A ids=()
declare -A files=()
declare -A card_states=()
declare -A card_dependencies=()
declare -A card_titles=()
declare -A card_validation=()
declare -A card_status_fm=()
declare -A card_commit=()
declare -A card_review=()
declare -A card_validation_record=()
declare -A card_has_record=()

fail() {
  printf 'board error: %s\n' "$1" >&2
  failed=1
  failure_count=$((failure_count + 1))
}

report_failures() {
  if (( failed == 1 )); then
    printf 'board invalid: %d error(s)\n' "$failure_count" >&2
    exit 1
  fi
}

delivery_evidence_ok() {
  local id="$1" commit="$2" evidence body
  evidence="$board_dir/evidence/$id/validated.json"
  [[ -f "$evidence" && ! -L "$evidence" ]] || return 1
  body="$(< "$evidence")"
  [[ "$body" == *"\"card\": \"$id\""* || "$body" == *"\"card\":\"$id\""* ]] || return 1
  [[ "$body" == *"\"commit\": \"$commit\""* || "$body" == *"\"commit\":\"$commit\""* ]] || return 1
  [[ "$body" == *'"review": "approved"'* || "$body" == *'"review":"approved"'* ]] || return 1
  [[ "$body" == *'"validation": "passed"'* || "$body" == *'"validation":"passed"'* ]] || return 1
  [[ "$body" == *'"exit_code": 0'* || "$body" == *'"exit_code":0'* ]] || return 1
}

for state in "${states[@]}"; do
  [[ -d "$board_dir/cards/$state" ]] || fail "missing state directory: cards/$state"
done
while IFS= read -r -d '' directory; do
  state="$(basename "$directory")"
  case " ${states[*]} " in
    *" $state "*) ;;
    *) fail "unknown card state directory: cards/$state" ;;
  esac
done < <(find "$board_dir/cards" -mindepth 1 -maxdepth 1 -type d -print0 | sort -z)

while IFS= read -r -d '' card; do
  filename="${card##*/}"
  state="${card%/*}"
  state="${state##*/}"
  id="${filename%.md}"

  [[ "$id" =~ ^AUR-[0-9]{3}$ ]] || fail "$card: filename must be AUR-NNN.md"
  if [[ -n "${ids[$id]+x}" ]]; then
    fail "$card: duplicate id also found at ${files[$id]}"
  fi
  ids[$id]=1
  files[$id]="$card"
  card_states[$id]="$state"

  mapfile -t card_lines < "$card"
  declare -A fm_title=()
  local_fence_done=0
  in_fm=0
  for line in "${card_lines[@]}"; do
    if (( in_fm == 0 )); then
      [[ "$line" == "---" ]] && in_fm=1
      continue
    fi
    [[ "$line" == "---" ]] && { in_fm=2; continue; }
    if (( in_fm == 1 )); then
      key="${line%%:*}"
      value="${line#*: }"
      fm_title[$key]="$value"
    elif (( local_fence_done == 0 )) && [[ "$line" == "## Delivery record" ]]; then
      local_fence_done=1
      card_has_record[$id]=1
    elif (( local_fence_done == 1 )); then
      case "$line" in
        '## '*) local_fence_done=2 ;;
        '- commit: '*)
          sha="${line#- commit: }"
          [[ "$sha" =~ ^[0-9a-f]{40}$ ]] && card_commit[$id]="$sha"
          ;;
        '- review: approved'*) card_review[$id]=1 ;;
        '- validation: passed'*) card_validation_record[$id]=1 ;;
      esac
    fi
  done

  [[ -n "${fm_title[id]+x}" ]] || fail "$card: front matter must contain id"
  [[ -n "${fm_title[title]+x}" ]] || fail "$card: front matter must contain title"
  [[ -n "${fm_title[status]+x}" ]] || fail "$card: front matter must contain status"
  [[ -n "${fm_title[depends_on]+x}" ]] || fail "$card: front matter must contain depends_on"

  [[ "${fm_title[id]}" == "$id" ]] || fail "$card: front matter id differs from filename"
  [[ "${fm_title[status]}" == "$state" ]] || fail "$card: status must match containing directory"

  title="${fm_title[title]}"
  [[ "$title" =~ ^[^[:space:]].{7,}$ ]] || fail "$card: title is empty or generic"
  card_titles[$id]="$title"

  dependencies="${fm_title[depends_on]}"
  [[ "$dependencies" =~ ^\[(|AUR-[0-9]{3}(,[[:space:]]AUR-[0-9]{3})*)\]$ ]] ||
    fail "$card: invalid depends_on list"
  dependencies="${dependencies#[}"
  dependencies="${dependencies%]}"
  card_dependencies[$id]="$dependencies"
  if [[ "$state" == "backlog" && -z "$dependencies" ]]; then
    fail "$card: an unblocked card belongs in ready, not backlog"
  fi

  validation="${fm_title[validation]:-none}"
  case " ${validation_kinds[*]} " in
    *" $validation "*) ;;
    *) fail "$card: invalid validation kind: $validation" ;;
  esac
  card_validation[$id]="$validation"

done < <(find "$board_dir/cards" -mindepth 2 -maxdepth 2 -type f -name 'AUR-*.md' -print0 | sort -z)

report_failures

for id in "${!ids[@]}"; do
  state="${card_states[$id]}"
  dependencies="${card_dependencies[$id]:-}"
  [[ -n "$dependencies" ]] || continue
  for dependency in ${dependencies//,/ }; do
    dependency="${dependency// /}"
    [[ -n "$dependency" ]] || continue
    [[ -n "${ids[$dependency]+x}" ]] || fail "${files[$id]}: unknown dependency $dependency"
    [[ "$dependency" != "$id" ]] || fail "${files[$id]}: self dependency"
    dependency_state="${card_states[$dependency]:-missing}"
    if [[ "$state" =~ ^(ready|doing|review|validating|done)$ && "$dependency_state" != "done" ]]; then
      fail "${files[$id]}: active card depends on non-done $dependency"
    fi
  done
done

report_failures

card_count="${#ids[@]}"
for (( sequence = 1; sequence <= card_count; sequence++ )); do
  printf -v expected_id 'AUR-%03d' "$sequence"
  [[ -n "${ids[$expected_id]+x}" ]] || fail "card sequence has a gap at $expected_id"
done

declare -A visit_state=()
visit() {
  local id="$1"
  local dependencies dependency
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
  for dependency in ${dependencies//,/ }; do
    dependency="${dependency// /}"
    [[ -n "${ids[$dependency]+x}" ]] && visit "$dependency"
  done
  visit_state[$id]="done"
}
for id in "${!ids[@]}"; do
  visit "$id"
done

report_failures

for id in "${!ids[@]}"; do
  state="${card_states[$id]}"
  validation="${card_validation[$id]}"
  case "$state" in
    review)
      if (( ${card_has_record[$id]:-0} == 1 )); then
        if [[ -z "${card_commit[$id]+x}" ]]; then
          fail "${files[$id]}: review card with delivery record lacks a 40-hex commit"
        else
          git -C "$repo_root" cat-file -e "${card_commit[$id]}^{commit}" 2>/dev/null ||
            fail "${files[$id]}: review commit does not exist in the repository: ${card_commit[$id]}"
        fi
      fi
      ;;
    validating)
      (( ${card_has_record[$id]:-0} == 1 )) || fail "${files[$id]}: validating card lacks a delivery record"
      if [[ -z "${card_commit[$id]+x}" ]]; then
        fail "${files[$id]}: validating card lacks a 40-hex commit"
      fi
      [[ -n "${card_review[$id]+x}" ]] || fail "${files[$id]}: validating card lacks an approved review"
      [[ "$validation" != "none" ]] || fail "${files[$id]}: validation: none card must not sit in validating"
      if [[ -n "${card_commit[$id]+x}" ]]; then
        git -C "$repo_root" cat-file -e "${card_commit[$id]}^{commit}" 2>/dev/null ||
          fail "${files[$id]}: validating commit does not exist in the repository: ${card_commit[$id]}"
      fi
      ;;
    done)
      if (( ${card_has_record[$id]:-0} == 1 )); then
        if [[ -z "${card_commit[$id]+x}" ]]; then
          fail "${files[$id]}: done card lacks a 40-hex commit"
        fi
        [[ -n "${card_review[$id]+x}" ]] || fail "${files[$id]}: done card lacks an approved review"
        if [[ "$validation" != "none" ]]; then
          [[ -n "${card_validation_record[$id]+x}" ]] ||
            fail "${files[$id]}: done card with validation $validation lacks a passed validation"
          if [[ -n "${card_commit[$id]+x}" ]] && ! delivery_evidence_ok "$id" "${card_commit[$id]}"; then
            fail "${files[$id]}: done card lacks matching validated.json evidence"
          fi
        fi
        if [[ -n "${card_commit[$id]+x}" ]]; then
          git -C "$repo_root" cat-file -e "${card_commit[$id]}^{commit}" 2>/dev/null ||
            fail "${files[$id]}: done commit does not exist in the repository: ${card_commit[$id]}"
        fi
      else
        printf 'board note: legacy done card without delivery record: %s\n' "$id" >&2
      fi
      ;;
  esac
done

report_failures

for state in doing review validating; do
  lane_count=0
  for id in "${!ids[@]}"; do
    [[ "${card_states[$id]}" == "$state" ]] && lane_count=$((lane_count + 1))
  done
  printf 'board lane %s: %d card(s)\n' "$state" "$lane_count"
done
printf 'board valid: %d atomic cards\n' "$card_count"
