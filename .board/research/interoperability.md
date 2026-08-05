# Interoperable artifact standards

- Researched: 2026-08-04
- Status: design input for AUR-017 and the adapter cards that depend on it
  (`O05-review` SARIF emission, `O06-scm` unified-diff proposals, `O04-agents`/
  `O11-mcp` JSON Schema contracts, an HTTP-facing OpenAPI description); not
  implementation proof
- Scope: the four artifact standards AurumCode's own adapters must interoperate
  with — SARIF, unified diff, JSON Schema, OpenAPI — plus the Model Context
  Protocol (MCP) context that makes the JSON Schema pin below a live
  interoperability boundary rather than an internal choice

## Standards matrix

This is the criteria matrix AC-001 resolves every standard named in the
Outcome against. A criterion is any column to the right of `Date`. Every
`Source` cell carries the primary normative reference as `[label](url)`; the
accept program checks the `url` host against the allowlist in
`tests/specs/AUR-017/allowed-source-domains.txt` and never dereferences it.
`Version` is either semantic-version-like (`2.1.0`, `3.10`), a draft-date
identifier (`2020-12`), or an immutable post identifier — the same three
shapes AUR-020 fixed for its own alternatives matrix, reused here rather than
invented twice.

| Standard | Source | Version | Date | Validator | Interoperability role |
|---|---|---|---|---|---|
| SARIF | [SARIF v2.1.0 OASIS Standard plus Errata 01](https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html) | 2.1.0 | 2023-08-28 | santhosh-tekuri/jsonschema v6.0.2 validates a `sarifLog` root against the OASIS-published `sarif-schema-2.1.0.json` (JSON Schema draft-07); this card's own lint checks the `version`, `$schema`, and non-empty `runs[].tool.driver.name` triad that `sarifLog` requires (§3.13.2-§3.13.4). | `internal/review` (`O05-review`) is the eventual emitter: deterministic and semantic findings become SARIF results for SARIF-aware hosts (GitHub code scanning, IDEs) once a dependent adapter card exists. This card fixes only the version and the validator it must satisfy. |
| Unified diff | [GNU Diffutils 3.10 manual — Detailed Description of Unified Format, Wayback Machine snapshot 2023-05-27](https://web.archive.org/web/20230527075044/https://www.gnu.org/software/diffutils/manual/html_node/Detailed-Unified.html) | 3.10 | 2023-05-21 | `git apply --check` (Git 2.43.0, [git-apply documentation](https://git-scm.com/docs/git-apply/2.43.0)) reports a clean or failed hunk application; this card's own lint checks that every `@@ -l,s +l,s @@` hunk's declared old/new line counts equal the context/removed and context/added line counts in its body. | `internal/scm` and `internal/publisher` (`O06-scm`) already read and write unified diffs as review output and proposed patches; this fixes the exact hunk-header contract those adapters must not silently violate. |
| JSON Schema | [JSON Schema Core, draft 2020-12](https://json-schema.org/draft/2020-12/json-schema-core) | 2020-12 | 2022-06-16 | santhosh-tekuri/jsonschema v6.0.2 validates a document against its declared `$schema` dialect; this card's own lint checks the root `$schema` is pinned to the draft-2020-12 meta-schema URI (`https://json-schema.org/draft/2020-12/schema`) and every `type` keyword names one of the seven JSON Schema primitive types. | `internal/mcp` (`O11-mcp`) tool schemas and `internal/skills` (`O04-agents`) skill contracts are JSON Schema documents. MCP's own SEP-1613 (below) fixes 2020-12 as its default dialect, so pinning the same draft here keeps AurumCode's schemas MCP-compatible without a translation step. |
| OpenAPI | [OpenAPI Specification v3.2.0](https://spec.openapis.org/oas/v3.2.0.html) | 3.2.0 | 2025-09-19 | getkin/kin-openapi v0.142.0 parses and validates an OpenAPI document against the 3.2.0 object model; this card's own lint checks the root `openapi` field equals the pinned version literal and both `info` and `paths` are present. | An HTTP-facing description of AurumCode's own Action/API surfaces (`ACTION_USAGE.md`) is the eventual publish target of a dependent adapter card; 3.2.0's native streaming support matters for `internal/orchestrator` (`O08-runtime`) event surfaces that unified-diff and SARIF adapters both stream through. |

## Interoperability context: MCP

The Model Context Protocol (MCP) is not one of the four artifact standards
this card fixes a version and validator for — it is the reason the JSON
Schema pin above is an external interoperability boundary and not an internal
implementation detail.

- **Version**: `2025-06-18`. **Source**: [MCP specification, revision
  2025-06-18](https://modelcontextprotocol.io/specification/2025-06-18)
  (host `modelcontextprotocol.io`).
- **OpenAI adoption**: on 2025-03-26, OpenAI's Sam Altman announced MCP
  support "across our products", available that day in the Agents SDK, with
  ChatGPT desktop and Responses API support to follow — [Sam Altman,
  post 1904957253456941061](https://x.com/sama/status/1904957253456941061)
  (host `x.com`). This is the second-adopter evidence that MCP is a
  cross-vendor interoperability boundary and not an Anthropic-only protocol;
  AurumCode's own `internal/mcp` (`O11-mcp`) must interoperate with both
  origins on the same wire contract.
- **Schema-dialect alignment**: [SEP-1613, "Establish JSON Schema 2020-12 as
  Default Dialect for
  MCP"](https://github.com/modelcontextprotocol/modelcontextprotocol/issues/1613)
  (host `github.com`) is accepted (wording and per-SDK reference
  implementation still rolling out as of this research) and resolves prior
  divergence between draft-07 and 2020-12 across MCP SDKs in favor of
  2020-12 — the same draft this card pins for JSON Schema above.

## Findings

1. SARIF, JSON Schema, and OpenAPI each have one currently normative,
   dated, versioned specification body (OASIS, the JSON Schema project, and
   the OpenAPI Initiative respectively); unified diff has no equivalent
   standards body; the closest dated, versioned normative text is the GNU
   Diffutils manual, which is why its "Version" is the diffutils release,
   not a format version number of its own.
2. SARIF 2.1.0 has not been superseded; the cited document is the OASIS
   Standard "plus Errata 01" text, the current normative version of 2.1.0,
   not a distinct 2.2 release.
3. JSON Schema's dialect identifier (`2020-12`) and the publication date of
   the currently hosted Core specification text (2022-06-16) are two
   different facts: the dialect name has been stable since December 2020,
   while the Internet-Draft text describing it has been periodically
   republished without changing the dialect name. This card cites the
   currently hosted text's own publication date rather than inventing an
   unverifiable December 2020 day-level date.
4. OpenAPI's latest minor version is 3.2.0 (2025-09-19), a small release
   that stays strictly compatible with 3.1; AurumCode pins 3.2.0 rather than
   3.1.x because 3.2.0 is the currently maintained line and its native
   streaming support is directly relevant to `O08-runtime` event surfaces
   named above.
5. MCP's independent, dated adoption by a second major provider (OpenAI,
   2025-03-26) is the concrete evidence that a JSON Schema pin feeding
   `internal/mcp` is an interoperability contract with parties outside this
   repository, not an internal convenience; SEP-1613 is the concrete
   evidence that 2020-12 is the dialect that contract has converged on.
6. Each validator pinned above is a real, versioned tool an adapter card may
   later run; this card does not run any of them against production data. It
   runs its own bounded structural lint against one accepting and one
   rejecting fixture per standard (`standards/contracts/*/fixtures/`) to
   prove the pinned version and validator identity stay consistent with the
   fixtures that exercise them, inside a network-denied container that
   cannot install or invoke the external validator binaries themselves.
7. Unlike SARIF (`sarif-v2.1.0` in the path), OpenAPI (`oas/v3.2.0`), JSON
   Schema (`draft/2020-12`), MCP (`specification/2025-06-18`), and
   `git-apply/2.43.0`, the live `www.gnu.org` GNU Diffutils manual page
   carries no version segment in its path: it republishes in place on every
   diffutils release and today reads "version 3.12, 12 January 2025" rather
   than 3.10. Citing that live URL as proof of the 3.10/2023-05-21 pin would
   therefore stop matching its own claim on the very next diffutils release,
   independent of anything this card's lint checks. The Source cell instead
   cites the Wayback Machine capture timestamped 2023-05-27 (six days after
   the 3.10 release), whose frozen content still reads "version 3.10, 2
   January 2023" and contains the identical Unified Format prose this card's
   validator rule is built from — an archived snapshot is exactly as
   immutable as a version-in-path URL for this purpose, so it satisfies the
   same pinning criterion the other four sources meet natively.

## MVP acceptance consequences

- Each of the four standards MUST have exactly one pinned version and one
  named validator in `standards/contracts/versions.yaml`; a version pinned
  here that no fixture demonstrates, or a fixture that names a version this
  document or `versions.yaml` does not pin, is a fixed regression AC-001
  detects (`AUR-017/AC-001/undecided-standard`).
- A `Source` cell whose host is not in
  `tests/specs/AUR-017/allowed-source-domains.txt`, or whose `Version` cannot
  be resolved as versioned/dated, fails the same way — the single-source
  host allowlist and version-shape rule are AUR-020's, not reinvented.
- This card produces no SARIF emitter, diff generator, schema author, or
  OpenAPI publisher. Those are dependent `O05-review`, `O06-scm`, `O04-agents`/
  `O11-mcp`, and integration adapter cards; this card only fixes what they
  must target.
