# Secure code review: standard selection and crosswalk baseline

Status: normative project baseline (design input, not implementation evidence)
Scope: what a reviewer or an automated review agent MUST check for
application-security defects during code review, independent of language,
SCM, or provider. This document does not claim or imply certification,
validation, or conformance by NIST, MITRE, or OWASP.

## Problem

`.board/research/code-review-standards.md` fixes the process contract for
*how* AurumCode reviews a change (double-blind coverage, evidence levels,
gates). It says nothing about *which application-security weaknesses* a
review must catch. Without a versioned, dated crosswalk from a named
weakness catalog to this project's own testable controls, "security review"
degenerates into an unstructured, non-reproducible checklist that a
reviewer — human or model — can silently narrow without anyone noticing the
narrowing. This card closes that gap: it selects the frameworks this project
adopts and maps every adopted rule to one existing `CR-*` control from
`code-review-standards.md`, one CWE identifier, and one NIST SSDF practice,
each cited with a version and an ISO-8601 date.

## Candidate frameworks matrix

Six candidate weakness/practice catalogs were compared. `Machine-testable
unit` is whether the catalog exposes an identifier fine-grained enough to
bind one review rule to one weakness (a prerequisite for `standards/security-review`,
which must name exactly one CWE per rule). `Open access` is whether the
normative text is readable without a paid membership or license purchase,
which this project requires because every cited source must be
independently re-derivable by any future reviewer, sealed or not.

| Framework | Source | Version | Date | Scope | Machine-testable unit | Open access | Decision |
|---|---|---|---|---|---|---|---|
| OWASP Top 10 | [owasp.org/Top10/2021](https://owasp.org/Top10/2021/) | 2021 | 2021-09-24 | Ten ranked web-application risk categories, each with example CWEs, attack scenarios, and prevention guidance | Category (`A01`–`A10`), each pre-mapped to a CWE set | Yes, CC BY-SA, no membership | Adopted — threat-oriented review prompt per rule |
| CWE (Common Weakness Enumeration) | [cwe.mitre.org/data](https://cwe.mitre.org/data/index.html) | List version 4.20 | 2024-11-19 | 944 catalogued software/hardware weakness types with a stable numeric ID, description, and consequence | Single weakness ID (`CWE-<n>`) | Yes, free, MITRE-maintained | Adopted — precise weakness typing per rule |
| NIST SP 800-218 SSDF | [csrc.nist.gov/pubs/sp/800/218/final](https://csrc.nist.gov/pubs/sp/800/218/final) | Version 1.1 | 2022-02-04 | Four practice groups (`PO`, `PS`, `PW`, `RV`) covering the software development lifecycle, including human-readable code review (`PW.7`) and executable testing (`PW.8`) | Named practice (`PW.7`, `RV.1`, ...) | Yes, US government publication, no license | Adopted — binds each rule to a lifecycle practice, not only a code pattern |
| ISO/IEC 27034 | [iso.org/standard/44378](https://www.iso.org/standard/44378.html) | Part 1, 2011 (confirmed current edition) | 2011 | Application-security process framework across the organization, project, and application levels | Process-level control, not a per-weakness identifier | No — full text is a paid purchase | Rejected — a process framework the project cannot cite verbatim without purchase, and it names no per-weakness identifier a `standards/security-review` rule could bind to |
| SANS/CWE Top 25 | [cwe.mitre.org/top25](https://cwe.mitre.org/top25/archive/2023/2023_top25_list.html) | 2023 edition | 2023-06-29 | 25 most dangerous software weaknesses, ranked by exploitability and prevalence | Single weakness ID, already a subset of the full CWE list | Yes, free | Rejected as a separate source — every identifier it carries is already reachable through the adopted CWE list above; adding it as a second source would double-cite the same weaknesses under two different version/date pairs for no additional coverage |
| BSIMM (Building Security In Maturity Model) | [bsimm.com](https://www.bsimm.com/) | BSIMM11+ | Rolling, membership-gated | Organizational security-practice maturity benchmark across participating firms | Practice activity, not a per-weakness identifier, and the underlying measurement data is participant-confidential | No — current data requires program membership | Rejected — maturity benchmarking, not a weakness catalog; no per-rule identifier and the source data cannot be independently re-derived by a future reviewer without membership |

## Decision

This project adopts exactly three sources for secure code review, each
cited with the version and date fixed in the matrix above:

- **OWASP Top 10** supplies the *threat* framing a reviewer starts from —
  which class of attacker-reachable behavior a rule defends against, and at
  which **trust boundary** the untrusted input crosses into a boundary that
  must enforce it (for example, external HTTP input crossing into a SQL
  execution boundary is `A03:2021 Injection`).
- **CWE** supplies the precise, machine-citable weakness identifier bound
  to each rule, so a finding names a specific, well-known defect type
  instead of a paraphrase.
- **NIST SSDF** supplies the lifecycle practice a rule operationalizes —
  most rules here bind to `PW.7` (review human-readable code) or `PW.8`
  (test executable code), and the response-side rules bind to the `RV`
  group.

Every rule that carries this crosswalk also names one existing `CR-*`
control from `code-review-standards.md` (`CR-EVD-001`–`003`, `CR-REV-001`–
`005`, `CR-GATE-001`) as the testable enforcement point: the control is what
actually **fails closed** when the rule is violated and a review or gate
runs, not a restatement of the weakness itself. The full nine-rule crosswalk
— one rule per one of those nine controls — lives in
[`standards/security-review/rules.md`](../../standards/security-review/rules.md),
each rule carrying its own vulnerable and fixed fixture pair under
`tests/specs/AUR-015/`.

## Definitions used throughout this baseline and `standards/security-review`

- **Threat**: an attacker-controlled input or capability that can reach a
  security-relevant operation (a query, a file path, a deserialization
  call, an authorization check) without first being validated, encoded, or
  authorized at the boundary that operation depends on.
- **Trust boundary**: the point in a data or control flow where data
  crosses from a context this project does not control (network input, a
  third-party dependency, a repository file, model or tool output) into a
  context that grants it new authority (a query, a shell command, a
  filesystem path, a deserializer, an authorization decision). Every rule
  in `standards/security-review/rules.md` names the trust boundary its
  vulnerable fixture crosses without a check.
- **Fail closed**: when a security-relevant decision cannot be made with
  confidence (a missing authorization check, an unparseable input, a review
  gate whose upstream step did not run), the system MUST deny, block, or
  reject rather than default to allow. `CR-GATE-001` states this
  project-wide; every rule below states it again at the specific weakness
  it defends against.

## Non-goals

This document and `standards/security-review/rules.md` are normative
mappings, not a vulnerability scan of AurumCode's own source. Running a
scanner against this repository's code using the crosswalk fixed here
belongs to the security office's dependent cards, not to this one. This
card also does not add, replace, or supersede any control ID in
`code-review-standards.md`; it only cites the nine that already exist.

## Sources not queried live

No network request was made while producing this document or the
acceptance program that checks it; every citation above was resolved ahead
of time to a versioned, dated, host-allowlisted reference recorded in
`tests/specs/AUR-015/allowed-source-domains.txt`, and the acceptance program
itself runs with no network access, consistent with `CR-TRUST-002`.
