package contracts

import (
	"os"
	"path/filepath"
	"testing"

	characterization "github.com/Mpaape/AurumCode/tests/characterization/legacy/documentation"
)

func ContractAUR308(t *testing.T) {
	root := os.Getenv("AURUMCODE_ROOT")
	if root == "" {
		t.Fatal("AURUMCODE_ROOT is required")
	}
	rows, err := characterization.LoadClassifications(filepath.Join(root, characterization.ManifestPath))
	if err != nil {
		t.Fatal(err)
	}
	probes, err := characterization.RunAllProbes()
	if err != nil {
		t.Fatal(err)
	}
	if err := characterization.VerifyClassifications(rows, probes, characterization.ExpectedPackages()); err != nil {
		t.Fatal(err)
	}

	counts := map[string]int{}
	for _, row := range rows {
		counts[row.Disposition]++
		if row.Package == "internal/documentation/welcome" {
			t.Fatal("welcome is outside AUR-308's closed package scope")
		}
	}
	if counts["migrate"] == 0 || counts["replace"] == 0 || counts["keep"] != 0 || counts["delete"] != 0 {
		t.Fatalf("unexpected disposition contract: %#v", counts)
	}
}
