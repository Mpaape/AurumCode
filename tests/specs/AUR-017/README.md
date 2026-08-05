# AUR-017 spec fixtures

Single source of truth shared by `tests/acceptance/AUR-017.sh`, so the
required standard names and the allowlisted source-reference domains are
declared exactly once.

- `formats.txt`: one standard name per line, exactly the four names AC-001
  requires `.board/research/interoperability.md`'s "Standards matrix" table
  and `standards/contracts/versions.yaml` to resolve, in the case used
  throughout both.
- `allowed-source-domains.txt`: one hostname per line. Every `Source` cell in
  the Standards matrix, and the MCP context section below it, cites a
  `[label](url)` reference whose URL host must appear in this list,
  case-sensitive, no scheme, no path, no trailing slash. This is a static
  allowlist checked against cited text — neither file, and no part of
  AC-001, makes a network request.

Changing either file changes what AC-001 accepts; both files are within this
card's owned paths and are covered by the same review as the research
document and `standards/contracts`.
