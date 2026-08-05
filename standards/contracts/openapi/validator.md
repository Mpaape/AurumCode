# OpenAPI validator

- Pinned version: `3.2.0` (see `../versions.yaml`, `openapi.version`).
- Source: [OpenAPI Specification v3.2.0](https://spec.openapis.org/oas/v3.2.0.html).
- Named validator for a future adapter card: getkin/kin-openapi v0.142.0.
  Not installed or invoked by this card.
- Structural rule this card's own bounded lint checks instead:
  1. A top-level `openapi:` field is present and equals the pinned version
     literal exactly.
  2. A top-level `info:` block is present.
  3. A top-level `paths:` block is present.
- `fixtures/valid.openapi.yaml` satisfies all three. `fixtures/invalid.openapi.yaml`
  keeps the correct `openapi:` and `info:` and omits `paths:` entirely, so
  the failure names rule 3 only.
