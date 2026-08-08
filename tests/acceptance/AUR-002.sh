#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-002'
readonly scenario='AC-001'
readonly spec='tests/specs/AUR-002/cases.yaml'
readonly baseline='tests/characterization/legacy-baseline'
readonly manifest="$baseline/manifest.tsv"
readonly research='.board/research/legacy-baseline.md'
readonly docs='docs/specs/AUR-002.md'

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
  printf '%s/%s/baseline-drift: %s\n' "$card" "$scenario" "$1" >&2
  exit 1
}

for tool in awk cmp grep sha256sum sort wc; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing tool: $tool"
done

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." 2>/dev/null && pwd -P)" ||
  infra 'repo root unresolved'
cd -- "$repo_root" || infra 'repo root unreachable'

for path in "$spec" "$manifest" "$research" "$docs"; do
  [[ -f "$path" && ! -L "$path" && -r "$path" && -s "$path" ]] ||
    fail "required artifact absent or unreadable: $path"
done
[[ -d "$baseline" && ! -L "$baseline" ]] || fail 'baseline root is not a directory'

case_rows="$(
  awk '
    function bad(msg) { print "ERROR\t" NR "\t" msg; invalid = 1; next }
    function flush(   key) {
      if (!open) return
      for (key in field) if (!(key in allowed)) bad("unknown field " key)
      for (key in required) if (!(key in field) || field[key] == "") bad("missing field " key)
      if (invalid) return
      print field["id"] "\t" field["kind"] "\t" field["entrypoint"] "\t" \
        field["command"] "\t" field["input"] "\t" field["expected_stdout_digest"] "\t" \
        field["expected_stderr_digest"] "\t" field["expected_exit_code"] "\t" \
        field["expected_effects"] "\t" field["silent_failure"]
      delete field
      open = 0
    }
    BEGIN {
      required["id"] = 1
      required["kind"] = 1
      required["entrypoint"] = 1
      required["command"] = 1
      required["input"] = 1
      required["expected_stdout_digest"] = 1
      required["expected_stderr_digest"] = 1
      required["expected_exit_code"] = 1
      required["expected_effects"] = 1
      required["silent_failure"] = 1
      for (key in required) allowed[key] = 1
      version = 0
      header = 0
    }
    /^[[:space:]]*$/ || /^[[:space:]]*#/ { next }
    NR == 1 {
      if ($0 != "version: 1") bad("header must be version: 1")
      else version = 1
      next
    }
    NR == 2 {
      if ($0 != "cases:") bad("second line must be cases:")
      else header = 1
      next
    }
    /^  - id: / {
      flush()
      line = $0
      sub(/^  - id: /, "", line)
      field["id"] = line
      open = 1
      next
    }
    /^    [a-z_]+: / {
      if (!open) bad("field outside case")
      line = $0
      sub(/^    /, "", line)
      split(line, parts, ": ")
      key = parts[1]
      value = substr(line, length(key) + 3)
      if (!(key in allowed)) bad("unknown field " key)
      else if (key in field) bad("duplicate field " key)
      else field[key] = value
      next
    }
    { bad("invalid syntax") }
    END {
      flush()
      if (!version || !header || invalid) exit 1
    }
  ' "$spec"
)" || fail 'cases.yaml is not the bounded canonical form'

[[ -n "$case_rows" ]] || fail 'cases.yaml declares no cases'
if printf '%s\n' "$case_rows" | awk -F '\t' '$1 == "ERROR" { exit 1 }'; then
  :
else
  fail 'cases.yaml contains a malformed case'
fi

readonly -a expected_ids=(complete-success missing-extractor extractor-error invalid-input boundary-overflow)
expected_count=${#expected_ids[@]}
case_count=0
silent_count=0
declare -A case_kind case_entrypoint case_command case_input case_stdout case_stderr case_exit case_effects case_silent

while IFS=$'\t' read -r id kind entrypoint command input stdout_digest stderr_digest exit_code effects silent; do
  [[ -n "$id" ]] || continue
  case_count=$((case_count + 1))
  [[ "$case_count" -le "$expected_count" ]] || fail 'cases.yaml contains an unexpected extra vector'
  [[ "$id" == "${expected_ids[$((case_count - 1))]}" ]] || fail "case order or id drift: $id"
  [[ -z "${case_kind[$id]+set}" ]] || fail "duplicate case id: $id"
  case_kind[$id]="$kind"
  case_entrypoint[$id]="$entrypoint"
  case_command[$id]="$command"
  case_input[$id]="$input"
  case_stdout[$id]="$stdout_digest"
  case_stderr[$id]="$stderr_digest"
  case_exit[$id]="$exit_code"
  case_effects[$id]="$effects"
  case_silent[$id]="$silent"

  [[ "$entrypoint" == 'cmd/regenerate-docs/main.go' ]] || fail "$id entrypoint is not the inventoried legacy command"
  [[ "$command" == 'go run ./cmd/regenerate-docs' || "$command" == 'baseline validate invalid' || "$command" == 'baseline validate boundary+1' ]] || fail "$id command is outside the sealed command set"
  [[ "$stdout_digest" =~ ^sha256:[0-9a-f]{64}$ && "$stderr_digest" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "$id has an invalid digest"
  [[ "$exit_code" =~ ^(0|64|65)$ ]] || fail "$id has an invalid exit code"
  [[ "$kind" =~ ^(nominal|invalid|boundary)$ ]] || fail "$id has an invalid kind"
  [[ "$silent" == true || "$silent" == false ]] || fail "$id has an invalid silent_failure value"
  [[ "$effects" =~ ^docs=[0-9]+,skipped=[0-9]+,errors=[0-9]+,writes=[0-9]+$ ]] || fail "$id has invalid effects"

  case "$id" in
    complete-success)
      [[ "$kind" == nominal && "$input" == source=go && "${case_exit[$id]}" == 0 && "$silent" == false && "$effects" == docs=1,skipped=0,errors=0,writes=1 ]] || fail "$id semantic contract drift"
      ;;
    missing-extractor)
      [[ "$kind" == nominal && "$input" == source=go+java && "${case_exit[$id]}" == 0 && "$silent" == true && "$effects" == docs=1,skipped=1,errors=0,writes=1 ]] || fail "$id semantic contract drift"
      silent_count=$((silent_count + 1))
      ;;
    extractor-error)
      [[ "$kind" == nominal && "$input" == source=go+python+gomarkdoc-error && "${case_exit[$id]}" == 0 && "$silent" == true && "$effects" == docs=1,skipped=0,errors=1,writes=1 ]] || fail "$id semantic contract drift"
      silent_count=$((silent_count + 1))
      ;;
    invalid-input)
      [[ "$kind" == invalid && "$input" == invalid && "$command" == 'baseline validate invalid' && "${case_exit[$id]}" == 64 && "$silent" == false && "$effects" == docs=0,skipped=0,errors=0,writes=0 ]] || fail "$id semantic contract drift"
      ;;
    boundary-overflow)
      [[ "$kind" == boundary && "$input" == boundary+1 && "$command" == 'baseline validate boundary+1' && "${case_exit[$id]}" == 65 && "$silent" == false && "$effects" == docs=0,skipped=0,errors=0,writes=0 ]] || fail "$id semantic contract drift"
      ;;
  esac
done <<< "$case_rows"

(( case_count == expected_count )) || fail "cases.yaml declares $case_count vectors, want $expected_count"
(( silent_count == 2 )) || fail "cases.yaml declares $silent_count silent failures, want exactly 2"

manifest_header="id	stdout_path	stderr_path	exit_code	effects	marker"
actual_header="$(awk 'NR == 1 { print; exit }' "$manifest")" || infra 'manifest header unreadable'
[[ "$actual_header" == "$manifest_header" ]] || fail 'manifest header drift'

manifest_rows="$({ awk 'NR > 1 { print }' "$manifest"; })" || infra 'manifest unreadable'
manifest_count=0
declare -A manifest_stdout manifest_stderr manifest_exit manifest_effects manifest_marker
while IFS=$'\t' read -r id stdout_path stderr_path exit_code effects marker extra; do
  [[ -n "$id" ]] || continue
  [[ -z "${extra:-}" ]] || fail "manifest row has extra fields: $id"
  manifest_count=$((manifest_count + 1))
  [[ -n "${case_kind[$id]+set}" ]] || fail "manifest names an unknown case: $id"
  [[ -z "${manifest_stdout[$id]+set}" ]] || fail "manifest repeats case: $id"
  [[ "$stdout_path" == "$id.stdout" && "$stderr_path" == "$id.stderr" ]] || fail "manifest stream path drift: $id"
  [[ "$exit_code" == "${case_exit[$id]}" && "$effects" == "${case_effects[$id]}" ]] || fail "manifest result drift: $id"
  [[ "$marker" == complete || "$marker" == silent-failure || "$marker" == typed-error ]] || fail "manifest marker drift: $id"
  manifest_stdout[$id]="$stdout_path"
  manifest_stderr[$id]="$stderr_path"
  manifest_exit[$id]="$exit_code"
  manifest_effects[$id]="$effects"
  manifest_marker[$id]="$marker"
done <<< "$manifest_rows"
(( manifest_count == expected_count )) || fail "manifest declares $manifest_count rows, want $expected_count"

for id in "${expected_ids[@]}"; do
  [[ -n "${manifest_stdout[$id]+set}" ]] || fail "manifest omits $id"
  case "${case_silent[$id]}:${manifest_marker[$id]}" in
    true:silent-failure) ;;
    false:complete|false:typed-error) ;;
    *) fail "marker mismatch for $id" ;;
  esac

  stdout_file="$baseline/${manifest_stdout[$id]}"
  stderr_file="$baseline/${manifest_stderr[$id]}"
  [[ -f "$stdout_file" && ! -L "$stdout_file" && -r "$stdout_file" ]] || fail "stdout replay missing: $id"
  [[ -f "$stderr_file" && ! -L "$stderr_file" && -r "$stderr_file" ]] || fail "stderr replay missing: $id"
  actual_stdout="sha256:$(sha256sum -- "$stdout_file" | awk '{print $1}')"
  actual_stderr="sha256:$(sha256sum -- "$stderr_file" | awk '{print $1}')"
  [[ "$actual_stdout" == "${case_stdout[$id]}" ]] || fail "stdout digest drift: $id"
  [[ "$actual_stderr" == "${case_stderr[$id]}" ]] || fail "stderr digest drift: $id"

  case "$id" in
    complete-success)
      grep -Fqx 'aurumcode: result=ok docs=1 skipped=0 failed=0 languages_skipped=none output=/tmp/aurum-a002-output index_pages=1 index_pages_excluded=0 config=true' "$stderr_file" || fail "$id replay content drift"
      ;;
    missing-extractor)
      grep -Fqx 'aurumcode: result=partial docs=1 skipped=1 failed=0 languages_skipped=java output=/tmp/aurum-a002-output index_pages=1 index_pages_excluded=0 config=true' "$stderr_file" || fail "$id replay content drift"
      ;;
    extractor-error)
      grep -Fqx 'aurumcode: result=partial docs=1 skipped=0 failed=1 languages_skipped=none output=/tmp/aurum-a002-output index_pages=1 index_pages_excluded=0 config=true' "$stderr_file" || fail "$id replay content drift"
      ;;
    invalid-input)
      grep -Fqx 'AUR-002/AC-001/invalid-input' "$stderr_file" || fail "$id typed error drift"
      ;;
    boundary-overflow)
      grep -Fqx 'AUR-002/AC-001/boundary-overflow' "$stderr_file" || fail "$id typed error drift"
      ;;
  esac
done

shopt -s nullglob dotglob
entries=("$baseline"/*)
shopt -u nullglob dotglob
(( ${#entries[@]} == 11 )) || fail "baseline contains ${#entries[@]} entries, want 11"
for entry in "${entries[@]}"; do
  [[ -f "$entry" && ! -L "$entry" ]] || fail "baseline contains a non-file entry"
done

for id in "${expected_ids[@]}"; do
  grep -Fq "| $id | cmd/regenerate-docs/main.go | ${case_stdout[$id]} | ${case_stderr[$id]} | ${case_exit[$id]} | ${case_effects[$id]} |" "$research" ||
    fail "research digest row drift: $id"
done
grep -Fq 'silent-failure' "$research" || fail 'research note omits silent-failure marker'
grep -Fq 'baseline-drift' "$research" || fail 'research note omits baseline-drift diagnostic'

expected_example="{\"card\":\"AUR-002\",\"scenario\":\"AC-001\",\"cases\":5,\"silent_failures\":2,\"result\":\"pass\"}"
doc_example="$(awk '
  /^## Example/ { in_example = 1; next }
  in_example && /^```console[[:space:]]*$/ { in_block = 1; next }
  in_block && /^```[[:space:]]*$/ { exit }
  in_block && /^\{/ { print; exit }
' "$docs")" || infra 'documentation example parse failed'
[[ "$doc_example" == "$expected_example" ]] || fail 'documentation example drift'

printf '{"card":"%s","scenario":"%s","cases":%d,"silent_failures":%d,"result":"pass"}\n' \
  "$card" "$scenario" "$case_count" "$silent_count"
