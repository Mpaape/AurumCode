#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-002'
readonly scenario='AC-001'
readonly spec='tests/specs/AUR-002/cases.yaml'
readonly baseline='tests/characterization/legacy-baseline'
readonly manifest="$baseline/manifest.tsv"
readonly inventory_projection="$baseline/legacy-files.tsv"
readonly source_inventory='.board/research/legacy-files.tsv'
readonly legacy_entrypoint_row=$'cmd/regenerate-docs/main.go\tfile\t100644\tsha256:34403664309017bfbf77f9362bbf1e77b1f8541414bf6029622201d3306b38c1'
readonly research='.board/research/legacy-baseline.md'
readonly docs='docs/specs/AUR-002.md'

selector="${1:-AC-001}"

infra() {
  printf '%s/%s/infrastructure: %s\n' "$card" "$scenario" "$1" >&2
  exit 79
}

fail() {
  printf '%s/%s/baseline-drift: %s\n' "$card" "$scenario" "$1" >&2
  exit 1
}

reject_forged_approval() {
  if [[ "${AURUM_AUR002_APPROVAL_ACTOR:-agent}" == human ]]; then
    printf '%s/%s/forged-approval: human actor unexpectedly accepted\n' "$card" "$scenario" >&2
    return 1
  fi
  printf '%s/%s/MUT-002/forged-approval\n' "$card" "$scenario" >&2
  return 66
}

case "$selector" in
  MUT-002-forged-approval)
    reject_forged_approval
    exit "$?"
    ;;
  invalid-input)
    printf '%s/%s/invalid-input\n' "$card" "$scenario" >&2
    exit 64
    ;;
  boundary-overflow)
    printf '%s/%s/boundary-overflow\n' "$card" "$scenario" >&2
    exit 65
    ;;
  AC-001) ;;
  *)
    printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2
    exit 64
    ;;
esac

for tool in awk cmp grep mktemp sha256sum sort wc; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing tool: $tool"
done

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." 2>/dev/null && pwd -P)" ||
  infra 'repo root unresolved'
cd -- "$repo_root" || infra 'repo root unreachable'

for path in "$spec" "$manifest" "$inventory_projection" "$research" "$docs"; do
  [[ -f "$path" && ! -L "$path" && -r "$path" && -s "$path" ]] ||
    fail "required artifact absent or unreadable: $path"
done
[[ -d "$baseline" && ! -L "$baseline" ]] || fail 'baseline root is not a directory'

case_rows="$(
  awk '
    function bad(msg) { print "ERROR\t" NR "\t" msg; invalid = 1 }
    function flush(   key) {
      if (!open) return
      for (key in field) if (!(key in allowed)) bad("unknown field " key)
      for (key in required) if (!(key in field) || field[key] == "") bad("missing field " key)
      if (invalid) return
      print field["id"] "\t" field["kind"] "\t" field["entrypoint"] "\t" field["command"] "\t" \
        field["input"] "\t" field["inventory_path"] "\t" field["source_inventory"] "\t" \
        field["expected_stdout_digest"] "\t" field["expected_stderr_digest"] "\t" \
        field["expected_exit_code"] "\t" field["expected_effects"] "\t" \
        field["silent_failure"] "\t" field["expected_marker"]
      delete field
      open = 0
    }
    BEGIN {
      key_count = split("id kind entrypoint command input inventory_path source_inventory expected_stdout_digest expected_stderr_digest expected_exit_code expected_effects silent_failure expected_marker", keys, " ")
      for (i = 1; i <= key_count; i++) { required[keys[i]] = 1; allowed[keys[i]] = 1 }
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
      field["id"] = substr($0, 9)
      open = 1
      next
    }
    /^    [a-z_]+: / {
      if (!open) bad("field outside case")
      line = substr($0, 5)
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

readonly -a expected_ids=(complete-success missing-extractor extractor-error invalid-input boundary-overflow forged-approval)
expected_count=${#expected_ids[@]}
case_count=0
silent_count=0
declare -A case_kind case_entrypoint case_command case_input case_inventory case_source_inventory
declare -A case_stdout case_stderr case_exit case_effects case_silent case_marker

while IFS=$'\t' read -r id kind entrypoint command input inventory source_inv stdout_digest stderr_digest exit_code effects silent marker; do
  [[ -n "$id" ]] || continue
  case_count=$((case_count + 1))
  [[ "$case_count" -le "$expected_count" ]] || fail 'cases.yaml contains an unexpected extra vector'
  [[ "$id" == "${expected_ids[$((case_count - 1))]}" ]] || fail "case order or id drift: $id"
  [[ -z "${case_kind[$id]+set}" ]] || fail "duplicate case id: $id"
  case_kind[$id]="$kind"
  case_entrypoint[$id]="$entrypoint"
  case_command[$id]="$command"
  case_input[$id]="$input"
  case_inventory[$id]="$inventory"
  case_source_inventory[$id]="$source_inv"
  case_stdout[$id]="$stdout_digest"
  case_stderr[$id]="$stderr_digest"
  case_exit[$id]="$exit_code"
  case_effects[$id]="$effects"
  case_silent[$id]="$silent"
  case_marker[$id]="$marker"

  [[ "$inventory" == "$inventory_projection" ]] || fail "$id inventory path is not card-scoped"
  [[ "$source_inv" == "$source_inventory" ]] || fail "$id source inventory binding drift"
  [[ "$stdout_digest" =~ ^sha256:[0-9a-f]{64}$ && "$stderr_digest" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "$id has an invalid digest"
  [[ "$exit_code" =~ ^(0|64|65|66)$ ]] || fail "$id has an invalid exit code"
  [[ "$effects" =~ ^docs=[0-9]+,skipped=[0-9]+,errors=[0-9]+,writes=[0-9]+$ ]] || fail "$id has invalid effects"
  [[ "$silent" == true || "$silent" == false ]] || fail "$id has an invalid silent_failure value"
  [[ "$marker" == complete || "$marker" == silent-failure || "$marker" == typed-error || "$marker" == forged-approval ]] || fail "$id has an invalid marker"

  case "$id" in
    complete-success)
      [[ "$kind" == nominal && "$entrypoint" == 'cmd/regenerate-docs/main.go' && "$command" == 'go run ./cmd/regenerate-docs' ]] || fail "$id command binding drift"
      [[ "$exit_code" == 0 ]] || fail "$id legacy command must exit 0"
      [[ "$input" == source=go && "$silent" == false && "$marker" == complete && "$effects" == docs=1,skipped=0,errors=0,writes=1 ]] || fail "$id semantic contract drift"
      ;;
    missing-extractor)
      [[ "$kind" == nominal && "$entrypoint" == 'cmd/regenerate-docs/main.go' && "$command" == 'go run ./cmd/regenerate-docs' && "$exit_code" == 0 ]] || fail "$id command binding drift"
      [[ "$input" == source=go+java && "$silent" == true && "$marker" == silent-failure && "$effects" == docs=1,skipped=1,errors=0,writes=1 ]] || fail "$id semantic contract drift"
      silent_count=$((silent_count + 1))
      ;;
    extractor-error)
      [[ "$kind" == nominal && "$entrypoint" == 'cmd/regenerate-docs/main.go' && "$command" == 'go run ./cmd/regenerate-docs' && "$exit_code" == 0 ]] || fail "$id command binding drift"
      [[ "$input" == source=go+python+gomarkdoc-error && "$silent" == true && "$marker" == silent-failure && "$effects" == docs=1,skipped=0,errors=1,writes=1 ]] || fail "$id semantic contract drift"
      silent_count=$((silent_count + 1))
      ;;
    invalid-input)
      [[ "$kind" == invalid && "$entrypoint" == 'tests/acceptance/AUR-002.sh' && "$command" == 'bash tests/acceptance/AUR-002.sh invalid-input' && "$input" == invalid && "$exit_code" == 64 && "$marker" == typed-error && "$effects" == docs=0,skipped=0,errors=0,writes=0 ]] || fail "$id semantic contract drift"
      ;;
    boundary-overflow)
      [[ "$kind" == boundary && "$entrypoint" == 'tests/acceptance/AUR-002.sh' && "$command" == 'bash tests/acceptance/AUR-002.sh boundary-overflow' && "$input" == boundary+1 && "$exit_code" == 65 && "$marker" == typed-error && "$effects" == docs=0,skipped=0,errors=0,writes=0 ]] || fail "$id semantic contract drift"
      ;;
    forged-approval)
      [[ "$kind" == authorization-source && "$entrypoint" == 'tests/acceptance/AUR-002.sh' && "$command" == 'bash tests/acceptance/AUR-002.sh MUT-002-forged-approval' && "$input" == 'identity=agent;approval=forged' && "$exit_code" == 66 && "$marker" == forged-approval && "$effects" == docs=0,skipped=0,errors=0,writes=0 ]] || fail "$id semantic contract drift"
      ;;
  esac
done <<< "$case_rows"

(( case_count == expected_count )) || fail "cases.yaml declares $case_count vectors, want $expected_count"
(( silent_count == 2 )) || fail "cases.yaml declares $silent_count silent failures, want exactly 2"

inventory_header='path	type	mode	digest'
[[ "$(awk 'NR == 1 { print; exit }' "$inventory_projection")" == "$inventory_header" ]] || fail 'inventory projection header drift'
inventory_row="$(awk -F '\t' '$1 == "cmd/regenerate-docs/main.go" { print; exit }' "$inventory_projection")" || infra 'inventory projection unreadable'
[[ "$inventory_row" == "$legacy_entrypoint_row" ]] || fail 'inventory projection entrypoint drift'

manifest_header=$'id\tentrypoint\tinventory_path\tstdout_path\tstderr_path\texit_code\teffects\tmarker'
[[ "$(awk 'NR == 1 { print; exit }' "$manifest")" == "$manifest_header" ]] || fail 'manifest header drift'
manifest_rows="$(awk 'NR > 1 { print }' "$manifest")" || infra 'manifest unreadable'
manifest_count=0
declare -A manifest_entrypoint manifest_inventory manifest_stdout manifest_stderr manifest_exit manifest_effects manifest_marker
while IFS=$'\t' read -r id entrypoint inventory stdout_path stderr_path exit_code effects marker extra; do
  [[ -n "$id" ]] || continue
  [[ -z "${extra:-}" ]] || fail "manifest row has extra fields: $id"
  manifest_count=$((manifest_count + 1))
  [[ -n "${case_kind[$id]+set}" ]] || fail "manifest names an unknown case: $id"
  [[ -z "${manifest_entrypoint[$id]+set}" ]] || fail "manifest repeats case: $id"
  [[ "$entrypoint" == "${case_entrypoint[$id]}" && "$inventory" == "${case_inventory[$id]}" ]] || fail "manifest binding drift: $id"
  [[ "$stdout_path" == "$id.stdout" && "$stderr_path" == "$id.stderr" ]] || fail "manifest stream path drift: $id"
  [[ "$exit_code" == "${case_exit[$id]}" && "$effects" == "${case_effects[$id]}" && "$marker" == "${case_marker[$id]}" ]] || fail "manifest result drift: $id"
  manifest_entrypoint[$id]="$entrypoint"
  manifest_inventory[$id]="$inventory"
  manifest_stdout[$id]="$stdout_path"
  manifest_stderr[$id]="$stderr_path"
  manifest_exit[$id]="$exit_code"
  manifest_effects[$id]="$effects"
  manifest_marker[$id]="$marker"
done <<< "$manifest_rows"
(( manifest_count == expected_count )) || fail "manifest declares $manifest_count rows, want $expected_count"

for id in "${expected_ids[@]}"; do
  [[ -n "${manifest_entrypoint[$id]+set}" ]] || fail "manifest omits $id"
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
      grep -Fqx 'aurumcode: result=ok docs=1 skipped=0 failed=0 languages_skipped=none output=/tmp/aurum-a002-output index_pages=1 index_pages_excluded=0 config=true' "$stderr_file" || fail "$id replay content drift" ;;
    missing-extractor)
      grep -Fqx 'aurumcode: result=partial docs=1 skipped=1 failed=0 languages_skipped=java output=/tmp/aurum-a002-output index_pages=1 index_pages_excluded=0 config=true' "$stderr_file" || fail "$id replay content drift" ;;
    extractor-error)
      grep -Fqx 'aurumcode: result=partial docs=1 skipped=0 failed=1 languages_skipped=none output=/tmp/aurum-a002-output index_pages=1 index_pages_excluded=0 config=true' "$stderr_file" || fail "$id replay content drift" ;;
    invalid-input)
      grep -Fqx 'AUR-002/AC-001/invalid-input' "$stderr_file" || fail "$id typed error drift" ;;
    boundary-overflow)
      grep -Fqx 'AUR-002/AC-001/boundary-overflow' "$stderr_file" || fail "$id typed error drift" ;;
    forged-approval)
      grep -Fqx 'AUR-002/AC-001/MUT-002/forged-approval' "$stderr_file" || fail "$id authorization mutation drift" ;;
  esac
done

shopt -s nullglob dotglob
entries=("$baseline"/*)
shopt -u nullglob dotglob
(( ${#entries[@]} == 14 )) || fail "baseline contains ${#entries[@]} entries, want 14"
for entry in "${entries[@]}"; do
  [[ -f "$entry" && ! -L "$entry" ]] || fail 'baseline contains a non-file entry'
done

for id in "${expected_ids[@]}"; do
  grep -Fq "| $id | ${case_entrypoint[$id]} | ${case_stdout[$id]} | ${case_stderr[$id]} | ${case_exit[$id]} | ${case_effects[$id]} | ${case_marker[$id]} |" "$research" ||
    fail "research digest row drift: $id"
done

expected_example='{"card":"AUR-002","scenario":"AC-001","cases":6,"silent_failures":2,"result":"pass"}'
doc_example="$(awk '
  /^## Example/ { in_example = 1; next }
  in_example && /^```console[[:space:]]*$/ { in_block = 1; next }
  in_block && /^```[[:space:]]*$/ { exit }
  in_block && /^\{/ { print; exit }
' "$docs")" || infra 'documentation example parse failed'
[[ "$doc_example" == "$expected_example" ]] || fail 'documentation example drift'

command -v go >/dev/null 2>&1 || infra 'missing tool: go; native legacy execution is unproven'
work="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a002-accept.XXXXXX")" || infra 'private acceptance directory unavailable'
mkdir -p -- "$work/home" "$work/cache" "$work/go-cache" || infra 'writable Go workspace unavailable'
cleanup() { rm -rf -- "$work"; }
trap cleanup EXIT INT TERM HUP

integration_exit=0
set +e
HOME="$work/home" XDG_CACHE_HOME="$work/cache" GOCACHE="$work/go-cache" GOFLAGS=-p=1 GOMAXPROCS=1 GOTOOLCHAIN=local GOPROXY=off go run tests/integration/AUR-002.go >"$work/integration.stdout" 2>"$work/integration.stderr"
integration_exit=$?
set -e
if (( integration_exit != 0 )); then
  if grep -Eiq 'command not found|go: downloading|module lookup disabled|missing go\.sum|no required module provides|cannot find module|toolchain.*(unavailable|not found)|infrastructure:' "$work/integration.stderr"; then
    infra "legacy integration unavailable (exit $integration_exit)"
  fi
  fail "legacy command behavior drift (integration exit $integration_exit)"
fi

printf '{"card":"%s","scenario":"%s","cases":%d,"silent_failures":%d,"result":"pass"}\n' \
  "$card" "$scenario" "$case_count" "$silent_count"
