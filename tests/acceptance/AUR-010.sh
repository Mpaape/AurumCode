#!/usr/bin/env bash
# AUR-010 / AC-001 -- "Bloquear atribuicao de IA em commits".
#
# This program does not inspect instruction files. It builds a disposable bare
# repository and exercises the artifact the card promises,
# `internal/governance/gitpolicy`, through the card-scoped entrypoint
# `tests/gitpolicy/check-commit-range`, which must dispatch into
# `gitpolicy.CheckCommitRange(git_dir, base, head)`.
#
# Entrypoint contract observed here (the card owns both sides of it):
#
#   tests/gitpolicy/check-commit-range <git_dir> <base_rev> <head_rev>
#
#   exit 0  no forbidden attribution in the exclusive range base..head;
#           stdout carries one JSON object with "result":"pass".
#   exit 3  at least one violation; stdout carries the ordered findings, each
#           with "commit_oid" and rule_id `commit_attribution_forbidden`, and
#           only the redacted offending field -- never the message body.
#   exit 4  bounded input error; stdout carries one distinct rule_id out of
#           `range_ref_missing`, `range_not_ancestor`, `range_too_large`.
#   exit 69 the entrypoint's own toolchain/environment failed. That is an
#           infrastructure diagnosis, never a verdict, and this program
#           re-raises it as such.
#
# Exit codes of this program, deliberately disjoint:
#   0   the promised behavior was observed;
#   1   behavioral RED, message `AUR-010/AC-001/<reason>`;
#   64  unknown selector;
#   69  infrastructure, message `AUR-010/AC-001/infrastructure/<reason>`.
#       A missing utility, a Git failure, a fixture failure or an entrypoint
#       toolchain failure exits 69 and is never valid red evidence.
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-010'
readonly scenario='AC-001'

selector="${1:-AC-001}"
case "$selector" in
  AC-001|TestAUR010|IntegrationAUR010) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() {
  printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2
  exit 1
}

infra() {
  printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2
  exit 69
}

work=''
cleanup() {
  [[ -z "$work" ]] || rm -rf -- "$work"
}
trap cleanup EXIT INT TERM HUP

for tool in git mktemp rm grep wc head; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing-utility:$tool"
done

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd -P)"

# ---------------------------------------------------------------------------
# The promised artifact. While `internal/governance/gitpolicy` does not exist,
# or does not export the range checker, this scenario is unreachable and the
# card is RED for exactly that reason.
# ---------------------------------------------------------------------------
package_dir="$repo_root/internal/governance/gitpolicy"
[[ -d "$package_dir" && ! -L "$package_dir" ]] || fail range-policy-missing

shopt -s nullglob
declared=0
for source_file in "$package_dir"/*.go; do
  [[ -f "$source_file" && ! -L "$source_file" ]] || continue
  [[ "$source_file" != *_test.go ]] || continue
  grep -Eq '^package[[:space:]]+gitpolicy[[:space:]]*$' "$source_file" || continue
  grep -Eq '^func[[:space:]]+CheckCommitRange\(' "$source_file" || continue
  declared=1
  break
done
shopt -u nullglob
(( declared == 1 )) || fail range-policy-missing

runner="$repo_root/tests/gitpolicy/check-commit-range"
[[ -f "$runner" && ! -L "$runner" ]] || fail range-policy-missing
[[ -x "$runner" ]] || fail range-policy-not-executable
# The entrypoint must dispatch into the promised package, otherwise this
# program would be measuring an unrelated checker.
grep -Fq 'internal/governance/gitpolicy' "$runner" || fail range-policy-unbound

# ---------------------------------------------------------------------------
# Disposable fixture. Everything below builds Git objects only: no checkout, no
# hook, no network, no state outside the temporary directory.
# ---------------------------------------------------------------------------
work="$(mktemp -d "${TMPDIR:-/tmp}/aurum-aur010.XXXXXX")" || infra mktemp-failed
[[ -d "$work" && ! -L "$work" ]] || infra unsafe-staging-directory

export HOME="$work/home"
export XDG_CONFIG_HOME="$work/home/.config"
export GIT_CONFIG_NOSYSTEM=1
export GIT_TERMINAL_PROMPT=0
mkdir -p "$HOME" "$XDG_CONFIG_HOME" || infra staging-mkdir-failed

canary="aurum-secret-canary-$$-${RANDOM}${RANDOM}"
readonly canary
readonly human='Jane Doe <jane.doe@example.test>'
readonly robot='dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.test>'
readonly stamp='1700000000 +0000'

git_dir="$work/repo.git"
git init --bare -q "$git_dir" >/dev/null 2>&1 || infra git-init-failed
empty_tree="$(git -C "$git_dir" hash-object -w -t tree /dev/null)" || infra git-empty-tree-failed

new_commit() {
  # $1 parent oid (empty for a root commit), $2 identity, $3.. message paragraphs
  local parent="$1" identity="$2"
  shift 2
  local name="${identity%% <*}" mail="${identity#*<}"
  mail="${mail%>}"
  local -a args=("$empty_tree")
  [[ -z "$parent" ]] || args+=(-p "$parent")
  local paragraph
  for paragraph in "$@"; do
    args+=(-m "$paragraph")
  done
  GIT_AUTHOR_NAME="$name" GIT_AUTHOR_EMAIL="$mail" GIT_AUTHOR_DATE="$stamp" \
  GIT_COMMITTER_NAME="$name" GIT_COMMITTER_EMAIL="$mail" GIT_COMMITTER_DATE="$stamp" \
    git -C "$git_dir" commit-tree "${args[@]}"
}

set_ref() {
  git -C "$git_dir" update-ref "$1" "$2" || infra "fixture-ref-failed:$1"
}

# Outside the approved range: a historical commit that does claim AI generation.
# The checker must ignore it, otherwise the permitted range becomes a false
# positive (the card's MUT-002).
historic_oid="$(new_commit '' "$human" 'chore: import legacy tree' 'Generated with ChatGPT')" ||
  infra fixture-commit-failed:historic
base_oid="$(new_commit "$historic_oid" "$human" 'chore: freeze reconstruction baseline')" ||
  infra fixture-commit-failed:base
set_ref refs/heads/base "$base_oid"

# Permitted range: human authorship, a human sign-off, a legitimate automation
# account with no AI claim, and prose that merely names the policy.
clean_oid="$(new_commit "$base_oid" "$human" 'feat: add commit range reader' \
  'Signed-off-by: Jane Doe <jane.doe@example.test>')" || infra fixture-commit-failed:clean-1
clean_oid="$(new_commit "$clean_oid" "$robot" 'build(deps): bump pinned base image digest')" ||
  infra fixture-commit-failed:clean-2
clean_oid="$(new_commit "$clean_oid" "$human" 'docs: explain the no-AI-attribution rule' \
  'Documents which trailers the policy refuses.')" || infra fixture-commit-failed:clean-3
set_ref refs/heads/clean "$clean_oid"

declare -a neg_labels=() neg_refs=() neg_oids=()
add_negative() {
  # $1 label, $2 subject, $3 forbidden line
  local label="$1" oid
  oid="$(new_commit "$base_oid" "$human" "$2" "$3" "Refs: $canary")" ||
    infra "fixture-commit-failed:$label"
  set_ref "refs/heads/neg-$label" "$oid"
  neg_labels+=("$label")
  neg_refs+=("refs/heads/neg-$label")
  neg_oids+=("$oid")
}

add_negative co-authored-by 'feat: add range walker' 'Co-Authored-By: Claude <noreply@anthropic.test>'
add_negative authored-by 'feat: add trailer parser' 'Authored-By: GPT-4 <assistant@example.invalid>'
add_negative generated-by 'feat: add finding serializer' 'Generated-By: Claude Code'
add_negative ai-generated 'feat: add redaction helper' 'AI-Generated: true'
add_negative generated-with 'feat: add bound checks' 'Generated with Claude Code'
add_negative signed-off-model 'feat: add ancestry guard' 'Signed-off-by: Claude Opus <noreply@anthropic.test>'

# Two violations in one range prove the findings are ordered oldest first.
multi_first_oid="$(new_commit "$base_oid" "$human" 'feat: first offending change' \
  'Co-Authored-By: Claude <noreply@anthropic.test>' "Refs: $canary")" ||
  infra fixture-commit-failed:multi-1
multi_second_oid="$(new_commit "$multi_first_oid" "$human" 'feat: second offending change' \
  'Generated-By: Claude Code' "Refs: $canary")" || infra fixture-commit-failed:multi-2
set_ref refs/heads/neg-multi "$multi_second_oid"

# Unrelated root, so base is not an ancestor of head.
orphan_oid="$(new_commit '' "$human" 'chore: unrelated root')" || infra fixture-commit-failed:orphan
set_ref refs/heads/orphan "$orphan_oid"

bulk_oid="$base_oid"
for index in $(seq 1 130); do
  bulk_oid="$(new_commit "$bulk_oid" "$human" "chore: bulk change $index")" ||
    infra "fixture-commit-failed:bulk-$index"
done
set_ref refs/heads/bulk "$bulk_oid"

# ---------------------------------------------------------------------------
# Exercise.
# ---------------------------------------------------------------------------
out="$work/stdout"
err="$work/stderr"
rc=0

invoke() {
  # $1 label, $2 base rev, $3 head rev
  : >"$out"
  : >"$err"
  if AURUM_SECRET_CANARY="$canary" "$runner" "$git_dir" "$2" "$3" >"$out" 2>"$err"; then
    rc=0
  else
    rc=$?
  fi
  (( rc != 69 )) || infra "range-policy-runner-failed:$1"
  local size
  size="$(wc -c <"$out")" || infra "capture-failed:$1"
  (( size <= 65536 )) || fail "unbounded-output:$1"
  size="$(wc -c <"$err")" || infra "capture-failed:$1"
  (( size <= 65536 )) || fail "unbounded-output:$1"
  if grep -Fq "$canary" "$out" || grep -Fq "$canary" "$err"; then
    fail "secret-leaked:$1"
  fi
  if grep -Fq "$historic_oid" "$out"; then
    fail "range-not-exclusive:$1"
  fi
}

# Permitted range.
invoke clean refs/heads/base refs/heads/clean
(( rc == 0 )) || fail human-commit-rejected
grep -Eq '"result"[[:space:]]*:[[:space:]]*"pass"' "$out" || fail human-commit-rejected

# One range per forbidden phrase.
for index in "${!neg_labels[@]}"; do
  label="${neg_labels[$index]}"
  invoke "$label" refs/heads/base "${neg_refs[$index]}"
  (( rc == 3 )) || fail "attribution-not-rejected:$label"
  grep -Fq 'commit_attribution_forbidden' "$out" || fail "wrong-rule:$label"
  grep -Fq 'commit_oid' "$out" || fail "finding-without-oid:$label"
  grep -Fq "${neg_oids[$index]}" "$out" || fail "finding-without-oid:$label"
done

# Ordered findings.
invoke multi refs/heads/base refs/heads/neg-multi
(( rc == 3 )) || fail attribution-not-rejected:multi
grep -Fq "$multi_first_oid" "$out" || fail finding-without-oid:multi
grep -Fq "$multi_second_oid" "$out" || fail finding-without-oid:multi
first_reported="$(grep -oE "$multi_first_oid|$multi_second_oid" "$out" | head -1)" ||
  infra capture-failed:multi
[[ "$first_reported" == "$multi_first_oid" ]] || fail findings-unordered

# Distinct bounded input errors.
expect_error() {
  # $1 label, $2 base, $3 head, $4 expected rule id, $5.. rule ids that must not appear
  local label="$1" base="$2" head="$3" expected="$4"
  shift 4
  invoke "$label" "$base" "$head"
  (( rc == 4 )) || fail "input-error-not-reported:$label"
  grep -Fq "$expected" "$out" || fail "input-error-not-reported:$label"
  grep -Fq 'commit_attribution_forbidden' "$out" && fail "input-error-not-distinct:$label"
  local other
  for other in "$@"; do
    if grep -Fq "$other" "$out"; then
      fail "input-error-not-distinct:$label"
    fi
  done
  return 0
}

expect_error missing-ref refs/heads/base refs/heads/absent \
  range_ref_missing range_not_ancestor range_too_large
expect_error non-ancestor refs/heads/orphan refs/heads/clean \
  range_not_ancestor range_ref_missing range_too_large
expect_error too-large refs/heads/base refs/heads/bulk \
  range_too_large range_ref_missing range_not_ancestor

printf '{"card":"%s","scenario":"%s","selector":"%s","permitted_ranges":1,"rejected_ranges":%d,"bounded_errors":3,"result":"pass"}\n' \
  "$card" "$scenario" "$selector" "$(( ${#neg_labels[@]} + 1 ))"
