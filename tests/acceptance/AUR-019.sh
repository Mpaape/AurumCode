#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-019'
readonly scenario='AC-001'

selector="${1:-AC-001}"
case "$selector" in
  AC-001|IntegrationAUR019) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

infra() {
  printf '%s/%s/infrastructure: %s\n' "$card" "$scenario" "$1" >&2
  exit 3
}
fail() {
  printf '%s/%s/%s: %s\n' "$card" "$scenario" "$1" "$2" >&2
  exit 1
}
emitted() {
  [[ -n $1 ]] || return 0
  fail "${1%%$'\t'*}" "${1#*$'\t'}"
}

check_allowed_bytes() {
  # $1=file -> standards/providers/<slug>.yaml and .board/research/providers.md
  # are ASCII-locked policy/data. Every byte in the file must be printable
  # ASCII (0x20-0x7E) or a bare LF (0x0A); anything else -- NUL, any other
  # C0 control byte, DEL (0x7F), or any byte >= 0x80 -- is a typed
  # malformed-entry failure. This runs first, before check_canonical_form,
  # flat_value, or the "## Provider contracts matrix" awk walk over
  # $research read a single byte of either file, because those readers are
  # exactly what a disallowed byte can fool.
  #
  # Superseding the narrower "reject byte >= 0x80" check this replaces:
  # that check caught non-ASCII (see docs/specs/AUR-019.md's prior
  # "ASCII-only invariant" note) but its own [\x80-\xff] range never
  # covered NUL (0x00). NUL is exactly the byte the accepted MUT-001
  # vector abuses: this engine's userland is BusyBox (verified against
  # the pinned bash@sha256:ae4668c2... image), and BusyBox's awk treats an
  # embedded NUL as if it split the record, not as ordinary data. A
  # single physical line that smuggles
  # "\x00capability_structured_output: supported\x00fixture_structured_output: <path>"
  # onto the end of a real key's value is therefore not one line to
  # BusyBox awk -- it reads as three, and each of those three is a clean,
  # canonical "key: value" declaration, so check_canonical_form and
  # flat_value's awk-based duplicate count would wave the poisoned line
  # through as three legitimate ones even with the real
  # capability_structured_output/fixture_structured_output lines deleted.
  # tests/integration/AUR-019_test.go's parseFlatYAMLAUR019 never makes
  # that mistake -- Go's strings.Split only ever breaks on a real '\n' --
  # so the exact same bytes that pass this engine fail `go test` unless
  # this function closes the gap before any awk or grep reads the file.
  #
  # This is also why detection below deliberately avoids awk and grep:
  # under this BusyBox userland they are the tools an embedded NUL can
  # fool. `tr`, `wc`, and `od` were verified against the pinned image to
  # treat NUL as ordinary data with no special splitting behavior, so
  # comparing the file's total byte count against the count that survives
  # deleting every allowed byte detects any disallowed byte -- NUL
  # included -- and `od` then recovers its exact value and line for a
  # typed, located failure.
  local file="$1" total kept
  total="$(LC_ALL=C wc -c < "$file")" || infra "byte-count scan failed to run: $file"
  kept="$(LC_ALL=C tr -cd '\012\040-\176' < "$file" | LC_ALL=C wc -c)" || infra "byte-count scan failed to run: $file"
  [[ "$total" == "$kept" ]] && return 0

  local line=1 v hex
  while read -r v; do
    [[ -n $v ]] || continue
    if (( v == 10 )); then
      line=$((line + 1))
      continue
    fi
    if (( v == 32 )) || { (( v >= 33 )) && (( v <= 126 )); }; then
      continue
    fi
    printf -v hex '%02x' "$v"
    fail malformed-entry "disallowed byte 0x$hex at $file:$line"
  done < <(LC_ALL=C od -An -v -tu1 -w1 -- "$file")
  infra "byte scan found no disallowed byte despite a length mismatch: $file"
}

for tool in awk grep sed sort wc tr od; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing tool: $tool"
done

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." 2>/dev/null && pwd -P)" || infra 'repo root unresolved'
cd -- "$repo_root" || infra 'repo root unreachable'

check_canonical_dir() {
  # A lexical prefix is not containment when an ancestor is a symlink.  Resolve
  # every directory in a card-owned path and require the physical path to equal
  # the repository-relative path we intended to read.
  local dir="$1" label="$2" resolved expected
  [[ -d "$dir" && ! -L "$dir" ]] || fail undecided-provider "$label is not a real directory: $dir"
  resolved="$(CDPATH= cd -- "$dir" 2>/dev/null && pwd -P)" ||
    infra "cannot resolve directory: $dir"
  expected="$repo_root/${dir#./}"
  [[ "$resolved" == "$expected" ]] ||
    fail undecided-provider "$label resolves outside the canonical repository path: $dir"
}

real_calendar_date() {
  # $1=YYYY-MM-DD -> exit 0 iff it is a real calendar date: month 01-12,
  # day between 01 and the number of days in that month for that year, with
  # leap years per the Gregorian rule (divisible by 4, except centuries,
  # except every 400 years). Plain POSIX awk arithmetic only: the sealed
  # container has bash/awk and no python3 or date(1) validator. The shape
  # regex by itself accepted impossible dates -- "2026-99-99" passed AC-001
  # before this gate existed -- so every 4-2-2 digit value this card calls a
  # Date now has to be a date that an actual calendar can hold.
  local value="$1"
  awk -v d="$value" '
    function days_in_month(y, m,    n) {
      if (m == 2) {
        n = 28
        if (y % 4 == 0 && (y % 100 != 0 || y % 400 == 0)) n = 29
        return n
      }
      if (m == 4 || m == 6 || m == 9 || m == 11) return 30
      return 31
    }
    BEGIN {
      if (d !~ /^[0-9]{4}-[0-9]{2}-[0-9]{2}$/) exit 1
      y = substr(d, 1, 4) + 0
      m = substr(d, 6, 2) + 0
      day = substr(d, 9, 2) + 0
      if (y < 1 || m < 1 || m > 12 || day < 1) exit 1
      if (day > days_in_month(y, m)) exit 1
      exit 0
    }
  '
}

check_canonical_dir tests 'tests directory'
check_canonical_dir tests/specs 'specs directory'
check_canonical_dir tests/specs/AUR-019 'AUR-019 specs directory'
check_canonical_dir tests/specs/AUR-019/fixtures 'fixture root directory'

# .board/research is the research-baseline directory this card reads its
# "Provider contracts matrix" from. The per-file guard below
# (`[[ -f $research && ! -L $research ]]`) proves providers.md itself is not
# a symlink, but a symlinked PARENT -- .board/research resolving outside the
# repository -- defeats it: the file is not a link, so the file check passes,
# while every read resolves to bytes outside the repo. check_canonical_dir
# resolves the directory and every ancestor via `pwd -P` (the realpath
# equivalent that needs no external tool -- the pinned container has
# bash/awk only) and requires the physical path to equal the canonical repo
# path, typed undecided-provider. A *missing* research directory is
# deliberately not rejected here: .board/research/providers.md missing must
# keep its documented "provider baseline absent" RED message, which the
# file-level check right below this emits.
if [[ -e .board/research || -L .board/research ]]; then
  check_canonical_dir .board/research 'research directory'
fi

readonly research='.board/research/providers.md'
readonly standards_dir='standards/providers'
readonly spec_dir='tests/specs/AUR-019'
readonly names_fixture="$spec_dir/provider-names.txt"
readonly caps_fixture="$spec_dir/capabilities.txt"
readonly allow_fixture="$spec_dir/allowed-source-domains.txt"

matrix_value() {
  # $1=provider $2=zero-based criterion index (0=Wire format) -> cell
  local provider="$1" index="$2"
  awk -F'\t' -v wanted="$provider" -v column="$((index + 5))" \
    '$1 == wanted { print $column; exit }' <<<"$data_rows"
}

check_versioned_source() {
  # $1=source markdown link $2=declared version -> reject mutable URLs and
  # require the URL itself to carry the declared release/API version.
  local source="$1" version="$2" url version_without_v source_url_re
  source_url_re='\((https?://[^)]*)\)'
  [[ "$source" =~ $source_url_re ]] ||
    fail unsourced-provider 'source is not a [label](url) link'
  url="${BASH_REMATCH[1]}"
  if [[ "$url" =~ (^|[/._-])(main|master|trunk|head|latest)([/._?#-]|$) ||
        "$url" == *'/tree/main'* || "$url" == *'/tree/master'* ||
        "$url" == *'branch=main'* || "$url" == *'branch=master'* ||
        "$url" == *'ref=main'* || "$url" == *'ref=master'* ]]; then
    fail unsourced-provider "source URL is mutable: $url"
  fi
  version_without_v="${version#v}"
  [[ "$url" == *"$version"* || "$url" == *"$version_without_v"* ]] ||
    fail unsourced-provider "source URL does not carry declared version $version: $url"
}

# provider baseline absent: this exact message and typed prefix is the
# card's documented RED behavior (AC-001 "Given" clause).
[[ -f "$research" && ! -L "$research" ]] || {
  printf '%s/%s: provider baseline absent\n' "$card" "$scenario" >&2
  exit 1
}
[[ -r "$research" ]] || infra "artifact unreadable: $research"
[[ -s "$research" ]] || {
  printf '%s/%s: provider baseline absent\n' "$card" "$scenario" >&2
  exit 1
}
for needle in 'OpenAI' 'LiteLLM' 'Anthropic' 'Ollama' 'Azure' 'Gemini' 'Bedrock'; do
  grep -Fq "$needle" "$research" || {
    printf '%s/%s: provider baseline absent\n' "$card" "$scenario" >&2
    exit 1
  }
done

# Byte-invariant gate on the research baseline itself: must run before the
# "## Provider contracts matrix" awk walk below ever reads $research, for
# the same reason it must run before check_canonical_form/flat_value on
# each standards/providers/<slug>.yaml file (see check_allowed_bytes).
check_allowed_bytes "$research"

for f in "$names_fixture" "$caps_fixture" "$allow_fixture"; do
  [[ -f $f && ! -L $f ]] || fail undecided-provider "required fixture absent: $f"
  [[ -r $f ]] || infra "fixture unreadable: $f"
  [[ -s $f ]] || fail undecided-provider "required fixture is empty: $f"
  check_allowed_bytes "$f"
done

mapfile -t required_names < <(grep -Ev '^[[:space:]]*$' "$names_fixture")
((${#required_names[@]} == 7)) ||
  fail undecided-provider "provider-names fixture declares ${#required_names[@]} names, want exactly 7"
# Count alone is not proof: padding the list with a second copy of an
# already-required name keeps the count at 7 while silently dropping one
# real provider (whichever name got replaced) out of every check the rest
# of this script drives from required_names -- the loop below would just
# revalidate the duplicate's standards/providers/<slug>.yaml twice and
# never touch the dropped provider's file at all, while still reporting
# "required_providers":7 in the final artifact. sort -u catches that
# unconditionally, before required_names drives anything.
unique_name_count="$(printf '%s\n' "${required_names[@]}" | sort -u | wc -l)"
((unique_name_count == ${#required_names[@]})) ||
  fail undecided-provider "provider-names fixture declares ${#required_names[@]} names but only $unique_name_count are distinct"
mapfile -t required_caps < <(grep -Ev '^[[:space:]]*$' "$caps_fixture")
((${#required_caps[@]} == 3)) ||
  fail undecided-provider "capabilities fixture declares ${#required_caps[@]} entries, want exactly 3"
unique_cap_count="$(printf '%s\n' "${required_caps[@]}" | sort -u | wc -l)"
((unique_cap_count == ${#required_caps[@]})) ||
  fail undecided-provider "capabilities fixture declares ${#required_caps[@]} capabilities but only $unique_cap_count are distinct"
for expected_cap in streaming tool_use structured_output; do
  printf '%s\n' "${required_caps[@]}" | grep -Fxq "$expected_cap" ||
    fail undecided-provider "capabilities fixture is not the exact required set; missing $expected_cap"
done
AURUM_ALLOWED_DOMAINS="$(grep -Ev '^[[:space:]]*$' "$allow_fixture")"
export AURUM_ALLOWED_DOMAINS

# ---------------------------------------------------------------------------
# Provider contracts matrix in the research doc: one row per required
# provider, each with a versioned, dated, allowlisted Source and four
# non-empty criterion cells (Wire format, Auth, Error taxonomy,
# Capabilities). Mirrors AUR-020's alternatives-matrix lint.
# ---------------------------------------------------------------------------
table="$(
  awk '
    BEGIN { in_section = 0; in_table = 0; header_done = 0 }
    /^## Provider contracts matrix[[:space:]]*$/ { in_section = 1; next }
    in_section && /^## / { in_section = 0 }
    in_section && !in_table && /^\|/ {
      in_table = 1
      line = $0
      sub(/^\|[[:space:]]*/, "", line)
      sub(/[[:space:]]*\|[[:space:]]*$/, "", line)
      n = split(line, cols, /[[:space:]]*\|[[:space:]]*/)
      out = "HEADER"
      for (i = 5; i <= n; i++) out = out "\t" cols[i]
      print out
      next
    }
    in_table && /^\|[[:space:]]*-+/ { next }
    in_table && /^\|/ {
      line = $0
      sub(/^\|[[:space:]]*/, "", line)
      sub(/[[:space:]]*\|[[:space:]]*$/, "", line)
      n = split(line, cols, /[[:space:]]*\|[[:space:]]*/)
      out = cols[1]
      for (i = 2; i <= n; i++) out = out "\t" cols[i]
      print out
      next
    }
    in_table && !/^\|/ { in_table = 0; in_section = 0 }
  ' "$research"
)" || infra 'provider contracts matrix parse failed to run'

[[ -n $table ]] || fail undecided-provider "no ## Provider contracts matrix table found in $research"

header_line="$(printf '%s\n' "$table" | awk -F'\t' 'NR == 1 && $1 == "HEADER" { sub(/^HEADER\t/, ""); print $0; exit }')"
[[ -n $header_line ]] || fail undecided-provider "provider contracts matrix has no header row"
data_rows="$(printf '%s\n' "$table" | awk 'NR > 1')"
[[ -n $data_rows ]] || fail undecided-provider "provider contracts matrix has no data rows"

criteria_count="$(awk -F'\t' '{ print NF }' <<<"$header_line")"
((criteria_count == 4)) ||
  fail undecided-provider "provider contracts matrix declares $criteria_count criteria columns, want exactly 4"
[[ "$header_line" == $'Wire format\tAuth\tError taxonomy\tCapabilities' ]] ||
  fail undecided-provider "provider contracts matrix criteria headers are not exactly Wire format, Auth, Error taxonomy, Capabilities"

AURUM_REQUIRED_NAMES="$(printf '%s\n' "${required_names[@]}")"
export AURUM_REQUIRED_NAMES
export AURUM_TABLE_DATA="$data_rows"
verdict="$(
  awk -F'\t' -v ccount="$criteria_count" '
    function emit(code, detail) { print code "\t" detail; done = 1; exit 0 }
    function trim(s) { gsub(/^[[:space:]]+|[[:space:]]+$/, "", s); return s }
    function days_in_month(y, m,    n) {
      if (m == 2) {
        n = 28
        if (y % 4 == 0 && (y % 100 != 0 || y % 400 == 0)) n = 29
        return n
      }
      if (m == 4 || m == 6 || m == 9 || m == 11) return 30
      return 31
    }
    function is_real_date(s,    y, m, d) {
      if (s !~ /^[0-9]{4}-[0-9]{2}-[0-9]{2}$/) return 0
      y = substr(s, 1, 4) + 0
      m = substr(s, 6, 2) + 0
      d = substr(s, 9, 2) + 0
      if (y < 1 || m < 1 || m > 12 || d < 1) return 0
      return (d <= days_in_month(y, m))
    }
    BEGIN {
      req_n = split(ENVIRON["AURUM_REQUIRED_NAMES"], req, "\n")
      for (i = 1; i <= req_n; i++) if (req[i] != "") want[req[i]] = 1
      dom_n = split(ENVIRON["AURUM_ALLOWED_DOMAINS"], doms, "\n")
      for (i = 1; i <= dom_n; i++) if (doms[i] != "") allowed[doms[i]] = 1
      n = split(ENVIRON["AURUM_TABLE_DATA"], lines, "\n")
      for (li = 1; li <= n; li++) {
        line = lines[li]
        if (line == "") continue
        m = split(line, c, "\t")
        if (m != 4 + ccount)
          emit("undecided-provider", "matrix row has " m " columns, want exactly " (4 + ccount) ": " line)
        name = trim(c[1]); src = c[2]; ver = trim(c[3]); dt = trim(c[4])
        if (!(name in want))
          emit("undecided-provider", "matrix contains unexpected provider: " name)
        seen[name]++
        if (seen[name] > 1) emit("undecided-provider", "provider cited twice in matrix: " name)
        if (match(src, /\(https?:\/\/[^)\/]+/) == 0)
          emit("unsourced-provider", "provider " name " source is not a [label](url) link")
        host = substr(src, RSTART, RLENGTH)
        sub(/^\(https?:\/\//, "", host)
        if (!(host in allowed))
          emit("unsourced-provider", "provider " name " source host is not allowlisted: " host)
        if (ver !~ /^v?[0-9]+(\.[0-9]+){1,2}$/ && ver !~ /^[0-9]{4}-[0-9]{2}-[0-9]{2}$/)
          emit("unsourced-provider", "provider " name " has no versioned source: " ver)
        if (dt !~ /^[0-9]{4}-[0-9]{2}-[0-9]{2}$/)
          emit("unsourced-provider", "provider " name " source date is not ISO 8601: " dt)
        else if (!is_real_date(dt))
          emit("unsourced-provider", "provider " name " source date is not a real calendar date: " dt)
        for (ci = 0; ci < ccount; ci++) {
          cell = trim(c[5 + ci])
          if (length(cell) < 4)
            emit("undecided-provider", "provider " name " has an empty criterion cell")
        }
      }
    }
    END {
      if (done) exit 0
      for (a in want) if (!(a in seen))
        emit("undecided-provider", "required provider missing from matrix: " a)
    }
  ' <<<"$data_rows"
)" || infra 'provider contracts matrix row lint failed to run'
emitted "$verdict"

# ---------------------------------------------------------------------------
# standards/providers/<slug>.yaml: one file per required provider, slug is
# the lowercase provider name with no other transformation. Every required
# flat key must be present and non-empty; every capability_<cap>: supported
# must have a matching fixture_<cap>: entry whose target exists, is
# non-empty, is valid JSON, and whose own "capability"/"provider" fields
# match the file it is declared from.
# ---------------------------------------------------------------------------
required_flat_keys=(provider source version date wire_endpoint wire_method wire_content_type \
  auth_scheme auth_header auth_format error_envelope error_taxonomy transport)

# auth_binding and wire_call are conditionally required, not unconditionally
# like required_flat_keys above: exactly one of them applies to a given
# provider, chosen by that same provider's own "transport" value (see the
# validation right after the required_flat_keys loop below), never by
# whether the fixture happens to carry a matching field. wire_streaming_suffix
# is genuinely optional -- its absence never widens what a fixture is allowed
# to match, only the presence of a declared value ever adds an accepted
# suffix (see wire_path_prefix_regex), so it carries no fixed-required
# counterpart.
optional_flat_keys=(auth_binding wire_call wire_streaming_suffix auth_query_param auth_evidence)

# known_keys is every schema key this card recognizes in a
# standards/providers/<slug>.yaml file: the fixed required_flat_keys, the
# conditionally-required/optional keys above, plus the per-tracked-capability
# capability_<cap>/fixture_<cap> pair. It is built once (required_caps is
# already resolved above) and sorted longest key first so a canonical-form
# scan that matches a line against each known key in turn never lets a short
# key shadow a longer key that shares its prefix.
known_keys=("${required_flat_keys[@]}" "${optional_flat_keys[@]}")
for cap in "${required_caps[@]}"; do
  known_keys+=("capability_$cap" "fixture_$cap" "wire_action_$cap")
done
AURUM_KNOWN_KEYS="$(printf '%s\n' "${known_keys[@]}" | awk '{ print length, $0 }' | sort -rn | cut -d' ' -f2-)"
export AURUM_KNOWN_KEYS

check_canonical_form() {
  # $1=file -> enforces one strict shape for every line that declares a
  # known schema key, and fails typed malformed-entry the instant a line
  # violates it -- before flat_value ever runs a duplicate count.
  #
  # Why this exists instead of another round of "widen flat_value's
  # anchor": tests/integration/AUR-019_test.go's parseFlatYAMLAUR019 finds
  # "key: value" with an unanchored strings.Index(line, ": ") and then
  # TrimSpace()s whatever sits to the left of it, so it recognizes a key
  # written with leading whitespace, or with a space before the colon, or
  # both, as the same key -- and folds it into Go's map with ordinary
  # last-write-wins semantics. Chasing that behavior permutation by
  # permutation in this bash reader (first "^", then "^[[:space:]]*", next
  # tolerating a space before the colon, and so on) only ever converges on
  # the *next* permutation the Go side still recognizes and this reader
  # still doesn't -- which is exactly how the prior round shipped a bypass:
  # a duplicate with a space before the colon slipped past flat_value's
  # hit count while Go's parser applied last-wins to it for real.
  #
  # This function ends that chase by inverting the problem: a known key is
  # never allowed to appear in any form other than the one canonical shape
  # both readers already agree on -- the key sitting at column 0,
  # immediately followed by ": " (one colon, one space, nothing between).
  # Every line is checked against that rule for every known key it could
  # plausibly be declaring (its whitespace-stripped content starting with
  # that key followed by zero-or-more spaces/tabs and a colon). A line
  # that clears that bar is canonical and goes on to flat_value's
  # duplicate count below; a line that does not is rejected right here,
  # typed malformed-entry, regardless of which specific whitespace
  # permutation produced it. There is no longer a form of "this key,
  # written some other way" that can be invisible to this gate while still
  # being visible -- and resolvable -- to the Go reader.
  local file="$1" verdict
  verdict="$(
    awk -v keys="$AURUM_KNOWN_KEYS" '
      BEGIN {
        nk = split(keys, karr, "\n")
        for (i = 1; i <= nk; i++) if (karr[i] != "") klist[++kn] = karr[i]
      }
      {
        # A trailing space is invisible in most editors and every reader of
        # this file must treat it the same way. It already cannot: this
        # readers value extraction below strips a value only when the whole
        # value is quote-wrapped, so a trailing space on an unquoted value
        # like "provider: OpenAI   " survives into the extracted value and
        # fails the caller comparison, while tests/integration/AUR-019_test.go
        # previously ran every extracted value through strings.TrimSpace and
        # would have accepted the padded value silently. Rejecting any
        # trailing space here, on every line, closes that gap at the same
        # layer this function already owns instead of leaving it for a
        # downstream comparison to discover by accident.
        if ($0 ~ /[ \t]$/) {
          print "malformed-entry\t" FILENAME " line " FNR " has trailing whitespace: " $0
          exit 0
        }
        trimmed = $0
        gsub(/^[[:space:]]+|[[:space:]]+$/, "", trimmed)
        if (trimmed == "" || trimmed ~ /^#/) next
        for (i = 1; i <= kn; i++) {
          k = klist[i]
          klen = length(k)
          if (substr(trimmed, 1, klen) != k) continue
          rest = substr(trimmed, klen + 1)
          if (rest !~ /^[ \t]*:/) continue
          canon = k ": "
          if (substr($0, 1, length(canon)) != canon) {
            print "malformed-entry\t" FILENAME " line " FNR " declares key \"" k "\" outside its canonical \"" k ": value\" form: " $0
            exit 0
          }
          break
        }
      }
    ' "$file"
  )" || infra "canonical form scan failed to run: $file"
  emitted "$verdict"
}

check_fixture_canonical_form() {
  # $1=file -> a tests/specs/AUR-019/fixtures/<slug>/<capability>.json
  # fixture. Enforces a strict canonical JSON shape line by line and fails
  # typed malformed-entry the instant any line violates it, the same way
  # check_canonical_form does for standards/providers/<slug>.yaml.
  #
  # Why this exists: the code this replaces extracted the fixture's
  # "capability"/"provider" binding with `grep -m1 ... | sed ...` -- two
  # substring greps, never a JSON parse. That accepted two classes of input
  # docs/specs/AUR-019.md's own Contract explicitly rules out ("JSON-valid
  # fixture"), and tests/integration/AUR-019_test.go's `json.Unmarshal`
  # correctly rejects both:
  #   1. A file that is not JSON at all -- free prose containing nothing
  #      but the two magic lines `"capability": "..."` / `"provider": "..."`
  #      satisfied grep -m1 while failing every real JSON parser.
  #   2. A file that is syntactically valid JSON with the "provider" key
  #      declared twice -- RFC 8259 allows this, and both a real JSON
  #      parser and `grep -m1` accept the file, but they disagree on which
  #      value wins: `encoding/json` (and every conforming decoder) takes
  #      the *last* occurrence, while `grep -m1` takes the *first*. A
  #      fixture whose first "provider" is correct and second is not would
  #      pass this engine while go test read a different provider entirely.
  #
  # The BusyBox container this runs in has no jq and no python3, so this
  # does not attempt a general-purpose JSON parser in awk. It imposes a
  # single strict canonical form instead -- authored at authoring time and
  # enforced byte-for-byte here, never inside a Python script -- and
  # validates the whole file against it:
  #   - every object's members appear in strictly ascending key order.
  #     Two equal keys can never be "strictly ascending", so this single
  #     ascending-order check is simultaneously how a duplicate key is
  #     detected and how "a fixed key order" (this format's one legal
  #     order) is enforced; there is no second, separate duplicate pass to
  #     fall out of sync with this one.
  #   - one member or element per line, a fixed two-space indent per
  #     nesting level, no inline "{...}" compaction of a non-empty
  #     container -- byte-for-byte the same shape
  #     `json.dumps(data, indent=2, sort_keys=True, ensure_ascii=True)`
  #     would produce, so any decoder agrees with this gate.
  #   - a line that is not one of: the bare root opener, an object/array
  #     opener, an object/array closer, a "key": value member, or a bare
  #     array element, is rejected outright -- so free-form prose fails on
  #     line 1 (it is not a bare "{") the same way a syntax-broken JSON
  #     document fails on whichever line stops matching the grammar.
  # Because every fixture this accepts has exactly one legal parse -- no
  # duplicate keys, no ambiguity -- `encoding/json.Unmarshal` on the same
  # bytes cannot land on a different "capability"/"provider" value than
  # this engine does: there is only one value to find.
  #
  # This also requires the top-level "capability" and "provider" keys to
  # exist and to hold a plain JSON string (not an object, array, number,
  # bool, or null) -- the same requirement
  # `struct{ Capability, Provider string }` + `json.Unmarshal` imposes on
  # the Go side, made explicit here instead of merely inherited by luck.
  #
  # Run check_allowed_bytes on $1 before this, for the same reason it runs
  # before check_canonical_form on a standards/providers/<slug>.yaml file:
  # an embedded NUL is exactly the kind of byte this BusyBox awk could
  # treat as a record boundary no other tool here agrees with.
  local file="$1" verdict
  verdict="$(
    awk '
      function fail(msg) {
        print "malformed-entry\t" FILENAME ": " msg
        bailed = 1
        exit
      }
      function repeat(s, times,    r, i) {
        r = ""
        for (i = 0; i < times; i++) r = r s
        return r
      }
       function close_comma(line, parent_ind, closer) {
         if (line == parent_ind closer) return 0
         if (line == parent_ind closer ",") return 1
         return -1
      }
      BEGIN {
        key_re = "\"([^\"\\\\]|\\\\[\"\\\\/bfnrt]|\\\\u[0-9a-fA-F]{4})*\""
        # The full JSON number grammar (RFC 8259 section 6), not just
        # integers: an optional fraction (dot-digits) and/or exponent
        # (e-plus-or-minus-digits) after the integer part.
        # docs/specs/AUR-019.md and this function own docstring both claim
        # this validates exactly what
        # json.dumps(data, indent=2, sort_keys=True, ensure_ascii=True)
        # produces -- an integer-only num_re made that claim false for any
        # legitimate decimal or exponent-notation parameter, e.g.
        # temperature 0.7: json.dumps emits it at the correct canonical
        # position and json.Unmarshal on the Go side accepts it without
        # complaint, while this awk grammar rejected the whole file as
        # malformed-entry. Widening num_re to the full grammar instead of
        # narrowing the doc claim means every fixture json.dumps can
        # legally produce is exactly what this gate accepts -- no future
        # fixture numeric field is permanently unrepresentable.
        num_re = "-?(0|[1-9][0-9]*)(\\.[0-9]+)?([eE][+-]?[0-9]+)?"
        scalar_re = "(" key_re "|" num_re "|true|false|null)"
        bailed = 0
        found_capability = 0
        found_provider = 0
      }
      { lines[NR] = $0 }
      END {
        n = NR
        if (n < 1) fail("empty file")

        if (lines[1] != "{") fail("line 1 is not a bare \"{\" object opener: " lines[1])
        sp = 1
        kind[1] = "O"
        lastkey[1] = ""

        for (i = 2; i <= n; i++) {
          if (bailed) break
          line = lines[i]
          if (sp == 0) fail("line " i " follows the closed root object: " line)
          if (bailed) break

          depth = sp
          ind = repeat("  ", depth)
          parent_ind = repeat("  ", depth - 1)

           close_status = close_comma(line, parent_ind, "}")
           if (close_status >= 0) {
             if (kind[sp] != "O") fail("line " i " closes an object but frame " sp " is an array: " line)
             if (sp == 1) {
               if (close_status != 0 || i != n) fail("root object has an invalid trailing comma or trailing data: " line)
             } else {
               parent_close = (kind[sp - 1] == "O" ? "}" : "]")
               if (i == n) fail("line " i " closes a nested object without closing its parent: " line)
               parent_ind = repeat("  ", sp - 2)
               next_is_parent_close = (lines[i + 1] == parent_ind parent_close || lines[i + 1] == parent_ind parent_close ",")
               if (close_status != (!next_is_parent_close)) fail("line " i " has an invalid comma after a nested object: " line)
             }
             sp--
             continue
           }
           close_status = close_comma(line, parent_ind, "]")
           if (close_status >= 0) {
             if (kind[sp] != "A") fail("line " i " closes an array but frame " sp " is an object: " line)
             if (sp == 1) fail("root array is not allowed: " line)
             parent_close = (kind[sp - 1] == "O" ? "}" : "]")
             if (i == n) fail("line " i " closes a nested array without closing its parent: " line)
             parent_ind = repeat("  ", sp - 2)
             next_is_parent_close = (lines[i + 1] == parent_ind parent_close || lines[i + 1] == parent_ind parent_close ",")
             if (close_status != (!next_is_parent_close)) fail("line " i " has an invalid comma after a nested array: " line)
             sp--
             continue
           }

          if (substr(line, 1, length(ind)) != ind) fail("line " i " is not indented to " length(ind) " spaces for depth " depth ": " line)
          if (bailed) break
          rest = substr(line, length(ind) + 1)

          keytext = ""
          if (kind[sp] == "O") {
            if (!match(rest, "^" key_re)) fail("line " i " does not open with a quoted key: " line)
            if (bailed) break
            keytext = substr(rest, 1, RLENGTH)
            after_key = substr(rest, RLENGTH + 1)
            if (substr(after_key, 1, 2) != ": ") fail("line " i " key is not followed by \": \": " line)
            if (bailed) break
            if (keytext <= lastkey[sp]) fail("line " i " key " keytext " is out of canonical ascending order (or a duplicate) after " lastkey[sp] ": " line)
            if (bailed) break
            lastkey[sp] = keytext
            valuepart = substr(after_key, 3)
          } else {
            valuepart = rest
          }

          if (depth == 1 && (keytext == "\"capability\"" || keytext == "\"provider\"")) {
            vp = valuepart
            sub(/,$/, "", vp)
            if (!(vp ~ ("^" key_re "$"))) fail("line " i " " keytext " must be a plain JSON string: " line)
            if (bailed) break
            if (keytext == "\"capability\"") found_capability = 1
            if (keytext == "\"provider\"") found_provider = 1
          }

          if (valuepart == "{") { sp++; kind[sp] = "O"; lastkey[sp] = ""; continue }
          if (valuepart == "[") { sp++; kind[sp] = "A"; lastkey[sp] = ""; continue }

          if (i + 1 > n) fail("line " i " is the last line of the file but no closing bracket follows: " line)
          if (bailed) break
           is_last = (close_comma(lines[i + 1], parent_ind, (kind[sp] == "O" ? "}" : "]")) >= 0)

          vp = valuepart
          trailing_comma = (substr(vp, length(vp)) == ",")
          if (trailing_comma) vp = substr(vp, 1, length(vp) - 1)
          if (trailing_comma == is_last) fail("line " i " comma usage disagrees with its position among siblings: " line)
          if (bailed) break

          if (vp != "{}" && vp != "[]" && !(vp ~ ("^" scalar_re "$"))) fail("line " i " is not a recognized scalar, empty container, or container opener: " line)
          if (bailed) break
        }
        if (bailed) exit 0
        if (sp != 0) fail(n " frame(s) still open at end of file")
        if (bailed) exit 0
        if (!found_capability) fail("fixture has no top-level \"capability\" key")
        if (bailed) exit 0
        if (!found_provider) fail("fixture has no top-level \"provider\" key")
      }
    ' "$file"
  )" || infra "fixture canonical form scan failed to run: $file"
  emitted "$verdict"
}

escape_ere_literal() {
  # $1=text -> prints $1 with every ERE metacharacter backslash-escaped, so
  # the result matches $1 literally when used inside another ERE. '/' is
  # deliberately left unescaped: it has no special meaning in POSIX ERE and
  # every path this is used on is '/'-delimited, so escaping it would only
  # add noise.
  #
  # Character-by-character in the shell rather than a single sed/awk
  # bracket-expression regex: BusyBox sed's -E rejects "{" and "}" inside a
  # "[...]" bracket expression outright (verified against the pinned
  # bootstrap-readonly-v1 image -- "bad regex ... Invalid contents of {}"),
  # even though POSIX ERE gives "{"/"}" no special meaning there. A loop
  # over individual bytes with a plain case statement has no bracket
  # expression to disagree about.
  local text="$1" out='' i ch
  for (( i = 0; i < ${#text}; i++ )); do
    ch="${text:i:1}"
    case "$ch" in
      '.'|'^'|'$'|'*'|'+'|'?'|'('|')'|'['|']'|'{'|'}'|'|'|'\')
        out+="\\$ch" ;;
      *)
        out+="$ch" ;;
    esac
  done
  printf '%s' "$out"
}

wire_path_prefix_regex() {
  # $1 = a provider's standards/providers/<slug>.yaml wire_endpoint value
  # $2 = optional literal suffix (e.g. Bedrock's wire_streaming_suffix,
  #      "-stream"), passed by the caller only while validating the
  #      "streaming" capability's own fixture; $3 is an optional
  #      capability-specific action (Gemini's generateContent vs
  #      streamGenerateContent) -> prints an ERE, anchored at
  #      both "^" and a real end boundary, that a fixture's recorded
  #      request.path must match for the fixture to be accepted as actually
  #      describing this provider's wire format (see the call site in the
  #      provider loop below for why this exists).
  #
  # Every "{placeholder}" segment (e.g. "{model}", "{deployment-id}") is
  # replaced with "[^/?]+" -- any run of one or more characters that is
  # neither a path separator nor the start of a query string -- since the
  # fixture records one concrete, real value there (a model id, a
  # deployment name), not the literal placeholder text. Every other
  # character is escaped so it can only match itself.
  #
  # What follows the resource segment (or the placeholder's greedy run) is
  # never left open-ended: a declared action or streaming suffix is required,
  # followed only by end-of-path or a query separator. Arbitrary trailing text
  # cannot satisfy this boundary.
  local endpoint suffix action declared_action out seg rest term
  endpoint="$1"
  suffix="${2:-}"
  action="${3:-}"
  if [[ "$endpoint" == *:* ]]; then
    declared_action="${endpoint#*:}"
    endpoint="${endpoint%%:*}"
    [[ -n "$action" ]] || action="$declared_action"
  fi
  out='^'
  rest="$endpoint"
  while [[ "$rest" == *'{'*'}'* ]]; do
    seg="${rest%%\{*}"
    out+="$(escape_ere_literal "$seg")"
    out+='[^/?]+'
    rest="${rest#*\}}"
  done
  out+="$(escape_ere_literal "$rest")"
  [[ -z "$action" ]] || out+=":$(escape_ere_literal "$action")"
  term='($|\?)'
  if [[ -n "$suffix" ]]; then
    out+="$(escape_ere_literal "$suffix")$term"
  else
    out+="$term"
  fi
  printf '%s' "$out"
}

fixture_request_field() {
  # $1=method|path|call|auth $2=file -> read only request.$1.  Indentation
  # alone is not a JSON path: response and metadata objects can contain the
  # same key at the same depth.  The canonical fixture shape lets this small
  # scanner use the literal request object boundary without becoming a second
  # general JSON parser.
  local field="$1" file="$2"
  awk -v target="$field" '
    $0 == "  \"request\": {" { in_request = 1; next }
    in_request && ($0 == "  }" || $0 == "  },") { in_request = 0; next }
    in_request {
      prefix = "    \"" target "\": "
      if (index($0, prefix) == 1) {
        value = substr($0, length(prefix) + 1)
        sub(/,$/, "", value)
        if (value ~ /^"([^"\\]|\\["\\\/bfnrt]|\\u[0-9a-fA-F]{4})*"$/) {
          sub(/^"/, "", value)
          sub(/"$/, "", value)
          print value
        }
        exit
      }
    }
  ' "$file"
}

fixture_has_request_headers() {
  # $1=file -> true iff request.headers is an object, not a response/decoy
  # object at the same indentation.
  awk '
    $0 == "  \"request\": {" { in_request = 1; next }
    in_request && ($0 == "  }" || $0 == "  },") { in_request = 0; next }
    in_request && ($0 == "    \"headers\": {" || $0 == "    \"headers\": {}," || $0 == "    \"headers\": {}") { found = 1; exit }
    END { exit(found ? 0 : 1) }
  ' "$1"
}

fixture_has_request_params() {
  # $1=file -> true iff request.params is an object. Explicit null is absent.
  awk '
    $0 == "  \"request\": {" { in_request = 1; next }
    in_request && ($0 == "  }" || $0 == "  },") { in_request = 0; next }
    in_request && ($0 == "    \"params\": {" || $0 == "    \"params\": {}," || $0 == "    \"params\": {}") { found = 1; exit }
    END { exit(found ? 0 : 1) }
  ' "$1"
}

fixture_header_present() {
  # $1=file $2=header-name -> true iff request.headers contains the exact
  # key with a NON-EMPTY JSON string value. Empty inline objects close
  # immediately. Status is 0=present, 2=present but not a JSON string,
  # 3=present and a JSON string but empty (""), 1=key absent/missing
  # header block. An empty value is a bound-but-blank credential: the key
  # declares the auth header and the JSON-string type check passes, yet the
  # value authorizes nothing -- refusing `""` here is what makes this gate
  # and the Go reader's `headers[k] == ""` check agree that an empty auth
  # header is not a real auth header.
  local file="$1" key="$2" status
  status="$(awk -v target="$key" '
    $0 == "  \"request\": {" { in_request = 1; next }
    in_request && ($0 == "  }" || $0 == "  },") { in_request = 0; next }
    in_request && ($0 == "    \"headers\": {") { in_headers = 1; next }
    in_request && ($0 == "    \"headers\": {}" || $0 == "    \"headers\": {},") { in_headers = 0; next }
    in_headers && ($0 == "    }" || $0 == "    },") { in_headers = 0; next }
    in_headers {
      prefix = "      \"" target "\": "
      if (index($0, prefix) == 1) {
        value = substr($0, length(prefix) + 1)
        sub(/,$/, "", value)
        found = 1
        if (value == "\"\"") print "empty"
        else if (value ~ /^"([^"\\]|\\["\\\/bfnrt]|\\u[0-9a-fA-F]{4})+"$/) print "present"
        else print "invalid"
        exit
      }
    }
    END { if (!found) print "missing" }
  ' "$file")"
  case "$status" in
    present) return 0 ;;
    invalid) return 2 ;;
    empty) return 3 ;;
    *) return 1 ;;
  esac
}

fixture_query_param_present() {
  # $1=request.path $2=parameter name -> true iff a non-empty query value is
  # present under exactly that name. The value is never printed.
  local path="$1" key="$2" query pair name value
  [[ "$path" == *\?* ]] || return 1
  query="${path#*\?}"
  while IFS= read -r -d '&' pair; do
    name="${pair%%=*}"
    [[ "$pair" == *=* ]] || continue
    value="${pair#*=}"
    [[ "$name" == "$key" && -n "$value" ]] && return 0
  done <<<"$query&"
  return 1
}

canonical_top_string_value() {
  # $1=key $2=file -> prints a required top-level (two-space-indented)
  # string field's value with its surrounding quotes stripped. Callers
  # must run check_fixture_canonical_form on $2 first: that gate already
  # proved the file has a strictly ascending, duplicate-free key order at
  # every depth and that "capability"/"provider" specifically hold a plain
  # JSON string, so a single anchored match here is unambiguous -- unlike
  # the `grep -m1` this replaces, there is no non-JSON or duplicate-key
  # input left that could make this function's answer disagree with what
  # `encoding/json.Unmarshal` decodes for the same field.
  local key="$1" file="$2"
  awk -v k="\"$key\": " '$0 ~ ("^  " k) { sub("^  " k, ""); sub(/,$/, ""); print; exit }' "$file" \
    | sed -E 's/^"(.*)"$/\1/'
}

flat_value() {
  # $1=key $2=file -> prints the value with surrounding double quotes
  # stripped, or nothing if the key is absent. Callers must run
  # check_canonical_form on $2 first: once that has passed, every line
  # that declares a known key is guaranteed to be in the single canonical
  # "key: value" shape (column 0, immediately followed by ": "), so a key
  # that appears more than once here is a genuine duplicate declaration,
  # not a whitespace variant this reader failed to notice. A key that
  # appears more than once is refused rather than resolved: this
  # hand-authored flat "key: value" shape is not schema-validated YAML,
  # and a real YAML parser applies last-occurrence-wins (see the Go-side
  # parseFlatYAMLAUR019 in tests/integration/AUR-019_test.go), while a
  # naive first-match reader would silently keep validating whatever came
  # first and never see a value appended later in the same file. Picking
  # either convention here would let an appended duplicate override -- or
  # get ignored by -- exactly one of the two readers of this file; failing
  # closed on any duplicate is the only answer both readers agree on.
  local key="$1" file="$2" hits
  hits="$(awk -F': ' -v k="$key" '$0 ~ "^" k ": " { c++ } END { print c+0 }' "$file")"
  case "$hits" in
    0) printf '' ;;
    1)
      awk -F': ' -v k="$key" '$0 ~ "^" k ": " { sub("^" k ": ", ""); print; exit }' "$file" \
        | sed -E 's/^"(.*)"$/\1/'
      ;;
    *) fail undecided-provider "$file has key '$key' duplicated $hits times" ;;
  esac
}

# validated_yaml_files proves each required provider's standards file was
# actually opened and validated, distinctly -- not just that
# required_names has 7 distinct entries (checked above) or that this loop
# ran 7 times. The two could still diverge if two distinct names ever
# mapped to the same slug (case folding is the obvious way); tallying the
# actual paths this loop opened, and requiring that tally to equal
# required_names' count, closes that regardless of how a future name and
# its slug could collide.
declare -A validated_yaml_files=()
declare -A cap_statuses=()

for name in "${required_names[@]}"; do
  slug="$(printf '%s' "$name" | tr '[:upper:]' '[:lower:]')"
  yaml="$standards_dir/$slug.yaml"
  [[ "$yaml" == "standards/providers/$slug.yaml" && "$yaml" != *..* ]] ||
    fail undecided-provider "standards path is not canonical for provider $name: $yaml"
  check_canonical_dir "$standards_dir" 'standards provider directory'
  [[ -f $yaml && ! -L $yaml ]] || fail undecided-provider "standards file absent for provider $name: $yaml"
  [[ -r $yaml ]] || infra "standards file unreadable: $yaml"
  [[ -s $yaml ]] || fail undecided-provider "standards file is empty for provider $name: $yaml"
  check_allowed_bytes "$yaml"
  check_canonical_form "$yaml"
  validated_yaml_files["$yaml"]=1

  # A capability_<name> key this card does not track is invisible to every
  # capability-driven check below (they all iterate required_caps, resolved
  # from capabilities.txt), so a YAML that smuggles "capability_foo:
  # supported" in is accepted unless this reader fails closed on any
  # capability_* key that is not in capabilities.txt -- same typed class as
  # an incomplete standards file (undecided-provider): a provider contract
  # that names a capability the card does not track is not fully decided.
  while IFS= read -r decl_cap_key; do
    [[ -n "$decl_cap_key" ]] || continue
    printf '%s\n' "${required_caps[@]}" | grep -Fxq "${decl_cap_key#capability_}" ||
      fail undecided-provider "$yaml declares capability key '$decl_cap_key' that is not in capabilities.txt"
  done < <(awk -F': ' '$1 ~ /^capability_/ { print $1 }' "$yaml")

  yaml_provider="$(flat_value provider "$yaml")"
  [[ "$yaml_provider" == "$name" ]] ||
    fail undecided-provider "$yaml provider field is '$yaml_provider', want '$name'"

  for key in "${required_flat_keys[@]}"; do
    val="$(flat_value "$key" "$yaml")"
    [[ -n "$val" ]] || fail undecided-provider "$yaml has no $key value"
  done

  # transport declares, explicitly and per-provider, which wire-format
  # fields a fixture for this provider must carry -- see "Wire-format
  # binding" below. It is one of required_flat_keys above, so its presence
  # is already proven; here its *value* is restricted to the two this card
  # knows how to bind. auth_binding and wire_call are each required only
  # for the transport value that names them: a provider cannot leave both
  # unset and rely on the fixture side to decide which checks apply to it,
  # the exact "field absent = not applicable" shape a prior round's review
  # found -- and reproduced -- as a bypass.
  yaml_transport="$(flat_value transport "$yaml")"
  case "$yaml_transport" in
    http | sdk-call) ;;
    *)
      fail undecided-provider \
        "$yaml transport has an unrecognized value: '$yaml_transport' (must be 'http' or 'sdk-call')"
      ;;
  esac

  yaml_auth_binding=''
  yaml_wire_call=''
  yaml_wire_streaming_suffix=''
  yaml_auth_query_param=''
  yaml_auth_evidence=''
  if [[ "$yaml_transport" == http ]]; then
    yaml_auth_binding="$(flat_value auth_binding "$yaml")"
    case "$yaml_auth_binding" in
      header | query-param | signed-request | none) ;;
      *)
        fail undecided-provider \
          "$yaml auth_binding has an unrecognized value: '$yaml_auth_binding' (must be 'header', 'query-param', 'signed-request', or 'none'; required because transport is http)"
        ;;
    esac
    yaml_wire_streaming_suffix="$(flat_value wire_streaming_suffix "$yaml")"
    if [[ "$yaml_auth_binding" == query-param ]]; then
      yaml_auth_query_param="$(flat_value auth_query_param "$yaml")"
      [[ -n "$yaml_auth_query_param" ]] ||
        fail undecided-provider "$yaml has no auth_query_param value (required because auth_binding is query-param)"
    elif [[ "$yaml_auth_binding" == signed-request ]]; then
      yaml_auth_evidence="$(flat_value auth_evidence "$yaml")"
      [[ -n "$yaml_auth_evidence" ]] ||
        fail undecided-provider "$yaml has no auth_evidence value (required because auth_binding is signed-request)"
    fi
  else
    yaml_wire_call="$(flat_value wire_call "$yaml")"
    [[ -n "$yaml_wire_call" ]] ||
      fail undecided-provider "$yaml has no wire_call value (required because transport is sdk-call)"
  fi

  yaml_source="$(flat_value source "$yaml")"
  source_host_re='\(https?://([^)/]+)'
  if [[ "$yaml_source" =~ $source_host_re ]]; then
    host="${BASH_REMATCH[1]}"
    grep -Fxq "$host" "$allow_fixture" ||
      fail unsourced-provider "$yaml source host is not allowlisted: $host"
  else
    fail unsourced-provider "$yaml source is not a [label](url) link"
  fi
  yaml_version="$(flat_value version "$yaml")"
  if [[ ! "$yaml_version" =~ ^v?[0-9]+(\.[0-9]+){1,2}$ && ! "$yaml_version" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
    fail unsourced-provider "$yaml has no versioned source: $yaml_version"
  fi
  yaml_date="$(flat_value date "$yaml")"
  [[ "$yaml_date" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] ||
    fail unsourced-provider "$yaml source date is not ISO 8601: $yaml_date"
  real_calendar_date "$yaml_date" ||
    fail unsourced-provider "$yaml source date is not a real calendar date: $yaml_date"
  check_versioned_source "$yaml_source" "$yaml_version"

  matrix_source="$(awk -F'\t' -v wanted="$name" '$1 == wanted { print $2; exit }' <<<"$data_rows")"
  matrix_version="$(awk -F'\t' -v wanted="$name" '$1 == wanted { print $3; exit }' <<<"$data_rows")"
  matrix_date="$(awk -F'\t' -v wanted="$name" '$1 == wanted { print $4; exit }' <<<"$data_rows")"
  [[ "$matrix_source" == "$yaml_source" ]] ||
    fail undecided-provider "matrix source for $name does not match $yaml source"
  [[ "$matrix_version" == "$yaml_version" ]] ||
    fail undecided-provider "matrix version for $name does not match $yaml version"
  [[ "$matrix_date" == "$yaml_date" ]] ||
    fail undecided-provider "matrix date for $name does not match $yaml date"

  # Read once per provider (not per capability, below): every capability's
  # fixture is checked against the same provider's single wire_method /
  # wire_endpoint / auth_header, so there is no reason to re-extract these
  # three values on every loop iteration.
  yaml_wire_method="$(flat_value wire_method "$yaml")"
  yaml_wire_endpoint="$(flat_value wire_endpoint "$yaml")"
  yaml_wire_content_type="$(flat_value wire_content_type "$yaml")"
  yaml_auth_header="$(flat_value auth_header "$yaml")"
  cap_statuses=()

  for cap in "${required_caps[@]}"; do
    status="$(flat_value "capability_$cap" "$yaml")"
    [[ -n "$status" ]] || fail undecided-provider "$yaml has no capability_$cap status"
    [[ "$status" == "supported" || "$status" == "unsupported" ]] ||
      fail undecided-provider "$yaml capability_$cap has an unrecognized status: $status"
    cap_statuses["$cap"]="$status"
    [[ "$status" == "supported" ]] || continue

    fixture_path="$(flat_value "fixture_$cap" "$yaml")"
    [[ -n "$fixture_path" ]] ||
      fail unfixtured-capability "$yaml marks capability_$cap supported with no fixture_$cap entry"
    expected_fixture_path="$spec_dir/fixtures/$slug/$cap.json"
    [[ "$fixture_path" == "$expected_fixture_path" && "$fixture_path" != *..* && "$fixture_path" != /* ]] ||
      fail unfixtured-capability "$yaml fixture_$cap is not the canonical path $expected_fixture_path: $fixture_path"
    check_canonical_dir "$spec_dir/fixtures/$slug" "fixture provider directory $slug"
    [[ -f "$fixture_path" && ! -L "$fixture_path" ]] ||
      fail unfixtured-capability "$yaml fixture_$cap points at a missing file: $fixture_path"
    [[ -r "$fixture_path" ]] || infra "fixture unreadable: $fixture_path"
    [[ -s "$fixture_path" ]] ||
      fail unfixtured-capability "$yaml fixture_$cap points at an empty file: $fixture_path"

    check_allowed_bytes "$fixture_path"
    check_fixture_canonical_form "$fixture_path"

    fixture_cap="$(canonical_top_string_value capability "$fixture_path")"
    [[ "$fixture_cap" == "$cap" ]] ||
      fail unfixtured-capability "$fixture_path capability field is '$fixture_cap', want '$cap'"
    fixture_provider="$(canonical_top_string_value provider "$fixture_path")"
    [[ "$fixture_provider" == "$slug" ]] ||
      fail unfixtured-capability "$fixture_path provider field is '$fixture_provider', want '$slug'"
    fixture_source="$(canonical_top_string_value source "$fixture_path")"
    [[ "$fixture_source" == "$yaml_source" ]] ||
      fail unfixtured-capability "$fixture_path source does not match $yaml source"
    check_versioned_source "$fixture_source" "$yaml_version"

    # Wire-format binding: "capability"/"provider" above are the fixture's
    # own self-declared strings -- exactly what an unrelated fixture with
    # only those two fields edited would still satisfy. A fixture is only
    # actual evidence for the provider it claims to document if its
    # recorded request also matches that provider's own
    # standards/providers/<slug>.yaml contract, not just its label.
    #
    # Which fields that requires is decided by $yaml_transport, read from
    # this fixture's own provider's standards file above -- never by
    # whether the fixture happens to carry the field. A missing field for
    # the transport this provider declares is a typed failure, not a
    # skipped check: omitting request.method/path/headers from an
    # http-transport fixture used to make this whole block a no-op (an
    # HTTP-shaped body borrowed from a different provider passed as long as
    # those three keys were deleted, not just edited); it no longer does,
    # because "http" now requires them unconditionally, and "sdk-call" now
    # requires its own corresponding request.call/request.params fields
    # unconditionally in exactly the same way -- neither transport has an
    # unchecked field left for a borrowed body to hide in.
    if [[ "$yaml_transport" == http ]]; then
      fixture_method="$(fixture_request_field method "$fixture_path")"
      [[ -n "$fixture_method" ]] ||
        fail unfixtured-capability "$fixture_path has no request.method; $yaml declares transport: http, which requires one"
      [[ "$fixture_method" == "$yaml_wire_method" ]] ||
        fail unfixtured-capability "$fixture_path request method '$fixture_method' does not match $yaml wire_method '$yaml_wire_method'"

      fixture_path_value="$(fixture_request_field path "$fixture_path")"
      [[ -n "$fixture_path_value" ]] ||
        fail unfixtured-capability "$fixture_path has no request.path; $yaml declares transport: http, which requires one"
      cap_suffix=''
      [[ "$cap" == streaming ]] && cap_suffix="$yaml_wire_streaming_suffix"
      yaml_wire_action="$(flat_value "wire_action_$cap" "$yaml")"
      wire_re="$(wire_path_prefix_regex "$yaml_wire_endpoint" "$cap_suffix" "$yaml_wire_action")"
      [[ "$fixture_path_value" =~ $wire_re ]] ||
        fail unfixtured-capability "$fixture_path request path '$fixture_path_value' does not match $yaml wire_endpoint '$yaml_wire_endpoint'"

      case "$yaml_auth_binding" in
        header)
        fixture_has_request_headers "$fixture_path" ||
          fail unfixtured-capability "$fixture_path has no request.headers object; $yaml declares auth_binding: header, which requires one"
        if fixture_header_present "$fixture_path" "$yaml_auth_header"; then
          header_status=0
        else
          header_status=$?
        fi
        if ((header_status == 3)); then
          fail unfixtured-capability "$fixture_path request header '$yaml_auth_header' is empty; must have a non-empty string value, the auth_header $yaml declares"
        elif ((header_status == 2)); then
          fail unfixtured-capability "$fixture_path request header '$yaml_auth_header' must have a JSON string value"
        elif ((header_status != 0)); then
          fail unfixtured-capability "$fixture_path request headers do not include '$yaml_auth_header', the auth_header $yaml declares"
        fi
        ;;
        query-param)
          fixture_query_param_present "$fixture_path_value" "$yaml_auth_query_param" ||
            fail unfixtured-capability "$fixture_path request path lacks the declared query auth parameter '$yaml_auth_query_param'"
          ;;
        signed-request)
          fixture_auth="$(fixture_request_field auth "$fixture_path")"
          [[ -n "$fixture_auth" && "$fixture_auth" == "$yaml_auth_evidence" ]] ||
            fail unfixtured-capability "$fixture_path request auth evidence does not match $yaml auth_evidence"
          ;;
        none) ;;
      esac
    else
      fixture_call="$(fixture_request_field call "$fixture_path")"
      [[ -n "$fixture_call" ]] ||
        fail unfixtured-capability "$fixture_path has no request.call; $yaml declares transport: sdk-call, which requires one"
      [[ "$fixture_call" == "$yaml_wire_call" ]] ||
        fail unfixtured-capability "$fixture_path request call '$fixture_call' does not match $yaml wire_call '$yaml_wire_call'"
      fixture_has_request_params "$fixture_path" ||
        fail unfixtured-capability "$fixture_path has no request.params object; $yaml declares transport: sdk-call, which requires one"
    fi
  done

  expected_wire="transport=$yaml_transport; endpoint=$yaml_wire_endpoint; method=$yaml_wire_method; content_type=$yaml_wire_content_type"
  if [[ "$yaml_transport" == http ]]; then
    expected_auth="scheme=$(flat_value auth_scheme "$yaml"); header=$yaml_auth_header; binding=$yaml_auth_binding; format=$(flat_value auth_format "$yaml")"
  else
    expected_auth="scheme=$(flat_value auth_scheme "$yaml"); header=$yaml_auth_header; binding=sdk-call; format=$(flat_value auth_format "$yaml"); call=$yaml_wire_call"
  fi
  expected_error="envelope=$(flat_value error_envelope "$yaml"); taxonomy=$(flat_value error_taxonomy "$yaml")"
  expected_caps=""
  for cap in streaming tool_use structured_output; do
    [[ -z "$expected_caps" ]] || expected_caps+='; '
    expected_caps+="$cap=${cap_statuses[$cap]}"
  done
  [[ "$(matrix_value "$name" 0)" == "$expected_wire" ]] ||
    fail undecided-provider "matrix wire format for $name does not match $yaml"
  [[ "$(matrix_value "$name" 1)" == "$expected_auth" ]] ||
    fail undecided-provider "matrix auth for $name does not match $yaml"
  [[ "$(matrix_value "$name" 2)" == "$expected_error" ]] ||
    fail undecided-provider "matrix error taxonomy for $name does not match $yaml"
  [[ "$(matrix_value "$name" 3)" == "$expected_caps" ]] ||
    fail undecided-provider "matrix capabilities for $name do not match $yaml"
done

((${#validated_yaml_files[@]} == ${#required_names[@]})) ||
  fail undecided-provider "validated ${#validated_yaml_files[@]} distinct standards files but required exactly ${#required_names[@]}"

printf '{"card":"%s","scenario":"%s","selector":"%s","required_providers":%d,"tracked_capabilities":%d,"result":"pass"}\n' \
  "$card" "$scenario" "$selector" "${#required_names[@]}" "${#required_caps[@]}"
