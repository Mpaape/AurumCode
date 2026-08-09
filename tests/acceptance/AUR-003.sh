#!/usr/bin/env bash
set -Eeuo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-003'
readonly scenario='AC-001'
readonly max_vectors=64
readonly max_bytes=$((4 * 1024 * 1024))
readonly max_deadline=30

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

for tool in awk go grep mktemp rm sha256sum wc; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing tool: $tool"
done

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." 2>/dev/null && pwd -P)" ||
  infra 'repository root unresolved'
cd -- "$repo_root" || infra 'repository root unreachable'

readonly schema='.board/schemas/task-spec.schema.json'
readonly acceptance='tests/acceptance/AUR-003.sh'
readonly docs='docs/specs/AUR-003.md'
readonly vectors='tests/specs/AUR-003/cases.yaml'
readonly unit='tests/unit/AUR-003.go'

for path in "$schema" "$acceptance" "$docs" "$vectors" "$unit"; do
  [[ -f "$path" && ! -L "$path" ]] || fail declared-path-absent "required artifact absent: $path"
  [[ -r "$path" ]] || infra "required artifact unreadable: $path"
done
[[ -x "$acceptance" ]] || fail acceptance-not-executable "$acceptance is not executable"

bytes="$(wc -c <"$vectors")" || infra 'vector size could not be read'
((bytes <= max_bytes)) || fail limit-exceeded "vectors exceed 4 MiB: $bytes bytes"

# The vector file intentionally uses a tiny closed YAML subset. Keeping this
# parser independent from the Go loader prevents a loader defect from proving
# its own acceptance test.
verdict="$({
  awk -v max_vectors="$max_vectors" -v max_deadline="$max_deadline" '
    function bad(msg) { print msg; failed = 1 }
    function require_case_field(name) {
      if (!(name in current)) bad("case " case_id " lacks " name)
    }
    function finish_case(   field) {
      if (!case_open) return
      require_case_field("kind")
      require_case_field("input_digest")
      for (field in expected) if (field == "") bad("empty expected field")
      if (!("exit_code" in expected) || !("code" in expected) || !("field" in expected) || !("effects" in expected) || !("artifact_digest" in expected))
        bad("case " case_id " has incomplete expected result")
      if (case_id == "") bad("vector has no id")
      if (substr(current["input_digest"], 1, 7) != "sha256:" || length(current["input_digest"]) != 71 || substr(current["input_digest"], 8) !~ /^[0-9a-f]+$/) bad("case " case_id " has an invalid input digest")
      if (substr(expected["artifact_digest"], 1, 7) != "sha256:" || length(expected["artifact_digest"]) != 71 || substr(expected["artifact_digest"], 8) !~ /^[0-9a-f]+$/) bad("case " case_id " has an invalid artifact digest")
      if (expected["exit_code"] != 0 && expected["exit_code"] != 1) bad("case " case_id " has an invalid exit code")
      if (expected["effects"] != 0) bad("case " case_id " permits an effect")
      if (current["kind"] != "nominal" && current["kind"] != "invalid" && current["kind"] != "boundary") bad("case kind is outside the closed set")
      if (expected["code"] !~ /^[a-z_]+$/) bad("case result code is malformed")
      if (expected["field"] != "none" && expected["field"] !~ /^[A-Za-z0-9:_-]+$/ &&
          expected["field"] !~ /^[A-Za-z0-9:_-]+\[[0-9]+\]$/) bad("case field is malformed")
      printf "V\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", case_id, current["kind"], current["input_digest"], expected["exit_code"], expected["code"], expected["field"], expected["effects"], expected["artifact_digest"]
      seen[case_id]++
      case_open = 0
      in_expected = 0
      delete current
      delete expected
    }
    function key_value(line,   colon, key, value) {
      colon = index(line, ":")
      if (colon < 2) return 0
      key = substr(line, 1, colon - 1)
      value = substr(line, colon + 1)
      sub(/^ /, "", value)
      pair_key = key
      pair_value = value
      return 1
    }
    {
      line = $0
      if (line ~ /\t/ || line ~ /[[:cntrl:]]/ || line ~ /[[:space:]]$/) bad("malformed whitespace on line " NR)
      if (line == "") next
      if (line == "schema: aurum.task-spec-cases") { if (started) bad("duplicate schema"); started = 1; next }
      if (!started) { bad("schema header must be first"); next }
      if (line == "version: 1") { version = 1; next }
      if (line == "limits:") { section = "limits"; next }
      if (line == "vectors:") { finish_case(); section = "vectors"; next }
      if (section == "limits" && line ~ /^  [a-z_]+: [0-9]+$/) {
        if (!key_value(substr(line, 3))) { bad("malformed limit"); next }
        if (pair_key != "max_vectors" && pair_key != "max_bytes" && pair_key != "deadline_seconds") bad("unknown limit " pair_key)
        if (pair_key in limits) bad("duplicate limit " pair_key)
        limits[pair_key] = pair_value
        next
      }
      if (section == "vectors" && line ~ /^  - id: [a-z-]+$/) {
        finish_case()
        case_open = 1
        case_id = line
        sub(/^  - id: /, "", case_id)
        next
      }
      if (case_open && line ~ /^    [a-z_]+:[[:space:]]*.*$/) {
        if (!key_value(substr(line, 5))) { bad("malformed vector field"); next }
        if (pair_key != "kind" && pair_key != "input_digest" && pair_key != "expected") bad("unknown vector field " pair_key)
        if (pair_key == "expected") { in_expected = 1; next }
        if (in_expected) bad("vector field follows expected block")
        if (pair_key in current) bad("duplicate vector field " pair_key)
        current[pair_key] = pair_value
        next
      }
      if (case_open && in_expected && line ~ /^      [a-z_]+: .+$/) {
        if (!key_value(substr(line, 7))) { bad("malformed expected field"); next }
        if (pair_key != "exit_code" && pair_key != "code" && pair_key != "field" && pair_key != "effects" && pair_key != "artifact_digest") bad("unknown expected field " pair_key)
        if (pair_key in expected) bad("duplicate expected field " pair_key)
        expected[pair_key] = pair_value
        next
      }
      bad("unexpected line " NR)
    }
    END {
      finish_case()
      if (!started || version != 1 || section != "vectors") bad("missing schema, version, limits, or vectors")
      if (limits["max_vectors"] != max_vectors) bad("max_vectors must be 64")
      if (limits["max_bytes"] != 4194304) bad("max_bytes must be 4194304")
      if (!(limits["deadline_seconds"] ~ /^[0-9]+$/) || limits["deadline_seconds"] > max_deadline) bad("deadline exceeds 30 seconds")
      if (limits["deadline_seconds"] == "") bad("deadline is missing")
      for (id in seen) if (seen[id] != 1) bad("duplicate vector " id)
      if (seen["nominal"] != 1 || seen["invalid"] != 1 || seen["invalid-path-whitespace"] != 1 || seen["invalid-double-slash"] != 1 || seen["boundary"] != 1 || seen["boundary-overflow"] != 1)
        bad("required nominal, invalid, path, boundary, and first-over-boundary vectors are missing")
      count = 0
      for (id in seen) count += seen[id]
      if (count == 0 || count > max_vectors) bad("vector count is outside 1..64")
      if (failed) exit 1
    }
  ' "$vectors"
} 2>/dev/null)" || fail vectors-invalid "$verdict"

[[ -n "$verdict" ]] || fail vectors-invalid 'no vectors were parsed'

scratch="$(mktemp -d "${TMPDIR:-/tmp}/aur003-accept.XXXXXX")" || infra 'temporary directory unavailable'
trap 'rm -rf -- "$scratch"' EXIT
runner="$scratch/aur003-unit"
go build -o "$runner" "$unit" || infra 'TaskSpec loader compilation failed'
observed=0
vector_count="$(awk '/^  - id: / { count++ } END { print count + 0 }' "$vectors")" || infra 'vector count failed'
while IFS=$'\t' read -r marker vector kind input_digest expected_exit expected_code expected_field expected_effects expected_artifact; do
  [[ $marker == V ]] || fail vectors-invalid 'vector parser emitted an invalid record'
  output_file="$scratch/$vector.out"
  error_file="$scratch/$vector.err"
  set +e
  "$runner" --schema "$schema" --case "$vector" >"$output_file" 2>"$error_file"
  actual_exit=$?
  set -e
  [[ $actual_exit == "$expected_exit" ]] || fail loader-exit-mismatch "vector execution returned an unexpected exit"
  [[ ! -s "$error_file" ]] || fail loader-stderr "vector execution emitted diagnostics"
  actual_output="$(<"$output_file")"
  [[ $expected_field != none ]] || expected_field=''
  expected_json="$(printf '{"card":"AUR-003","scenario":"AC-001","vector":"%s","exit_code":%s,"code":"%s","field":"%s","effects":%s,"input_digest":"%s","artifact_digest":"%s"}' \
    "$vector" "$expected_exit" "$expected_code" "$expected_field" \
    "$expected_effects" "$input_digest" "$expected_artifact")"
  [[ $actual_output == "$expected_json" ]] || fail loader-observation-mismatch "vector output did not match its expected result"
  [[ $actual_output != *AURUM_SECRET_CANARY* ]] || fail secret-leak "loader output contains the canary"
  ((observed += 1))
done <<<"$verdict"
((observed == vector_count)) || fail vector-count-mismatch 'loader did not execute every vector'

schema_digest="sha256:$(sha256sum -- "$schema" | awk '{print $1}')" || infra 'schema digest failed'
printf '{"card":"%s","scenario":"%s","selector":"%s","vectors":%s,"max_vectors":%s,"max_bytes":%s,"deadline_seconds":%s,"schema_digest":"%s","effects":0,"result":"pass"}\n' \
  "$card" "$scenario" "$selector" "$vector_count" "$max_vectors" "$max_bytes" "$(awk -F': ' '/^  deadline_seconds:/ { print $2; exit }' "$vectors")" "$schema_digest"
