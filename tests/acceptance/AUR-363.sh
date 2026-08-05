#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-363'
selector="${1:-AC-001}"
case "$selector" in
  AC-001|ContractAUR363|IntegrationAUR363|E2EAUR363) ;;
  *) printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64 ;;
esac

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd -P)"
source_file="$repo_root/.board/bootstrap/locks/parsers.yml"

fail() {
  printf '%s/AC-001/%s\n' "$card" "$1" >&2
  exit 1
}

# infra() is reserved for what this card does NOT own as a behavioral claim:
# a missing tool, a bound exceeded, this program's own fixture machinery
# failing to reproduce. An environment gap must never be laundered into
# behavioral red. See tests/acceptance/EXIT_CODE_CONVENTION.md.
infra() {
  printf '%s/AC-001/inconclusive/%s\n' "$card" "$1" >&2
  exit 79
}

[[ -f "$source_file" && ! -L "$source_file" ]] || fail parser_lock_incomplete
grep -Fqx 'schema: bootstrap-lock-v1' "$source_file" || fail invalid_lock_or_manifest
grep -Eq 'sha256:[0-9a-f]{64}' "$source_file" || fail invalid_lock_or_manifest
if grep -Eiq '(^|[^a-z])latest([^a-z]|$)|@[[:space:]]*(main|master|head)([^a-z]|$)' "$source_file"; then
  fail mutable_reference
fi

# ---------------------------------------------------------------------------
# Deeper per-adapter verification.
#
# The four checks above are this card's original sealed contract and are
# left byte-for-byte untouched above this line. On their own they cannot
# ever produce two of the three disposition words the card's own Skeptical
# mutations section (MUT-001 `parser-unlocked`, MUT-002
# `corpus-digest-mismatch`) requires: neither word appears anywhere above,
# and no check above associates a sha256 occurrence with the specific field
# or file it is supposed to bind, or cross-references the declared adapter
# set against the extractor adapters that actually exist in the repository.
# That is a real gap between what tests/acceptance/AUR-363.sh could prove
# and what the card's own TDD proof commits to proving for MUT-001/MUT-002,
# so this section extends the program strictly additively: every check
# above is unchanged, in the same order, with the same patterns, and every
# new disposition word introduced below is new vocabulary, never a
# replacement for an existing one.
#
# verify_adapter_field <root> <adapter> <field>
#   Prints the value of `adapter_<adapter>_<field>: <value>` from
#   `<root>/.board/bootstrap/locks/parsers.yml`, or nothing if absent.
# ---------------------------------------------------------------------------
verify_adapter_field() {
  local root="$1" adapter="$2" field="$3" lock="$root/.board/bootstrap/locks/parsers.yml"
  local key="adapter_${adapter}_${field}"
  local line
  line="$(grep -F -- "${key}: " "$lock" 2>/dev/null | head -n1)" || true
  [[ -n "$line" ]] || return 1
  printf '%s\n' "${line#"${key}: "}"
}

# 16 KiB: a small flat manifest that lists eight adapters, not free-form
# input. The real lock is ~5 KiB; this leaves headroom for future adapters
# without redefining the bound.
readonly max_lock_bytes=16384
readonly max_read_bytes=4194304
readonly grammar_version_pattern='^(unpinned|not-applicable|v[0-9]+\.[0-9]+\.[0-9]+)$'

# verify_parsers_lock <root>
#   Prints one of: pass | parser_lock_incomplete | invalid_lock_or_manifest |
#   input_limit_exceeded | parser-unlocked | corpus-digest-mismatch |
#   grammar-ref-mutable | source_integrity_error
#   Returns: 0 pass; 1 behavioral (the printed word is the typed code);
#            2 infra (the printed word is the infra reason).
# Reuses the exact same four checks already proven above against the real
# tree, then adds the per-adapter cross-reference, corpus-digest and
# grammar-version-pin checks the sealed four cannot express. No parser is
# instantiated and no file above max_read_bytes is ever opened for hashing.
verify_parsers_lock() {
  local root="$1"
  local lock="$root/.board/bootstrap/locks/parsers.yml"
  local extractors_dir="$root/internal/documentation/extractors"
  local bytes

  [[ -e "$lock" ]] || { printf 'parser_lock_incomplete\n'; return 1; }
  [[ -f "$lock" && ! -L "$lock" ]] || { printf 'parser_lock_incomplete\n'; return 1; }
  [[ -r "$lock" ]] || { printf 'lock_unreadable\n'; return 2; }
  bytes="$(wc -c <"$lock")" || { printf 'lock_unreadable\n'; return 2; }
  [[ "$bytes" =~ ^[0-9]+$ ]] || { printf 'lock_size_unreadable\n'; return 2; }
  (( bytes > 0 )) || { printf 'invalid_lock_or_manifest\n'; return 1; }
  (( bytes <= max_lock_bytes )) || { printf 'input_limit_exceeded\n'; return 1; }
  grep -Fqx 'schema: bootstrap-lock-v1' "$lock" || { printf 'invalid_lock_or_manifest\n'; return 1; }
  grep -Eq 'sha256:[0-9a-f]{64}' "$lock" || { printf 'invalid_lock_or_manifest\n'; return 1; }
  if grep -Eiq '(^|[^a-z])latest([^a-z]|$)|@[[:space:]]*(main|master|head)([^a-z]|$)' "$lock"; then
    printf 'mutable_reference\n'; return 1
  fi

  # -- cross-reference: every adapter directory that actually exists in the
  #    tree must have a matching declared adapter line. An adapter directory
  #    absent from the declared set is unlocked source, not evidence.
  if [[ -d "$extractors_dir" ]]; then
    local dir name
    for dir in "$extractors_dir"/*/; do
      [[ -e "${dir}extractor.go" ]] || continue
      name="$(basename -- "$dir")"
      grep -Fqx "adapter_${name}_language: ${name}" "$lock" || { printf 'parser-unlocked\n'; return 1; }
    done
  fi

  # -- per declared adapter: grammar version pin format, corpus digest,
  #    implementation digest, max_file_bytes.
  local adapters adapter mfb gv impl impl_sha corpus corpus_sha rel_impl rel_corpus actual bytes2 line
  adapters="$(grep -Eo '^adapter_[A-Za-z0-9]+_language:' "$lock" | sed -E 's/^adapter_(.*)_language:$/\1/')" || true
  [[ -n "$adapters" ]] || { printf 'invalid_lock_or_manifest\n'; return 1; }
  while IFS= read -r adapter; do
    [[ -n "$adapter" ]] || continue

    gv="$(verify_adapter_field "$root" "$adapter" grammar_version)" || { printf 'invalid_lock_or_manifest\n'; return 1; }
    [[ "$gv" =~ $grammar_version_pattern ]] || { printf 'grammar-ref-mutable\n'; return 1; }

    mfb="$(verify_adapter_field "$root" "$adapter" max_file_bytes)" || { printf 'invalid_lock_or_manifest\n'; return 1; }
    [[ "$mfb" == "4194304" ]] || { printf 'invalid_lock_or_manifest\n'; return 1; }

    rel_impl="$(verify_adapter_field "$root" "$adapter" implementation_path)" || { printf 'invalid_lock_or_manifest\n'; return 1; }
    impl_sha="$(verify_adapter_field "$root" "$adapter" implementation_sha256)" || { printf 'invalid_lock_or_manifest\n'; return 1; }
    [[ "$impl_sha" =~ ^sha256:[0-9a-f]{64}$ ]] || { printf 'invalid_lock_or_manifest\n'; return 1; }
    impl="$root/$rel_impl"
    [[ -e "$impl" ]] || { printf 'source_integrity_error\n'; return 1; }
    [[ -f "$impl" && ! -L "$impl" ]] || { printf 'source_integrity_error\n'; return 1; }
    [[ -r "$impl" ]] || { printf 'implementation_unreadable\n'; return 2; }
    bytes2="$(wc -c <"$impl")" || { printf 'implementation_unreadable\n'; return 2; }
    [[ "$bytes2" =~ ^[0-9]+$ ]] || { printf 'implementation_size_unreadable\n'; return 2; }
    (( bytes2 <= max_read_bytes )) || { printf 'input_limit_exceeded\n'; return 1; }
    line="$(sha256sum -- "$impl")" || { printf 'implementation_unreadable\n'; return 2; }
    actual="sha256:${line%% *}"
    [[ "$actual" == "$impl_sha" ]] || { printf 'source_integrity_error\n'; return 1; }

    rel_corpus="$(verify_adapter_field "$root" "$adapter" fixture_corpus_path)" || { printf 'invalid_lock_or_manifest\n'; return 1; }
    corpus_sha="$(verify_adapter_field "$root" "$adapter" fixture_corpus_sha256)" || { printf 'invalid_lock_or_manifest\n'; return 1; }
    [[ "$corpus_sha" =~ ^sha256:[0-9a-f]{64}$ ]] || { printf 'invalid_lock_or_manifest\n'; return 1; }
    corpus="$root/$rel_corpus"
    [[ -e "$corpus" ]] || { printf 'corpus-digest-mismatch\n'; return 1; }
    [[ -f "$corpus" && ! -L "$corpus" ]] || { printf 'corpus-digest-mismatch\n'; return 1; }
    [[ -r "$corpus" ]] || { printf 'corpus_unreadable\n'; return 2; }
    bytes2="$(wc -c <"$corpus")" || { printf 'corpus_unreadable\n'; return 2; }
    [[ "$bytes2" =~ ^[0-9]+$ ]] || { printf 'corpus_size_unreadable\n'; return 2; }
    (( bytes2 <= max_read_bytes )) || { printf 'input_limit_exceeded\n'; return 1; }
    line="$(sha256sum -- "$corpus")" || { printf 'corpus_unreadable\n'; return 2; }
    actual="sha256:${line%% *}"
    [[ "$actual" == "$corpus_sha" ]] || { printf 'corpus-digest-mismatch\n'; return 1; }
  done <<<"$adapters"

  printf 'pass\n'
  return 0
}

set +e
deep_word="$(verify_parsers_lock "$repo_root")"
deep_rc=$?
set -e
case "$deep_rc" in
  0) ;;
  1) fail "$deep_word" ;;
  *) infra "$deep_word" ;;
esac

# ---------------------------------------------------------------------------
# Fixture sensitivity self-test. Only once the real lock verifies clean does
# this program trust itself to judge sealed fixtures against
# tests/bootstrap/locks/AUR-363/cases.tsv: case name, fixture subdirectory
# under tests/bootstrap/locks/AUR-363/_fixtures/, the recomputed SHA-256 of
# the fixture's own lock (or `-` for deliberate absence), and the expected
# disposition. A fixture whose recomputed digest diverges from the sealed
# row is this program's own setup being unsound: infra, never a verdict
# about the real tree already accepted above.
#
# The fixtures tree lives under a `_`-prefixed directory name
# (`_fixtures/`, not `fixtures/`) on purpose: every fixture below embeds a
# synthetic `internal/documentation/extractors/<adapter>/extractor.go`
# tree, including one (`implementationsymlink`) where `extractor.go` is a
# real symlink to a sibling `real_impl.go` that declares the same type. Go
# module-aware package discovery (`go build ./...`, `go vet ./...`) skips
# any path component beginning with `.` or `_`, so the `_` prefix keeps
# these fixtures reachable by name and by `sha256sum` for this script
# while remaining invisible to the Go toolchain's own package scan of the
# whole repository — no fixture is removed or weakened to achieve that.
# ---------------------------------------------------------------------------
readonly cases_rel='tests/bootstrap/locks/AUR-363/cases.tsv'
readonly fixtures_rel='tests/bootstrap/locks/AUR-363/_fixtures'
readonly max_cases_bytes=4194304
readonly max_cases_rows=256

cases_file="$repo_root/$cases_rel"
[[ -e "$cases_file" ]] || infra 'cases_tsv_absent'
[[ -f "$cases_file" && ! -L "$cases_file" ]] || infra 'cases_tsv_unreadable'
[[ -r "$cases_file" ]] || infra 'cases_tsv_unreadable'
cases_bytes="$(wc -c <"$cases_file")" || infra 'cases_tsv_unreadable'
[[ "$cases_bytes" =~ ^[0-9]+$ ]] || infra 'cases_tsv_unreadable'
(( cases_bytes > 0 && cases_bytes <= max_cases_bytes )) || infra 'cases_tsv_out_of_bounds'

case_count=0
while IFS=$'\t' read -r c_name c_fixture c_input_sha c_expect || [[ -n "$c_name" ]]; do
  [[ -n "$c_name" ]] || continue
  [[ "${c_name:0:1}" != '#' ]] || continue
  case_count=$(( case_count + 1 ))
  (( case_count <= max_cases_rows )) || infra 'cases_tsv_exceeds_row_bound'

  fixture_root="$repo_root/$fixtures_rel/$c_fixture"
  [[ -d "$fixture_root" ]] || infra "fixture_directory_absent:$c_fixture"

  fixture_lock="$fixture_root/.board/bootstrap/locks/parsers.yml"
  if [[ "$c_input_sha" == '-' ]]; then
    [[ ! -e "$fixture_lock" ]] || infra "fixture_expected_absent:$c_fixture"
  else
    [[ -e "$fixture_lock" && -r "$fixture_lock" ]] || infra "fixture_lock_unreadable:$c_fixture"
    fixture_digest_line="$(sha256sum -- "$fixture_lock")" || infra "fixture_unhashable:$c_fixture"
    fixture_digest="${fixture_digest_line%% *}"
    [[ "$fixture_digest" == "$c_input_sha" ]] || infra "fixture_digest_diverges:$c_fixture"
  fi

  set +e
  case_word="$(verify_parsers_lock "$fixture_root")"
  case_rc=$?
  set -e
  case "$case_rc" in
    0|1) ;;
    *) infra "fixture_case_infra:$c_fixture/$case_word" ;;
  esac
  [[ "$case_word" == "$c_expect" ]] ||
    fail 'fixture_sensitivity_error'
done < "$cases_file"
(( case_count > 0 )) || infra 'cases_tsv_empty'

printf '{"card":"%s","scenario":"AC-001","selector":"%s","fixture_cases":%d,"result":"pass"}\n' \
  "$card" "$selector" "$case_count"
