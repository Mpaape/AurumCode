#!/usr/bin/env bash
# AUR-601 acceptance: the done acceptances the nightly sample found red on
# main fffeef9e pass again, and the cache determinism fix is load-bearing.
#
# Selectors:
#   all      AC-003 then MUT-001
#   AC-003   AUR-441 (AC-001), 491, 541, 560, 574, 576, 578 exit 0
#   MUT-001  redacting the cached message together with its rule citation
#            (the cause of AUR-441 non-deterministic) turns AUR-441 red
# Exit: 0 pass, 1 behavioral failure, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C

readonly card='AUR-601'
selector="${1:-all}"
case "$selector" in
  all|AC-003|MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

run_acc() {
  local name="$1" sel="$2" log="$3"
  bash "$repo_root/tests/acceptance/$name.sh" "$sel" >"$log" 2>&1
}

ac003() {
  local log name sel pairs
  log="$(mktemp)"
  # AUR-541, 574, 576 and 578 rerun e2e programs and tutorials that need git
  # and the tutorial image; the sealed profile has neither. There they are
  # proved by the nightly sample (acceptance-sample.yml), not here.
  pairs='AUR-441:AC-001 AUR-491:all AUR-560:all'
  if command -v git >/dev/null 2>&1 && command -v docker >/dev/null 2>&1; then
    pairs="$pairs AUR-541:all AUR-574:all AUR-576:all AUR-578:all"
  else
    printf '%s/AC-003/sealed-subset: 441, 491, 560 (541, 574, 576, 578 pela amostra noturna)\n' "$card" >&2
  fi
  for pair in $pairs; do
    name="${pair%%:*}"; sel="${pair#*:}"
    if ! run_acc "$name" "$sel" "$log"; then
      tail -n 20 "$log" >&2
      rm -f -- "$log"
      fail "still-red:$name"
    fi
  done
  rm -f -- "$log"
  printf '%s/AC-003/pass\n' "$card"
}

mut001() {
  local root file log
  root="$(mktemp -d)"
  trap 'chmod -R u+w -- "$root" 2>/dev/null || true; rm -rf -- "$root"' RETURN
  cp -R "$repo_root/." "$root/"
  chmod -R u+w -- "$root"
  file="$root/cmd/aurumcode/cache_redaction.go"
  grep -Fq 'issue.Message = redactCachedMessage(f, issue.Message, issue.RuleID)' "$file" || infra mutation-anchor-missing
  sed -i 's/issue.Message = redactCachedMessage(f, issue.Message, issue.RuleID)/issue.Message = redactProse(f, issue.Message)/' "$file"
  log="$root/mut.log"
  if bash "$root/tests/acceptance/AUR-441.sh" AC-001 >"$log" 2>&1; then
    fail "mutation-survived"
  fi
  grep -Fq 'AUR-441/' "$log" || { tail -n 20 "$log" >&2; infra mutation-did-not-run; }
  printf '%s/MUT-001/rejected (%s)\n' "$card" "$(grep -o 'AUR-441/[A-Za-z0-9/_:-]*' "$log" | tail -n 1)"
}

case "$selector" in
  AC-003) ac003 ;;
  MUT-001) mut001 ;;
  all) ac003; mut001; printf '%s/all/pass\n' "$card" ;;
esac
