#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C

board_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd "$board_dir/.." && pwd -P)"

states=(backlog ready doing review validating done blocked-on-owner cancelled)
validation_kinds=(none tested skeptical)

failed=0
failure_count=0
directory_scan=''
card_scan=''

cleanup_scans() {
  rm -f -- "${directory_scan:-}" "${card_scan:-}"
}
trap cleanup_scans EXIT INT TERM HUP

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
declare -A card_paths=()
declare -A card_read_paths=()

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
  local id="$1" commit="$2" evidence
  evidence="$board_dir/evidence/$id/validated.json"
  [[ -f "$evidence" && ! -L "$evidence" ]] || return 1
  command -v python3 >/dev/null 2>&1 || return 1
  python3 "$board_dir/bin/check-delivery-evidence.py" "$evidence" "$id" "$commit"
}

for state in "${states[@]}"; do
  [[ -d "$board_dir/cards/$state" ]] || fail "missing state directory: cards/$state"
  [[ -n "$(git -C "$repo_root" ls-files -- ".board/cards/$state")" ]] ||
    fail "state directory is not represented in Git: cards/$state"
done
directory_scan="$(mktemp "${TMPDIR:-/tmp}/aurum-pipeline-directories.XXXXXX")"
if ! find "$board_dir/cards" -mindepth 1 -maxdepth 1 -type d -print0 >"$directory_scan"; then
  rm -f -- "$directory_scan"
  directory_scan=''
  fail 'card state directory scan failed'
  report_failures
fi
while IFS= read -r -d '' directory; do
  state="$(basename "$directory")"
  case " ${states[*]} " in
    *" $state "*) ;;
    *) fail "unknown card state directory: cards/$state" ;;
  esac
done <"$directory_scan"
rm -f -- "$directory_scan"
directory_scan=''

card_scan="$(mktemp "${TMPDIR:-/tmp}/aurum-pipeline-cards.XXXXXX")"
if ! find "$board_dir/cards" -mindepth 2 -maxdepth 2 -type f -name 'AUR-*.md' -print0 >"$card_scan"; then
  rm -f -- "$card_scan"
  card_scan=''
  fail 'card scan failed'
  report_failures
fi
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
  case "$state" in
    ready|doing|review|validating)
      [[ -n "${fm_title[validation]+x}" ]] ||
        fail "$card: active card must declare validation explicitly"
      ;;
  esac
  case " ${validation_kinds[*]} " in
    *" $validation "*) ;;
    *) fail "$card: invalid validation kind: $validation" ;;
  esac
  card_validation[$id]="$validation"
  card_paths[$id]="${fm_title[paths]:-}"
  card_read_paths[$id]="${fm_title[read_paths]:-[]}"

done <"$card_scan"
rm -f -- "$card_scan"
card_scan=''

report_failures

path_is_within() {
  [[ "$1" == "$2" || "$1" == "$2/"* ]]
}

tracked_path_exists() {
  local path="$1"
  [[ -e "$repo_root/$path" && ! -L "$repo_root/$path" ]] || return 1
  if [[ -f "$repo_root/$path" ]]; then
    git -C "$repo_root" ls-files --error-unmatch -- "$path" >/dev/null 2>&1
  else
    [[ -n "$(git -C "$repo_root" ls-files -- "$path")" ]]
  fi
}

parse_contract_list() {
  local value="$1" body item old_ifs
  [[ "$value" =~ ^\[(.*)\]$ ]] || return 1
  body="${BASH_REMATCH[1]}"
  [[ -n "$body" ]] || return 0
  old_ifs="$IFS"
  IFS=','
  read -ra items <<< "$body"
  IFS="$old_ifs"
  for item in "${items[@]}"; do
    item="${item# }"
    item="${item% }"
    [[ -n "$item" ]] || return 1
    printf '%s\n' "$item"
  done
}

# A structurally valid card is not executable merely because its YAML parses.
# Review/validation candidates must have every input materialized and must bind
# the declared acceptance command to a registered, digest-locked profile.
for id in "${!ids[@]}"; do
  state="${card_states[$id]}"
  [[ "$state" == review || "$state" == validating ]] || continue
  owned_spec="${card_paths[$id]}"
  read_spec="${card_read_paths[$id]}"
  [[ "$owned_spec" =~ ^\[(|[A-Za-z0-9._/-]+(,[[:space:]]*[A-Za-z0-9._/-]+)*)\]$ ]] || {
    fail "${files[$id]}: paths is not a canonical list"
    continue
  }
  [[ "$owned_spec" != '[]' ]] || {
    fail "${files[$id]}: paths must own at least one artifact"
    continue
  }
  [[ "$read_spec" =~ ^\[(|[A-Za-z0-9._/-]+(,[[:space:]]*[A-Za-z0-9._/-]+)*)\]$ ]] || {
    fail "${files[$id]}: read_paths is not a canonical list"
    continue
  }
  mapfile -t owned_paths < <(parse_contract_list "$owned_spec")
  mapfile -t read_paths < <(parse_contract_list "$read_spec")
  acceptance_path="tests/acceptance/$id.sh"
  acceptance_owned=0
  for path in "${owned_paths[@]}"; do
    tracked_path_exists "$path" || fail "${files[$id]}: active candidate path is missing or untracked: $path"
    if path_is_within "$acceptance_path" "$path"; then
      acceptance_owned=1
    fi
    for read_path in "${read_paths[@]}"; do
      if path_is_within "$path" "$read_path" || path_is_within "$read_path" "$path"; then
        fail "${files[$id]}: read_path overlaps owned path: $read_path <> $path"
      fi
    done
  done
  (( acceptance_owned == 1 )) || fail "${files[$id]}: acceptance is outside owned paths"
  for read_path in "${read_paths[@]}"; do
    tracked_path_exists "$read_path" || fail "${files[$id]}: read_path is missing or untracked: $read_path"
  done
  acceptance="$repo_root/$acceptance_path"
  [[ -f "$acceptance" && ! -L "$acceptance" && -x "$acceptance" ]] ||
    fail "${files[$id]}: active candidate lacks executable acceptance: $acceptance_path"
  bash -n "$acceptance" || fail "${files[$id]}: acceptance syntax failed"
  grep -Eiq 'MUT|mutation|mutat' "${files[$id]}" ||
    fail "${files[$id]}: card has no declared mutation/skeptic signal"
  profile="$(sed -n 's/^container_profile: `\([^`]*\)`$/\1/p' "${files[$id]}")"
  accept="$(sed -n 's/^accept: `\(.*\)`$/`\1`/p' "${files[$id]}")"
  [[ "$profile" =~ ^[a-z0-9][a-z0-9.-]{2,63}$ ]] ||
    fail "${files[$id]}: active candidate lacks a valid container_profile"
  expected_accept="\`./.board/bin/oci-run --profile $profile --card $id\`"
  [[ "$accept" == "$expected_accept" ]] ||
    fail "${files[$id]}: accept is not bound to its declared profile"
  profile_file="$board_dir/oci/profiles/$profile.json"
  lock_file="$board_dir/locks/oci/$profile.lock.json"
  [[ -f "$profile_file" && ! -L "$profile_file" ]] || fail "${files[$id]}: profile is not registered: $profile"
  [[ -f "$lock_file" && ! -L "$lock_file" ]] || fail "${files[$id]}: profile lock is missing: $profile"
  profile_name="$(sed -n 's/^[[:space:]]*"profile"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$profile_file" 2>/dev/null || true)"
  profile_lock="$(sed -n 's/^[[:space:]]*"lock"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$profile_file" 2>/dev/null || true)"
  profile_lock_digest="$(sed -n 's/^[[:space:]]*"lock_digest"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$profile_file" 2>/dev/null || true)"
  lock_profile="$(sed -n 's/^[[:space:]]*"profile"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$lock_file" 2>/dev/null || true)"
  lock_schema="$(sed -n 's/^[[:space:]]*"schema"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$lock_file" 2>/dev/null || true)"
  lock_version="$(sed -n 's/^[[:space:]]*"version"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p' "$lock_file" 2>/dev/null || true)"
  [[ "$profile_name" == "$profile" && "$lock_profile" == "$profile" ]] || fail "${files[$id]}: profile and lock names do not bind"
  [[ "$profile_lock" == ".board/locks/oci/$profile.lock.json" ]] || fail "${files[$id]}: profile points at a foreign lock"
  [[ "$lock_schema" == 'aurum.oci-image-lock' && "$lock_version" == 1 ]] || fail "${files[$id]}: profile lock schema/version is invalid"
  actual_lock_digest="sha256:$(sha256sum -- "$lock_file" | awk '{print $1}')"
  [[ "$profile_lock_digest" == "$actual_lock_digest" ]] || fail "${files[$id]}: profile lock_digest does not match lock bytes"
  image="$(sed -n 's/^[[:space:]]*"image"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$lock_file" 2>/dev/null || true)"
  [[ "$image" =~ ^[a-z0-9][a-z0-9._/-]*@sha256:[0-9a-f]{64}$ ]] ||
    fail "${files[$id]}: profile lock does not contain a digest-pinned image"
done

report_failures

legacy_done_without_delivery=(AUR-015 AUR-016 AUR-017 AUR-020 AUR-021 AUR-359 AUR-360 AUR-361 AUR-362 AUR-363 AUR-364)
is_legacy_done_without_delivery() {
  local wanted="$1" legacy
  for legacy in "${legacy_done_without_delivery[@]}"; do
    [[ "$wanted" == "$legacy" ]] && return 0
  done
  return 1
}

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
        if is_legacy_done_without_delivery "$id"; then
          printf 'board note: allowlisted legacy done card without delivery record: %s\n' "$id" >&2
        else
          fail "${files[$id]}: done card lacks delivery record"
        fi
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
