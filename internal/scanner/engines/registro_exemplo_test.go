//go:build aurum_exemplo

package engines_test

import (
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/scanner"
	"github.com/Mpaape/AurumCode/internal/scanner/engines/exemplo"
)

// A binary built with the aurum_exemplo tag registers the example engine
// with its typed origin, and the configuration accepts it.
func TestTaggedBuildRegistersExampleEngine(t *testing.T) {
	e, ok := scanner.Lookup(exemplo.Name)
	if !ok || e.TypedOrigin() != exemplo.Name {
		t.Fatalf("example engine not registered (ok=%v, engine=%+v)", ok, e)
	}
	if _, err := config.Parse([]byte(exampleConfig), "teste"); err != nil {
		t.Fatalf("Parse: %v", err)
	}
}
