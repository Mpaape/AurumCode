# AUR-016 spec fixtures

Single source of truth shared by `tests/acceptance/AUR-016.sh`, so the
required ISO/IEC 25010:2023 characteristic names and the allowlisted
source-reference domains are declared exactly once instead of duplicated
inside the acceptance program.

- `characteristics.txt`: one characteristic name per line, exactly the nine
  ISO/IEC 25010:2023 (edition 2) product-quality characteristics that
  `standards/quality/characteristics.yaml` must resolve, in the order used
  throughout `.board/research/quality-standards.md` and
  `docs/specs/AUR-016.md`.
- `allowed-source-domains.txt`: one hostname per line. Every `Source` cell
  in `.board/research/quality-standards.md`'s "Sources examined" table
  cites a `[label](url)` reference whose URL host must appear in this list,
  case-sensitive, no scheme, no path, no trailing slash. This is a static
  allowlist checked against cited text — neither file, and no part of
  AC-001, makes a network request.

Changing either file changes what AC-001 accepts; both files are within
this card's owned paths and are covered by the same review as the research
document and the standards table.
