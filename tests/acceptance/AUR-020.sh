#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-020'
readonly scenario='AC-001'

selector="${1:-AC-001}"
case "$selector" in
  AC-001|IntegrationAUR020) ;;
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

for tool in awk grep sed sort wc; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing tool: $tool"
done

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." 2>/dev/null && pwd -P)" || infra 'repo root unresolved'
cd -- "$repo_root" || infra 'repo root unreachable'

readonly research='.board/research/memory-design.md'
readonly adr='.board/decisions/ADR-0001-optional-local-memory.md'
readonly limits='standards/memory/limits.yaml'
readonly alt_fixture='tests/specs/AUR-020/alternatives.txt'
readonly allow_fixture='tests/specs/AUR-020/allowed-source-domains.txt'

for f in "$research" "$adr" "$limits" "$alt_fixture" "$allow_fixture"; do
  [[ -f $f && ! -L $f ]] || fail undecided-criterion "required artifact absent: $f"
  [[ -r $f ]] || infra "artifact unreadable: $f"
  [[ -s $f ]] || fail undecided-criterion "required artifact is empty: $f"
done

# ---------------------------------------------------------------------------
# Baseline sanity: the original three modes plus the no-vector-database and
# typed-status invariants still have to be present in the ADR. These are the
# terms the outcome names directly and losing any of them is a behavioral
# regression regardless of the richer matrix/criteria checks below.
# ---------------------------------------------------------------------------
for needle in 'off' 'ephemeral' 'local' '.git/aurumcode/memory.sqlite3' \
  'SQLite' 'FTS5' 'no vector database' 'typed status' 'graph'; do
  grep -Fiq "$needle" "$adr" || fail undecided-criterion "ADR is missing required term: $needle"
done

# ---------------------------------------------------------------------------
# MUT-001 guard: nothing in the researched baseline may make a vector
# database mandatory. This is checked as its own forbidden pattern, not just
# the absence of the positive "no vector database" claim above, so a
# mutation that adds a contradictory mandate without touching that claim
# still fails.
# ---------------------------------------------------------------------------
readonly vecdb_pattern_a='(REQUIRES?|REQUIRED|MANDATORY|MUST (HAVE|USE|INCLUDE|CONTAIN)|NEEDS?)[^.]{0,80}(VECTOR[ -]?(DATABASE|STORE|INDEX)|EMBEDDING[ -]?(DATABASE|STORE|INDEX|SERVICE))'
readonly vecdb_pattern_b='(VECTOR[ -]?(DATABASE|STORE|INDEX)|EMBEDDING[ -]?(DATABASE|STORE|INDEX|SERVICE))[^.]{0,80}(IS|ARE)?[ ]?(REQUIRED|MANDATORY)'
for f in "$adr" "$research" "$limits"; do
  if grep -Eiq "$vecdb_pattern_a" "$f" || grep -Eiq "$vecdb_pattern_b" "$f"; then
    fail vector-database-mandated "$f" mandates a vector/embedding database in the baseline
  fi
done

# ---------------------------------------------------------------------------
# Required alternative names and allowlisted source hosts: the single source
# of truth is tests/specs/AUR-020, read once here.
# ---------------------------------------------------------------------------
mapfile -t required_alts < <(grep -Ev '^[[:space:]]*$' "$alt_fixture")
((${#required_alts[@]} == 5)) ||
  fail undecided-criterion "alternatives fixture declares ${#required_alts[@]} names, want exactly 5"
AURUM_REQUIRED_ALTS="$(printf '%s\n' "${required_alts[@]}")"
AURUM_ALLOWED_DOMAINS="$(grep -Ev '^[[:space:]]*$' "$allow_fixture")"
export AURUM_REQUIRED_ALTS AURUM_ALLOWED_DOMAINS

# ---------------------------------------------------------------------------
# Parse the "## Alternatives matrix" pipe table out of memory-design.md into
# tab-separated rows on stdout: alternative<TAB>source<TAB>version<TAB>date
# <TAB>criterion1<TAB>criterion2... The first emitted line is instead the
# tab-joined criteria column names (everything right of "Date"), prefixed
# with a literal "HEADER" marker, so the caller can split header from data
# without a second pass.
# ---------------------------------------------------------------------------
table="$(
  awk '
    BEGIN { in_section = 0; in_table = 0; header_done = 0 }
    /^## Alternatives matrix[[:space:]]*$/ { in_section = 1; next }
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
)" || infra 'alternatives matrix parse failed to run'

[[ -n $table ]] || fail undecided-criterion "no ## Alternatives matrix table found in $research"

header_line="$(printf '%s\n' "$table" | awk -F'\t' 'NR == 1 && $1 == "HEADER" { sub(/^HEADER\t/, ""); print $0; exit }')"
[[ -n $header_line ]] || fail undecided-criterion "alternatives matrix has no header row"
data_rows="$(printf '%s\n' "$table" | awk 'NR > 1')"
[[ -n $data_rows ]] || fail undecided-criterion "alternatives matrix has no data rows"

criteria_count="$(awk -F'\t' '{ print NF }' <<<"$header_line")"
((criteria_count >= 2)) ||
  fail undecided-criterion "alternatives matrix declares fewer than 2 criteria columns"

# Lower-cased criteria set, one per line, used both to validate matrix rows
# and to bound what the ADR is allowed to invoke.
AURUM_CRITERIA="$(awk -F'\t' '{ for (i = 1; i <= NF; i++) print tolower($i) }' <<<"$header_line")"
export AURUM_CRITERIA

# Each required alternative resolves to exactly one row; every identity and
# criterion cell is non-empty; version and date are parseable; the source
# link's host is allowlisted.
export AURUM_TABLE_DATA="$data_rows"
verdict="$(
  awk -F'\t' -v ccount="$criteria_count" '
    function emit(code, detail) { print code "\t" detail; done = 1; exit 0 }
    function trim(s) { gsub(/^[[:space:]]+|[[:space:]]+$/, "", s); return s }
    BEGIN {
      req_n = split(ENVIRON["AURUM_REQUIRED_ALTS"], req, "\n")
      for (i = 1; i <= req_n; i++) if (req[i] != "") want[req[i]] = 1
      dom_n = split(ENVIRON["AURUM_ALLOWED_DOMAINS"], doms, "\n")
      for (i = 1; i <= dom_n; i++) if (doms[i] != "") allowed[doms[i]] = 1
      n = split(ENVIRON["AURUM_TABLE_DATA"], lines, "\n")
      for (li = 1; li <= n; li++) {
        line = lines[li]
        if (line == "") continue
        m = split(line, c, "\t")
        if (m < 4 + ccount)
          emit("undecided-criterion", "matrix row has " m " columns, want at least " (4 + ccount) ": " line)
        alt = trim(c[1]); src = c[2]; ver = trim(c[3]); dt = trim(c[4])
        if (alt in want) {
          seen[alt]++
          if (seen[alt] > 1) emit("undecided-criterion", "alternative cited twice in matrix: " alt)
          # Source must be a markdown link with an allowlisted host.
          if (match(src, /\(https?:\/\/[^)\/]+/) == 0)
            emit("undecided-criterion", "alternative " alt " source is not a [label](url) link")
          host = substr(src, RSTART, RLENGTH)
          sub(/^\(https?:\/\//, "", host)
          if (!(host in allowed))
            emit("undecided-criterion", "alternative " alt " source host is not allowlisted: " host)
          # Version: semantic-version-like, or an immutable post identifier.
          if (ver !~ /^v?[0-9]+(\.[0-9]+){1,2}$/ && ver !~ /^post [0-9]{6,}$/)
            emit("undecided-criterion", "alternative " alt " has no versioned source: " ver)
          # Date: ISO 8601 calendar date.
          if (dt !~ /^[0-9]{4}-[0-9]{2}-[0-9]{2}$/)
            emit("undecided-criterion", "alternative " alt " source date is not ISO 8601: " dt)
          for (ci = 0; ci < ccount; ci++) {
            cell = trim(c[5 + ci])
            if (length(cell) < 4)
              emit("undecided-criterion", "alternative " alt " has an empty criterion cell")
          }
        }
      }
    }
    END {
      if (done) exit 0
      for (a in want) if (!(a in seen))
        emit("undecided-criterion", "required alternative missing from matrix: " a)
    }
  ' <<<"$data_rows"
)" || infra 'alternatives matrix row lint failed to run'
emitted "$verdict"

# ---------------------------------------------------------------------------
# ADR content: every alternative is named as Rejected, the decision closes
# on SQLite FTS5 plus graph, and every "- Criterion: <name> --- <text>" line
# in the ADR cites a name present in the matrix's own criteria set; a
# Rejected/Decision section missing a justification for any matrix criterion
# also fails here.
# ---------------------------------------------------------------------------
for alt in "${required_alts[@]}"; do
  grep -Fq "### Rejected: ${alt}" "$adr" ||
    fail undecided-criterion "ADR does not name ${alt} as a rejected alternative"
done
grep -Eq '^### Decision: ' "$adr" ||
  fail undecided-criterion 'ADR has no ### Decision: section'
grep -Fiq 'SQLite FTS5' "$adr" ||
  fail undecided-criterion 'ADR decision does not close on SQLite FTS5'

AURUM_ADR_TEXT="$(cat "$adr")"
export AURUM_ADR_TEXT
verdict="$(
  awk '
    function emit(code, detail) { print code "\t" detail; done = 1; exit 0 }
    function trim(s) { gsub(/^[[:space:]]+|[[:space:]]+$/, "", s); return s }
    BEGIN {
      cn = split(ENVIRON["AURUM_CRITERIA"], crit, "\n")
      for (i = 1; i <= cn; i++) if (crit[i] != "") criteria[crit[i]] = 1
      n = split(ENVIRON["AURUM_ADR_TEXT"], lines, "\n")
      section = ""
      for (i = 1; i <= n; i++) {
        line = lines[i]
        if (line ~ /^### Rejected: /) {
          if (section != "") checkcoverage(section)
          section = line; sub(/^### Rejected: /, "", section); delete cited
        } else if (line ~ /^### Decision: /) {
          if (section != "") checkcoverage(section)
          section = "__decision__"; delete cited
        } else if (line ~ /^## / || line ~ /^### /) {
          if (section != "") checkcoverage(section)
          section = ""
        } else if (section != "" && line ~ /^- Criterion: /) {
          rest = line
          sub(/^- Criterion: /, "", rest)
          dash = index(rest, " \342\200\224 ")
          if (dash > 0) label = substr(rest, 1, dash - 1)
          else {
            dash2 = index(rest, " -- ")
            label = (dash2 > 0) ? substr(rest, 1, dash2 - 1) : rest
          }
          label = tolower(trim(label))
          if (!(label in criteria))
            emit("undecided-criterion", "ADR cites a criterion outside the matrix: " label)
          cited[label] = 1
        }
      }
      if (section != "") checkcoverage(section)
    }
    function checkcoverage(name,   c, missing) {
      if (done) return
      for (c in criteria) {
        if (!(c in cited)) {
          missing = c
          emit("undecided-criterion", "\"" name "\" section has no justification for criterion: " missing)
        }
      }
    }
  '
)" || infra 'ADR criteria coverage lint failed to run'
emitted "$verdict"

# ---------------------------------------------------------------------------
# standards/memory limits: an index-size ceiling and a context-window
# ceiling must both be declared with a positive integer value.
# ---------------------------------------------------------------------------
grep -Eq '^index_size:[[:space:]]*$' "$limits" ||
  fail undecided-criterion "$limits has no index_size: section"
grep -Eq '^context_window:[[:space:]]*$' "$limits" ||
  fail undecided-criterion "$limits has no context_window: section"

max_index_bytes="$(awk -F': *' '/^[[:space:]]+max_index_bytes:/ { gsub(/[[:space:]#].*$/, "", $2); print $2; exit }' "$limits")"
[[ $max_index_bytes =~ ^[0-9]+$ ]] && ((max_index_bytes > 0)) ||
  fail undecided-criterion "$limits has no positive max_index_bytes index-size limit"

max_context_tokens="$(awk -F': *' '/^[[:space:]]+max_context_window_tokens:/ { gsub(/[[:space:]#].*$/, "", $2); print $2; exit }' "$limits")"
[[ $max_context_tokens =~ ^[0-9]+$ ]] && ((max_context_tokens > 0)) ||
  fail undecided-criterion "$limits has no positive max_context_window_tokens context-window limit"

printf '{"card":"%s","scenario":"%s","selector":"%s","required_alternatives":%d,"criteria_columns":%d,"result":"pass"}\n' \
  "$card" "$scenario" "$selector" "${#required_alts[@]}" "$criteria_count"
