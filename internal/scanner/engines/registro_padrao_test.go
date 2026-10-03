//go:build !aurum_exemplo

package engines_test

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/scanner"
	"github.com/Mpaape/AurumCode/internal/scanner/engines/exemplo"
)

// The default binary does not hold the example engine: the registry does
// not know it and the configuration refuses it when parsed.
func TestDefaultBuildRefusesExampleEngine(t *testing.T) {
	if _, ok := scanner.Lookup(exemplo.Name); ok {
		t.Fatal("the example engine is registered without the aurum_exemplo tag")
	}
	_, err := config.Parse([]byte(exampleConfig), "teste")
	if err == nil || !strings.Contains(err.Error(), `unknown engine "exemplo"`) {
		t.Fatalf("Parse err = %v, want unknown engine", err)
	}
}
