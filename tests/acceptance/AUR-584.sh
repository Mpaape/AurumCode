#!/usr/bin/env bash
# AUR-584 acceptance: cmd/aurumcode split by responsibility, interface texts in
# the internal/i18n catalog, built-in reviewer profiles in embedded YAML, the
# response parser split with its patterns compiled at package level, and the
# observable output unchanged (e2e tests and every tutorial's --check).
#
# Selectors:
#   all        AC-001..AC-004, then MUT-001..MUT-003
#   AC-001     no production file of cmd/aurumcode over 400 lines, no package
#              doc citing a card, no bool parameter in an exported function,
#              no per-call regexp compile in the parser
#   AC-002     every catalog key exists in pt-BR and en with the same verbs,
#              and no Portuguese language tag outside the catalog
#   AC-003     built-in profiles from YAML give the pinned prompt digest
#   AC-004     e2e review tests, the malformed-skill closure and --check of
#              every recorded tutorial
#   MUT-001    an `if language == "pt-BR"` branch with its own literal in cmd
#              turns AC-002 RED
#   MUT-002    a key removed from the pt-BR catalog turns AC-002 RED
#   MUT-003    one edited built-in instruction turns AC-003 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-584'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|MUT-001|MUT-002|MUT-003) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum cmd internal pkg docs/architecture.md cmd/aurumcode/structure_test.go cmd/aurumcode/aur584_test.go internal/i18n/catalog.yml internal/i18n/i18n_test.go internal/reviewprofile/builtin.yml internal/prompt/parser_patterns_test.go; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a584.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP
mkdir -p "$run_dir/gotmp"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1 -buildvcs=false'
: "${GOCACHE:=$run_dir/gocache}"
export GOCACHE GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

# stage copies the whole module (never enumerated packages) and the docs the
# layer guard reads to a fresh root. demo/ is not copied: the tutorials are
# checked in place, read-only, and their recorded output is large.
stage() {
  local root="$1" source
  mkdir -p "$root"
  for source in go.mod go.sum cmd internal pkg docs; do
    if [[ -e "$repo_root/$source" ]]; then cp -R "$repo_root/$source" "$root/$source"; fi
  done
  for source in tests/fixtures tests/e2e; do
    if [[ -d "$repo_root/$source" ]]; then mkdir -p "$root/tests"; cp -R "$repo_root/$source" "$root/$source"; fi
  done
  chmod -R u+w -- "$root"
}

# drop removes a staged root once its run is judged.
drop() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1"; }

# go_test root log pattern pkgs... runs the named tests; rc is go's.
go_test() {
  local root="$1" log="$2" pattern="$3"; shift 3
  ( cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "$@" ) >"$log" 2>&1
}

# require_pass log names... fails unless every named test passed.
require_pass() {
  local log="$1" name; shift
  for name in "$@"; do
    grep -Eq -- "^--- PASS: ${name} " "$log" || { cat "$log" >&2; fail "missing-pass:$name"; }
  done
}

# replace_once file anchor replacement: a literal, unique edit.
replace_once() {
  local file="$1" anchor="$2" replacement="$3" count
  count="$(grep -Fc -- "$anchor" "$file")" || infra "anchor-missing:${file##*/}"
  [[ "$count" == 1 ]] || infra "anchor-not-unique:${file##*/}"
  ANCHOR="$anchor" REPL="$replacement" awk '
    BEGIN { a = ENVIRON["ANCHOR"]; r = ENVIRON["REPL"] }
    { i = index($0, a); if (i > 0) { print substr($0, 1, i - 1) r substr($0, i + length(a)) } else { print } }
  ' "$file" >"$file.mut" && mv "$file.mut" "$file"
  grep -Fq -- "$replacement" "$file" || infra "mutation-not-applied:${file##*/}"
}

pattern_of() { local IFS='|'; printf '^(%s)$' "$*"; }

readonly ac001_tests=(TestProductionFilesStayShort TestPackageDocNamesNoCard TestNoBoolParameterInExportedFunction TestParserCompilesPatternsOnce TestNoFunctionExceedsLineLimitAnywhere TestNoProductionFileNamedByCardAnywhere TestImportsFollowLayerTable TestAUR558ArchitectureDocCitesEveryInternalPackage)
readonly ac002_tests=(TestEveryKeyExistsInEveryLocale TestCatalogMissingKeyIsRefused TestCatalogVerbMismatchIsRefused TestLocaleOf TestNoLanguageBranchOutsideCatalog TestEveryCatalogKeyUsedExists)
readonly ac003_tests=(TestAUR584BuiltinProfilesKeepThePromptDigest)
readonly ac004_tests=(TestAUR519GateSeverityBreachFailsCheck TestAUR519GateInconclusiveProviderFailureBlocks TestAUR519PRGateInconclusiveBlockTable TestAUR519BaseVerdictWithheldAcrossModelVerdicts TestPRJourneyCarriesConversationAndPublishesDeletionAtBase TestPRHistoryFailureIsVisibleAndHistoryCannotAuthorizeApproval TestAUR584MalformedRepositorySkillDropsOnlyItself TestAUR584MalformedPolicySkillFailsClosedNamingTheFile TestAUR565PolicySkillLoadErrorFailsClosed)
readonly pkgs=(./cmd/aurumcode/ ./internal/i18n/ ./internal/prompt/)

run_ac() {
  local name="$1"; shift
  local root="$run_dir/root-$name" log="$run_dir/$name.log"
  stage "$root"
  go_test "$root" "$log" "$(pattern_of "$@")" "${pkgs[@]}" || { cat "$log" >&2; fail go-test-failed; }
  require_pass "$log" "$@"
  drop "$root"
  printf '%s/%s/pass\n' "$card" "$name"
}

run_ac004() {
  run_ac AC-004 "${ac004_tests[@]}"
  local dir t seen=0
  for dir in "$repo_root"/demo/tutoriais/*/; do
    t="$(basename "$dir")"
    [[ -f "$dir/run.sh" ]] || continue
    seen=$((seen + 1))
    (cd "$repo_root" && bash "demo/tutoriais/$t/run.sh" --check >"$run_dir/tut-$t.log" 2>&1) || { cat "$run_dir/tut-$t.log" >&2; fail "tutorial-check-failed:$t"; }
  done
  ((seen >= 15)) || infra "tutorials-not-seen:$seen"
  printf '%s/AC-004/tutorials/%d/pass\n' "$card" "$seen"
}

# expect_red root log tests...: the mutated copy must compile and turn at
# least one named test RED by its assertion, never by a build error.
expect_red() {
  local root="$1" log="$2"; shift 2
  if go_test "$root" "$log" "$(pattern_of "$@")" "${pkgs[@]}"; then
    cat "$log" >&2; fail mutation-survived
  fi
  if grep -Eq 'build failed|cannot use|undefined:|syntax error|declared and not used|imported and not used|import cycle' "$log"; then
    cat "$log" >&2; fail mutation-did-not-compile
  fi
  grep -Eq -- '^--- FAIL: ' "$log" || { cat "$log" >&2; fail mutation-not-behavioral; }
  grep -E -- '^--- FAIL: |_test\.go:[0-9]+:|^    ' "$log" | sed -n '1,4p' >&2
  drop "$root"
}

# MUT-001: a language branch with its own Portuguese literal comes back.
run_mut001() {
  local root="$run_dir/root-mut1"
  stage "$root"
  replace_once "$root/cmd/aurumcode/passes.go" \
    'return i18n.Text(language, "notice.changelog_unavailable")' \
    'if language == "pt-BR" { return "Changelog indisponivel." }; return i18n.Text(language, "notice.changelog_unavailable")'
  expect_red "$root" "$run_dir/mut1.log" "${ac002_tests[@]}"
  printf '%s/MUT-001/red\n' "$card"
}

# MUT-002: one key disappears from the pt-BR half of the catalog.
run_mut002() {
  local root="$run_dir/root-mut2"
  stage "$root"
  local line
  line="$(grep -n '^  notice.history_unavailable: ' "$root/internal/i18n/catalog.yml" | sed -n '1p' | cut -d: -f1)"
  [[ -n "$line" ]] || infra anchor-missing:catalog.yml
  sed -i "${line}d" "$root/internal/i18n/catalog.yml"
  [[ "$(grep -c '^  notice.history_unavailable: ' "$root/internal/i18n/catalog.yml")" == 1 ]] || infra mutation-not-applied:catalog.yml
  expect_red "$root" "$run_dir/mut2.log" "${ac002_tests[@]}"
  printf '%s/MUT-002/red\n' "$card"
}

# MUT-003: a built-in profile's instruction changes by one word.
run_mut003() {
  local root="$run_dir/root-mut3"
  stage "$root"
  replace_once "$root/internal/reviewprofile/builtin.yml" \
    'retencao de recursos.' 'retencao de memoria.'
  expect_red "$root" "$run_dir/mut3.log" "${ac003_tests[@]}"
  printf '%s/MUT-003/red\n' "$card"
}

case "$selector" in
  AC-001) run_ac AC-001 "${ac001_tests[@]}" ;;
  AC-002) run_ac AC-002 "${ac002_tests[@]}" ;;
  AC-003) run_ac AC-003 "${ac003_tests[@]}" ;;
  AC-004) run_ac004 ;;
  MUT-001) run_mut001 ;;
  MUT-002) run_mut002 ;;
  MUT-003) run_mut003 ;;
  all)
    run_ac AC-001 "${ac001_tests[@]}"
    run_ac AC-002 "${ac002_tests[@]}"
    run_ac AC-003 "${ac003_tests[@]}"
    run_ac004
    run_mut001
    run_mut002
    run_mut003
    printf '%s/all/pass\n' "$card"
    ;;
esac
