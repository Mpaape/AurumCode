// Package tools is the set of tools a review offers the model during
// deliberation (internal/deliberation): the scanner engines the
// configuration does not require (the required ones already ran before the
// model) and the codebase context of a changed file. Each tool is
// read-only, declares its cost and result size in the manifest the prompt
// shows, and reports to its caller what it produced, so a scanner's findings
// reach the gate with their origin whatever the model writes about them.
package tools
