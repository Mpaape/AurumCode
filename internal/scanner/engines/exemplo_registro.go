//go:build aurum_exemplo

package engines

import (
	"github.com/Mpaape/AurumCode/internal/scanner"
	"github.com/Mpaape/AurumCode/internal/scanner/engines/exemplo"
)

// The example engine of the extension guide exists only in a binary built
// with the tag aurum_exemplo (docker build --build-arg GO_TAGS=aurum_exemplo).
func init() { scanner.Register(exemplo.Engine()) }
