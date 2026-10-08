// Package mcp is the client side of a configured MCP (Model Context
// Protocol) context source: it starts the server the trusted
// configuration names, calls one declared tool with only the declared,
// redacted payload, and returns the tool's text as untrusted background
// whose origin is the source's name. It never discovers servers, never
// sends the repository, and never calls a tool the configuration did not
// name. What the server answers is data for the prompt's repository-context
// slot: it cannot approve, change a rule, a gate or a permission, because
// nothing that decides those reads it.
package mcp
