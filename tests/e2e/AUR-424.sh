#!/usr/bin/env bash
# AUR-424 E2E: the Go documentation extractor is a standard-library-only
# implementation, and the checked-in fixture it is proved against carries real
# exported symbols with real doc comments. This is deliberately a static,
# byte-level check against the committed source (not a `go run`): it is the
# single fastest, most direct falsifier of MUT-001 (reintroducing the
# gomarkdoc dependency) because that mutation can only pass by putting back
# exactly the imports and literal this script refuses.
set -euo pipefail
export LC_ALL=C
[[ "${1:-E2EAUR424}" == E2EAUR424 ]] || { printf 'AUR-424/AC-001/unknown-selector\n' >&2; exit 64; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" ||
  { printf 'AUR-424/AC-001/infrastructure/repo_root\n' >&2; exit 69; }
command -v grep >/dev/null 2>&1 || { printf 'AUR-424/AC-001/infrastructure/missing_grep\n' >&2; exit 69; }

extractor="$repo_root/internal/documentation/extractors/go/extractor.go"
fixture_dir="$repo_root/tests/fixtures/docs/goproject"

test -s "$extractor" || { printf 'AUR-424/AC-001/behavior-missing: extractor.go absent\n' >&2; exit 1; }
[[ -d "$fixture_dir" ]] || { printf 'AUR-424/AC-001/behavior-missing: card fixture absent\n' >&2; exit 1; }

fail() { printf 'AUR-424/AC-001/%s\n' "$1" >&2; exit 1; }

# No external process, ever: the standard-library rewrite has no subprocess to
# start. Reintroducing os/exec (MUT-001, in whole or in part) trips this.
grep -Fq 'os/exec' "$extractor" && fail 'MUT-001/os-exec-reintroduced'

# The literal gomarkdoc invocation must not be present. This deliberately
# checks for the quoted Go string literal the old code passed to the runner
# (`runner.Run(ctx, "gomarkdoc", ...)`), not the bare word: the fix's own
# doc comments name "gomarkdoc" in prose to explain the defect they close, and
# a check that refused the bare word would flag its own explanation.
grep -Fq '"gomarkdoc"' "$extractor" && fail 'MUT-001/gomarkdoc-reintroduced'

# The fix must actually be the standard library documented in the card: all
# three packages present, none of them merely mentioned in a comment that
# forgot the import.
for pkg in '"go/parser"' '"go/ast"' '"go/doc"'; do
  grep -Fq "$pkg" "$extractor" || fail "behavior-missing: extractor.go does not import $pkg"
done

# Validate must be the always-nil fix, not a re-introduced lookup: this is the
# exact function whose old body called out to gomarkdoc --version.
validate_body="$(awk '
  /^func \(g \*GoExtractor\) Validate\(/ { infunc = 1; next }
  infunc && /^}/ { infunc = 0; exit }
  infunc { print }
' "$extractor")"
if [[ -z "$validate_body" ]]; then
  fail 'behavior-missing: Validate method not found'
fi
if grep -Fq 'runner.Run' <<<"$validate_body"; then
  fail 'MUT-001/validate-calls-runner'
fi
grep -Fq 'return nil' <<<"$validate_body" || fail 'behavior-missing: Validate no longer always succeeds'

# The checked-in fixture carries real exported symbols with real doc
# comments -- the raw material the acceptance and integration proofs parse.
grep -Fq 'package goproject' "$fixture_dir/doc.go" || fail 'behavior-missing: fixture package clause missing'
grep -Fq '// Package goproject' "$fixture_dir/doc.go" || fail 'behavior-missing: fixture package doc comment missing'
grep -Fq 'func Add(a, b int) int' "$fixture_dir/mathutil.go" || fail 'behavior-missing: fixture Add signature missing'
grep -Fq '// Add returns the sum of a and b.' "$fixture_dir/mathutil.go" || fail 'behavior-missing: fixture Add doc comment missing'
grep -Fq 'type Greeting struct' "$fixture_dir/greeting.go" || fail 'behavior-missing: fixture Greeting type missing'
grep -Fq 'func NewGreeting(' "$fixture_dir/greeting.go" || fail 'behavior-missing: fixture NewGreeting missing'

printf 'e2e=ok extractor=stdlib-only fixture=%s\n' "$(basename "$fixture_dir")"
