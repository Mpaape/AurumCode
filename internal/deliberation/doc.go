// Package deliberation runs a bounded tool conversation with a model: the
// model may ask for tools the caller offers, each call's arguments are
// validated against the tool's declared schema before anything runs, each
// result goes back as a tool message, and the conversation ends with the
// model's answer or with a typed limit error. It knows nothing about code
// review: a tool is a Spec and a Run, a limit is a number from the
// configuration, and every exceeded limit is an error the caller must treat
// as inconclusive. A partial answer is never returned beside a limit error.
package deliberation
