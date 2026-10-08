#!/usr/bin/env bash
# AUR-604 acceptance: changelog_check.mode has three modes. off does nothing,
# suggest writes the suggested entry (log, job summary, review body) and never
# fails, required fails and still suggests; the docs explain the modes and
# the audit record and SARIF to a human reader.
#
# Selectors:
#   all      AC-001..AC-005, MUT-001, MUT-002
#   AC-001   suggest without an entry: suggestion everywhere, exit 0
#   AC-002   required without an entry: exit 1 with the suggestion
#   AC-003   off or no section: nothing, exit 0
#   AC-004   unknown mode refused with the three modes listed
#   AC-005   the docs describe the three modes, the audit record and SARIF,
#            with examples and a diagram
#   MUT-001  suggest failing the check turns AC-001 red
#   MUT-002  suggest leaving the suggestion out of the review body turns AC-001 red
# Exit: 0 pass, 1 behavioral failure, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C

readonly card='AUR-604'
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

# stage copies the module (whole cmd, internal, pkg) into a fresh root.
stage() {
  local root="$1"
  mkdir -p "$root"
  for item in go.mod go.sum cmd internal pkg; do
    [[ -e "$repo_root/$item" ]] || infra "missing-$item"
    cp -R "$repo_root/$item" "$root/"
  done
  chmod -R u+w -- "$root"
}

# run_test runs one Go test in root; the log keeps the output.
run_test() {
  local root="$1" pattern="$2" log="$3"
  (cd "$root" && go test -buildvcs=false -count=1 -run "$pattern" ./cmd/aurumcode) >"$log" 2>&1
}

base="$work/base"
stage "$base"

ac() {
  local id="$1" pattern="$2" log="$work/$1.log"
  if ! run_test "$base" "$pattern" "$log"; then
    tail -n 30 "$log" >&2
    fail "test-failed"
  fi
  grep -Fq 'ok  ' "$log" || infra "$id-did-not-run"
  printf '%s/%s/pass\n' "$card" "$id"
}

mutate() {
  local id="$1" file="$2" anchor="$3" expr="$4" pattern="$5" assertion="$6"
  local root="$work/$id" log="$work/$id.log"
  stage "$root"
  grep -Fq "$anchor" "$root/$file" || infra "$id-anchor-missing"
  sed -i "$expr" "$root/$file"
  if grep -Fq "$anchor" "$root/$file"; then
    infra "$id-not-applied"
  fi
  if run_test "$root" "$pattern" "$log"; then
    fail "$id-survived"
  fi
  if grep -Fq 'build failed' "$log"; then
    tail -n 20 "$log" >&2
    infra "$id-did-not-compile"
  fi
  grep -Fq -- "$assertion" "$log" || {
    tail -n 20 "$log" >&2
    infra "$id-unexpected-failure"
  }
  printf '%s/%s/rejected (%s)\n' "$card" "$id" "$assertion"
}

mut001() {
  mutate MUT-001 cmd/aurumcode/cmd_changelog.go 'const exitChangelogSuggested = 0' \
    's/const exitChangelogSuggested = 0/const exitChangelogSuggested = 1/' '^TestAUR604AC001' 'suggest mode failed the check'
}

mut002() {
  mutate MUT-002 cmd/aurumcode/pr_changelog_suggestion.go '!p.cfg.ChangelogCheck.Active()' \
    's/!p.cfg.ChangelogCheck.Active()/!p.cfg.ChangelogCheck.Required()/' '^TestAUR604AC001' 'suggest mode left the suggestion out of the review body'
}

# need <file> <literal> fails unless the file carries the literal.
need() {
  [[ -f "$repo_root/$1" ]] || infra "missing-$1"
  grep -Fq -- "$2" "$repo_root/$1" || fail "AC-005/$1/lacks:$2"
}

ac005() {
  local f
  for f in docs/configuration.md docs/tutorials/changelog.md; do
    need "$f" 'mode: suggest'
    need "$f" '`off`'
    need "$f" '`suggest`'
    need "$f" '`required`'
  done
  need docs/tutorials/changelog.md '<!-- saida: modo-sugerir -->'
  need docs/tutorials/changelog.md '```mermaid'
  need docs/tutorials/auditoria-sarif.md '```mermaid'
  for f in docs/configuration.md docs/tutorials/auditoria-sarif.md; do
    need "$f" '--auditoria'
    need "$f" '--sarif'
    need "$f" 'Code scanning'
    need "$f" '"gate"'
  done
  need mkdocs.yml 'pymdownx.superfences'
  need mkdocs.yml 'name: mermaid'
  printf '%s/AC-005/pass\n' "$card"
}

if [[ "$selector" == 'AC-001' ]]; then
  ac AC-001 '^TestAUR604AC001'
elif [[ "$selector" == 'AC-002' ]]; then
  ac AC-002 '^TestAUR604AC002'
elif [[ "$selector" == 'AC-003' ]]; then
  ac AC-003 '^TestAUR604AC003'
elif [[ "$selector" == 'AC-004' ]]; then
  ac AC-004 '^TestAUR604AC004'
elif [[ "$selector" == 'AC-005' ]]; then
  ac005
elif [[ "$selector" == 'MUT-001' ]]; then
  mut001
elif [[ "$selector" == 'MUT-002' ]]; then
  mut002
else
  ac AC-001 '^TestAUR604AC001'
  ac AC-002 '^TestAUR604AC002'
  ac AC-003 '^TestAUR604AC003'
  ac AC-004 '^TestAUR604AC004'
  ac005
  mut001
  mut002
  printf '%s/all/pass\n' "$card"
fi
