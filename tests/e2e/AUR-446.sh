#!/usr/bin/env bash
# AUR-446 E2E: run the exact command AC-001 describes -- "grep de
# verificacao estatica sobre os specs entregues" -- against the REAL
# corrected spec files on disk, end to end, inside the offline sandbox.
#
# WHAT IT DOES
#   For each of the six specs this card corrects, greps the real file for
#   (a) the corrected, present-tense anchor that must be there and (b) the
#   stale, pre-correction anchor that must NOT be there. Every anchor is
#   checked after collapsing whitespace (tabs/newlines/spaces all become one
#   space), so a check never depends on where Markdown happens to wrap a
#   paragraph -- verified against this repository's actual pre-fix line
#   wrapping before this script was written (see docs/specs/AUR-446.md).
#   docs/specs/AUR-437.md gets only a positive check: the original defect-4
#   description was incomplete, not false, so there is no stale claim to
#   forbid.
#
# MUT-001 FALSIFIER
#   Reintroducing the stale "No `aurumcode docs` subcommand exists:" claim
#   into docs/specs/AUR-425.md or docs/specs/AUR-429.md (verbatim, including
#   its original line wrap) makes this script fail non-zero, printing
#   AUR-446/AC-001/MUT-001. Restoring the correction reproduces the exact
#   GREEN, byte for byte.
set -euo pipefail
export LC_ALL=C
[[ "${1:-E2EAUR446}" == E2EAUR446 ]] || { printf 'AUR-446/AC-001/unknown-selector\n' >&2; exit 64; }

fail() { printf 'AUR-446/AC-001/%s\n' "$1" >&2; exit 1; }
mut() { printf 'AUR-446/AC-001/MUT-001/%s\n' "$1" >&2; exit 1; }
infra() { printf 'AUR-446/AC-001/infrastructure/%s\n' "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v grep >/dev/null 2>&1 || infra missing_grep
command -v tr   >/dev/null 2>&1 || infra missing_tr
command -v sha256sum >/dev/null 2>&1 || infra missing_sha256sum

# The six specs this card corrects (its own declared deliverables, per
# `paths`): absence is behavior-missing, never infrastructure.
specs=(
  docs/specs/AUR-424.md
  docs/specs/AUR-425.md
  docs/specs/AUR-428.md
  docs/specs/AUR-429.md
  docs/specs/AUR-437.md
  docs/specs/AUR-440.md
)
for s in "${specs[@]}"; do
  [[ -f "$repo_root/$s" ]] || fail "behavior-missing:entrypoint_missing:$s"
done

# normalized <file> prints the file with every run of whitespace collapsed
# to one space, so a `grep -F` against it is immune to paragraph wrapping.
normalized() { tr -s '[:space:]' ' ' <"$1"; }

# check_present/check_absent print a transcript line per anchor instead of
# failing immediately, so run_checks reports every anchor (not just the
# first) and produces a deterministic transcript the repeat clause below can
# hash and compare. Violations are recovered by parsing that transcript
# (below), not by mutating a shell array from inside these functions: both
# calls to run_checks run through `$(...)` command substitution, which forks
# a subshell -- any array append inside would vanish when the subshell
# exits, silently discarding every violation. That exact bug was caught by
# hand-testing the MUT-001 restoration before this script was accepted.
check_present() {
  local file="$1" anchor="$2"
  if normalized "$repo_root/$file" | grep -Fq -- "$anchor"; then
    printf 'PRESENT-OK %s :: %s\n' "$file" "$anchor"
  else
    printf 'PRESENT-MISSING %s :: %s\n' "$file" "$anchor"
  fi
}

check_absent() {
  local file="$1" anchor="$2"
  if normalized "$repo_root/$file" | grep -Fq -- "$anchor"; then
    printf 'ABSENT-STALE %s :: %s\n' "$file" "$anchor"
  else
    printf 'ABSENT-OK %s :: %s\n' "$file" "$anchor"
  fi
}

run_checks() {
  check_absent  docs/specs/AUR-424.md 'Dentro do sandbox ela sai 69'
  check_present docs/specs/AUR-424.md 'O accept selado sai **0**'

  check_absent  docs/specs/AUR-425.md 'No `aurumcode docs` subcommand exists:'
  check_present docs/specs/AUR-425.md 'delivered by AUR-426 (`cmd/aurumcode/docs.go`'

  check_absent  docs/specs/AUR-428.md 'Os demais exemplos em `.github/workflows/examples/` (code-review, qa-testing, all-pipelines) continuam pinados por SHA'
  check_present docs/specs/AUR-428.md 'code-review.yml` **não** está mais nesse grupo: o AUR-440 o repinou para a tag `v1`'

  check_absent  docs/specs/AUR-429.md 'No `aurumcode docs` subcommand exists:'
  check_present docs/specs/AUR-429.md 'docs` subcommand from AUR-426 (`cmd/aurumcode/docs.go`)'

  check_present docs/specs/AUR-437.md 'internal/git/githubclient/client.go:795-802'

  check_absent  docs/specs/AUR-440.md 'restaurado pelo AUR-437 e ainda nao foi executado'
  check_present docs/specs/AUR-440.md 'AUR-438 esta done: `cmd/aurumcode` ja publica'
}

# report_violations reads the PRESENT-MISSING/ABSENT-STALE lines back out of
# a transcript (see the note above check_present/check_absent for why this
# is a parse of stdout rather than a shared array) and fails on the first
# one, after printing every violation found.
report_violations() {
  local transcript="$1" line file anchor first=""
  while IFS= read -r line; do
    case "$line" in
      'PRESENT-MISSING '*)
        file="${line#PRESENT-MISSING }"; file="${file%% :: *}"
        anchor="${line#*:: }"
        printf 'AUR-446/AC-001/behavior-missing:%s:missing:%s\n' "$file" "$anchor" >&2
        [[ -n "$first" ]] || first="fail:behavior-missing:$file:missing:$anchor"
        ;;
      'ABSENT-STALE '*)
        file="${line#ABSENT-STALE }"; file="${file%% :: *}"
        anchor="${line#*:: }"
        printf 'AUR-446/AC-001/MUT-001/%s:stale-claim-present:%s\n' "$file" "$anchor" >&2
        [[ -n "$first" ]] || first="mut:$file:stale-claim-present:$anchor"
        ;;
    esac
  done <<<"$transcript"
  [[ -z "$first" ]] || printf '%s' "$first"
}

transcript_1="$(run_checks)"
digest_1="$(printf '%s' "$transcript_1" | sha256sum | cut -d' ' -f1)"

first_violation="$(report_violations "$transcript_1")"
if [[ -n "$first_violation" ]]; then
  case "$first_violation" in
    mut:*)  mut "${first_violation#mut:}" ;;
    fail:*) fail "${first_violation#fail:}" ;;
  esac
fi

# AC-001's repeat clause, actually proven rather than asserted: a second,
# independent pass over the same real files must produce a byte-identical
# transcript. A mismatch here would mean the checks are not deterministic
# (e.g. an anchor whose match depends on something other than file content)
# and is reported distinctly from a plain behavioural miss.
transcript_2="$(run_checks)"
digest_2="$(printf '%s' "$transcript_2" | sha256sum | cut -d' ' -f1)"
[[ "$digest_1" == "$digest_2" ]] || fail "nondeterministic-output:$digest_1:$digest_2"

printf '{"card":"AUR-446","scenario":"E2EAUR446","result":"pass","specs_checked":%d,"anchors_verified":11,"repeat_digest":"%s"}\n' \
  "${#specs[@]}" "$digest_1"
