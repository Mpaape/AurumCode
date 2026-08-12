#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C

# Keep this gate byte-for-byte aligned with .board/bin/oci-run. A credential-
# shaped literal in any declared input would otherwise let a builder finish and
# fail only when the complete candidate first reaches OCI materialization.
readonly credential_pattern='(-----BEGIN[[:space:]][A-Z ]*PRIVATE KEY-----|AKIA[0-9A-Z]{16}|sk-[A-Za-z0-9_-]{20,}|gh[pousr]_[A-Za-z0-9]{20,})'

# This is a dispatch gate, not a report generator. It deliberately repeats the
# runtime checks performed by oci-run so a stale agent cannot turn an impossible
# acceptance contract into a doing or validating card.

card_id="${1:-}"
worktree="${2:-$(pwd -P)}"
[[ "$card_id" =~ ^AUR-[0-9]{3}$ ]] || { printf 'preflight error: use AUR-NNN\n' >&2; exit 1; }
[[ -d "$worktree/.git" || -f "$worktree/.git" ]] || { printf 'preflight error: not a Git worktree\n' >&2; exit 1; }

states=(backlog ready doing review validating done blocked-on-owner cancelled)
card=''
for state in "${states[@]}"; do
  candidate="$worktree/.board/cards/$state/$card_id.md"
  if [[ -f "$candidate" ]]; then
    [[ -z "$card" ]] || { printf 'preflight error: duplicate card state\n' >&2; exit 1; }
    card="$candidate"
  fi
done
[[ -n "$card" ]] || { printf 'preflight error: card not found: %s\n' "$card_id" >&2; exit 1; }

frontmatter_value() {
  local key="$1"
  awk -v key="$key" '
    NR == 1 { if ($0 != "---") exit 2; in_fm=1; next }
    in_fm && $0 == "---" { exit }
    in_fm && index($0, key ":") == 1 {
      count++
      value=substr($0, length(key) + 2)
      sub(/^[[:space:]]+/, "", value)
    }
    END { if (count != 1) exit 3; print value }
  ' "$card"
}

parse_list() {
  local value="$1" body item old_ifs
  [[ "$value" =~ ^\[(.*)\]$ ]] || return 1
  body="${BASH_REMATCH[1]}"
  [[ -n "$body" ]] || return 0
  old_ifs="$IFS"
  IFS=','
  read -ra items <<< "$body"
  IFS="$old_ifs"
  for item in "${items[@]}"; do
    item="${item# }"
    item="${item% }"
    [[ -n "$item" ]] || return 1
    printf '%s\n' "$item"
  done
}

path_is_within() {
  [[ "$1" == "$2" || "$1" == "$2/"* ]]
}

tracked_path_exists() {
  local path="$1"
  [[ -e "$worktree/$path" && ! -L "$worktree/$path" ]] || return 1
  if [[ -f "$worktree/$path" ]]; then
    git -C "$worktree" ls-files --error-unmatch -- "$path" >/dev/null 2>&1
  else
    [[ -n "$(git -C "$worktree" ls-files -- "$path")" ]]
  fi
}

status="$(frontmatter_value status)" || { printf 'preflight error: unreadable status\n' >&2; exit 1; }
validation="$(frontmatter_value validation 2>/dev/null || printf 'none')"
case "$status" in
  ready|doing|review|validating) ;;
  *) printf 'preflight error: card is not dispatchable: status=%s\n' "$status" >&2; exit 1 ;;
esac
case "$validation" in
  none|tested|skeptical) ;;
  *) printf 'preflight error: invalid validation kind: %s\n' "$validation" >&2; exit 1 ;;
esac

# A ready card is a builder candidate, not yet an immutable implementation.
# Its declared paths and acceptance may legitimately be absent; requiring them
# here makes every new card impossible to dispatch. Review/validation still
# require the complete tracked candidate below.
builder_preflight=0
if [[ "$status" == ready ]]; then
  builder_preflight=1
fi

[[ -z "$(git -C "$worktree" status --porcelain)" ]] || {
  printf 'preflight error: worktree is dirty; validate an immutable candidate\n' >&2
  exit 1
}

owned_spec="$(frontmatter_value paths)" || {
  printf 'preflight error: malformed paths\n' >&2
  exit 1
}
[[ "$owned_spec" =~ ^\[(|[A-Za-z0-9._/-]+(,[[:space:]]*[A-Za-z0-9._/-]+)*)\]$ ]] || {
  printf 'preflight error: malformed paths\n' >&2
  exit 1
}
[[ "$owned_spec" != '[]' ]] || {
  printf 'preflight error: paths must own at least one artifact\n' >&2
  exit 1
}
read_spec="$(frontmatter_value read_paths 2>/dev/null || printf '[]')"
[[ "$read_spec" =~ ^\[(|[A-Za-z0-9._/-]+(,[[:space:]]*[A-Za-z0-9._/-]+)*)\]$ ]] || {
  printf 'preflight error: malformed read_paths\n' >&2
  exit 1
}
mapfile -t owned_paths < <(parse_list "$owned_spec")
mapfile -t read_paths < <(parse_list "$read_spec")

materializes_path() {
  local target="$1" declared
  for declared in "${owned_paths[@]}" "${read_paths[@]}"; do
    if path_is_within "$target" "$declared"; then
      return 0
    fi
  done
  return 1
}

acceptance_surface="$worktree/tests/acceptance/$card_id.sh"
if grep -Eq '\.go([`:[:space:]]|$)|(^|[[:space:];|&])go[[:space:]]+(test|run|build|vet)' "$card" ||
  { [[ -f "$acceptance_surface" ]] && grep -Eq '(^|[[:space:];|&])go[[:space:]]+(test|run|build|vet)' "$acceptance_surface"; }; then
  materializes_path go.mod || {
    printf 'preflight error: Go card does not materialize go.mod in paths/read_paths\n' >&2
    exit 1
  }
  if tracked_path_exists go.sum && ! materializes_path go.sum; then
    printf 'preflight error: Go card does not materialize go.sum in paths/read_paths\n' >&2
    exit 1
  fi
fi

# A reviewed candidate remains in ready until approval. Once every owned path is
# tracked, ready must no longer receive the relaxed builder checks; otherwise a
# reviewer can miss a non-executable acceptance or an absent candidate artifact.
if (( builder_preflight == 1 )); then
  complete_candidate=1
  for owned_path in "${owned_paths[@]}"; do
    if ! tracked_path_exists "$owned_path"; then
      complete_candidate=0
      break
    fi
  done
  if (( complete_candidate == 1 )); then
    builder_preflight=0
  fi
fi

acceptance_path="tests/acceptance/$card_id.sh"
acceptance_owned=0
for owned_path in "${owned_paths[@]}"; do
  if path_is_within "$acceptance_path" "$owned_path"; then
    acceptance_owned=1
  fi
done
(( acceptance_owned == 1 )) || {
  printf 'preflight error: acceptance is outside owned paths: %s\n' "$acceptance_path" >&2
  exit 1
}

for path in "${owned_paths[@]}"; do
  [[ "$path" != /* && "$path" != *..* && "$path" != *' '* ]] || {
    printf 'preflight error: unsafe owned path: %s\n' "$path" >&2
    exit 1
  }
  if (( builder_preflight == 0 )); then
    tracked_path_exists "$path" || {
      printf 'preflight error: active candidate path is missing or untracked: %s\n' "$path" >&2
      exit 1
    }
  fi
done
for read_path in "${read_paths[@]}"; do
  tracked_path_exists "$read_path" || {
    printf 'preflight error: read_path is missing or untracked: %s\n' "$read_path" >&2
    exit 1
  }
  for owned_path in "${owned_paths[@]}"; do
    if path_is_within "$read_path" "$owned_path" || path_is_within "$owned_path" "$read_path"; then
      printf 'preflight error: read_path overlaps owned path: %s <> %s\n' "$read_path" "$owned_path" >&2
      exit 1
    fi
  done
done

# oci-run materializes exactly the tracked files below paths/read_paths and
# refuses credential-shaped bytes before starting the worker. Mirror that scan
# during every dispatch so synthetic fixtures are generated at runtime (or
# split in source) before implementation begins, while real-looking material
# remains fail-closed.
mapfile -d '' -t materialized_files < <(
  git -C "$worktree" ls-files -z -- "${owned_paths[@]}" "${read_paths[@]}"
)
for materialized_path in "${materialized_files[@]}"; do
  materialized_source="$worktree/$materialized_path"
  [[ -f "$materialized_source" && ! -L "$materialized_source" ]] || continue
  if grep -Eiq -- "$credential_pattern" "$materialized_source"; then
    printf 'preflight error: credential pattern in declared materialized input: %s\n' "$materialized_path" >&2
    exit 1
  fi
done

acceptance="$worktree/tests/acceptance/$card_id.sh"
if (( builder_preflight == 0 )); then
  [[ -f "$acceptance" && ! -L "$acceptance" && -x "$acceptance" ]] || {
    printf 'preflight error: tested card lacks executable acceptance: %s\n' "$acceptance" >&2
    exit 1
  }
  bash -n "$acceptance" || { printf 'preflight error: acceptance syntax failed\n' >&2; exit 1; }
fi
grep -Eiq 'MUT|mutation|mutat' "$card" || {
  printf 'preflight error: card has no declared mutation/skeptic signal\n' >&2
  exit 1
}

profile="$(sed -n 's/^container_profile: `\([^`]*\)`$/\1/p' "$card")"
accept_command="$(sed -n 's/^accept: `\(.*\)`$/\1/p' "$card")"
[[ "$profile" =~ ^[a-z0-9][a-z0-9.-]{2,63}$ ]] || {
  printf 'preflight error: missing or invalid container_profile\n' >&2
  exit 1
}
expected_accept="./.board/bin/oci-run --profile $profile --card $card_id"
[[ "$accept_command" == "$expected_accept" ]] || {
  printf 'preflight error: accept must be exactly: %s\n' "$expected_accept" >&2
  exit 1
}

profile_file="$worktree/.board/oci/profiles/$profile.json"
lock_file="$worktree/.board/locks/oci/$profile.lock.json"
[[ -f "$profile_file" && ! -L "$profile_file" ]] || {
  printf 'preflight error: profile is not registered: %s\n' "$profile" >&2
  exit 1
}
[[ -f "$lock_file" && ! -L "$lock_file" ]] || {
  printf 'preflight error: profile lock is missing: %s\n' "$lock_file" >&2
  exit 1
}
image="$(sed -n 's/^[[:space:]]*"image"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$lock_file")"
user="$(sed -n 's/^[[:space:]]*"user"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$profile_file")"
profile_name="$(sed -n 's/^[[:space:]]*"profile"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$profile_file")"
profile_lock="$(sed -n 's/^[[:space:]]*"lock"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$profile_file")"
profile_lock_digest="$(sed -n 's/^[[:space:]]*"lock_digest"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$profile_file")"
lock_profile="$(sed -n 's/^[[:space:]]*"profile"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$lock_file")"
lock_schema="$(sed -n 's/^[[:space:]]*"schema"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$lock_file")"
lock_version="$(sed -n 's/^[[:space:]]*"version"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p' "$lock_file")"
[[ "$image" =~ ^[a-z0-9][a-z0-9._/-]*@sha256:[0-9a-f]{64}$ ]] || {
  printf 'preflight error: profile image is not digest pinned\n' >&2
  exit 1
}
[[ "$user" =~ ^[1-9][0-9]{2,6}:[1-9][0-9]{2,6}$ ]] || {
  printf 'preflight error: profile user is not non-root numeric\n' >&2
  exit 1
}
[[ "$profile_name" == "$profile" && "$lock_profile" == "$profile" ]] || {
  printf 'preflight error: profile and lock names do not bind to %s\n' "$profile" >&2
  exit 1
}
[[ "$profile_lock" == ".board/locks/oci/$profile.lock.json" ]] || {
  printf 'preflight error: profile points at a foreign lock\n' >&2
  exit 1
}
[[ "$lock_schema" == 'aurum.oci-image-lock' && "$lock_version" == 1 ]] || {
  printf 'preflight error: profile lock schema/version is invalid\n' >&2
  exit 1
}
actual_lock_digest="sha256:$(sha256sum -- "$lock_file" | awk '{print $1}')"
[[ "$profile_lock_digest" == "$actual_lock_digest" ]] || {
  printf 'preflight error: profile lock_digest does not match lock bytes\n' >&2
  exit 1
}

# Per-key extraction above cannot tell whether the runner will accept the
# document at all. AUR-023 burned a builder proving that: preflight and the
# pipeline both exited 0 for a profile whose first contact with oci-run was
# `die 65 ... is not a canonical flat JSON object`. The runner's contract is
# read out of the runner itself rather than restated here, so the two cannot
# drift apart.
runner_profile_schema="$(sed -n "s/^[[:space:]]*require_literal profile_doc \"\$profile_label\" schema s '\([^']*\)'.*/\1/p" "$worktree/.board/bin/oci-run" | head -1)"
[[ -n "$runner_profile_schema" ]] || {
  printf 'preflight error: cannot read the runner profile schema contract from .board/bin/oci-run\n' >&2
  exit 1
}
runner_profile_keys="$(awk '
  /^[[:space:]]*require_exact_keys profile_doc/ { collecting = 1 }
  collecting {
    line = $0
    sub(/^[[:space:]]*require_exact_keys profile_doc[^\\]*\\?[[:space:]]*$/, "", line)
    gsub(/\\$/, "", line)
    printf "%s ", line
    if ($0 !~ /\\[[:space:]]*$/) { exit }
  }
' "$worktree/.board/bin/oci-run" | tr -s ' \t' '\n' | grep -Ev '^(require_exact_keys|profile_doc|"\$profile_label"|)$' | sort -u)"
[[ -n "$runner_profile_keys" ]] || {
  printf 'preflight error: cannot read the runner profile key set from .board/bin/oci-run\n' >&2
  exit 1
}
profile_schema="$(sed -n 's/^[[:space:]]*"schema"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$profile_file" | head -1)"
[[ "$profile_schema" == "$runner_profile_schema" ]] || {
  printf 'preflight error: profile %s declares schema %s but the runner only loads %s; the acceptance would die 65 before any container\n' \
    "$profile" "$profile_schema" "$runner_profile_schema" >&2
  exit 1
}
profile_keys="$(sed -n 's/^[[:space:]]*"\([A-Za-z_][A-Za-z0-9_]*\)"[[:space:]]*:.*/\1/p' "$profile_file" | sort -u)"
unknown_keys="$(comm -23 <(printf '%s\n' "$profile_keys") <(printf '%s\n' "$runner_profile_keys") | tr '\n' ' ')"
[[ -z "${unknown_keys// /}" ]] || {
  printf 'preflight error: profile %s carries keys the runner rejects as unknown: %s\n' \
    "$profile" "${unknown_keys% }" >&2
  exit 1
}
grep -Eq '^[[:space:]]*"[A-Za-z_][A-Za-z0-9_]*"[[:space:]]*:[[:space:]]*[[{]' "$profile_file" && {
  printf 'preflight error: profile %s is not a canonical flat JSON object; the runner refuses nested values\n' "$profile" >&2
  exit 1
}

if command -v podman >/dev/null 2>&1; then
  engine=podman
elif command -v docker >/dev/null 2>&1; then
  engine=docker
else
  printf 'preflight infrastructure: neither Podman nor Docker is available\n' >&2
  exit 79
fi
if [[ -n "${AURUM_OCI_ENGINE:-}" ]]; then
  engine="$AURUM_OCI_ENGINE"
  [[ "$engine" == docker || "$engine" == podman ]] || {
    printf 'preflight error: AURUM_OCI_ENGINE must be docker or podman\n' >&2
    exit 1
  }
  command -v "$engine" >/dev/null 2>&1 || {
    printf 'preflight infrastructure: requested OCI engine is unavailable: %s\n' "$engine" >&2
    exit 79
  }
fi

"$engine" image inspect "$image" >/dev/null 2>&1 || {
  printf 'preflight infrastructure: pinned image is not present locally: %s\n' "$image" >&2
  exit 79
}

runtime_probe='set -eu; command -v bash >/dev/null'
required_tools='bash'
if (( builder_preflight == 0 )) && grep -Eq 'command[[:space:]]+-v[[:space:]]+go|(^|[[:space:];|&])go[[:space:]]+(test|run|build|vet)' "$acceptance"; then
  runtime_probe="$runtime_probe; command -v go >/dev/null"
  required_tools="$required_tools,go"
fi
set +e
probe_output="$($engine run --rm --pull=never --network=none --read-only \
  --user="$user" --cap-drop=ALL --security-opt=no-new-privileges \
  --pids-limit=64 --memory=128m --cpus=0.5 \
  --tmpfs '/tmp:rw,nosuid,nodev,size=128m' --entrypoint= \
  "$image" bash -c "$runtime_probe" 2>&1)"
probe_rc=$?
set -e
if (( probe_rc == 125 )); then
  printf 'preflight infrastructure: runtime probe could not start for %s: %s\n' "$image" "$probe_output" >&2
  exit 79
fi
(( probe_rc == 0 )) || {
  printf 'preflight error: profile %s lacks required runtime (%s)\n' "$profile" "$required_tools" >&2
  exit 1
}

# A profile-owner card is accepted through an already-registered bootstrap
# profile, so probing only the acceptance profile misses an unusable image in
# the profile being published. Once the candidate is complete, probe every
# owned profile document with the same Bash entrypoint contract used by
# oci-run, plus Go when its plan declares a Go command/cache.
if (( builder_preflight == 0 )); then
  for owned_profile_path in "${materialized_files[@]}"; do
    case "$owned_profile_path" in
      .board/oci/profiles/*.json) ;;
      *) continue ;;
    esac
    owned_profile_file="$worktree/$owned_profile_path"
    owned_profile_name="$(sed -n 's/^[[:space:]]*"profile"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$owned_profile_file")"
    [[ -n "$owned_profile_name" && "$owned_profile_name" != "$profile" ]] || continue
    owned_profile_lock="$(sed -n 's/^[[:space:]]*"lock"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$owned_profile_file")"
    owned_user="$(sed -n 's/^[[:space:]]*"user"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$owned_profile_file")"
    owned_lock_digest="$(sed -n 's/^[[:space:]]*"lock_digest"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$owned_profile_file")"
    [[ "$owned_profile_name" =~ ^[a-z0-9][a-z0-9.-]{2,63}$ &&
       "$owned_profile_lock" == ".board/locks/oci/$owned_profile_name.lock.json" &&
       "$owned_user" =~ ^[1-9][0-9]{2,6}:[1-9][0-9]{2,6}$ ]] || {
      printf 'preflight error: owned profile contract is invalid: %s\n' "$owned_profile_path" >&2
      exit 1
    }
    owned_lock_file="$worktree/$owned_profile_lock"
    [[ -f "$owned_lock_file" && ! -L "$owned_lock_file" ]] || {
      printf 'preflight error: owned profile lock is missing: %s\n' "$owned_profile_lock" >&2
      exit 1
    }
    owned_lock_profile="$(sed -n 's/^[[:space:]]*"profile"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$owned_lock_file")"
    owned_image="$(sed -n 's/^[[:space:]]*"image"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$owned_lock_file")"
    owned_actual_lock_digest="sha256:$(sha256sum -- "$owned_lock_file" | awk '{print $1}')"
    [[ "$owned_lock_profile" == "$owned_profile_name" &&
       "$owned_lock_digest" == "$owned_actual_lock_digest" &&
       "$owned_image" =~ ^[a-z0-9][a-z0-9._/-]*@sha256:[0-9a-f]{64}$ ]] || {
      printf 'preflight error: owned profile lock binding is invalid: %s\n' "$owned_profile_name" >&2
      exit 1
    }
    "$engine" image inspect "$owned_image" >/dev/null 2>&1 || {
      printf 'preflight infrastructure: owned profile image is not present locally: %s\n' "$owned_image" >&2
      exit 79
    }
    owned_runtime_probe='set -eu; command -v bash >/dev/null'
    owned_required_tools='bash'
    if grep -Eq '"module_cache"|"command"[[:space:]]*:[[:space:]]*\[[[:space:]]*"go"' "$owned_profile_file"; then
      owned_runtime_probe="$owned_runtime_probe; command -v go >/dev/null"
      owned_required_tools='bash,go'
    fi
    set +e
    owned_probe_output="$($engine run --rm --pull=never --network=none --read-only \
      --user="$owned_user" --cap-drop=ALL --security-opt=no-new-privileges \
      --pids-limit=64 --memory=128m --cpus=0.5 \
      --tmpfs '/tmp:rw,nosuid,nodev,size=128m' --entrypoint= \
      "$owned_image" bash -c "$owned_runtime_probe" 2>&1)"
    owned_probe_rc=$?
    set -e
    if (( owned_probe_rc == 125 )); then
      printf 'preflight infrastructure: owned profile runtime probe could not start for %s: %s\n' "$owned_image" "$owned_probe_output" >&2
      exit 79
    fi
    (( owned_probe_rc == 0 )) || {
      printf 'preflight error: owned profile %s lacks required runtime (%s)\n' "$owned_profile_name" "$owned_required_tools" >&2
      exit 1
    }
  done
fi

if [[ "${PREFLIGHT_RUN:-0}" == 1 && "$builder_preflight" == 0 ]]; then
  set +e
  (cd "$worktree" && ./.board/bin/oci-run --profile "$profile" --card "$card_id")
  acceptance_rc=$?
  set -e
  case "$acceptance_rc" in
    0) ;;
    3|69|79|124)
      printf 'preflight infrastructure: acceptance unavailable (exit %s)\n' "$acceptance_rc" >&2
      exit 69
      ;;
    *)
      printf 'preflight error: acceptance setup failed (exit %s)\n' "$acceptance_rc" >&2
      exit 1
      ;;
  esac
fi

printf 'preflight ok: %s status=%s validation=%s profile=%s runtime=%s worktree=%s\n' \
  "$card_id" "$status" "$validation" "$profile" "$required_tools" "$worktree"
