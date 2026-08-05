# AUR-020 spec fixtures

Single source of truth shared by `tests/acceptance/AUR-020.sh` and
`tests/integration/AUR-020.go`, so the required alternative names and the
allowlisted source-reference domains are declared exactly once.

- `alternatives.txt`: one alternative name per line, exactly the five names
  AC-001 requires `.board/research/memory-design.md`'s alternatives matrix
  and `.board/decisions/ADR-0001-optional-local-memory.md` to resolve, in
  the case used throughout both documents.
- `allowed-source-domains.txt`: one hostname per line. Every `Source` cell in
  the alternatives matrix cites a `[label](url)` reference whose URL host
  must appear in this list, case-sensitive, no scheme, no path, no
  trailing slash. This is a static allowlist checked against cited text —
  neither file, and no part of AC-001, makes a network request.

Changing either file changes what AC-001 accepts; both files are within this
card's owned paths and are covered by the same review as the ADR and the
research document.
