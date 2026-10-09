#!/usr/bin/env bash
# AUR-610 acceptance: with changelog_check.mode required, a bot's pull
# request (user.type Bot or a login ending in [bot], read from the event)
# gets changelog_check.bots (default suggest): it passes with a line saying
# why and still gets the suggested entry. bots only lowers the mode; a
# person's run is unchanged; an absent author is a person (fail-closed).
#
# Selectors:
#   all      AC-001..AC-005, MUT-001..MUT-002
#   AC-001   a bot's PR under required passes with the explanation, in the
#            check, the review body, the script and the workflow
#   AC-002   bots off skips the check; bots required refuses like a person
#   AC-003   a person's output is byte-identical with or without author flags
#   AC-004   no author flags: a person
#   AC-005   bot-lover is a person; renovate[bot] typed User is a bot
#   MUT-001  "contains bot" instead of the [bot] suffix turns AC-005 red
#   MUT-002  a bots policy that can raise the mode turns the policy test red
# Exit: 0 pass, 1 behavioral failure, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C

readonly card='AUR-610'
selector="${1:-all}"
known='all AC-001 AC-002 AC-003 AC-004 AC-005 MUT-001 MUT-002'
if [[ " $known " != *" $selector "* ]]; then
  printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2
  exit 64
fi

fail() {
  printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2
  exit 1
}
infra() {
  printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2
  exit 79
}

script_dir="${0%/*}"
[[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
: "${GOCACHE:=$(mktemp -d)}"
export GOCACHE

work="$(mktemp -d)"
trap 'chmod -R u+w -- "$work" 2>/dev/null || true; rm -rf -- "$work"' EXIT

readonly pkgs=(./internal/changelog ./internal/config ./internal/git/githubclient ./cmd/aurumcode)
# The edge files the workflow and script tests read, outside the module.
readonly edges=(.github/workflows/changelog.yml scripts/ci/changelog-check.sh)

# stage copies the module (whole cmd, internal, pkg) and the edge files into
# a fresh root.
stage() {
  local root="$1" edge
  mkdir -p "$root"
  for item in go.mod go.sum cmd internal pkg; do
    [[ -e "$repo_root/$item" ]] || infra "missing-$item"
    cp -R "$repo_root/$item" "$root/"
  done
  for edge in "${edges[@]}"; do
    [[ -f "$repo_root/$edge" ]] || infra "missing-$edge"
    mkdir -p "$root/${edge%/*}"
    cp "$repo_root/$edge" "$root/$edge"
  done
  chmod -R u+w -- "$root"
}

# run_test runs the named tests of the four packages in root.
run_test() {
  local root="$1" pattern="$2" log="$3"
  (cd "$root" && AURUMCODE_MODULE_ONLY='' go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "${pkgs[@]}") >"$log" 2>&1
}

base="$work/base"
stage "$base"

# ac id pattern tests...: every named test must pass.
ac() {
  local id="$1" pattern="$2" log="$work/$1.log" name
  shift 2
  if ! run_test "$base" "$pattern" "$log"; then
    tail -n 30 "$log" >&2
    fail "test-failed"
  fi
  for name in "$@"; do
    grep -Eq -- "^--- PASS: $name " "$log" || { tail -n 30 "$log" >&2; fail "$id-missing-pass:$name"; }
  done
  printf '%s/%s/pass\n' "$card" "$id"
}

mutate() {
  local id="$1" file="$2" anchor="$3" expr="$4" pattern="$5" assertion="$6"
  local root="$work/$id" log="$work/$id.log"
  stage "$root"
  grep -Fq -- "$anchor" "$root/$file" || infra "$id-anchor-missing"
  sed -i "$expr" "$root/$file"
  if grep -Fq -- "$anchor" "$root/$file"; then
    infra "$id-not-applied"
  fi
  if run_test "$root" "$pattern" "$log"; then
    fail "$id-survived"
  fi
  if grep -Eq 'build failed|setup failed' "$log"; then
    tail -n 20 "$log" >&2
    infra "$id-did-not-compile"
  fi
  grep -Eq -- "^--- FAIL: $assertion" "$log" || {
    tail -n 20 "$log" >&2
    infra "$id-unexpected-failure"
  }
  rm -rf -- "$root"
  printf '%s/%s/rejected (%s)\n' "$card" "$id" "$assertion"
}

# need <file> <literal> fails unless the file carries the literal.
need() {
  [[ -f "$repo_root/$1" ]] || infra "missing-$1"
  grep -Fq -- "$2" "$repo_root/$1" || fail "$1/lacks:$2"
}

ac001() {
  need docs/configuration.md '`changelog_check.bots`'
  need docs/configuration.md '--tipo-autor'
  need docs/changelog.md 'changelog_check.bots'
  ac AC-001 '^TestAUR610(AC001BotPassesWithTheReason|ReviewBodyUsesTheAuthorsMode|WorkflowHandsTheAuthorToTheCheck|ScriptPassesTheAuthorFlags|MetadataCarriesTheAuthor)$' \
    TestAUR610AC001BotPassesWithTheReason TestAUR610ReviewBodyUsesTheAuthorsMode TestAUR610WorkflowHandsTheAuthorToTheCheck \
    TestAUR610ScriptPassesTheAuthorFlags TestAUR610MetadataCarriesTheAuthor
}
ac002() {
  ac AC-002 '^TestAUR610(AC002BotsOffSkipsAndRequiredRefuses|BotsNeverRaiseTheMode|UnknownBotsRefused)$' \
    TestAUR610AC002BotsOffSkipsAndRequiredRefuses TestAUR610BotsNeverRaiseTheMode TestAUR610UnknownBotsRefused
}
ac003() {
  ac AC-003 '^TestAUR(610AC003HumanOutputUnchanged|509AC003CheckFailsClosed|604AC002RequiredModeUnchanged)$' \
    TestAUR610AC003HumanOutputUnchanged TestAUR509AC003CheckFailsClosed TestAUR604AC002RequiredModeUnchanged
}
ac004() {
  ac AC-004 '^TestAUR610(AC004NoAuthorFlagsIsAPerson|ScriptPassesTheAuthorFlags)$' \
    TestAUR610AC004NoAuthorFlagsIsAPerson TestAUR610ScriptPassesTheAuthorFlags
}
ac005() {
  ac AC-005 '^TestAUR610AC005AuthorIsBot$' TestAUR610AC005AuthorIsBot
}

mut001() {
  mutate MUT-001 internal/changelog/author.go 'strings.HasSuffix(strings.TrimSpace(a.Login), botLoginSuffix)' \
    's/strings\.HasSuffix(strings\.TrimSpace(a\.Login), botLoginSuffix)/strings.Contains(strings.TrimSpace(a.Login), "bot")/' \
    '^TestAUR610AC005AuthorIsBot$' 'TestAUR610AC005AuthorIsBot'
}
mut002() {
  mutate MUT-002 internal/config/changelog_check.go 'if changelogModeRank[bots] < changelogModeRank[mode] {' \
    's/if changelogModeRank\[bots\] < changelogModeRank\[mode\] {/if true {/' \
    '^TestAUR610BotsNeverRaiseTheMode$' 'TestAUR610BotsNeverRaiseTheMode'
}

# One function per selector; all runs every one in order.
if [[ "$selector" == 'all' ]]; then
  for step in ac001 ac002 ac003 ac004 ac005 mut001 mut002; do
    "$step"
  done
  printf '%s/all/pass\n' "$card"
else
  step="${selector//-/}"
  step="${step,,}"
  "$step"
fi
