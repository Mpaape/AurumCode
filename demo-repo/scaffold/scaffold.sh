#!/usr/bin/env bash
#
# demo-repo/scaffold/scaffold.sh - the allowlist copier.
#
#   bash demo-repo/scaffold/scaffold.sh <scaffold-dir> <target-dir>
#
# It copies into <target-dir> exactly the entries sealed in
# <scaffold-dir>/allowlist.manifest, taken from <scaffold-dir>/<tree-root>, and
# nothing else. The target must be an empty checkout: an existing directory with
# no entries, or a path that does not exist yet.
#
# This program is the copier, not the guard. It knows one rule -- copy the
# allowlist and only the allowlist -- and it deliberately does not screen paths
# or payloads. `tests/e2e/consumer/bootstrap/bootstrap.sh` is the guard: it
# validates the scaffold before calling this program and re-verifies the
# produced checkout afterwards, so a copier that leaks a file is caught by a
# program that shares no code with it.
#
# Output on success, in this order:
#   copied\t<path>            one line per entry, in sealed order
#   demo-scaffold/copied entries=<n>
#
# Output on refusal: a single `demo-scaffold/<code>[:<detail>]` line on stderr.
# A detail is a path or a count. Payload bytes are never printed.
#
# Exit codes: 0 copied, 1 typed refusal, 64 usage, 79 inconclusive environment.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly rc_red=1

refuse() {
  printf 'demo-scaffold/%s\n' "$1" >&2
  exit "$rc_red"
}

infra() {
  printf 'demo-scaffold/inconclusive/%s\n' "$1" >&2
  exit 79
}

usage() {
  printf 'demo-scaffold/usage: scaffold.sh <scaffold-dir> <target-dir>\n' >&2
  exit 64
}

for tool in cat find grep head mkdir printf sed sort; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing_tool:$tool"
done

(( $# == 2 )) || usage
scaffold_dir="$1"
target_dir="$2"
[[ -n "$scaffold_dir" && -n "$target_dir" ]] || usage
[[ -d "$scaffold_dir" && ! -L "$scaffold_dir" ]] || refuse "scaffold-dir-missing:$scaffold_dir"

manifest="$scaffold_dir/allowlist.manifest"
[[ -f "$manifest" && ! -L "$manifest" ]] || refuse 'manifest-missing'

head_line="$(head -n 1 "$manifest")"
[[ "$head_line" == 'schema: aurum.demo-scaffold-allowlist' ]] || refuse 'manifest-shape'
grep -qx 'version: 1' "$manifest" || refuse 'manifest-shape'

tree_root="$(sed -n 's/^tree-root: \([A-Za-z0-9][A-Za-z0-9._-]*\)$/\1/p' "$manifest")"
[[ -n "$tree_root" ]] || refuse 'manifest-shape'
tree_dir="$scaffold_dir/$tree_root"
[[ -d "$tree_dir" && ! -L "$tree_dir" ]] || refuse "tree-root-missing:$tree_root"

# The target must be an empty checkout. A non-empty one is refused before a
# single byte is written, so a failed bootstrap never half-populates a repo.
if [[ -e "$target_dir" || -L "$target_dir" ]]; then
  [[ -d "$target_dir" && ! -L "$target_dir" ]] || refuse 'target-not-a-directory'
  if [[ -n "$(find "$target_dir" -mindepth 1 -print -quit)" ]]; then
    refuse 'target-not-empty'
  fi
fi

# Parse the sealed entries. A line that does not match the exact entry shape
# stops the program instead of being skipped: a manifest this copier cannot
# read in full is a manifest it must not act on.
declare -a entry_paths=()
declared_entries="$(grep -c '^entry: ' "$manifest" || true)"
while IFS= read -r path; do
  [[ -n "$path" ]] || continue
  entry_paths+=("$path")
done < <(sed -n 's/^entry: [A-Za-z0-9-]\{1,\} [0-7]\{4\} [0-9]\{1,\} [0-9a-f]\{64\} \(.*\)$/\1/p' "$manifest" | sort)

(( ${#entry_paths[@]} == declared_entries )) || refuse "manifest-entry-parse:$declared_entries"
(( ${#entry_paths[@]} >= 1 )) || refuse 'manifest-empty'

# Phase 1: resolve every source. Nothing is written while this loop runs, so a
# missing or irregular source is refused before the first effect.
for path in "${entry_paths[@]}"; do
  source_path="$tree_dir/$path"
  [[ ! -L "$source_path" ]] || refuse "source-is-symlink:$path"
  [[ -f "$source_path" ]] || refuse "source-missing:$path"
  [[ -r "$source_path" ]] || refuse "source-unreadable:$path"
done

# Phase 2: apply. The order is the sealed order, so two runs of the same
# scaffold write the same paths in the same sequence.
mkdir -p "$target_dir" || infra 'target-mkdir'
for path in "${entry_paths[@]}"; do
  destination="$target_dir/$path"
  case "$path" in
    */*) mkdir -p "${destination%/*}" || infra 'destination-mkdir' ;;
  esac
  cat -- "$tree_dir/$path" >"$destination" || infra "copy:$path"
  printf 'copied\t%s\n' "$path"
done

printf 'demo-scaffold/copied entries=%d\n' "${#entry_paths[@]}"
