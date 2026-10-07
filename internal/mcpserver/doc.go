// Package mcpserver is the Model Context Protocol adapter of the review: a
// local stdio server a coding agent (Claude Code, Codex, Cursor) asks before
// it commits. It adds no review logic of its own. Every review and gate
// answer comes from the Gateway, which the command implements with the very
// review session the CLI and CI run, so the agent sees the same policy, the
// same skills, the same redaction and the same fail-closed decision.
//
// The server is read only: no tool writes files, opens a pull request or
// changes policy, and no tool argument can turn a rule off or point at
// another policy. Arguments are validated against the declared JSON Schema
// before anything runs, and every response passes through one redaction
// choke point before it reaches the client.
package mcpserver
