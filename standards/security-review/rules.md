# Secure code review rule catalog

Status: normative project baseline, enforced by `tests/acceptance/AUR-015.sh`.
Source selection and definitions (threat, trust boundary, fail closed) are
fixed in [`.board/research/secure-code-review.md`](../../.board/research/secure-code-review.md).

Every rule below binds exactly one OWASP Top 10 category, one CWE
identifier, and one NIST SSDF practice — each cited with the source's
version and an ISO-8601 date — to exactly one existing `CR-*` control from
[`.board/research/code-review-standards.md`](../../.board/research/code-review-standards.md).
The control is what fails closed when the rule is violated; the OWASP/CWE/
SSDF citations are what make the rule's scope precise and independently
re-derivable. Every rule also carries a vulnerable and a fixed fixture under
`tests/specs/AUR-015/<rule-id>/`, and a one-line trust-boundary statement
naming the untrusted-to-trusted crossing the rule defends.

Required rule set: `tests/specs/AUR-015/rule-ids.txt` is the single source
of truth for which nine rule IDs this file must resolve, in the same order
the card's own `controls:` list declares (`CR-EVD-001`–`003`, `CR-REV-001`–
`005`, `CR-GATE-001`); `tests/specs/AUR-015/allowed-source-domains.txt` is
the single source of truth for which source hosts an `OWASP`/`CWE`/`NIST
SSDF` link may cite.

## Field format

Each rule is a level-3 heading `### Rule: <ID> — <title>` followed by
exactly these fields, one per line, in this order:

```
- OWASP: [label](url) | version <v> | <YYYY-MM-DD>
- CWE: [label](url) | version <v> | <YYYY-MM-DD>
- NIST SSDF: [label](url) | version <v> | <YYYY-MM-DD>
- Control: CR-<FAMILY>-<NNN>
- Trust boundary: <one sentence naming the untrusted-to-trusted crossing>
- Vulnerable fixture: tests/specs/AUR-015/<ID>/vulnerable.md
- Fixed fixture: tests/specs/AUR-015/<ID>/fixed.md
```

## Rules

### Rule: SCR-001 — Injection

- OWASP: [A03:2021 – Injection](https://owasp.org/Top10/2021/A03_2021-Injection/) | version 2021 | 2021-09-24
- CWE: [CWE-89: Improper Neutralization of Special Elements used in an SQL Command ('SQL Injection')](https://cwe.mitre.org/data/definitions/89.html) | version 4.20 | 2024-11-19
- NIST SSDF: [PW.7 Review the Human-Readable Code](https://csrc.nist.gov/pubs/sp/800/218/final) | version 1.1 | 2022-02-04
- Control: CR-REV-001
- Trust boundary: external request input crosses into a SQL statement without parameterization, so the two reviewers `CR-REV-001` requires must both inspect every query-construction hunk rather than sampling it.
- Vulnerable fixture: tests/specs/AUR-015/SCR-001/vulnerable.md
- Fixed fixture: tests/specs/AUR-015/SCR-001/fixed.md

### Rule: SCR-002 — Broken Access Control

- OWASP: [A01:2021 – Broken Access Control](https://owasp.org/Top10/2021/A01_2021-Broken_Access_Control/) | version 2021 | 2021-09-24
- CWE: [CWE-862: Missing Authorization](https://cwe.mitre.org/data/definitions/862.html) | version 4.20 | 2024-11-19
- NIST SSDF: [PW.8 Test Executable Code](https://csrc.nist.gov/pubs/sp/800/218/final) | version 1.1 | 2022-02-04
- Control: CR-GATE-001
- Trust boundary: an authenticated-but-unauthorized caller crosses into a privileged handler; the gate ordering `CR-GATE-001` fixes must deny by default when the authorization check has not run, never allow by default.
- Vulnerable fixture: tests/specs/AUR-015/SCR-002/vulnerable.md
- Fixed fixture: tests/specs/AUR-015/SCR-002/fixed.md

### Rule: SCR-003 — Cryptographic Failures

- OWASP: [A02:2021 – Cryptographic Failures](https://owasp.org/Top10/2021/A02_2021-Cryptographic_Failures/) | version 2021 | 2021-09-24
- CWE: [CWE-321: Use of Hard-coded Cryptographic Key](https://cwe.mitre.org/data/definitions/321.html) | version 4.20 | 2024-11-19
- NIST SSDF: [PW.6 Configure the Compilation, Interpreter, and Build Processes](https://csrc.nist.gov/pubs/sp/800/218/final) | version 1.1 | 2022-02-04
- Control: CR-EVD-002
- Trust boundary: a secret embedded in source crosses from the repository — a widely-readable, untrusted-for-secrets boundary — into the runtime credential a cryptographic operation trusts; rotating the key or the source of truth must invalidate the evidence `CR-EVD-002` binds to it.
- Vulnerable fixture: tests/specs/AUR-015/SCR-003/vulnerable.md
- Fixed fixture: tests/specs/AUR-015/SCR-003/fixed.md

### Rule: SCR-004 — Vulnerable and Outdated Components

- OWASP: [A06:2021 – Vulnerable and Outdated Components](https://owasp.org/Top10/2021/A06_2021-Vulnerable_and_Outdated_Components/) | version 2021 | 2021-09-24
- CWE: [CWE-1104: Use of Unmaintained Third Party Components](https://cwe.mitre.org/data/definitions/1104.html) | version 4.20 | 2024-11-19
- NIST SSDF: [PW.4 Reuse Existing, Well-Secured Software](https://csrc.nist.gov/pubs/sp/800/218/final) | version 1.1 | 2022-02-04
- Control: CR-EVD-001
- Trust boundary: an unpinned or unlocked third-party dependency crosses from the network/registry — mutable and untrusted — into the build without a bound digest; the gate verdict `CR-EVD-001` requires must name the lock digest actually resolved, not the range requested.
- Vulnerable fixture: tests/specs/AUR-015/SCR-004/vulnerable.md
- Fixed fixture: tests/specs/AUR-015/SCR-004/fixed.md

### Rule: SCR-005 — Software and Data Integrity Failures

- OWASP: [A08:2021 – Software and Data Integrity Failures](https://owasp.org/Top10/2021/A08_2021-Software_and_Data_Integrity_Failures/) | version 2021 | 2021-09-24
- CWE: [CWE-502: Deserialization of Untrusted Data](https://cwe.mitre.org/data/definitions/502.html) | version 4.20 | 2024-11-19
- NIST SSDF: [PW.9 Configure Software to Have Secure Settings by Default](https://csrc.nist.gov/pubs/sp/800/218/final) | version 1.1 | 2022-02-04
- Control: CR-REV-002
- Trust boundary: attacker-controlled serialized bytes cross from an external channel into a deserializer that can instantiate arbitrary types by default; sealing the review under `CR-REV-002` prevents a shortcut approval before both reviewers see the exact deserialization surface.
- Vulnerable fixture: tests/specs/AUR-015/SCR-005/vulnerable.md
- Fixed fixture: tests/specs/AUR-015/SCR-005/fixed.md

### Rule: SCR-006 — Identification and Authentication Failures

- OWASP: [A07:2021 – Identification and Authentication Failures](https://owasp.org/Top10/2021/A07_2021-Identification_and_Authentication_Failures/) | version 2021 | 2021-09-24
- CWE: [CWE-287: Improper Authentication](https://cwe.mitre.org/data/definitions/287.html) | version 4.20 | 2024-11-19
- NIST SSDF: [PW.5 Create Source Code by Adhering to Secure Coding Practices](https://csrc.nist.gov/pubs/sp/800/218/final) | version 1.1 | 2022-02-04
- Control: CR-REV-003
- Trust boundary: an unauthenticated caller crosses into a session-bearing boundary when a credential or token check is missing or bypassable; `CR-REV-003`'s reviewer isolation stops the author of that check from being the sole approver of its own patch.
- Vulnerable fixture: tests/specs/AUR-015/SCR-006/vulnerable.md
- Fixed fixture: tests/specs/AUR-015/SCR-006/fixed.md

### Rule: SCR-007 — Security Logging and Monitoring Failures

- OWASP: [A09:2021 – Security Logging and Monitoring Failures](https://owasp.org/Top10/2021/A09_2021-Security_Logging_and_Monitoring_Failures/) | version 2021 | 2021-09-24
- CWE: [CWE-532: Insertion of Sensitive Information into Log File](https://cwe.mitre.org/data/definitions/532.html) | version 4.20 | 2024-11-19
- NIST SSDF: [RV.1 Identify and Confirm Vulnerabilities on an Ongoing Basis](https://csrc.nist.gov/pubs/sp/800/218/final) | version 1.1 | 2022-02-04
- Control: CR-REV-004
- Trust boundary: secret or sensitive request data crosses from the request boundary into a log sink read by a wider audience than the request's own trust level; the skeptical approver `CR-REV-004` requires must fault-inject exactly this leak and observe the acceptance command fail for it.
- Vulnerable fixture: tests/specs/AUR-015/SCR-007/vulnerable.md
- Fixed fixture: tests/specs/AUR-015/SCR-007/fixed.md

### Rule: SCR-008 — Security Misconfiguration

- OWASP: [A05:2021 – Security Misconfiguration](https://owasp.org/Top10/2021/A05_2021-Security_Misconfiguration/) | version 2021 | 2021-09-24
- CWE: [CWE-16: Configuration](https://cwe.mitre.org/data/definitions/16.html) | version 4.20 | 2024-11-19
- NIST SSDF: [PO.5 Implement Supporting Toolchains](https://csrc.nist.gov/pubs/sp/800/218/final) | version 1.1 | 2022-02-04
- Control: CR-REV-005
- Trust boundary: a permissive default configuration (debug mode, verbose errors, an open management port) crosses from the deployment boundary into production without an explicit, reviewed override; a single blocking misconfiguration finding must reject the candidate under `CR-REV-005`, never be waived by majority vote.
- Vulnerable fixture: tests/specs/AUR-015/SCR-008/vulnerable.md
- Fixed fixture: tests/specs/AUR-015/SCR-008/fixed.md

### Rule: SCR-009 — Server-Side Request Forgery (SSRF)

- OWASP: [A10:2021 – Server-Side Request Forgery (SSRF)](https://owasp.org/Top10/2021/A10_2021-Server-Side_Request_Forgery_%28SSRF%29/) | version 2021 | 2021-09-24
- CWE: [CWE-918: Server-Side Request Forgery (SSRF)](https://cwe.mitre.org/data/definitions/918.html) | version 4.20 | 2024-11-19
- NIST SSDF: [RV.3 Analyze Vulnerabilities to Identify Their Root Causes](https://csrc.nist.gov/pubs/sp/800/218/final) | version 1.1 | 2022-02-04
- Control: CR-EVD-003
- Trust boundary: a server-side fetch crosses from attacker-supplied URL input into an outbound network call without a destination allowlist, reaching internal-only endpoints; the red/green/mutation evidence `CR-EVD-003` demands must show the fetch actually blocked once the allowlist check exists, not merely asserted.
- Vulnerable fixture: tests/specs/AUR-015/SCR-009/vulnerable.md
- Fixed fixture: tests/specs/AUR-015/SCR-009/fixed.md
