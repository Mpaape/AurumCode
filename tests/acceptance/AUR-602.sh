#!/usr/bin/env bash
# AUR-602 acceptance: a refused changelog check still fails, but offers the
# suggested entry (model or commit subjects), redacted, in the log, the job
# summary and the review's PR body; the workflow declares the first
# introduction of the checker instead of a missing-file error.
#
# Selectors:
#   all      AC-001..AC-005, MUT-001, MUT-002
#   AC-001   no entry: exit 1 and the model's suggestion in the output
#   AC-002   no model: deterministic suggestion without agent noise
#   AC-003   the run-time canary never reaches the suggestion
#   AC-004   the --pr review body carries the suggestion block
#   AC-005   base without the checker: notice and exit 0 in the own repo
#   MUT-001  passing a refused check when there is a suggestion turns AC-001 red
#   MUT-002  dropping the redaction of the suggestion turns AC-003 red
# Exit: 0 pass, 1 behavioral failure, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C

readonly card='AUR-602'
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
# AC-005 executes the staged workflow; a module-only skip would prove nothing.
unset AURUMCODE_MODULE_ONLY

work="$(mktemp -d)"
trap 'chmod -R u+w -- "$work" 2>/dev/null || true; rm -rf -- "$work"' EXIT

# stage copies the module (whole cmd, internal, pkg) and the workflow the
# AC-005 test executes into a fresh root.
stage() {
  local root="$1"
  mkdir -p "$root/.github/workflows"
  for item in go.mod go.sum cmd internal pkg; do
    [[ -e "$repo_root/$item" ]] || infra "missing-$item"
    cp -R "$repo_root/$item" "$root/"
  done
  [[ -f "$repo_root/.github/workflows/changelog.yml" ]] || infra missing-changelog-workflow
  cp "$repo_root/.github/workflows/changelog.yml" "$root/.github/workflows/"
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

# mutate applies one sed to a staged copy and requires the named test to
# fail by its assertion, not by a build error.
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
  mutate MUT-001 cmd/aurumcode/cmd_changelog.go 'return exitChangelogRefused' \
    's/return exitChangelogRefused/return 0/' '^TestAUR602AC001' 'passed with a suggestion'
}

mut002() {
  mutate MUT-002 cmd/aurumcode/changelog_suggestion.go 'return filter.Redact(block)' \
    's/return filter.Redact(block)/return block/' '^TestAUR602AC003' 'canary leaked'
}

if [[ "$selector" == 'AC-001' ]]; then
  ac AC-001 '^TestAUR602AC001'
elif [[ "$selector" == 'AC-002' ]]; then
  ac AC-002 '^TestAUR602AC002'
elif [[ "$selector" == 'AC-003' ]]; then
  ac AC-003 '^TestAUR602AC003'
elif [[ "$selector" == 'AC-004' ]]; then
  ac AC-004 '^TestAUR602AC004'
elif [[ "$selector" == 'AC-005' ]]; then
  ac AC-005 '^TestAUR602AC005'
elif [[ "$selector" == 'MUT-001' ]]; then
  mut001
elif [[ "$selector" == 'MUT-002' ]]; then
  mut002
else
  ac AC-001 '^TestAUR602AC001'
  ac AC-002 '^TestAUR602AC002'
  ac AC-003 '^TestAUR602AC003'
  ac AC-004 '^TestAUR602AC004'
  ac AC-005 '^TestAUR602AC005'
  mut001
  mut002
  printf '%s/all/pass\n' "$card"
fi
