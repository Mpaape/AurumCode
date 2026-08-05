# SARIF validator

- Pinned version: `2.1.0` (see `../versions.yaml`, `sarif.version`).
- Source: [SARIF v2.1.0 OASIS Standard plus Errata 01](https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html).
- Named validator for a future adapter card: santhosh-tekuri/jsonschema
  v6.0.2, run against the OASIS-published `sarif-schema-2.1.0.json`
  (JSON Schema draft-07). Not installed or invoked by this card.
- Structural rule this card's own bounded lint checks in the sealed
  acceptance container instead (§3.13.2-§3.13.4 of the cited standard):
  1. `"version"` is present and equals the pinned version exactly.
  2. `"$schema"` is present and equals `sarif.schema_uri` in
     `../versions.yaml` exactly.
  3. A `"driver"` object with a `"name"` property is present — the
     structural proxy for "at least one non-empty `runs[].tool.driver`",
     since an empty `runs: []` array has no `driver` key anywhere in the
     document.
- `fixtures/valid.sarif.json` satisfies all three. `fixtures/invalid.sarif.json`
  declares an empty `runs: []` and so fails rule 3 only, independent of the
  version pin.
