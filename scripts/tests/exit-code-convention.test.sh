#!/usr/bin/env bash
#
# Guard test for the split between two independent exit-code conventions in
# this tree:
#
#   * tests/acceptance/EXIT_CODE_CONVENTION.md governs acceptance programs,
#     each materialized alone into an ephemeral per-card container. Its
#     "inconclusive/infrastructure" code is 79.
#   * scripts/action-entrypoint.sh, scripts/build-docs-site.sh,
#     scripts/generate-enhanced-docs.sh and
#     scripts/tests/documentation-mode.test.sh are a separate family running
#     inside the built image, using 3 for "environment failure" and 20 for a
#     documentation generator's own "legitimate no-op".
#
# The two families are free to give the same number different meanings for two
# reasons this program recomputes rather than asserts: the entrypoint reads its
# helpers' statuses by literal value (so renumbering one file breaks the
# others), and no acceptance program ever runs a scripts/ file, so no exit
# status is ever read against the other family's table (EC-2).
#
# DESIGN NOTE (round 1): earlier revisions caught false claims with a
# negation heuristic - find an assertion, then decide whether the words right
# before it disclaim it ("does not mirror", "is not a defect"). That heuristic
# is inherently a word list trying to parse English negation in bash, and
# three independent review rounds each produced a sentence that walked
# through it: an unrelated negator from an earlier clause bled into the
# lookback window when no punctuation separated the two, and decorative or
# idiomatic negator-shaped words ("rather", "no doubt") flipped a real,
# un-negated order into a falsely-accepted disclaim. No window size and no
# negator list closes that class; it is not a bug in one constant, it is the
# approach. That revision removed negation analysis entirely (EC-1, EC-4,
# EC-5 no longer looked at what preceded a match) and replaced the two cases
# that depended on it (EC-4's authority disclaim, all of EC-5) with a fixed
# line matched with `grep -F` plus a forbidden word-pair list that failed
# regardless of framing, negation, hedging or irony.
#
# DESIGN NOTE (round 2, this revision): the forbidden-word-pair list lasted
# exactly as long as it took to try fresh wording against it. Two further
# independent review rounds each produced a sentence that stated the same
# false claim while dodging the list: a convergence verb the list did not
# enumerate ("land on 79" instead of any of converge/renumber/migrate/switch/
# adopt/standardize), and a defect claim with no backtick around the digit
# the pattern required ("3 a mistake" instead of `` `3` is a mistake ``).
# Five rounds, two independent heuristics, one conclusion: a lexical detector
# over free-form prose is exactly as paraphrasable as the prose itself, no
# matter how the word list or window is tuned. Continuing to widen the list
# is not closing a gap - it is fingir capacidade, claiming a capability this
# program does not have. This revision deletes FORBIDDEN_CONVERGE,
# FORBIDDEN_DEFECT and every prose-opinion scan from EC-5 outright, with no
# lexical successor, and replaces EC-5's non-AUTHORITY_LINE assertion with a
# measured fact a sentence cannot talk around: see the comment directly above
# case_ec5() for what that fact is and why it is not vulnerable to the same
# class of attack. EC-1 keeps its own, separately-scoped lexical existence
# check (no verbatim line is possible across N independent files with
# free-form headers) - see the case_ec1 comment for the honest limit that
# remains there; that limit is orthogonal to the one closed in EC-5 here,
# because EC-1 checks structural presence of an agreement claim, not the
# truth of an opinion about which code family should converge on which
# number.
#
# EXIT CODES: 0 all selected cases held, 1 at least one case failed, 3
# harness/setup problem (nothing measured), 64 unknown selector. This program
# lives under scripts/tests/, so it uses the scripts/ family's numbering
# described above; tests/acceptance/EXIT_CODE_CONVENTION.md does not apply to
# it.

set -uo pipefail
export LC_ALL=C

SCRIPTS_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
REPO_ROOT="$(cd -- "$SCRIPTS_DIR/.." && pwd -P)"
CONVENTION="$REPO_ROOT/tests/acceptance/EXIT_CODE_CONVENTION.md"
ACCEPTANCE_DIR="$REPO_ROOT/tests/acceptance"
SELF="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)/$(basename -- "${BASH_SOURCE[0]}")"

# The case registry is the single source of truth for both selector validation
# and dispatch, so "selector selected zero cases" and "unknown selector" cannot
# drift apart into a branch nothing can reach.
ALL_CASES=(EC-1 EC-2 EC-3 EC-4 EC-5 EC-6)

# EXIT_CODE_CONVENTION.md's own rule: a literal list a program depends on must
# assert its own non-emptiness before it is used, or an emptied list reports
# success having checked nothing. This is that assertion for this program.
if [ "${#ALL_CASES[@]}" -eq 0 ]; then
  printf 'HARNESS: the case registry is empty; this run would report success having measured nothing\n' >&2
  exit 3
fi

selector="${1:-ALL}"
selected=()
for c in "${ALL_CASES[@]}"; do
  if [ "$selector" = ALL ] || [ "$selector" = "$c" ]; then
    selected+=("$c")
  fi
done
if [ "${#selected[@]}" -eq 0 ]; then
  printf 'exit-code-convention/unknown-selector: %s\n' "$selector" >&2
  exit 64
fi

[ -f "$CONVENTION" ] || { printf 'ENVIRONMENT: %s not found\n' "$CONVENTION" >&2; exit 3; }
[ -d "$ACCEPTANCE_DIR" ] || { printf 'ENVIRONMENT: %s not found\n' "$ACCEPTANCE_DIR" >&2; exit 3; }

# PREFLIGHT: EC-1, EC-4 and EC-5 depend on `grep -E` honoring ERE interval
# expressions ({0,N}) to bound how far one anchor may sit from another
# (an agreement verb from "tests/acceptance" in EC-1; `3` from "environment
# failure" in EC-4; a variable name from its documented digit in EC-5). If
# the engine silently ignores the interval, the patterns never match, every
# match-based case emits nothing, and each one built on it reports PASS
# having measured nothing - a degraded environment must be reported as
# environment failure (exit 3), never as a passing verdict. Assert the
# actual mechanism now, before any case trusts it.
if ! printf 'mirroring tests/acceptance\n' \
  | grep -qE -- '(mirror[a-z]*|match[a-z]*).{0,30}tests/acceptance'; then
  printf 'HARNESS: grep -E on this host does not honor ERE interval expressions ({0,N}); EC-1/EC-4/EC-5 would measure nothing\n' >&2
  exit 3
fi

failures=0
verdicts=0
pass() { printf 'PASS  %-5s %s\n' "$1" "$2"; verdicts=$((verdicts + 1)); }
fail() { printf 'FAIL  %-5s %s\n' "$1" "$2"; failures=$((failures + 1)); verdicts=$((verdicts + 1)); }

# --------------------------------------------------------------------------
# Shared text-normalization helper.
#
# Markdown and shell-comment prose wraps a sentence across multiple source
# lines; `grep -E` matches within one line only, so an interval pattern like
# `verb.{0,30}anchor` would miss a case where the wrap happens to fall between
# the verb and the anchor even though a reader sees one sentence. flatten()
# joins a file into a single lowercased, whitespace-squashed line before any
# pattern is tested against it, purely so a bounded-distance pattern sees the
# same text a human reading the rendered prose would. It performs no negation
# analysis and keeps no memory of clause boundaries - it is a formatting
# step, not a semantic one.
# --------------------------------------------------------------------------
flatten() {
  awk '{ buf = buf " " $0 }
       END {
         gsub(/[ \t]+/, " ", buf)
         print tolower(buf)
       }' "$1"
}

# pattern_present FILE PATTERN
#   True (0) iff PATTERN (ERE) matches anywhere in FILE's flattened text.
#   This is a bare existence check: it does not look at, or care about, what
#   comes before or after the match. A match fails the case it backs,
#   unconditionally - reworded, hedged, ironized or "disclaimed" text is not
#   read differently than a flat assertion, because nothing here reads it at
#   all beyond "does the pattern occur".
pattern_present() {
  local file="$1" pat="$2" flat
  flat="$(flatten "$file")"
  [ -z "${flat// /}" ] && return 1
  printf '%s' "$flat" | grep -qE -- "$pat"
}

# The mawk (`/usr/bin/awk` on Debian/Ubuntu, so `ubuntu-latest`) risk noted in
# earlier revisions is unchanged by this rewrite: mawk does not honor ERE
# intervals, so no pattern below asks awk to evaluate one. flatten()'s only
# regex is a fixed-character-class whitespace collapse; every interval-bearing
# pattern (MIRROR_CLAIM, the EC-4 environment-failure check, EC-5's
# EXIT_ENVIRONMENT/ENHANCE_NOOP/EXIT_NOOP digit-proximity checks) is matched
# by `grep -E`, which honors ERE intervals in gawk's grep, GNU grep on
# Debian/Ubuntu, and BusyBox grep alike (see the PREFLIGHT above, which
# asserts this before any case trusts it).

# --------------------------------------------------------------------------
# EC-1  No file under scripts/ asserts that its exit codes agree with
#       tests/acceptance/*.sh's convention. It cannot: scripts/ uses 3 for
#       environment failure and that convention uses 79. Detected as an
#       agreement verb within a bounded distance of "tests/acceptance",
#       structural existence only - no negation is evaluated, so a
#       disclaiming sentence must simply never place one of these verbs that
#       close to the anchor (the legitimate headers in this family are
#       written to name tests/acceptance/EXIT_CODE_CONVENTION.md without ever
#       using an agreement verb near it). This program is excluded from its
#       own scan: it is the guard, not a member of the family making claims.
#
#       HONEST LIMIT: MIRROR_CLAIM is a literal keyword list, not semantic
#       understanding of English. A synonym this list does not enumerate
#       (e.g. "harmonizes with tests/acceptance") would not be caught. The
#       list is deliberately small, literal and reviewable rather than
#       exhaustive; closing that residual gap completely would require
#       parsing meaning, not matching words, and is out of scope for this
#       guard.
# --------------------------------------------------------------------------
MIRROR_CLAIM='(mirror[a-z]*|match[a-z]*|same|identical|equivalent|consistent|agree[a-z]*|align[a-z]*|follow[a-z]*|governed by|bound by|conform[a-z]*|reus[a-z]*|copi[a-z]*|in sync|shared).{0,30}tests/acceptance'

case_ec1() {
  local hits=() scanned=() f
  while IFS= read -r -d '' f; do
    [ "$f" = "$SELF" ] && continue
    scanned+=("$f")
    if pattern_present "$f" "$MIRROR_CLAIM"; then
      hits+=("$f")
    fi
  done < <(find "$SCRIPTS_DIR" -type f -name '*.sh' -print0)

  if [ "${#scanned[@]}" -eq 0 ]; then
    printf 'HARNESS: no scripts/*.sh file found under %s (excluding this guard); EC-1 has nothing to scan and would measure nothing\n' "$SCRIPTS_DIR" >&2
    exit 3
  fi

  if [ "${#hits[@]}" -gt 0 ]; then
    fail EC-1 "scripts/ file(s) still assert agreement with tests/acceptance/*.sh's exit codes: ${hits[*]}"
    return
  fi
  pass EC-1 "no scripts/*.sh file asserts that its exit codes agree with tests/acceptance/*.sh"
}

# --------------------------------------------------------------------------
# EC-2  Nothing under tests/acceptance/*.sh invokes a scripts/ file. This is
#       the fact that makes the two conventions independently changeable
#       without a runtime regression; if it ever stops holding, the two
#       families DO have to agree on one set of numbers and this guard must
#       be revisited, not silently left green.
#
#       Every line naming one of the three dispatchable scripts counts as an
#       invocation unless it matches one of three shapes that are data, not a
#       command: a comment, a value bound to a name (`x=path`, `[k]=path`,
#       `awk -v n=path`), or a line that is nothing but the bare path (the
#       inventory lists in AUR-001 and AUR-312). Any other position - `bash
#       path`, a direct `"$REPO/path"` execution, a path handed to `docker
#       run` - is reported. The bias is deliberate: an unrecognized shape is
#       reported rather than assumed harmless.
# --------------------------------------------------------------------------
SCRIPT_PATH_RE='scripts/(action-entrypoint|build-docs-site|generate-enhanced-docs)\.sh'

case_ec2() {
  local invokers=() scanned=() f line
  while IFS= read -r -d '' f; do
    scanned+=("$f")
    while IFS= read -r line; do
      # Shape 1: comment line.
      printf '%s' "$line" | grep -qE '^[[:space:]]*#' && continue
      # Shape 2: the path is the value bound to a name.
      printf '%s' "$line" | grep -qE "=[\"']?[^[:space:]\"']*${SCRIPT_PATH_RE}" && continue
      # Shape 3: the line is nothing but the bare relative path (inventory).
      printf '%s' "$line" | grep -qE "^[[:space:]]*${SCRIPT_PATH_RE}[[:space:]]*$" && continue
      invokers+=("$f: $line")
    done < <(grep -hE "$SCRIPT_PATH_RE" "$f" 2>/dev/null)
  done < <(find "$ACCEPTANCE_DIR" -maxdepth 1 -type f -name '*.sh' -print0)

  if [ "${#scanned[@]}" -eq 0 ]; then
    printf 'HARNESS: no tests/acceptance/*.sh file found under %s; EC-2 has nothing to scan and would measure nothing\n' "$ACCEPTANCE_DIR" >&2
    exit 3
  fi

  if [ "${#invokers[@]}" -gt 0 ]; then
    fail EC-2 "tests/acceptance/*.sh now invokes scripts/ (the two conventions must be reconciled): ${invokers[*]}"
    return
  fi
  pass EC-2 "no tests/acceptance/*.sh program invokes a scripts/ file"
}

# --------------------------------------------------------------------------
# EC-3  EXIT_NOOP=20 (scripts/generate-enhanced-docs.sh) is covered by an
#       explicit, standing explanation of what it means - not left as a bare
#       number nothing documents.
# --------------------------------------------------------------------------
case_ec3() {
  local enhance="$SCRIPTS_DIR/generate-enhanced-docs.sh"
  [ -f "$enhance" ] || { fail EC-3 "$enhance not found"; return; }
  if ! grep -q 'EXIT_NOOP=20' "$enhance"; then
    fail EC-3 "EXIT_NOOP=20 no longer defined in generate-enhanced-docs.sh; update this guard if it was intentionally removed"
    return
  fi
  if ! grep -q 'legitimate no-op' "$enhance"; then
    fail EC-3 "EXIT_NOOP=20 is defined but its header no longer explains the 'legitimate no-op' meaning"
    return
  fi
  pass EC-3 "EXIT_NOOP=20 is defined and its meaning is documented in generate-enhanced-docs.sh"
}

# Prints the "## Scope" section of the convention document, heading included.
scope_section() {
  awk '/^## Scope/ { inside = 1; print; next }
       /^## / { inside = 0 }
       inside { print }' "$CONVENTION"
}

# --------------------------------------------------------------------------
# AUTHORITY_LINE  The single sentence this file uses to state, in a form a
#   machine can check without reading for meaning, that it does not govern
#   scripts/: each scripts/*.sh file documents its own codes inline, and this
#   file's authority stops at tests/acceptance/. EC-4 and EC-5 both require
#   this exact line, byte for byte (`grep -F`, whole-line), to be present in
#   tests/acceptance/EXIT_CODE_CONVENTION.md. A fixed-string, whole-line
#   match cannot be defeated by rewording, negation tricks or decorative
#   filler the way a semantic "does this disclaim authority" check could -
#   the line either is there, unedited, or the case fails. The cost is the
#   mirror image of that strength: this checks presence of the sentence, not
#   the absence of some other sentence that might contradict it elsewhere in
#   the file. That tradeoff is deliberate here; see EC-5 for the guard that
#   cross-checks the rest of the document's exit-code claims against
#   measured digits instead - not by scanning prose for a forbidden pattern
#   (see EC-5's own comment for why that approach was abandoned).
# --------------------------------------------------------------------------
AUTHORITY_LINE='Authority: each scripts/*.sh file documents its own exit codes inline; this file governs tests/acceptance/ only.'

# --------------------------------------------------------------------------
# EC-4  tests/acceptance/EXIT_CODE_CONVENTION.md is honest about its own
#       scope. Three conjuncts, all required, because any one of them alone
#       would be satisfied by prose that says the opposite of the fix:
#         (a) the scope section names scripts/action-entrypoint.sh, so it is
#             documenting this concrete family and not scripts/ in passing;
#         (b) it records 3 as that family's environment-failure code, so a
#             reader of the 79 table does not conclude 3 was an error;
#         (c) the document contains AUTHORITY_LINE verbatim - the fixed,
#             machine-readable statement that this file's authority does not
#             extend past tests/acceptance/.
# --------------------------------------------------------------------------
case_ec4() {
  local section
  section="$(scope_section)"
  if [ -z "$section" ]; then
    fail EC-4 "$CONVENTION has no '## Scope' section, so it documents nothing about the scripts/ family"
    return
  fi

  local tmp
  tmp="$(mktemp)" || { fail EC-4 "could not create a temporary file"; return; }
  printf '%s\n' "$section" >"$tmp"

  if ! grep -q 'scripts/action-entrypoint.sh' "$tmp"; then
    fail EC-4 "the scope section never names scripts/action-entrypoint.sh, so it is not documenting the split with that family"
    rm -f "$tmp"
    return
  fi
  # shellcheck disable=SC2016  # the backticks are literal Markdown, not a subshell
  if ! grep -qE '`3`.{0,40}environment failure' "$tmp"; then
    fail EC-4 "the scope section does not record 3 as the scripts/ family's environment-failure code"
    rm -f "$tmp"
    return
  fi
  rm -f "$tmp"

  if ! grep -qxF -- "$AUTHORITY_LINE" "$CONVENTION"; then
    fail EC-4 "$CONVENTION does not contain the AUTHORITY_LINE verbatim (a wording edit, not just a false claim elsewhere, trips this)"
    return
  fi
  pass EC-4 "the scope section names scripts/action-entrypoint.sh, records 3, and carries AUTHORITY_LINE verbatim"
}

# --------------------------------------------------------------------------
# EC-5  LIMITATION - read this before touching the case below. Opinion or
#       editorial framing in free prose is not mechanically assertable: five
#       rounds of adversarial paraphrase against earlier revisions of this
#       case proved it. A negation-aware heuristic got bled by an unrelated
#       negator bleeding across a missing clause boundary; a plain
#       forbidden-word-pair list that replaced it got dodged by a convergence
#       verb the list did not enumerate and a defect claim missing the
#       backtick the pattern required. Every fix was a bigger word list, and
#       every bigger word list was still just a word list - paraphrasable by
#       construction, no matter how it is tuned. This revision stops trying
#       to referee prose. EC-5 no longer reads a single word of the Scope
#       section's editorial content for intent, and there is no lexical
#       successor to FORBIDDEN_CONVERGE/FORBIDDEN_DEFECT.
#
#       The real risk that prose-scanning was reaching for - scripts/
#       actually changing which exit code means what, out of step with what
#       the doc claims - is caught instead by a fact a sentence cannot talk
#       around: this case cross-checks the digits
#       tests/acceptance/EXIT_CODE_CONVENTION.md documents next to the names
#       EXIT_ENVIRONMENT and ENHANCE_NOOP/EXIT_NOOP against the digits those
#       same names are actually assigned in scripts/action-entrypoint.sh,
#       scripts/build-docs-site.sh and scripts/generate-enhanced-docs.sh -
#       the same documented-equals-measured pattern EC-6 already uses for the
#       69-USERS block, applied to two variable names instead of a card-ID
#       list. A convergence has no effect on behavior until an `EXIT_*=` line
#       somewhere is actually edited, and that line is exactly what this case
#       measures; no amount of surrounding wording changes a measured digit.
#       EC-1's structural, negation-free existence check plays the analogous
#       role for "does scripts/ claim it mirrors tests/acceptance/".
#
#       Two assertions remain, neither of which reads Scope-section prose for
#       intent:
#         (a) AUTHORITY_LINE (see above) is present verbatim - `grep -F`,
#             fixed string, whole line. Same check as EC-4(c); listed again
#             here because EC-5 is what a reader should consult for "does
#             this file claim authority it shouldn't".
#         (b) the digit(s) documented next to EXIT_ENVIRONMENT, and the
#             digit(s) documented next to ENHANCE_NOOP/EXIT_NOOP, in
#             tests/acceptance/EXIT_CODE_CONVENTION.md equal the digit(s)
#             those same names are actually assigned to in the three
#             dispatched scripts/ files - checked in both directions, so a
#             doc that cites a code no script uses, and a script that uses a
#             code the doc never attributes to it, both fail.
#
#       NOT covered any more, on purpose: prose in the Scope section that
#       opines, argues or editorializes about the split (for example,
#       "arguably scripts/ should converge on 79 eventually") no longer fails
#       this case. That prose changes nothing a script does, so it is not a
#       risk this guard needs to police; the moment it stops being talk and
#       starts being an edited `EXIT_*=` line, (b) above catches it.
# --------------------------------------------------------------------------
DISPATCHED_SCRIPTS_RE='EXIT_ENVIRONMENT=[0-9]+'
NOOP_SCRIPTS_RE='(ENHANCE_NOOP|EXIT_NOOP)=[0-9]+'

case_ec5() {
  if ! grep -qxF -- "$AUTHORITY_LINE" "$CONVENTION"; then
    fail EC-5 "$CONVENTION does not contain the AUTHORITY_LINE verbatim"
    return
  fi

  local dispatched=(
    "$SCRIPTS_DIR/action-entrypoint.sh"
    "$SCRIPTS_DIR/build-docs-site.sh"
    "$SCRIPTS_DIR/generate-enhanced-docs.sh"
  )
  local f
  for f in "${dispatched[@]}"; do
    if [ ! -f "$f" ]; then
      printf 'HARNESS: %s not found; EC-5 has nothing to cross-check\n' "$f" >&2
      exit 3
    fi
  done

  local measured_env measured_noop
  measured_env="$(grep -hoE -- "$DISPATCHED_SCRIPTS_RE" "${dispatched[@]}" | grep -oE '[0-9]+$' | sort -u)"
  measured_noop="$(grep -hoE -- "$NOOP_SCRIPTS_RE" "${dispatched[@]}" | grep -oE '[0-9]+$' | sort -u)"

  if [ -z "$measured_env" ]; then
    printf 'HARNESS: no dispatched scripts/*.sh file assigns EXIT_ENVIRONMENT=<digits>; EC-5 has nothing to cross-check and measured nothing\n' >&2
    exit 3
  fi
  if [ -z "$measured_noop" ]; then
    printf 'HARNESS: no dispatched scripts/*.sh file assigns ENHANCE_NOOP=<digits> or EXIT_NOOP=<digits>; EC-5 has nothing to cross-check and measured nothing\n' >&2
    exit 3
  fi

  local flat
  flat="$(flatten "$CONVENTION")"
  if [ -z "${flat// /}" ]; then
    fail EC-5 "$CONVENTION is empty; EC-5 cannot cross-check documented exit codes"
    return
  fi

  local documented_env documented_noop
  documented_env="$(printf '%s' "$flat" \
    | grep -oE 'exit_environment[^0-9]{0,8}[0-9]+|[0-9]+[^0-9]{0,8}exit_environment' \
    | grep -oE '[0-9]+' | sort -u)"
  documented_noop="$(printf '%s' "$flat" \
    | grep -oE '(enhance_noop|exit_noop)[^0-9]{0,8}[0-9]+|[0-9]+[^0-9]{0,8}(enhance_noop|exit_noop)' \
    | grep -oE '[0-9]+' | sort -u)"

  if [ -z "$documented_env" ]; then
    fail EC-5 "$CONVENTION never states a digit near EXIT_ENVIRONMENT, so its claim about the scripts/ family's environment-failure code is unverifiable"
    return
  fi
  if [ -z "$documented_noop" ]; then
    fail EC-5 "$CONVENTION never states a digit near ENHANCE_NOOP/EXIT_NOOP, so its claim about the scripts/ family's no-op code is unverifiable"
    return
  fi

  if [ "$measured_env" != "$documented_env" ]; then
    fail EC-5 "EXIT_ENVIRONMENT staleness: scripts/ assigns [$(echo "$measured_env" | tr '\n' ' ')] but $CONVENTION documents [$(echo "$documented_env" | tr '\n' ' ')] next to that name"
    return
  fi
  if [ "$measured_noop" != "$documented_noop" ]; then
    fail EC-5 "ENHANCE_NOOP/EXIT_NOOP staleness: scripts/ assigns [$(echo "$measured_noop" | tr '\n' ' ')] but $CONVENTION documents [$(echo "$documented_noop" | tr '\n' ' ')] next to that name"
    return
  fi

  pass EC-5 "AUTHORITY_LINE is present verbatim, and the documented EXIT_ENVIRONMENT/ENHANCE_NOOP/EXIT_NOOP digits match what scripts/ actually assigns"
}

# --------------------------------------------------------------------------
# EC-6  The document's list of acceptance programs still using 69 is
#       recomputed from the tree, not trusted. The document carries the list
#       inside 69-USERS markers; this case regenerates it from
#       tests/acceptance/AUR-*.sh and fails on any difference, so the
#       enumeration cannot silently rot as programs are added or renumbered.
# --------------------------------------------------------------------------
case_ec6() {
  local measured documented
  measured="$(grep -lE '(exit|=)[[:space:]]*69([^0-9]|$)' "$ACCEPTANCE_DIR"/AUR-*.sh 2>/dev/null \
    | sed -e 's#.*/##' -e 's#\.sh$##' | sort -u)"

  if [ -z "$measured" ]; then
    printf 'HARNESS: no tests/acceptance/AUR-*.sh program uses 69; EC-6 has nothing to compare and measured nothing\n' >&2
    exit 3
  fi

  documented="$(awk '/69-USERS:BEGIN/ { inside = 1; next }
                     /69-USERS:END/ { inside = 0 }
                     inside { print }' "$CONVENTION" \
    | grep -oE 'AUR-[0-9]+' | sort -u)"

  if [ -z "$documented" ]; then
    fail EC-6 "$CONVENTION has no non-empty 69-USERS block, so its claim about which programs still use 69 is unverifiable"
    return
  fi
  if [ "$measured" != "$documented" ]; then
    fail EC-6 "the 69-USERS block is stale: documented [$(echo "$documented" | tr '\n' ' ')] but measured [$(echo "$measured" | tr '\n' ' ')]"
    return
  fi
  pass EC-6 "the 69-USERS block matches the $(echo "$measured" | wc -l | tr -d ' ') program(s) measured in tests/acceptance/"
}

for c in "${selected[@]}"; do
  case "$c" in
    EC-1) case_ec1 ;;
    EC-2) case_ec2 ;;
    EC-3) case_ec3 ;;
    EC-4) case_ec4 ;;
    EC-5) case_ec5 ;;
    EC-6) case_ec6 ;;
    *) printf 'HARNESS: no implementation for selected case %s\n' "$c" >&2; exit 3 ;;
  esac
done

if [ "$verdicts" -ne "${#selected[@]}" ]; then
  printf '\nHARNESS: %d case(s) selected but only %d reached a verdict\n' "${#selected[@]}" "$verdicts" >&2
  exit 3
fi

if [ "$failures" -ne 0 ]; then
  printf '\n%d of %d case(s) failed\n' "$failures" "$verdicts"
  exit 1
fi
printf '\nall %d case(s) held\n' "$verdicts"
exit 0
