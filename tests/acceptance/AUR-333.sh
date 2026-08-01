#!/usr/bin/env bash
#
# Acceptance program for card AUR-333, scenario AC-001.
#
# WHAT THIS PROVES (a durable property, not a point-in-time diff):
#
#   1. Every path the card declares as its own exists as a readable regular
#      file, so the property below is asserted over the card's COMPLETE
#      declared surface instead of over whichever files happen to be present.
#   2. None of those files carries a value with the SHAPE of a credential:
#      OpenAI `sk-`, AWS `AKIA`, GitHub `gh[pousr]_`, or a PEM private-key
#      opening marker. The shape families are the same ones the board runner
#      refuses to let cross a container boundary.
#   3. `RUN_DOCS_PIPELINE.md` obtains the provider secret only through an
#      environment indirection named OPENAI_API_KEY, that indirection carries
#      no inline fallback value, and the identifier really resolves through
#      the process environment: a synthetic value exported under that name is
#      read back through it, and the slot is empty when the name is unset.
#   4. `docs/specs/AUR-333.md` names the environment variable and records the
#      owner's credential-rotation obligation.
#
# WHAT THIS DELIBERATELY DOES NOT PROVE: that the credential once committed to
# this repository was rotated or revoked at the provider. Rotation is the
# repository owner's action, it happens outside this repository, and no local
# program can observe it. A pass here means the declared working surface has
# stopped distributing a credential-shaped value. It never means the historical
# secret became safe.
#
# SECRET HYGIENE: a matched value is never printed, never becomes a command
# argument, and never reaches another process. Scanning is bash pattern
# matching over lines read in-process; a finding reports the file, the line
# number and the shape class, and nothing else. The scanner is self-tested
# against synthetic tokens before any "zero findings" claim is made, so an
# empty result cannot come from a detector that stopped detecting.
#
# EXIT CODES are disjoint on purpose:
#   0 = the promised property holds
#   1 = behavioral RED: the property does not hold on this tree
#   3 = harness or environment error, which is never valid red evidence
#
# READ SURFACE: only the paths this card declares in `paths`, plus one
# ephemeral self-test fixture the program creates and deletes under the
# system temporary directory. The card list below is a deliberate mirror of
# the card front matter: `.board/cards` is in this card's `forbidden_paths`,
# so the card itself must not be read at run time.

set -Eeuo pipefail

readonly card='AUR-333'
readonly scenario='AC-001'
readonly rc_red=1
readonly rc_env=3

# Mirrors `paths` of .board/cards/*/AUR-333.md, in card order.
readonly -a declared_paths=(
  'RUN_DOCS_PIPELINE.md'
  'tests/security/redact-run-docs-pipeline.sh'
  'tests/acceptance/AUR-333.sh'
  'docs/specs/AUR-333.md'
)
readonly documented_path='RUN_DOCS_PIPELINE.md'
readonly guard_path='tests/security/redact-run-docs-pipeline.sh'
readonly spec_path='docs/specs/AUR-333.md'
readonly env_name='OPENAI_API_KEY'

# Credential SHAPE families. Every pattern below is written so that this file
# can be scanned by its own scanner without matching itself: the literal that
# would trigger a match is split across two adjacent quoted strings, or is
# followed here by a regex metacharacter that the pattern does not accept.
readonly -a shape_names=(
  'openai-secret-key'
  'aws-access-key-id'
  'github-token'
  'pem-private-key'
)
readonly -a shape_patterns=(
  'sk-[A-Za-z0-9_-]{20,}'
  'AKIA[0-9A-Z]{16}'
  'gh[pousr]_[A-Za-z0-9]{20,}'
  '-----BEG''IN[[:space:]][A-Z ]*PRIVATE KEY-----'
)

deliberate_exit=0
work_dir=''

cleanup() {
  if [[ -n "$work_dir" && -d "$work_dir" && ! -L "$work_dir" ]]; then
    rm -rf -- "$work_dir"
  fi
  return 0
}

# A harness error is loud and distinct. It is never reported as red evidence,
# because a missing tool or an unreadable file says nothing about the card.
env_error() {
  deliberate_exit=1
  printf '%s/%s: HARNESS-ERROR: %s\n' "$card" "$scenario" "$*" >&2
  exit "$rc_env"
}

red() {
  deliberate_exit=1
  printf '%s/%s: RED: %s\n' "$card" "$scenario" "$*" >&2
  exit "$rc_red"
}

on_unexpected() {
  local status=$?
  (( deliberate_exit == 1 )) && return 0
  printf '%s/%s: HARNESS-ERROR: unexpected failure (status %d) near line %s\n' \
    "$card" "$scenario" "$status" "${BASH_LINENO[0]:-unknown}" >&2
  exit "$rc_env"
}

trap cleanup EXIT
trap on_unexpected ERR

# --------------------------------------------------------------------------
# Preflight. Everything here is an environment fact, never a card fact.
# --------------------------------------------------------------------------

(( ${BASH_VERSINFO[0]:-0} >= 4 )) ||
  env_error "bash 4 or newer is required for indexed-array regex dispatch"

for tool in mktemp rm; do
  command -v "$tool" >/dev/null 2>&1 ||
    env_error "required host utility is absent: $tool"
done

work_dir="$(mktemp -d 2>/dev/null)" ||
  env_error "no writable temporary directory for the scanner self-test"
[[ -d "$work_dir" && -w "$work_dir" ]] ||
  env_error "temporary directory is not usable: $work_dir"

# --------------------------------------------------------------------------
# Scanner. Reports file, line number and shape class. Never the matched bytes.
# --------------------------------------------------------------------------

scan_file() {
  local file="$1"
  local line idx
  local -i line_no=0
  while IFS= read -r line || [[ -n "$line" ]]; do
    line_no=$(( line_no + 1 ))
    for idx in "${!shape_patterns[@]}"; do
      if [[ "$line" =~ ${shape_patterns[$idx]} ]]; then
        # Only these three fields ever leave the loop.
        printf '%s:%d: credential-shaped value, class=%s\n' \
          "$file" "$line_no" "${shape_names[$idx]}"
      fi
    done
  done < "$file"
}

# --------------------------------------------------------------------------
# Scanner self-test. A "zero findings" verdict is worthless unless the
# detector is known to still detect. The synthetic tokens are assembled at run
# time from fragments so that this source file never contains one; they are
# written to an ephemeral fixture outside the repository, and the fixture is
# removed on exit.
# --------------------------------------------------------------------------

self_test_scanner() {
  local fixture="$work_dir/shape-canary.txt"
  local findings leaked=0 name
  local -a canaries=()
  local sk_tail='A1b2C3d4E5f6G7h8J9k0L1m2'
  local aws_tail='V7QX3ZR2LM9TB4KD'
  local gh_tail='q4W7e1R8t5Y2u9I6o3P0aS7d'

  canaries+=( "sk-${sk_tail}" )
  canaries+=( "AKIA${aws_tail}" )
  canaries+=( "ghp_${gh_tail}" )
  canaries+=( "-----BEG""IN PRIVATE KEY-----" )

  {
    printf 'openai %s\n' "${canaries[0]}"
    printf 'aws %s\n' "${canaries[1]}"
    printf 'github %s\n' "${canaries[2]}"
    printf 'pem %s\n' "${canaries[3]}"
    printf 'clean line with no credential shape at all\n'
  } > "$fixture" 2>/dev/null ||
    env_error "cannot materialize the scanner self-test fixture"

  findings="$(scan_file "$fixture")" ||
    env_error "the scanner aborted on its own self-test fixture"

  for name in "${shape_names[@]}"; do
    [[ "$findings" == *"class=$name"* ]] ||
      env_error "the scanner no longer detects shape class $name; a zero-finding result would be meaningless"
  done

  # Sink canary: a finding line must never carry the matched bytes. The
  # offending output is detected, counted and discarded, never echoed.
  for name in "${canaries[@]}"; do
    if [[ "$findings" == *"$name"* ]]; then
      leaked=$(( leaked + 1 ))
    fi
  done
  findings=''
  (( leaked == 0 )) ||
    env_error "the scanner emitted matched bytes into its own findings ($leaked of ${#canaries[@]} classes); output withheld"

  rm -f -- "$fixture" 2>/dev/null || true
}

self_test_scanner

# --------------------------------------------------------------------------
# 1. The declared surface must be complete before any claim is made about it.
# --------------------------------------------------------------------------

declared_purpose() {
  case "$1" in
    "$documented_path") printf 'the pipeline guide whose credential slot this card redacts' ;;
    "$guard_path") printf 'the re-runnable redaction guard this card promises' ;;
    "$spec_path") printf "the card's credential-reference specification" ;;
    *) printf 'this acceptance program' ;;
  esac
}

for declared in "${declared_paths[@]}"; do
  if [[ ! -e "$declared" && ! -L "$declared" ]]; then
    red "declared artifact absent: $declared ($(declared_purpose "$declared")); the no-credential-shape property cannot be asserted over an incomplete declared surface"
  fi
  [[ ! -L "$declared" ]] ||
    red "declared artifact is a symlink and may resolve outside the declared surface: $declared"
  [[ -f "$declared" ]] ||
    red "declared artifact is not a regular file: $declared"
  [[ -r "$declared" ]] ||
    env_error "declared artifact exists but is unreadable: $declared"
done

for executable in "$guard_path" 'tests/acceptance/AUR-333.sh'; do
  [[ -x "$executable" ]] ||
    red "declared program is present but not executable, so the property cannot be re-verified on demand: $executable"
done

# --------------------------------------------------------------------------
# 2. No declared file may carry a credential-shaped value.
# --------------------------------------------------------------------------

shape_findings=''
for declared in "${declared_paths[@]}"; do
  file_findings="$(scan_file "$declared")" ||
    env_error "the scanner aborted while reading $declared"
  if [[ -n "$file_findings" ]]; then
    shape_findings+="$file_findings"$'\n'
  fi
done

if [[ -n "$shape_findings" ]]; then
  printf '%s/%s: RED: credential-shaped values found on the declared surface (file, line and class only):\n' \
    "$card" "$scenario" >&2
  printf '%s' "$shape_findings" >&2
  red "the declared surface must carry zero credential-shaped values"
fi

# --------------------------------------------------------------------------
# 3. The environment indirection must be present, undefaulted, and resolvable.
# --------------------------------------------------------------------------

reference="\${${env_name}}"
reference_lines=0
defaulted_line=0
literal_assignment_line=0
line_no=0

while IFS= read -r doc_line || [[ -n "$doc_line" ]]; do
  line_no=$(( line_no + 1 ))
  if [[ "$doc_line" == *"$reference"* ]]; then
    reference_lines=$(( reference_lines + 1 ))
  fi
  # `${OPENAI_API_KEY:-something}` would smuggle an inline fallback value back
  # into the document, so the reference must be the bare identifier.
  if (( defaulted_line == 0 )) && [[ "$doc_line" =~ \$\{$env_name[^}]+\} ]]; then
    defaulted_line=$line_no
  fi
  # `OPENAI_API_KEY=<anything that is not the indirection and not empty>`
  # would put the value back into the document in plain form.
  if (( literal_assignment_line == 0 )) && [[ "$doc_line" =~ (^|[^A-Za-z0-9_])$env_name=[^[:space:]]+ ]]; then
    if [[ "$doc_line" != *"$env_name=$reference"* ]]; then
      literal_assignment_line=$line_no
    fi
  fi
done < "$documented_path"

(( reference_lines > 0 )) ||
  red "$documented_path does not reference the environment variable $env_name as \${$env_name}, so the provider secret is not sourced from the environment"
(( defaulted_line == 0 )) ||
  red "$documented_path line $defaulted_line expands $env_name with an inline fallback; the reference must be the bare identifier"
(( literal_assignment_line == 0 )) ||
  red "$documented_path line $literal_assignment_line assigns $env_name a literal instead of the environment indirection"

[[ "$env_name" =~ ^[A-Z][A-Z0-9_]*$ ]] ||
  red "$env_name is not a resolvable environment identifier"

# Resolution probe. It runs in subshells that first unset the name, so a real
# value present in the caller's environment can never be read, exported,
# compared or printed here. The probe value is synthetic and local.
probe_value="aur-333-probe-$$-${RANDOM}"
resolved_value="$(unset -v "$env_name"; export "$env_name=$probe_value"; printf '%s' "${!env_name}")"
absent_value="$(unset -v "$env_name"; printf '%s' "${!env_name-}")"

[[ "$resolved_value" == "$probe_value" ]] ||
  env_error "the shell did not resolve $env_name from the process environment; the probe cannot judge the document"
[[ -z "$absent_value" ]] ||
  env_error "the shell resolved $env_name while it was unset; the probe cannot judge the document"
resolved_value=''
probe_value=''

# --------------------------------------------------------------------------
# 4. The specification must record the contract and the owner's obligation.
# --------------------------------------------------------------------------

spec_names_variable=0
spec_records_rotation=0
while IFS= read -r spec_line || [[ -n "$spec_line" ]]; do
  [[ "$spec_line" != *"$env_name"* ]] || spec_names_variable=1
  if [[ "$spec_line" =~ [Rr]ota(t(e|ion|ed)|c[aã]o|cionar|ciona) ]]; then
    spec_records_rotation=1
  fi
done < "$spec_path"

(( spec_names_variable == 1 )) ||
  red "$spec_path does not name the environment variable $env_name, so the documented contract does not match the redacted slot"
(( spec_records_rotation == 1 )) ||
  red "$spec_path does not record the owner's credential-rotation obligation, which this acceptance cannot prove for the exposed history"

# --------------------------------------------------------------------------
# Observation only. This program writes no evidence and issues no verdict.
# `history_rotation_proven` is false by construction: rotating the credential
# that reached Git history is the owner's action and is out of scope here.
# --------------------------------------------------------------------------

printf '{"card":"%s","scenario":"%s","result":"pass","declared_paths_scanned":%d,"shape_classes_checked":%d,"credential_shape_findings":0,"env_reference":"%s","env_reference_resolves":true,"secret_bytes_emitted":false,"history_rotation_proven":false}\n' \
  "$card" "$scenario" "${#declared_paths[@]}" "${#shape_patterns[@]}" "$env_name"
