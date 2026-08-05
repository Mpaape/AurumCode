# JSON Schema validator

- Pinned version: `2020-12` (see `../versions.yaml`, `json-schema.version`),
  the draft dialect identifier — not to be confused with the 2022-06-16
  publication date of the currently hosted Core specification text that
  describes it (`.board/research/interoperability.md`, Findings #3).
- Source: [JSON Schema Core, draft 2020-12](https://json-schema.org/draft/2020-12/json-schema-core).
- Named validator for a future adapter card: santhosh-tekuri/jsonschema
  v6.0.2. Not installed or invoked by this card.
- Structural rule this card's own bounded lint checks instead:
  1. `"$schema"` is present and equals `json-schema.schema_uri` in
     `../versions.yaml` exactly (the draft-2020-12 meta-schema URI).
  2. Every `"type"` keyword value is one of the seven JSON Schema primitive
     types: `object`, `array`, `string`, `number`, `integer`, `boolean`,
     `null`.
- `fixtures/valid.schema.json` satisfies both. `fixtures/invalid.schema.json`
  keeps a correct `"$schema"` and fails rule 2 only, with a `"type": "text"`
  value that is not one of the seven primitives — so the failure names the
  single offending keyword rather than the dialect pin.
