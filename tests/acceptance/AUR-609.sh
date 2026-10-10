#!/usr/bin/env bash
# AUR-609 acceptance: the redaction before the prompt masks only literals and
# secret-shaped tokens, never an identifier, a call, an expression or a
# comparison operator; the analysis catalog skips code rules on prose files
# and command-injection never fires on a mention inside a string literal,
# while SQL concatenated inside a string stays a finding.
#
# Selectors:
#   all      AC-001..AC-005, MUT-001..MUT-006
#   AC-001   the seven expression/operator forms pass through intact
#   AC-002   a password literal, a weak bare password, a bare token, a JWT and
#            an AKIA key stay masked
#   AC-003   a docstring or string documenting subprocess/exec is no finding;
#            the real call still is, and so is SQL built inside a string
#   AC-004   the same line in README.md or .log is no code-rule finding;
#            a secret in prose still is
#   AC-005   the gate (fail_on: [error]) does not fail the docstring diff
#   MUT-001  no code-shape check on bare values turns AC-001 red
#   MUT-002  no literal guard on command-injection turns AC-003 red
#   MUT-003  no prose skip turns AC-004 red
#   MUT-004  an unanchored call grammar leaks a bare secret (AC-002 red)
#   MUT-005  brackets paired across line breaks leak a bare secret (AC-002 red)
#   MUT-006  a key search that reads the value again turns the redaction
#            quadratic (AC-002 red)
# Exit: 0 pass, 1 behavioral failure, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C

readonly card='AUR-609'
selector="${1:-all}"
known='all AC-001 AC-002 AC-003 AC-004 AC-005 MUT-001 MUT-002 MUT-003 MUT-004 MUT-005 MUT-006'
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

readonly pkgs=(./internal/security/redaction ./internal/analysis ./internal/prosefiles)

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

# run_test runs the named tests of the three packages in root.
run_test() {
  local root="$1" pattern="$2" log="$3"
  (cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "${pkgs[@]}") >"$log" 2>&1
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

ac001() {
  ac AC-001 '^TestAUR609(KeyValueKeepsExpressionsAndOperators|AssignDoesNotEatComparisonOperators|BareSecretOrCode)$' \
    TestAUR609KeyValueKeepsExpressionsAndOperators TestAUR609AssignDoesNotEatComparisonOperators TestAUR609BareSecretOrCode
}
ac002() {
  ac AC-002 '^TestAUR609(KeyValueStillMasksLiteralsAndTokens|BareWeakSecretStaysMasked|BareSecretOrCode|MemberChainMaskedWhole|RedactionIsLinear)$' \
    TestAUR609KeyValueStillMasksLiteralsAndTokens TestAUR609BareWeakSecretStaysMasked TestAUR609BareSecretOrCode \
    TestAUR609MemberChainMaskedWhole TestAUR609RedactionIsLinear
}
ac003() {
  ac AC-003 '^TestAUR609(DocstringMentionIsNotCommandInjection|MultilineDocstringIsNotCommandInjection|RealCallStillCommandInjection|SQLInsideStringStillFound)$' \
    TestAUR609DocstringMentionIsNotCommandInjection TestAUR609MultilineDocstringIsNotCommandInjection \
    TestAUR609RealCallStillCommandInjection TestAUR609SQLInsideStringStillFound
}
ac004() {
  ac AC-004 '^TestAUR609(ProseFilesSkipCodeRules|SecretInProseStillFound|IsProsePath)$' \
    TestAUR609ProseFilesSkipCodeRules TestAUR609SecretInProseStillFound TestAUR609IsProsePath
}
ac005() {
  ac AC-005 '^TestAUR609GateDoesNotFailOnDocstring$' TestAUR609GateDoesNotFailOnDocstring
}

mut001() {
  mutate MUT-001 internal/security/redaction/redaction.go 'if t.codeValue(s, start, end) {' \
    's/if t.codeValue(s, start, end) {/if false \&\& t.codeValue(s, start, end) {/' \
    '^TestAUR609KeyValueKeepsExpressionsAndOperators$' 'TestAUR609KeyValueKeepsExpressionsAndOperators'
}
mut002() {
  mutate MUT-002 internal/analysis/literalguard.go 'if !isInsideStringLiteral(body, pos, inRaw) {' \
    's/if !isInsideStringLiteral(body, pos, inRaw) {/if true {/' \
    '^TestAUR609DocstringMentionIsNotCommandInjection$' 'TestAUR609DocstringMentionIsNotCommandInjection'
}
mut003() {
  mutate MUT-003 internal/analysis/analysis.go 'if prose && rule.codeOnly {' \
    's/if prose \&\& rule.codeOnly {/if prose \&\& rule.codeOnly \&\& false {/' \
    '^TestAUR609ProseFilesSkipCodeRules$' 'TestAUR609ProseFilesSkipCodeRules'
}

mut004() {
  mutate MUT-004 internal/security/redaction/redaction.go 'return ok && expressionStop(s, end) && (end >= valueEnd || int(t.closers[end]) >= valueEnd)' \
    's/return ok \&\& expressionStop(s, end) \&\& (end >= valueEnd || int(t.closers\[end\]) >= valueEnd)/return ok \&\& end >= 0/' \
    '^TestAUR609BareSecretOrCode$' 'TestAUR609BareSecretOrCode'
}

# MUT-005 pairs brackets across line breaks again.
mut005() {
  mutate MUT-005 internal/security/redaction/redaction.go "if c == '\\r' || c == '\\n' {" \
    's/if c == .\\r. || c == .\\n. {/if false {/' \
    '^TestAUR609BareSecretOrCode$' 'TestAUR609BareSecretOrCode'
}

# MUT-006 lets the key search consume the bare value again, so every resumed
# search inside a kept call reads the rest of the line.
mut006() {
  mutate MUT-006 internal/security/redaction/redaction.go 'assign + `)`)' \
    's/assign + `)`)$/assign + `)` + bareValue)/' \
    '^TestAUR609RedactionIsLinear$' 'TestAUR609RedactionIsLinear'
}

# One function per selector; all runs every one in order.
if [[ "$selector" == 'all' ]]; then
  for step in ac001 ac002 ac003 ac004 ac005 mut001 mut002 mut003 mut004 mut005 mut006; do
    "$step"
  done
  printf '%s/all/pass\n' "$card"
else
  step="${selector//-/}"
  step="${step,,}"
  "$step"
fi
