// Package changelog turns a list of reviewed commit messages into a
// deterministic semver bump and a Markdown changelog entry.
//
// The engine never calls an LLM itself, never touches the network, never
// reads .git at runtime and never writes to the repository. The bump and the
// rendered entry are model-free: the same []Commit always produces
// byte-identical output. For the required-entry suggestion it only validates,
// as untrusted data, a model answer the caller obtained (SuggestFromModel).
// Commit
// text is treated as untrusted data. It is classified, bounded and escaped
// before it reaches the rendered entry; it is never executed or shelled out,
// and text such as "chore: release v9.0.0" cannot inflate the computed bump.
//
// The rendered entry is advisory. It is not a git tag, not a release and not
// an authorization to publish anything; publication is owned by a later card.
package changelog
