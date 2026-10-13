#!/usr/bin/env bash
# AUR-615 acceptance: no stale text in the code (comments and package docs
# describe the product as it is) and the terminal lines that counted with a
# parenthesized plural go through the catalog with a singular and a plural.
#
# Selectors:
#   all      AC-001..AC-003, MUT-001
#   AC-001   every phrase the card lists is gone from the code and the text
#            that replaced it names the real file, function or behavior
#   AC-002   the counted terminal lines use the catalog: the default language
#            keeps its bytes, Portuguese has a singular and a plural
#   AC-003   go build and go vet of the whole module pass, the tests of the
#            packages the card owns pass (the self-contained packages in
#            full, minus the one that reads the shipped config.yml files;
#            in cmd/aurumcode and internal/review the tests of this card and
#            the renamed AUR-607 test, since the sealed tree carries no
#            tests/fixtures), and readJSON is defined once
#   MUT-001  a pt-BR catalog key counting with "(s)" turns the catalog test red
# Exit: 0 pass, 1 behavioral failure, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C

readonly card='AUR-615'
selector="${1:-all}"
known='all AC-001 AC-002 AC-003 MUT-001'
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
for tool in grep sed; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing_$tool"
done
: "${GOCACHE:=$(mktemp -d)}"
export GOCACHE

work="$(mktemp -d)"
trap 'chmod -R u+w -- "$work" 2>/dev/null || true; rm -rf -- "$work"' EXIT

# The Go files the card owns, where no listed phrase may survive.
readonly code_dirs=(cmd/aurumcode internal/config internal/render internal/i18n internal/reviewprofile internal/review internal/gate internal/supplychain internal/sbom internal/feedback)

# search <literal> prints the Go files of the owned directories that carry
# the literal (find + grep -l: no GNU-only grep flag, so a BusyBox grep
# behaves the same).
search() {
  (cd "$repo_root" && find "${code_dirs[@]}" -name '*.go' -exec grep -lF -- "$1" {} + 2>/dev/null) || true
}
[[ -n "$(search 'package config')" ]] || infra 'search-finds-nothing'

# absent <literal>: no Go file of the owned directories carries the literal.
absent() {
  local hits
  hits="$(search "$1")"
  [[ -z "$hits" ]] || fail "phrase-survives:$1:$(printf '%s\n' "$hits" | head -n 1)"
}

# absent_in_code <literal>: like absent, but a test may still spell the
# literal as the bytes it asserts.
absent_in_code() {
  local hits
  hits="$(search "$1" | grep -v '_test\.go$' || true)"
  [[ -z "$hits" ]] || fail "phrase-survives:$1:$(printf '%s\n' "$hits" | head -n 1)"
}

# need <file> <literal>: the file carries the literal.
need() {
  [[ -f "$repo_root/$1" ]] || infra "missing-$1"
  grep -Fq -- "$2" "$repo_root/$1" || fail "$1/lacks:$2"
}

# catalog_twice <key>: the key exists in both locales of the catalog.
catalog_twice() {
  local n
  n="$(grep -c -- "^  $1: " "$repo_root/internal/i18n/catalog.yml" || true)"
  [[ "$n" == 2 ]] || fail "catalog-key-not-in-both-locales:$1:$n"
}

# stage <root>: a fresh copy of the module (go.mod, go.sum, cmd, internal, pkg).
stage() {
  local root="$1"
  mkdir -p "$root"
  for item in go.mod go.sum cmd internal pkg; do
    [[ -e "$repo_root/$item" ]] || infra "missing-$item"
    cp -R "$repo_root/$item" "$root/"
  done
  chmod -R u+w -- "$root"
}

# run_test <root> <pattern> <log> <packages...>: one package at a time, so
# the sealed temporary space never holds several test binaries at once.
run_test() {
  local root="$1" pattern="$2" log="$3"
  shift 3
  (cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "$@") >"$log" 2>&1
}

ac001() {
  # Stale phrases the card lists, and the ones the sweep of the same
  # directories found: files that no longer exist, cards described as future.
  for phrase in \
    'closed list of GateSourceSkills' 'All three entries are Markdown' 'AUR-471' 'future xBOM card' \
    'aur551.go' 'aur520.go' 'aur515.go' 'aur536.go' 'aur537.go' 'aur549.go' 'aur550.go' 'aur521.go' \
    'synthetic until' 'until that card' 'once that card lands' 'until AUR-520 lands' \
    'publishes no numbered release' 'the fixed "review inconclusive' 'exactly two things' \
    'every text the review prints' 'aur607ReadJSON' 'EnglishUnchanged' 'aurumcode docs:' \
    "Camada 1's" 'four later cards' 'corporate-adoption cards each write' 'exceptionsConfigured'; do
    absent "$phrase"
  done
  need internal/config/gate.go 'Only FailOn and Inconclusive declare a gate'
  need internal/config/gate.go 'or any registered scanner'
  need internal/config/config.go 'MCP names configured tool sources'
  need internal/config/config.go 'supply_chain (the Cosign signature of the image'
  need internal/config/qualitygates.go 'cmd_sign.go, cmd/aurumcode'
  need internal/config/qualitygates.go 'ValidateArtifactRef, which cmd_sign.go runs'
  need internal/config/provider_files.go 'DefaultProviders is ConfiguredProviders with no explicit configuration'
  need internal/config/exceptions.go 'internal/gate (MatchException, exceptions.go)'
  need internal/config/exceptions.go '(gate.MatchException)'
  need internal/render/audit.go 'empty when none applied'
  need internal/render/sarif.go 'come from an applied exception'
  need internal/render/finding_identity.go 'pr_rounds.go'
  need cmd/aurumcode/main.go 'AURUMCODE_VERSION build argument (Dockerfile) stamps it'
  need cmd/aurumcode/status_description.go 'gate.inconclusive_line'
  need cmd/aurumcode/status_description.go 'gate.AcceptedExceptionLine'
  need internal/reviewprofile/reviewprofile.go 'INSTRUCTIONS the prompt carries'
  need internal/i18n/aur607_test.go '"(is)"'
  need cmd/aurumcode/aur607_test.go 'TestAUR607PolicyGateStatusDefaultLanguageUnchanged'
  need internal/gate/aur607_test.go 'the default language keeps the code'
  absent_in_code '"Exceptions disabled: '
  need internal/supplychain/signer.go 'cmd_sign.go'
  need internal/sbom/generator.go 'cmd_sbom.go'
  need internal/gate/exceptions.go 'gate.exceptions_identity_unavailable'
  catalog_twice gate.exceptions_identity_unavailable
  printf '%s/AC-001/pass\n' "$card"
}

ac002() {
  local log="$work/ac002.log"
  for phrase in 'comentario(s)' 'manifesto(s)' 'componente(s)' 'descartado(s)' 'finding(s)' 'sinal(is)' 'novo(s)' 'advisory(s)' 'proposta(s)'; do
    absent_in_code "$phrase"
  done
  need cmd/aurumcode/review_pr_publish.go 'publishFailuresText(p.reviewLanguage'
  need cmd/aurumcode/cmd_dependencies.go 'dependenciesSummaryText(cfg.Review.Language'
  need cmd/aurumcode/cmd_xbom.go 'xbomSummaryText(effective.Review.Language'
  need cmd/aurumcode/cmd_feedback.go 'feedbackSummaryText('
  need internal/review/scope.go 'review.scope_discarded'
  need internal/review/reviewer.go 'review.rule_discarded'
  need internal/feedback/plan.go 'feedback.plan_title'
  catalog_twice feedback.plan_title
  catalog_twice feedback.plan_signals
  catalog_twice feedback.plan_signals_one
  catalog_twice feedback.plan_proposals
  catalog_twice feedback.plan_proposals_one
  catalog_twice feedback.plan_discarded
  catalog_twice feedback.plan_discarded_one
  catalog_twice review.rule_discarded
  catalog_twice review.rule_discarded_one
  catalog_twice review.rule_discarded_missing
  catalog_twice review.rule_discarded_unknown
  catalog_twice review.scope_discarded
  catalog_twice review.scope_discarded_one
  for key in terminal.publish_failures terminal.deps.manifests terminal.deps.packages terminal.deps.advisories \
    terminal.xbom.components terminal.xbom.dropped terminal.feedback.signals terminal.feedback.fresh; do
    catalog_twice "$key"
    catalog_twice "${key}_one"
  done
  if ! run_test "$repo_root" 'TestAUR615' "$log" ./cmd/aurumcode ./internal/review ./internal/feedback ./internal/gate; then
    tail -n 30 "$log" >&2
    fail 'counted-lines-test-failed'
  fi
  for name in TestAUR615CountedLinesKeepDefaultBytesAndAgreeInPortuguese TestAUR615RuleDiscardWarningFollowsTheLanguage \
    TestAUR615PlanTitleKeepsTheDefaultBytes TestAUR615RepoIdentityUnavailableNoticeFollowsTheLanguage; do
    grep -Eq -- "^--- PASS: $name " "$log" || fail "counted-lines-test-missing:$name"
  done
  printf '%s/AC-002/pass\n' "$card"
}

ac003() {
  local log="$work/ac003.log" defs
  defs="$(cd "$repo_root" && grep -l -- '^func readJSON(' cmd/aurumcode/*_test.go | wc -l)"
  [[ "$defs" == 1 ]] || fail "readJSON-defined-$defs-times"
  (cd "$repo_root" && go build ./... && go vet ./...) >"$log" 2>&1 || {
    tail -n 40 "$log" >&2
    fail 'build-vet-failed'
  }
  # TestAUR575ExistingConfigsStayValid reads the shipped config.yml files
  # (demo/, .aurumcode/), which the sealed tree does not carry.
  (cd "$repo_root" && go test -count=1 -p 1 -skip 'TestAUR575ExistingConfigsStayValid' ./internal/config ./internal/render ./internal/i18n ./internal/gate ./internal/reviewprofile &&
    go test -count=1 -p 1 ./internal/feedback ./internal/supplychain ./internal/sbom &&
    go test -count=1 -p 1 -run 'TestAUR615|TestAUR607PolicyGateStatusDefaultLanguageUnchanged|TestFilterModelIssues|TestDeletionRegression' ./cmd/aurumcode ./internal/review) >"$log" 2>&1 || {
    grep -E '^(--- FAIL|FAIL|ok)' "$log" | head -n 40 >&2
    tail -n 20 "$log" >&2
    fail 'test-failed'
  }
  printf '%s/AC-003/pass\n' "$card"
}

mut001() {
  local root="$work/mut001" log="$work/mut001.log"
  local file='internal/i18n/catalog.yml' anchor='  terminal.deps.manifests: "%d manifestos"'
  stage "$root"
  grep -Fq -- "$anchor" "$root/$file" || infra 'mut001-anchor-missing'
  sed -i 's|^  terminal.deps.manifests: "%d manifestos"$|  terminal.deps.manifests: "%d manifesto(s)"|' "$root/$file"
  if grep -Fq -- "$anchor" "$root/$file"; then
    infra 'mut001-not-applied'
  fi
  if run_test "$root" 'TestAUR607PortugueseHasNoParenthesizedPlural' "$log" ./internal/i18n; then
    fail 'mut001-survived'
  fi
  grep -Eq -- '^--- FAIL: TestAUR607PortugueseHasNoParenthesizedPlural' "$log" || {
    tail -n 20 "$log" >&2
    infra 'mut001-unexpected-failure'
  }
  rm -rf -- "$root"
  printf '%s/MUT-001/rejected (TestAUR607PortugueseHasNoParenthesizedPlural)\n' "$card"
}

case "$selector" in
  AC-001) ac001 ;;
  AC-002) ac002 ;;
  AC-003) ac003 ;;
  MUT-001) mut001 ;;
  all) ac001; ac002; ac003; mut001 ;;
esac
