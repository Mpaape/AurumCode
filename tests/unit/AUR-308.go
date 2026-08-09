package unit

import (
	"os"
	"path/filepath"
	"testing"

	characterization "github.com/Mpaape/AurumCode/tests/characterization/legacy/documentation"
)

func TestAUR308(t *testing.T) {
	root := os.Getenv("AURUMCODE_ROOT")
	if root == "" {
		t.Fatal("AURUMCODE_ROOT is required")
	}
	manifest := os.Getenv("AURUM_A308_MANIFEST")
	if manifest == "" {
		manifest = filepath.Join(root, characterization.ManifestPath)
	}
	packageList := os.Getenv("AURUM_A308_PACKAGE_LIST")
	if packageList == "" {
		t.Fatal("AURUM_A308_PACKAGE_LIST is required")
	}
	rows, err := characterization.LoadClassifications(manifest)
	if err != nil {
		t.Fatal(err)
	}
	discovered, err := characterization.LoadPackageList(packageList)
	if err != nil {
		t.Fatal(err)
	}
	probes, err := characterization.RunAllProbes()
	if err != nil {
		t.Fatal(err)
	}
	if err := characterization.VerifyClassifications(rows, probes, discovered); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 12 {
		t.Fatalf("classified %d packages, want 12", len(rows))
	}
}
