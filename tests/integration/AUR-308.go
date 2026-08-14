package integration

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	characterization "github.com/Mpaape/AurumCode/tests/characterization/legacy/documentation"
)

func IntegrationAUR308(t *testing.T) {
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
	discovered := characterization.ExpectedPackages()
	if packageList := os.Getenv("AURUM_A308_PACKAGE_LIST"); packageList != "" {
		discovered, err = characterization.LoadPackageList(packageList)
		if err != nil {
			t.Fatal(err)
		}
	}
	stage := t.TempDir()
	first, err := characterization.WriteArtifact(stage, filepath.Join(stage, "first.json"), "sha256:replay", rows, probes, discovered)
	if err != nil {
		t.Fatal(err)
	}
	second, err := characterization.WriteArtifact(stage, filepath.Join(stage, "second.json"), "sha256:replay", rows, probes, discovered)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("exact replay produced different artifact bytes")
	}

	if output := os.Getenv("AURUM_A308_ARTIFACT"); output != "" {
		outputStage := os.Getenv("AURUM_A308_ARTIFACT_STAGE")
		if outputStage == "" {
			t.Fatal("AURUM_A308_ARTIFACT_STAGE is required with artifact output")
		}
		if _, err := characterization.WriteArtifact(outputStage, output, os.Getenv("AURUM_A308_INPUT_DIGEST"), rows, probes, discovered); err != nil {
			t.Fatal(err)
		}
	}
}
