# AUR-019 spec fixtures

Single source of truth shared by `tests/acceptance/AUR-019.sh` and
`tests/integration/AUR-019_test.go`, so the required provider names, the tracked
capability set and the allowlisted source domains are declared exactly once.

- `provider-names.txt`: one provider name per line, exactly the seven names
  AC-001 requires `.board/research/providers.md`'s contracts matrix and every
  `standards/providers/<slug>.yaml` file to resolve, in the case used
  throughout both. The provider's slug (the `standards/providers` file
  stem and the `fixtures/` directory name) is the lowercase form of this
  exact name, with no other transformation.
- `capabilities.txt`: exactly three capability keys, `streaming`, `tool_use`,
  and `structured_output`. Every `standards/providers/<slug>.yaml` must declare
  a `capability_<name>:` status for all three. A status of `supported` requires
  the matching `fixture_<name>:` entry in the same file.
- `allowed-source-domains.txt`: one hostname per line. Every `Source` cell in
  the research matrix and every `source:` field in
  `standards/providers/<slug>.yaml` cites a `[label](url)` reference whose
  URL host must appear in this list, case-sensitive, no scheme, no path, no
  trailing slash. This is a static allowlist checked against cited text —
  neither file, and no part of AC-001, makes a network request.
- `fixtures/<slug>/<capability>.json`: one recorded, non-secret fixture per
  `capability_<name>: supported` declaration in
  `standards/providers/<slug>.yaml`. Each fixture is transcribed from the
  provider's own cited documentation, never captured from a live call. A
  fixture's own `capability`, `provider`, and `source` fields must match the
  directory, file, and standards record it lives under. The YAML reference must
  be exactly this path, with no `..`, absolute path, decoy descendant, `.flat`
  sidecar, or symlinked ancestor. The standards records themselves are flat
  `key: value` files at `standards/providers/<slug>.yaml`.

The shell and Go readers apply the same printable-ASCII-plus-LF byte allowlist
to every machine-readable input before parsing. They also require the matrix's
four exact criteria columns and compare every source, version, date, wire,
auth, error, and capability cell to a canonical projection of the matching
standards record. Source URLs must contain their declared version and must not
use mutable branch or `latest` references.

Changing any of these files changes what AC-001 accepts; all of them are
within this card's owned paths and are covered by the same review as the
research document and the standards.
