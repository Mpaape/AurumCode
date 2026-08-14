#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C

# Deterministic 20-minute office monitor. It measures state and records the
# stagnation counter; it never dispatches agents, changes cards, or invokes the
# frozen legacy validator.

board_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd "$board_dir/.." && pwd -P)"
repo_token="$(printf '%s' "$repo_root" | sha256sum | awk '{print $1}')"
state_file="${AURUM_OFFICE_STATE_FILE:-/tmp/aurumcode-office-cycle-$repo_token.state}"
mode="${1:-status}"
branch="$(git -C "$repo_root" symbolic-ref --short -q HEAD || printf 'detached')"
head="$(git -C "$repo_root" rev-parse HEAD)"

usage() {
  printf 'usage: %s --start | --review | --status [state-file]\n' "${BASH_SOURCE[0]}" >&2
  exit 64
}

case "$mode" in
  --start|--review|--status) ;;
  *) usage ;;
esac
if [[ $# -ge 2 ]]; then
  state_file="$2"
fi

count_cards() {
  local state="$1"
  find "$board_dir/cards/$state" -maxdepth 1 -type f -name 'AUR-*.md' -print | wc -l
}

done_count="$(count_cards done)"
backlog_count="$(count_cards backlog)"
ready_count="$(count_cards ready)"
doing_count="$(count_cards doing)"
review_count="$(count_cards review)"
validating_count="$(count_cards validating)"
blocked_count="$(count_cards blocked-on-owner)"
now="$(date -u +%s)"
now_iso="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

if ! bash "$board_dir/pipeline.sh"; then
  printf 'office cycle: STOP board pipeline failed; do not dispatch\n' >&2
  exit 1
fi

printf 'office cycle: time=%s done=%s backlog=%s ready=%s doing=%s review=%s validating=%s blocked-on-owner=%s\n' \
  "$now_iso" "$done_count" "$backlog_count" "$ready_count" "$doing_count" \
  "$review_count" "$validating_count" "$blocked_count"
if [[ -n "$(git -C "$repo_root" status --porcelain)" ]]; then
  printf 'office cycle: coordinator worktree is dirty; builders and validators must use clean worktrees\n' >&2
fi

printf 'office cycle: ready queue\n'
shopt -s nullglob
ready_files=("$board_dir/cards/ready"/AUR-*.md)
if (( ${#ready_files[@]} == 0 )); then
  printf '%s\n' '(empty)'
else
  for file in "${ready_files[@]}"; do
    printf '%s\n' "${file##*/}"
  done | sort
fi
printf 'office cycle: active cards\n'
for state in doing review validating; do
  files=("$board_dir/cards/$state"/AUR-*.md)
  for file in "${files[@]}"; do
    printf '%s\t%s\n' "$state" "${file##*/}"
  done
done | sort

if [[ "$mode" == --status ]]; then
  exit 0
fi

mkdir -p "$(dirname "$state_file")"
if [[ "$mode" == --start ]]; then
  tmp="${state_file}.tmp.$$"
  {
    printf 'repo_root=%s\n' "$repo_root"
    printf 'branch=%s\n' "$branch"
    printf 'started_at=%s\n' "$now"
    printf 'last_at=%s\n' "$now"
    printf 'last_done=%s\n' "$done_count"
    printf 'stagnant_cycles=0\n'
    printf 'started_head=%s\n' "$head"
  } >"$tmp"
  mv -- "$tmp" "$state_file"
  printf 'office cycle: baseline recorded in %s\n' "$state_file"
  exit 0
fi

[[ -f "$state_file" && ! -L "$state_file" ]] || {
  printf 'office cycle: missing baseline; run --start first: %s\n' "$state_file" >&2
  exit 64
}
read_state() {
  awk -F= -v key="$1" '$1 == key { count++; value=substr($0, length(key) + 2) } END { if (count != 1) exit 1; print value }' "$state_file"
}
state_repo_root="$(read_state repo_root)"
state_branch="$(read_state branch)"
[[ "$state_repo_root" == "$repo_root" ]] || {
  printf 'office cycle: baseline belongs to another checkout: %s\n' "$state_repo_root" >&2
  exit 64
}
[[ "$state_branch" == "$branch" ]] || {
  printf 'office cycle: baseline belongs to another branch: %s\n' "$state_branch" >&2
  exit 64
}
started_at="$(read_state started_at)"
last_at="$(read_state last_at)"
last_done="$(read_state last_done)"
stagnant_cycles="$(read_state stagnant_cycles)"
started_head="$(read_state started_head)"
[[ "$started_at" =~ ^[0-9]+$ && "$last_at" =~ ^[0-9]+$ && "$last_done" =~ ^[0-9]+$ && "$stagnant_cycles" =~ ^[0-9]+$ && "$started_head" =~ ^[0-9a-f]{40}$ ]] || {
  printf 'office cycle: malformed baseline; remove it and run --start again\n' >&2
  exit 64
}
if [[ "$branch" == detached && "$started_head" != "$head" ]]; then
  printf 'office cycle: detached checkout changed commit; start a new baseline\n' >&2
  exit 64
fi
since_last=$((now - last_at))
(( since_last >= 1200 )) || {
  printf 'office cycle: review is only due every 1200 seconds (last was %s seconds ago)\n' "$since_last" >&2
  exit 64
}

delta=$((done_count - last_done))
if (( delta > 0 )); then
  stagnant_cycles=0
else
  stagnant_cycles=$((stagnant_cycles + 1))
fi
elapsed=$((now - started_at))
tmp="${state_file}.tmp.$$"
{
  printf 'repo_root=%s\n' "$repo_root"
  printf 'branch=%s\n' "$branch"
  printf 'started_at=%s\n' "$started_at"
  printf 'last_at=%s\n' "$now"
  printf 'last_done=%s\n' "$done_count"
  printf 'stagnant_cycles=%s\n' "$stagnant_cycles"
  printf 'started_head=%s\n' "$started_head"
} >"$tmp"
mv -- "$tmp" "$state_file"

printf 'office cycle: done_delta=%s elapsed_seconds=%s stagnant_cycles=%s\n' \
  "$delta" "$elapsed" "$stagnant_cycles"
if (( stagnant_cycles >= 2 )); then
  printf 'office cycle: STOP two reviews without done progress; change the approach before dispatching again\n' >&2
  exit 75
fi
