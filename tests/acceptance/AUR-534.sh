#!/usr/bin/env bash
#
# Acceptance program for card AUR-534.
#
# WHAT THIS PROVES
#
#   The toolchain moved from Go 1.21 (end of upstream support, 2024) to Go
#   1.27: go.mod, the repository Dockerfile, every GitHub workflow and the
#   board's go-* scripts agree on the new version and carry no leftover
#   "1.21" reference (AC-001); the suite materialized into this container
#   actually builds and runs under the new toolchain (AC-002); Dependabot
#   watches gomod, docker and github-actions on a schedule so the next bump
#   is automation's job, not a human's (AC-003); the pinned toolchain image
#   really was built from the versioned Dockerfile this card adds under
#   .board/oci/images/, from the repository's own go.mod/go.sum, not from an
#   image nobody can reproduce (AC-004).
#
# SELECTORS
#   all             every scenario below
#   AC-001          go.mod / Dockerfile / workflows / go-* scripts: same Go
#                   version, no stray "1.21"
#   AC-002          go build ./... and go test -count=1 ./... pass under the
#                   toolchain actually running this container
#   AC-003          .github/dependabot.yml covers gomod, docker (both the
#                   root context and the new image's own directory) and
#                   github-actions, each on a schedule
#   AC-004          the running image's baked Dockerfile/go.mod/go.sum are
#                   byte-identical to the versioned ones materialized here
#   AC-001-MUT-001  reintroducing "1.21" into a scratch copy of a workflow
#                   makes the AC-001 detector itself report the violation --
#                   proves the check is not vacuously green
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving mutation)
#   64 = unknown selector
#   79 = inconclusive / infrastructure. Never valid red evidence.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-534'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

readonly image_dir='.board/oci/images/go-bash-1.27'
readonly image_dockerfile="$repo_root/$image_dir/Dockerfile"

# This card's own declared deliverables (its `paths:`). Their absence is the
# card's own behavior failing to exist, not an environment gap.
owned_inputs=(
  go.mod go.sum Dockerfile Makefile
  .github/workflows/ci.yml .github/dependabot.yml
  "$image_dir/Dockerfile"
  .board/locks/oci/go-unit-offline-v1.lock.json
  .board/oci/profiles/go-unit-offline-v1.json
  .board/bin/go-shared .board/bin/go-sealed .board/bin/go-live
  .board/bootstrap/locks/go.yml .board/bootstrap/locks.yml
)
for input in "${owned_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || fail "missing-deliverable:$input"
done

# cmd/internal/pkg are read_paths, not this card's own deliverable: absence
# here is a materialization gap, not red evidence.
for input in cmd internal pkg; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a534.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP

# scan_for_121 prints every line of $1 that still names the retired Go 1.21
# toolchain. Empty output means clean. Matches the literal version string in
# the shapes this repository actually uses it (go directive, image tag,
# bare "1.21" token) without flagging an unrelated "1.21" substring inside a
# longer version number such as "1.215" or "1.210".
scan_for_121() {
  grep -nE '(^|[^0-9.])1\.21([^0-9]|$)' -- "$1" 2>/dev/null || true
}

# check_ac001 takes the root to check against (default: the real worktree).
# AC-001-MUT-001 passes a scratch copy with ci.yml mutated back to 1.21, and
# requires THIS function -- the actual AC-001 gate, not a private helper --
# to report the regression.
check_ac001() {
  local root="${1:-$repo_root}"
  local hits=0 line

  grep -Fxq 'go 1.27' "$root/go.mod" || { printf 'go.mod does not declare go 1.27\n' >&2; hits=1; }

  grep -Fq 'golang:1.27.1-alpine3.24@sha256:' "$root/Dockerfile" ||
    { printf 'Dockerfile does not pin golang 1.27.1-alpine3.24 by digest\n' >&2; hits=1; }

  grep -Fq 'golang:1.27.1-alpine3.24@sha256:' "$root/$image_dir/Dockerfile" ||
    { printf '%s does not pin golang 1.27.1-alpine3.24 by digest\n' "$image_dir/Dockerfile" >&2; hits=1; }

  grep -Fq 'golang:1.27.1-alpine3.24@sha256:' "$root/.github/workflows/ci.yml" ||
    { printf 'ci.yml build-and-test job does not pin golang 1.27.1-alpine3.24\n' >&2; hits=1; }
  grep -Fq 'golang:1.27.1-bookworm@sha256:' "$root/.github/workflows/ci.yml" ||
    { printf 'ci.yml race job does not pin golang 1.27.1-bookworm\n' >&2; hits=1; }

  grep -Fq 'golang:1.27.1-alpine3.24@sha256:' "$root/Makefile" ||
    { printf 'Makefile GO_IMAGE does not pin golang 1.27.1-alpine3.24\n' >&2; hits=1; }
  grep -Fq 'golang:1.27.1-bookworm@sha256:' "$root/Makefile" ||
    { printf 'Makefile test-race target does not pin golang 1.27.1-bookworm\n' >&2; hits=1; }

  # Every workflow file, not just ci.yml -- a stray 1.21 pin in any other
  # workflow (examples/ included) must fail this just as loudly. Makefile
  # too: it carries its own golang image pins, independent of ci.yml.
  local scan_targets=(go.mod Dockerfile Makefile "$image_dir/Dockerfile"
                       .board/bin/go-shared .board/bin/go-sealed .board/bin/go-live)
  local wf
  while IFS= read -r -d '' wf; do
    scan_targets+=("${wf#"$root"/}")
  done < <(find "$root/.github/workflows" -type f \( -name '*.yml' -o -name '*.yaml' \) -print0 2>/dev/null)

  local target
  for target in "${scan_targets[@]}"; do
    [[ -f "$root/$target" ]] || continue
    while IFS= read -r line; do
      [[ -z "$line" ]] && continue
      printf 'stray 1.21 reference: %s: %s\n' "$target" "$line" >&2
      hits=1
    done < <(scan_for_121 "$root/$target")
    # A setup-go/go-version action pinned to anything other than 1.27 would
    # not match the "1.21" literal scan above; catch that shape too.
    while IFS= read -r line; do
      [[ -z "$line" ]] && continue
      [[ "$line" == *'1.27'* ]] && continue
      printf 'go-version action pin is not 1.27: %s: %s\n' "$target" "$line" >&2
      hits=1
    done < <(grep -nE 'go-version:' "$root/$target" 2>/dev/null || true)
  done

  (( hits == 0 ))
}

check_ac002() {
  command -v go >/dev/null 2>&1 || infra missing_go

  # AC-002 promises "a suite inteira (go test ./...)". Card v4 widened this
  # card's own read_paths to [cmd, internal, pkg, action.yml, scripts,
  # .board/bin, tests, demo, standards, docs/specs, .board/schemas,
  # .board/research, .board/decisions], which materializes everything the
  # whole suite reads (tests/fixtures/repos/git-demo/repo.git,
  # tests/integration, .board/schemas/task-spec.schema.json included) --
  # verified independently: oci-run now runs the full tree inside the
  # sandbox (560 PASS / 7 SKIP, identical to an unsandboxed run). A package
  # that genuinely fails to resolve here is therefore a real compile
  # defect, not a materialization gap: every go build/test failure is
  # behavioral red (1), never infra.
  local build_log="$run_dir/ac002-build.log"
  if ! ( cd "$repo_root" && go build -buildvcs=false ./... ) >"$build_log" 2>&1; then
    cat "$build_log" >&2
    printf 'go build ./... failed\n' >&2
    return 1
  fi
  ( cd "$repo_root" && go test -count=1 ./... ) || { printf 'go test ./... failed\n' >&2; return 1; }
  return 0
}

check_ac003() {
  local dep="$repo_root/.github/dependabot.yml"
  [[ -f "$dep" ]] || fail missing-deliverable:.github/dependabot.yml
  grep -Fq 'package-ecosystem: gomod' "$dep" || { printf 'dependabot.yml missing gomod ecosystem\n' >&2; return 1; }
  grep -Fq 'package-ecosystem: docker' "$dep" || { printf 'dependabot.yml missing docker ecosystem\n' >&2; return 1; }
  grep -Fq 'package-ecosystem: github-actions' "$dep" || { printf 'dependabot.yml missing github-actions ecosystem\n' >&2; return 1; }
  # Docker coverage must include both the root build context and the new
  # toolchain image's own directory -- two docker entries, not one.
  local docker_entries
  docker_entries="$(grep -c 'package-ecosystem: docker' "$dep")"
  (( docker_entries >= 2 )) || { printf 'dependabot.yml has only %s docker entry(ies), expected >= 2\n' "$docker_entries" >&2; return 1; }
  grep -Fq "directory: \"/$image_dir\"" "$dep" || { printf 'dependabot.yml does not watch %s\n' "$image_dir" >&2; return 1; }
  # Every ecosystem entry declares a schedule; absence of any "interval:" at
  # all is the cheapest real signal that a block was pasted without one.
  grep -Fq 'interval:' "$dep" || { printf 'dependabot.yml has no schedule interval\n' >&2; return 1; }
  local schedules
  schedules="$(grep -c 'interval:' "$dep")"
  (( schedules >= 4 )) || { printf 'dependabot.yml has only %s scheduled update(s), expected >= 4\n' "$schedules" >&2; return 1; }
  return 0
}

check_ac004() {
  local baked_dockerfile='/opt/aurum-a006/toolchain.Dockerfile'
  local baked_gomod='/opt/aurum-a006/go.mod'
  local baked_gosum='/opt/aurum-a006/go.sum'

  [[ -f "$baked_dockerfile" ]] || infra missing_baked_dockerfile
  [[ -f "$baked_gomod" && -f "$baked_gosum" ]] || infra missing_baked_module_files

  cmp -s "$baked_dockerfile" "$image_dockerfile" ||
    { printf 'running image Dockerfile differs from %s\n' "$image_dir/Dockerfile" >&2; return 1; }
  cmp -s "$baked_gomod" "$repo_root/go.mod" ||
    { printf 'running image go.mod differs from the repository go.mod\n' >&2; return 1; }
  cmp -s "$baked_gosum" "$repo_root/go.sum" ||
    { printf 'running image go.sum differs from the repository go.sum\n' >&2; return 1; }

  command -v go >/dev/null 2>&1 || infra missing_go
  local want_version actual_version
  want_version="$(awk '$1 == "go" { print $2; exit }' "$repo_root/go.mod")"
  [[ -n "$want_version" ]] || infra unreadable_go_directive
  actual_version="$(go version)"
  grep -Fq "go$want_version" <<< "$actual_version" ||
    { printf 'running go toolchain (%s) does not match go.mod directive (go %s)\n' "$actual_version" "$want_version" >&2; return 1; }
  return 0
}

check_ac001_mut001() {
  # Copy every input check_ac001 reads into a scratch root (clean, matching
  # the real worktree), then mutate ONE file at a time back to 1.21 and
  # require check_ac001 itself -- the actual AC-001 gate, not a private
  # helper -- to report each regression independently. Two files are
  # exercised because Makefile and ci.yml carry independent golang pins;
  # proving the ci.yml mutation trips the detector would say nothing about
  # whether Makefile is actually being scanned.
  local mutant_root="$run_dir/mutant"
  mkdir -p "$mutant_root/.github/workflows" "$mutant_root/$image_dir" "$mutant_root/.board/bin"
  cp -- "$repo_root/go.mod" "$mutant_root/go.mod"
  cp -- "$repo_root/Dockerfile" "$mutant_root/Dockerfile"
  cp -- "$repo_root/Makefile" "$mutant_root/Makefile"
  cp -- "$repo_root/.github/workflows/ci.yml" "$mutant_root/.github/workflows/ci.yml"
  cp -- "$image_dockerfile" "$mutant_root/$image_dir/Dockerfile"
  cp -- "$repo_root/.board/bin/go-shared" "$mutant_root/.board/bin/go-shared"
  cp -- "$repo_root/.board/bin/go-sealed" "$mutant_root/.board/bin/go-sealed"
  cp -- "$repo_root/.board/bin/go-live" "$mutant_root/.board/bin/go-live"

  # Control: the clean scratch copy must itself pass AC-001 before either
  # mutation runs. Without this, a copying mistake (a stale file, a missing
  # one) could make check_ac001 fail on the UNMUTATED tree, and the
  # mutation "succeeding" below would prove nothing -- the function would
  # already have been red for the wrong reason.
  if ! check_ac001 "$mutant_root" 2>/dev/null; then
    printf 'clean scratch copy failed check_ac001 before any mutation; the control is broken\n' >&2
    return 1
  fi

  # Scenario 1: reintroduce the retired version into ci.yml's race job base
  # image only; Makefile stays clean.
  sed -i 's/golang:1\.27\.1-bookworm@sha256:[0-9a-f]*/golang:1.21-bookworm/' \
    "$mutant_root/.github/workflows/ci.yml"
  if check_ac001 "$mutant_root" 2>/dev/null; then
    printf 'ci.yml mutation did not trip check_ac001; detector is vacuous\n' >&2
    return 1
  fi
  cp -- "$repo_root/.github/workflows/ci.yml" "$mutant_root/.github/workflows/ci.yml"

  # Scenario 2: restore ci.yml, reintroduce the retired version into
  # Makefile's race target only.
  sed -i 's/golang:1\.27\.1-bookworm@sha256:[0-9a-f]*/golang:1.21-bookworm/' \
    "$mutant_root/Makefile"
  if check_ac001 "$mutant_root" 2>/dev/null; then
    printf 'Makefile mutation did not trip check_ac001; Makefile is not actually scanned\n' >&2
    return 1
  fi
  return 0
}

run_selector() {
  case "$1" in
    AC-001) check_ac001 || fail ac001-stale-or-mismatched-version ;;
    AC-002) check_ac002 || fail ac002-build-or-test-failed ;;
    AC-003) check_ac003 || fail ac003-dependabot-incomplete ;;
    AC-004) check_ac004 || fail ac004-image-does-not-match-dockerfile ;;
    AC-001-MUT-001) check_ac001_mut001 || fail mut001-detector-did-not-trip ;;
  esac
}

if [[ "$selector" == 'all' ]]; then
  for s in AC-001 AC-002 AC-003 AC-004 AC-001-MUT-001; do
    run_selector "$s"
  done
  printf '%s/all/pass\n' "$card"
else
  run_selector "$selector"
  printf '%s/%s/pass\n' "$card" "$selector"
fi
