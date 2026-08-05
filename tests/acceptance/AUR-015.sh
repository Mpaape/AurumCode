#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-015'
readonly scenario='AC-001'

selector="${1:-AC-001}"
case "$selector" in
  AC-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

infra() {
  printf '%s/%s/infrastructure: %s\n' "$card" "$scenario" "$1" >&2
  exit 79
}
fail() {
  printf '%s/%s/%s: %s\n' "$card" "$scenario" "$1" "$2" >&2
  exit 1
}
emitted() {
  [[ -n $1 ]] || return 0
  fail "${1%%$'\t'*}" "${1#*$'\t'}"
}

for tool in awk grep sed sort wc; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing tool: $tool"
done

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." 2>/dev/null && pwd -P)" || infra 'repo root unresolved'
cd -- "$repo_root" || infra 'repo root unreachable'

readonly research='.board/research/secure-code-review.md'
readonly rules='standards/security-review/rules.md'
readonly specdir='tests/specs/AUR-015'
readonly ruleids_fixture="$specdir/rule-ids.txt"
readonly domains_fixture="$specdir/allowed-source-domains.txt"

# All four are this card's own declared deliverables (paths:); their
# absence is the behavior under test failing to exist, never infrastructure.
for f in "$research" "$rules" "$ruleids_fixture" "$domains_fixture"; do
  [[ -f $f && ! -L $f ]] || fail behavior-missing "required artifact absent: $f"
  [[ -r $f ]] || fail behavior-missing "required artifact unreadable: $f"
  [[ -s $f ]] || fail behavior-missing "required artifact is empty: $f"
done

# ---------------------------------------------------------------------------
# AC-001 baseline terms. This exact message form and exit code are pinned by
# the card's Given/When/Then: absence of any one of these four terms in the
# research baseline is the invalid case this scenario names directly.
# ---------------------------------------------------------------------------
for needle in 'OWASP' 'threat' 'trust boundary' 'fail closed'; do
  if ! grep -Fiq "$needle" "$research"; then
    printf '%s/%s: missing %s\n' "$card" "$scenario" "$needle" >&2
    exit 1
  fi
done

# ---------------------------------------------------------------------------
# Required rule IDs and allowed source hosts: single source of truth is
# tests/specs/AUR-015, read once here. Anti-shortcut: the embedded list is
# asserted to have exactly the expected count before anything consumes it,
# so a silently emptied fixture cannot report a false pass having checked
# nothing (tests/acceptance/EXIT_CODE_CONVENTION.md).
# ---------------------------------------------------------------------------
mapfile -t required_rules < <(grep -Ev '^[[:space:]]*$' "$ruleids_fixture")
(( ${#required_rules[@]} == 9 )) ||
  fail behavior-missing "rule-ids fixture declares ${#required_rules[@]} rule IDs, want exactly 9"
mapfile -t allowed_domains < <(grep -Ev '^[[:space:]]*$' "$domains_fixture")
(( ${#allowed_domains[@]} >= 1 )) ||
  fail behavior-missing 'allowed-source-domains fixture is empty'

AURUM_REQUIRED_RULES="$(printf '%s\n' "${required_rules[@]}")"
AURUM_ALLOWED_DOMAINS="$(printf '%s\n' "${allowed_domains[@]}")"
export AURUM_REQUIRED_RULES AURUM_ALLOWED_DOMAINS

# This program's own fixed expectation of full control coverage: the nine
# controls the card's `controls:` frontmatter declares. Not read from the
# card file — `.board/cards` is outside this card's owned paths.
export AURUM_EXPECTED_CONTROLS='CR-EVD-001
CR-EVD-002
CR-EVD-003
CR-REV-001
CR-REV-002
CR-REV-003
CR-REV-004
CR-REV-005
CR-GATE-001'

# ---------------------------------------------------------------------------
# Parse every "### Rule: <ID> — <title>" block in the "## Rules" section into
# one tab-separated record: id, owasp url/version/date, cwe number/url/
# version/date, ssdf url/version/date, control, trust-boundary present
# (1/0), vulnerable fixture path, fixed fixture path.
# ---------------------------------------------------------------------------
AURUM_RULES_TEXT="$(cat "$rules")"
export AURUM_RULES_TEXT
records="$(
  awk '
    function flushrec() {
      if (id == "") return
      print id "\t" owasp_url "\t" owasp_ver "\t" owasp_date "\t" \
            cwe_num "\t" cwe_url "\t" cwe_ver "\t" cwe_date "\t" \
            ssdf_url "\t" ssdf_ver "\t" ssdf_date "\t" control "\t" \
            (trust_boundary != "" ? 1 : 0) "\t" vuln_fixture "\t" fixed_fixture
      id = ""; owasp_url = ""; owasp_ver = ""; owasp_date = ""
      cwe_num = ""; cwe_url = ""; cwe_ver = ""; cwe_date = ""
      ssdf_url = ""; ssdf_ver = ""; ssdf_date = ""; control = ""
      trust_boundary = ""; vuln_fixture = ""; fixed_fixture = ""
    }
    function urlof(labelpart,   s, o, c) {
      s = labelpart
      o = index(s, "](")
      if (o == 0) return ""
      c = length(s)
      while (c > o + 1 && substr(s, c, 1) != ")") c--
      if (c <= o + 1) return ""
      return substr(s, o + 2, c - o - 2)
    }
    function verof(verpart,   s) {
      s = verpart
      sub(/^version /, "", s)
      return s
    }
    BEGIN { in_section = 0; n = 3 }
    /^## Rules[[:space:]]*$/ { in_section = 1; next }
    in_section && /^## / { in_section = 0 }
    !in_section { next }
    /^### Rule: / {
      flushrec()
      line = $0
      sub(/^### Rule: /, "", line)
      d = index(line, " \342\200\224 ")
      id = (d > 0) ? substr(line, 1, d - 1) : line
      next
    }
    id != "" && /^- OWASP: / {
      rest = $0; sub(/^- OWASP: /, "", rest)
      split(rest, fld, / \| /)
      owasp_url = urlof(fld[1]); owasp_ver = verof(fld[2]); owasp_date = fld[3]
      next
    }
    id != "" && /^- CWE: / {
      rest = $0; sub(/^- CWE: /, "", rest)
      split(rest, fld, / \| /)
      cwe_url = urlof(fld[1]); cwe_ver = verof(fld[2]); cwe_date = fld[3]
      if (match(fld[1], /CWE-[0-9]+/)) cwe_num = substr(fld[1], RSTART + 4, RLENGTH - 4)
      next
    }
    id != "" && /^- NIST SSDF: / {
      rest = $0; sub(/^- NIST SSDF: /, "", rest)
      split(rest, fld, / \| /)
      ssdf_url = urlof(fld[1]); ssdf_ver = verof(fld[2]); ssdf_date = fld[3]
      next
    }
    id != "" && /^- Control: / {
      control = $0; sub(/^- Control: /, "", control)
      next
    }
    id != "" && /^- Trust boundary: / {
      trust_boundary = $0; sub(/^- Trust boundary: /, "", trust_boundary)
      next
    }
    id != "" && /^- Vulnerable fixture: / {
      vuln_fixture = $0; sub(/^- Vulnerable fixture: /, "", vuln_fixture)
      next
    }
    id != "" && /^- Fixed fixture: / {
      fixed_fixture = $0; sub(/^- Fixed fixture: /, "", fixed_fixture)
      next
    }
    END { flushrec() }
  ' <<< "$AURUM_RULES_TEXT"
)" || infra 'rules.md parse failed to run'

[[ -n $records ]] || fail undecided-rule "no ## Rules section with any ### Rule: block found in $rules"

record_count="$(wc -l <<< "$records")"
(( record_count == 9 )) || fail undecided-rule "rules.md declares $record_count rule blocks, want exactly 9"

export AURUM_RECORDS="$records"
verdict="$(
  awk -F'\t' '
    function emit(code, detail) { print code "\t" detail; done = 1; exit 0 }
    function trim(s) { gsub(/^[[:space:]]+|[[:space:]]+$/, "", s); return s }
    function checkurl(id, label, url,   host) {
      if (url == "") emit("undecided-rule", id ": " label " has no [label](url) link")
      if (match(url, /^https?:\/\/[^\/]+/) == 0)
        emit("undecided-rule", id ": " label " url is not http(s): " url)
      host = substr(url, RSTART, RLENGTH)
      sub(/^https?:\/\//, "", host)
      if (!(host in allowed))
        emit("undecided-rule", id ": " label " source host is not allowlisted: " host)
    }
    function checkver(id, label, ver) {
      if (ver == "" || ver !~ /^[0-9]+(\.[0-9]+){0,2}$/)
        emit("undecided-rule", id ": " label " has no versioned source: " ver)
    }
    function checkdate(id, label, dt) {
      if (dt !~ /^[0-9]{4}-[0-9]{2}-[0-9]{2}$/)
        emit("undecided-rule", id ": " label " source date is not ISO 8601: " dt)
    }
    BEGIN {
      req_n = split(ENVIRON["AURUM_REQUIRED_RULES"], req, "\n")
      for (i = 1; i <= req_n; i++) if (req[i] != "") want[req[i]] = 1
      dom_n = split(ENVIRON["AURUM_ALLOWED_DOMAINS"], doms, "\n")
      for (i = 1; i <= dom_n; i++) if (doms[i] != "") allowed[doms[i]] = 1
      ctl_n = split(ENVIRON["AURUM_EXPECTED_CONTROLS"], ctls, "\n")
      for (i = 1; i <= ctl_n; i++) if (ctls[i] != "") expected_ctl[ctls[i]] = 1

      n = split(ENVIRON["AURUM_RECORDS"], lines, "\n")
      for (li = 1; li <= n; li++) {
        line = lines[li]
        if (line == "") continue
        m = split(line, c, "\t")
        if (m != 15) emit("undecided-rule", "rule record has " m " fields, want 15: " line)
        id = c[1]
        if (!(id in want)) emit("undecided-rule", "rule id not in tests/specs/AUR-015/rule-ids.txt: " id)
        if (id in seen) emit("undecided-rule", "rule id cited twice: " id)
        seen[id] = 1

        checkurl(id, "OWASP", c[2])
        checkver(id, "OWASP", c[3])
        checkdate(id, "OWASP", c[4])

        if (c[5] !~ /^[0-9]+$/) emit("undecided-rule", id ": CWE has no numeric CWE-<n> identifier")
        checkurl(id, "CWE", c[6])
        checkver(id, "CWE", c[7])
        checkdate(id, "CWE", c[8])

        checkurl(id, "NIST SSDF", c[9])
        checkver(id, "NIST SSDF", c[10])
        checkdate(id, "NIST SSDF", c[11])

        control = c[12]
        if (control !~ /^CR-(EVD|REV|GATE)-[0-9]{3}$/)
          emit("undecided-rule", id ": control is missing or not a CR-<FAMILY>-<NNN> id: " control)
        if (control in used_control)
          emit("undecided-rule", id ": control already bound to another rule: " control)
        used_control[control] = 1

        if (c[13] != "1") emit("undecided-rule", id ": has no Trust boundary statement")

        vuln = c[14]; fixedf = c[15]
        if (vuln == "" || fixedf == "")
          emit("undecided-rule", id ": missing vulnerable or fixed fixture path")
        if (vuln !~ ("^tests/specs/AUR-015/" id "/") || fixedf !~ ("^tests/specs/AUR-015/" id "/"))
          emit("undecided-rule", id ": fixture path is outside its own tests/specs/AUR-015/" id "/ directory")
      }
    }
    END {
      if (done) exit 0
      for (a in want) if (!(a in seen))
        emit("undecided-rule", "required rule missing from rules.md: " a)
      for (ectl in expected_ctl) if (!(ectl in used_control))
        emit("undecided-rule", "expected control has no bound rule: " ectl)
    }
  ' <<< "$records"
)" || infra 'rules.md field lint failed to run'
emitted "$verdict"

# ---------------------------------------------------------------------------
# Fixture pairs: every vulnerable/fixed file this card's own rules.md names
# must exist, be non-empty, carry the matching marker, and differ from each
# other — swapped, identical, or empty fixtures fail here, not in a later
# review.
# ---------------------------------------------------------------------------
verdict=""
while IFS=$'\t' read -r id owasp_url owasp_ver owasp_date cwe_num cwe_url cwe_ver cwe_date \
    ssdf_url ssdf_ver ssdf_date control trust vuln_fixture fixed_fixture; do
  [[ -n $id ]] || continue
  for pair in "$vuln_fixture:VULNERABLE" "$fixed_fixture:FIXED"; do
    fx="${pair%%:*}"; marker="${pair##*:}"
    if [[ ! -f $fx || -L $fx ]]; then
      verdict="undecided-rule"$'\t'"$id: fixture absent: $fx"
      break 2
    fi
    if [[ ! -s $fx ]]; then
      verdict="undecided-rule"$'\t'"$id: fixture is empty: $fx"
      break 2
    fi
    if ! grep -Fq "Fixture: $marker" "$fx"; then
      verdict="undecided-rule"$'\t'"$id: fixture is missing its 'Fixture: $marker' marker: $fx"
      break 2
    fi
  done
  if [[ -n $vuln_fixture && -n $fixed_fixture ]] && cmp -s -- "$vuln_fixture" "$fixed_fixture"; then
    verdict="undecided-rule"$'\t'"$id: vulnerable and fixed fixtures are byte-identical: $vuln_fixture"
    break
  fi
done <<< "$records"
emitted "$verdict"

printf '{"card":"%s","scenario":"%s","selector":"%s","rules":%d,"result":"pass"}\n' \
  "$card" "$scenario" "$selector" "$record_count"
