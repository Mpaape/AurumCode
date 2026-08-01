#!/usr/bin/env bash
#
# Acceptance program for card AUR-014, scenario AC-001.
#
# WHAT THIS PROVES
#
#   The versioned code-review standard exists as a machine-checkable catalog
#   under `standards/code-review/` and covers EVERY control the card declares in
#   its `controls` field, with, per control:
#     * a primary source: name, allowlisted https URL, version, ISO date, and a
#       `current` (not withdrawn) status;
#     * at least one POSITIVE case whose declared verdict is `pass`;
#     * at least one NEGATIVE case whose declared verdict is `fail`;
#     * for each case, a bounded LOCAL fixture that exists under
#       `standards/code-review/` and names the rule ID it belongs to, plus a
#       non-empty `assert` stating the expected observation;
#     * fixtures that actually DISTINGUISH the two verdicts: no fixture file may
#       be referenced by two cases anywhere in the catalog, and a rule's positive
#       and negative fixtures may not be byte-identical. Without this, a single
#       file naming every rule ID satisfies every case and the catalog proves
#       nothing.
#
#   Coverage is credited only for cases that are themselves fully valid, so a
#   control cannot be covered by a case with a missing fixture, an inverted
#   verdict, or a fixture shared with another case.
#
#   It also retains the coverage of the previous revision of this program: the
#   research note at `.board/research/code-review-standards.md` must still exist
#   and still name every baseline standard and every declared control.
#
# WHAT THIS DELIBERATELY DOES NOT PROVE: conformance to NIST, ISO, IEEE, OASIS,
# OWASP, Google or GitHub. It checks that a citation is present, allowlisted,
# versioned and dated. Nothing is fetched: no network call is made.
#
# READ SURFACE. The card declares `read_paths: []` and lists `.board/cards` in
# `forbidden_paths`, so the card itself must not be read at run time and the
# declared control list below is a deliberate mirror of the card front matter
# that has to be updated with it. At run time this program reads only
# `standards/code-review/` plus one ephemeral self-test tree it creates and
# removes under the system temporary directory.
#
# REQUIRED CATALOG FORMAT — `standards/code-review/cases.yaml`, a strict YAML
# subset: block style only, exact indentation, no tabs, no trailing spaces,
# `#` comments allowed, values optionally double-quoted.
#
#   rules:
#     - id: CR-EVD-001
#       source: NIST SP 800-218
#       url: https://csrc.nist.gov/pubs/sp/800/218/final
#       version: SSDF 1.1
#       date: 2022-02-03
#       status: current
#       cases:
#         - kind: positive
#           fixture: standards/code-review/fixtures/CR-EVD-001-positive.md
#           assert: what a conforming change looks like
#           expect: pass
#         - kind: negative
#           fixture: standards/code-review/fixtures/CR-EVD-001-negative.md
#           assert: what the violation looks like
#           expect: fail
#
# Rules beyond the declared control set are allowed and are validated by the
# same schema; they simply do not add coverage.
#
# EXIT CODES are disjoint on purpose:
#   0  = the promised property holds
#   1  = behavioral RED: the catalog is absent, incomplete or malformed
#   3  = harness or environment error, which is never valid red evidence
#   64 = unknown scenario selector
#
# The validator is self-tested against synthetic well-formed and mutated
# catalogs before any pass is claimed, so a green result cannot come from a
# detector that stopped detecting.

set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-014'
readonly scenario='AC-001'
readonly rc_red=1
readonly rc_infra=3
readonly max_reported=40
readonly max_catalog_bytes=262144

selector="${1:-AC-001}"
case "$selector" in
  AC-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

# Mirrors `controls` of .board/cards/*/AUR-014.md, in card order.
readonly -a declared_controls=(
  'CR-EVD-001'
  'CR-EVD-002'
  'CR-EVD-003'
  'CR-REV-001'
  'CR-REV-002'
  'CR-REV-003'
  'CR-REV-004'
  'CR-REV-005'
  'CR-GATE-001'
)

# Primary-source hosts of the researched standards baseline. A URL outside this
# set is a finding, not a warning.
readonly -a allowed_hosts=(
  'csrc.nist.gov'
  'docs.github.com'
  'docs.oasis-open.org'
  'google.github.io'
  'owasp.org'
  'standards.ieee.org'
  'www.iso.org'
  'www.rfc-editor.org'
)

# Researched baseline the catalog is derived from. Retained from the previous
# revision of this program: the research note must keep naming every standard it
# claims to be built on, and every declared control, or the catalog is citing a
# document that no longer says what it cites.
readonly -a required_sources=(
  'NIST SP 800-218'
  'Google Engineering Practices'
  'ISO/IEC 20246:2017'
  'ISO/IEC 25010:2023'
  'IEEE 1028-2008'
)

readonly standard_dir='standards/code-review'
readonly catalog_rel="${standard_dir}/cases.yaml"
readonly fixture_prefix="${standard_dir}/"
readonly research_rel='.board/research/code-review-standards.md'

selftest_dir=''

infra() {
  printf '%s/%s/infrastructure: %s\n' "$card" "$scenario" "$1" >&2
  exit "$rc_infra"
}

cleanup() {
  if [[ -n $selftest_dir && -d $selftest_dir ]]; then
    rm -rf -- "$selftest_dir"
  fi
  return 0
}
trap cleanup EXIT

for tool in awk cmp grep mktemp wc; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing tool: $tool"
done

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." 2>/dev/null && pwd -P)" ||
  infra 'repo root unresolved'
readonly repo_root

read -r -d '' parser_awk <<'AWK' || true
BEGIN { us = sprintf("%c", 31) }
function bad(ln, msg) { printf "E%s%d%s%s\n", us, ln, us, msg }
function emit_case() {
  if (!case_open) return
  printf "CASE%s%d%s%s%s%s%s%s%s%s%s%s\n", us, case_line, us, rule_id, us, cf["kind"], us, cf["expect"], us, cf["fixture"], us, cf["assert"]
  case_open = 0
  delete cf
}
function emit_rule() {
  emit_case()
  if (!rule_open) return
  printf "RULE%s%d%s%s%s%s%s%s%s%s%s%s%s%s\n", us, rule_line, us, rule_id, us, rf["source"], us, rf["url"], us, rf["version"], us, rf["date"], us, rf["status"]
  rule_open = 0
  in_cases = 0
  delete rf
}
function kv(s, out,   p, v) {
  p = index(s, ":")
  if (p < 2) return 0
  out["k"] = substr(s, 1, p - 1)
  v = substr(s, p + 1)
  sub(/^ /, "", v)
  if (length(v) >= 2 && v ~ /^".*"$/) v = substr(v, 2, length(v) - 2)
  out["v"] = v
  return (out["k"] ~ /^[a-z]+$/)
}
{
  line = $0
  sub(/\r$/, "", line)
  if (index(line, "\t") > 0) { bad(NR, "tab-character"); next }
  if (line ~ /[[:cntrl:]]/) { bad(NR, "control-character"); next }
  if (line ~ /^ *$/) next
  if (line ~ /^ *#/) next
  if (line ~ / $/) { bad(NR, "trailing-space"); next }
  n = 0
  while (substr(line, n + 1, 1) == " ") n++
  body = substr(line, n + 1)
  if (!started) {
    if (n == 0 && body == "rules:") { started = 1; next }
    bad(NR, "expected-header-rules")
    next
  }
  if (n == 2 && substr(body, 1, 2) == "- ") {
    emit_rule()
    if (!kv(substr(body, 3), pair) || pair["k"] != "id") { bad(NR, "rule-must-start-with-id"); next }
    rule_id = pair["v"]
    if (rule_id !~ /^[A-Z]+(-[A-Z0-9]+)+$/) bad(NR, "bad-rule-id")
    rule_open = 1
    rule_line = NR
    in_cases = 0
    delete rf
    next
  }
  if (!rule_open) { bad(NR, "content-outside-rule"); next }
  if (n == 4) {
    if (body == "cases:") {
      emit_case()
      if (in_cases) bad(NR, "duplicate-key-cases")
      in_cases = 1
      next
    }
    if (in_cases) { bad(NR, "rule-field-after-cases"); next }
    if (!kv(body, pair)) { bad(NR, "bad-rule-field"); next }
    if (pair["k"] !~ /^(source|url|version|date|status)$/) { bad(NR, "unknown-rule-key-" pair["k"]); next }
    if (pair["k"] in rf) { bad(NR, "duplicate-key-" pair["k"]); next }
    rf[pair["k"]] = pair["v"]
    next
  }
  if (n == 6 && substr(body, 1, 2) == "- ") {
    if (!in_cases) { bad(NR, "case-outside-cases"); next }
    emit_case()
    if (!kv(substr(body, 3), pair) || pair["k"] != "kind") { bad(NR, "case-must-start-with-kind"); next }
    delete cf
    cf["kind"] = pair["v"]
    case_open = 1
    case_line = NR
    next
  }
  if (n == 8) {
    if (!case_open) { bad(NR, "case-field-outside-case"); next }
    if (!kv(body, pair)) { bad(NR, "bad-case-field"); next }
    if (pair["k"] !~ /^(fixture|assert|expect)$/) { bad(NR, "unknown-case-key-" pair["k"]); next }
    if (pair["k"] in cf) { bad(NR, "duplicate-key-" pair["k"]); next }
    cf[pair["k"]] = pair["v"]
    next
  }
  bad(NR, "bad-indentation")
}
END {
  emit_rule()
  if (!started) printf "E%s0%scatalog-has-no-rules-header\n", us, us
}
AWK

finding_codes=()
finding_texts=()
rules_seen=0
cases_seen=0

add_finding() {
  finding_codes+=("$1")
  finding_texts+=("$1: $2")
}

host_allowed() {
  local host=$1 candidate
  for candidate in "${allowed_hosts[@]}"; do
    [[ $host == "$candidate" ]] && return 0
  done
  return 1
}

# validate <catalog-path> <fixture-root> <fixture-prefix> <control>...
# Resets and repopulates finding_codes / finding_texts. Never prints file
# content: a finding names the rule, the line and the defect class only.
validate() {
  local catalog=$1 root=$2 prefix=$3
  shift 3
  local -a controls=("$@")

  finding_codes=()
  finding_texts=()
  rules_seen=0
  cases_seen=0

  local -A rule_line=() rule_pos=() rule_neg=() pos_fix=() neg_fix=() fix_owner=()
  local parsed rec a b c d e f g size ctrl ok rest host rid src readable=1
  local research=$root/$research_rel

  # Retained coverage: the research note backing the catalog must still exist
  # and still name the baseline standards and every declared control.
  if [[ ! -f $research || -L $research ]]; then
    add_finding 'missing-research' "$research_rel is not a regular file"
  elif [[ ! -s $research ]]; then
    add_finding 'missing-research' "$research_rel is empty"
  else
    [[ -r $research ]] || infra "research note unreadable: $research_rel"
    for src in "${required_sources[@]}"; do
      grep -Fq -- "$src" "$research" ||
        add_finding 'research-missing-source' "$research_rel never names $src"
    done
    for ctrl in "${controls[@]}"; do
      grep -Fq -- "$ctrl" "$research" ||
        add_finding 'research-missing-control' "$research_rel never names $ctrl"
    done
  fi

  if [[ ! -d $root/$standard_dir ]]; then
    add_finding 'missing-standard-dir' "$standard_dir/ does not exist"
    readable=0
  elif [[ ! -f $catalog || -L $catalog ]]; then
    add_finding 'missing-catalog' "$catalog_rel is not a regular file"
    readable=0
  elif [[ ! -s $catalog ]]; then
    [[ -r $catalog ]] || infra "catalog unreadable: $catalog_rel"
    add_finding 'empty-catalog' "$catalog_rel is empty"
    readable=0
  else
    [[ -r $catalog ]] || infra "catalog unreadable: $catalog_rel"
    size="$(wc -c <"$catalog")" || infra 'catalog size unreadable'
    size=${size//[^0-9]/}
    if ((size > max_catalog_bytes)); then
      add_finding 'catalog-unbounded' "$catalog_rel exceeds $max_catalog_bytes bytes"
      readable=0
    fi
  fi

  # An unreadable catalog still owes every declared control a finding, so the
  # red signal names the uncovered controls instead of stopping at the first
  # structural defect.
  if ((readable == 0)); then
    for ctrl in "${controls[@]}"; do
      add_finding 'missing-rule-case' "$ctrl has no entry in $catalog_rel"
    done
    return 0
  fi

  # The operand is always an absolute path built above, never an option-like word.
  parsed="$(awk "$parser_awk" "$catalog")" || infra 'catalog parser failed to run'

  if [[ -n $parsed ]]; then
    while IFS=$'\037' read -r rec a b c d e f g; do
      case $rec in
        E)
          add_finding 'catalog-syntax' "$catalog_rel line $a: $b"
          ;;
        RULE)
          rules_seen=$((rules_seen + 1))
          if [[ -n ${rule_line[$b]:-} ]]; then
            add_finding 'duplicate-rule' "$b declared twice, lines ${rule_line[$b]} and $a"
            continue
          fi
          rule_line[$b]=$a
          [[ -n $c ]] || add_finding 'missing-source' "$b names no primary source"
          [[ -n $e ]] || add_finding 'missing-version' "$b declares no source version"
          if [[ -z $d ]]; then
            add_finding 'missing-source-url' "$b declares no primary source URL"
          elif [[ $d != https://* ]]; then
            add_finding 'source-url-not-allowlisted' "$b source URL is not https"
          else
            rest=${d#https://}
            host=${rest%%/*}
            host_allowed "$host" ||
              add_finding 'source-url-not-allowlisted' "$b cites host $host"
          fi
          if [[ ! $f =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
            add_finding 'missing-source-date' "$b declares no ISO source date"
          fi
          if [[ -z $g ]]; then
            add_finding 'missing-status' "$b declares no source status"
          elif [[ $g != current ]]; then
            add_finding 'withdrawn-source' "$b source status is '$g', not current"
          fi
          ;;
        CASE)
          cases_seen=$((cases_seen + 1))
          ok=1
          if [[ $c != positive && $c != negative ]]; then
            add_finding 'bad-case-kind' "$b line $a: kind '$c' is not positive or negative"
            ok=0
          fi
          if [[ $d != pass && $d != fail ]]; then
            add_finding 'bad-verdict' "$b line $a: expect '$d' is not pass or fail"
            ok=0
          elif [[ ($c == positive && $d != pass) || ($c == negative && $d != fail) ]]; then
            add_finding 'verdict-mismatch' "$b line $a: $c case declares expect '$d'"
            ok=0
          fi
          if [[ -z $f ]]; then
            add_finding 'missing-assert' "$b line $a: case declares no assert"
            ok=0
          fi
          if [[ -z $e ]]; then
            add_finding 'missing-fixture' "$b line $a: case declares no fixture"
            ok=0
          elif [[ $e != "$prefix"* || $e == *..* ]]; then
            add_finding 'fixture-outside-standard' "$b line $a: fixture is not under $prefix"
            ok=0
          elif [[ ! -f $root/$e || -L $root/$e ]]; then
            add_finding 'missing-fixture' "$b line $a: fixture $e is not a regular file"
            ok=0
          elif [[ ! -s $root/$e ]]; then
            add_finding 'missing-fixture' "$b line $a: fixture $e is empty"
            ok=0
          else
            [[ -r $root/$e ]] || infra "fixture unreadable: $e"
            if ! grep -Fq -- "$b" "$root/$e"; then
              add_finding 'fixture-not-bound-to-rule' "$b line $a: fixture $e never names $b"
              ok=0
            fi
          fi
          # A fixture that serves two cases distinguishes neither of them: one
          # file naming every rule ID would otherwise satisfy the whole catalog.
          if ((ok == 1)) && [[ -n ${fix_owner[$e]:-} ]]; then
            add_finding 'fixture-reused' \
              "$b line $a: fixture $e already serves ${fix_owner[$e]}"
            ok=0
          fi
          if ((ok == 1)); then
            fix_owner[$e]="$b/$c"
            if [[ $c == positive ]]; then
              rule_pos[$b]=1
              [[ -n ${pos_fix[$b]:-} ]] || pos_fix[$b]=$e
            else
              rule_neg[$b]=1
              [[ -n ${neg_fix[$b]:-} ]] || neg_fix[$b]=$e
            fi
          fi
          ;;
        *)
          infra "unrecognised parser record: $rec"
          ;;
      esac
    done <<<"$parsed"
  fi

  if ((rules_seen == 0)) && ((${#finding_codes[@]} == 0)); then
    add_finding 'empty-catalog' "$catalog_rel declares no rule"
  fi

  # Two byte-identical files cannot be a positive and a negative example.
  if ((${#pos_fix[@]} > 0)); then
    while IFS= read -r rid; do
      [[ -n ${neg_fix[$rid]:-} ]] || continue
      if cmp -s "$root/${pos_fix[$rid]}" "$root/${neg_fix[$rid]}"; then
        add_finding 'fixture-not-distinct' \
          "$rid positive and negative fixtures are byte-identical"
        unset 'rule_pos[$rid]'
        unset 'rule_neg[$rid]'
      fi
    done < <(printf '%s\n' "${!pos_fix[@]}" | LC_ALL=C sort)
  fi

  for ctrl in "${controls[@]}"; do
    if [[ -z ${rule_line[$ctrl]:-} ]]; then
      add_finding 'missing-rule-case' "$ctrl has no entry in $catalog_rel"
      continue
    fi
    [[ -n ${rule_pos[$ctrl]:-} ]] ||
      add_finding 'missing-positive-case' "$ctrl has no valid positive case expecting pass"
    [[ -n ${rule_neg[$ctrl]:-} ]] ||
      add_finding 'missing-negative-case' "$ctrl has no valid negative case expecting fail"
  done
  return 0
}

has_finding() {
  local want=$1 code
  for code in "${finding_codes[@]}"; do
    [[ $code == "$want" ]] && return 0
  done
  return 1
}

write_selftest_catalog() {
  local dst=$1 mode=$2
  local source_line='    source: NIST SP 800-218'
  local url_line='    url: https://csrc.nist.gov/pubs/sp/800/218/final'
  local status_line='    status: current'
  local negative_expect='        expect: fail'
  local negative_fixture='fixtures/ST-SELF-001-negative.txt'
  case $mode in
    good) ;;
    drop-source) source_line='' ;;
    drop-url) url_line='' ;;
    bad-host) url_line='    url: https://evil.example/pubs/sp/800/218/final' ;;
    withdrawn) status_line='    status: withdrawn' ;;
    inverted) negative_expect='        expect: pass' ;;
    shared-fixture) negative_fixture='fixtures/ST-SELF-001-positive.txt' ;;
    *) infra "unknown self-test mode: $mode" ;;
  esac
  {
    printf 'rules:\n'
    printf '  - id: ST-SELF-001\n'
    [[ -n $source_line ]] && printf '%s\n' "$source_line"
    [[ -n $url_line ]] && printf '%s\n' "$url_line"
    printf '    version: SSDF 1.1\n'
    printf '    date: 2022-02-03\n'
    printf '%s\n' "$status_line"
    printf '    cases:\n'
    printf '      - kind: positive\n'
    printf '        fixture: %sfixtures/ST-SELF-001-positive.txt\n' "$fixture_prefix"
    printf '        assert: conforming change\n'
    printf '        expect: pass\n'
    printf '      - kind: negative\n'
    printf '        fixture: %s%s\n' "$fixture_prefix" "$negative_fixture"
    printf '        assert: violating change\n'
    printf '%s\n' "$negative_expect"
  } >"$dst" || infra 'self-test catalog unwritable'
}

write_selftest_research() {
  local dst=$1 mode=$2 src
  {
    printf '# self-test research note\n'
    for src in "${required_sources[@]}"; do
      [[ $mode == drop-source-name && $src == 'IEEE 1028-2008' ]] && continue
      printf '%s\n' "$src"
    done
    printf 'ST-SELF-001\n'
  } >"$dst" || infra 'self-test research note unwritable'
}

selftest() {
  local root cat_path pair want

  selftest_dir="$(mktemp -d "${TMPDIR:-/tmp}/aur014.XXXXXX")" ||
    infra 'self-test directory uncreatable'
  root=$selftest_dir
  cat_path="$root/$catalog_rel"
  mkdir -p "$root/$standard_dir/fixtures" || infra 'self-test tree uncreatable'
  mkdir -p "$root/$(dirname "$research_rel")" || infra 'self-test tree uncreatable'
  write_selftest_research "$root/$research_rel" good
  printf 'ST-SELF-001 conforming example\n' \
    >"$root/$standard_dir/fixtures/ST-SELF-001-positive.txt" || infra 'self-test fixture unwritable'
  printf 'ST-SELF-001 violating example\n' \
    >"$root/$standard_dir/fixtures/ST-SELF-001-negative.txt" || infra 'self-test fixture unwritable'

  write_selftest_catalog "$cat_path" good
  validate "$cat_path" "$root" "$fixture_prefix" 'ST-SELF-001'
  ((${#finding_codes[@]} == 0)) ||
    infra "detector reports ${finding_codes[*]} on a well-formed catalog"

  # Covers every mutation dimension the card declares, MUT-001 (source removed)
  # and MUT-002 (source URL off the allowlist) included: a detector that stopped
  # detecting any one of them must not be able to report a pass.
  for pair in 'drop-source:missing-source' 'drop-url:missing-source-url' \
    'bad-host:source-url-not-allowlisted' 'withdrawn:withdrawn-source' \
    'inverted:verdict-mismatch' 'shared-fixture:fixture-reused'; do
    want=${pair#*:}
    write_selftest_catalog "$cat_path" "${pair%%:*}"
    validate "$cat_path" "$root" "$fixture_prefix" 'ST-SELF-001'
    has_finding "$want" || infra "detector missed $want under mutation ${pair%%:*}"
  done

  # Distinct paths whose bytes are identical are still one example, not two.
  write_selftest_catalog "$cat_path" good
  printf 'ST-SELF-001 conforming example\n' \
    >"$root/$standard_dir/fixtures/ST-SELF-001-negative.txt" ||
    infra 'self-test fixture unwritable'
  validate "$cat_path" "$root" "$fixture_prefix" 'ST-SELF-001'
  has_finding 'fixture-not-distinct' ||
    infra 'detector missed byte-identical positive and negative fixtures'
  has_finding 'missing-negative-case' ||
    infra 'detector credited coverage from an indistinct fixture pair'
  printf 'ST-SELF-001 violating example\n' \
    >"$root/$standard_dir/fixtures/ST-SELF-001-negative.txt" ||
    infra 'self-test fixture unwritable'

  # The retained research-note coverage must itself stay detectable.
  write_selftest_research "$root/$research_rel" drop-source-name
  validate "$cat_path" "$root" "$fixture_prefix" 'ST-SELF-001'
  has_finding 'research-missing-source' ||
    infra 'detector missed a baseline standard dropped from the research note'
  rm -f -- "$root/$research_rel"
  validate "$cat_path" "$root" "$fixture_prefix" 'ST-SELF-001'
  has_finding 'missing-research' ||
    infra 'detector missed an absent research note'
  write_selftest_research "$root/$research_rel" good

  write_selftest_catalog "$cat_path" good
  validate "$cat_path" "$root" "$fixture_prefix" 'ST-SELF-001' 'ST-SELF-002'
  has_finding 'missing-rule-case' ||
    infra 'detector missed missing-rule-case for an uncovered control'
  has_finding 'research-missing-control' ||
    infra 'detector missed a control absent from the research note'

  rm -f -- "$root/$standard_dir/fixtures/ST-SELF-001-negative.txt"
  validate "$cat_path" "$root" "$fixture_prefix" 'ST-SELF-001'
  has_finding 'missing-fixture' ||
    infra 'detector missed missing-fixture for an absent fixture'
  has_finding 'missing-negative-case' ||
    infra 'detector credited coverage from an invalid case'

  rm -rf -- "$selftest_dir"
  selftest_dir=''
}

selftest

validate "$repo_root/$catalog_rel" "$repo_root" "$fixture_prefix" "${declared_controls[@]}"

if ((${#finding_codes[@]} > 0)); then
  printf '%s/%s: %d finding(s) over %d declared control(s)\n' \
    "$card" "$scenario" "${#finding_codes[@]}" "${#declared_controls[@]}" >&2
  reported=0
  for text in "${finding_texts[@]}"; do
    if ((reported >= max_reported)); then
      printf '%s/%s: %d further finding(s) suppressed\n' \
        "$card" "$scenario" $((${#finding_texts[@]} - max_reported)) >&2
      break
    fi
    printf '%s/%s/%s\n' "$card" "$scenario" "$text" >&2
    reported=$((reported + 1))
  done
  exit "$rc_red"
fi

printf '{"card":"%s","scenario":"%s","result":"pass","controls":%d,"rules":%d,"cases":%d}\n' \
  "$card" "$scenario" "${#declared_controls[@]}" "$rules_seen" "$cases_seen"
