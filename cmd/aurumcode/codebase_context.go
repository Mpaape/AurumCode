// Codebase context: the bounded structural pack of the changed files, as the
// JSON the prompt carries.
package main

import (
	"encoding/json"

	codebasectx "github.com/Mpaape/AurumCode/internal/context"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// resolveCodebaseContext is --base's codebase-context pass: bounded
// dependency context for the changed paths, resolved from the checkout so
// the model sees what else the change can affect. It is an enhancement,
// never a gate: any failure degrades to empty context and the review
// continues on the diff alone. --pr's own entrypoint,
// resolveVerifiedCodebaseContext (aur536.go), shares codebaseContextJSON's
// marshal-or-empty tail with this one -- AUR-490 parity -- but resolves
// through ResolveWithFiles against its own already-verified file set
// instead of codebaseContextPack's unrestricted walk, since on --pr the
// checkout is not necessarily the change under review (see aur515.go).
func resolveCodebaseContext(diff *types.Diff) string {
	return codebaseContextJSON(func() (*codebasectx.Pack, error) {
		return codebaseContextPack(diffPaths(diff))
	})
}

// codebaseContextJSON is the shared tail of both codebase-context passes
// (resolveCodebaseContext above, resolveVerifiedCodebaseContext in
// aur536.go): resolve, by whichever means the caller's closure embeds, and
// marshal the result, or degrade to "" on any error. Neither pass is ever
// a gate, so this never returns an error of its own.
func codebaseContextJSON(resolve func() (*codebasectx.Pack, error)) string {
	pack, err := resolve()
	if err != nil || pack == nil {
		return ""
	}
	data, err := json.Marshal(pack)
	if err != nil {
		return ""
	}
	return string(data)
}
