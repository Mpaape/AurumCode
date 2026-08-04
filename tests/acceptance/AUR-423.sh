#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

# AUR-423/AC-001 -- browser proof over the four site variants.
#
# WHAT THIS GATE IS FOR
#
# A card is only delivered when an agent drives a pinned headless browser over the
# published artifact and sees the promised effect on screen. `internal/qa/browserproof`
# publishes that verdict contract (BrowserProofResultV1) but nothing pointed it at the
# site the pipeline actually produces, so the card had no acceptance program at all:
# it could neither pass nor fail, and the terminal gate depends on it.
#
# THREAT MODEL THIS GATE IS WRITTEN AGAINST
#
# Seven sibling gates fell to the same two moves, so neither is available here:
#
#   1. "A file asserts the verdict." A manifest that says `"proved": true`, a TSV row
#      that says `pass`, a field in which a file states its own digest. This gate reads
#      no verdict from any file. Every verdict comes from RE-EXECUTING the card's prover
#      over a site tree and is then corroborated against values this gate RECOMPUTES from
#      the served bytes. The manifest may name the prover and the driver lock; it may not
#      state what the outcome is, what the route is, what the selector is or what text is
#      expected -- those are literals embedded in THIS file.
#
#   2. "A stub takes the place of the program." A prover replaced by `exit 0`, or by a
#      constant that always prints the nominal document. Exit status is therefore never
#      read as a verdict, only checked for agreement with a document that must exist and
#      must substantiate itself. The prover is defeated structurally instead:
#        * four fixture variants must return four DIFFERENT and pairwise distinct
#          (outcome, code) pairs, each matching an embedded literal;
#        * a private shadow tree with an mktemp-random name is proved, then MUTATED
#          (the symbol page is removed) and must flip to BROWSERPROOF_TARGET_ABSENT,
#          then restored and must reproduce the pristine verdict AND the pristine
#          artifact digest. A prover keyed on directory names, or one that answers
#          from a table, cannot satisfy the pristine leg and the mutated leg at once;
#        * `artifact_digest` in every document must equal the digest this gate computes
#          independently over the served tree, with the package's own algorithm;
#        * `driver_digest` in every document must equal the sha256 this gate computes
#          over the driver file it is about to hand the prover;
#        * a `proved` document with an empty selector or empty observed text is refused
#          (MUT-003), and the observed text must literally occur in the served file.
#
# The digests of the prover and of the driver are recomputed here, from content, before
# either is executed, and are reported in the pass record. The repository cannot attest
# to itself, so an operator may additionally pin them from outside through
# AUR423_PROVER_DIGEST / AUR423_DRIVER_DIGEST; a pin can only ever make this gate
# stricter, never green.
#
# MATERIALIZATION NOTE (why this gate reads nothing outside the card)
#   AUR-423 declares `paths` and no `read_paths`, so a container that materializes the
#   card's allowlist gives this program exactly:
#       internal/qa/browserproof/**   tests/fixtures/AUR-423/**   tests/acceptance/AUR-423.sh
#       docs/specs/AUR-423.md         tests/{unit,contracts,integration}/AUR-423.go
#       tests/e2e/AUR-423.sh
#   Nothing else is read -- not go.mod, not .board, not the toolchain layout. A gate that
#   reached outside that set would be green on a developer host and red inside the profile
#   for a reason indistinguishable from a real defect. Scratch space is mktemp only.
#
# EXIT CODES (a caller must be able to tell "not proven" from "wrong")
#   0   proved: every variant returned its expected verdict, the shadow mutation was
#       detected, and every field this gate accepted was recomputed from bytes
#   1   behavioural divergence, or a card artifact that has not been built yet
#   3   infrastructure: the harness itself is unusable (missing tool, unresolvable root,
#       artifact tree this gate cannot digest)
#   64  unknown selector
#   79  inconclusive: no verdict could be obtained (prover timed out, driver absent or
#       outside the lock, local server would not start, navigation did not stabilise,
#       output past the bound). Never promoted to a behavioural red -- the board forbids
#       reading an environment diagnosis as a statement about the feature.

readonly card='AUR-423'
readonly scenario='AC-001'

# --- embedded contract: not read from any file -------------------------------------
readonly entry_route='/index.html'
readonly target_route='/symbols/extractorpipeline.html'
readonly target_selector='#symbol-name'
readonly expected_text='ExtractorPipeline'
readonly deadline_ms=30000          # AC-001 refuses a navigation deadline above 30 s
readonly max_routes=8               # AC-001 refuses more than 8 routes
readonly max_page_bytes=$((4 * 1024 * 1024))
readonly schema='BrowserProofResultV1'
readonly manifest_rel='internal/qa/browserproof/aur423.harness.json'
readonly scope_prefix='internal/qa/browserproof/'

# variant directory -> expected outcome / expected code. Four verdicts, pairwise distinct.
readonly variants='nominal:proved:
missing-symbol-page:refused:BROWSERPROOF_TARGET_ABSENT
unlinked-symbol-page:refused:BROWSERPROOF_UNREACHABLE_ROUTE
empty-render:refused:BROWSERPROOF_EMPTY_RENDER'

readonly max_stdout_bytes=$((256 * 1024))
readonly max_stderr_bytes=$((64 * 1024))
readonly max_manifest_bytes=$((64 * 1024))
readonly max_artifact_files=64
readonly max_program_bytes=$((8 * 1024 * 1024))
readonly prover_timeout=45

selector="${1:-AC-001}"
case "$selector" in
  AC-001|AUR423|E2EAUR423|IntegrationAUR423) ;;
  *) printf '%s/%s/unknown-selector: %s\n' "$card" "$scenario" "$selector" >&2; exit 64 ;;
esac

work=''
cleanup() { [ -n "$work" ] && rm -rf -- "$work" 2>/dev/null || true; }
trap cleanup EXIT INT TERM

infra() {
  printf '%s/%s/infrastructure: %s\n' "$card" "$scenario" "$1" >&2
  exit 3
}
fail() {
  printf '%s/%s/%s: %s\n' "$card" "$scenario" "$1" "$2" >&2
  exit 1
}
inconclusive() {
  printf '%s/%s/inconclusive: %s: %s\n' "$card" "$scenario" "$1" "$2" >&2
  exit 79
}

for tool in awk sed grep find sort uniq tr sha256sum wc timeout mktemp cp rm cut od; do
  command -v "$tool" >/dev/null 2>&1 || infra "required tool is missing: $tool"
done

# --- root resolution ---------------------------------------------------------------
# The default root is derived from this file's own location, so the gate works from any
# working directory. AUR423_ROOT redirects it at another tree; that only changes WHERE
# the artifact is read from. Every check below is a recomputation, so a forged tree
# cannot make the gate green -- it can only fail faster.
if [ -n "${AUR423_ROOT:-}" ]; then
  root=$AUR423_ROOT
else
  self_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" 2>/dev/null && pwd -P) || infra 'cannot resolve this program directory'
  root=$(cd -- "$self_dir/../.." 2>/dev/null && pwd -P) || infra 'cannot resolve the repository root'
fi
root=$(cd -- "$root" 2>/dev/null && pwd -P) || infra "root is not a readable directory: ${root}"

readonly fixture_root="$root/tests/fixtures/AUR-423"
[ -d "$fixture_root" ] || fail 'behavior-missing' "card fixture tree absent: tests/fixtures/AUR-423"

work=$(mktemp -d 2>/dev/null) || infra 'cannot create scratch directory'

# --- helpers -----------------------------------------------------------------------

# sha_file prints the sha256 of a file's CONTENT. No field in any file is ever accepted
# as a substitute for this.
sha_file() {
  local path=$1 out
  out=$(sha256sum -- "$path" 2>/dev/null) || return 1
  printf 'sha256:%s\n' "${out:0:64}"
}

size_of() {
  wc -c <"$1" 2>/dev/null | tr -d ' '
}

# safe_tree refuses a tree this gate cannot digest byte-for-byte the way the package
# does: non-regular entries (symlinks, fifos, devices) and names outside a printable
# portable set, which would make the line-oriented digest ambiguous.
safe_tree() {
  local dir=$1 odd count
  odd=$(find "$dir" ! -type d ! -type f -print 2>/dev/null) || return 1
  [ -z "$odd" ] || return 2
  count=$(find "$dir" -type f -print 2>/dev/null | wc -l) || return 1
  [ "$count" -le "$max_artifact_files" ] || return 3
  if find "$dir" -print 2>/dev/null | grep -q '[^A-Za-z0-9._/-]'; then
    return 4
  fi
  return 0
}

# artifact_digest reimplements internal/qa/browserproof.artifactDigest: one line
# "<relative-slash-path> <sha256hex>" per regular file, lines sorted, joined with "\n"
# with no trailing newline, sha256 of that, prefixed "sha256:". Recomputing it here is
# what makes the document's own artifact_digest field checkable instead of believed.
artifact_digest() {
  local dir=$1 f rel h lines
  lines=''
  while IFS= read -r f; do
    rel=${f#"$dir"/}
    h=$(sha256sum -- "$f" 2>/dev/null) || return 1
    lines+="$rel ${h:0:64}"$'\n'
  done < <(find "$dir" -type f -print 2>/dev/null | sort)
  lines=${lines%$'\n'}
  printf 'sha256:%s\n' "$(printf '%s' "$lines" | sha256sum | cut -c1-64)"
}

# json_scan flattens a JSON document into "path<TAB>value" lines. It is intentionally
# strict: bounded depth, no \u escapes, no trailing bytes. Anything it cannot parse is
# not a verdict.
scanner="$work/scan.awk"
cat >"$scanner" <<'AWK'
function fatal(m) { printf("json: %s\n", m) > "/dev/stderr"; exit 65 }
function skipws(   c) {
  while (pos <= len) {
    c = substr(s, pos, 1)
    if (c == " " || c == "\t" || c == "\n" || c == "\r") { pos++ } else { return }
  }
}
function emit(p, v) {
  gsub(/\\/, "\\\\", v); gsub(/\n/, "\\n", v); gsub(/\t/, "\\t", v); gsub(/\r/, "\\r", v)
  printf("%s\t%s\n", p, v)
}
function pstring(   out, c, e) {
  pos++
  out = ""
  while (pos <= len) {
    c = substr(s, pos, 1)
    if (c == "\\") {
      e = substr(s, pos + 1, 1)
      if (e == "\"" || e == "\\" || e == "/") { out = out e }
      else if (e == "n") { out = out "\n" }
      else if (e == "t") { out = out "\t" }
      else if (e == "r") { out = out "\r" }
      else if (e == "b" || e == "f") { out = out " " }
      else { fatal("unsupported escape") }
      pos += 2
      continue
    }
    if (c == "\"") { pos++; return out }
    out = out c
    pos++
  }
  fatal("unterminated string")
}
function pvalue(path, depth,   c, k, i, v) {
  if (depth > 8) fatal("too deep")
  skipws()
  c = substr(s, pos, 1)
  if (c == "{") {
    pos++
    skipws()
    if (substr(s, pos, 1) == "}") { pos++; return }
    while (1) {
      skipws()
      if (substr(s, pos, 1) != "\"") fatal("object key")
      k = pstring()
      skipws()
      if (substr(s, pos, 1) != ":") fatal("missing colon")
      pos++
      pvalue(path == "" ? k : path "." k, depth + 1)
      skipws()
      c = substr(s, pos, 1)
      if (c == ",") { pos++; continue }
      if (c == "}") { pos++; return }
      fatal("object separator")
    }
  }
  if (c == "[") {
    pos++
    i = 0
    skipws()
    if (substr(s, pos, 1) == "]") { pos++; emit(path ".len", 0); return }
    while (1) {
      pvalue(path "." i, depth + 1)
      i++
      skipws()
      c = substr(s, pos, 1)
      if (c == ",") { pos++; continue }
      if (c == "]") { pos++; emit(path ".len", i); return }
      fatal("array separator")
    }
  }
  if (c == "\"") { emit(path, pstring()); return }
  v = ""
  while (pos <= len) {
    c = substr(s, pos, 1)
    if (c ~ /[-+0-9eE.a-z]/) { v = v c; pos++ } else { break }
  }
  if (v == "") fatal("value expected")
  emit(path, v)
}
BEGIN { pos = 1; s = "" }
{ s = s $0 "\n" }
END {
  len = length(s)
  if (len == 0) fatal("empty document")
  pvalue("", 0)
  skipws()
  if (pos <= len) fatal("trailing bytes")
}
AWK

pairs=''
field() {
  awk -F'\t' -v k="$1" 'BEGIN { rc = 1 } $1 == k { print $2; rc = 0; exit } END { exit rc }' "$pairs"
}
has_field() { field "$1" >/dev/null 2>&1; }

# --- the card artifact under test --------------------------------------------------
manifest="$root/$manifest_rel"
if [ ! -f "$manifest" ]; then
  fail 'behavior-missing' \
    "browser proof harness absent: $manifest_rel is not built, so BrowserProof.Run is never reached over the four site variants of AC-001"
fi

msize=$(size_of "$manifest") || infra 'cannot size the harness manifest'
[ "$msize" -le "$max_manifest_bytes" ] || fail 'manifest-oversize' "$manifest_rel exceeds $max_manifest_bytes bytes"

pairs="$work/manifest.tsv"
if ! awk -f "$scanner" "$manifest" >"$pairs" 2>"$work/manifest.err"; then
  fail 'manifest-unparsable' "$manifest_rel is not a parsable flat JSON document"
fi
if [ -n "$(cut -f1 "$pairs" | sort | uniq -d)" ]; then
  fail 'manifest-duplicate-key' "$manifest_rel repeats a key, so its meaning is not single-valued"
fi

# A file may not attest to itself. Any key in which the manifest states its own digest is
# unverifiable by construction and is refused before anything else is read from it.
for self_key in manifest_digest self_digest harness_digest digest sealed_digest; do
  if has_field "$self_key"; then
    fail 'self-attesting-manifest' "$manifest_rel declares '$self_key' about itself; a file cannot witness its own identity"
  fi
done

manifest_card=$(field card 2>/dev/null) || fail 'manifest-incomplete' "$manifest_rel has no 'card'"
[ "$manifest_card" = "$card" ] || fail 'manifest-foreign' "$manifest_rel is scoped to '$manifest_card', not $card"

prover_rel=$(field prover 2>/dev/null) || fail 'manifest-incomplete' "$manifest_rel has no 'prover'"
driver_rel=$(field driver 2>/dev/null) || fail 'manifest-incomplete' "$manifest_rel has no 'driver'"
driver_lock=$(field driver_digest 2>/dev/null) || fail 'manifest-incomplete' "$manifest_rel has no 'driver_digest' lock"
driver_kind=$(field driver_kind 2>/dev/null) || fail 'manifest-incomplete' "$manifest_rel has no 'driver_kind' lock"

# The manifest may only name programs INSIDE the card's own path allowlist, and may name
# neither itself nor one file in both roles.
check_program_path() {
  local role=$1 rel=$2
  case "$rel" in
    "$scope_prefix"*) ;;
    *) fail 'out-of-scope-program' "$role '$rel' is outside $scope_prefix, which this card may not read" ;;
  esac
  case "$rel" in
    *..*|*//*|/*) fail 'out-of-scope-program' "$role '$rel' is not a normalised in-tree path" ;;
  esac
  [ "$rel" != "$manifest_rel" ] || fail 'self-attesting-manifest' "$role points at the manifest itself"
  [ -f "$root/$rel" ] || fail 'behavior-missing' "$role '$rel' declared by the manifest does not exist"
  [ ! -L "$root/$rel" ] || fail 'out-of-scope-program' "$role '$rel' is a symlink; its bytes are not the card's"
  [ -x "$root/$rel" ] || fail 'behavior-missing' "$role '$rel' is not executable"
  local sz
  sz=$(size_of "$root/$rel") || infra "cannot size $role"
  [ "$sz" -le "$max_program_bytes" ] || fail 'program-oversize' "$role '$rel' exceeds $max_program_bytes bytes"
}
check_program_path prover "$prover_rel"
check_program_path driver "$driver_rel"
[ "$prover_rel" != "$driver_rel" ] || fail 'harness-collapsed' 'the prover and the driver are the same file'

prover="$root/$prover_rel"
driver="$root/$driver_rel"

# Recompute, from bytes, the identity of what is about to be executed. The manifest's
# lock is checked against this; it never replaces it.
prover_digest=$(sha_file "$prover") || infra 'cannot digest the prover'
driver_digest=$(sha_file "$driver") || infra 'cannot digest the driver'

[ "$driver_kind" != '' ] || fail 'manifest-incomplete' 'driver_kind is empty'
if [ "$driver_lock" != "$driver_digest" ]; then
  # AC-001 / MUT-002: a driver outside the lock is a browser-runtime diagnosis, never a
  # statement about the feature.
  inconclusive 'BROWSERPROOF_DRIVER_MISMATCH' \
    "driver $driver_rel hashes to $driver_digest, outside the lock $driver_lock"
fi

# External pins may only tighten.
if [ -n "${AUR423_PROVER_DIGEST:-}" ] && [ "${AUR423_PROVER_DIGEST}" != "$prover_digest" ]; then
  fail 'pin-mismatch' "prover $prover_rel hashes to $prover_digest, outside the external pin"
fi
if [ -n "${AUR423_DRIVER_DIGEST:-}" ] && [ "${AUR423_DRIVER_DIGEST}" != "$driver_digest" ]; then
  inconclusive 'BROWSERPROOF_DRIVER_MISMATCH' \
    "driver $driver_rel hashes to $driver_digest, outside the external pin"
fi

# --- corroborate the embedded expectation against the fixture bytes ----------------
# The route, the selector and the expected text are literals in this file, but they are
# only meaningful if the nominal fixture really carries them; otherwise the gate would be
# asserting against nothing.
selector_id=${target_selector#\#}
case "$target_selector" in
  '#'*) ;;
  *) infra "embedded selector $target_selector is not an id selector" ;;
esac
nominal_target="$fixture_root/nominal${target_route}"
[ -f "$nominal_target" ] || fail 'behavior-missing' "nominal fixture has no page at $target_route"
grep -qF "id=\"$selector_id\"" -- "$nominal_target" \
  || fail 'fixture-contract-broken' "nominal fixture page $target_route has no element with id=\"$selector_id\""
grep -qF "$expected_text" -- "$nominal_target" \
  || fail 'fixture-contract-broken' "nominal fixture page $target_route does not contain '$expected_text'"

# --- run one variant ---------------------------------------------------------------
# Returns 0 and leaves the flattened document in $pairs when the observed verdict matches
# the expectation; exits otherwise.
observed_summary=''

run_variant() {
  local label=$1 site=$2 want_outcome=$3 want_code=$4
  local rc out err doc_outcome doc_code doc_proved routes n
  local expect_digest canary

  [ -d "$site" ] || fail 'behavior-missing' "variant '$label' has no site directory"
  safe_tree "$site" || case $? in
    2) infra "variant '$label' contains a non-regular entry; its digest is not recomputable here" ;;
    3) infra "variant '$label' exceeds $max_artifact_files files" ;;
    4) infra "variant '$label' contains a path outside the portable name set" ;;
    *) infra "cannot inspect variant '$label'" ;;
  esac
  expect_digest=$(artifact_digest "$site") || infra "cannot digest variant '$label'"

  out="$work/$label.out"
  err="$work/$label.err"
  canary="AURUM-CANARY-$(od -An -N16 -tx1 /dev/urandom 2>/dev/null | tr -d ' \n')"
  : >"$out"
  : >"$err"

  # No pipe: a pipeline would hand back the exit status of the reader, not of the prover.
  set +e
  env -i \
    PATH="${PATH:-/usr/bin:/bin}" HOME="${HOME:-/nonexistent}" LC_ALL=C TMPDIR="$work" \
    AURUM_SECRET_CANARY="$canary" \
    AURUM_BROWSERPROOF_DRIVER="$driver" \
    timeout -k 5 "$prover_timeout" "$prover" \
      --card "$card" \
      --site "$site" \
      --entry "$entry_route" \
      --route "$target_route" \
      --selector "$target_selector" \
      --expect-text "$expected_text" \
      --deadline-ms "$deadline_ms" \
      --max-routes "$max_routes" \
      --max-page-bytes "$max_page_bytes" \
      >"$out" 2>"$err"
  rc=$?
  set -e

  if [ "$rc" -eq 124 ] || [ "$rc" -eq 137 ]; then
    inconclusive 'BROWSERPROOF_NAVIGATION_TIMEOUT' "variant '$label' did not return within ${prover_timeout}s"
  fi
  # 126/127 are the loader answering, not the card: the prover could not be executed at
  # all (missing interpreter, missing shared object, no exec permission). The board
  # forbids reading that as a statement about the feature, so it can never be a red.
  if [ "$rc" -eq 126 ] || [ "$rc" -eq 127 ]; then
    inconclusive 'harness-unexecutable' \
      "variant '$label': the prover could not be executed (status $rc); its runtime is absent from this environment"
  fi

  local osize esize
  osize=$(size_of "$out"); esize=$(size_of "$err")
  [ "${osize:-0}" -le "$max_stdout_bytes" ] || inconclusive 'output-bound' "variant '$label' wrote $osize bytes on stdout"
  [ "${esize:-0}" -le "$max_stderr_bytes" ] || inconclusive 'output-bound' "variant '$label' wrote $esize bytes on stderr"

  if grep -qF -- "$canary" "$out" "$err" 2>/dev/null; then
    fail 'canary-leak' "variant '$label' echoed AURUM_SECRET_CANARY into its output"
  fi
  if [ "${osize:-0}" -eq 0 ]; then
    # An `exit 0` stub lands exactly here: a status without a document is not a verdict.
    fail 'no-verdict' "variant '$label': the prover exited $rc without emitting a BrowserProofResultV1 document"
  fi
  if grep -qE '<[a-zA-Z/!?]' "$out"; then
    fail 'unsanitised-evidence' "variant '$label': the verdict carries raw markup"
  fi
  if grep -qE '"(screenshot|screenshot_png|html|raw_html)"[[:space:]]*:' "$out"; then
    fail 'unsanitised-evidence' "variant '$label': the verdict carries a screenshot or raw HTML field"
  fi

  pairs="$work/$label.tsv"
  if ! awk -f "$scanner" "$out" >"$pairs" 2>"$work/$label.parse"; then
    fail 'verdict-unparsable' "variant '$label' did not emit a parsable JSON document"
  fi
  if [ -n "$(cut -f1 "$pairs" | sort | uniq -d)" ]; then
    fail 'verdict-duplicate-key' "variant '$label' repeats a key in its verdict"
  fi

  [ "$(field schema 2>/dev/null || true)" = "$schema" ] \
    || fail 'verdict-foreign' "variant '$label' did not emit a $schema document"
  [ "$(field card 2>/dev/null || true)" = "$card" ] \
    || fail 'verdict-foreign' "variant '$label' emitted a verdict for another card"

  doc_outcome=$(field outcome 2>/dev/null) || fail 'verdict-incomplete' "variant '$label' has no outcome"
  doc_proved=$(field proved 2>/dev/null) || fail 'verdict-incomplete' "variant '$label' has no proved flag"
  doc_code=$(field code 2>/dev/null || true)

  # proved must agree with outcome, exactly as BrowserProofResultV1.Validate requires.
  case "$doc_outcome:$doc_proved" in
    proved:true|refused:false|inconclusive:false) ;;
    *) fail 'verdict-contradictory' "variant '$label': outcome '$doc_outcome' contradicts proved=$doc_proved" ;;
  esac

  # Identity fields must equal what this gate computed itself.
  [ "$(field artifact_digest 2>/dev/null || true)" = "$expect_digest" ] \
    || fail 'artifact-digest-mismatch' \
      "variant '$label' reported an artifact digest this gate cannot reproduce from the served bytes"
  [ "$(field driver_digest 2>/dev/null || true)" = "$driver_digest" ] \
    || fail 'driver-digest-mismatch' "variant '$label' reported a driver other than the one it was given"
  [ "$(field driver_kind 2>/dev/null || true)" = "$driver_kind" ] \
    || fail 'driver-kind-mismatch' "variant '$label' reported a driver kind other than the locked '$driver_kind'"
  [ "$(field entry_route 2>/dev/null || true)" = "$entry_route" ] \
    || fail 'route-plan-mismatch' "variant '$label' walked from an entry route this gate did not ask for"

  local dl
  dl=$(field navigation_deadline_ms 2>/dev/null || echo 0)
  [ "$dl" -le 30000 ] 2>/dev/null \
    || fail 'deadline-out-of-contract' "variant '$label' used a navigation deadline of ${dl}ms, above the 30s ceiling"

  n=$(field routes.len 2>/dev/null || echo 0)
  [ "$n" -ge 1 ] 2>/dev/null || fail 'verdict-incomplete' "variant '$label' observed no route"
  [ "$n" -le "$max_routes" ] || fail 'route-plan-mismatch' "variant '$label' declared $n routes, above the ceiling of $max_routes"
  [ "$(field routes.0.route 2>/dev/null || true)" = "$target_route" ] \
    || fail 'route-plan-mismatch' "variant '$label' did not observe $target_route"
  [ "$(field routes.0.selector 2>/dev/null || true)" = "$target_selector" ] \
    || fail 'route-plan-mismatch' "variant '$label' did not observe the selector $target_selector"

  # The environment diagnoses outrank everything: they are never read as a verdict about
  # the feature, in either direction.
  case "$doc_code" in
    BROWSERPROOF_TARGET_ABSENT|BROWSERPROOF_UNREACHABLE_ROUTE|BROWSERPROOF_EMPTY_RENDER|BROWSERPROOF_TEXT_MISMATCH|'') ;;
    *) inconclusive "$doc_code" "variant '$label': $(field detail 2>/dev/null || echo 'no verdict could be produced')" ;;
  esac

  if [ "$doc_outcome" = 'proved' ]; then
    # AC-001 / MUT-003: a document that only declares success is not evidence.
    local observed found reach status assertion
    observed=$(field routes.0.observed_text 2>/dev/null || true)
    found=$(field routes.0.selector_found 2>/dev/null || true)
    reach=$(field routes.0.reachable 2>/dev/null || true)
    status=$(field routes.0.status 2>/dev/null || true)
    assertion=$(field routes.0.assertion 2>/dev/null || true)
    if [ -n "$doc_code" ]; then
      fail 'verdict-contradictory' "variant '$label' is proved and still carries code '$doc_code'"
    fi
    [ -n "$(printf '%s' "$observed" | tr -d '[:space:]')" ] \
      || fail 'unsubstantiated-proof' "variant '$label' claims proof with no extracted text"
    [ "$found" = 'true' ] || fail 'unsubstantiated-proof' "variant '$label' claims proof without finding the selector"
    [ "$reach" = 'true' ] || fail 'unsubstantiated-proof' "variant '$label' claims proof over a route no navigation reached"
    [ "$status" = '200' ] || fail 'unsubstantiated-proof' "variant '$label' claims proof over HTTP status '$status'"
    [ "$assertion" = 'pass' ] || fail 'unsubstantiated-proof' "variant '$label' claims proof with assertion '$assertion'"
    case "$observed" in
      *"$expected_text"*) ;;
      *) fail 'unsubstantiated-proof' "variant '$label' extracted text that does not contain '$expected_text'" ;;
    esac
    # The extracted text must actually be in the served page: the prover may not invent it.
    grep -qF -- "$expected_text" "$site$target_route" 2>/dev/null \
      || fail 'unsubstantiated-proof' "variant '$label' reported text that the served page does not carry"
  fi

  # Only now is the exit status looked at, and only for AGREEMENT: the package returns a
  # non-nil error for every outcome that is not proof, so a prover that exits 0 while
  # refusing, or non-zero while proving, is not the published contract.
  if [ "$doc_outcome" = 'proved' ] && [ "$rc" -ne 0 ]; then
    fail 'status-contradicts-verdict' "variant '$label' proved the feature and exited $rc"
  fi
  if [ "$doc_outcome" != 'proved' ] && [ "$rc" -eq 0 ]; then
    fail 'status-contradicts-verdict' "variant '$label' refused the feature and still exited 0"
  fi

  if [ "$doc_outcome" != "$want_outcome" ] || [ "$doc_code" != "$want_code" ]; then
    fail 'AC-001' \
      "variant '$label' returned outcome='$doc_outcome' code='${doc_code:-<none>}', expected outcome='$want_outcome' code='${want_code:-<none>}'"
  fi

  observed_summary+="    {\"variant\":\"$label\",\"route\":\"$target_route\",\"selector\":\"$target_selector\",\"outcome\":\"$doc_outcome\",\"code\":\"${doc_code}\",\"artifact_digest\":\"$expect_digest\"},"$'\n'
  return 0
}

# --- the four variants of AC-001 ---------------------------------------------------
seen_codes=''
while IFS=: read -r label want_outcome want_code; do
  [ -n "$label" ] || continue
  case "$seen_codes" in
    *"|$want_outcome/$want_code|"*)
      infra "the embedded expectation table repeats the verdict $want_outcome/$want_code" ;;
  esac
  seen_codes+="|$want_outcome/$want_code|"
  run_variant "$label" "$fixture_root/$label" "$want_outcome" "$want_code"
done <<<"$variants"

# --- MUT-001 on a private shadow: prove, mutate, restore ---------------------------
# The shadow directory name is mktemp-random and its contents are copies, so a prover
# that answers from the fixture's name, or from a table, cannot pass the pristine leg and
# the mutated leg at the same time.
shadow="$work/shadow"
mkdir -p "$shadow"
cp -R -- "$fixture_root/nominal/." "$shadow/" || infra 'cannot build the shadow artifact'
pristine_digest=$(artifact_digest "$shadow") || infra 'cannot digest the shadow artifact'

run_variant 'shadow-pristine' "$shadow" 'proved' ''

rm -f -- "$shadow$target_route" || infra 'cannot apply MUT-001 to the shadow artifact'
mutated_digest=$(artifact_digest "$shadow") || infra 'cannot digest the mutated shadow artifact'
[ "$mutated_digest" != "$pristine_digest" ] || infra 'MUT-001 did not change the shadow artifact'
run_variant 'shadow-MUT-001' "$shadow" 'refused' 'BROWSERPROOF_TARGET_ABSENT'

cp -R -- "$fixture_root/nominal/." "$shadow/" || infra 'cannot restore the shadow artifact'
restored_digest=$(artifact_digest "$shadow") || infra 'cannot digest the restored shadow artifact'
[ "$restored_digest" = "$pristine_digest" ] \
  || fail 'restore-not-reproducible' 'restoring MUT-001 did not reproduce the pristine artifact identity'
run_variant 'shadow-restored' "$shadow" 'proved' ''

# --- pass record -------------------------------------------------------------------
printf '{\n'
printf '  "schema": "AcceptanceRecordV1",\n'
printf '  "card": "%s",\n' "$card"
printf '  "scenario": "%s",\n' "$scenario"
printf '  "result": "proved",\n'
printf '  "prover": "%s",\n' "$prover_rel"
printf '  "prover_digest": "%s",\n' "$prover_digest"
printf '  "driver": "%s",\n' "$driver_rel"
printf '  "driver_kind": "%s",\n' "$driver_kind"
printf '  "driver_digest": "%s",\n' "$driver_digest"
printf '  "entry_route": "%s",\n' "$entry_route"
printf '  "navigation_deadline_ms": %s,\n' "$deadline_ms"
printf '  "mutation": {"id": "MUT-001", "detected": true, "restored_digest": "%s"},\n' "$restored_digest"
printf '  "variants": [\n%s\n  ]\n' "${observed_summary%,$'\n'}"
printf '}\n'
exit 0
