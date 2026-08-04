#!/usr/bin/env bash
#
# Behavioral test program for the `documentation` mode of the GitHub Action
# entrypoint and the two helper scripts it drives.
#
# WHAT THIS PROVES
#
#   `documentation` is the only mode the image built from this repository's
#   Dockerfile can actually serve: that image ships exactly one binary,
#   /app/regenerate-docs. The mode is therefore only allowed to report success
#   when THAT generator ran and left documentation derived from the scanned
#   source tree on disk.
#
#   The failure this program was written against was a green run that generated
#   nothing: generate-enhanced-docs.sh returned 0 on "No Go files changed",
#   build-docs-site.sh wrote a hardcoded homepage unconditionally, the
#   entrypoint's "did the build produce a site" check was satisfied by that
#   hardcoded homepage, and the real generator was never invoked at all.
#
# WHAT THIS DELIBERATELY REFUSES TO TRUST
#
#   No word printed by a script is a verdict here. Each case asserts on the raw
#   exit status, and on artifacts re-derived from disk. The static homepage
#   template is explicitly treated as NOT evidence of a generator run, because
#   producing it requires no source tree, no generator and no work.
#
# EXIT CODES (disjoint, mirroring tests/acceptance/*.sh):
#   0  every selected case ran AND held; the count is printed and can be checked
#   1  behavioral RED: at least one case ran and failed
#   3  harness or environment error - missing git, unwritable TMPDIR, or a
#      selected case that never reached a verdict. This status means NOTHING
#      WAS MEASURED and is never evidence that the scripts under test are sound
#   64 unknown selector
#
# The scratch workspaces are always created under TMPDIR. Nothing is written
# inside the repository.

set -uo pipefail
export LC_ALL=C

selector="${1:-ALL}"
case "$selector" in
  ALL|AC-0|AC-1|AC-2|AC-3|AC-4|AC-5|AC-6|AC-7|AC-8|AC-9|AC-10|AC-11|AC-12|AC-13|AC-14|AC-15|AC-16|AC-17) ;;
  *) printf 'documentation-mode/unknown-selector: %s\n' "$selector" >&2; exit 64 ;;
esac

SCRIPTS_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
ENTRYPOINT="$SCRIPTS_DIR/action-entrypoint.sh"
BUILD_SITE="$SCRIPTS_DIR/build-docs-site.sh"
ENHANCE="$SCRIPTS_DIR/generate-enhanced-docs.sh"

# Exit statuses this suite expects the scripts under test to use.
readonly EXPECT_BEHAVIOR=1
readonly EXPECT_ENVIRONMENT=3
readonly EXPECT_NOOP=20

env_error() {
  printf 'ENVIRONMENT: %s\n' "$1" >&2
  exit 3
}

command -v git >/dev/null 2>&1 || env_error 'git is not installed; this suite needs it to build a workspace'
[ -f "$ENTRYPOINT" ] || env_error "entrypoint not found at $ENTRYPOINT"
[ -f "$BUILD_SITE" ] || env_error "build script not found at $BUILD_SITE"
[ -f "$ENHANCE" ] || env_error "enhancement script not found at $ENHANCE"

ROOT="$(mktemp -d "${TMPDIR:-/tmp}/aurumcode-docmode.XXXXXX")" || env_error 'cannot create a scratch directory'
trap 'rm -rf "$ROOT"' EXIT

failures=0
# Every case must reach exactly one verdict. Counting them is what makes a run
# that executed nothing distinguishable from a run in which everything held:
# the first revision of this harness called env_error from inside a command
# substitution, so `exit 3` killed only that subshell, every case returned
# without asserting anything, and the suite printed "all cases held" and exited
# 0 having measured precisely nothing - while being the gate the Dockerfile
# relied on to ship the image.
verdicts=0
pass() { printf 'PASS  %-6s %s\n' "$1" "$2"; verdicts=$((verdicts + 1)); }
fail() { printf 'FAIL  %-6s %s\n' "$1" "$2"; failures=$((failures + 1)); verdicts=$((verdicts + 1)); }

# make_workspace <name>
# A real git workspace with two commits whose HEAD commit touches no .go file.
# This is exactly the situation the reported false green came from.
#
# The workspace path is returned in the global WS, NOT on stdout. A
# `ws="$(make_workspace x)"` call would run this whole function in a subshell,
# where env_error's `exit 3` cannot stop the suite.
#
# make_workspace <name> [go-change]
# With a second argument, the HEAD commit also modifies a .go file, which is
# what a normal commit to a Go repository looks like.
WS=''
make_workspace() {
  local ws="$ROOT/$1" go_change="${2:-}"
  mkdir -p "$ws/internal/pkg" "$ws/docs"
  cat >"$ws/README.md" <<'MD'
# Fixture repository
MD
  cat >"$ws/CHANGELOG.md" <<'MD'
# Changelog
MD
  cat >"$ws/internal/pkg/thing.go" <<'GO'
package pkg

// Thing does a thing.
func Thing() {}
GO
  (
    cd "$ws" || exit 3
    git init -q .
    git config user.email tester@example.invalid
    git config user.name 'Fixture Tester'
    git add -A
    git commit -q -m 'initial commit with go sources'
    printf 'a docs-only change\n' >>"$ws/README.md"
    if [ -n "$go_change" ]; then
      printf '\n// Other does another thing.\nfunc Other() {}\n' >>"$ws/internal/pkg/thing.go"
    fi
    git add -A
    git commit -q -m 'second commit'
  ) >/dev/null 2>&1 || env_error "could not build the git workspace $1"
  WS="$ws"
}

# make_generator <dir> <behavior>
# Installs a stand-in for /app/regenerate-docs. The real binary needs the Go
# toolchain and external doc tools; what the entrypoint must honour is its
# contract, which is the `aurumcode: result=... docs=N ...` verdict line it
# prints and the markdown it leaves under AURUMCODE_OUTPUT_DIR.
make_generator() {
  local dir="$1" behavior="$2"
  mkdir -p "$dir"
  case "$behavior" in
    empty)
      cat >"$dir/regenerate-docs" <<'SH'
#!/usr/bin/env bash
out="${AURUMCODE_OUTPUT_DIR:-.aurumcode}"
mkdir -p "$out"
echo "aurumcode: result=empty docs=0 skipped=0 failed=0 languages_skipped=none output=$out index=false config=false"
exit 0
SH
      ;;
    silent)
      cat >"$dir/regenerate-docs" <<'SH'
#!/usr/bin/env bash
exit 0
SH
      ;;
    scaffold)
      # Writes only the site scaffolding the pipeline emits on every run: a
      # landing page and a config. Neither documents anything from the source,
      # but both are files under the output directory, so a check that merely
      # counts files there is satisfied by them.
      cat >"$dir/regenerate-docs" <<'SH'
#!/usr/bin/env bash
out="${AURUMCODE_OUTPUT_DIR:-.aurumcode}"
mkdir -p "$out"
printf '# index\n' >"$out/index.md"
printf 'title: docs\n' >"$out/_config.yml"
echo "aurumcode: result=empty docs=0 skipped=0 failed=0 languages_skipped=none output=$out index=true config=true"
exit 0
SH
      ;;
    liar)
      cat >"$dir/regenerate-docs" <<'SH'
#!/usr/bin/env bash
out="${AURUMCODE_OUTPUT_DIR:-.aurumcode}"
mkdir -p "$out"
echo "aurumcode: result=ok docs=7 skipped=0 failed=0 languages_skipped=none output=$out index=true config=true"
exit 0
SH
      ;;
    ok)
      cat >"$dir/regenerate-docs" <<'SH'
#!/usr/bin/env bash
out="${AURUMCODE_OUTPUT_DIR:-.aurumcode}"
mkdir -p "$out/go/internal/pkg"
printf '# pkg.Thing\n\nExtracted from internal/pkg/thing.go\n' >"$out/go/internal/pkg/thing.go.md"
printf '# index\n' >"$out/index.md"
printf 'title: docs\n' >"$out/_config.yml"
echo "aurumcode: result=ok docs=1 skipped=0 failed=0 languages_skipped=none output=$out index=true config=true"
exit 0
SH
      ;;
    hostile-name)
      # Documents a source file whose NAME carries shell metacharacters. The
      # name reaches the manifest and then the entrypoint's verification of it.
      # Any verifier that splices that name into a command line - an awk
      # system(), an eval, a `sh -c` - runs the payload with the action's
      # privileges and the workflow's secrets in the environment.
      cat >"$dir/regenerate-docs" <<'SH'
#!/usr/bin/env bash
out="${AURUMCODE_OUTPUT_DIR:-.aurumcode}"
mkdir -p "$out/go"
printf '# hostile\n' >"$out/go/x\";touch PWNED;\".md"
printf '# pkg.Thing\n\nExtracted from internal/pkg/thing.go\n' >"$out/go/thing.go.md"
printf '# index\n' >"$out/index.md"
echo "aurumcode: result=ok docs=2 skipped=0 failed=0 languages_skipped=none output=$out index=true config=false"
exit 0
SH
      ;;
    *) env_error "unknown generator behavior $behavior" ;;
  esac
  chmod +x "$dir/regenerate-docs"
}

# run_entrypoint <workspace> <bindir> -> writes combined output to $LAST_OUT,
# sets $LAST_RC. The decisive command is run raw so no pipe swallows its status.
LAST_OUT=''
LAST_RC=0
run_entrypoint() {
  local ws="$1" bindir="$2"
  shift 2
  LAST_OUT="$ROOT/out.$$.$RANDOM"
  env -i \
    PATH="$PATH" HOME="$HOME" LC_ALL=C \
    TMPDIR="${TMPDIR:-/tmp}" \
    AURUMCODE_MODE=documentation \
    GITHUB_WORKSPACE="$ws" \
    AURUMCODE_BIN_DIR="$bindir" \
    "$@" \
    bash "$ENTRYPOINT" >"$LAST_OUT" 2>&1
  LAST_RC=$?
}

show() { sed -e 's/^/      | /' "$1"; }

# --------------------------------------------------------------------------
# AC-0  The literal reported repro: documentation mode over a real git
#       workspace with no .go change, on an image that has no generator, must
#       not announce a complete pipeline with status 0.
# --------------------------------------------------------------------------
case_ac0() {
  local ws bindir
  make_workspace ac0; ws="$WS"
  bindir="$ROOT/bin-empty-ac0"
  mkdir -p "$bindir"
  run_entrypoint "$ws" "$bindir"

  if [ "$LAST_RC" -eq 0 ]; then
    fail AC-0 "documentation mode exited 0 without generating anything (reported false green)"
    show "$LAST_OUT"
    return
  fi
  if grep -q 'AurumCode Pipeline Complete' "$LAST_OUT"; then
    fail AC-0 "run claimed 'AurumCode Pipeline Complete' while failing"
    show "$LAST_OUT"
    return
  fi
  pass AC-0 "no-op documentation run is not reported as a complete pipeline (rc=$LAST_RC)"
}

# --------------------------------------------------------------------------
# AC-1  A missing generator binary is an ENVIRONMENT failure (exit 3), named.
# --------------------------------------------------------------------------
case_ac1() {
  local ws bindir
  make_workspace ac1; ws="$WS"
  bindir="$ROOT/bin-empty-ac1"
  mkdir -p "$bindir"
  run_entrypoint "$ws" "$bindir"

  if [ "$LAST_RC" -ne "$EXPECT_ENVIRONMENT" ]; then
    fail AC-1 "expected environment exit $EXPECT_ENVIRONMENT for an absent generator, got $LAST_RC"
    show "$LAST_OUT"
    return
  fi
  if ! grep -q 'regenerate-docs' "$LAST_OUT"; then
    fail AC-1 "environment failure did not name the missing 'regenerate-docs' executable"
    show "$LAST_OUT"
    return
  fi
  pass AC-1 "absent generator fails closed as environment error naming regenerate-docs"
}

# --------------------------------------------------------------------------
# AC-2  The generator ran and reported zero documentation derived from source.
#       That is a BEHAVIORAL failure (exit 1), not a success.
# --------------------------------------------------------------------------
case_ac2() {
  local ws bindir
  make_workspace ac2; ws="$WS"
  bindir="$ROOT/bin-empty-generator"
  make_generator "$bindir" empty
  run_entrypoint "$ws" "$bindir"

  if [ "$LAST_RC" -ne "$EXPECT_BEHAVIOR" ]; then
    fail AC-2 "expected behavioral exit $EXPECT_BEHAVIOR when the generator produced docs=0, got $LAST_RC"
    show "$LAST_OUT"
    return
  fi
  if ! grep -q 'docs=0' "$LAST_OUT"; then
    fail AC-2 "failure message did not quote the generator verdict (docs=0)"
    show "$LAST_OUT"
    return
  fi
  # The DIAGNOSIS has to be right, not just the exit status. "The generator
  # documented nothing" and "the documentation on disk pre-dates this run" are
  # different faults with different remedies, and an operator who is handed the
  # wrong one goes looking in the wrong place. This also keeps the empty-output
  # gate falsifiable: without it, the freshness gate downstream would swallow
  # this case and report it as a stale directory.
  if ! grep -q 'left no documentation derived from the source tree' "$LAST_OUT"; then
    fail AC-2 "an empty generation was not diagnosed as one"
    show "$LAST_OUT"
    return
  fi
  pass AC-2 "generator verdict docs=0 is a behavioral failure diagnosed as an empty generation, quoting the verdict"
}

# --------------------------------------------------------------------------
# AC-3  A generator that exits 0 silently and writes nothing leaves the run
#       with no artifact to point at. Nothing on disk is never success.
# --------------------------------------------------------------------------
case_ac3() {
  local ws bindir
  make_workspace ac3; ws="$WS"
  bindir="$ROOT/bin-silent-generator"
  make_generator "$bindir" silent
  run_entrypoint "$ws" "$bindir"

  if [ "$LAST_RC" -ne "$EXPECT_BEHAVIOR" ]; then
    fail AC-3 "expected behavioral exit $EXPECT_BEHAVIOR when the generator wrote nothing, got $LAST_RC"
    show "$LAST_OUT"
    return
  fi
  pass AC-3 "a silent generator that produced no artifact fails closed"
}

# --------------------------------------------------------------------------
# AC-4  Legitimate success: the generator produced markdown derived from the
#       source tree, the site carries that derived artifact, and the run does
#       not claim a publication it did not perform.
# --------------------------------------------------------------------------
case_ac4() {
  local ws bindir
  make_workspace ac4; ws="$WS"
  bindir="$ROOT/bin-ok-generator"
  make_generator "$bindir" ok
  run_entrypoint "$ws" "$bindir"

  if [ "$LAST_RC" -ne 0 ]; then
    fail AC-4 "a run that really generated documentation was rejected (rc=$LAST_RC)"
    show "$LAST_OUT"
    return
  fi
  if [ ! -f "$ws/docs/public/.aurumcode-derived.manifest" ]; then
    fail AC-4 "successful run left no derived-artifact manifest under docs/public"
    show "$LAST_OUT"
    return
  fi
  if ! grep -q '^derived_files=[1-9]' "$ws/docs/public/.aurumcode-derived.manifest"; then
    fail AC-4 "derived-artifact manifest does not record any derived file"
    show "$ws/docs/public/.aurumcode-derived.manifest"
    return
  fi
  if ! grep -q 'thing.go' "$ws/docs/public/.aurumcode-derived.manifest"; then
    fail AC-4 "the site does not carry the artifact the generator derived from the source"
    show "$ws/docs/public/.aurumcode-derived.manifest"
    return
  fi
  pass AC-4 "a real generator run succeeds and the site carries its source-derived artifact"
}

# --------------------------------------------------------------------------
# AC-5  build-docs-site.sh alone: with no generated documentation to absorb,
#       the hardcoded homepage template is not a built site.
# --------------------------------------------------------------------------
case_ac5() {
  local ws rc out
  make_workspace ac5; ws="$WS"
  out="$ROOT/out-ac5"
  ( cd "$ws" && bash "$BUILD_SITE" ) >"$out" 2>&1
  rc=$?

  if [ "$rc" -ne "$EXPECT_BEHAVIOR" ]; then
    fail AC-5 "expected behavioral exit $EXPECT_BEHAVIOR when only the static template could be written, got $rc"
    show "$out"
    return
  fi
  if [ -f "$ws/docs/public/index.html" ]; then
    fail AC-5 "a failed site build still left a homepage that looks like a built site"
    return
  fi
  pass AC-5 "a site made only of the static template is refused, and no misleading homepage is left"
}

# --------------------------------------------------------------------------
# AC-6  generate-enhanced-docs.sh must report "no Go files changed" as a
#       distinguishable no-op, not as ordinary success.
# --------------------------------------------------------------------------
case_ac6() {
  local ws rc out
  make_workspace ac6; ws="$WS"
  out="$ROOT/out-ac6"
  ( cd "$ws" && bash "$ENHANCE" incremental ) >"$out" 2>&1
  rc=$?

  if [ "$rc" -ne "$EXPECT_NOOP" ]; then
    fail AC-6 "expected no-op exit $EXPECT_NOOP when no Go file changed, got $rc"
    show "$out"
    return
  fi
  if ! grep -q 'aurumcode-enhanced: result=noop' "$out"; then
    fail AC-6 "no-op run did not emit the machine-readable noop verdict"
    show "$out"
    return
  fi
  pass AC-6 "'no Go files changed' is reported as an explicit no-op, not as work done"
}

# --------------------------------------------------------------------------
# AC-7  A generator that PRINTS a convincing success while writing nothing is
#       still a failed run. A verdict a program declares about itself is a
#       claim; only the artifact on disk is evidence.
# --------------------------------------------------------------------------
case_ac7() {
  local ws bindir
  make_workspace ac7; ws="$WS"
  bindir="$ROOT/bin-lying-generator"
  make_generator "$bindir" liar
  run_entrypoint "$ws" "$bindir"

  if [ "$LAST_RC" -ne "$EXPECT_BEHAVIOR" ]; then
    fail AC-7 "a generator claiming 'result=ok docs=7' while writing nothing was believed (rc=$LAST_RC)"
    show "$LAST_OUT"
    return
  fi
  pass AC-7 "a self-declared success with no artifact on disk is rejected"
}

# --------------------------------------------------------------------------
# AC-8  A second incremental run over an unchanged workspace is a legitimate
#       no-op: it may exit 0, but it must be reported as a no-op and must not
#       be dressed up as a run that generated documentation.
# --------------------------------------------------------------------------
case_ac8() {
  local ws bindir
  make_workspace ac8; ws="$WS"
  bindir="$ROOT/bin-ok-generator-ac8"
  make_generator "$bindir" ok

  run_entrypoint "$ws" "$bindir" DOCUMENTATION_MODE=incremental
  if [ "$LAST_RC" -ne 0 ]; then
    fail AC-8 "the first (generating) run did not succeed (rc=$LAST_RC)"
    show "$LAST_OUT"
    return
  fi
  if ! grep -q '^  Documentation: generated$' "$LAST_OUT"; then
    fail AC-8 "the first run was not reported as having generated documentation"
    show "$LAST_OUT"
    return
  fi

  run_entrypoint "$ws" "$bindir" DOCUMENTATION_MODE=incremental
  if [ "$LAST_RC" -ne 0 ]; then
    fail AC-8 "a legitimate incremental no-op was turned into a failure (rc=$LAST_RC)"
    show "$LAST_OUT"
    return
  fi
  if ! grep -q '^  Documentation: noop$' "$LAST_OUT"; then
    fail AC-8 "the no-op run was not reported as a no-op"
    show "$LAST_OUT"
    return
  fi
  pass AC-8 "an unchanged incremental rerun is reported as a no-op, not as generation"
}

# --------------------------------------------------------------------------
# AC-9  Isolates the entrypoint's own gate on the generator output. The
#       generator writes the site scaffolding (index.md, _config.yml) and
#       nothing else, so every downstream check that only counts files under
#       the output directory is satisfied. Documenting nothing is still a
#       failed documentation run, and the entrypoint has to say so itself.
# --------------------------------------------------------------------------
case_ac9() {
  local ws bindir
  make_workspace ac9; ws="$WS"
  bindir="$ROOT/bin-scaffold-generator"
  make_generator "$bindir" scaffold
  run_entrypoint "$ws" "$bindir"

  if [ "$LAST_RC" -ne "$EXPECT_BEHAVIOR" ]; then
    fail AC-9 "a generator that wrote only site scaffolding and documented nothing was accepted (rc=$LAST_RC)"
    show "$LAST_OUT"
    return
  fi
  pass AC-9 "scaffolding under the output directory does not count as documentation"
}

# --------------------------------------------------------------------------
# AC-10 Isolates the entrypoint's site verification. The generator does its
#       job, but the site build is a version that writes only the hardcoded
#       homepage template. The entrypoint must not accept that as a built
#       documentation site. No production seam is used: the scripts are copied
#       to a scratch directory and the build script is replaced there, exactly
#       as a drifted image would ship them.
# --------------------------------------------------------------------------
case_ac10() {
  local ws bindir fake
  make_workspace ac10; ws="$WS"
  bindir="$ROOT/bin-ok-generator-ac10"
  make_generator "$bindir" ok

  fake="$ROOT/scripts-template-only"
  mkdir -p "$fake"
  cp "$ENTRYPOINT" "$ENHANCE" "$fake/"
  cat >"$fake/build-docs-site.sh" <<'SH'
#!/usr/bin/env bash
# A build that only writes the hardcoded homepage, as the pre-fix one did.
set -euo pipefail
mkdir -p docs/public
printf '<!DOCTYPE html><html><body><h1>AurumCode Documentation</h1></body></html>\n' >docs/public/index.html
echo "✅ Documentation site built successfully"
exit 0
SH

  LAST_OUT="$ROOT/out.ac10"
  env -i \
    PATH="$PATH" HOME="$HOME" LC_ALL=C TMPDIR="${TMPDIR:-/tmp}" \
    AURUMCODE_MODE=documentation \
    GITHUB_WORKSPACE="$ws" \
    AURUMCODE_BIN_DIR="$bindir" \
    bash "$fake/action-entrypoint.sh" >"$LAST_OUT" 2>&1
  LAST_RC=$?

  if [ "$LAST_RC" -ne "$EXPECT_BEHAVIOR" ]; then
    fail AC-10 "a site consisting only of the hardcoded homepage template was accepted (rc=$LAST_RC)"
    show "$LAST_OUT"
    return
  fi
  pass AC-10 "a template-only site is rejected even when the site build exits 0"
}

# --------------------------------------------------------------------------
# AC-11 A documentation directory that was ALREADY on disk does not buy a green
#       run. The generator writes nothing this time; every markdown file under
#       the output directory pre-dates the run, exactly as it would if the
#       consumer had committed `.aurumcode` to their repository or a previous
#       run had left it behind. Counting files there answers "does a directory
#       exist", which is not the question.
# --------------------------------------------------------------------------
case_ac11() {
  local ws bindir
  make_workspace ac11; ws="$WS"
  bindir="$ROOT/bin-empty-generator-ac11"
  make_generator "$bindir" empty

  # Pre-existing documentation, byte-for-byte plausible, written by nobody.
  mkdir -p "$ws/.aurumcode/go"
  printf '# pkg.Thing\n\nHand written, derived from no source file at all.\n' \
    >"$ws/.aurumcode/go/thing.go.md"

  # Deliberately the INCREMENTAL mode. A full regeneration is separately
  # required to rewrite everything it ships, so running this under full mode
  # would let that rule reject the workspace and hide whether the run can tell
  # documentation it produced from documentation it merely found.
  run_entrypoint "$ws" "$bindir" DOCUMENTATION_MODE=incremental

  if [ "$LAST_RC" -ne "$EXPECT_BEHAVIOR" ]; then
    fail AC-11 "a pre-existing .aurumcode directory satisfied the gate for a generator that wrote nothing (rc=$LAST_RC)"
    show "$LAST_OUT"
    return
  fi
  if ! grep -q 'pre-dates this run' "$LAST_OUT"; then
    fail AC-11 "the failure did not say that the documentation pre-dates the run"
    show "$LAST_OUT"
    return
  fi
  pass AC-11 "documentation that pre-dates the run is not evidence that the run generated anything"
}

# --------------------------------------------------------------------------
# AC-12 A file NAME from the repository being documented is data, never code.
#       The generator documents a source file whose name carries shell
#       metacharacters; that name travels into the site manifest and then into
#       the entrypoint's verification of it. A verifier that splices the name
#       into a command line executes the payload with the action's privileges
#       and the workflow's secrets in its environment.
# --------------------------------------------------------------------------
case_ac12() {
  local ws bindir
  make_workspace ac12; ws="$WS"
  bindir="$ROOT/bin-hostile-generator"
  make_generator "$bindir" hostile-name

  run_entrypoint "$ws" "$bindir"

  # The marker is written into the workspace, which is the entrypoint's cwd.
  if [ -e "$ws/PWNED" ]; then
    fail AC-12 "a file name from the documented repository was executed as a command (marker '$ws/PWNED' exists)"
    show "$LAST_OUT"
    return
  fi
  if [ "$LAST_RC" -ne 0 ]; then
    fail AC-12 "a repository holding an awkwardly named file could not be documented at all (rc=$LAST_RC)"
    show "$LAST_OUT"
    return
  fi
  pass AC-12 "a file name carrying shell metacharacters is counted as data and never executed"
}

# --------------------------------------------------------------------------
# AC-13 DOCUMENTATION_MODE=full has to work end to end. It used to abort with
#       an environment error on a missing 'docs/static/godoc' tree, which
#       nothing in this repository has ever produced, so the mode was
#       unreachable no matter what the generator did.
# --------------------------------------------------------------------------
case_ac13() {
  local ws bindir
  make_workspace ac13; ws="$WS"
  bindir="$ROOT/bin-ok-generator-ac13"
  make_generator "$bindir" ok

  run_entrypoint "$ws" "$bindir" DOCUMENTATION_MODE=full

  if [ "$LAST_RC" -ne 0 ]; then
    fail AC-13 "full mode failed over a workspace the generator did document (rc=$LAST_RC)"
    show "$LAST_OUT"
    return
  fi
  if ! grep -q '^  Documentation: generated$' "$LAST_OUT"; then
    fail AC-13 "full mode did not report that it generated documentation"
    show "$LAST_OUT"
    return
  fi
  pass AC-13 "DOCUMENTATION_MODE=full completes against the generator's own output"
}

# --------------------------------------------------------------------------
# AC-14 The ordinary case for a Go repository: the HEAD commit changes a .go
#       file. The enhancement step used to treat "no godoc HTML matched the
#       changed packages" as a behavioral failure, and since nothing produces
#       docs/static/godoc, every commit that touched Go code failed the whole
#       pipeline. The run must complete on the strength of what the generator
#       actually wrote.
# --------------------------------------------------------------------------
case_ac14() {
  local ws bindir
  make_workspace ac14 with-go-change; ws="$WS"
  bindir="$ROOT/bin-ok-generator-ac14"
  make_generator "$bindir" ok

  # The incremental mode is named explicitly: it is the only one that diffs the
  # commit and looks for godoc HTML per changed package, so it is the only one
  # in which "a Go file changed" could fail the run.
  run_entrypoint "$ws" "$bindir" DOCUMENTATION_MODE=incremental

  if [ "$LAST_RC" -ne 0 ]; then
    fail AC-14 "a commit that changed a Go file failed the documentation run (rc=$LAST_RC)"
    show "$LAST_OUT"
    return
  fi
  if ! grep -q '^  Documentation: generated$' "$LAST_OUT"; then
    fail AC-14 "the run did not report that it generated documentation"
    show "$LAST_OUT"
    return
  fi
  pass AC-14 "a commit touching Go sources completes the documentation run"
}

# --------------------------------------------------------------------------
# AC-15 Isolates build-docs-site.sh's own notion of a derived artifact. The
#       generator output directory holds ONLY what the generator writes on
#       every run whether or not it documented anything: the landing page, the
#       site config, and its incremental cache. Absorbing that tree and
#       counting the files in it makes scaffolding and a cache file look like
#       documentation, which is enough to satisfy the caller's "is anything
#       derived here" check all by itself. Run directly, so the entrypoint's
#       earlier gate cannot be what rejects this.
# --------------------------------------------------------------------------
case_ac15() {
  local ws rc out
  make_workspace ac15; ws="$WS"
  out="$ROOT/out-ac15"

  mkdir -p "$ws/.aurumcode/cache"
  printf '# index\n' >"$ws/.aurumcode/index.md"
  printf 'title: docs\n' >"$ws/.aurumcode/_config.yml"
  printf '{"commit":"deadbeef"}\n' >"$ws/.aurumcode/cache/incremental.json"

  ( cd "$ws" && bash "$BUILD_SITE" ) >"$out" 2>&1
  rc=$?

  if [ "$rc" -ne "$EXPECT_BEHAVIOR" ]; then
    fail AC-15 "scaffolding and an incremental cache were accepted as derived documentation (rc=$rc)"
    show "$out"
    return
  fi
  if [ -d "$ws/docs/public" ]; then
    fail AC-15 "a refused build still materialised a site directory"
    return
  fi
  pass AC-15 "the landing page, the site config and the generator cache do not count as derived documentation"
}

# --------------------------------------------------------------------------
# AC-16 A full regeneration has to account for every documentation file it
#       leaves behind. A file the generator did not rewrite has no source this
#       run confirmed - it may document code that was deleted - and publishing
#       it inside the site would present it as current. Only the incremental
#       mode may leave such a file alone.
# --------------------------------------------------------------------------
case_ac16() {
  local ws bindir
  make_workspace ac16; ws="$WS"
  bindir="$ROOT/bin-ok-generator-ac16"
  make_generator "$bindir" ok

  # Documentation for a package that no longer exists, left by an older run.
  mkdir -p "$ws/.aurumcode/go/internal/gone"
  printf '# gone.Removed\n\nDocuments a package that was deleted.\n' \
    >"$ws/.aurumcode/go/internal/gone/removed.go.md"

  run_entrypoint "$ws" "$bindir" DOCUMENTATION_MODE=full

  if [ "$LAST_RC" -ne "$EXPECT_BEHAVIOR" ]; then
    fail AC-16 "a full regeneration shipped documentation it never rewrote (rc=$LAST_RC)"
    show "$LAST_OUT"
    return
  fi
  pass AC-16 "a full regeneration refuses to leave documentation it did not write"
}

# --------------------------------------------------------------------------
# AC-17 The DEFAULT mode must be able to produce documentation. The generator's
#       incremental mode asks its cache which commit it last documented and,
#       with no cache, falls back to the workspace's UNSTAGED changes - of
#       which a freshly checked out CI workspace has none. Defaulting to it
#       therefore asks the generator to document nothing, every time. This
#       asserts the default is not that mode, by running the default over a
#       clean two-commit checkout with no unstaged change at all.
# --------------------------------------------------------------------------
case_ac17() {
  local ws bindir
  make_workspace ac17; ws="$WS"
  bindir="$ROOT/bin-mode-reporting-generator"

  # A generator that documents only what the caller's mode actually asks for,
  # the way the real one does: it produces nothing under incremental mode
  # unless something is unstaged.
  mkdir -p "$bindir"
  cat >"$bindir/regenerate-docs" <<'SH'
#!/usr/bin/env bash
out="${AURUMCODE_OUTPUT_DIR:-.aurumcode}"
mkdir -p "$out"
if [ "${AURUMCODE_INCREMENTAL:-false}" = "true" ]; then
  echo "aurumcode: result=empty docs=0 skipped=0 failed=0 languages_skipped=none output=$out index=true config=true"
  exit 0
fi
mkdir -p "$out/go/internal/pkg"
printf '# pkg.Thing\n\nExtracted from internal/pkg/thing.go\n' >"$out/go/internal/pkg/thing.go.md"
printf '# index\n' >"$out/index.md"
echo "aurumcode: result=ok docs=1 skipped=0 failed=0 languages_skipped=none output=$out index=true config=true"
exit 0
SH
  chmod +x "$bindir/regenerate-docs"

  run_entrypoint "$ws" "$bindir"

  if [ "$LAST_RC" -ne 0 ]; then
    fail AC-17 "the default documentation mode could not document a clean checkout (rc=$LAST_RC)"
    show "$LAST_OUT"
    return
  fi
  if ! grep -q '^  Documentation: generated$' "$LAST_OUT"; then
    fail AC-17 "the default mode did not report generated documentation"
    show "$LAST_OUT"
    return
  fi
  pass AC-17 "the default mode asks for a regeneration a fresh checkout can actually satisfy"
}

# --------------------------------------------------------------------------
# Runner.
#
# A verifier that reports success without executing anything is worse than no
# verifier, so the selection is materialised first, every selected case must
# reach exactly one verdict, and the counts are re-derived and compared at the
# end rather than assumed. "Nothing ran" and "everything held" get different
# exit statuses: 3 says nothing was measured, 0 says N cases were.
# --------------------------------------------------------------------------
selected=()
for c in AC-0 AC-1 AC-2 AC-3 AC-4 AC-5 AC-6 AC-7 AC-8 AC-9 AC-10 AC-11 AC-12 AC-13 AC-14 AC-15 AC-16 AC-17; do
  if [ "$selector" = ALL ] || [ "$selector" = "$c" ]; then
    selected+=("$c")
  fi
done

if [ "${#selected[@]}" -eq 0 ]; then
  printf 'HARNESS: selector %s selected zero cases; this run measured nothing and proves nothing\n' "$selector" >&2
  exit 3
fi

for c in "${selected[@]}"; do
  case "$c" in
    AC-0) case_ac0 ;;
    AC-1) case_ac1 ;;
    AC-2) case_ac2 ;;
    AC-3) case_ac3 ;;
    AC-4) case_ac4 ;;
    AC-5) case_ac5 ;;
    AC-6) case_ac6 ;;
    AC-7) case_ac7 ;;
    AC-8) case_ac8 ;;
    AC-9) case_ac9 ;;
    AC-10) case_ac10 ;;
    AC-11) case_ac11 ;;
    AC-12) case_ac12 ;;
    AC-13) case_ac13 ;;
    AC-14) case_ac14 ;;
    AC-15) case_ac15 ;;
    AC-16) case_ac16 ;;
    AC-17) case_ac17 ;;
    *) printf 'HARNESS: no implementation for selected case %s\n' "$c" >&2; exit 3 ;;
  esac
done

if [ "$verdicts" -ne "${#selected[@]}" ]; then
  printf '\nHARNESS: %d case(s) selected but only %d reached a verdict; the rest returned without asserting anything, so this run proves nothing\n' \
    "${#selected[@]}" "$verdicts" >&2
  exit 3
fi

if [ "$failures" -ne 0 ]; then
  printf '\n%d of %d case(s) failed\n' "$failures" "$verdicts"
  exit 1
fi
printf '\nall %d case(s) held\n' "$verdicts"
exit 0
