#!/usr/bin/env bash
# tests/acceptance/AUR-364.sh — AUR-364 (fixar ferramentas de documentação).
#
# ============================================================================
# WHY THIS FILE WAS REDESIGNED AGAIN (round 4 of the reprojeto)
# ============================================================================
# Rounds 1-6 verified a hand-authored lock field by field. Round 7 replaced
# authored prose with a DERIVED lock. Round 8 added an "independent" scanner
# that was itself a list of install VERBS. Round 9 (redesign r3) deleted the
# verb list and sealed every non-manifest read_path by whole-file digest.
#
# Two blind reviews broke r3, and both findings were the SAME class in two
# disguises: a check expressed as a comparison of CARDINALITY standing in for
# a comparison of SETS, and a security label ("pinned") computed by a SYNTAX
# HEURISTIC over dependency constraints.
#
#   B1  the pin heuristic was "the constraint contains a digit", so
#       `gem "x", "0"` and `gem "x", ">= 0"` — both of which match ANY
#       published version — were labelled pinned:true / pinned-lockable.
#   B2  the pin was computed per NAME over the concatenation of the
#       constraints of EVERY manifest citing that name, so one manifest
#       pinning a name laundered a genuinely version-less declaration in
#       another manifest.
#   B3  a declaration with no version at all was labelled
#       unpinned-must-not-install and then... passed, because the oracle only
#       checked that its own counters agreed with each other. Internal
#       consistency is not a security property.
#   B4  the read_path attestation compared the NUMBER of rows against the
#       number of canonical read_paths and never marked a path as seen, so
#       duplicating one row in place of another kept the count and dropped a
#       whole file out of the seal.
#
# ============================================================================
# THE REDESIGN: STOP JUDGING CONSTRAINTS, READ THE RESOLUTION
# ============================================================================
# The semantic analysis of dependency-constraint languages is deleted, not
# extended. There is no `pinned` field, no operator parsing, no digit test,
# no notion of "is this a real pin?" anywhere in this file. That surface was
# unbounded (four ecosystems, every operator, every degenerate bound) and it
# was the direct source of B1, B2 and B3.
#
# What replaces it is the property a resolved lockfile already has by
# construction — EXACTLY ONE resolved version per tool:
#
#   PROPERTY R. For every manifest M and every dependency name N that M's
#   grammar yields, the card's RESOLUTION artifact must contain exactly one
#   record (M, N) carrying an EXACT release version and an artifact digest.
#   And for every record (M, N) in the resolution, M's grammar must yield N.
#   The two sides are compared as SETS, keyed on the PAIR (manifest, name) —
#   never on the name alone (B2), never by cardinality (B4).
#
# Consequences, stated as totals rather than as a list of covered cases:
#   * `gem "x"`, `gem "x", "0"`, `gem "x", ">= 0"`, `gem "x", "~> 0"` and
#     every other constraint form a future Bundler may invent are treated
#     IDENTICALLY: the constraint is recorded verbatim and never interpreted,
#     and the declaration fails unless the resolution names an exact version
#     for that exact (manifest, name) pair.
#   * A declaration with no resolution record is a typed failure
#     (docs_tool_lock_incomplete), never an omission.
#   * A resolution record with no declaration is a typed failure
#     (invalid_lock_or_manifest): the resolution cannot carry dead weight
#     that nothing in the repository asked for.
#   * If a manifest format's required resolution source does not exist in the
#     repository, that is a typed failure (docs_tool_lock_incomplete), never
#     a silent skip.
#
# DERIVATION 2 (unchanged from r3, and unbroken by either review). Every
# read_path that is NOT a machine-readable manifest is NOT interpreted at
# all: it enters the lock as the sha256 of the ENTIRE file, sealed a second
# time in tests/bootstrap/locks/AUR-364/read-paths.attested, which
# `--generate` never writes. `brew install`, `curl ... | sh`, a base64 blob,
# or a syntax nobody has invented yet all change the file's bytes and all
# fail — not because this file recognises them, but because it never needed
# to.
#
# ============================================================================
# SETS, NEVER COUNTS
# ============================================================================
# Every coverage check in this file marks each canonical item as SEEN in an
# associative array, rejects a duplicate, and then asserts that EVERY item was
# seen BY NAME. There is no `(( a == b ))` between two derived counts anywhere
# in this file; the only arithmetic comparisons that remain are:
#   * `# NONEMPTY` — Lei 12 item 4: a derived set of size zero is a FAILURE.
#   * `# BOUND`    — a size or limit test against a declared constant.
#   * `# ACCOUNTING` — Regra dos Seis item 3: the per-line classification sum
#     inside the manifest/resolution parsers, which proves no `continue` ever
#     dropped a physical line. That is a statement about the parser's own
#     traversal, not a stand-in for a set comparison; the sets themselves are
#     compared by the SEEN maps below.
# `tests/bootstrap/locks/AUR-364/verify-fixtures.sh` re-greps this file and
# fails if any other arithmetic comparison appears. Its detector is validated
# against a synthetic positive control, so an empty grep cannot be mistaken for
# a clean file.
#
# ============================================================================
# NO DECISION IS TAKEN FROM A PIPELINE'S STATUS
# ============================================================================
# `set -o pipefail` plus a consumer that exits as soon as it has an answer
# (`grep -q`, `head`) makes the pipeline's exit status depend on a RACE: the
# producer is killed by SIGPIPE, the pipeline reports 141, and the surrounding
# `if` reads that as "no match" — the check silently DOES NOT HAPPEN. That is
# the same class as B1-B4, and it was live here: a 200-run stress of the
# NOMINAL case, with byte-identical inputs, failed 7 times.
#
# Every decision below therefore comes from a SINGLE command's own status or
# from a here-string (`grep -Fq ... <<< "$text"`), never from a pipeline whose
# head can be killed. The `no-pipefail-race` oracle in verify-fixtures.sh
# forbids the shape mechanically, and is likewise validated by a positive
# control.
#
# ============================================================================
# ENVIRONMENT
# ============================================================================
# Runs inside `bootstrap-readonly-v1`:
# bash@sha256:ae4668c2560999e65e89532cd2ad1b6688bb23298189f0bd229ef80fa4bd0831
# — real GNU bash 5 on Alpine 3.22, but awk/grep/sed/stat/sha256sum/tr/wc are
# BusyBox applets. Only POSIX ERE is used; no grep -P, no GNU long options.
# BusyBox awk/grep mis-handle NUL (0x00) and bash's `read` drops NUL
# silently, so NUL detection here is done by byte accounting (`wc -c` before
# and after `tr -d '\000'`), never by awk/grep/read — see Lei 15.
# ============================================================================
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-364'

fail() {
  printf '%s/AC-001/%s\n' "$card" "$1" >&2
  exit 1
}

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd -P)"
readonly repo_root
readonly lock_file="$repo_root/.board/bootstrap/locks/docs.yml"
readonly attest_file="$repo_root/tests/bootstrap/locks/AUR-364/read-paths.attested"

# The card's OTHER two declared paths. Lei 20: a path a card declares but the
# accept never opens is a claim nobody checks — remove it from the tree and
# the oracle stays green while the card goes on asserting what the file says.
# Both are opened below: this script by its own bytes (section 3b derives the
# set of error codes it can emit from them), and the spec both by digest (it
# is sealed in the lock like every other card artifact) and by content (its
# error-code table must equal that derived set).
readonly SPEC_REL='docs/specs/AUR-364.md'
readonly spec_file="$repo_root/$SPEC_REL"
readonly SELF_REL='tests/acceptance/AUR-364.sh'
readonly self_file="$repo_root/$SELF_REL"

# The card's FOURTH declared path is a DIRECTORY, and until round 5 only two of
# the files inside it (resolved.lock, read-paths.attested) were ever opened.
# cases.tsv, mutations.sh and verify-fixtures.sh — the card's entire TDD
# apparatus — could be deleted or rewritten and this accept stayed green. That
# is Lei 20 at file granularity, and the fix is NOT a hand-typed list of the
# three names: the set is DERIVED by listing the declared directory, so a file
# added to it tomorrow is sealed too, and a file removed from it is missing
# from the derivation and fails the whole-artifact equality of section 8.
readonly FIXTURE_DIR_REL='tests/bootstrap/locks/AUR-364'
readonly fixture_dir="$repo_root/$FIXTURE_DIR_REL"
readonly MAX_FIXTURE_ENTRIES=64
readonly MAX_FIXTURE_BYTES=262144
# Filled by assert_card_fixtures, consumed by generate_lock and by section 7k.
declare -a CARD_FIXTURES=()

# The card's declared read_paths, in the card's own order. Every one of them
# must exist, be a regular file, be NUL-free, and be either parsed as a
# manifest or sealed as a document. None may be skipped.
readonly -a CANONICAL_READ_PATHS=(
  'Gemfile'
  '.aurumcode/Gemfile'
  '.docker/docs.Dockerfile'
  '.github/workflows/documentation.yml'
)

readonly MAX_LOCK_BYTES=131072
readonly MAX_LOCK_LINES=1024
readonly MAX_READ_PATH_BYTES=1048576
readonly MAX_ATTEST_BYTES=8192
readonly MAX_RESOLUTION_BYTES=65536
readonly MAX_RESOLUTION_LINES=256
readonly MAX_SPEC_BYTES=131072
readonly MAX_SPEC_LINES=1024

# Trust anchor (policy constant, disclosed in nonderivable[]): the single gem
# registry a Bundler manifest of this card may install from. It is an
# allowlist of one; a manifest naming any other origin fails. It is not a
# blocklist of known-bad origins, so an origin nobody anticipated fails too.
readonly TRUSTED_GEM_REGISTRY='https://rubygems.org'

# The resolution artifact's sealed envelope. Both are allowlists of one, so an
# unanticipated schema or digest domain FAILS instead of being accepted.
readonly RESOLUTION_SCHEMA='bootstrap-resolution-v1'
readonly RESOLUTION_DIGEST_DOMAIN='synthetic-offline-identifier-v1'

# An EXACT release: three numeric components, nothing else. No operator, no
# range, no wildcard, no prerelease suffix. This regex is the entire
# definition of "resolved" in this card; it replaces every heuristic that r3
# used to decide whether a constraint was a pin.
readonly RE_EXACT_VERSION='^[0-9]+\.[0-9]+\.[0-9]+$'
readonly RE_DIGEST='^sha256:[0-9a-f]{64}$'

# ---------------------------------------------------------------------------
# FORMAT REGISTRY. Maps a read_path basename to a dependency-manifest format.
# Total by construction and fail-closed in both directions:
#   * a basename that IS a known manifest format and HAS a parser -> parsed
#     by that grammar;
#   * a basename that IS a known manifest format and has NO parser ->
#     unsupported-manifest-format, a hard failure (never guessed at, never
#     silently demoted to an opaque document);
#   * anything else -> opaque document, sealed whole-file by digest.
# No branch of this function can cause a check to pass by absence.
# ---------------------------------------------------------------------------
manifest_format_of() {
  case "${1##*/}" in
    Gemfile|gems.rb)                       printf 'bundler' ;;
    go.mod)                                printf 'gomod' ;;
    package.json)                          printf 'npm' ;;
    requirements.txt|constraints.txt)      printf 'pip' ;;
    Pipfile|pyproject.toml|poetry.lock)    printf 'python-other' ;;
    Cargo.toml|composer.json|pom.xml)      printf 'other-ecosystem' ;;
    build.gradle|mix.exs|Package.swift)    printf 'other-ecosystem' ;;
    *)                                     printf '' ;;
  esac
}

# Runtime family implied by a manifest FORMAT (a property of the format, not
# of any tool name).
runtime_of_format() {
  case "$1" in
    bundler) printf 'ruby' ;;
    gomod)   printf 'go' ;;
    npm)     printf 'node' ;;
    pip)     printf 'python' ;;
    *)       printf '' ;;
  esac
}

# The parser registered for a format, or empty. Empty means the accept stops;
# it never means "treat it as prose".
parser_of_format() {
  case "$1" in
    bundler) printf 'scan_bundler' ;;
    *)       printf '' ;;
  esac
}

# ---------------------------------------------------------------------------
# THE REQUIRED RESOLUTION SOURCE for a manifest format, as a repo-relative
# path. This is the "lockfile" side of Property R.
#
# Bundler's ecosystem-native resolved lockfile is `Gemfile.lock`. This
# repository ships none — no `Gemfile.lock` and no `.aurumcode/Gemfile.lock`
# exist, and neither is a read_path of this card, so neither is materialised
# into the accept container. That fact is DISCLOSED in limits[] of the
# generated lock and in docs/specs/AUR-364.md; it is not worked around by
# skipping the check. The card therefore carries its own resolution artifact,
# card-scoped exactly as the card's Public contract requires, and its ABSENCE
# is a typed failure (docs_tool_lock_incomplete), never an omission.
# ---------------------------------------------------------------------------
required_resolution_of_format() {
  case "$1" in
    bundler) printf 'tests/bootstrap/locks/AUR-364/resolved.lock' ;;
    *)       printf '' ;;
  esac
}

# One allowlisted invocation template per FORMAT (not per tool). Every tool
# derived from a manifest of that format carries this same command, so no
# per-name command table exists anywhere.
allowlisted_command_of_format() {
  case "$1" in
    bundler) printf "bundle config set --local path 'vendor/bundle' && bundle install --deployment && bundle exec <tool>" ;;
    *)       printf 'n/a' ;;
  esac
}

# One output-schema template per FORMAT, instantiated with the derived name
# and the RESOLVED version. The name and version are substituted in, never
# looked up.
output_schema_of_format() {
  local fmt="$1" name="$2" version="$3" e_name e_ver
  e_name="$(printf '%s' "$name" | sed -e 's/[.[\*^$+?(){}|\\]/\\&/g')"
  e_ver="$(printf '%s' "$version" | sed -e 's/[.[\*^$+?(){}|\\]/\\&/g')"
  case "$fmt" in
    bundler) printf '^%s \\(%s\\)$' "$e_name" "$e_ver" ;;
    *)       printf 'n/a' ;;
  esac
}

sha_of_file() { sha256sum -- "$1" | awk '{print $1}'; }
sha_of_line() { printf '%s\n' "$1" | sha256sum | awk '{print $1}'; }

# ---------------------------------------------------------------------------
# split_tabs <line>  ->  SPLIT[]
#
# EXACT split on TAB. `IFS=$'\t' read` cannot be used for hostile input: TAB is
# IFS *whitespace*, so bash collapses runs of it and strips it from both ends,
# which silently normalises `a<TAB><TAB>b` into two fields instead of three and
# would let a malformed row be read as a well-formed one. This splitter
# preserves empty fields and never trims.
#
# Every record this file emits internally is guaranteed by its producer to have
# NO empty field and NO embedded TAB (see the tab guard in scan_bundler and the
# field checks in scan_resolution), which is what makes the `IFS=$'\t' read`
# used on those internal record streams exact rather than merely convenient.
# ---------------------------------------------------------------------------
split_tabs() {
  local rest="$1"
  SPLIT=()
  while [[ "$rest" == *$'\t'* ]]; do
    SPLIT+=("${rest%%$'\t'*}")
    rest="${rest#*$'\t'}"
  done
  SPLIT+=("$rest")
}

# ---------------------------------------------------------------------------
# scan_bundler <file>
#
# The Bundler-format grammar parser. Emits TAB-separated records on stdout
# and NEVER returns non-zero, so no caller can lose an error to a pipeline,
# an errexit quirk, or a nested command substitution.
#
#   S <url>                         the manifest's source directive
#   R <version>                     the manifest's ruby directive
#   D <name> <constraints> <sha>    a dependency declaration
#   V <lineno> <kind>               a line outside the accepted grammar
#   C <lines> <accounted> <decls>   the closing count assertion
#
# Every branch of the loop increments exactly one counter, and the C record
# reports both the physical line count and the sum of all counters, so the
# caller can assert that no `continue` ever dropped a line on the floor
# (Regra dos Seis, item 3). Violation records carry a line NUMBER and a kind
# only — never the offending text — so hostile content cannot reach stdout,
# the lock, or the evidence.
#
# NOTE: the <constraints> field is recorded VERBATIM and is never
# interpreted, compared, or scored anywhere in this file. It exists so the
# lock shows a human what the manifest asked for; the version that matters is
# the one in the resolution artifact.
# ---------------------------------------------------------------------------
scan_bundler() {
  local file="$1"
  local re_source="^source[[:space:]]+'([^']+)'[[:space:]]*$"
  local re_ruby="^ruby[[:space:]]+'([^']+)'[[:space:]]*$"
  local re_block="^[[:space:]]*(group|platforms|platform)[[:space:]]+:[A-Za-z0-9_]+([[:space:]]*,[[:space:]]*:[A-Za-z0-9_]+)*[[:space:]]+do[[:space:]]*$"
  local re_end="^[[:space:]]*end[[:space:]]*$"
  local re_gem="^[[:space:]]*gem[[:space:]]+'([^']+)'((,[[:space:]]*'[^']+')*)(,[[:space:]]*(:platforms|:platform)[[:space:]]*=>[[:space:]]*(\[[^]]*\]|:[A-Za-z0-9_]+)|,[[:space:]]*(platforms|platform):[[:space:]]*(\[[^]]*\]|:[A-Za-z0-9_]+))?[[:space:]]*$"

  local lineno=0 depth=0
  local c_blank=0 c_comment=0 c_source=0 c_gem=0 c_open=0 c_close=0 c_ruby=0 c_viol=0
  # c_struct counts violations that are NOT a line-classification failure
  # (block nesting). It is deliberately excluded from the accounting sum, so
  # the sum stays a pure statement about "every physical line was classified
  # into exactly one bucket".
  local c_struct=0
  local line norm name vlist vers

  while IFS= read -r line || [[ -n "$line" ]]; do
    lineno=$((lineno + 1))
    # Quote-style normalisation only: Bundler accepts ' and " and this card
    # must not care which one a manifest uses. A gem name cannot contain a
    # quote, so this cannot merge two distinct names.
    norm="${line//\"/\'}"

    if [[ -z "${line//[[:space:]]/}" ]]; then
      c_blank=$((c_blank + 1)); continue
    fi
    if [[ "$line" =~ ^[[:space:]]*# ]]; then
      c_comment=$((c_comment + 1)); continue
    fi
    if [[ "$norm" =~ $re_source ]]; then
      c_source=$((c_source + 1))
      # TAB GUARD. A value carrying a TAB would shift the fields of the record
      # stream below and could be read as a DIFFERENT value entirely. Rejected
      # as a violation, never normalised away.
      if [[ "${BASH_REMATCH[1]}" == *$'\t'* ]]; then
        printf 'V\t%d\ttab-in-value\n' "$lineno"
      else
        printf 'S\t%s\n' "${BASH_REMATCH[1]}"
      fi
      continue
    fi
    if [[ "$norm" =~ $re_ruby ]]; then
      c_ruby=$((c_ruby + 1))
      if [[ "${BASH_REMATCH[1]}" == *$'\t'* ]]; then
        printf 'V\t%d\ttab-in-value\n' "$lineno"
      else
        printf 'R\t%s\n' "${BASH_REMATCH[1]}"
      fi
      continue
    fi
    if [[ "$norm" =~ $re_block ]]; then
      c_open=$((c_open + 1)); depth=$((depth + 1)); continue
    fi
    if [[ "$norm" =~ $re_end ]]; then
      c_close=$((c_close + 1)); depth=$((depth - 1))
      if (( depth < 0 )); then                          # ACCOUNTING
        c_struct=$((c_struct + 1))
        printf 'V\t%d\tunbalanced-block\n' "$lineno"
        depth=0
      fi
      continue
    fi
    if [[ "$norm" =~ $re_gem ]]; then
      name="${BASH_REMATCH[1]}"
      vlist="${BASH_REMATCH[2]}"
      vers='-'
      if [[ -n "$vlist" ]]; then
        vers="$(printf '%s' "$vlist" | tr ',' '\n' | sed -n "s/^[[:space:]]*'\\(.*\\)'[[:space:]]*$/\\1/p" | tr '\n' ',' | sed -e 's/,$//')"
        [[ -n "$vers" ]] || vers='-'
      fi
      c_gem=$((c_gem + 1))
      # TAB GUARD. `gem 'webrick<TAB>evil'` would otherwise be emitted as
      # `D<TAB>webrick<TAB>evil<TAB>...` and read back as the declaration of
      # `webrick` — laundering a differently named dependency into an existing
      # resolution record. Rejected, not trimmed.
      if [[ "$name" == *$'\t'* || "$vers" == *$'\t'* ]]; then
        printf 'V\t%d\ttab-in-value\n' "$lineno"
      else
        printf 'D\t%s\t%s\t%s\n' "$name" "$vers" "$(sha_of_line "$line")"
      fi
      continue
    fi
    # No production matched. Rejected, not skipped.
    c_viol=$((c_viol + 1))
    printf 'V\t%d\tgrammar\n' "$lineno"
  done < "$file"

  if (( depth != 0 )); then                             # ACCOUNTING
    c_struct=$((c_struct + 1))
    printf 'V\t%d\tunbalanced-block\n' "$lineno"
  fi
  printf 'C\t%d\t%d\t%d\n' "$lineno" \
    "$((c_blank + c_comment + c_source + c_gem + c_open + c_close + c_ruby + c_viol))" \
    "$c_gem"
}

# ---------------------------------------------------------------------------
# scan_resolution <file>
#
# The resolution artifact's grammar parser. Same discipline as scan_bundler:
# an ALLOWLIST of productions, every non-matching physical line is a recorded
# violation, never a skip, and the function never returns non-zero.
#
#   H <key> <value>                          an envelope line
#   G <manifest> <name> <version> <digest>   a resolved dependency
#   T <family> <version> <digest>            a resolved runtime
#   V <lineno> <kind>                        a line outside the grammar
#   C <lines> <accounted>                    the closing count assertion
#
# The resolution artifact is HAND-MAINTAINED and is never written by
# `--generate` or `--attest`. That is deliberate: it is the reference truth
# Property R is compared against, and a reference truth generated from the
# thing it verifies would prove nothing (Regra dos Seis, item 6).
# ---------------------------------------------------------------------------
scan_resolution() {
  local file="$1"
  local lineno=0
  local c_head=0 c_gem=0 c_rt=0 c_viol=0
  local line
  local -a SPLIT=()

  while IFS= read -r line || [[ -n "$line" ]]; do
    lineno=$((lineno + 1))
    case "$line" in
      "resolution-schema: "*|"digest-domain: "*|"end: "*)
        c_head=$((c_head + 1))
        printf 'H\t%s\t%s\n' "${line%%: *}" "${line#*: }"
        continue
        ;;
    esac
    # EXACT split: an empty field or an extra field is a violation, never a
    # value that bash quietly folded away.
    split_tabs "$line"
    if [[ "${SPLIT[0]}" == 'gem' && "${#SPLIT[@]}" == '5' ]] &&                 # BOUND
       [[ -n "${SPLIT[1]}" && -n "${SPLIT[2]}" && -n "${SPLIT[3]}" && -n "${SPLIT[4]}" ]]; then
      c_gem=$((c_gem + 1))
      printf 'G\t%s\t%s\t%s\t%s\n' "${SPLIT[1]}" "${SPLIT[2]}" "${SPLIT[3]}" "${SPLIT[4]}"
      continue
    fi
    if [[ "${SPLIT[0]}" == 'runtime' && "${#SPLIT[@]}" == '4' ]] &&             # BOUND
       [[ -n "${SPLIT[1]}" && -n "${SPLIT[2]}" && -n "${SPLIT[3]}" ]]; then
      c_rt=$((c_rt + 1))
      printf 'T\t%s\t%s\t%s\n' "${SPLIT[1]}" "${SPLIT[2]}" "${SPLIT[3]}"
      continue
    fi
    c_viol=$((c_viol + 1))
    printf 'V\t%d\tresolution-grammar\n' "$lineno"
  done < "$file"

  printf 'C\t%d\t%d\n' "$lineno" "$((c_head + c_gem + c_rt + c_viol))"
}

# ---------------------------------------------------------------------------
# load_resolution <file>
#
# Fills the caller-visible maps below from the resolution artifact, appending
# a violation for every defect it finds. It NEVER exits: both invocation
# routes (`--generate` and the AC-001 oracle) must see identical behaviour
# under error, so a defect is always data, never an exit status.
#
#   RES_V[<manifest>\t<name>]   exact resolved version
#   RES_D[<manifest>\t<name>]   artifact digest
#   RES_KEYS                    insertion-ordered key list
#   RTV[<family>] / RTD[<family>]  resolved runtime version / digest
#   RES_ENV[<key>]              envelope lines seen
#
# The caller must have declared those names. `add_violation` likewise belongs
# to the caller.
# ---------------------------------------------------------------------------
load_resolution() {
  local file="$1"
  local rec a b c d key
  local -A env_seen=() dig_owner=() dig_of=()

  while IFS=$'\t' read -r rec a b c d; do
    case "$rec" in
      H)
        if [[ -n "${env_seen[$a]:-}" ]]; then
          add_violation "$file" 0 'resolution-duplicate-envelope-line'
        fi
        env_seen[$a]=1
        RES_ENV[$a]="$b"
        ;;
      G)
        key="$a"$'\t'"$b"
        if [[ -n "${RES_V[$key]+x}" ]]; then
          add_violation "$file" 0 'resolution-duplicate-record'
          continue
        fi
        RES_V[$key]="$c"
        RES_D[$key]="$d"
        RES_KEYS+=("$key")
        if [[ ! "$c" =~ $RE_EXACT_VERSION ]]; then
          add_violation "$file" 0 'resolved-version-not-exact'
        fi
        if [[ ! "$d" =~ $RE_DIGEST ]]; then
          add_violation "$file" 0 'artifact-digest-malformed'
        fi
        # A degenerate null digest is a declared SENTINEL, not a claim of
        # completeness: it catches the "field present but empty of meaning"
        # form. The real gate on digests is the pair of set properties below.
        case "$d" in
          'sha256:0000000000000000000000000000000000000000000000000000000000000000') add_violation "$file" 0 'artifact-digest-null' ;;
          'sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff') add_violation "$file" 0 'artifact-digest-null' ;;
        esac
        # SET property 1: one artifact identity per (name, version).
        if [[ -n "${dig_of[$b/$c]:-}" && "${dig_of[$b/$c]}" != "$d" ]]; then
          add_violation "$file" 0 'artifact-digest-inconsistent'
        fi
        dig_of[$b/$c]="$d"
        # SET property 2: no digest may be shared by two different artifacts.
        if [[ -n "${dig_owner[$d]:-}" && "${dig_owner[$d]}" != "$b/$c" ]]; then
          add_violation "$file" 0 'artifact-digest-collision'
        fi
        dig_owner[$d]="$b/$c"
        ;;
      T)
        if [[ -n "${RTV[$a]+x}" ]]; then
          add_violation "$file" 0 'resolution-duplicate-runtime'
          continue
        fi
        RTV[$a]="$b"
        RTD[$a]="$c"
        if [[ ! "$b" =~ $RE_EXACT_VERSION ]]; then
          add_violation "$file" 0 'resolved-runtime-version-not-exact'
        fi
        if [[ ! "$c" =~ $RE_DIGEST ]]; then
          add_violation "$file" 0 'artifact-digest-malformed'
        fi
        if [[ -n "${dig_owner[$c]:-}" && "${dig_owner[$c]}" != "runtime:$a/$b" ]]; then
          add_violation "$file" 0 'artifact-digest-collision'
        fi
        dig_owner[$c]="runtime:$a/$b"
        ;;
      V) add_violation "$file" "$a" "$b" ;;
      C)
        if (( a != b )); then                            # ACCOUNTING
          add_violation "$file" 0 'resolution-line-accounting-mismatch'
        fi
        ;;
      *) add_violation "$file" 0 'resolution-parser-emitted-unknown-record' ;;
    esac
  done < <(scan_resolution "$file")

  # Envelope: allowlists of one. An unanticipated schema or digest domain
  # FAILS; it is never accepted and never ignored.
  [[ "${RES_ENV[resolution-schema]:-}" == "$RESOLUTION_SCHEMA" ]] ||
    add_violation "$file" 0 'resolution-schema-unknown'
  [[ "${RES_ENV[digest-domain]:-}" == "$RESOLUTION_DIGEST_DOMAIN" ]] ||
    add_violation "$file" 0 'resolution-digest-domain-unknown'
  [[ "${RES_ENV[end]:-}" == "$RESOLUTION_SCHEMA" ]] ||
    add_violation "$file" 0 'resolution-terminator-missing'
}

# ---------------------------------------------------------------------------
# generate_lock: total function of the real bytes of the card's read_paths and
# of the card's resolution artifact. Always completes, always emits a
# well-formed document terminated by `end: bootstrap-lock-v5`, and never reads
# the committed lock or the read-path attestation. Any problem it finds is
# EMITTED as a violation[] record rather than raised as an exit status, so the
# two invocation routes cannot diverge under error.
# ---------------------------------------------------------------------------
generate_lock() {
  local -a viol_path=() viol_line=() viol_kind=()
  local -a man_rel=() man_fmt=() man_rt=() man_src=() man_sha=() man_lines=() man_acct=() man_decls=()
  local -a doc_rel=() doc_sha=() doc_bytes=() doc_lines=()
  local -a rp_role=() rp_fmt=() rp_sha=() rp_bytes=() rp_lines=()
  local -A D_CONSTRAINT=() D_DSHA=() D_MSHA=() D_RT=() D_FMT=() D_SEEN=()
  local -a D_KEYS=()
  local -A RES_V=() RES_D=() RTV=() RTD=() RES_ENV=() RES_USED=() RT_USED=()
  local -a RES_KEYS=()
  local -A fam_seen=() fam_paths=()
  local -a fam_list=()

  add_violation() {
    viol_path+=("$1"); viol_line+=("$2"); viol_kind+=("$3")
  }

  # ---- the resolution artifact, loaded once ----
  local res_rel res_abs res_sha='absent' res_present=0
  res_rel="$(required_resolution_of_format bundler)"
  res_abs="$repo_root/$res_rel"
  if [[ -L "$res_abs" ]]; then
    add_violation "$res_rel" 0 'resolution-not-a-regular-file'
  elif [[ ! -f "$res_abs" ]]; then
    add_violation "$res_rel" 0 'resolution-file-missing'
  else
    res_present=1
    res_sha="$(sha_of_file "$res_abs")"
    load_resolution "$res_abs"
  fi

  local rel abs fmt parser runtime bytes lines sha key
  local rec a b c
  for rel in "${CANONICAL_READ_PATHS[@]}"; do
    abs="$repo_root/$rel"
    if [[ ! -f "$abs" || -L "$abs" ]]; then
      rp_role+=('missing'); rp_fmt+=('n/a'); rp_sha+=('n/a'); rp_bytes+=(0); rp_lines+=(0)
      add_violation "$rel" 0 'read-path-missing-or-not-regular'
      continue
    fi
    bytes="$(wc -c < "$abs" | tr -d ' ')"
    lines="$(awk 'END{print NR}' "$abs")"
    sha="$(sha_of_file "$abs")"
    fmt="$(manifest_format_of "$rel")"

    if [[ -z "$fmt" ]]; then
      # Derivation 2: opaque document, uninterpreted, sealed whole-file.
      rp_role+=('document'); rp_fmt+=('opaque'); rp_sha+=("$sha"); rp_bytes+=("$bytes"); rp_lines+=("$lines")
      doc_rel+=("$rel"); doc_sha+=("$sha"); doc_bytes+=("$bytes"); doc_lines+=("$lines")
      continue
    fi

    parser="$(parser_of_format "$fmt")"
    runtime="$(runtime_of_format "$fmt")"
    if [[ -z "$parser" ]]; then
      rp_role+=('manifest'); rp_fmt+=("$fmt"); rp_sha+=("$sha"); rp_bytes+=("$bytes"); rp_lines+=("$lines")
      add_violation "$rel" 0 'unsupported-manifest-format'
      continue
    fi
    if [[ -z "$(required_resolution_of_format "$fmt")" ]]; then
      add_violation "$rel" 0 'format-without-required-resolution-source'
    fi

    rp_role+=('manifest'); rp_fmt+=("$fmt"); rp_sha+=("$sha"); rp_bytes+=("$bytes"); rp_lines+=("$lines")
    if [[ -z "${fam_seen[$runtime]:-}" ]]; then
      fam_seen[$runtime]=1
      fam_list+=("$runtime")
      fam_paths[$runtime]="$rel"
    else
      fam_paths[$runtime]="${fam_paths[$runtime]};$rel"
    fi

    local m_src='' m_lines=0 m_acct=0 m_decls=0 src_count=0
    while IFS=$'\t' read -r rec a b c; do
      case "$rec" in
        S)
          src_count=$((src_count + 1))
          if [[ -z "$m_src" ]]; then
            m_src="$a"
          else
            add_violation "$rel" 0 'multiple-source-directives'
          fi
          ;;
        R) : ;;
        D)
          key="$rel"$'\t'"$a"
          if [[ -n "${D_SEEN[$key]:-}" ]]; then
            add_violation "$rel" 0 'duplicate-declaration'
            continue
          fi
          D_SEEN[$key]=1
          D_KEYS+=("$key")
          D_CONSTRAINT[$key]="$b"
          D_DSHA[$key]="sha256:$c"
          D_MSHA[$key]="sha256:$sha"
          D_RT[$key]="$runtime"
          D_FMT[$key]="$fmt"
          ;;
        V) add_violation "$rel" "$a" "$b" ;;
        C) m_lines="$a"; m_acct="$b"; m_decls="$c" ;;
        *) add_violation "$rel" 0 'parser-emitted-unknown-record' ;;
      esac
    done < <("$parser" "$abs")

    if (( m_lines != m_acct )); then                     # ACCOUNTING
      add_violation "$rel" 0 'line-accounting-mismatch'
    fi
    if (( m_lines != lines )); then                      # ACCOUNTING
      add_violation "$rel" 0 'parser-did-not-consume-whole-file'
    fi
    if (( m_decls == 0 )); then                          # NONEMPTY
      add_violation "$rel" 0 'manifest-declares-nothing'
    fi
    if (( src_count == 0 )); then                        # NONEMPTY
      add_violation "$rel" 0 'manifest-without-source-directive'
      m_src='none'
    fi
    if [[ "$m_src" != "$TRUSTED_GEM_REGISTRY" ]]; then
      add_violation "$rel" 0 'untrusted-registry'
    fi
    man_rel+=("$rel"); man_fmt+=("$fmt"); man_rt+=("$runtime"); man_src+=("$m_src")
    man_sha+=("$sha"); man_lines+=("$m_lines"); man_acct+=("$m_acct"); man_decls+=("$m_decls")
  done

  # ---- PROPERTY R, expressed as two SET inclusions -------------------------
  # Declarations -> resolution. Every declared (manifest, name) must have a
  # record; the record is MARKED as used so the reverse inclusion below is a
  # real set statement and not a count.
  local k
  for k in "${D_KEYS[@]+"${D_KEYS[@]}"}"; do
    if [[ -z "${RES_V[$k]+x}" ]]; then
      add_violation "${k%%$'\t'*}" 0 'declaration-without-resolution'
      continue
    fi
    RES_USED[$k]=1
  done
  # Resolution -> declarations. A record nothing declared is dead weight the
  # card refuses to carry.
  for k in "${RES_KEYS[@]+"${RES_KEYS[@]}"}"; do
    if [[ -z "${RES_USED[$k]:-}" ]]; then
      add_violation "$res_rel" 0 'resolution-without-declaration'
    fi
  done
  # Runtime families -> resolved runtimes, both inclusions, same discipline.
  local fam
  for fam in "${fam_list[@]+"${fam_list[@]}"}"; do
    if [[ -z "${RTV[$fam]+x}" ]]; then
      add_violation "$res_rel" 0 'runtime-unresolved'
      continue
    fi
    RT_USED[$fam]=1
  done
  for fam in "${!RTV[@]}"; do
    if [[ -z "${RT_USED[$fam]:-}" ]]; then
      add_violation "$res_rel" 0 'resolved-runtime-without-manifest'
    fi
  done
  # One resolved version per NAME across every manifest of the card. Derived
  # from the resolution, never from a constraint.
  local -A name_ver=() name_rt=()
  local nm mf
  for k in "${D_KEYS[@]+"${D_KEYS[@]}"}"; do
    # This `continue` cannot hide anything: a key with no RES_V entry ALREADY
    # produced `declaration-without-resolution` in the inclusion loop above, so
    # the run is already failing. Skipping it here only avoids a second, less
    # precise violation for the same defect.
    [[ -n "${RES_V[$k]+x}" ]] || continue
    nm="${k#*$'\t'}"
    if [[ -n "${name_ver[$nm]:-}" && "${name_ver[$nm]}" != "${RES_V[$k]}" ]]; then
      add_violation "${k%%$'\t'*}" 0 'resolved-version-conflict'
    fi
    name_ver[$nm]="${RES_V[$k]}"
    if [[ -n "${name_rt[$nm]:-}" && "${name_rt[$nm]}" != "${D_RT[$k]}" ]]; then
      add_violation "${k%%$'\t'*}" 0 'runtime-family-conflict'
    fi
    name_rt[$nm]="${D_RT[$k]}"
  done

  # ---- header ----
  echo 'schema: bootstrap-lock-v5'
  echo 'card: AUR-364'
  echo 'generated_without_network: true'
  echo 'derivation[1]: tool identity derived from machine-readable manifest grammars, keyed on (manifest, name)'
  echo 'derivation[2]: every non-manifest read_path sealed by whole-file sha256, never interpreted'
  echo 'derivation[3]: every resolved version and artifact digest read from the card resolution artifact; no dependency constraint is interpreted anywhere'
  printf 'trusted_registry: %s\n' "$TRUSTED_GEM_REGISTRY"
  printf 'resolution.path: %s\n' "$res_rel"
  printf 'resolution.present: %s\n' "$( [[ "$res_present" == '1' ]] && printf 'true' || printf 'false' )"
  printf 'resolution.sha256: %s\n' "$( [[ "$res_sha" == 'absent' ]] && printf 'absent' || printf 'sha256:%s' "$res_sha" )"
  printf 'resolution.schema: %s\n' "${RES_ENV[resolution-schema]:-absent}"
  printf 'resolution.digest_domain: %s\n' "${RES_ENV[digest-domain]:-absent}"
  # The card's spec document, sealed like every other artifact this card owns.
  # It is a declared path, so it is a read path (Lei 20): a byte change here
  # fails the accept until --generate is re-run deliberately.
  printf 'spec.path: %s\n' "$SPEC_REL"
  printf 'spec.sha256: sha256:%s\n' "$(sha_of_file "$spec_file")"
  # The card's fixture directory, sealed file by file from the LISTING. The
  # count is emitted alongside the entries so a truncated tail cannot look
  # like a shorter directory (section 7k asserts both against the listing it
  # derives independently).
  local fx_i fx_n fx_rel
  fx_n="${#CARD_FIXTURES[@]}"
  printf 'card_artifact.dir: %s\n' "$FIXTURE_DIR_REL"
  printf 'card_artifact.count: %s\n' "$fx_n"
  for (( fx_i = 0; fx_i < fx_n; fx_i++ )); do
    fx_rel="${CARD_FIXTURES[$fx_i]}"
    printf 'card_artifact[%d].path: %s\n' "$((fx_i + 1))" "$fx_rel"
    printf 'card_artifact[%d].bytes: %s\n' "$((fx_i + 1))" "$(wc -c < "$repo_root/$fx_rel" | tr -d ' ')"
    printf 'card_artifact[%d].sha256: sha256:%s\n' "$((fx_i + 1))" "$(sha_of_file "$repo_root/$fx_rel")"
  done

  local i n
  n="${#CANONICAL_READ_PATHS[@]}"
  for (( i = 0; i < n; i++ )); do
    printf 'read_path[%d].path: %s\n' "$((i + 1))" "${CANONICAL_READ_PATHS[$i]}"
    printf 'read_path[%d].role: %s\n' "$((i + 1))" "${rp_role[$i]}"
    printf 'read_path[%d].format: %s\n' "$((i + 1))" "${rp_fmt[$i]}"
    printf 'read_path[%d].bytes: %s\n' "$((i + 1))" "${rp_bytes[$i]}"
    printf 'read_path[%d].lines: %s\n' "$((i + 1))" "${rp_lines[$i]}"
    printf 'read_path[%d].sha256: sha256:%s\n' "$((i + 1))" "${rp_sha[$i]}"
  done

  n="${#man_rel[@]}"
  for (( i = 0; i < n; i++ )); do
    printf 'manifest[%d].path: %s\n' "$((i + 1))" "${man_rel[$i]}"
    printf 'manifest[%d].format: %s\n' "$((i + 1))" "${man_fmt[$i]}"
    printf 'manifest[%d].runtime: %s\n' "$((i + 1))" "${man_rt[$i]}"
    printf 'manifest[%d].source: %s\n' "$((i + 1))" "${man_src[$i]}"
    printf 'manifest[%d].sha256: sha256:%s\n' "$((i + 1))" "${man_sha[$i]}"
    printf 'manifest[%d].required_resolution: %s\n' "$((i + 1))" "$(required_resolution_of_format "${man_fmt[$i]}")"
    printf 'manifest[%d].lines_total: %s\n' "$((i + 1))" "${man_lines[$i]}"
    printf 'manifest[%d].lines_accounted: %s\n' "$((i + 1))" "${man_acct[$i]}"
    printf 'manifest[%d].declarations: %s\n' "$((i + 1))" "${man_decls[$i]}"
  done

  n="${#doc_rel[@]}"
  for (( i = 0; i < n; i++ )); do
    printf 'document[%d].path: %s\n' "$((i + 1))" "${doc_rel[$i]}"
    printf 'document[%d].sha256: sha256:%s\n' "$((i + 1))" "${doc_sha[$i]}"
    printf 'document[%d].bytes: %s\n' "$((i + 1))" "${doc_bytes[$i]}"
    printf 'document[%d].lines: %s\n' "$((i + 1))" "${doc_lines[$i]}"
    printf 'document[%d].interpreted: false\n' "$((i + 1))"
    printf 'document[%d].install_instructions: uninterpreted-sealed-by-whole-file-digest\n' "$((i + 1))"
  done

  # ---- resolved runtimes, C-sorted by family ----
  local sorted idx=0
  sorted=''
  if (( ${#RTV[@]} > 0 )); then                          # NONEMPTY
    sorted="$(printf '%s\n' "${!RTV[@]}" | sort)"
  fi
  if [[ -n "$sorted" ]]; then
    while IFS= read -r fam; do
      [[ -n "$fam" ]] || continue
      idx=$((idx + 1))
      printf 'runtime[%d].family: %s\n' "$idx" "$fam"
      printf 'runtime[%d].resolved_version: %s\n' "$idx" "${RTV[$fam]}"
      printf 'runtime[%d].artifact_digest: %s\n' "$idx" "${RTD[$fam]}"
      printf 'runtime[%d].manifests: %s\n' "$idx" "${fam_paths[$fam]:-none}"
    done <<< "$sorted"
  fi

  # ---- derived tool set, one entry per (name, manifest), C-sorted ----
  local tools_total=0 tkey
  idx=0
  sorted=''
  if (( ${#D_KEYS[@]} > 0 )); then                       # NONEMPTY
    sorted="$(
      for k in "${D_KEYS[@]}"; do
        printf '%s\t%s\n' "${k#*$'\t'}" "${k%%$'\t'*}"
      done | sort
    )"
  fi
  if [[ -n "$sorted" ]]; then
    while IFS=$'\t' read -r nm mf; do
      [[ -n "$nm" ]] || continue
      tkey="$mf"$'\t'"$nm"
      idx=$((idx + 1))
      tools_total=$((tools_total + 1))
      printf 'tool[%d].name: %s\n' "$idx" "$nm"
      printf 'tool[%d].declared_in: %s\n' "$idx" "$mf"
      printf 'tool[%d].runtime: %s\n' "$idx" "${D_RT[$tkey]}"
      printf 'tool[%d].manifest_format: %s\n' "$idx" "${D_FMT[$tkey]}"
      printf 'tool[%d].version_constraint: %s\n' "$idx" "${D_CONSTRAINT[$tkey]}"
      printf 'tool[%d].constraint_interpreted: false\n' "$idx"
      printf 'tool[%d].resolved_version: %s\n' "$idx" "${RES_V[$tkey]:-unresolved}"
      printf 'tool[%d].artifact_digest: %s\n' "$idx" "${RES_D[$tkey]:-unresolved}"
      printf 'tool[%d].runtime_version: %s\n' "$idx" "${RTV[${D_RT[$tkey]}]:-unresolved}"
      printf 'tool[%d].declaration_sha256: %s\n' "$idx" "${D_DSHA[$tkey]}"
      printf 'tool[%d].manifest_sha256: %s\n' "$idx" "${D_MSHA[$tkey]}"
      printf 'tool[%d].allowlisted_command: %s\n' "$idx" "$(allowlisted_command_of_format "${D_FMT[$tkey]}")"
      printf 'tool[%d].output_schema: %s\n' "$idx" "$(output_schema_of_format "${D_FMT[$tkey]}" "$nm" "${RES_V[$tkey]:-unresolved}")"
    done <<< "$sorted"
  fi

  # ---- counts, emitted as DATA ONLY --------------------------------------
  # Nothing in the AC-001 oracle gates on a comparison between two of these
  # numbers. They exist so a human reading the artifact can see its shape;
  # every coverage decision is made by the SEEN maps in the oracle.
  printf 'count.read_paths: %d\n' "${#CANONICAL_READ_PATHS[@]}"
  printf 'count.manifests: %d\n' "${#man_rel[@]}"
  printf 'count.documents: %d\n' "${#doc_rel[@]}"
  printf 'count.declarations: %d\n' "${#D_KEYS[@]}"
  printf 'count.resolution_records: %d\n' "${#RES_KEYS[@]}"
  printf 'count.tools: %d\n' "$tools_total"

  # ---- violations ----
  n="${#viol_path[@]}"
  if (( n == 0 )); then                                    # NONEMPTY
    echo 'violations: none'
  else
    for (( i = 0; i < n; i++ )); do
      printf 'violation[%d].path: %s\n' "$((i + 1))" "${viol_path[$i]}"
      printf 'violation[%d].line: %s\n' "$((i + 1))" "${viol_line[$i]}"
      printf 'violation[%d].kind: %s\n' "$((i + 1))" "${viol_kind[$i]}"
    done
  fi
  printf 'violations_total: %d\n' "$n"

  # ---- disclosed limits ----
  echo 'limits[1].scope: document read_paths are sealed by whole-file digest and are never interpreted'
  echo 'limits[1].consequence: this card proves no read_path byte changed without a deliberate reseal; it does not classify what a documentation command does'
  echo 'limits[2].scope: tool identity is derived only from read_paths whose basename is a registered dependency-manifest format'
  echo 'limits[2].consequence: a manifest format with no registered parser stops the accept instead of being guessed at or demoted to an opaque document'
  echo 'limits[3].scope: this repository ships no ecosystem-native resolved lockfile (no Gemfile.lock, no .aurumcode/Gemfile.lock) and neither is a read_path of this card'
  echo 'limits[3].consequence: the required resolution source is the card-scoped artifact named in resolution.path; its absence is a typed failure, and a future Gemfile.lock must be added to the card read_paths before it can be honoured'
  echo 'limits[4].scope: artifact digests come from the resolution artifact under the declared digest-domain and are NOT verified against rubygems.org, because the accept has no network'
  echo 'limits[4].consequence: this card proves every tool has exactly one well-formed digest, that one artifact identity maps to one digest, that no two artifacts share a digest, and that the value is byte-stable across runs; it does NOT prove the digest equals the upstream .gem checksum'
  echo 'limits[5].scope: dependency constraints are recorded verbatim and never interpreted'
  echo 'limits[5].consequence: a constraint that would admit many versions is neither blessed nor rejected by its syntax; what binds is the exact resolved version, so a version-less or open-bounded declaration fails unless the resolution names one exact release for that exact (manifest, name) pair'
  echo 'limits[6].scope: the resolved runtime version is checked for RESOLUTION CONSISTENCY only and is NEVER compared against any tool version; this card performs no compatibility judgement of any kind'
  echo 'limits[6].consequence: this card proves that every manifest format has its runtime family resolved to exactly one exact version, that every resolved runtime is used by some manifest, and that every tool[] entry cites that same family and that same version; it does NOT prove the resolved runtime satisfies a tool required_ruby_version, and it never could offline, because that metadata lives in the upstream gemspec and appears in no read_path of this card'
  echo 'limits[6].why_not_delegated: an ecosystem resolver is not a substitute either. Measured with bundler 2.4.19 under ruby 3.2.11 on 2026-08-06: a manifest declaring the runtime pin 2.4.0 together with a documentation dependency whose upstream gemspec requires runtime >= 2.5.0 produced a resolution with exit 0 and recorded the RUNNING interpreter 3.2.11p268, not the declared pin. The resolver enforces the upstream runtime requirement against the interpreter executing it (measured in the opposite direction: bundler 1.17.3 under ruby 2.4.10 refused the same dependency with exit 6), never against a declared runtime version, so the existence of a resolution is not evidence that a declared runtime pin is compatible with a resolved dependency. The names, versions and full command lines of both measurements are in section 6 of the spec sealed above, which is a document rather than executable code'
  echo 'limits[6].why_not_runnable: the one bundler subcommand that does refuse a declared/running runtime mismatch is not available to this accept either. Measured on 2026-08-06: bundle check and bundle install on that same Gemfile both exited 18 with "Your Ruby version is 3.2.11, but your Gemfile specified 2.4.0" - an equality test between the declared pin and the interpreter already executing, not a tool-versus-runtime judgement - and the sealed accept image bash@sha256:ae4668c2560999e65e89532cd2ad1b6688bb23298189f0bd229ef80fa4bd0831 has no ruby, no bundle and no gem at all (command -v for all three exited 127), so no resolver can run inside the accept and no lockfile it produced could be trusted as evidence without also trusting the network it needed'
  echo 'limits[6].error_code: the code emitted by the checks in this scope is runtime-resolution-inconsistent, named for what it verifies; it was called runtime-incompatible through round 4 and that name promised a comparison no branch of this card has ever performed'

  # ---- non-derivable engineering constants ----
  echo 'nonderivable[1].field: trusted_registry'
  echo 'nonderivable[1].justification: the single gem origin this card accepts is a trust decision, not a fact extracted from read_path bytes; it is an allowlist of one, so an unanticipated origin fails rather than passes'
  echo 'nonderivable[2].field: format registry (basename -> manifest format)'
  echo 'nonderivable[2].justification: which basenames name a machine-readable dependency manifest is ecosystem knowledge; every non-match falls through to the whole-file-digest path, so the registry can never create a blind spot'
  echo 'nonderivable[3].field: tool[*].allowlisted_command'
  echo 'nonderivable[3].justification: one template per manifest format, deliberately narrower than the raw source (no network install at accept time); instantiated identically for every tool, never looked up per name'
  echo 'nonderivable[4].field: tool[*].output_schema'
  echo 'nonderivable[4].justification: one regex template per manifest format with the derived name and the resolved version substituted in; not present in any read_path'
  echo 'nonderivable[5].field: resolution.path and resolution.digest_domain'
  echo 'nonderivable[5].justification: which artifact resolves a manifest format, and under which digest domain, are trust decisions; both are allowlists of one, so an unanticipated resolution source or digest domain fails rather than passes'
  echo 'end: bootstrap-lock-v5'
}

# ---------------------------------------------------------------------------
# Read-path integrity, shared by every mode. Symlink behaviour is DECLARED:
# this card never enumerates a directory, so it never relies on `find -type f`
# skipping symlinks; each declared path is tested with -L BEFORE -f and a
# symlink is a hard source_integrity_error, whether it points inside or
# outside the repository.
# ---------------------------------------------------------------------------
assert_read_paths() {
  local rel abs raw stripped
  for rel in "${CANONICAL_READ_PATHS[@]}"; do
    abs="$repo_root/$rel"
    [[ -e "$abs" || -L "$abs" ]] || fail source_integrity_error
    [[ ! -L "$abs" ]] || fail source_integrity_error
    [[ -f "$abs" ]] || fail source_integrity_error
    raw="$(wc -c < "$abs" | tr -d ' ')"
    (( raw > 0 )) || fail source_integrity_error          # NONEMPTY
    (( raw <= MAX_READ_PATH_BYTES )) || fail input_limit_exceeded   # BOUND
    # Lei 15: BusyBox awk/grep and bash `read` all lose NUL silently, so NUL
    # is detected by byte accounting alone.
    stripped="$(tr -d '\000' < "$abs" | wc -c | tr -d ' ')"
    (( raw == stripped )) || fail source_integrity_error   # BOUND
  done
}

# ---------------------------------------------------------------------------
# The card's spec document, structurally. Shared by --generate (which seals it
# into the lock) and by the oracle (which also reads its content). Same
# discipline as every other artifact: symlink and NUL are hard failures, and
# absence is typed, never an omission.
# ---------------------------------------------------------------------------
assert_spec_file() {
  local raw stripped lines
  [[ -e "$spec_file" || -L "$spec_file" ]] || fail docs_tool_lock_incomplete
  [[ ! -L "$spec_file" ]] || fail source_integrity_error
  [[ -f "$spec_file" ]] || fail docs_tool_lock_incomplete
  raw="$(wc -c < "$spec_file" | tr -d ' ')"
  (( raw > 0 )) || fail docs_tool_lock_incomplete            # NONEMPTY
  (( raw <= MAX_SPEC_BYTES )) || fail input_limit_exceeded   # BOUND
  lines="$(awk 'END{print NR}' "$spec_file")"
  (( lines <= MAX_SPEC_LINES )) || fail input_limit_exceeded # BOUND
  stripped="$(tr -d '\000' < "$spec_file" | wc -c | tr -d ' ')"
  (( raw == stripped )) || fail source_integrity_error       # BOUND
  [[ -e "$self_file" || -L "$self_file" ]] || fail docs_tool_lock_incomplete
  [[ ! -L "$self_file" ]] || fail source_integrity_error
  [[ -f "$self_file" ]] || fail docs_tool_lock_incomplete
}

# ---------------------------------------------------------------------------
# The card's declared FIXTURE DIRECTORY, listed rather than enumerated.
#
# Lei 20: `tests/bootstrap/locks/AUR-364` is a declared path, so every file
# under it must be a file this accept opens. The listing is the derivation —
# no basename is typed here, so a fixture added, renamed or deleted changes
# the derived set and therefore changes the lock. Symlink, NUL byte, empty
# file and over-limit are all typed failures BEFORE any digest is taken, and
# the derived set being empty is a failure, never a vacuous agreement.
#
# Glob order under LC_ALL=C (set at the top of this file) is byte order, so
# the derived list is deterministic across runs and across filesystems.
# ---------------------------------------------------------------------------
assert_card_fixtures() {
  local entry raw stripped
  local -a entries=()
  CARD_FIXTURES=()
  [[ -e "$fixture_dir" || -L "$fixture_dir" ]] || fail docs_tool_lock_incomplete
  [[ ! -L "$fixture_dir" ]] || fail source_integrity_error
  [[ -d "$fixture_dir" ]] || fail docs_tool_lock_incomplete

  local had_nullglob=0 had_dotglob=0
  shopt -q nullglob && had_nullglob=1
  shopt -q dotglob && had_dotglob=1
  shopt -s nullglob dotglob
  entries=( "$fixture_dir"/* )
  (( had_nullglob == 1 )) || shopt -u nullglob                   # BOUND
  (( had_dotglob == 1 )) || shopt -u dotglob                     # BOUND

  (( ${#entries[@]} > 0 )) || fail docs_tool_lock_incomplete     # NONEMPTY
  (( ${#entries[@]} <= MAX_FIXTURE_ENTRIES )) || fail input_limit_exceeded  # BOUND

  for entry in "${entries[@]}"; do
    [[ ! -L "$entry" ]] || fail source_integrity_error
    [[ -f "$entry" ]] || fail source_integrity_error
    raw="$(wc -c < "$entry" | tr -d ' ')"
    (( raw > 0 )) || fail docs_tool_lock_incomplete              # NONEMPTY
    (( raw <= MAX_FIXTURE_BYTES )) || fail input_limit_exceeded  # BOUND
    stripped="$(tr -d '\000' < "$entry" | wc -c | tr -d ' ')"
    (( raw == stripped )) || fail source_integrity_error         # BOUND
    CARD_FIXTURES+=("${entry#"$repo_root"/}")
  done
}

# ---------------------------------------------------------------------------
# Modes
# ---------------------------------------------------------------------------
case "${1:-AC-001}" in
  --generate)
    assert_read_paths
    assert_spec_file
    assert_card_fixtures
    generate_lock
    exit 0
    ;;
  --attest)
    # Deliberate, separate reseal of the card's read_paths. Never invoked by
    # --generate and never invoked by the oracle: blessing a read_path change
    # is a second, explicit, reviewable action. It does NOT touch the
    # resolution artifact either, so blessing a NEW DEPENDENCY still requires
    # a third, hand-authored action.
    assert_read_paths
    echo 'attest-schema: read-paths-v1'
    for rel in "${CANONICAL_READ_PATHS[@]}"; do
      printf '%s\tsha256:%s\n' "$rel" "$(sha_of_file "$repo_root/$rel")"
    done
    exit 0
    ;;
esac

selector="${1:-AC-001}"
case "$selector" in
  AC-001|ContractAUR364|IntegrationAUR364|E2EAUR364) ;;
  *) printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64 ;;
esac

# --- 1. read_paths ---------------------------------------------------------
assert_read_paths

# --- 2. the committed lock: presence, shape, bounds ------------------------
[[ -e "$lock_file" || -L "$lock_file" ]] || fail docs_tool_lock_incomplete
[[ ! -L "$lock_file" ]] || fail source_integrity_error
[[ -f "$lock_file" ]] || fail docs_tool_lock_incomplete
lock_size="$(wc -c < "$lock_file" | tr -d ' ')"
(( lock_size > 0 )) || fail invalid_lock_or_manifest                  # NONEMPTY
(( lock_size <= MAX_LOCK_BYTES )) || fail input_limit_exceeded        # BOUND
lock_lines="$(awk 'END{print NR}' "$lock_file")"
(( lock_lines <= MAX_LOCK_LINES )) || fail input_limit_exceeded       # BOUND
lock_stripped="$(tr -d '\000' < "$lock_file" | wc -c | tr -d ' ')"
(( lock_size == lock_stripped )) || fail source_integrity_error       # BOUND

grep -Fqx 'schema: bootstrap-lock-v5' "$lock_file" || fail invalid_lock_or_manifest
grep -Fqx 'card: AUR-364' "$lock_file" || fail invalid_lock_or_manifest
grep -Fqx 'end: bootstrap-lock-v5' "$lock_file" || fail invalid_lock_or_manifest
# NO PIPELINE HERE, ON PURPOSE. `set -o pipefail` plus a right-hand side that
# exits as soon as it has an answer (`grep -q`, `head`) is a SIGPIPE race: the
# left-hand side dies with 141, the pipeline reports 141, and the `if` reads
# that as "no match" — the check silently DOES NOT HAPPEN. That is the exact
# class this card is under indictment for, so every decision in this file is
# taken from a SINGLE command's own exit status or from a here-string, never
# from the status of a pipeline whose head can be killed.
if grep -vqE '^[A-Za-z0-9_]+(\[[0-9]+\])?(\.[A-Za-z0-9_.]+)?: .*$' "$lock_file"; then
  fail invalid_lock_or_manifest
fi

# --- 3. the card's read-path attestation: presence, shape, bounds ----------
[[ -e "$attest_file" || -L "$attest_file" ]] || fail docs_tool_lock_incomplete
[[ ! -L "$attest_file" ]] || fail source_integrity_error
[[ -f "$attest_file" ]] || fail docs_tool_lock_incomplete
attest_size="$(wc -c < "$attest_file" | tr -d ' ')"
(( attest_size > 0 )) || fail docs_tool_lock_incomplete               # NONEMPTY
(( attest_size <= MAX_ATTEST_BYTES )) || fail input_limit_exceeded    # BOUND
attest_stripped="$(tr -d '\000' < "$attest_file" | wc -c | tr -d ' ')"
(( attest_size == attest_stripped )) || fail source_integrity_error   # BOUND
grep -Fqx 'attest-schema: read-paths-v1' "$attest_file" || fail invalid_lock_or_manifest

# ===========================================================================
# SECTION 3b — THE ERROR-CODE CONTRACT, AS A SET.
#
# `docs/specs/AUR-364.md` is a declared path of this card. Until round 5 no
# branch of this file opened it: deleting the spec from the tree left the
# accept green while the card went on asserting, in writing, that the spec
# documents the schema, the bounds and the codes. That is Lei 20 and it is
# Lei 12 in its easiest form to miss — the file is there, the gate is green,
# and the check simply never happens.
#
# So the spec is read, and the property checked is the one that actually
# binds: THE SET OF CODES THIS SCRIPT CAN EMIT EQUALS THE SET OF CODES THE
# SPEC DOCUMENTS. Both sides are DERIVED, neither is typed:
#
#   * the emitted side comes from this script's OWN BYTES — every `fail`
#     call site plus the one `/AC-001/` printf form. Add a code in code and
#     forget the spec, and this fails.
#   * the documented side comes from the spec's section 5 table. Document a
#     code that no branch can reach, and this fails too.
#
# Compared as SETS, both inclusions, each code marked SEEN by name — never by
# cardinality, which was blocker B4. Either side empty is a failure, not a
# vacuous agreement (Lei 12 item 4).
#
# This is the gate that makes round 5's rename mechanical rather than
# cosmetic: the code that fires when the resolution's runtime records are
# internally inconsistent is now called `runtime-resolution-inconsistent`,
# and the spec's table and limits[6] say in the same breath that this card
# performs NO tool-versus-runtime compatibility judgement. Renaming one and
# not the other stops the accept here.
# ===========================================================================
assert_spec_file

declare -A CODE_EMITTED=() CODE_DOCUMENTED=()
# The emitted side is read from this script's own bytes, line by line, with
# comment lines skipped (a comment is not a call site) and EVERY `fail <code>`
# on a line consumed, not just the first.
#
# Round 5 defect, found by running the mutation that is supposed to prove this
# gate: the previous derivation required `|| `, `; ` or start-of-line
# immediately before `fail`, so a call site written `...; then fail x; fi` was
# INVISIBLE to it and `script_fail_code_undocumented` passed the accept while
# claiming to fail it. The prefix is now "any character that cannot be part of
# an identifier", which is a property of the shell's own tokenisation rather
# than a list of the three spellings someone happened to think of.
while IFS= read -r c_line; do
  [[ ! "$c_line" =~ ^[[:space:]]*# ]] || continue
  while [[ "$c_line" =~ (^|[^a-zA-Z0-9_-])fail\ ([a-z0-9_][a-z0-9_-]*) ]]; do
    CODE_EMITTED["${BASH_REMATCH[2]}"]=1
    c_line="${c_line#*fail }"
  done
done < "$self_file"
while IFS= read -r c_line; do
  [[ -n "$c_line" ]] || continue
  CODE_EMITTED["${c_line#/AC-001/}"]=1
done < <(grep -oE '/AC-001/[a-z0-9_][a-z0-9_-]*' "$self_file" || true)
(( ${#CODE_EMITTED[@]} > 0 )) || fail invalid_lock_or_manifest        # NONEMPTY

while IFS= read -r c_line; do
  [[ -n "$c_line" ]] || continue
  CODE_DOCUMENTED[$c_line]=1
done < <(awk '
  /^## 5\./                    { in5 = 1; next }
  /^## /                       { in5 = 0 }
  in5 && /^\| `[a-z0-9_-]+` \|/ {
    s = $0
    sub(/^\| `/, "", s)
    sub(/`.*$/, "", s)
    print s
  }
' "$spec_file" || true)
(( ${#CODE_DOCUMENTED[@]} > 0 )) || fail docs-tool-unlocked           # NONEMPTY

for c_code in "${!CODE_EMITTED[@]}"; do
  [[ -n "${CODE_DOCUMENTED[$c_code]:-}" ]] || fail docs-tool-unlocked
done
for c_code in "${!CODE_DOCUMENTED[@]}"; do
  [[ -n "${CODE_EMITTED[$c_code]:-}" ]] || fail docs-tool-unlocked
done

# --- 4. the REQUIRED RESOLUTION SOURCE: presence, shape, bounds -----------
# Its absence is a typed failure, never an omission. There is no branch of
# this oracle that reaches the pass statement without having read it.
resolution_rel="$(required_resolution_of_format bundler)"
[[ -n "$resolution_rel" ]] || fail docs_tool_lock_incomplete
resolution_file="$repo_root/$resolution_rel"
[[ -e "$resolution_file" || -L "$resolution_file" ]] || fail docs_tool_lock_incomplete
[[ ! -L "$resolution_file" ]] || fail source_integrity_error
[[ -f "$resolution_file" ]] || fail docs_tool_lock_incomplete
resolution_size="$(wc -c < "$resolution_file" | tr -d ' ')"
(( resolution_size > 0 )) || fail docs_tool_lock_incomplete                    # NONEMPTY
(( resolution_size <= MAX_RESOLUTION_BYTES )) || fail input_limit_exceeded     # BOUND
resolution_lines="$(awk 'END{print NR}' "$resolution_file")"
(( resolution_lines <= MAX_RESOLUTION_LINES )) || fail input_limit_exceeded    # BOUND
resolution_stripped="$(tr -d '\000' < "$resolution_file" | wc -c | tr -d ' ')"
(( resolution_size == resolution_stripped )) || fail source_integrity_error    # BOUND
grep -Fqx "resolution-schema: $RESOLUTION_SCHEMA" "$resolution_file" || fail invalid_lock_or_manifest
grep -Fqx "digest-domain: $RESOLUTION_DIGEST_DOMAIN" "$resolution_file" || fail invalid_lock_or_manifest
grep -Fqx "end: $RESOLUTION_SCHEMA" "$resolution_file" || fail invalid_lock_or_manifest

# --- 4b. the card's declared FIXTURE DIRECTORY (Lei 20) --------------------
# Structural first, typed, before any digest is taken. The listing it produces
# is what section 7k compares against the committed lock.
assert_card_fixtures

# --- 5. derive once --------------------------------------------------------
generated_text="$(generate_lock)"
# A truncated generator run is detected here rather than trusted: the
# derivation is only usable if it reached its own terminator.
grep -Fqx 'end: bootstrap-lock-v5' <<< "$generated_text" || fail invalid_lock_or_manifest
grep -Fqx 'schema: bootstrap-lock-v5' <<< "$generated_text" || fail invalid_lock_or_manifest

# --- 6. violations found while deriving ------------------------------------
# The mapping below only chooses the NAME of the failure. Its final clause is
# unconditional, so an unmapped violation kind still fails: there is no branch
# here that lets a violation through.
# Every grep below reads a HERE-STRING, so each `if` tests that one grep's own
# exit status. Written as `printf ... | grep -q` these would have been SIGPIPE
# races in which a violation that IS present reports "absent" and the accept
# falls through to a milder code — or, for the terminator checks above, fails a
# nominal run at random. Both were observed at ~2-4% of runs before this change.
violations_total="$(grep -F 'violations_total: ' <<< "$generated_text" || true)"
violations_total="${violations_total#violations_total: }"
[[ -n "$violations_total" ]] || fail invalid_lock_or_manifest
if [[ "$violations_total" != '0' ]]; then
  if grep -Fq '.kind: read-path-missing-or-not-regular' <<< "$generated_text"; then
    fail source_integrity_error
  fi
  if grep -Fq '.kind: untrusted-registry' <<< "$generated_text"; then
    fail docs-tool-source-untrusted
  fi
  if grep -Eq '\.kind: (resolved-version-conflict|runtime-family-conflict|runtime-unresolved|resolved-runtime-version-not-exact)$' <<< "$generated_text"; then
    fail runtime-resolution-inconsistent
  fi
  if grep -Eq '\.kind: (resolved-version-not-exact|artifact-digest-malformed|artifact-digest-null|artifact-digest-collision|artifact-digest-inconsistent)$' <<< "$generated_text"; then
    fail docs-tool-mutable
  fi
  if grep -Eq '\.kind: (declaration-without-resolution|resolution-file-missing|resolution-not-a-regular-file|manifest-declares-nothing|format-without-required-resolution-source)$' <<< "$generated_text"; then
    fail docs_tool_lock_incomplete
  fi
  fail invalid_lock_or_manifest
fi

# ===========================================================================
# SECTION 7 — THE SET GATES.
#
# From here to the end, every coverage decision is made by marking canonical
# items SEEN and then asserting that each one was seen BY NAME. This section
# reads the RAW ARTIFACTS (the manifests, the resolution artifact, the
# committed lock, the real bytes on disk) — not the generator's counters — so
# that "the generator's numbers agree with each other" can never stand in for
# "the sets are equal". That substitution was blocker B3.
# ===========================================================================

# --- 7a. every canonical read_path appears exactly once in the lock, and
#         every read_path line in the lock names a canonical path -----------
declare -A RP_SEEN=() RP_ROLE=()
while IFS= read -r line; do
  [[ -n "$line" ]] || continue
  rp_idx="$(printf '%s' "$line" | sed -e 's/^read_path\[\([0-9]*\)\]\.path: .*$/\1/')"
  rp_path="$(printf '%s' "$line" | sed -e 's/^read_path\[[0-9]*\]\.path: //')"
  [[ -n "${RP_SEEN[$rp_path]:-}" ]] && fail invalid_lock_or_manifest   # duplicate row
  RP_SEEN[$rp_path]="$rp_idx"
done < <(grep -E '^read_path\[[0-9]+\]\.path: ' "$lock_file" || true)
for rel in "${CANONICAL_READ_PATHS[@]}"; do
  [[ -n "${RP_SEEN[$rel]:-}" ]] || fail invalid_lock_or_manifest
done
for rp_path in "${!RP_SEEN[@]}"; do
  rp_known='no'
  for rel in "${CANONICAL_READ_PATHS[@]}"; do
    [[ "$rel" != "$rp_path" ]] || rp_known='yes'
  done
  [[ "$rp_known" == 'yes' ]] || fail invalid_lock_or_manifest
done

# --- 7b. the role of each canonical read_path is DERIVED here, by the same
#         total function the generator uses, and the committed lock is then
#         required to AGREE with the derivation ---------------------------
# The derivation drives the rest of section 7; the lock is a claim compared
# against it. Nothing below is steered by a field the lock supplies, so a
# forged `role:` row cannot silently remove a manifest from the parse set.
declare -A DERIVED_ROLE=()
have_manifest='no'
have_document='no'
for rel in "${CANONICAL_READ_PATHS[@]}"; do
  if [[ -n "$(manifest_format_of "$rel")" ]]; then
    DERIVED_ROLE[$rel]='manifest'
    have_manifest='yes'
  else
    DERIVED_ROLE[$rel]='document'
    have_document='yes'
  fi
done
[[ "$have_manifest" == 'yes' ]] || fail docs_tool_lock_incomplete   # NONEMPTY
[[ "$have_document" == 'yes' ]] || fail docs_tool_lock_incomplete   # NONEMPTY

while IFS= read -r line; do
  [[ -n "$line" ]] || continue
  rp_idx="$(printf '%s' "$line" | sed -e 's/^read_path\[\([0-9]*\)\]\.role: .*$/\1/')"
  rp_val="$(printf '%s' "$line" | sed -e 's/^read_path\[[0-9]*\]\.role: //')"
  for rel in "${!RP_SEEN[@]}"; do
    if [[ "${RP_SEEN[$rel]}" == "$rp_idx" ]]; then
      [[ -z "${RP_ROLE[$rel]:-}" ]] || fail invalid_lock_or_manifest    # duplicate role row
      RP_ROLE[$rel]="$rp_val"
    fi
  done
done < <(grep -E '^read_path\[[0-9]+\]\.role: ' "$lock_file" || true)
for rel in "${CANONICAL_READ_PATHS[@]}"; do
  [[ "${RP_ROLE[$rel]:-}" == "${DERIVED_ROLE[$rel]}" ]] || fail invalid_lock_or_manifest
done

# --- 7c. the resolution artifact, re-parsed here from its own bytes -------
declare -A RES_VERSION=() RES_DIGEST=() RES_MATCHED=() RT_VERSION=() RT_DIGEST=() RT_MATCHED=()
declare -a RES_KEYLIST=()
declare -A DIG_OWNER=() DIG_OF=()
while IFS=$'\t' read -r r_kind r_a r_b r_c r_d; do
  case "$r_kind" in
    G)
      res_key="$r_a"$'\t'"$r_b"
      [[ -z "${RES_VERSION[$res_key]+x}" ]] || fail invalid_lock_or_manifest   # duplicate record
      [[ "$r_c" =~ $RE_EXACT_VERSION ]] || fail docs-tool-mutable
      [[ "$r_d" =~ $RE_DIGEST ]] || fail docs-tool-mutable
      case "$r_d" in
        'sha256:0000000000000000000000000000000000000000000000000000000000000000') fail docs-tool-mutable ;;
        'sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff') fail docs-tool-mutable ;;
      esac
      if [[ -n "${DIG_OF[$r_b/$r_c]:-}" && "${DIG_OF[$r_b/$r_c]}" != "$r_d" ]]; then
        fail docs-tool-mutable
      fi
      DIG_OF[$r_b/$r_c]="$r_d"
      if [[ -n "${DIG_OWNER[$r_d]:-}" && "${DIG_OWNER[$r_d]}" != "$r_b/$r_c" ]]; then
        fail docs-tool-mutable
      fi
      DIG_OWNER[$r_d]="$r_b/$r_c"
      RES_VERSION[$res_key]="$r_c"
      RES_DIGEST[$res_key]="$r_d"
      RES_KEYLIST+=("$res_key")
      ;;
    T)
      [[ -z "${RT_VERSION[$r_a]+x}" ]] || fail invalid_lock_or_manifest
      [[ "$r_b" =~ $RE_EXACT_VERSION ]] || fail runtime-resolution-inconsistent
      [[ "$r_c" =~ $RE_DIGEST ]] || fail docs-tool-mutable
      if [[ -n "${DIG_OWNER[$r_c]:-}" && "${DIG_OWNER[$r_c]}" != "runtime:$r_a/$r_b" ]]; then
        fail docs-tool-mutable
      fi
      DIG_OWNER[$r_c]="runtime:$r_a/$r_b"
      RT_VERSION[$r_a]="$r_b"
      RT_DIGEST[$r_a]="$r_c"
      ;;
    V) fail invalid_lock_or_manifest ;;
    H|C) : ;;
    *) fail invalid_lock_or_manifest ;;
  esac
done < <(scan_resolution "$resolution_file")
(( ${#RES_KEYLIST[@]} > 0 )) || fail docs_tool_lock_incomplete   # NONEMPTY
(( ${#RT_VERSION[@]} > 0 )) || fail docs_tool_lock_incomplete    # NONEMPTY

# --- 7d. the declarations, re-parsed here from the manifests' own bytes ---
declare -A DECL_CONSTRAINT=() DECL_RUNTIME=() DECL_SEEN=()
declare -a DECL_KEYLIST=()
declare -A FAMILY_SEEN=() CLASSIFIED=()
for rel in "${CANONICAL_READ_PATHS[@]}"; do
  # No `continue` anywhere in this loop: every canonical path lands in exactly
  # one arm and is marked CLASSIFIED, and the assertion after the loop demands
  # that all of them were (Regra dos Seis, item 3 and item 5).
  d_fmt="$(manifest_format_of "$rel")"
  if [[ "${DERIVED_ROLE[$rel]}" == 'manifest' ]]; then
    [[ -n "$d_fmt" ]] || fail invalid_lock_or_manifest
    d_parser="$(parser_of_format "$d_fmt")"
    [[ -n "$d_parser" ]] || fail invalid_lock_or_manifest
    d_runtime="$(runtime_of_format "$d_fmt")"
    [[ -n "$d_runtime" ]] || fail invalid_lock_or_manifest
    [[ -n "$(required_resolution_of_format "$d_fmt")" ]] || fail docs_tool_lock_incomplete
    FAMILY_SEEN[$d_runtime]=1
    d_saw_decl='no'
    while IFS=$'\t' read -r d_kind d_a d_b d_c; do
      case "$d_kind" in
        D)
          decl_key="$rel"$'\t'"$d_a"
          [[ -z "${DECL_SEEN[$decl_key]:-}" ]] || fail invalid_lock_or_manifest
          DECL_SEEN[$decl_key]=1
          DECL_KEYLIST+=("$decl_key")
          DECL_CONSTRAINT[$decl_key]="$d_b"
          DECL_RUNTIME[$decl_key]="$d_runtime"
          d_saw_decl='yes'
          ;;
        V) fail invalid_lock_or_manifest ;;
        S|R|C) : ;;
        *) fail invalid_lock_or_manifest ;;
      esac
    done < <("$d_parser" "$repo_root/$rel")
    [[ "$d_saw_decl" == 'yes' ]] || fail docs_tool_lock_incomplete   # NONEMPTY
    CLASSIFIED[$rel]='manifest'
  else
    [[ -z "$d_fmt" ]] || fail invalid_lock_or_manifest
    # A document is never interpreted; it is sealed by digest in 7g and 7h,
    # both of which assert this same path BY NAME.
    CLASSIFIED[$rel]='document'
  fi
done
for rel in "${CANONICAL_READ_PATHS[@]}"; do
  [[ "${CLASSIFIED[$rel]:-}" == "${DERIVED_ROLE[$rel]}" ]] || fail invalid_lock_or_manifest
done
[[ "${#DECL_KEYLIST[@]}" != '0' ]] || fail docs_tool_lock_incomplete   # NONEMPTY

# --- 7e. PROPERTY R, both inclusions, by NAME ------------------------------
# Declarations -> resolution. This is the gate that makes `gem "x"`,
# `gem "x", "0"` and `gem "x", ">= 0"` behave identically: none of them is
# read as a pin, and all three fail here unless the resolution names one
# exact release for that exact (manifest, name) pair.
for decl_key in "${DECL_KEYLIST[@]}"; do
  [[ -n "${RES_VERSION[$decl_key]+x}" ]] || fail docs_tool_lock_incomplete
  RES_MATCHED[$decl_key]=1
done
# Resolution -> declarations.
for res_key in "${RES_KEYLIST[@]}"; do
  [[ -n "${RES_MATCHED[$res_key]:-}" ]] || fail invalid_lock_or_manifest
done
# Runtime families -> resolved runtimes, both inclusions.
for family in "${!FAMILY_SEEN[@]}"; do
  [[ -n "${RT_VERSION[$family]+x}" ]] || fail runtime-resolution-inconsistent
  RT_MATCHED[$family]=1
done
for family in "${!RT_VERSION[@]}"; do
  [[ -n "${RT_MATCHED[$family]:-}" ]] || fail invalid_lock_or_manifest
done
# One resolved version and one runtime family per NAME across the card.
declare -A NAME_VERSION=() NAME_RUNTIME=()
for decl_key in "${DECL_KEYLIST[@]}"; do
  decl_name="${decl_key#*$'\t'}"
  if [[ -n "${NAME_VERSION[$decl_name]:-}" ]]; then
    [[ "${NAME_VERSION[$decl_name]}" == "${RES_VERSION[$decl_key]}" ]] || fail runtime-resolution-inconsistent
    [[ "${NAME_RUNTIME[$decl_name]}" == "${DECL_RUNTIME[$decl_key]}" ]] || fail runtime-resolution-inconsistent
  fi
  NAME_VERSION[$decl_name]="${RES_VERSION[$decl_key]}"
  NAME_RUNTIME[$decl_name]="${DECL_RUNTIME[$decl_key]}"
done

# --- 7f. the committed lock's tool blocks, as a SET keyed on (manifest,name)
declare -A LOCK_FIELD=() LOCK_KEY_OF_IDX=() LOCK_IDX_SEEN=() LOCK_KEY_SEEN=()
declare -a LOCK_IDXLIST=()
while IFS= read -r line; do
  [[ -n "$line" ]] || continue
  t_idx="$(printf '%s' "$line" | sed -e 's/^tool\[\([0-9]*\)\]\..*$/\1/')"
  t_field="$(printf '%s' "$line" | sed -e 's/^tool\[[0-9]*\]\.\([A-Za-z0-9_]*\): .*$/\1/')"
  t_value="$(printf '%s' "$line" | sed -e 's/^tool\[[0-9]*\]\.[A-Za-z0-9_]*: //')"
  [[ -z "${LOCK_FIELD[$t_idx/$t_field]+x}" ]] || fail invalid_lock_or_manifest   # duplicate field row
  LOCK_FIELD[$t_idx/$t_field]="$t_value"
  if [[ -z "${LOCK_IDX_SEEN[$t_idx]:-}" ]]; then
    LOCK_IDX_SEEN[$t_idx]=1
    LOCK_IDXLIST+=("$t_idx")
  fi
done < <(grep -E '^tool\[[0-9]+\]\.[A-Za-z0-9_]+: ' "$lock_file" || true)
(( ${#LOCK_IDXLIST[@]} > 0 )) || fail docs_tool_lock_incomplete   # NONEMPTY

readonly -a REQUIRED_TOOL_FIELDS=(
  name declared_in runtime manifest_format version_constraint
  constraint_interpreted resolved_version artifact_digest runtime_version
  declaration_sha256 manifest_sha256 allowlisted_command output_schema
)
for t_idx in "${LOCK_IDXLIST[@]}"; do
  # Every required field of every entry must be present BY NAME. An entry that
  # silently lost a field is a failure, not an omission.
  for t_field in "${REQUIRED_TOOL_FIELDS[@]}"; do
    [[ -n "${LOCK_FIELD[$t_idx/$t_field]+x}" ]] || fail docs-tool-mutable
  done
  t_key="${LOCK_FIELD[$t_idx/declared_in]}"$'\t'"${LOCK_FIELD[$t_idx/name]}"
  [[ -z "${LOCK_KEY_SEEN[$t_key]:-}" ]] || fail docs-tool-identity-mismatch   # duplicate entry
  LOCK_KEY_SEEN[$t_key]=1
  # The committed entry must name a real declaration...
  [[ -n "${DECL_SEEN[$t_key]:-}" ]] || fail docs-tool-identity-mismatch
  # ...and its resolved facts must equal the resolution artifact's, field by
  # field. This is what makes a forged lock entry useless.
  [[ "${LOCK_FIELD[$t_idx/resolved_version]}" == "${RES_VERSION[$t_key]}" ]] || fail docs-tool-mutable
  [[ "${LOCK_FIELD[$t_idx/artifact_digest]}" == "${RES_DIGEST[$t_key]}" ]] || fail docs-tool-mutable
  [[ "${LOCK_FIELD[$t_idx/resolved_version]}" =~ $RE_EXACT_VERSION ]] || fail docs-tool-mutable
  [[ "${LOCK_FIELD[$t_idx/artifact_digest]}" =~ $RE_DIGEST ]] || fail docs-tool-mutable
  [[ "${LOCK_FIELD[$t_idx/version_constraint]}" == "${DECL_CONSTRAINT[$t_key]}" ]] || fail docs-tool-identity-mismatch
  [[ "${LOCK_FIELD[$t_idx/runtime]}" == "${DECL_RUNTIME[$t_key]}" ]] || fail runtime-resolution-inconsistent
  [[ "${LOCK_FIELD[$t_idx/runtime_version]}" == "${RT_VERSION[${DECL_RUNTIME[$t_key]}]}" ]] || fail runtime-resolution-inconsistent
done
# ...and every declaration must appear in the committed lock. Both inclusions,
# both by name.
for decl_key in "${DECL_KEYLIST[@]}"; do
  [[ -n "${LOCK_KEY_SEEN[$decl_key]:-}" ]] || fail docs-tool-identity-mismatch
done

# --- 7g. the read-path attestation, as a SET -------------------------------
# The reference truth here is the REAL bytes on disk, recomputed now; the
# attestation and the lock are two independently maintained records compared
# against it, not against each other (Regra dos Seis, item 6). Blocker B4:
# a duplicated row can no longer stand in for a missing one, because each
# canonical path is marked SEEN and every canonical path is asserted present
# BY NAME afterwards.
# The grammar is an ALLOWLIST with no skip arm: line 1 is the schema, EVERY
# other physical line must be a two-field row naming a canonical read_path
# that has not been seen yet. There is no comment production and no blank
# production, so nothing can be smuggled past the loop by being ignored.
declare -A ATTEST_SEEN=()
attest_lineno=0
while IFS= read -r a_line || [[ -n "$a_line" ]]; do
  attest_lineno=$((attest_lineno + 1))
  if [[ "$attest_lineno" == '1' ]]; then
    [[ "$a_line" == 'attest-schema: read-paths-v1' ]] || fail invalid_lock_or_manifest
  else
    # EXACT split: exactly two non-empty fields. A row with a doubled TAB is
    # rejected instead of being folded into a well-formed one.
    split_tabs "$a_line"
    [[ "${#SPLIT[@]}" == '2' ]] || fail invalid_lock_or_manifest              # BOUND
    a_path="${SPLIT[0]}"
    a_sha="${SPLIT[1]}"
    [[ -n "$a_path" && -n "$a_sha" ]] || fail invalid_lock_or_manifest
    [[ -z "${ATTEST_SEEN[$a_path]:-}" ]] || fail invalid_lock_or_manifest   # duplicate row
    a_known='no'
    for rel in "${CANONICAL_READ_PATHS[@]}"; do
      [[ "$rel" != "$a_path" ]] || a_known='yes'
    done
    [[ "$a_known" == 'yes' ]] || fail invalid_lock_or_manifest
    real_sha="sha256:$(sha_of_file "$repo_root/$a_path")"
    [[ "$a_sha" == "$real_sha" ]] || fail docs-tool-unlocked
    ATTEST_SEEN[$a_path]=1
  fi
done < "$attest_file"
# B4: a duplicated row can no longer stand in for a missing one, because the
# duplicate is rejected above AND every canonical path must appear here BY
# NAME. Neither assertion is a comparison of row counts.
for rel in "${CANONICAL_READ_PATHS[@]}"; do
  [[ -n "${ATTEST_SEEN[$rel]:-}" ]] || fail invalid_lock_or_manifest
done

# --- 7h. the committed lock's own read_path digests, as a SET -------------
declare -A LOCKDIG_SEEN=()
while IFS= read -r line; do
  [[ -n "$line" ]] || continue
  rp_idx="$(printf '%s' "$line" | sed -e 's/^read_path\[\([0-9]*\)\]\.sha256: .*$/\1/')"
  rp_val="$(printf '%s' "$line" | sed -e 's/^read_path\[[0-9]*\]\.sha256: //')"
  for rel in "${CANONICAL_READ_PATHS[@]}"; do
    if [[ "${RP_SEEN[$rel]}" == "$rp_idx" ]]; then
      [[ -z "${LOCKDIG_SEEN[$rel]:-}" ]] || fail invalid_lock_or_manifest   # duplicate row
      [[ "$rp_val" == "sha256:$(sha_of_file "$repo_root/$rel")" ]] || fail docs-tool-unlocked
      LOCKDIG_SEEN[$rel]=1
    fi
  done
done < <(grep -E '^read_path\[[0-9]+\]\.sha256: ' "$lock_file" || true)
for rel in "${CANONICAL_READ_PATHS[@]}"; do
  [[ -n "${LOCKDIG_SEEN[$rel]:-}" ]] || fail invalid_lock_or_manifest
done

# --- 7i. the lock must record the resolution artifact it was derived from --
grep -Fqx "resolution.path: $resolution_rel" "$lock_file" || fail invalid_lock_or_manifest
grep -Fqx "resolution.sha256: sha256:$(sha_of_file "$resolution_file")" "$lock_file" || fail docs-tool-unlocked
grep -Fqx "resolution.schema: $RESOLUTION_SCHEMA" "$lock_file" || fail invalid_lock_or_manifest
grep -Fqx "resolution.digest_domain: $RESOLUTION_DIGEST_DOMAIN" "$lock_file" || fail invalid_lock_or_manifest

# --- 7j. the lock must seal the card's spec document ----------------------
# Second inclusion of the Lei 20 fix: section 3b proves the spec's CONTENT
# agrees with this script; this proves its BYTES are the ones the lock was
# generated from. A spec edited without --generate fails here.
grep -Fqx "spec.path: $SPEC_REL" "$lock_file" || fail invalid_lock_or_manifest
grep -Fqx "spec.sha256: sha256:$(sha_of_file "$spec_file")" "$lock_file" || fail docs-tool-unlocked

# --- 7k. the card's fixture directory, as a SET (Lei 20) -------------------
# Third and last inclusion of the Lei 20 fix. Both directions, each path
# marked SEEN by name, never by cardinality:
#   * every file the LISTING found must appear in the committed lock with the
#     digest of its real bytes right now — a fixture rewritten without a
#     deliberate --generate fails here;
#   * every card_artifact[] path the committed lock carries must be a file the
#     listing actually found — a fixture DELETED from the tree leaves its row
#     behind in the lock and fails here, which is exactly the removal test
#     Lei 20 prescribes.
# Neither side may be empty (Lei 12 item 4).
grep -Fqx "card_artifact.dir: $FIXTURE_DIR_REL" "$lock_file" || fail invalid_lock_or_manifest
grep -Fqx "card_artifact.count: ${#CARD_FIXTURES[@]}" "$lock_file" || fail docs-tool-unlocked

declare -A FIXTURE_ON_DISK=() FIXTURE_IN_LOCK=() FIXTURE_LOCK_PATH=() FIXTURE_LOCK_SHA=()

for fx_rel in "${CARD_FIXTURES[@]+"${CARD_FIXTURES[@]}"}"; do
  [[ -z "${FIXTURE_ON_DISK[$fx_rel]:-}" ]] || fail invalid_lock_or_manifest
  FIXTURE_ON_DISK[$fx_rel]=1
done
(( ${#FIXTURE_ON_DISK[@]} > 0 )) || fail docs_tool_lock_incomplete            # NONEMPTY

# The lock side is re-parsed from the lock's OWN bytes, index by index, so a
# path row and a digest row can never be read as belonging to different files.
while IFS= read -r fx_line; do
  [[ -n "$fx_line" ]] || continue
  fx_idx="${fx_line#card_artifact[}"; fx_idx="${fx_idx%%]*}"
  [[ -z "${FIXTURE_LOCK_PATH[$fx_idx]:-}" ]] || fail invalid_lock_or_manifest
  FIXTURE_LOCK_PATH[$fx_idx]="${fx_line#*: }"
done < <(grep -oE '^card_artifact\[[0-9]+\]\.path: .+$' "$lock_file" || true)

while IFS= read -r fx_line; do
  [[ -n "$fx_line" ]] || continue
  fx_idx="${fx_line#card_artifact[}"; fx_idx="${fx_idx%%]*}"
  [[ -z "${FIXTURE_LOCK_SHA[$fx_idx]:-}" ]] || fail invalid_lock_or_manifest
  FIXTURE_LOCK_SHA[$fx_idx]="${fx_line#*: }"
done < <(grep -oE '^card_artifact\[[0-9]+\]\.sha256: sha256:[0-9a-f]{64}$' "$lock_file" || true)

for fx_idx in "${!FIXTURE_LOCK_PATH[@]}"; do
  fx_rel="${FIXTURE_LOCK_PATH[$fx_idx]}"
  [[ -n "${FIXTURE_LOCK_SHA[$fx_idx]:-}" ]] || fail invalid_lock_or_manifest
  [[ -z "${FIXTURE_IN_LOCK[$fx_rel]:-}" ]] || fail invalid_lock_or_manifest
  FIXTURE_IN_LOCK[$fx_rel]="${FIXTURE_LOCK_SHA[$fx_idx]}"
done
(( ${#FIXTURE_IN_LOCK[@]} > 0 )) || fail docs_tool_lock_incomplete            # NONEMPTY

# Inclusion 1: on disk -> in lock, with the digest of the bytes that are
# there RIGHT NOW.
for fx_rel in "${!FIXTURE_ON_DISK[@]}"; do
  [[ -n "${FIXTURE_IN_LOCK[$fx_rel]:-}" ]] || fail docs-tool-unlocked
  [[ "${FIXTURE_IN_LOCK[$fx_rel]}" == "sha256:$(sha_of_file "$repo_root/$fx_rel")" ]] || fail docs-tool-unlocked
done
# Inclusion 2: in lock -> on disk. This is the removal test.
for fx_rel in "${!FIXTURE_IN_LOCK[@]}"; do
  [[ -n "${FIXTURE_ON_DISK[$fx_rel]:-}" ]] || fail docs-tool-unlocked
done

# --- 8. whole-artifact equality -------------------------------------------
committed_sha="$(sha_of_file "$lock_file")"
generated_sha="$(sha256sum <<< "$generated_text" | awk '{print $1}')"
if [[ "$committed_sha" == "$generated_sha" ]]; then
  printf '{"card":"%s","scenario":"AC-001","selector":"%s","result":"pass"}\n' "$card" "$selector"
  exit 0
fi

fail docs-tool-unlocked
