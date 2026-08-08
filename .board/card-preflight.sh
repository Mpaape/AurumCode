#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C

card_id="${1:-}"
worktree="${2:-$(pwd -P)}"
[[ "$card_id" =~ ^AUR-[0-9]{3}$ ]] || { printf 'preflight error: use AUR-NNN\n' >&2; exit 1; }
[[ -d "$worktree/.git" || -f "$worktree/.git" ]] || { printf 'preflight error: not a Git worktree\n' >&2; exit 1; }

card=''
for state in backlog ready doing review validating done blocked-on-owner cancelled; do
  candidate="$worktree/.board/cards/$state/$card_id.md"
  if [[ -f "$candidate" ]]; then
    [[ -z "$card" ]] || { printf 'preflight error: duplicate card state\n' >&2; exit 1; }
    card="$candidate"
  fi
done
[[ -n "$card" ]] || { printf 'preflight error: card not found: %s\n' "$card_id" >&2; exit 1; }

status=''
validation='none'
while IFS= read -r line || [[ -n "$line" ]]; do
  case "$line" in
    status:\ *) status="${line#status: }" ;;
    validation:\ *) validation="${line#validation: }" ;;
  esac
done < <(awk '/^---$/{n++; next} n==1 {print}' "$card")

[[ "$status" =~ ^(ready|doing|review|validating|done)$ ]] || {
  printf 'preflight error: unsupported active status: %s\n' "$status" >&2
  exit 1
}
case "$validation" in
  none) ;;
  tested|skeptical) ;;
  *) printf 'preflight error: invalid validation kind: %s\n' "$validation" >&2; exit 1 ;;
esac

acceptance="$worktree/tests/acceptance/$card_id.sh"
if [[ "$validation" != none ]]; then
  [[ -f "$acceptance" && -x "$acceptance" ]] || {
    printf 'preflight error: tested card lacks executable acceptance: %s\n' "$acceptance" >&2
    exit 1
  }
  bash -n "$acceptance" || { printf 'preflight error: acceptance syntax failed\n' >&2; exit 1; }
  if grep -Eq '(^|[^[:alnum:]_])go([[:space:]]|$)|go[[:space:]]+test' "$acceptance"; then
    if ! command -v go >/dev/null 2>&1; then
      if grep -Eiq 'oci-run|container_profile' "$acceptance"; then
        printf 'preflight warning: host Go unavailable; canonical OCI acceptance is required\n' >&2
      else
        printf 'preflight infrastructure: Go toolchain unavailable for %s\n' "$card_id" >&2
        exit 69
      fi
    fi
  fi
  grep -Eiq 'MUT|mutation|mutat' "$acceptance" || {
    printf 'preflight error: validation has no mutation/skeptic signal\n' >&2
    exit 1
  }
  if [[ "${PREFLIGHT_RUN:-0}" == 1 ]]; then
    set +e
    bash "$acceptance" AC-001
    acceptance_rc=$?
    set -e
    case "$acceptance_rc" in
      0|1) ;;
      3|69|79) printf 'preflight infrastructure: acceptance unavailable (exit %s)\n' "$acceptance_rc" >&2; exit 69 ;;
      *) printf 'preflight error: acceptance setup failed (exit %s)\n' "$acceptance_rc" >&2; exit 1 ;;
    esac
  fi
fi

if [[ -n "$(git -C "$worktree" status --porcelain)" ]]; then
  printf 'preflight error: worktree is dirty; validate an immutable candidate\n' >&2
  exit 1
fi

printf 'preflight ok: %s validation=%s worktree=%s\n' "$card_id" "$validation" "$worktree"
