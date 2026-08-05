# AUR-021 spec fixtures

Single source of truth read by `tests/acceptance/AUR-021.sh`, so the
required literal terms, the allowlisted reference-source domains, and the
tools that must be classified `mutable` with explicit consent are declared
exactly once instead of duplicated inside the shell script.

- `required-terms.txt`: one term per line, the exact five case-insensitive
  terms `.board/research/mcp.md` must contain (`stdio`, `read-only`,
  `OAuth`, `injection`, `capability`), per AC-001's Given.
- `allowed-source-domains.txt`: one hostname per line. Every source link
  cited in `.board/research/mcp.md`'s "Sources matrix" table must resolve
  to a host in this list, case-sensitive, no scheme, no path, no trailing
  slash. This is a static allowlist checked against cited text — no part of
  AC-001 makes a network request.
- `required-mutable-tools.txt`: one tool name per line. Every name listed
  here must appear in `standards/mcp/tools.yaml` with `class: mutable` and
  `requires_explicit_consent: true`; a tool this file lists that is
  reclassified `read_only` (MUT-001) fails AC-001.

Changing any of these three files changes what AC-001 accepts; all three
are within this card's owned paths and are covered by the same review as
the research document and `standards/mcp/`.
