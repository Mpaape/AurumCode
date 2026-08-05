#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-017'
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
emitted() {
  [[ -n $1 ]] || return 0
  fail "${1%%$'\t'*}" "${1#*$'\t'}"
}

for tool in awk grep sed sort wc; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing tool: $tool"
done

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." 2>/dev/null && pwd -P)" || infra 'repo root unresolved'
cd -- "$repo_root" || infra 'repo root unreachable'

readonly research='.board/research/interoperability.md'
readonly versions='standards/contracts/versions.yaml'
readonly contracts_readme='standards/contracts/README.md'
readonly formats_fixture='tests/specs/AUR-017/formats.txt'
readonly allow_fixture='tests/specs/AUR-017/allowed-source-domains.txt'

for f in "$research" "$versions" "$contracts_readme" "$formats_fixture" "$allow_fixture"; do
  [[ -f $f && ! -L $f ]] || fail undecided-standard "required artifact absent: $f"
  [[ -r $f ]] || infra "artifact unreadable: $f"
  [[ -s $f ]] || fail undecided-standard "required artifact is empty: $f"
done

# ---------------------------------------------------------------------------
# Baseline sanity: the exact three terms the Outcome/Given clause names
# directly must be present, case-sensitive. Losing any of them is a
# behavioral regression regardless of the richer matrix/version checks below.
# ---------------------------------------------------------------------------
for needle in 'SARIF 2.1.0' 'OpenAI' 'MCP'; do
  grep -Fq "$needle" "$research" || fail undecided-standard "research is missing required term: $needle"
done

# ---------------------------------------------------------------------------
# Required standard names and allowlisted source hosts: the single source of
# truth is tests/specs/AUR-017, read once here.
# ---------------------------------------------------------------------------
mapfile -t required_formats < <(grep -Ev '^[[:space:]]*$' "$formats_fixture")
((${#required_formats[@]} == 4)) ||
  fail undecided-standard "formats fixture declares ${#required_formats[@]} names, want exactly 4"
AURUM_REQUIRED_FORMATS="$(printf '%s\n' "${required_formats[@]}")"
AURUM_ALLOWED_DOMAINS="$(grep -Ev '^[[:space:]]*$' "$allow_fixture")"
export AURUM_REQUIRED_FORMATS AURUM_ALLOWED_DOMAINS

# ---------------------------------------------------------------------------
# Parse the "## Standards matrix" pipe table out of interoperability.md into
# tab-separated rows on stdout: standard<TAB>source<TAB>version<TAB>date
# <TAB>criterion1<TAB>criterion2... The first emitted line is instead the
# tab-joined criteria column names (everything right of "Date"), prefixed
# with a literal "HEADER" marker, mirroring AUR-020's matrix parser.
# ---------------------------------------------------------------------------
table="$(
  awk '
    BEGIN { in_section = 0; in_table = 0; header_done = 0 }
    /^## Standards matrix[[:space:]]*$/ { in_section = 1; next }
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
)" || infra 'standards matrix parse failed to run'

[[ -n $table ]] || fail undecided-standard "no ## Standards matrix table found in $research"

header_line="$(printf '%s\n' "$table" | awk -F'\t' 'NR == 1 && $1 == "HEADER" { sub(/^HEADER\t/, ""); print $0; exit }')"
[[ -n $header_line ]] || fail undecided-standard "standards matrix has no header row"
data_rows="$(printf '%s\n' "$table" | awk 'NR > 1')"
[[ -n $data_rows ]] || fail undecided-standard "standards matrix has no data rows"

criteria_count="$(awk -F'\t' '{ print NF }' <<<"$header_line")"
((criteria_count >= 2)) ||
  fail undecided-standard "standards matrix declares fewer than 2 criteria columns"

# Each required standard resolves to exactly one row; every identity and
# criterion cell is non-empty; version and date are parseable; the source
# link's host is allowlisted.
export AURUM_TABLE_DATA="$data_rows"
verdict="$(
  awk -F'\t' -v ccount="$criteria_count" '
    function emit(code, detail) { print code "\t" detail; done = 1; exit 0 }
    function trim(s) { gsub(/^[[:space:]]+|[[:space:]]+$/, "", s); return s }
    BEGIN {
      req_n = split(ENVIRON["AURUM_REQUIRED_FORMATS"], req, "\n")
      for (i = 1; i <= req_n; i++) if (req[i] != "") want[req[i]] = 1
      dom_n = split(ENVIRON["AURUM_ALLOWED_DOMAINS"], doms, "\n")
      for (i = 1; i <= dom_n; i++) if (doms[i] != "") allowed[doms[i]] = 1
      n = split(ENVIRON["AURUM_TABLE_DATA"], lines, "\n")
      for (li = 1; li <= n; li++) {
        line = lines[li]
        if (line == "") continue
        m = split(line, c, "\t")
        if (m < 4 + ccount)
          emit("undecided-standard", "matrix row has " m " columns, want at least " (4 + ccount) ": " line)
        std = trim(c[1]); src = c[2]; ver = trim(c[3]); dt = trim(c[4])
        if (std in want) {
          seen[std]++
          if (seen[std] > 1) emit("undecided-standard", "standard cited twice in matrix: " std)
          # Source must be a markdown link with an allowlisted host.
          if (match(src, /\(https?:\/\/[^)\/]+/) == 0)
            emit("undecided-standard", "standard " std " source is not a [label](url) link")
          host = substr(src, RSTART, RLENGTH)
          sub(/^\(https?:\/\//, "", host)
          if (!(host in allowed))
            emit("undecided-standard", "standard " std " source host is not allowlisted: " host)
          # Version: semantic-version-like, a JSON-Schema-style draft-date
          # identifier, or an immutable post identifier.
          if (ver !~ /^v?[0-9]+(\.[0-9]+){1,2}$/ && ver !~ /^post [0-9]{6,}$/ && ver !~ /^[0-9]{4}-[0-9]{2}$/)
            emit("undecided-standard", "standard " std " has no versioned source: " ver)
          # Date: ISO 8601 calendar date.
          if (dt !~ /^[0-9]{4}-[0-9]{2}-[0-9]{2}$/)
            emit("undecided-standard", "standard " std " source date is not ISO 8601: " dt)
          for (ci = 0; ci < ccount; ci++) {
            cell = trim(c[5 + ci])
            if (length(cell) < 4)
              emit("undecided-standard", "standard " std " has an empty criterion cell")
          }
        }
      }
    }
    END {
      if (done) exit 0
      for (a in want) if (!(a in seen))
        emit("undecided-standard", "required standard missing from matrix: " a)
    }
  ' <<<"$data_rows"
)" || infra 'standards matrix row lint failed to run'
emitted "$verdict"

# ---------------------------------------------------------------------------
# standards/contracts: for each required standard, the pinned version in
# standards/contracts/versions.yaml must equal the matrix's own Version
# cell, and the format's validator.md plus both fixtures must exist,
# be non-empty, and be resolved by this card's own bounded structural lint
# exactly as standards/contracts/README.md documents: the valid fixture is
# accepted and the invalid fixture is rejected, for the reason its
# validator.md names.
# ---------------------------------------------------------------------------
matrix_version_for() {
  awk -F'\t' -v want="$1" '
    { v = $1; gsub(/^[[:space:]]+|[[:space:]]+$/, "", v); if (v == want) { ver = $3; gsub(/^[[:space:]]+|[[:space:]]+$/, "", ver); print ver; exit } }
  ' <<<"$data_rows"
}

yaml_field() {
  local slug="$1" field="$2"
  awk -v slug="$slug" -v field="$field" '
    BEGIN { insec = 0 }
    $0 ~ "^" slug ":[[:space:]]*$" { insec = 1; next }
    insec && /^[^[:space:]]/ { insec = 0 }
    insec && $0 ~ "^[[:space:]]+" field ":" {
      line = $0
      sub("^[[:space:]]+" field ":[[:space:]]*", "", line)
      gsub(/^"|"[[:space:]]*$/, "", line)
      print line
      exit
    }
  ' "$versions"
}

# Extracts the first JSON string value of "key": "value" in $1, using literal
# substring search (never a regex) so a key such as $schema needs no
# escaping. This is a bounded lint over this card's own authored fixtures,
# not a general JSON parser.
get_json_string() {
  local file="$1" key="$2"
  awk -v k="$key" '
    {
      idx = index($0, "\"" k "\"")
      if (idx == 0) next
      rest = substr($0, idx + length(k) + 2)
      colon = index(rest, ":")
      if (colon == 0) next
      rest = substr(rest, colon + 1)
      gsub(/^[[:space:]]*/, "", rest)
      if (substr(rest, 1, 1) != "\"") next
      rest = substr(rest, 2)
      q = index(rest, "\"")
      if (q == 0) next
      print substr(rest, 1, q - 1)
      exit
    }
  ' "$file"
}

# Every JSON string value of "key": "value" anywhere in $1, one per line.
get_all_json_strings() {
  local file="$1" key="$2"
  awk -v k="$key" '
    {
      line = $0
      while (1) {
        idx = index(line, "\"" k "\"")
        if (idx == 0) break
        rest = substr(line, idx + length(k) + 2)
        colon = index(rest, ":")
        if (colon > 0) {
          rest2 = substr(rest, colon + 1)
          gsub(/^[[:space:]]*/, "", rest2)
          if (substr(rest2, 1, 1) == "\"") {
            rest2 = substr(rest2, 2)
            q = index(rest2, "\"")
            if (q > 0) print substr(rest2, 1, q - 1)
          }
        }
        line = substr(line, idx + length(k) + 2)
      }
    }
  ' "$file"
}

# Prints a reason and nothing else when $1 is not a conforming SARIF
# fixture per standards/contracts/sarif/validator.md; prints nothing when it
# conforms.
check_sarif() {
  local file="$1" want_version="$2" want_schema="$3"
  local v s
  v="$(get_json_string "$file" version)"
  if [[ "$v" != "$want_version" ]]; then
    printf 'version mismatch: got "%s" want "%s"' "$v" "$want_version"
    return
  fi
  s="$(get_json_string "$file" '$schema')"
  if [[ "$s" != "$want_schema" ]]; then
    printf 'schema mismatch: got "%s" want "%s"' "$s" "$want_schema"
    return
  fi
  if ! grep -Fq '"driver"' "$file" || ! grep -Fq '"name"' "$file"; then
    printf 'no non-empty runs[].tool.driver.name'
    return
  fi
}

# Prints a reason and nothing else when $1 is not a conforming unified-diff
# fixture per standards/contracts/unified-diff/validator.md; prints nothing
# when it conforms.
check_unified_diff() {
  local file="$1"
  awk '
    BEGIN { have_old_hdr = 0; have_new_hdr = 0; in_hunk = 0; old_count = 0; new_count = 0; want_old = -1; want_new = -1; bad = "" }
    function checkhunk() {
      if (want_old >= 0 && (old_count != want_old || new_count != want_new))
        bad = "hunk declares old=" want_old " new=" want_new " but body has old=" old_count " new=" new_count
    }
    /^--- a\// { have_old_hdr = 1 }
    /^\+\+\+ b\// { have_new_hdr = 1 }
    /^@@ / {
      checkhunk()
      if (bad != "") { print bad; exit }
      n = split($0, nums, /[^0-9]+/)
      want_old = nums[3] + 0
      want_new = nums[5] + 0
      old_count = 0; new_count = 0
      in_hunk = 1
      next
    }
    in_hunk && /^ / { old_count++; new_count++; next }
    in_hunk && /^-/ { old_count++; next }
    in_hunk && /^\+/ { new_count++; next }
    END {
      if (bad != "") exit
      if (!have_old_hdr) { print "missing --- a/ file header"; exit }
      if (!have_new_hdr) { print "missing +++ b/ file header"; exit }
      if (!in_hunk) { print "no @@ hunk header found"; exit }
      checkhunk()
      if (bad != "") print bad
    }
  ' "$file"
}

# Prints a reason and nothing else when $1 is not a conforming JSON Schema
# fixture per standards/contracts/json-schema/validator.md; prints nothing
# when it conforms.
check_json_schema() {
  local file="$1" want_schema="$2"
  local s bad_type
  s="$(get_json_string "$file" '$schema')"
  if [[ "$s" != "$want_schema" ]]; then
    printf 'schema mismatch: got "%s" want "%s"' "$s" "$want_schema"
    return
  fi
  bad_type="$(get_all_json_strings "$file" type | awk '!/^(object|array|string|number|integer|boolean|null)$/ { print; exit }')"
  if [[ -n $bad_type ]]; then
    printf 'type keyword is not a JSON Schema primitive: "%s"' "$bad_type"
    return
  fi
}

# Prints a reason and nothing else when $1 is not a conforming OpenAPI
# fixture per standards/contracts/openapi/validator.md; prints nothing when
# it conforms.
check_openapi() {
  local file="$1" want_version="$2"
  local v
  v="$(awk -F': *' '/^openapi:/ { v = $2; gsub(/^[[:space:]]+|[[:space:]]+$/, "", v); print v; exit }' "$file")"
  if [[ "$v" != "$want_version" ]]; then
    printf 'openapi version mismatch: got "%s" want "%s"' "$v" "$want_version"
    return
  fi
  if ! grep -Eq '^info:' "$file"; then
    printf 'missing top-level info: block'
    return
  fi
  if ! grep -Eq '^paths:' "$file"; then
    printf 'missing top-level paths: block'
    return
  fi
}

declare -A slug_of=(
  ['SARIF']='sarif'
  ['Unified diff']='unified-diff'
  ['JSON Schema']='json-schema'
  ['OpenAPI']='openapi'
)

for std in "${required_formats[@]}"; do
  slug="${slug_of[$std]:-}"
  [[ -n $slug ]] || fail undecided-standard "no known standards/contracts directory for standard: $std"
  dir="standards/contracts/$slug"

  vfile="$dir/validator.md"
  [[ -f $vfile && -s $vfile ]] || fail undecided-standard "required artifact absent or empty: $vfile"

  yaml_version="$(yaml_field "$slug" version)"
  [[ -n $yaml_version ]] || fail undecided-standard "$versions is missing version for $slug"

  matrix_version="$(matrix_version_for "$std")"
  [[ "$matrix_version" == "$yaml_version" ]] ||
    fail undecided-standard "standard $std matrix version ($matrix_version) diverges from $versions ($yaml_version)"

  case "$slug" in
    sarif)
      valid_f="$dir/fixtures/valid.sarif.json"
      invalid_f="$dir/fixtures/invalid.sarif.json"
      for f in "$valid_f" "$invalid_f"; do
        [[ -f $f && -s $f ]] || fail undecided-standard "required artifact absent or empty: $f"
      done
      want_schema="$(yaml_field sarif schema_uri)"
      [[ -n $want_schema ]] || fail undecided-standard "$versions is missing sarif.schema_uri"
      reason="$(check_sarif "$valid_f" "$yaml_version" "$want_schema")"
      [[ -z $reason ]] || fail undecided-standard "sarif valid fixture rejected by lint: $reason"
      reason="$(check_sarif "$invalid_f" "$yaml_version" "$want_schema")"
      [[ -n $reason ]] || fail undecided-standard "sarif invalid fixture was accepted by lint; it must be rejected"
      ;;
    unified-diff)
      valid_f="$dir/fixtures/valid.patch"
      invalid_f="$dir/fixtures/invalid.patch"
      for f in "$valid_f" "$invalid_f"; do
        [[ -f $f && -s $f ]] || fail undecided-standard "required artifact absent or empty: $f"
      done
      reason="$(check_unified_diff "$valid_f")"
      [[ -z $reason ]] || fail undecided-standard "unified-diff valid fixture rejected by lint: $reason"
      reason="$(check_unified_diff "$invalid_f")"
      [[ -n $reason ]] || fail undecided-standard "unified-diff invalid fixture was accepted by lint; it must be rejected"
      ;;
    json-schema)
      valid_f="$dir/fixtures/valid.schema.json"
      invalid_f="$dir/fixtures/invalid.schema.json"
      for f in "$valid_f" "$invalid_f"; do
        [[ -f $f && -s $f ]] || fail undecided-standard "required artifact absent or empty: $f"
      done
      want_schema="$(yaml_field json-schema schema_uri)"
      [[ -n $want_schema ]] || fail undecided-standard "$versions is missing json-schema.schema_uri"
      reason="$(check_json_schema "$valid_f" "$want_schema")"
      [[ -z $reason ]] || fail undecided-standard "json-schema valid fixture rejected by lint: $reason"
      reason="$(check_json_schema "$invalid_f" "$want_schema")"
      [[ -n $reason ]] || fail undecided-standard "json-schema invalid fixture was accepted by lint; it must be rejected"
      ;;
    openapi)
      valid_f="$dir/fixtures/valid.openapi.yaml"
      invalid_f="$dir/fixtures/invalid.openapi.yaml"
      for f in "$valid_f" "$invalid_f"; do
        [[ -f $f && -s $f ]] || fail undecided-standard "required artifact absent or empty: $f"
      done
      reason="$(check_openapi "$valid_f" "$yaml_version")"
      [[ -z $reason ]] || fail undecided-standard "openapi valid fixture rejected by lint: $reason"
      reason="$(check_openapi "$invalid_f" "$yaml_version")"
      [[ -n $reason ]] || fail undecided-standard "openapi invalid fixture was accepted by lint; it must be rejected"
      ;;
    *)
      infra "unmapped standard slug: $slug"
      ;;
  esac
done

printf '{"card":"%s","scenario":"%s","selector":"%s","standards_fixed":%d,"criteria_columns":%d,"result":"pass"}\n' \
  "$card" "$scenario" "$selector" "${#required_formats[@]}" "$criteria_count"
