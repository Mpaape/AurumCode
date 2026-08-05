# AurumCode interoperable artifact contracts

This directory fixes the exact version and validator AurumCode adopts for
each of the four interoperable artifact standards named in AUR-017's
Outcome: SARIF, unified diff, JSON Schema, and OpenAPI. It does not emit,
parse, or publish any of them — later `O05-review`, `O06-scm`, `O04-agents`/
`O11-mcp`, and integration adapter cards implement against what is fixed
here.

- `versions.yaml` is the single source of truth for the pinned version (and,
  where relevant, pinned schema/dialect URI) per format; `.board/research/
  interoperability.md`'s "Standards matrix" table and every fixture below
  must stay consistent with it. `tests/acceptance/AUR-017.sh` re-derives
  that consistency rather than trusting either file alone.
- `<format>/validator.md` names the exact validator tool and version an
  adapter card will run against real output, and the specific structural
  rule this card's own bounded lint checks in its place (no external
  validator binary is installed or invoked inside the sealed acceptance
  container).
- `<format>/fixtures/valid.*` is one minimal example the named validator
  accepts; `<format>/fixtures/invalid.*` is one minimal example it rejects,
  for a stated, single reason. Neither fixture is real AurumCode output —
  both are illustrative, secret-free, and exist only to prove the pinned
  version and validator identity are exercised by something.

| Format | Directory | Version | Validator |
|---|---|---|---|
| SARIF | `sarif/` | 2.1.0 | santhosh-tekuri/jsonschema v6.0.2 against the OASIS `sarif-schema-2.1.0.json` |
| Unified diff | `unified-diff/` | 3.10 | `git apply --check` (Git 2.43.0) |
| JSON Schema | `json-schema/` | 2020-12 | santhosh-tekuri/jsonschema v6.0.2 |
| OpenAPI | `openapi/` | 3.2.0 | getkin/kin-openapi v0.142.0 |

Raising a pinned version here without migrating the matching row in
`.board/research/interoperability.md` and both fixtures under that format's
`fixtures/` directory is exactly AUR-017's `MUT-001` mutation, and
`tests/acceptance/AUR-017.sh` is required to fail typed when it happens.
