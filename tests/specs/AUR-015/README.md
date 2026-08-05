# AUR-015 spec fixtures

Single source of truth shared by `tests/acceptance/AUR-015.sh` and
`standards/security-review/rules.md`, so the required rule IDs and the
allowlisted source-reference hosts are declared exactly once.

- `rule-ids.txt`: one rule ID per line, exactly the nine IDs
  `standards/security-review/rules.md` must resolve — one per `CR-*`
  control the card's `controls:` list declares, in that list's order.
- `allowed-source-domains.txt`: one hostname per line. Every `OWASP`,
  `CWE`, and `NIST SSDF` citation in `rules.md` links to a `[label](url)`
  reference whose URL host must appear in this list, case-sensitive, no
  scheme, no path, no trailing slash. This is a static allowlist checked
  against cited text — neither this file, nor any part of AC-001, makes a
  network request.
- `<rule-id>/vulnerable.md` and `<rule-id>/fixed.md`: one pair per rule ID
  above. Each is a short fenced code example: `vulnerable.md` demonstrates
  the weakness the rule's CWE names, and `fixed.md` demonstrates the
  corrected version of the same example. Both files carry a leading
  `Fixture: VULNERABLE` or `Fixture: FIXED` marker line so the acceptance
  program can tell them apart and refuse a pair that was accidentally
  swapped, left identical, or left empty.

Changing any of these files changes what `AC-001` accepts; all are within
this card's owned paths and are covered by the same review as
`standards/security-review/rules.md` and `.board/research/secure-code-review.md`.
