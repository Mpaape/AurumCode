#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-016'
readonly scenario='AC-001'

selector="${1:-AC-001}"
case "$selector" in
  AC-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

infra() {
  printf '%s/%s/infrastructure: %s\n' "$card" "$scenario" "$1" >&2
  exit 79
}
fail() {
  printf '%s/%s/%s: %s\n' "$card" "$scenario" "$1" "$2" >&2
  exit 1
}
emitted() {
  [[ -n $1 ]] || return 0
  fail "${1%%$'\t'*}" "${1#*$'\t'}"
}

for tool in awk grep sed wc; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing tool: $tool"
done

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." 2>/dev/null && pwd -P)" || infra 'repo root unresolved'
cd -- "$repo_root" || infra 'repo root unreachable'

readonly research='.board/research/quality-standards.md'
readonly standards='standards/quality/characteristics.yaml'
readonly abstention_doc='standards/quality/README.md'
readonly char_fixture='tests/specs/AUR-016/characteristics.txt'
readonly domain_fixture='tests/specs/AUR-016/allowed-source-domains.txt'

# ---------------------------------------------------------------------------
# All five artifacts below are this card's own declared deliverables
# (tests/acceptance/EXIT_CODE_CONVENTION.md: "this card's own declared
# deliverable" -> absence is the behavior under test failing to exist, not
# an infrastructure gap), so every one of these is `fail` (exit 1), never
# `infra` (exit 79).
# ---------------------------------------------------------------------------
for f in "$research" "$standards" "$abstention_doc" "$char_fixture" "$domain_fixture"; do
  [[ -f $f && ! -L $f ]] || fail behavior-missing "required artifact absent: $f"
  [[ -r $f ]] || infra "artifact unreadable: $f"
  [[ -s $f ]] || fail behavior-missing "required artifact is empty: $f"
done

# ---------------------------------------------------------------------------
# Baseline sanity the card's own Given clause names directly: the version
# string and the non-conformity phrase must be literally present in the
# research document. Losing either is `invalid` per the card and must fail
# before any other check runs.
# ---------------------------------------------------------------------------
grep -Fq 'ISO/IEC 25010:2023' "$research" ||
  fail behavior-missing "$research is missing the literal version string ISO/IEC 25010:2023"
grep -Fiq 'does not claim conformity' "$research" ||
  fail behavior-missing "$research is missing the case-insensitive phrase 'does not claim conformity'"

# ---------------------------------------------------------------------------
# The outcome's own vocabulary must be documented, not only implemented:
# the abstention rule doc must state the rule in fixed terms a reader (and
# this lint) can both check independently of the data file below.
# ---------------------------------------------------------------------------
grep -Fq 'not_assessed' "$abstention_doc" ||
  fail behavior-missing "$abstention_doc never states the not_assessed status"
grep -Eiq 'MUST NOT[^.]*happen|invalid evidence' "$abstention_doc" ||
  fail behavior-missing "$abstention_doc does not forbid assigning a score without a wired metric"

# ---------------------------------------------------------------------------
# Required characteristic names and allowlisted source hosts: the single
# source of truth is tests/specs/AUR-016, read once here.
# ---------------------------------------------------------------------------
mapfile -t required_chars < <(grep -Ev '^[[:space:]]*$' "$char_fixture")
((${#required_chars[@]} == 9)) ||
  fail behavior-missing "characteristics fixture declares ${#required_chars[@]} names, want exactly 9"
AURUM_REQUIRED_CHARS="$(printf '%s\n' "${required_chars[@]}")"
AURUM_ALLOWED_DOMAINS="$(grep -Ev '^[[:space:]]*$' "$domain_fixture")"
export AURUM_REQUIRED_CHARS AURUM_ALLOWED_DOMAINS

# ---------------------------------------------------------------------------
# Parse the "## Sources examined" pipe table out of quality-standards.md
# into tab-separated rows: source<TAB>version<TAB>date. Every row must cite
# an allowlisted host, a non-empty version token, and an ISO-8601 date, or
# AC-001 fails with a code naming which cell is defective. This is the
# check MUT-002 (substitute the researched reference for a non-allowlisted
# URL or version-less content) must trip.
# ---------------------------------------------------------------------------
table="$(
  awk '
    BEGIN { in_section = 0; in_table = 0 }
    /^## Sources examined[[:space:]]*$/ { in_section = 1; next }
    in_section && /^## / { in_section = 0 }
    in_section && !in_table && /^\|/ {
      in_table = 1
      next
    }
    in_table && /^\|[[:space:]]*-+/ { next }
    in_table && /^\|/ {
      line = $0
      sub(/^\|[[:space:]]*/, "", line)
      sub(/[[:space:]]*\|[[:space:]]*$/, "", line)
      n = split(line, cols, /[[:space:]]*\|[[:space:]]*/)
      if (n >= 3) {
        # A literal tab embedded in a cell would silently smuggle an extra
        # column into the tab-separated row this prints below, desyncing
        # every downstream tab-split (this row here, and the row-lint pass
        # that re-splits it again on "\t"). Reject it as malformed input
        # instead of reserializing it - the same fail-closed treatment
        # applied to standards/quality/characteristics.yaml records below.
        if (cols[1] ~ /\t/ || cols[2] ~ /\t/ || cols[3] ~ /\t/) {
          print "Sources examined row cell contains a literal tab character, which would desync tab-separated row reserialization: " line > "/dev/stderr"
          exit 2
        }
        print cols[1] "\t" cols[2] "\t" cols[3]
      }
      next
    }
    in_table && !/^\|/ { in_table = 0; in_section = 0 }
  ' "$research"
)" || {
  rc=$?
  ((rc == 2)) && fail malformed-entry "$research Sources examined table has a cell containing a literal tab character"
  infra 'sources table parse failed to run'
}

[[ -n $table ]] || fail behavior-missing "no ## Sources examined table found in $research"

source_row_count="$(printf '%s\n' "$table" | grep -c . || true)"
((source_row_count >= 1)) ||
  fail behavior-missing "## Sources examined table in $research has no data rows"

export AURUM_TABLE_DATA="$table"
verdict="$(
  awk -F'\t' '
    function emit(code, detail) { print code "\t" detail; done = 1; exit 0 }
    function trim(s) { gsub(/^[[:space:]]+|[[:space:]]+$/, "", s); return s }
    BEGIN {
      dom_n = split(ENVIRON["AURUM_ALLOWED_DOMAINS"], doms, "\n")
      for (i = 1; i <= dom_n; i++) if (doms[i] != "") allowed[doms[i]] = 1
      n = split(ENVIRON["AURUM_TABLE_DATA"], lines, "\n")
      for (li = 1; li <= n; li++) {
        line = lines[li]
        if (line == "") continue
        m = split(line, c, "\t")
        if (m < 3) emit("unversioned-source", "sources row has " m " columns, want at least 3: " line)
        src = c[1]; ver = trim(c[2]); dt = trim(c[3])
        # Every [label](url) occurrence in the cell must be checked, not
        # just the first: a second, non-allowlisted link appended (or
        # inserted between) allowlisted ones must still trip
        # unversioned-source. Loop match()/substr() forward over the
        # remaining tail instead of matching once against the whole cell.
        link_found = 0
        rest = src
        while ((lstart = match(rest, /\(https?:\/\/[^)\/]+/)) > 0) {
          link_found = 1
          host = substr(rest, lstart, RLENGTH)
          sub(/^\(https?:\/\//, "", host)
          if (!(host in allowed))
            emit("unversioned-source", "source host is not allowlisted: " host)
          rest = substr(rest, lstart + RLENGTH)
        }
        if (!link_found)
          emit("unversioned-source", "source is not a [label](url) link: " src)
        if (ver !~ /^(Edition [0-9]+|v[0-9]+(\.[0-9]+){0,2}|rev ?[0-9]{4}|post [0-9]{4}-[0-9]{2}-[0-9]{2})$/)
          emit("unversioned-source", "source has no versioned citation: " ver)
        if (dt !~ /^[0-9]{4}-[0-9]{2}-[0-9]{2}$/)
          emit("unversioned-source", "source date is not ISO 8601: " dt)
      }
    }
  ' <<<"$table"
)" || infra 'sources table row lint failed to run'
emitted "$verdict"

# ---------------------------------------------------------------------------
# Parse standards/quality/characteristics.yaml into one tab-separated record
# per characteristic: name<TAB>measurable<TAB>status<TAB>score<TAB>metric
# <TAB>metric_source. The file has a fixed two-space/four-space indent shape
# by construction (standards/quality/README.md documents the contract); no
# YAML library is used because this program runs alone inside a minimal,
# network-denied container.
# ---------------------------------------------------------------------------
records="$(
  awk '
    function flush(   ) {
      if (aborted) return
      if (name != "") {
        print name "\t" measurable "\t" status "\t" score "\t" metric "\t" metric_source
      }
    }
    function value(line,   v) {
      v = line
      sub(/^[^:]*:[[:space:]]*/, "", v)
      gsub(/[[:space:]]+$/, "", v)
      # A literal tab inside a field value would land inside the tab-
      # separated record flush() prints below, injecting an extra column
      # that desyncs every positional field read downstream (c[1]..c[6] in
      # the row-lint pass, and the measured/not_assessed counters at the
      # bottom of this script) - the exact vector that let a fabricated
      # score hide past the score-without-signal check on a
      # measurable:false entry with stdout unchanged from GREEN. Reject it
      # here, fail-closed, before it ever reaches the tab-join.
      if (v ~ /\t/) {
        aborted = 1
        print "characteristics.yaml field contains a literal tab character, which would desync tab-separated record reserialization: " line > "/dev/stderr"
        exit 2
      }
      return v
    }
    BEGIN { name = ""; measurable = ""; status = ""; score = ""; metric = ""; metric_source = ""; aborted = 0 }
    /^  - name: / {
      flush()
      name = value($0); measurable = ""; status = ""; score = ""; metric = ""; metric_source = ""
      next
    }
    /^    measurable: / { measurable = value($0); next }
    /^    status: / { status = value($0); next }
    /^    score: / { score = value($0); next }
    /^    metric: / { metric = value($0); next }
    /^    metric_source: / { metric_source = value($0); next }
    END { flush() }
  ' "$standards"
)" || {
  rc=$?
  ((rc == 2)) && fail malformed-entry "$standards has a field containing a literal tab character"
  infra 'characteristics.yaml parse failed to run'
}

[[ -n $records ]] || fail behavior-missing "no characteristic entries parsed from $standards"

export AURUM_RECORDS="$records"
verdict="$(
  awk -F'\t' '
    function emit(code, detail) { print code "\t" detail; done = 1; exit 0 }
    function isnull(v,   t) {
      # A field counts as null for empty, whitespace-only once trimmed, the
      # literal token `null` in any case (null/NULL/Null/nUlL/...), and the
      # YAML 1.1/1.2 null shorthand `~` (also case/whitespace-normalized,
      # though `~` has no case). Trim first, then lowercase, so leading/
      # trailing blanks and any spelling of null/~ are all caught the same
      # way a truthy-looking but blank cell must not slip past the
      # measurable:true / metric-named checks below.
      t = v
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", t)
      t = tolower(t)
      return (t == "" || t == "null" || t == "~")
    }
    BEGIN {
      req_n = split(ENVIRON["AURUM_REQUIRED_CHARS"], req, "\n")
      for (i = 1; i <= req_n; i++) if (req[i] != "") want[req[i]] = 1
      n = split(ENVIRON["AURUM_RECORDS"], lines, "\n")
      for (li = 1; li <= n; li++) {
        line = lines[li]
        if (line == "") continue
        split(line, c, "\t")
        nm = c[1]; measurable = c[2]; status = c[3]; score = c[4]; metric = c[5]; msrc = c[6]
        if (!(nm in want))
          emit("unknown-characteristic", "characteristics.yaml has an entry outside the fixed nine names: " nm)
        seen[nm]++
        if (seen[nm] > 1)
          emit("duplicate-characteristic", "characteristic cited twice in characteristics.yaml: " nm)
        if (measurable != "true" && measurable != "false")
          emit("malformed-entry", nm ": measurable must be literal true or false, got: " measurable)
        if (status != "measured" && status != "not_assessed")
          emit("malformed-entry", nm ": status must be measured or not_assessed, got: " status)
        # The abstention rule, checked structurally: measurable:false (the
        # only value shipped by this card - see standards/quality/README.md)
        # forces status not_assessed and score null. This is exactly what
        # MUT-001 ("assign a numeric score without an observable signal")
        # must trip: a non-null score on a measurable:false / not_assessed
        # entry.
        if (measurable == "false") {
          if (status != "not_assessed")
            emit("score-without-signal", nm ": measurable is false but status is " status ", not not_assessed")
          if (!isnull(score))
            emit("score-without-signal", nm ": measurable is false but score is not null: " score)
        }
        if (status == "not_assessed" && !isnull(score))
          emit("score-without-signal", nm ": status is not_assessed but score is not null: " score)
        if (status == "measured") {
          if (measurable != "true")
            emit("score-without-signal", nm ": status is measured but measurable is not true")
          if (isnull(metric))
            emit("score-without-signal", nm ": status is measured but metric is null")
          if (isnull(msrc))
            emit("score-without-signal", nm ": status is measured but metric_source is null")
          if (score !~ /^-?[0-9]+(\.[0-9]+)?$/)
            emit("score-without-signal", nm ": status is measured but score is not numeric: " score)
        }
        if (!isnull(metric) && isnull(msrc))
          emit("malformed-entry", nm ": metric is named but metric_source is null")
        # The abstention rule converse, checked structurally: measurable
        # can only be true once a metric is actually named (README.md:
        # measurable true only if a metric is both named AND wired).
        # This is exactly what the r1/r2 finding mutation must trip
        # (measurable: false -> true on an entry left with
        # metric/metric_source/score all null, status still
        # not_assessed), in every spelling of not named that isnull()
        # recognizes: literal null case-insensitively (null/NULL/Null/...),
        # the YAML `~` shorthand, and blank/whitespace-only.
        if (measurable == "true" && isnull(metric))
          emit("malformed-entry", nm ": measurable is true but metric is not named")
        if (isnull(metric) && !isnull(msrc))
          emit("malformed-entry", nm ": metric_source is named but metric is null")
      }
    }
    END {
      if (done) exit 0
      for (a in want) if (!(a in seen))
        emit("missing-characteristic", "required characteristic missing from characteristics.yaml: " a)
    }
  ' <<<"$records"
)" || infra 'characteristics.yaml row lint failed to run'
emitted "$verdict"

measured_count="$(awk -F'\t' '$3 == "measured" { n++ } END { print n + 0 }' <<<"$records")"
not_assessed_count="$(awk -F'\t' '$3 == "not_assessed" { n++ } END { print n + 0 }' <<<"$records")"

printf '{"card":"%s","scenario":"%s","selector":"%s","characteristics":%d,"measured":%d,"not_assessed":%d,"result":"pass"}\n' \
  "$card" "$scenario" "$selector" "${#required_chars[@]}" "$measured_count" "$not_assessed_count"
