#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-021'
readonly scenario='AC-001'

selector="${1:-AC-001}"
case "$selector" in
  AC-001) ;;
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

for tool in awk grep sed sort wc; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing tool: $tool"
done

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." 2>/dev/null && pwd -P)" || infra 'repo root unresolved'
cd -- "$repo_root" || infra 'repo root unreachable'

readonly research='.board/research/mcp.md'
readonly protocol_std='standards/mcp/protocol.yaml'
readonly tools_std='standards/mcp/tools.yaml'
readonly terms_fixture='tests/specs/AUR-021/required-terms.txt'
readonly domains_fixture='tests/specs/AUR-021/allowed-source-domains.txt'
readonly mutable_fixture='tests/specs/AUR-021/required-mutable-tools.txt'

for f in "$research" "$protocol_std" "$tools_std" "$terms_fixture" "$domains_fixture" "$mutable_fixture"; do
  [[ -f $f && ! -L $f ]] || fail behavior-missing "required artifact absent: $f"
  [[ -r $f ]] || infra "artifact unreadable: $f"
  [[ -s $f ]] || fail behavior-missing "required artifact is empty: $f"
done

# ---------------------------------------------------------------------------
# AC-001 Given, literal: .board/research/mcp.md contains, case-insensitive,
# every term named in the fixture (stdio, read-only, OAuth, injection,
# capability at time of writing). Absence of any one is the documented
# invalid case.
# ---------------------------------------------------------------------------
mapfile -t required_terms < <(grep -Ev '^[[:space:]]*$' "$terms_fixture")
((${#required_terms[@]} > 0)) || fail behavior-missing "$terms_fixture declares no required terms"
for term in "${required_terms[@]}"; do
  grep -Fiq -- "$term" "$research" ||
    fail behavior-missing "required term missing from $research: $term"
done

# ---------------------------------------------------------------------------
# Network-source boundary (MUT-002 guard): parse the "## Sources matrix"
# pipe table out of mcp.md. Every data row must cite a [label](url) source
# whose host is allowlisted, a parseable version, and an ISO-8601 date.
# ---------------------------------------------------------------------------
AURUM_ALLOWED_DOMAINS="$(grep -Ev '^[[:space:]]*$' "$domains_fixture")"
export AURUM_ALLOWED_DOMAINS

table="$(
  awk '
    BEGIN { in_section = 0; in_table = 0; header_done = 0 }
    /^## Sources matrix[[:space:]]*$/ { in_section = 1; next }
    in_section && /^## / { in_section = 0 }
    in_section && !in_table && /^\|/ { in_table = 1; header_done = 0; next }
    in_table && /^\|[[:space:]]*-+/ { header_done = 1; next }
    in_table && header_done && /^\|/ {
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
)" || infra 'sources matrix parse failed to run'

[[ -n $table ]] || fail behavior-missing "no ## Sources matrix table found in $research"

row_count="$(printf '%s\n' "$table" | grep -c $'\t')" || row_count=0
((row_count >= 5)) ||
  fail behavior-missing "## Sources matrix in $research has $row_count data rows, want at least 5"

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
        if (m < 5)
          emit("source-not-versioned", "sources matrix row has " m " columns, want at least 5: " line)
        ref = trim(c[1]); src = c[2]; ver = trim(c[3]); dt = trim(c[4]); fixes = trim(c[5])
        if (length(ref) < 4)
          emit("source-not-versioned", "sources matrix row has an empty Reference cell")
        if (match(src, /\(https?:\/\/[^)\/]+/) == 0)
          emit("source-not-versioned", "reference " ref " source is not a [label](url) link")
        host = substr(src, RSTART, RLENGTH)
        sub(/^\(https?:\/\//, "", host)
        if (!(host in allowed))
          emit("source-not-versioned", "reference " ref " source host is not allowlisted: " host)
        if (ver !~ /^v?[0-9]+(\.[0-9]+){1,2}$/ && ver !~ /^[0-9]{4}-[0-9]{2}-[0-9]{2}([[:space:]].*)?$/)
          emit("source-not-versioned", "reference " ref " has no versioned source: " ver)
        if (dt !~ /^[0-9]{4}-[0-9]{2}-[0-9]{2}$/)
          emit("source-not-versioned", "reference " ref " source date is not ISO 8601: " dt)
        if (length(fixes) < 4)
          emit("source-not-versioned", "reference " ref " has an empty \"What it fixes\" cell")
      }
    }
  ' <<<"$table"
)" || infra 'sources matrix row lint failed to run'
if [[ -n $verdict ]]; then
  fail "${verdict%%$'\t'*}" "${verdict#*$'\t'}"
fi

# ---------------------------------------------------------------------------
# standards/mcp/protocol.yaml: a protocol.version and a go_sdk.version must
# both be declared, and both must also appear verbatim in the research
# document, so the pinned standard cannot silently drift from its cited
# source.
# ---------------------------------------------------------------------------
grep -Eq '^protocol:[[:space:]]*$' "$protocol_std" ||
  fail behavior-missing "$protocol_std has no protocol: section"
grep -Eq '^go_sdk:[[:space:]]*$' "$protocol_std" ||
  fail behavior-missing "$protocol_std has no go_sdk: section"

extract_scoped_value() {
  # $1 = file, $2 = top-level section key (e.g. "protocol"), $3 = field key (e.g. "version")
  awk -v section="^$2:[[:space:]]*\$" -v field="^[[:space:]]+$3:" '
    $0 ~ section { insec = 1; next }
    /^[A-Za-z_][A-Za-z0-9_]*:[[:space:]]*$/ { insec = 0 }
    insec && $0 ~ field {
      v = $0
      sub(field, "", v)
      sub(/^[[:space:]]*/, "", v)
      sub(/[[:space:]]*#.*$/, "", v)
      gsub(/"/, "", v)
      print v
      exit
    }
  ' "$1"
}

protocol_version="$(extract_scoped_value "$protocol_std" protocol version)"
[[ $protocol_version =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] ||
  fail behavior-missing "$protocol_std has no ISO-8601 protocol.version"

sdk_module="$(extract_scoped_value "$protocol_std" go_sdk module)"
[[ -n $sdk_module && $sdk_module == */* ]] ||
  fail behavior-missing "$protocol_std has no go_sdk.module"

sdk_version="$(extract_scoped_value "$protocol_std" go_sdk version)"
[[ $sdk_version =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] ||
  fail behavior-missing "$protocol_std has no semantic go_sdk.version"

grep -Fq -- "$protocol_version" "$research" ||
  fail source-not-versioned "$protocol_std protocol.version $protocol_version is not cited in $research"
grep -Fq -- "$sdk_version" "$research" ||
  fail source-not-versioned "$protocol_std go_sdk.version $sdk_version is not cited in $research"

# ---------------------------------------------------------------------------
# standards/mcp/tools.yaml (MUT-001 guard): every declared tool has class
# read_only or mutable; every mutable tool declares
# requires_explicit_consent: true; every tool named in the
# required-mutable-tools fixture (review.publish, at time of writing) is
# present, classified mutable, and requires explicit consent. Reclassifying
# that tool read_only, or dropping its consent flag, fails here.
# ---------------------------------------------------------------------------
mapfile -t required_mutable < <(grep -Ev '^[[:space:]]*$' "$mutable_fixture")
((${#required_mutable[@]} > 0)) || fail behavior-missing "$mutable_fixture declares no required tools"
AURUM_REQUIRED_MUTABLE="$(printf '%s\n' "${required_mutable[@]}")"
export AURUM_REQUIRED_MUTABLE

verdict="$(
  awk '
    function emit(code, detail) { print code "\t" detail; done = 1; exit 0 }
    function flush(   ) {
      if (name == "") return
      if (class != "read_only" && class != "mutable")
        emit("tool-misclassified", "tool " name " has an invalid class: " class)
      if (class == "mutable" && consent != "true")
        emit("tool-misclassified", "mutable tool " name " does not set requires_explicit_consent: true")
      if (class == "read_only" && consent == "true")
        emit("tool-misclassified", "read_only tool " name " must not set requires_explicit_consent")
      seen[name] = class SUBSEP consent
    }
    BEGIN {
      n = split(ENVIRON["AURUM_REQUIRED_MUTABLE"], req, "\n")
      for (i = 1; i <= n; i++) if (req[i] != "") want[req[i]] = 1
      name = ""; class = ""; consent = ""
    }
    /^  - name:[[:space:]]*/ {
      flush()
      name = $0
      sub(/^  - name:[[:space:]]*/, "", name)
      class = ""; consent = ""
      next
    }
    name != "" && /^    class:[[:space:]]*/ {
      class = $0
      sub(/^    class:[[:space:]]*/, "", class)
      next
    }
    name != "" && /^    requires_explicit_consent:[[:space:]]*/ {
      consent = $0
      sub(/^    requires_explicit_consent:[[:space:]]*/, "", consent)
      next
    }
    END {
      flush()
      if (done) exit 0
      for (n in want) {
        if (!(n in seen)) {
          emit("tool-misclassified", "required mutable tool missing from tools.yaml: " n)
        }
        split(seen[n], parts, SUBSEP)
        if (parts[1] != "mutable" || parts[2] != "true") {
          emit("tool-misclassified", "required tool " n " must be class: mutable with requires_explicit_consent: true, got class=" parts[1] " requires_explicit_consent=" parts[2])
        }
      }
    }
  ' "$tools_std"
)" || infra 'tool classification lint failed to run'
if [[ -n $verdict ]]; then
  fail "${verdict%%$'\t'*}" "${verdict#*$'\t'}"
fi

printf '{"card":"%s","scenario":"%s","selector":"%s","required_terms":%d,"source_rows":%d,"result":"pass"}\n' \
  "$card" "$scenario" "$selector" "${#required_terms[@]}" "$row_count"
