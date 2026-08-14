#!/usr/bin/env bash
#
# tests/e2e/consumer/bootstrap/bootstrap.sh - the consumer bootstrap guard.
#
#   bash tests/e2e/consumer/bootstrap/bootstrap.sh <scaffold-dir> <target-dir>
#
# WHAT IT GUARANTEES
#
#   An empty checkout receives every scenario the scaffold declares, and it
#   receives nothing else. "Nothing else" is enforced three ways, because a
#   single allowlist lookup is not enough to keep private AurumCode data out of
#   a consumer repository:
#
#     1. PATH CLOSURE. Every file present under the scaffold tree must be an
#        allowlist entry, and every allowlist entry must be present. A path on
#        neither side aborts the bootstrap: `path-not-allowlisted` for a file
#        the allowlist does not name, `entry-missing` for the reverse.
#     2. PATH POLICY. An allowlist is data, so a tampered allowlist could name
#        `internal/...` or `.board/...` and be obeyed. Every entry path is
#        therefore checked against a closed policy that no manifest can widen:
#        a safe relative shape, a bounded length, and no segment naming an
#        AurumCode-private root.
#     3. PAYLOAD SCREEN. Bytes are read, not just hashed. A payload carrying an
#        AurumCode-private marker or a value shaped like a real credential is
#        refused even when the manifest seals its digest, so resealing a planted
#        file does not smuggle it through.
#
#   Everything above runs before a single byte is written. The copy itself is
#   delegated to `demo-repo/scaffold/scaffold.sh`, and the checkout it produced
#   is then verified again by this program: path closure, digests and the
#   payload screen are re-applied to the bytes that actually landed. A copier
#   that writes a file nobody allowlisted is caught there even though the
#   scaffold it read was clean.
#
# WHY NO `git` IS INVOKED
#
#   The sealed acceptance profile `bootstrap-readonly-v1` has no `git` binary,
#   no network and a read-only rootfs. An "empty checkout" here is therefore a
#   working tree with no entries, which is exactly what a fresh clone or
#   `git init` leaves behind, and the bootstrap is provable in the only
#   environment that gates it.
#
# OUTPUT
#
#   stdout line 1 is always the typed result:
#     consumer-bootstrap/bootstrapped entries=<n> scenarios=<n> tree_digest=sha256:<hex>
#     consumer-bootstrap/<code>[:<detail>]
#   On success, one `copied\t<path>` line per entry follows in sealed order.
#   A detail is a path, a name or a count. Payload bytes are never printed, so a
#   refusal can never become the leak it is reporting.
#
# Exit codes: 0 bootstrapped, 1 typed refusal, 64 usage, 79 inconclusive.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly rc_red=1

# The closed domain of this bootstrap. A manifest declares the same numbers and
# is refused when it disagrees: limits live in the program, never in the data.
readonly max_entries=64
readonly max_file_bytes=65536
readonly max_total_bytes=1048576
readonly max_path_bytes=128
readonly max_scenarios=32

# The credential shape gate, mirrored from `.board/bin/oci-run`. The expression
# is a pattern: none of its own bytes match it.
readonly credential_pattern='(-----BEGIN[[:space:]][A-Z ]*PRIVATE KEY-----|AKIA[0-9A-Z]{16}|sk-[A-Za-z0-9_-]{20,}|gh[pousr]_[A-Za-z0-9]{20,})'

# Markers that only AurumCode's own private material carries. A demo payload has
# no reason to mention the board, its evidence chain or its candidate identity,
# so any of these in a file bound for the consumer means private data is being
# imported. The synthetic placeholder shape `AURUM-FAKE-<NAME>` is deliberately
# absent from this list: a demo scenario about a leaked credential needs one.
readonly private_marker_pattern='(AURUM-PRIVATE-|AURUM_SECRET_CANARY|CandidateIdentityV1|aurum\.(evidence|acceptance-execution|second-reader-observation)|\.board/(evidence|cards)|\.taskmaster/)'

# Path segments that name an AurumCode-private root. No manifest can widen this.
readonly -a denied_segments=(
  '.board' '.git' '.github' '.taskmaster' '.claude' '.ssh' '.aurum-bootstrap'
  'credentials' 'secrets' 'internal'
)

refuse() {
  printf 'consumer-bootstrap/%s\n' "$1"
  exit "$rc_red"
}

infra() {
  printf 'consumer-bootstrap/inconclusive/%s\n' "$1" >&2
  exit 79
}

usage() {
  printf 'consumer-bootstrap/usage: bootstrap.sh <scaffold-dir> <target-dir>\n' >&2
  exit 64
}

for tool in bash cat find grep head mkdir mktemp printf rm sed sha256sum sort stat tr wc; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing_tool:$tool"
done

(( $# == 2 )) || usage
scaffold_dir="$1"
target_dir="$2"
[[ -n "$scaffold_dir" && -n "$target_dir" ]] || usage
[[ -d "$scaffold_dir" && ! -L "$scaffold_dir" ]] || refuse "scaffold-dir-missing:$scaffold_dir"

copier="$scaffold_dir/scaffold.sh"
manifest="$scaffold_dir/allowlist.manifest"
[[ -f "$copier" && ! -L "$copier" ]] || refuse 'copier-missing'
[[ -f "$manifest" && ! -L "$manifest" ]] || refuse 'manifest-missing'

# ---------------------------------------------------------------------------
# Manifest.
# ---------------------------------------------------------------------------
[[ "$(head -n 1 "$manifest")" == 'schema: aurum.demo-scaffold-allowlist' ]] || refuse 'manifest-schema'
grep -qx 'version: 1' "$manifest" || refuse 'manifest-version'

tree_root="$(sed -n 's/^tree-root: \([A-Za-z0-9][A-Za-z0-9._-]*\)$/\1/p' "$manifest")"
[[ -n "$tree_root" ]] || refuse 'manifest-tree-root'
tree_dir="$scaffold_dir/$tree_root"
[[ -d "$tree_dir" && ! -L "$tree_dir" ]] || refuse "tree-root-missing:$tree_root"

# A limit is read into a variable rather than returned through a command
# substitution: a refusal raised inside a subshell would only end the subshell
# and the caller would report the wrong code.
declared_limit=''
read_limit() {
  local name="$1"
  declared_limit="$(sed -n "s/^limit: $name \\([0-9]\\{1,\\}\\)\$/\\1/p" "$manifest")"
  [[ "$declared_limit" =~ ^[0-9]+$ ]] || refuse "limit-missing:$name"
}
require_limit() {
  local name="$1" expected="$2"
  read_limit "$name"
  [[ "$declared_limit" == "$expected" ]] || refuse "limit-divergence:$name"
}
require_limit max_entries "$max_entries"
require_limit max_file_bytes "$max_file_bytes"
require_limit max_total_bytes "$max_total_bytes"
require_limit max_path_bytes "$max_path_bytes"

declare -a entry_scenario=() entry_bytes=() entry_digest=() entry_path=()
declared_entries="$(grep -c '^entry: ' "$manifest" || true)"
while IFS=$'\t' read -r e_scenario e_bytes e_digest e_path; do
  [[ -n "$e_path" ]] || continue
  entry_scenario+=("$e_scenario"); entry_bytes+=("$e_bytes")
  entry_digest+=("$e_digest"); entry_path+=("$e_path")
done < <(sed -n 's/^entry: \([A-Za-z0-9-]\{1,\}\) [0-7]\{4\} \([0-9]\{1,\}\) \([0-9a-f]\{64\}\) \(.*\)$/\1\t\2\t\3\t\4/p' "$manifest")

entry_count="${#entry_path[@]}"
(( entry_count == declared_entries )) || refuse "manifest-entry-parse:$declared_entries"
(( entry_count >= 1 )) || refuse 'manifest-empty'
(( entry_count <= max_entries )) || refuse "too-many-entries:$entry_count"

declare -a scenario_id=() scenario_count=()
declared_scenarios="$(grep -c '^scenario: ' "$manifest" || true)"
while IFS=$'\t' read -r s_id s_count; do
  [[ -n "$s_id" ]] || continue
  scenario_id+=("$s_id"); scenario_count+=("$s_count")
done < <(sed -n 's/^scenario: \([a-z][a-z0-9-]\{1,40\}\) \([0-9]\{1,\}\)$/\1\t\2/p' "$manifest")

scenario_total="${#scenario_id[@]}"
(( scenario_total == declared_scenarios )) || refuse "manifest-scenario-parse:$declared_scenarios"
(( scenario_total >= 1 )) || refuse 'no-scenarios'
(( scenario_total <= max_scenarios )) || refuse "too-many-scenarios:$scenario_total"

# ---------------------------------------------------------------------------
# Path policy. Applied to every entry before the file behind it is touched.
# ---------------------------------------------------------------------------
declare -A entry_index=()
for (( i = 0; i < entry_count; i++ )); do
  path="${entry_path[i]}"
  (( ${#path} <= max_path_bytes )) || refuse "path-too-long:${#path}"
  [[ "$path" =~ ^[A-Za-z0-9][A-Za-z0-9._/-]*$ ]] || refuse "path-shape-denied:$path"
  [[ "$path" != */ ]] || refuse "path-shape-denied:$path"
  case "$path" in
    *//*|*/../*|../*|*/..) refuse "path-shape-denied:$path" ;;
  esac
  while IFS= read -r segment; do
    [[ -n "$segment" ]] || continue
    for denied in "${denied_segments[@]}"; do
      [[ "$segment" != "$denied" ]] || refuse "private-path-denied:$path"
    done
    case "$segment" in
      .env|.env.*) refuse "private-path-denied:$path" ;;
    esac
  done < <(printf '%s\n' "$path" | tr '/' '\n')
  [[ -z "${entry_index[$path]:-}" ]] || refuse "duplicate-entry:$path"
  entry_index[$path]=1
done

# Sealed order is byte order on the path, and the manifest must already be in it.
sorted_paths="$(printf '%s\n' "${entry_path[@]}" | sort)"
[[ "$sorted_paths" == "$(printf '%s\n' "${entry_path[@]}")" ]] || refuse 'manifest-unsorted'

# ---------------------------------------------------------------------------
# Scenario completeness. Every declared scenario must own exactly the number of
# entries it claims, under its own directory; every scenario-owned entry must
# belong to a declared scenario. `-` marks a shared entry that belongs to none.
# ---------------------------------------------------------------------------
declare -A scenario_declared=()
for (( s = 0; s < scenario_total; s++ )); do
  id="${scenario_id[s]}"
  [[ -z "${scenario_declared[$id]:-}" ]] || refuse "duplicate-scenario:$id"
  scenario_declared[$id]="${scenario_count[s]}"
  (( scenario_count[s] >= 1 )) || refuse "empty-scenario:$id"
done

declare -A scenario_observed=()
for (( i = 0; i < entry_count; i++ )); do
  id="${entry_scenario[i]}"
  path="${entry_path[i]}"
  if [[ "$id" == '-' ]]; then
    case "$path" in
      scenarios/*/*) refuse "unclaimed-scenario-entry:$path" ;;
    esac
    continue
  fi
  [[ -n "${scenario_declared[$id]:-}" ]] || refuse "undeclared-scenario:$id"
  [[ "$path" == "scenarios/$id/"* ]] || refuse "scenario-path-mismatch:$path"
  scenario_observed[$id]=$(( ${scenario_observed[$id]:-0} + 1 ))
done

for (( s = 0; s < scenario_total; s++ )); do
  id="${scenario_id[s]}"
  (( ${scenario_observed[$id]:-0} == scenario_count[s] )) || refuse "scenario-incomplete:$id"
done

# ---------------------------------------------------------------------------
# Source verification: measure, compare, screen. Still no effect on the target.
# ---------------------------------------------------------------------------
screen_file() {
  # Reports the reason a payload must not be imported, or nothing.
  local file="$1"
  if [[ -n "${AURUM_SECRET_CANARY:-}" ]] && grep -Fq -- "$AURUM_SECRET_CANARY" "$file"; then
    printf 'secret-canary'
    return 0
  fi
  if grep -Eq -- "$private_marker_pattern" "$file"; then
    printf 'private-marker'
    return 0
  fi
  if grep -Eiq -- "$credential_pattern" "$file"; then
    printf 'credential-shape'
    return 0
  fi
  return 0
}

total_bytes=0
for (( i = 0; i < entry_count; i++ )); do
  path="${entry_path[i]}"
  source_path="$tree_dir/$path"
  [[ ! -L "$source_path" ]] || refuse "symlink-denied:$path"
  [[ -e "$source_path" ]] || refuse "entry-missing:$path"
  [[ -f "$source_path" ]] || refuse "not-a-regular-file:$path"
  measured_bytes="$(stat -c '%s' -- "$source_path")" || infra "stat:$path"
  [[ "$measured_bytes" == "${entry_bytes[i]}" ]] || refuse "content-bytes-mismatch:$path"
  (( measured_bytes <= max_file_bytes )) || refuse "file-too-large:$path"
  total_bytes=$(( total_bytes + measured_bytes ))
  (( total_bytes <= max_total_bytes )) || refuse "tree-too-large:$total_bytes"
  measured_digest="$(sha256sum -- "$source_path" | sed 's/ .*//')"
  [[ "$measured_digest" == "${entry_digest[i]}" ]] || refuse "content-digest-mismatch:$path"
  reason="$(screen_file "$source_path")"
  case "$reason" in
    '') ;;
    secret-canary) refuse "secret-canary-detected:$path" ;;
    private-marker) refuse "private-marker-detected:$path" ;;
    credential-shape) refuse "credential-shape-detected:$path" ;;
    *) refuse "screen-error:$path" ;;
  esac
done

# Path closure over the scaffold tree itself: a file the allowlist does not name
# must abort the bootstrap rather than be silently left behind.
while IFS= read -r present; do
  [[ -n "$present" ]] || continue
  [[ -n "${entry_index[$present]:-}" ]] || refuse "path-not-allowlisted:$present"
done < <(cd -- "$tree_dir" && find . -mindepth 1 \( -type f -o -type l \) -print | sed 's|^\./||' | sort)

observed_tree_files="$(cd -- "$tree_dir" && find . -mindepth 1 \( -type f -o -type l \) -print | wc -l | tr -d ' ')"
(( observed_tree_files == entry_count )) || refuse "tree-entry-count:$observed_tree_files"

# ---------------------------------------------------------------------------
# Target. It must be an empty checkout before anything is copied.
# ---------------------------------------------------------------------------
if [[ -e "$target_dir" || -L "$target_dir" ]]; then
  [[ -d "$target_dir" && ! -L "$target_dir" ]] || refuse 'target-not-a-directory'
  [[ -z "$(find "$target_dir" -mindepth 1 -print -quit)" ]] || refuse 'target-not-empty'
fi

# ---------------------------------------------------------------------------
# Apply, then verify what actually landed. The copier is a separate program and
# is treated as untrusted: its plan is not evidence of what it wrote.
# ---------------------------------------------------------------------------
copier_stdout="$(mktemp "${TMPDIR:-/tmp}/consumer-bootstrap.XXXXXX")" || infra mktemp
copier_stderr="$(mktemp "${TMPDIR:-/tmp}/consumer-bootstrap.XXXXXX")" || infra mktemp
cleanup() { rm -f -- "$copier_stdout" "$copier_stderr" 2>/dev/null || true; }
trap cleanup EXIT INT TERM HUP

set +e
bash "$copier" "$scaffold_dir" "$target_dir" >"$copier_stdout" 2>"$copier_stderr"
copier_status=$?
set -e
if (( copier_status != 0 )); then
  copier_code="$(head -n 1 "$copier_stderr" | sed -n 's|^demo-scaffold/\([a-z-]\{1,\}\).*|\1|p')"
  [[ -n "$copier_code" ]] || copier_code='unknown'
  refuse "copier-failed:$copier_code:$copier_status"
fi

while IFS= read -r produced; do
  [[ -n "$produced" ]] || continue
  [[ -n "${entry_index[$produced]:-}" ]] || refuse "unlisted-entry-copied:$produced"
done < <(cd -- "$target_dir" && find . -mindepth 1 \( -type f -o -type l \) -print | sed 's|^\./||' | sort)

produced_count="$(cd -- "$target_dir" && find . -mindepth 1 \( -type f -o -type l \) -print | wc -l | tr -d ' ')"
(( produced_count == entry_count )) || refuse "copied-entry-count:$produced_count"

for (( i = 0; i < entry_count; i++ )); do
  path="${entry_path[i]}"
  produced_path="$target_dir/$path"
  [[ ! -L "$produced_path" ]] || refuse "copied-symlink:$path"
  [[ -f "$produced_path" ]] || refuse "copy-missing:$path"
  [[ "$(stat -c '%s' -- "$produced_path")" == "${entry_bytes[i]}" ]] || refuse "copy-divergence:$path"
  [[ "$(sha256sum -- "$produced_path" | sed 's/ .*//')" == "${entry_digest[i]}" ]] ||
    refuse "copy-divergence:$path"
  reason="$(screen_file "$produced_path")"
  case "$reason" in
    '') ;;
    secret-canary) refuse "secret-canary-copied:$path" ;;
    private-marker) refuse "private-marker-copied:$path" ;;
    credential-shape) refuse "credential-shape-copied:$path" ;;
    *) refuse "screen-error:$path" ;;
  esac
done

tree_digest="$(
  ( cd -- "$target_dir" && find . -type f -print | sed 's|^\./||' | sort ) |
    while IFS= read -r rel; do
      printf '%s\t%s\n' "$rel" "$(sha256sum -- "$target_dir/$rel" | sed 's/ .*//')"
    done | sha256sum | sed 's/ .*//'
)"
[[ "$tree_digest" =~ ^[0-9a-f]{64}$ ]] || infra 'tree-digest'

printf 'consumer-bootstrap/bootstrapped entries=%d scenarios=%d tree_digest=sha256:%s\n' \
  "$entry_count" "$scenario_total" "$tree_digest"
for (( i = 0; i < entry_count; i++ )); do
  printf 'copied\t%s\n' "${entry_path[i]}"
done
