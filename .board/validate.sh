#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

board_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd "$board_dir/.." && pwd -P)"
states=(backlog ready doing review done blocked-on-owner)
required_sections=(
  "## Outcome"
  "## Non-goals"
  "## Preconditions"
  "## Postconditions"
  "## Acceptance scenarios"
  "## Public contract"
  "## TDD proof"
  "## Acceptance"
  "## Skeptical mutations"
  "## Security and privacy"
  "## Documentation"
  "## Compatibility, migration, rollback"
  "## Review"
  "## Evidence"
)
failed=0
failure_count=0
max_reported_failures=200
declare -A ids=()
declare -A files=()
declare -A card_states=()
declare -A card_dependencies=()
declare -A card_offices=()
declare -A card_risks=()
declare -A card_titles=()
declare -A card_base_shas=()
declare -A card_spec_digests=()
declare -A visit_state=()
declare -A path_owners=()
declare -A reachability=()
declare -A referenced_requirements=()
declare -A referenced_controls=()
# Specification-by-slogan detectors. A card that reuses another card's Given,
# When, Then, Green or Non-goal verbatim cannot distinguish its own promised
# behavior from its neighbour's, so the acceptance proof stops being falsifiable.
# Owners are accumulated per normalized value and adjudicated after the card loop.
declare -A scenario_given_owners=()
declare -A scenario_when_owners=()
declare -A scenario_then_owners=()
declare -A tdd_green_owners=()
declare -A non_goal_owners=()

fail() {
  failed=1
  failure_count=$((failure_count + 1))
  if (( failure_count <= max_reported_failures )); then
    printf 'board error: %s\n' "$*" >&2
  elif (( failure_count == max_reported_failures + 1 )); then
    printf 'board error: further errors suppressed (limit=%d)\n' "$max_reported_failures" >&2
  fi
}

finish_failures() {
  if (( failed != 0 )); then
    printf 'board invalid: %d error(s)\n' "$failure_count" >&2
    exit 1
  fi
}

trim() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

# Front matter is deliberately parsed only between the first two delimiters.
# A body line cannot spoof a required field.
frontmatter_value() {
  local card="$1"
  local key="$2"
  awk -v key="$key" '
    NR == 1 { if ($0 != "---") exit 2; in_frontmatter=1; next }
    in_frontmatter && $0 == "---" { exit }
    in_frontmatter && index($0, key ":") == 1 {
      count++
      value=substr($0, length(key) + 2)
      sub(/^[[:space:]]+/, "", value)
      print value
    }
    END { if (count != 1) exit 3 }
  ' "$card"
}

frontmatter_count() {
  local card="$1"
  local key="$2"
  awk -v key="$key" '
    NR == 1 { if ($0 != "---") exit 2; in_frontmatter=1; next }
    in_frontmatter && $0 == "---" { print count + 0; exit }
    in_frontmatter && index($0, key ":") == 1 { count++ }
  ' "$card"
}

section_body() {
  local card="$1"
  local section="$2"
  awk -v section="$section" '
    $0 == section { found=1; next }
    found && /^##[[:space:]]/ { exit }
    found { print }
  ' "$card"
}

subsection_body() {
  local card="$1"
  local prefix="$2"
  awk -v prefix="$prefix" '
    index($0, prefix) == 1 { found=1; next }
    found && /^###[[:space:]]/ { exit }
    found && /^##[[:space:]]/ { exit }
    found { print }
  ' "$card"
}

normalize_spec_text() {
  printf '%s' "$1" \
    | tr '[:upper:]' '[:lower:]' \
    | tr -d '`*_' \
    | sed -E 's/AUR-[0-9]{3}/CARDREF/g; s/[[:space:]]+/ /g; s/^ //; s/ $//; s/[.;,]+$//'
}

record_spec_owner() {
  local -n owners="$1"
  local value="$2"
  local id="$3"
  local key
  [[ -n "$value" ]] || return 0
  key="$(normalize_spec_text "$value")"
  # Too short to carry a card-specific claim; length is already policed elsewhere.
  (( ${#key} >= 16 )) || return 0
  if [[ -n "${owners[$key]+x}" ]]; then
    owners[$key]="${owners[$key]} $id"
  else
    owners[$key]="$id"
  fi
}

report_spec_collisions() {
  local -n owners="$1"
  local label="$2"
  local key list count
  for key in "${!owners[@]}"; do
    list="${owners[$key]}"
    count="$(wc -w <<< "$list")"
    (( count > 1 )) || continue
    fail "$label is shared verbatim by $count cards ($(tr ' ' ',' <<< "$list" | sed 's/,$//')): \"${key:0:72}\""
  done
}

is_generic_text() {
  local original="$1"
  local text
  if [[ "$original" =~ (^|[^A-Za-z])(TBD|TODO:|FIXME)([^A-Za-z]|$) ]]; then
    return 0
  fi
  text="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | tr '\n' ' ')"
  [[ "$text" =~ (placeholder|lorem[[:space:]]+ipsum|(^|[^a-z])noop([^a-z]|$)|replace[[:space:]]+every|one[[:space:]]+externally[[:space:]]+observable[[:space:]]+outcome|implementar[[:space:]]+o[[:space:]]+m[ií]nimo|ainda[[:space:]]+n[aã]o[[:space:]]+existe[[:space:]]+ou[[:space:]]+n[aã]o[[:space:]]+[eé][[:space:]]+imposto|unit[[:space:]]+e[[:space:]]+contract[[:space:]]+sempre|state[[:space:]]+which[[:space:]]+layers[[:space:]]+apply|exact[[:space:]]+failing[[:space:]]+test|exact[[:space:]]+reversible[[:space:]]+change|path,[[:space:]]+schema,[[:space:]]+endpoint|fazer[[:space:]]+funcionar|comportamento[[:space:]]+desejado|delet(e|ar)[[:space:]]+(the[[:space:]]+)?test|quebrar[[:space:]]+(a[[:space:]]+)?compila|break(ing)?[[:space:]]+compil|infrastructure[[:space:]]+failure|falha[[:space:]]+de[[:space:]]+infra) ]]
}

require_meaningful_section() {
  local card="$1"
  local section="$2"
  local body compact
  body="$(section_body "$card" "$section")"
  compact="$(printf '%s' "$body" | tr -d '[:space:]#*`_-')"
  if (( ${#compact} < 24 )); then
    fail "$card: $section must contain a specific, falsifiable statement"
  elif is_generic_text "$body"; then
    fail "$card: $section contains generic or placeholder language"
  fi
}

parse_list() {
  local value="$1"
  [[ "$value" =~ ^\[(.*)\]$ ]] || return 1
  value="${BASH_REMATCH[1]}"
  [[ -n "$value" ]] || return 0
  local old_ifs="$IFS"
  IFS=','
  read -ra parsed_list <<< "$value"
  IFS="$old_ifs"
  local item
  for item in "${parsed_list[@]}"; do
    item="$(trim "$item")"
    [[ -n "$item" ]] || return 1
    printf '%s\n' "$item"
  done
}

safe_repo_path() {
  local path="$1"
  local without_spaces
  [[ -n "$path" && "$path" != "." && "$path" != "/" ]] || return 1
  [[ "$path" != ' '* && "$path" != *' ' ]] || return 1
  [[ "$path" != *$'\t'* && "$path" != *$'\r'* && "$path" != *$'\n'* ]] || return 1
  [[ "$path" != /* && "$path" != ~* && "$path" != *\\* ]] || return 1
  without_spaces="${path// /}"
  [[ "$without_spaces" =~ ^[A-Za-z0-9._][A-Za-z0-9._/-]*$ ]] || return 1
  [[ "$path" != */ && "$path" != *//* ]] || return 1
  [[ "/$path/" != */../* && "/$path/" != */./* ]] || return 1
  [[ "$path" != *'$'* && "$path" != *'`'* && "$path" != *'*'* && "$path" != *'?'* && "$path" != *'['* ]] || return 1
}

canonical_repo_path_list() {
  local value="$1"
  local allow_empty="${2:-false}"
  local body remainder
  [[ "$value" =~ ^\[(.*)\]$ ]] || return 1
  body="${BASH_REMATCH[1]}"
  if [[ -z "$body" ]]; then
    [[ "$allow_empty" == true ]]
    return
  fi
  [[ "$body" != ' '* && "$body" != *' ' ]] || return 1
  [[ "$body" != *' ,'* && "$body" != *',  '* ]] || return 1
  # Every comma is the list delimiter and is followed by exactly one space.
  remainder="$body"
  while [[ "$remainder" == *,* ]]; do
    remainder="${remainder#*,}"
    [[ "$remainder" == ' '* && "$remainder" != '  '* ]] || return 1
    remainder="${remainder# }"
  done
  [[ "$body" != *$'\t'* && "$body" != *$'\r'* && "$body" != *$'\n'* ]]
}

paths_overlap() {
  local first="$1"
  local second="$2"
  [[ "$first" == "$second" || "$first" == "$second/"* || "$second" == "$first/"* ]]
}

# Ownership is directional. A card owning `tests/acceptance` owns a file below
# it; a card owning `tests/acceptance/AUR-001.sh/subdir` does not own the
# ancestor `tests/acceptance/AUR-001.sh`.
path_is_within() {
  local child="$1"
  local owner="$2"
  [[ "$child" == "$owner" || "$child" == "$owner/"* ]]
}

validate_owned_test_path() {
  local card="$1"
  local label="$2"
  local test_path="$3"
  shift 3
  local owner owned=0

  safe_repo_path "$test_path" || {
    fail "$card: $label test path is unsafe or not repository-relative: $test_path"
    return
  }
  [[ "$test_path" == tests/* || "$test_path" == *_test.go ]] ||
    fail "$card: $label test artifact must be under tests/ or name an owned _test.go file: $test_path"
  for owner in "$@"; do
    if path_is_within "$test_path" "$owner"; then
      owned=1
      break
    fi
  done
  (( owned == 1 )) || fail "$card: $label TDD test is outside owned paths: $test_path"
}

validate_tdd_layer() {
  local card="$1"
  local layer="$2"
  local tdd="$3"
  shift 3
  local line count test_path selector reason

  count="$(grep -Ec "^- $layer: .+" <<< "$tdd" || true)"
  [[ "$count" -eq 1 ]] || {
    fail "$card: TDD proof must specify $layer exactly once"
    return
  }
  line="$(grep -E "^- $layer: .+" <<< "$tdd")"
  if [[ "$line" =~ ^-[[:space:]]$layer:[[:space:]]not-applicable:[[:space:]](.{16,})$ ]]; then
    reason="${BASH_REMATCH[1]}"
    [[ "$reason" != *'`'* ]] || fail "$card: $layer not-applicable must not cite a test artifact"
    return
  fi
  if [[ "$line" =~ ^-[[:space:]]$layer:[[:space:]]\`(.+)::([A-Za-z0-9_./:-]+)\`\.$ ]]; then
    test_path="${BASH_REMATCH[1]}"
    selector="${BASH_REMATCH[2]}"
    [[ -n "$selector" ]] || fail "$card: $layer test selector is empty"
    validate_owned_test_path "$card" "$layer" "$test_path" "$@"
    return
  fi
  fail "$card: $layer must be exactly a backtick-delimited path::selector or not-applicable with a concrete reason"
}

validate_compose_file() {
  local card="$1"
  local compose_path="$2"
  local compose_file="$repo_root/$compose_path"
  local service_count network_none_count read_only_count pids_count memory_count cpu_count user_count
  local cap_drop_count cap_all_count nnp_count

  safe_repo_path "$compose_path" || {
    fail "$card: compose file path is not repository-relative and bounded: $compose_path"
    return
  }
  [[ -f "$compose_file" && ! -L "$compose_file" ]] || {
    fail "$card: compose file is missing or a symlink: $compose_path"
    return
  }
  if grep -Eiq '^[[:space:]]*(privileged|pid|ipc|cap_add|devices|build|extends|include|env_file|secrets|configs)[[:space:]]*:' "$compose_file"; then
    fail "$card: compose acceptance contains a forbidden privilege, namespace, device, or local build directive"
  fi
  if grep -Eiq '^[[:space:]]*network_mode[[:space:]]*:[[:space:]]*(host|service:|container:)' "$compose_file" || grep -Eiq '^[[:space:]]*networks?[[:space:]]*:' "$compose_file"; then
    fail "$card: compose acceptance contains a host/shared/custom network"
  fi
  if grep -Eiq '(docker\.sock|podman\.sock|containerd\.sock|^[[:space:]]*-[[:space:]]*(/|\.|~|\$\{?PWD)|type[[:space:]]*:[[:space:]]*bind|source[[:space:]]*:[[:space:]]*(/|\.|~|\$))' "$compose_file"; then
    fail "$card: compose acceptance contains a host bind/socket mount"
  fi
  service_count="$(grep -Ec '^[[:space:]]*image[[:space:]]*:' "$compose_file" || true)"
  (( service_count > 0 )) || fail "$card: compose acceptance must declare at least one pinned image"
  if grep -Eq '^[[:space:]]*image[[:space:]]*:' "$compose_file" && grep -Ev '^[[:space:]]*(#|$)' "$compose_file" | grep -E '^[[:space:]]*image[[:space:]]*:' | grep -Evq '@sha256:[0-9a-f]{64}[[:space:]]*$'; then
    fail "$card: every compose image must be digest-pinned"
  fi
  network_none_count="$(grep -Eic "^[[:space:]]*network_mode[[:space:]]*:[[:space:]]*['\"]?none['\"]?[[:space:]]*$" "$compose_file" || true)"
  read_only_count="$(grep -Eic '^[[:space:]]*read_only[[:space:]]*:[[:space:]]*true[[:space:]]*$' "$compose_file" || true)"
  pids_count="$(grep -Eic '^[[:space:]]*pids_limit[[:space:]]*:[[:space:]]*[0-9]+[[:space:]]*$' "$compose_file" || true)"
  memory_count="$(grep -Eic '^[[:space:]]*mem_limit[[:space:]]*:[[:space:]]*[^[:space:]]+[[:space:]]*$' "$compose_file" || true)"
  cpu_count="$(grep -Eic '^[[:space:]]*cpus[[:space:]]*:[[:space:]]*[^[:space:]]+[[:space:]]*$' "$compose_file" || true)"
  user_count="$(grep -Eic "^[[:space:]]*user[[:space:]]*:[[:space:]]*['\"]?[1-9][0-9]*(:[1-9][0-9]*)?['\"]?[[:space:]]*$" "$compose_file" || true)"
  (( network_none_count >= service_count )) || fail "$card: every compose service must deny network"
  (( read_only_count >= service_count )) || fail "$card: every compose service must use a read-only rootfs"
  (( pids_count >= service_count )) || fail "$card: every compose service must bound PIDs"
  (( memory_count >= service_count )) || fail "$card: every compose service must bound memory"
  (( cpu_count >= service_count )) || fail "$card: every compose service must bound CPUs"
  (( user_count >= service_count )) || fail "$card: every compose service must select a non-root numeric user"
  cap_drop_count="$(grep -Eic '^[[:space:]]*cap_drop[[:space:]]*:' "$compose_file" || true)"
  cap_all_count="$(grep -Eic "^[[:space:]]*-[[:space:]]*['\"]?ALL['\"]?[[:space:]]*$" "$compose_file" || true)"
  nnp_count="$(grep -Eic 'no-new-privileges' "$compose_file" || true)"
  (( cap_drop_count >= service_count && cap_all_count >= service_count )) || fail "$card: every compose service must drop ALL capabilities"
  (( nnp_count >= service_count )) || fail "$card: every compose service must deny new privileges"
}

validate_acceptance() {
  local card="$1"
  local id="$2"
  local count accept_line command compose_path profile declared_profile volume_count mount_count compose_file_count
  count="$(grep -Ec '^accept: `[^`]+`$' "$card" || true)"
  [[ "$count" -eq 1 ]] || {
    fail "$card: acceptance must be exactly one backtick-delimited command"
    return
  }
  accept_line="$(grep -E '^accept: `[^`]+`$' "$card")"
  command="${accept_line#accept: \`}"
  command="${command%\`}"

  [[ "$command" == *"$id"* ]] || fail "$card: acceptance does not bind to its card id"
  [[ "$command" != *':latest'* ]] || fail "$card: acceptance uses a mutable latest image"
  if [[ "$command" =~ (\||\;|\&\&|\|\||\>\>?|\<\<|\$\(|\`|\\|[[:space:]]--privileged([[:space:]]|$)|--network(=|[[:space:]])host|--pid(=|[[:space:]])host|--ipc(=|[[:space:]])host|--userns(=|[[:space:]])host|--cap-add|--device|docker\.sock|podman\.sock|containerd\.sock|--env-file|--project-directory|(^|[[:space:]])(-e|--env)(=|[[:space:]])) ]]; then
    fail "$card: acceptance contains shell composition, privilege escalation, host namespace/socket, or environment injection"
  fi

  declared_profile="$(grep -E '^container_profile: `[^`]+`$' "$card" | sed -E 's/^container_profile: `([^`]+)`$/\1/' || true)"
  [[ "$declared_profile" =~ ^[a-z0-9][a-z0-9._-]{2,63}$ ]] || fail "$card: container_profile must name one versioned bounded profile"

  if [[ "$command" =~ ^\./\.board/bin/oci-run[[:space:]]+--profile[[:space:]]+([a-z0-9][a-z0-9._-]{2,63})[[:space:]]+--card[[:space:]]+$id$ ]]; then
    profile="${BASH_REMATCH[1]}"
    [[ "$profile" == "$declared_profile" ]] || fail "$card: acceptance wrapper profile differs from container_profile"
    return
  fi

  if [[ "$command" =~ ^(docker|podman)[[:space:]]+run[[:space:]] ]]; then
    [[ "$command" =~ ^(docker|podman)[[:space:]]+run[[:space:]]+--rm([[:space:]]|$) || "$command" == *' --rm '* ]] || fail "$card: direct OCI run must be ephemeral (--rm)"
    [[ "$command" =~ (^|[[:space:]])[^[:space:]]+@sha256:[0-9a-f]{64}([[:space:]]|$) ]] || fail "$card: direct OCI image is not digest-pinned"
    [[ "$command" =~ (^|[[:space:]])--network=none([[:space:]]|$) ]] || fail "$card: direct OCI run must deny network"
    [[ "$command" =~ (^|[[:space:]])--read-only([[:space:]]|$) ]] || fail "$card: direct OCI run must use a read-only rootfs"
    [[ "$command" =~ (^|[[:space:]])--cap-drop=ALL([[:space:]]|$) ]] || fail "$card: direct OCI run must drop all capabilities"
    [[ "$command" =~ (^|[[:space:]])--security-opt=no-new-privileges([[:space:]]|$) ]] || fail "$card: direct OCI run must deny new privileges"
    [[ "$command" =~ --pids-limit(=|[[:space:]])[0-9]+ ]] || fail "$card: direct OCI run must bound PIDs"
    [[ "$command" =~ --memory(=|[[:space:]])[^[:space:]]+ ]] || fail "$card: direct OCI run must bound memory"
    [[ "$command" =~ --cpus(=|[[:space:]])[^[:space:]]+ ]] || fail "$card: direct OCI run must bound CPUs"
    [[ "$command" =~ --user(=|[[:space:]])[1-9][0-9]*(:[1-9][0-9]*)? ]] || fail "$card: direct OCI run must select a non-root numeric user"
    [[ "$command" != *'seccomp=unconfined'* && "$command" != *'apparmor=unconfined'* && "$command" != *'label=disable'* ]] || fail "$card: direct OCI run disables a mandatory security policy"
    if [[ "$command" == *' -v '* || "$command" == *' --volume '* || "$command" == *'--volume='* ]]; then
      [[ "$command" =~ -v[[:space:]]+\"?\$\{?PWD\}?\"?:/src:ro([[:space:]]|$) ]] || fail "$card: direct OCI run may bind only PWD to /src read-only"
      volume_count="$(grep -oE '(^|[[:space:]])(-v|--volume(=|[[:space:]]))' <<< "$command" | wc -l | tr -d ' ')"
      [[ "$volume_count" -eq 1 ]] || fail "$card: direct OCI run must not contain additional volume mounts"
    fi
    if [[ "$command" == *'--mount'* ]]; then
      [[ "$command" =~ --mount[[:space:]]+type=bind,src=\"?\$\{?PWD\}?\"?,dst=/src,readonly([[:space:]]|$) ]] || fail "$card: direct OCI --mount may bind only PWD to /src read-only"
      mount_count="$(grep -oE '(^|[[:space:]])--mount(=|[[:space:]])' <<< "$command" | wc -l | tr -d ' ')"
      [[ "$mount_count" -eq 1 ]] || fail "$card: direct OCI run must not contain additional mounts"
    fi
    return
  fi

  if [[ "$command" =~ ^(docker|podman)[[:space:]]+compose[[:space:]] ]]; then
    [[ "$command" != *' -v '* && "$command" != *'--volume'* ]] || fail "$card: compose command must not add host volumes"
    [[ "$command" =~ [[:space:]]run[[:space:]]+--rm([[:space:]]|$) || "$command" =~ [[:space:]]up[[:space:]].*--abort-on-container-exit.*--exit-code-from ]] || fail "$card: compose acceptance must use an ephemeral run or a bounded up with decisive exit code"
    compose_file_count="$(grep -oE '(^|[[:space:]])(-f([[:space:]]|=)|--file(=|[[:space:]]))' <<< "$command" | wc -l | tr -d ' ')"
    [[ "$compose_file_count" -eq 1 ]] || fail "$card: compose acceptance must use exactly one explicit file"
    if [[ "$command" =~ (^|[[:space:]])--file=([A-Za-z0-9._/-]+)([[:space:]]|$) ]]; then
      compose_path="${BASH_REMATCH[2]}"
    elif [[ "$command" =~ (^|[[:space:]])-f[[:space:]]+([A-Za-z0-9._/-]+)([[:space:]]|$) ]]; then
      compose_path="${BASH_REMATCH[2]}"
    else
      fail "$card: compose acceptance must name one explicit repository-relative file"
      return
    fi
    validate_compose_file "$card" "$compose_path"
    return
  fi

  fail "$card: acceptance must use the repository acceptance wrapper or a hardened Docker/Podman command"
}

# Minimal dependency-free JSON parser. It validates syntax, rejects duplicate
# object keys, and emits path/type/value triples for schema checks below.
json_flatten() {
  local file="$1"
  awk '
    function die(message) { if (!bad) print "json error at byte " pos ": " message > "/dev/stderr"; bad=1 }
    function ws(   c) { while (pos <= length(src)) { c=substr(src,pos,1); if (c ~ /[ \t\r\n]/) pos++; else break } }
    function string(   out,c,e,h) {
      if (substr(src,pos,1) != "\"") { die("expected string"); return "" }
      pos++
      while (pos <= length(src)) {
        c=substr(src,pos++,1)
        if (c == "\"") return out
        if (c == "\\") {
          if (pos > length(src)) { die("unfinished escape"); return "" }
          e=substr(src,pos++,1)
          if (e == "u") {
            h=substr(src,pos,4)
            if (length(h) != 4 || h !~ /^[0-9A-Fa-f]{4}$/) { die("invalid unicode escape"); return "" }
            pos += 4; out=out "?"
          } else if (e ~ /^["\\\/bfnrt]$/) {
            out=out "?"
          } else { die("invalid escape"); return "" }
        } else {
          if (c ~ /[\001-\037]/) { die("control character in string"); return "" }
          out=out c
        }
      }
      die("unterminated string"); return ""
    }
    function value(path,   c,v,start) {
      ws(); c=substr(src,pos,1)
      if (c == "{") { print path "\tobject\t"; object(path); return }
      if (c == "[") { print path "\tarray\t"; array(path); return }
      if (c == "\"") { v=string(); print path "\tstring\t" v; return }
      if (substr(src,pos,4) == "true") { pos+=4; print path "\tbool\ttrue"; return }
      if (substr(src,pos,5) == "false") { pos+=5; print path "\tbool\tfalse"; return }
      if (substr(src,pos,4) == "null") { pos+=4; print path "\tnull\tnull"; return }
      start=pos
      while (substr(src,pos,1) ~ /[0-9eE+.-]/) pos++
      v=substr(src,start,pos-start)
      if (v ~ /^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$/) { print path "\tnumber\t" v; return }
      die("invalid value")
    }
    function object(path,   key,child,c) {
      pos++; ws()
      if (substr(src,pos,1) == "}") { pos++; return }
      while (!bad) {
        ws(); key=string(); ws()
        if (substr(src,pos,1) != ":") { die("expected colon"); return }
        pos++; child=(path == "" ? key : path "." key)
        if (seen[child]++) { die("duplicate object key " child); return }
        value(child); ws(); c=substr(src,pos,1)
        if (c == "}") { pos++; return }
        if (c != ",") { die("expected comma or object end"); return }
        pos++
      }
    }
    function array(path,   i,c) {
      pos++; ws()
      if (substr(src,pos,1) == "]") { pos++; return }
      i=0
      while (!bad) {
        value(path "[" i "]"); i++; ws(); c=substr(src,pos,1)
        if (c == "]") { pos++; return }
        if (c != ",") { die("expected comma or array end"); return }
        pos++
      }
    }
    { src=src $0 "\n" }
    END {
      pos=1; ws(); value(""); ws()
      if (!bad && pos <= length(src)) die("trailing content")
      if (bad) exit 2
    }
  ' "$file"
}

json_get() {
  local flattened="$1"
  local path="$2"
  awk -F '\t' -v path="$path" '$1 == path { print $3; found=1 } END { if (!found) exit 1 }' <<< "$flattened"
}

json_type() {
  local flattened="$1"
  local path="$2"
  awk -F '\t' -v path="$path" '$1 == path { print $2; found=1 } END { if (!found) exit 1 }' <<< "$flattened"
}

require_json_value() {
  local evidence_file="$1"
  local flattened="$2"
  local path="$3"
  local expected="$4"
  local actual
  actual="$(json_get "$flattened" "$path" 2>/dev/null || true)"
  [[ "$actual" == "$expected" ]] || fail "$evidence_file: $path must equal $expected"
}

require_json_pattern() {
  local evidence_file="$1"
  local flattened="$2"
  local path="$3"
  local pattern="$4"
  local actual
  actual="$(json_get "$flattened" "$path" 2>/dev/null || true)"
  [[ "$actual" =~ $pattern ]] || fail "$evidence_file: $path is missing or malformed"
}

validate_evidence_artifact() {
  local card="$1"
  local artifact_path="$2"
  local expected_sha="$3"
  local expected_identity="$4"
  local expected_schema="$5"
  local expected_role="$6"
  local expected_context="$7"
  local expected_backend="$8"
  local expected_independence="${9:-}"
  local evidence_root="$board_dir/evidence/$card"
  local artifact_file flattened actual_sha resolved verdict dimension_id dimension_status dimension_index
  local -a dimension_ids=(contract design compatibility tests security concurrency errors-operations documentation scope-simplicity hunks)

  safe_repo_path "$artifact_path" || {
    fail "$evidence_root/manifest.json: unsafe evidence path $artifact_path"
    return
  }
  [[ "$artifact_path" == ".board/evidence/$card/"* ]] || {
    fail "$evidence_root/manifest.json: evidence path escapes the card directory: $artifact_path"
    return
  }
  artifact_file="$repo_root/$artifact_path"
  [[ -f "$artifact_file" && ! -L "$artifact_file" ]] || {
    fail "$evidence_root/manifest.json: referenced artifact is missing or a symlink: $artifact_path"
    return
  }
  if grep -Eiq '"(api[_-]?key|access[_-]?token|authorization|credential|raw[_-]?prompt|model[_-]?response|chain[_-]?of[_-]?thought)"[[:space:]]*:' "$artifact_file"; then
    fail "$artifact_file: forbidden sensitive/raw model field"
  fi
  resolved="$(cd "$(dirname "$artifact_file")" && pwd -P)/$(basename "$artifact_file")"
  [[ "$resolved" == "$evidence_root/"* ]] || {
    fail "$evidence_root/manifest.json: referenced artifact resolves outside the card directory"
    return
  }
  actual_sha="sha256:$(sha256sum "$artifact_file" | awk '{print $1}')"
  [[ "$actual_sha" == "$expected_sha" ]] || fail "$evidence_root/manifest.json: digest mismatch for $artifact_path"
  if ! flattened="$(json_flatten "$artifact_file" 2>/dev/null)"; then
    fail "$artifact_file: invalid JSON"
    return
  fi
  require_json_value "$artifact_file" "$flattened" schema "$expected_schema"
  require_json_value "$artifact_file" "$flattened" version 1
  require_json_value "$artifact_file" "$flattened" card_id "$card"
  require_json_value "$artifact_file" "$flattened" candidate_identity_digest "$expected_identity"
  require_json_value "$artifact_file" "$flattened" role "$expected_role"
  require_json_value "$artifact_file" "$flattened" sealed true
  require_json_pattern "$artifact_file" "$flattened" role_nonce '^[A-Za-z0-9._:-]{16,128}$'
  require_json_value "$artifact_file" "$flattened" context_digest "$expected_context"
  require_json_value "$artifact_file" "$flattened" backend_family_digest "$expected_backend"

  if [[ "$expected_schema" == "aurum.review-report" ]]; then
    verdict="$(json_get "$flattened" verdict 2>/dev/null || true)"
    [[ "$verdict" =~ ^(pass|pass_with_nits)$ ]] || fail "$artifact_file: a review/done card requires a non-blocking sealed verdict"
    require_json_value "$artifact_file" "$flattened" coverage.all_hunks true
    require_json_value "$artifact_file" "$flattened" coverage.all_dimensions true
    require_json_pattern "$artifact_file" "$flattened" coverage.manifest_digest '^sha256:[0-9a-f]{64}$'
    require_json_value "$artifact_file" "$flattened" independence_level "$expected_independence"
    require_json_value "$artifact_file" "$flattened" isolation.peer_report_visible_before_seal false
    require_json_value "$artifact_file" "$flattened" isolation.builder_trace_received false
    require_json_value "$artifact_file" "$flattened" isolation.shared_memory false
    require_json_value "$artifact_file" "$flattened" coverage.uncovered_hunks 0
    [[ "$(json_type "$flattened" coverage.dimensions 2>/dev/null || true)" == "array" ]] || fail "$artifact_file: coverage.dimensions must be an array"
    for (( dimension_index = 0; dimension_index < ${#dimension_ids[@]}; dimension_index++ )); do
      dimension_id="${dimension_ids[$dimension_index]}"
      require_json_value "$artifact_file" "$flattened" "coverage.dimensions[$dimension_index].id" "$dimension_id"
      dimension_status="$(json_get "$flattened" "coverage.dimensions[$dimension_index].status" 2>/dev/null || true)"
      [[ "$dimension_status" =~ ^(covered|not_applicable)$ ]] || fail "$artifact_file: dimension $dimension_id is not covered or justified not_applicable"
      if [[ "$dimension_status" == "covered" ]]; then
        require_json_pattern "$artifact_file" "$flattened" "coverage.dimensions[$dimension_index].evidence_digest" '^sha256:[0-9a-f]{64}$'
      else
        require_json_pattern "$artifact_file" "$flattened" "coverage.dimensions[$dimension_index].justification" '^.{16,512}$'
      fi
    done
    [[ -z "$(json_type "$flattened" "coverage.dimensions[${#dimension_ids[@]}]" 2>/dev/null || true)" ]] || fail "$artifact_file: coverage.dimensions must contain exactly ten entries"
  else
    require_json_value "$artifact_file" "$flattened" verdict pass
    require_json_value "$artifact_file" "$flattened" challenge_plan.sealed_before_reviews true
    require_json_pattern "$artifact_file" "$flattened" challenge_plan.digest '^sha256:[0-9a-f]{64}$'
    require_json_value "$artifact_file" "$flattened" challenge_plan.all_acceptance_criteria true
    require_json_value "$artifact_file" "$flattened" challenge_plan.all_trust_boundaries true
    require_json_value "$artifact_file" "$flattened" isolation.reviews_visible_before_challenge_seal false
    require_json_value "$artifact_file" "$flattened" mutation.detected true
    require_json_value "$artifact_file" "$flattened" clean_replay.pass true
    require_json_value "$artifact_file" "$flattened" secret_canaries.pass true
  fi
}

validate_evidence_hashes() {
  local card="$1"
  local flattened="$2"
  local manifest="$board_dir/evidence/$card/manifest.json"
  local evidence_root="$board_dir/evidence/$card"
  local index path expected_sha artifact_file resolved actual_sha path_key actual_relative count=0
  local expected_index=0 previous_path='' chain_input='' identity computed_chain declared_chain
  local -a evidence_indices=()
  declare -A hashed_paths=()

  mapfile -t evidence_indices < <(
    awk -F '\t' '$1 ~ /^evidence_hashes\[[0-9]+\]\.path$/ {
      value=$1
      sub(/^evidence_hashes\[/, "", value)
      sub(/\]\.path$/, "", value)
      print value
    }' <<< "$flattened" | sort -n -u
  )
  (( ${#evidence_indices[@]} >= 3 )) || fail "$manifest: evidence_hashes must include both reviews and the skeptic report"
  for index in "${evidence_indices[@]}"; do
    [[ "$index" -eq "$expected_index" ]] || fail "$manifest: evidence_hashes indices must be contiguous from zero"
    expected_index=$((expected_index + 1))
    path="$(json_get "$flattened" "evidence_hashes[$index].path" 2>/dev/null || true)"
    expected_sha="$(json_get "$flattened" "evidence_hashes[$index].sha256" 2>/dev/null || true)"
    safe_repo_path "$path" || {
      fail "$manifest: evidence_hashes[$index].path is unsafe"
      continue
    }
    [[ "$path" == ".board/evidence/$card/"* ]] || {
      fail "$manifest: evidence_hashes[$index].path escapes the card directory"
      continue
    }
    [[ -z "${hashed_paths[$path]+x}" ]] || fail "$manifest: duplicate evidence hash path $path"
    hashed_paths[$path]=1
    if [[ -n "$previous_path" && "$previous_path" > "$path" ]]; then
      fail "$manifest: evidence_hashes paths must be strictly byte-sorted"
    fi
    previous_path="$path"
    [[ "$expected_sha" =~ ^sha256:[0-9a-f]{64}$ ]] || {
      fail "$manifest: evidence_hashes[$index].sha256 is malformed"
      continue
    }
    artifact_file="$repo_root/$path"
    [[ -f "$artifact_file" && ! -L "$artifact_file" ]] || {
      fail "$manifest: hashed evidence is missing or a symlink: $path"
      continue
    }
    resolved="$(cd "$(dirname "$artifact_file")" && pwd -P)/$(basename "$artifact_file")"
    [[ "$resolved" == "$evidence_root/"* ]] || {
      fail "$manifest: hashed evidence resolves outside the card directory: $path"
      continue
    }
    actual_sha="sha256:$(sha256sum "$artifact_file" | awk '{print $1}')"
    [[ "$actual_sha" == "$expected_sha" ]] || fail "$manifest: evidence hash mismatch for $path"
    printf -v chain_input '%sartifact.path=%s\nartifact.sha256=%s\n' "$chain_input" "$path" "$expected_sha"
    count=$((count + 1))
  done
  [[ "$count" -eq "${#evidence_indices[@]}" ]] || fail "$manifest: evidence_hashes is incomplete or invalid"

  if find "$evidence_root" -type l -print -quit | grep -q .; then
    fail "$manifest: evidence directory contains a symlink"
  fi
  while IFS= read -r -d '' artifact_file; do
    [[ "$artifact_file" != "$manifest" ]] || continue
    actual_relative=".board/evidence/$card/${artifact_file#"$evidence_root/"}"
    [[ -n "${hashed_paths[$actual_relative]+x}" ]] || fail "$manifest: unlisted evidence artifact $actual_relative"
    if grep -Eiq -- '(-----BEGIN[[:space:]][A-Z ]*PRIVATE KEY-----|AKIA[0-9A-Z]{16}|sk-[A-Za-z0-9_-]{20,}|gh[pousr]_[A-Za-z0-9]{20,})' "$artifact_file"; then
      fail "$manifest: evidence artifact contains a credential-like value: $actual_relative"
    fi
  done < <(find "$evidence_root" -type f -print0 | sort -z)

  for path_key in reviews.a.path reviews.b.path skeptic.path; do
    path="$(json_get "$flattened" "$path_key" 2>/dev/null || true)"
    if [[ -z "$path" || -z "${hashed_paths[$path]+x}" ]]; then
      fail "$manifest: $path_key is absent from evidence_hashes"
    fi
  done
  path="$(json_get "$flattened" human_approval.path 2>/dev/null || true)"
  if [[ -n "$path" && -z "${hashed_paths[$path]+x}" ]]; then
    fail "$manifest: human_approval.path is absent from evidence_hashes"
  fi

  identity="$(json_get "$flattened" candidate_identity_digest 2>/dev/null || true)"
  declared_chain="$(json_get "$flattened" evidence_chain_digest 2>/dev/null || true)"
  computed_chain="sha256:$(printf 'candidate_identity_digest=%s\n%s' "$identity" "$chain_input" | sha256sum | awk '{print $1}')"
  [[ "$declared_chain" == "$computed_chain" ]] ||
    fail "$manifest: evidence_chain_digest does not match the canonical CandidateIdentity/path/hash chain"
}

validate_manifest() {
  local card="$1"
  local state="$2"
  local manifest="$board_dir/evidence/$card/manifest.json"
  local flattened identity candidate_path value report_path report_sha report_identity
  local candidate_digest_input computed_identity
  local review_a_context review_b_context skeptic_context
  local review_a_session review_b_session skeptic_session
  local review_a_backend review_b_backend skeptic_backend independence_level

  # `authenticated: true` inside a repository JSON file is a claim, not
  # authentication. Until a separately implemented verifier validates a
  # trusted SCM/service event, no card can cross the human `done` boundary.
  if [[ "$state" == "done" ]]; then
    fail "${files[$card]}: done is disabled until an authenticated human-approval verifier is bootstrapped"
    return
  fi

  [[ -f "$manifest" && ! -L "$manifest" ]] || {
    fail "${files[$card]}: $state card lacks a regular evidence/$card/manifest.json"
    return
  }
  if grep -Eiq '"(api[_-]?key|access[_-]?token|authorization|credential|raw[_-]?prompt|model[_-]?response|chain[_-]?of[_-]?thought)"[[:space:]]*:' "$manifest"; then
    fail "$manifest: forbidden sensitive/raw model field"
  fi
  if grep -Eiq -- '(-----BEGIN[[:space:]][A-Z ]*PRIVATE KEY-----|AKIA[0-9A-Z]{16}|sk-[A-Za-z0-9_-]{20,}|gh[pousr]_[A-Za-z0-9]{20,})' "$manifest"; then
    fail "$manifest: contains a credential-like value"
  fi
  if ! flattened="$(json_flatten "$manifest" 2>/dev/null)"; then
    fail "$manifest: invalid JSON or duplicate key"
    return
  fi
  require_json_value "$manifest" "$flattened" schema aurum.evidence-manifest
  require_json_value "$manifest" "$flattened" version 1
  require_json_value "$manifest" "$flattened" card_id "$card"
  require_json_value "$manifest" "$flattened" clean_tree true
  require_json_value "$manifest" "$flattened" gates.status pass
  require_json_value "$manifest" "$flattened" gates.fail_closed true
  require_json_value "$manifest" "$flattened" gates.secret_canary pass
  require_json_value "$manifest" "$flattened" gates.supply_chain pass
  require_json_pattern "$manifest" "$flattened" gates.coverage_manifest_digest '^sha256:[0-9a-f]{64}$'
  require_json_pattern "$manifest" "$flattened" gates.container_profile_digest '^sha256:[0-9a-f]{64}$'
  require_json_value "$manifest" "$flattened" tdd.acceptance_tests_locked true
  require_json_value "$manifest" "$flattened" tdd.red.behavioral_failure_verified true
  require_json_value "$manifest" "$flattened" tdd.green.status pass
  require_json_value "$manifest" "$flattened" tdd.refactor.status pass
  require_json_value "$manifest" "$flattened" reviews.barrier_sealed true
  require_json_value "$manifest" "$flattened" reviews.sealed_before_reconciliation true
  require_json_pattern "$manifest" "$flattened" reviews.independence_level '^I[1-3]$'
  require_json_value "$manifest" "$flattened" skeptic.challenge_presealed true
  require_json_value "$manifest" "$flattened" approval.unresolved_blockers 0
  require_json_value "$manifest" "$flattened" approval.verdict candidate_approved
  require_json_value "$manifest" "$flattened" evidence_hashes_complete true
  require_json_pattern "$manifest" "$flattened" evidence_chain_digest '^sha256:[0-9a-f]{64}$'
  [[ "$(json_type "$flattened" evidence_hashes 2>/dev/null || true)" == "array" ]] || fail "$manifest: evidence_hashes must be an array"
  validate_evidence_hashes "$card" "$flattened"

  identity="$(json_get "$flattened" candidate_identity_digest 2>/dev/null || true)"
  [[ "$identity" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "$manifest: candidate_identity_digest is missing or malformed"
  require_json_value "$manifest" "$flattened" candidate_identity.schema CandidateIdentityV1
  require_json_value "$manifest" "$flattened" candidate_identity.version 1
  for candidate_path in repository_identity base_tree_digest head_tree_digest change_digest task_spec_digest configuration_digest policy_digest prompt_and_rubric_digest skill_set_digest provider_model_backend_identity_digest toolchain_and_tool_set_digest dependency_lock_digest container_image_set_digest test_manifest_digest role_context_manifest_digest; do
    require_json_pattern "$manifest" "$flattened" "candidate_identity.$candidate_path" '^sha256:[0-9a-f]{64}$'
  done
  require_json_value "$manifest" "$flattened" candidate_identity.task_spec_digest "${card_spec_digests[$card]}"
  require_json_value "$manifest" "$flattened" provenance.base_sha "${card_base_shas[$card]}"
  require_json_pattern "$manifest" "$flattened" provenance.head_sha '^[0-9a-f]{40}([0-9a-f]{24})?$'
  require_json_value "$manifest" "$flattened" gates.test_manifest_digest "$(json_get "$flattened" candidate_identity.test_manifest_digest 2>/dev/null || true)"

  # CandidateIdentity/v1 is canonicalized in this exact field order. Reports
  # bind only to its aggregate digest, preventing identity-formula drift.
  candidate_digest_input="$(
    for candidate_path in repository_identity base_tree_digest head_tree_digest change_digest task_spec_digest configuration_digest policy_digest prompt_and_rubric_digest skill_set_digest provider_model_backend_identity_digest toolchain_and_tool_set_digest dependency_lock_digest container_image_set_digest test_manifest_digest role_context_manifest_digest; do
      value="$(json_get "$flattened" "candidate_identity.$candidate_path" 2>/dev/null || true)"
      printf '%s=%s\n' "$candidate_path" "$value"
    done
  )"
  computed_identity="sha256:$(printf '%s\n' "$candidate_digest_input" | sha256sum | awk '{print $1}')"
  [[ "$identity" == "$computed_identity" ]] || fail "$manifest: candidate_identity_digest does not match canonical CandidateIdentity/v1"

  for role in a b; do
    report_path="$(json_get "$flattened" "reviews.$role.path" 2>/dev/null || true)"
    report_sha="$(json_get "$flattened" "reviews.$role.sha256" 2>/dev/null || true)"
    report_identity="$(json_get "$flattened" "reviews.$role.candidate_identity_digest" 2>/dev/null || true)"
    [[ "$report_sha" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "$manifest: reviews.$role.sha256 is missing or malformed"
    [[ "$report_identity" == "$identity" ]] || fail "$manifest: reviews.$role is bound to a different CandidateIdentity"
    require_json_value "$manifest" "$flattened" "reviews.$role.sealed" true
    require_json_pattern "$manifest" "$flattened" "reviews.$role.context_digest" '^sha256:[0-9a-f]{64}$'
    require_json_pattern "$manifest" "$flattened" "reviews.$role.session_digest" '^sha256:[0-9a-f]{64}$'
    require_json_pattern "$manifest" "$flattened" "reviews.$role.backend_family_digest" '^sha256:[0-9a-f]{64}$'
    validate_evidence_artifact "$card" "$report_path" "$report_sha" "$identity" aurum.review-report "reviewer-$role" "$(json_get "$flattened" "reviews.$role.context_digest" 2>/dev/null || true)" "$(json_get "$flattened" "reviews.$role.backend_family_digest" 2>/dev/null || true)" "$(json_get "$flattened" reviews.independence_level 2>/dev/null || true)"
  done
  review_a_context="$(json_get "$flattened" reviews.a.context_digest 2>/dev/null || true)"
  review_b_context="$(json_get "$flattened" reviews.b.context_digest 2>/dev/null || true)"
  review_a_session="$(json_get "$flattened" reviews.a.session_digest 2>/dev/null || true)"
  review_b_session="$(json_get "$flattened" reviews.b.session_digest 2>/dev/null || true)"
  review_a_backend="$(json_get "$flattened" reviews.a.backend_family_digest 2>/dev/null || true)"
  review_b_backend="$(json_get "$flattened" reviews.b.backend_family_digest 2>/dev/null || true)"
  independence_level="$(json_get "$flattened" reviews.independence_level 2>/dev/null || true)"
  [[ "$review_a_context" != "$review_b_context" ]] || fail "$manifest: reviewers A and B reuse the same context"
  [[ "$review_a_session" != "$review_b_session" ]] || fail "$manifest: reviewers A and B reuse the same session"
  if [[ "$independence_level" =~ ^I[23]$ && "$review_a_backend" == "$review_b_backend" ]]; then
    fail "$manifest: I2/I3 claimed with the same backend family"
  fi
  if [[ "$state" == "review" && "$independence_level" == "I3" ]]; then
    fail "$manifest: I3 cannot be claimed before authenticated human approval"
  fi
  report_path="$(json_get "$flattened" skeptic.path 2>/dev/null || true)"
  report_sha="$(json_get "$flattened" skeptic.sha256 2>/dev/null || true)"
  report_identity="$(json_get "$flattened" skeptic.candidate_identity_digest 2>/dev/null || true)"
  [[ "$report_sha" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "$manifest: skeptic.sha256 is missing or malformed"
  [[ "$report_identity" == "$identity" ]] || fail "$manifest: skeptic is bound to a different CandidateIdentity"
  require_json_value "$manifest" "$flattened" skeptic.sealed true
  require_json_pattern "$manifest" "$flattened" skeptic.context_digest '^sha256:[0-9a-f]{64}$'
  require_json_pattern "$manifest" "$flattened" skeptic.session_digest '^sha256:[0-9a-f]{64}$'
  require_json_pattern "$manifest" "$flattened" skeptic.backend_family_digest '^sha256:[0-9a-f]{64}$'
  skeptic_context="$(json_get "$flattened" skeptic.context_digest 2>/dev/null || true)"
  skeptic_session="$(json_get "$flattened" skeptic.session_digest 2>/dev/null || true)"
  skeptic_backend="$(json_get "$flattened" skeptic.backend_family_digest 2>/dev/null || true)"
  [[ "$skeptic_context" != "$review_a_context" && "$skeptic_context" != "$review_b_context" ]] || fail "$manifest: skeptic reuses a reviewer context"
  [[ "$skeptic_session" != "$review_a_session" && "$skeptic_session" != "$review_b_session" ]] || fail "$manifest: skeptic reuses a reviewer session"
  validate_evidence_artifact "$card" "$report_path" "$report_sha" "$identity" aurum.skeptic-report skeptic "$skeptic_context" "$skeptic_backend"

  if [[ -n "$(json_type "$flattened" human_approval 2>/dev/null || true)" ]]; then
    fail "$manifest: human approval must not be attached before the done transition"
  fi
}

for state in "${states[@]}"; do
  directory="$board_dir/cards/$state"
  [[ -d "$directory" ]] || fail "missing state directory: cards/$state"
done
while IFS= read -r -d '' directory; do
  state="$(basename "$directory")"
  case " ${states[*]} " in
    *" $state "*) ;;
    *) fail "unknown card state directory: cards/$state" ;;
  esac
done < <(find "$board_dir/cards" -mindepth 1 -maxdepth 1 -type d -print0 | sort -z)
while IFS= read -r -d '' unexpected_file; do
  [[ "$(basename "$unexpected_file")" == ".gitkeep" ]] || fail "unexpected file in card state directory: $unexpected_file"
done < <(find "$board_dir/cards" -mindepth 2 -maxdepth 2 -type f ! -name 'AUR-*.md' -print0 | sort -z)
if find "$board_dir/cards" -type l -print -quit | grep -q .; then
  fail "cards tree must not contain symlinks"
fi

while IFS= read -r -d '' card; do
  filename="$(basename "$card")"
  state="$(basename "$(dirname "$card")")"
  id="${filename%.md}"

  [[ "$id" =~ ^AUR-[0-9]{3}$ ]] || fail "$card: filename must be AUR-NNN.md"
  if grep -Eiq '(tokens truncated|warning:[[:space:]]+truncated output|original token count)' "$card"; then
    fail "$card: contains a tool-output truncation marker"
  fi
  if [[ -n "${ids[$id]+x}" ]]; then
    fail "$card: duplicate id also found at ${files[$id]}"
  fi
  ids[$id]=1
  files[$id]="$card"
  card_states[$id]="$state"

  for field in id version title status office depends_on requirements controls paths forbidden_paths base_sha spec_digest risk data_class trust_boundaries; do
    count="$(frontmatter_count "$card" "$field" 2>/dev/null || true)"
    [[ "$count" == 1 ]] || fail "$card: front matter must contain $field exactly once"
  done
  count="$(frontmatter_count "$card" read_paths 2>/dev/null || true)"
  [[ "$count" -le 1 ]] || fail "$card: optional front matter read_paths may occur at most once"
  field_id="$(frontmatter_value "$card" id 2>/dev/null || true)"
  title="$(frontmatter_value "$card" title 2>/dev/null || true)"
  field_status="$(frontmatter_value "$card" status 2>/dev/null || true)"
  office="$(frontmatter_value "$card" office 2>/dev/null || true)"
  dependencies="$(frontmatter_value "$card" depends_on 2>/dev/null || true)"
  requirements="$(frontmatter_value "$card" requirements 2>/dev/null || true)"
  controls="$(frontmatter_value "$card" controls 2>/dev/null || true)"
  paths="$(frontmatter_value "$card" paths 2>/dev/null || true)"
  if [[ "$count" -eq 1 ]]; then
    read_paths="$(frontmatter_value "$card" read_paths 2>/dev/null || true)"
  else
    read_paths='[]'
  fi
  forbidden_paths="$(frontmatter_value "$card" forbidden_paths 2>/dev/null || true)"
  risk="$(frontmatter_value "$card" risk 2>/dev/null || true)"
  data_class="$(frontmatter_value "$card" data_class 2>/dev/null || true)"
  trust_boundaries="$(frontmatter_value "$card" trust_boundaries 2>/dev/null || true)"
  spec_version="$(frontmatter_value "$card" version 2>/dev/null || true)"
  base_sha="$(frontmatter_value "$card" base_sha 2>/dev/null || true)"
  spec_digest="$(frontmatter_value "$card" spec_digest 2>/dev/null || true)"

  [[ "$field_id" == "$id" ]] || fail "$card: front matter id differs from filename"
  [[ "$title" =~ ^[^[:space:]].{7,}$ ]] && ! is_generic_text "$title" || fail "$card: title is empty or generic"
  [[ "$field_status" == "$state" ]] || fail "$card: status must match containing directory"
  [[ "$office" =~ ^O[0-9]{2}-[a-z0-9-]+$ ]] || fail "$card: invalid office"
  [[ "$dependencies" =~ ^\[(|AUR-[0-9]{3}(,[[:space:]]AUR-[0-9]{3})*)\]$ ]] || fail "$card: invalid depends_on list"
  [[ "$risk" =~ ^(low|medium|high|critical)$ ]] || fail "$card: invalid risk"
  [[ "$data_class" =~ ^(public|internal|confidential|restricted|mixed)$ ]] || fail "$card: invalid data_class"
  [[ "$spec_version" =~ ^[1-9][0-9]*$ ]] || fail "$card: spec_version must be a positive integer"
  if [[ "$state" =~ ^(backlog|ready|blocked-on-owner)$ ]]; then
    [[ "$base_sha" == "lock-at-execution" ]] || fail "$card: unlocked card base_sha must be lock-at-execution"
    [[ "$spec_digest" == "lock-at-execution" ]] || fail "$card: unlocked card spec_digest must be lock-at-execution"
  else
    [[ "$base_sha" =~ ^[0-9a-f]{40}([0-9a-f]{24})?$ ]] || fail "$card: locked base_sha must be an immutable Git object id"
    [[ "$spec_digest" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "$card: locked spec_digest must be a SHA-256 digest"
  fi
  card_titles[$id]="$title"
  card_offices[$id]="$office"
  card_risks[$id]="$risk"
  card_spec_digests[$id]="$spec_digest"

  if [[ "$state" =~ ^(doing|review|done)$ ]]; then
    computed_spec_digest="sha256:$(sed -E 's/^status: .+$/status: STATE/; s/^spec_digest: sha256:[0-9a-f]{64}$/spec_digest: sha256:SELF/' "$card" | sha256sum | awk '{print $1}')"
    [[ "$spec_digest" == "$computed_spec_digest" ]] || fail "$card: spec_digest does not match canonical locked card content"
    card_base_shas[$id]="$base_sha"
  else
    card_base_shas[$id]=""
  fi

  if ! mapfile -t requirement_list < <(parse_list "$requirements"); then
    fail "$card: requirements must be a non-empty canonical list"
    requirement_list=()
  fi
  [[ "$requirements" =~ ^\[PR-[A-Z]+-[0-9]{3}(,[[:space:]]PR-[A-Z]+-[0-9]{3})*\]$ ]] || fail "$card: requirements list is not canonical"
  (( ${#requirement_list[@]} > 0 )) || fail "$card: requirements must bind at least one product requirement"
  unset seen_requirements
  declare -A seen_requirements=()
  for requirement in "${requirement_list[@]}"; do
    [[ -z "${seen_requirements[$requirement]+x}" ]] || fail "$card: duplicate requirement $requirement"
    seen_requirements[$requirement]=1
    [[ "$requirement" =~ ^PR-[A-Z]+-[0-9]{3}$ ]] || {
      fail "$card: malformed requirement id $requirement"
      continue
    }
    grep -Fq "| $requirement |" "$board_dir/requirements/REQUIREMENTS.md" || fail "$card: unknown product requirement $requirement"
    referenced_requirements[$requirement]=1
  done
  if ! mapfile -t control_list < <(parse_list "$controls"); then
    fail "$card: controls must be a non-empty canonical list"
    control_list=()
  fi
  [[ "$controls" =~ ^\[CR-[A-Z]+-[0-9]{3}(,[[:space:]]CR-[A-Z]+-[0-9]{3})*\]$ ]] || fail "$card: controls list is not canonical"
  (( ${#control_list[@]} > 0 )) || fail "$card: controls must bind at least one CR-* rule"
  unset seen_controls
  declare -A seen_controls=()
  for control in "${control_list[@]}"; do
    [[ -z "${seen_controls[$control]+x}" ]] || fail "$card: duplicate control $control"
    seen_controls[$control]=1
    [[ "$control" =~ ^CR-[A-Z]+-[0-9]{3}$ ]] || {
      fail "$card: malformed control id $control"
      continue
    }
    grep -Fq "\`$control\`" "$board_dir/research/code-review-standards.md" || fail "$card: unknown code-review control $control"
    referenced_controls[$control]=1
  done

  if ! mapfile -t declared_paths < <(parse_list "$paths"); then
    fail "$card: paths must be a non-empty canonical list"
    declared_paths=()
  fi
  canonical_repo_path_list "$paths" false || fail "$card: paths list is not canonical"
  (( ${#declared_paths[@]} > 0 )) || fail "$card: paths must be non-empty"
  if ! mapfile -t readable_paths < <(parse_list "$read_paths"); then
    fail "$card: read_paths must be a canonical list when present"
    readable_paths=()
  fi
  canonical_repo_path_list "$read_paths" true || fail "$card: read_paths list is not canonical"
  if ! mapfile -t denied_paths < <(parse_list "$forbidden_paths"); then
    fail "$card: forbidden_paths must be a non-empty canonical list"
    denied_paths=()
  fi
  canonical_repo_path_list "$forbidden_paths" false || fail "$card: forbidden_paths list is not canonical"
  (( ${#denied_paths[@]} > 0 )) || fail "$card: forbidden_paths must explicitly deny at least one path"

  unset seen_declared_paths seen_denied_paths seen_readable_paths
  declare -A seen_declared_paths=()
  declare -A seen_denied_paths=()
  declare -A seen_readable_paths=()
  for declared_path in "${declared_paths[@]}"; do
    [[ -z "${seen_declared_paths[$declared_path]+x}" ]] || fail "$card: duplicate owned path $declared_path"
    seen_declared_paths[$declared_path]=1
    if ! safe_repo_path "$declared_path"; then
      fail "$card: unsafe or non-repository-relative owned path: $declared_path"
      continue
    fi
    path_owners[$declared_path]="${path_owners[$declared_path]:-} $id"
  done
  for denied_path in "${denied_paths[@]}"; do
    [[ -z "${seen_denied_paths[$denied_path]+x}" ]] || fail "$card: duplicate forbidden path $denied_path"
    seen_denied_paths[$denied_path]=1
    safe_repo_path "$denied_path" || fail "$card: unsafe or non-repository-relative forbidden path: $denied_path"
    for declared_path in "${declared_paths[@]}"; do
      paths_overlap "$declared_path" "$denied_path" && fail "$card: owned and forbidden paths overlap: $declared_path <> $denied_path"
    done
  done
  for readable_path in "${readable_paths[@]}"; do
    [[ -z "${seen_readable_paths[$readable_path]+x}" ]] || fail "$card: duplicate read path $readable_path"
    seen_readable_paths[$readable_path]=1
    safe_repo_path "$readable_path" || {
      fail "$card: unsafe or non-repository-relative read path: $readable_path"
      continue
    }
    for denied_path in "${denied_paths[@]}"; do
      paths_overlap "$readable_path" "$denied_path" && fail "$card: readable and forbidden paths overlap: $readable_path <> $denied_path"
    done
  done

  if ! mapfile -t boundary_list < <(parse_list "$trust_boundaries"); then
    fail "$card: trust_boundaries must be a non-empty canonical list"
    boundary_list=()
  fi
  [[ "$trust_boundaries" =~ ^\[[^][,]+(,[[:space:]][^][,]+)*\]$ ]] || fail "$card: trust_boundaries list is not canonical"
  (( ${#boundary_list[@]} > 0 )) || fail "$card: trust_boundaries must be explicit"
  unset seen_boundaries
  declare -A seen_boundaries=()
  for boundary in "${boundary_list[@]}"; do
    [[ -z "${seen_boundaries[$boundary]+x}" ]] || fail "$card: duplicate trust boundary $boundary"
    seen_boundaries[$boundary]=1
    [[ "$boundary" =~ ^[A-Za-z0-9][A-Za-z0-9._/-]{2,63}$ ]] || fail "$card: malformed trust boundary $boundary"
  done

  for section in "${required_sections[@]}"; do
    section_count="$(grep -Fxc "$section" "$card" || true)"
    [[ "$section_count" -eq 1 ]] || fail "$card: section must occur exactly once: $section"
  done
  for section in "## Outcome" "## Non-goals" "## Preconditions" "## Postconditions" "## Public contract" "## Security and privacy" "## Documentation" "## Compatibility, migration, rollback"; do
    require_meaningful_section "$card" "$section"
  done

  non_goals="$(section_body "$card" "## Non-goals")"
  grep -Eq '^- .+' <<< "$non_goals" || fail "$card: Non-goals must list an explicit exclusion"
  record_spec_owner non_goal_owners "$(sed -n 's/^- \(.*\)$/\1/p' <<< "$non_goals" | head -1)" "$id"
  scenarios="$(section_body "$card" "## Acceptance scenarios")"
  mapfile -t acceptance_ids < <(sed -n 's/^### \(AC-[0-9]\{3\}\): .\+$/\1/p' <<< "$scenarios")
  (( ${#acceptance_ids[@]} > 0 )) || fail "$card: missing named AC-NNN Given/When/Then scenario"
  all_scenario_heading_count="$(grep -Ec '^### AC-' <<< "$scenarios" || true)"
  [[ "$all_scenario_heading_count" -eq "${#acceptance_ids[@]}" ]] || fail "$card: malformed acceptance scenario heading"
  covers_enabled=0
  grep -Eq '^- Covers (requirements|controls):' <<< "$scenarios" && covers_enabled=1
  unset covered_requirement_ids covered_control_ids
  declare -A covered_requirement_ids=()
  declare -A covered_control_ids=()
  for (( scenario_index = 0; scenario_index < ${#acceptance_ids[@]}; scenario_index++ )); do
    printf -v expected_scenario_id 'AC-%03d' "$((scenario_index + 1))"
    scenario_id="${acceptance_ids[$scenario_index]}"
    [[ "$scenario_id" == "$expected_scenario_id" ]] || fail "$card: acceptance scenarios must be unique and sequential from AC-001"
    scenario_block="$(subsection_body "$card" "### $scenario_id:")"
    [[ "$(grep -Ec '^- Given: .+' <<< "$scenario_block" || true)" -eq 1 ]] || fail "$card: $scenario_id must contain exactly one Given"
    [[ "$(grep -Ec '^- When: .+' <<< "$scenario_block" || true)" -eq 1 ]] || fail "$card: $scenario_id must contain exactly one When"
    [[ "$(grep -Ec '^- Then: .+' <<< "$scenario_block" || true)" -eq 1 ]] || fail "$card: $scenario_id must contain exactly one Then"

    scenario_given_text="$(sed -n 's/^- Given: \(.*\)$/\1/p' <<< "$scenario_block" | head -1)"
    scenario_when_text="$(sed -n 's/^- When: \(.*\)$/\1/p' <<< "$scenario_block" | head -1)"
    scenario_then_text="$(sed -n 's/^- Then: \(.*\)$/\1/p' <<< "$scenario_block" | head -1)"
    # A Then that restates the Outcome proves nothing the Outcome did not already
    # assert, so the scenario cannot fail for a behavioral reason.
    if [[ -n "$scenario_then_text" ]] &&
       [[ "$(normalize_spec_text "$scenario_then_text")" == "$(normalize_spec_text "$(section_body "$card" '## Outcome')")" ]]; then
      fail "$card: $scenario_id Then restates the Outcome instead of naming an observable result"
    fi
    record_spec_owner scenario_given_owners "$scenario_given_text" "$id/$scenario_id"
    record_spec_owner scenario_when_owners "$scenario_when_text" "$id/$scenario_id"
    record_spec_owner scenario_then_owners "$scenario_then_text" "$id/$scenario_id"
    if (( covers_enabled == 1 )); then
      requirement_covers_count="$(grep -Ec '^- Covers requirements: .+' <<< "$scenario_block" || true)"
      control_covers_count="$(grep -Ec '^- Covers controls: .+' <<< "$scenario_block" || true)"
      [[ "$requirement_covers_count" -eq 1 ]] || fail "$card: $scenario_id must contain exactly one Covers requirements list"
      [[ "$control_covers_count" -eq 1 ]] || fail "$card: $scenario_id must contain exactly one Covers controls list"
      scenario_requirements="$(sed -n 's/^- Covers requirements: \(.*\)$/\1/p' <<< "$scenario_block")"
      scenario_controls="$(sed -n 's/^- Covers controls: \(.*\)$/\1/p' <<< "$scenario_block")"
      [[ "$scenario_requirements" =~ ^\[PR-[A-Z]+-[0-9]{3}(,[[:space:]]PR-[A-Z]+-[0-9]{3})*\]$ ]] ||
        fail "$card: $scenario_id Covers requirements list is not canonical"
      [[ "$scenario_controls" =~ ^\[CR-[A-Z]+-[0-9]{3}(,[[:space:]]CR-[A-Z]+-[0-9]{3})*\]$ ]] ||
        fail "$card: $scenario_id Covers controls list is not canonical"
      mapfile -t scenario_requirement_list < <(parse_list "$scenario_requirements" 2>/dev/null || true)
      mapfile -t scenario_control_list < <(parse_list "$scenario_controls" 2>/dev/null || true)
      for requirement in "${scenario_requirement_list[@]}"; do
        [[ -n "${seen_requirements[$requirement]+x}" ]] || fail "$card: $scenario_id covers undeclared requirement $requirement"
        covered_requirement_ids[$requirement]=1
      done
      for control in "${scenario_control_list[@]}"; do
        [[ -n "${seen_controls[$control]+x}" ]] || fail "$card: $scenario_id covers undeclared control $control"
        covered_control_ids[$control]=1
      done
    fi
  done
  if (( covers_enabled == 1 )); then
    for requirement in "${requirement_list[@]}"; do
      [[ -n "${covered_requirement_ids[$requirement]+x}" ]] || fail "$card: declared requirement $requirement is not covered by any AC"
    done
    for control in "${control_list[@]}"; do
      [[ -n "${covered_control_ids[$control]+x}" ]] || fail "$card: declared control $control is not covered by any AC"
    done
  fi

  tdd="$(section_body "$card" "## TDD proof")"
  red="$(grep -E '^- Red: .+' <<< "$tdd" || true)"
  green="$(grep -E '^- Green: .+' <<< "$tdd" || true)"
  refactor="$(grep -E '^- Refactor: .+' <<< "$tdd" || true)"
  test_count="$(grep -Ec '^- Test: .+' <<< "$tdd" || true)"
  test_line="$(grep -E '^- Test: .+' <<< "$tdd" || true)"
  [[ -n "$red" ]] || fail "$card: missing Red proof definition"
  [[ -n "$green" ]] || fail "$card: missing Green proof definition"
  [[ -n "$refactor" ]] || fail "$card: missing Refactor invariant"
  [[ "$test_count" -eq 1 ]] || fail "$card: TDD proof must contain exactly one Test reference"
  is_generic_text "$red" && fail "$card: Red proof is generic rather than a behavioral failure"
  is_generic_text "$green" && fail "$card: Green proof is generic rather than minimum observable behavior"
  record_spec_owner tdd_green_owners "${green#- Green: }" "$id"
  is_generic_text "$refactor" && fail "$card: Refactor proof is generic rather than an invariant"
  if [[ "$test_line" =~ ^-[[:space:]]Test:[[:space:]]\`(.+)::(AC-[0-9]{3}(,AC-[0-9]{3})*)\`\.$ ]]; then
    test_path="${BASH_REMATCH[1]}"
    test_selectors="${BASH_REMATCH[2]}"
    validate_owned_test_path "$card" Acceptance "$test_path" "${declared_paths[@]}"
    unset seen_test_scenarios
    declare -A seen_test_scenarios=()
    old_ifs="$IFS"
    IFS=','
    read -ra test_scenario_list <<< "$test_selectors"
    IFS="$old_ifs"
    for scenario_id in "${test_scenario_list[@]}"; do
      [[ -z "${seen_test_scenarios[$scenario_id]+x}" ]] || fail "$card: duplicate Test selector $scenario_id"
      seen_test_scenarios[$scenario_id]=1
      [[ " ${acceptance_ids[*]} " == *" $scenario_id "* ]] || fail "$card: Test binds unknown acceptance scenario $scenario_id"
    done
    for scenario_id in "${acceptance_ids[@]}"; do
      [[ -n "${seen_test_scenarios[$scenario_id]+x}" ]] || fail "$card: Test does not bind acceptance scenario $scenario_id"
    done
    if [[ -e "$repo_root/$test_path" || -L "$repo_root/$test_path" ]]; then
      [[ -f "$repo_root/$test_path" && ! -L "$repo_root/$test_path" ]] || fail "$card: acceptance Test must be a regular non-symlink: $test_path"
      [[ -x "$repo_root/$test_path" ]] || fail "$card: existing acceptance Test is not executable: $test_path"
    elif [[ "$state" =~ ^(doing|review|done)$ ]]; then
      fail "$card: active card acceptance Test is missing: $test_path"
    fi
  else
    fail "$card: Test must use the exact backtick-delimited tests/path::AC-NNN[,AC-NNN] grammar"
  fi
  for layer in Unit Contract Integration E2E; do
    validate_tdd_layer "$card" "$layer" "$tdd" "${declared_paths[@]}"
  done

  validate_acceptance "$card" "$id"
  grep -Eq '^Expected artifact: .+' "$card" || fail "$card: acceptance lacks a concrete expected artifact"
  acceptance_body="$(section_body "$card" "## Acceptance")"
  for scenario_id in "${acceptance_ids[@]}"; do
    grep -Fq "$scenario_id" <<< "$acceptance_body" || fail "$card: Acceptance evidence does not bind $scenario_id"
  done
  skeptical="$(section_body "$card" "## Skeptical mutations")"
  mapfile -t mutation_headings < <(grep -E '^### MUT-[0-9]{3}: AC-[0-9]{3} / [A-Za-z0-9._/-]+$' <<< "$skeptical" || true)
  (( ${#mutation_headings[@]} > 0 )) || fail "$card: skeptical mutation lacks a named MUT-NNN hypothesis mapped to AC and trust boundary"
  all_mutation_heading_count="$(grep -Ec '^### MUT-' <<< "$skeptical" || true)"
  [[ "$all_mutation_heading_count" -eq "${#mutation_headings[@]}" ]] || fail "$card: malformed skeptical mutation heading"
  unset covered_scenarios covered_boundaries
  declare -A covered_scenarios=()
  declare -A covered_boundaries=()
  for (( mutation_index = 0; mutation_index < ${#mutation_headings[@]}; mutation_index++ )); do
    printf -v mutation_id 'MUT-%03d' "$((mutation_index + 1))"
    mutation_heading="${mutation_headings[$mutation_index]}"
    [[ "$mutation_heading" == "### $mutation_id:"* ]] || fail "$card: skeptical mutations must be unique and sequential from MUT-001"
    mapped_scenario="$(sed -E 's/^### MUT-[0-9]{3}: (AC-[0-9]{3}) \/ .+$/\1/' <<< "$mutation_heading")"
    mapped_boundary="$(sed -E 's|^### MUT-[0-9]{3}: AC-[0-9]{3} / (.+)$|\1|' <<< "$mutation_heading")"
    [[ " ${acceptance_ids[*]} " == *" $mapped_scenario "* ]] || fail "$card: $mutation_id maps unknown scenario $mapped_scenario"
    [[ " ${boundary_list[*]} " == *" $mapped_boundary "* ]] || fail "$card: $mutation_id maps unknown trust boundary $mapped_boundary"
    covered_scenarios[$mapped_scenario]=1
    covered_boundaries[$mapped_boundary]=1
    mutation_block="$(subsection_body "$card" "### $mutation_id:")"
    [[ "$(grep -Ec '^- Change: .+' <<< "$mutation_block" || true)" -eq 1 ]] || fail "$card: $mutation_id must contain exactly one reversible Change"
    [[ "$(grep -Ec '^- Expected: .+' <<< "$mutation_block" || true)" -eq 1 ]] || fail "$card: $mutation_id must contain exactly one falsifiable Expected result"
    mutation_change="$(grep -E '^- Change: .+' <<< "$mutation_block" || true)"
    mutation_expected="$(grep -E '^- Expected: .+' <<< "$mutation_block" || true)"
    is_generic_text "$mutation_change" && fail "$card: $mutation_id Change is generic or invalid"
    is_generic_text "$mutation_expected" && fail "$card: $mutation_id Expected result accepts an invalid failure mode"
  done
  for scenario_id in "${acceptance_ids[@]}"; do
    [[ -n "${covered_scenarios[$scenario_id]+x}" ]] || fail "$card: no skeptical mutation covers $scenario_id"
  done
  for boundary in "${boundary_list[@]}"; do
    [[ -n "${covered_boundaries[$boundary]+x}" ]] || fail "$card: no skeptical mutation covers trust boundary $boundary"
  done

  review="$(section_body "$card" "## Review")"
  reviewer_a="$(grep -E '^- Reviewer A: .+' <<< "$review" || true)"
  reviewer_b="$(grep -E '^- Reviewer B: .+' <<< "$review" || true)"
  [[ -n "$reviewer_a" ]] || fail "$card: missing reviewer A lens"
  [[ -n "$reviewer_b" ]] || fail "$card: missing reviewer B lens"
  grep -Eq '^- Skeptical approver: .+' <<< "$review" || fail "$card: missing skeptical approver lens"
  grep -Eq '^- Independence: .+' <<< "$review" || fail "$card: missing explicit reviewer independence contract"
  [[ "$reviewer_a" =~ (every-hunk|all[[:space:]]+hunks|todos[[:space:]]+os[[:space:]]+hunks|cada[[:space:]]+hunk) ]] || fail "$card: Reviewer A must cover every hunk, not a partial specialty review"
  [[ "$reviewer_b" =~ (every-hunk|all[[:space:]]+hunks|todos[[:space:]]+os[[:space:]]+hunks|cada[[:space:]]+hunk) ]] || fail "$card: Reviewer B must cover every hunk, not a partial specialty review"
  [[ "$reviewer_a" =~ ((10|ten)[[:space:]-]+dimension|(10|dez)[[:space:]]+dimens) ]] || fail "$card: Reviewer A must cover all ten protocol dimensions"
  [[ "$reviewer_b" =~ ((10|ten)[[:space:]-]+dimension|(10|dez)[[:space:]]+dimens) ]] || fail "$card: Reviewer B must cover all ten protocol dimensions"
  grep -Fq ".board/evidence/$id/" "$card" || fail "$card: expected artifact is not card-scoped"
done < <(find "$board_dir/cards" -mindepth 2 -maxdepth 2 -type f -name 'AUR-*.md' -print0 | sort -z)

(( ${#ids[@]} > 0 )) || fail "board contains no cards"

# Reverse traceability is as important as rejecting unknown references. A
# registry row with no card is an unimplemented product/control claim.
while IFS= read -r requirement; do
  [[ -n "$requirement" ]] || continue
  [[ -n "${referenced_requirements[$requirement]+x}" ]] ||
    fail "requirements registry contains orphan product requirement $requirement"
done < <(
  awk -F '|' '
    {
      value=$2
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
      if (value ~ /^PR-[A-Z]+-[0-9][0-9][0-9]$/) print value
    }
  ' "$board_dir/requirements/REQUIREMENTS.md" | sort -u
)
while IFS= read -r control; do
  [[ -n "$control" ]] || continue
  [[ -n "${referenced_controls[$control]+x}" ]] ||
    fail "code-review standards contain orphan control $control"
done < <(sed -n 's/^`\(CR-[A-Z][A-Z]*-[0-9][0-9][0-9]\)`.*/\1/p' "$board_dir/research/code-review-standards.md" | sort -u)

while IFS= read -r -d '' card; do
  id="$(basename "$card" .md)"
  dependency_field="$(frontmatter_value "$card" depends_on 2>/dev/null || true)"
  if [[ ! "$dependency_field" =~ ^\[(|AUR-[0-9]{3}(,[[:space:]]AUR-[0-9]{3})*)\]$ ]]; then
    card_dependencies[$id]=""
    continue
  fi
  dependencies="$dependency_field"
  dependencies="${dependencies#[}"
  dependencies="${dependencies%]}"
  card_dependencies[$id]="$dependencies"
  if [[ "${card_states[$id]}" == "backlog" && -z "$dependencies" ]]; then
    fail "$card: an unblocked card belongs in ready, not backlog"
  fi
  [[ -z "$dependencies" ]] && continue
  old_ifs="$IFS"
  IFS=','
  read -ra dependency_list <<< "$dependencies"
  IFS="$old_ifs"
  for dependency in "${dependency_list[@]}"; do
    dependency="${dependency// /}"
    [[ -n "${ids[$dependency]+x}" ]] || fail "$card: unknown dependency $dependency"
    [[ "$dependency" != "$id" ]] || fail "$card: self dependency"
    if [[ "${card_states[$id]}" =~ ^(ready|doing|review|done)$ && "${card_states[$dependency]:-missing}" != "done" ]]; then
      fail "$card: active card depends on non-done $dependency"
    fi
  done
done < <(find "$board_dir/cards" -mindepth 2 -maxdepth 2 -type f -name 'AUR-*.md' -print0 | sort -z)

visit() {
  local id="$1"
  local dependency dependencies old_ifs
  case "${visit_state[$id]:-new}" in
    visiting)
      fail "dependency cycle reaches $id"
      return
      ;;
    done)
      return
      ;;
  esac

  visit_state[$id]="visiting"
  dependencies="${card_dependencies[$id]:-}"
  if [[ -n "$dependencies" ]]; then
    old_ifs="$IFS"
    IFS=','
    read -ra dependency_list <<< "$dependencies"
    IFS="$old_ifs"
    for dependency in "${dependency_list[@]}"; do
      dependency="${dependency// /}"
      [[ -n "${ids[$dependency]+x}" ]] && visit "$dependency"
    done
  fi
  visit_state[$id]="done"
}

for id in "${!ids[@]}"; do
  visit "$id"
done

# Unknown dependencies make reachability unsafe; stop rather than attempting to
# prove that overlapping ownership is ordered.
finish_failures

depends_on_transitively() {
  local from="$1"
  local target="$2"
  local key="$from|$target"
  local dependency dependencies old_ifs

  if [[ -n "${reachability[$key]+x}" ]]; then
    [[ "${reachability[$key]}" == 1 ]]
    return
  fi
  if [[ "$from" == "$target" ]]; then
    reachability[$key]=1
    return 0
  fi
  dependencies="${card_dependencies[$from]:-}"
  if [[ -z "$dependencies" ]]; then
    reachability[$key]=0
    return 1
  fi
  old_ifs="$IFS"
  IFS=','
  read -ra dependency_list <<< "$dependencies"
  IFS="$old_ifs"
  for dependency in "${dependency_list[@]}"; do
    dependency="${dependency// /}"
    if depends_on_transitively "$dependency" "$target"; then
      reachability[$key]=1
      return 0
    fi
  done
  reachability[$key]=0
  return 1
}

mapfile -t owned_path_list < <(printf '%s\n' "${!path_owners[@]}" | sort)
for (( path_left = 0; path_left < ${#owned_path_list[@]}; path_left++ )); do
  for (( path_right = path_left; path_right < ${#owned_path_list[@]}; path_right++ )); do
    first_path="${owned_path_list[$path_left]}"
    second_path="${owned_path_list[$path_right]}"
    # The list is byte-sorted. A path precedes its descendants, so once the
    # current right-hand value is neither equal nor under `first_path`, no
    # later value can overlap it. This preserves the fail-closed prefix check
    # without the quadratic scan that made a 300+ card board impractical.
    if [[ "$first_path" != "$second_path" && "$second_path" != "$first_path/"* ]]; then
      break
    fi
    read -ra first_owners <<< "${path_owners[$first_path]}"
    read -ra second_owners <<< "${path_owners[$second_path]}"
    for first in "${first_owners[@]}"; do
      for second in "${second_owners[@]}"; do
        [[ "$first" != "$second" ]] || continue
        # For the same exact key, test a pair only once.
        [[ "$first_path" != "$second_path" || "$first" < "$second" ]] || continue
        if ! depends_on_transitively "$first" "$second" && ! depends_on_transitively "$second" "$first"; then
          fail "overlapping paths $first_path ($first) and $second_path ($second) are concurrently owned by unordered cards"
        fi
      done
    done
  done
done

report_spec_collisions scenario_given_owners "acceptance Given"
report_spec_collisions scenario_when_owners "acceptance When"
report_spec_collisions scenario_then_owners "acceptance Then"
report_spec_collisions tdd_green_owners "TDD Green"
report_spec_collisions non_goal_owners "first Non-goal"

card_count="${#ids[@]}"
for (( sequence = 1; sequence <= card_count; sequence++ )); do
  printf -v expected_id 'AUR-%03d' "$sequence"
  [[ -n "${ids[$expected_id]+x}" ]] || fail "card sequence has a gap at $expected_id"
done

if [[ -f "$board_dir/INDEX.md" ]]; then
  for id in "${!ids[@]}"; do
    relative="cards/${card_states[$id]}/$id.md"
    index_row="| [$id]($relative) | ${card_states[$id]} | ${card_offices[$id]} | ${card_risks[$id]} | \`[${card_dependencies[$id]}]\` | ${card_titles[$id]} |"
    grep -Fqx "$index_row" "$board_dir/INDEX.md" || fail "INDEX.md lacks the canonical row for $id"
  done
  index_rows="$(grep -Ec '^\| \[AUR-[0-9]{3}\]\(cards/' "$board_dir/INDEX.md" || true)"
  [[ "$index_rows" -eq "$card_count" ]] || fail "INDEX.md has $index_rows card rows, expected $card_count"
else
  fail "missing INDEX.md"
fi

for id in "${!ids[@]}"; do
  if [[ "${card_states[$id]}" =~ ^(review|done)$ ]]; then
    validate_manifest "$id" "${card_states[$id]}"
  fi
done

finish_failures
printf 'board valid: %d atomic cards\n' "${#ids[@]}"
