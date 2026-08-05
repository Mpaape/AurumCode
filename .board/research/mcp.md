# MCP conformance and security baseline (AUR-021)

This document fixes the Model Context Protocol (MCP) facts a future
`O11-mcp` server card must be testable against: the protocol revision, the
Go SDK version, the transport/authorization split, the tool-safety model,
and the injection-class attacks the specification itself names. It makes no
network request at acceptance time — every claim below is pinned to a
versioned, dated, allowlisted source and re-checked textually by
`tests/acceptance/AUR-021.sh`.

This card does not implement a server, does not register an executable
tool, and does not grant any new capability. It is policy/research data
that `internal/mcp/` (O11-mcp) implements against in a dependent card.

## Sources matrix

| Reference | Source | Version | Date | What it fixes |
|---|---|---|---|---|
| MCP core specification | [Specification](https://modelcontextprotocol.io/specification/2025-11-25) | 2025-11-25 | 2025-11-25 | Base JSON-RPC protocol, capability negotiation, `stdio`/Streamable HTTP transports |
| MCP schema (TypeScript source of truth) | [schema.ts](https://github.com/modelcontextprotocol/modelcontextprotocol/blob/2025-11-25/schema/2025-11-25/schema.ts) | 2025-11-25 | 2025-11-25 | `ToolAnnotations.readOnlyHint` / `destructiveHint` wire fields |
| MCP authorization specification | [Authorization](https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization) | 2025-11-25 | 2025-11-25 | OAuth 2.1-based authorization for HTTP transports; `stdio` explicitly out of scope for OAuth |
| MCP security best practices | [Security Best Practices](https://modelcontextprotocol.io/specification/2025-11-25/basic/security_best_practices) | 2025-11-25 | 2025-11-25 | Confused deputy, token passthrough, session-hijack prompt injection, local-server compromise, `stdio` proxy escalation |
| MCP release-candidate timeline | [The 2026-07-28 MCP Specification Release Candidate](https://blog.modelcontextprotocol.io/posts/2026-07-28-release-candidate/) | 2026-07-28 (RC locked 2026-05-21) | 2026-07-28 | Confirms 2025-11-25 is the last non-breaking stable revision at pin time; next revision is a breaking, stateless rewrite |
| Go SDK release | [go-sdk v1.6.1](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.6.1) | v1.6.1 | 2026-05-22 | Adopted Go SDK version; module `github.com/modelcontextprotocol/go-sdk`, targets protocol 2025-11-25, predates the 2026-07-28 breaking rewrite shipped in v1.7.0 |
| Go SDK breaking-change disclosure | [go-sdk v1.7.0](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.7.0) | v1.7.0 | 2026-07-28 | Documents why v1.7.0 (stateless model, `2026-07-28` only) is *not* adopted yet: it is one release old at pin time and rewrites the wire protocol |

## Protocol version and SDK adopted

- **MCP protocol version fixed: `2025-11-25`.** It is the last stable,
  non-release-candidate specification revision as of this pin (see the RC
  timeline row above); the successor revision `2026-07-28` is a breaking,
  stateless rewrite that deprecates roots/sampling/logging and had shipped
  for exactly zero days of ecosystem validation when this document was
  written.
- **Go SDK version fixed: `github.com/modelcontextprotocol/go-sdk` `v1.6.1`**
  (published 2026-05-22). It is the newest release that targets protocol
  `2025-11-25` without forcing the `2026-07-28` stateless rewrite; v1.7.0
  (2026-07-28) is the SDK release that introduces that rewrite and is
  explicitly not adopted by this baseline.

## Transport: `stdio`

Per the Base Protocol and Security Best Practices references above, the
`stdio` transport connects exactly one client process to exactly one server
process over the child's stdin/stdout. The specification states the
`stdio` transport itself is not inherently vulnerable, but a *proxy*
architecture that spawns MCP servers as child processes over `stdio` on a
client-side vulnerability (e.g. an OAuth-URL XSS) can escalate to remote
code execution ("`stdio` Transport Security in Proxy Scenarios"). Servers
intending to run locally **SHOULD** use `stdio` "to limit access to just
the MCP client" rather than an HTTP transport reachable from other local
processes.

## Authorization: OAuth

The MCP authorization specification (2025-11-25) scopes authorization to
HTTP-based transports only:

> Implementations using an HTTP-based transport SHOULD conform to this
> specification. Implementations using an STDIO transport SHOULD NOT follow
> this specification, and instead retrieve credentials from the
> environment.

Where authorization applies, MCP servers act as OAuth 2.1 resource servers
and clients as OAuth 2.1 clients; the authorization server issues access
tokens validated by the MCP server (PKCE required, `resource` parameter
required per RFC 8707, token passthrough forbidden, `stdio` credentials
sourced from the environment instead of an OAuth flow).

## Injection-class attacks

The Security Best Practices reference names an explicit "Session Hijack
Prompt Injection" attack: an attacker delivers a malicious payload to one
stateful HTTP server instance keyed by a guessed or leaked session id, and
a second server instance polling a shared event queue for that session id
relays the payload back to the original client as if it were a legitimate
asynchronous response, so the client acts on attacker-controlled content it
believes came from the server it is talking to. Mitigation: MCP servers
implementing authorization MUST verify every inbound request and MUST NOT
use sessions for authentication; session ids MUST be non-deterministic and
SHOULD be bound to the authorized user, not accepted as a bare capability
token.

Tool descriptions and `annotations` are a second injection surface: the
Tools reference is explicit that "clients MUST consider tool annotations to
be untrusted unless they come from trusted servers" — a hint is not a
security boundary an untrusted server can set for itself.

## Capability negotiation

MCP's base protocol advertises what a session may do through `capabilities`
exchanged during `initialize`/`server/discover`: a server that supports
tools MUST declare the `tools` capability (with `listChanged` if it emits
list-change notifications) before a client may call `tools/list` or
`tools/call`; clients similarly declare `sampling`, `roots`, and
`elicitation` capabilities. A tool call for a capability the peer never
declared is a protocol violation, not an implicit grant.

## Read-only versus mutable tool separation

The MCP schema (`schema.ts`, `ToolAnnotations`) already carries this
distinction at the wire level:

- `readOnlyHint` (`boolean`, default `false`): "If true, the tool does not
  modify its environment."
- `destructiveHint` (`boolean`, default `true`, meaningful only when
  `readOnlyHint == false`): "If true, the tool may perform destructive
  updates to its environment. If false, the tool performs only additive
  updates."

Because these are server-declared *hints* the specification itself treats
as untrusted, this baseline does not let a server's own claim decide
whether a tool may mutate state. Instead `standards/mcp/tools.yaml` fixes
the project's own classification, independent of what any server
advertises: every tool this project's own agents may call is listed with a
`class` of `read_only` or `mutable`, and every `mutable` entry MUST set
`requires_explicit_consent: true`, matching the specification's own rule
that "Hosts must obtain explicit user consent before invoking any tool."
`review.publish` is fixed as `mutable` with `requires_explicit_consent:
true`: it is a privileged, state-changing action (it publishes a review to
an external system) and MUST NOT be reachable as `read_only`, regardless of
what any future MCP server's own tool annotation claims.

## Non-goals

- This document does not implement `internal/mcp/`, does not start a
  server process, and does not register a callable tool. A dependent
  O11-mcp card builds the transport and tool registry against the pins
  above.
- It does not re-litigate the alternatives already fixed by AUR-020
  (memory/index design); MCP is a transport and tool-exposure concern, not
  a storage concern.
