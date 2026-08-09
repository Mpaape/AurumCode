package documentationcharacterization

import "testing"

func assertPackageProbe(t *testing.T, pkg string) {
	t.Helper()
	result, err := ProbePackage(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Package != pkg || result.Test == "" || result.Observation == "" {
		t.Fatalf("probe for %s returned an incomplete observation", pkg)
	}
}

func TestExtractorCoreRegistryAndDetector(t *testing.T) {
	assertPackageProbe(t, "internal/documentation/extractors")
}

func TestBashExtractorComments(t *testing.T) {
	assertPackageProbe(t, "internal/documentation/extractors/bash")
}

func TestCPPExtractorDoxygenOutput(t *testing.T) {
	assertPackageProbe(t, "internal/documentation/extractors/cpp")
}

func TestCSharpExtractorExecutesBuild(t *testing.T) {
	assertPackageProbe(t, "internal/documentation/extractors/csharp")
}

func TestGoExtractorWritesConfirmedOutput(t *testing.T) {
	assertPackageProbe(t, "internal/documentation/extractors/go")
}

func TestJavaScriptExtractorTypeDocOutput(t *testing.T) {
	assertPackageProbe(t, "internal/documentation/extractors/javascript")
}

func TestPowerShellExtractorComments(t *testing.T) {
	assertPackageProbe(t, "internal/documentation/extractors/powershell")
}

func TestPythonExtractorCapturesNonDocstringLiteral(t *testing.T) {
	assertPackageProbe(t, "internal/documentation/extractors/python")
}

func TestRustExtractorExecutesCargoDoc(t *testing.T) {
	assertPackageProbe(t, "internal/documentation/extractors/rust")
}

func TestIncrementalCacheFollowsOutputSymlink(t *testing.T) {
	assertPackageProbe(t, "internal/documentation/incremental")
}

func TestNormalizerPreservesFrontMatter(t *testing.T) {
	assertPackageProbe(t, "internal/documentation/normalizer")
}

func TestSiteScaffoldPreservesConfig(t *testing.T) {
	assertPackageProbe(t, "internal/documentation/site")
}

func TestLegacyPackageMatrix(t *testing.T) {
	results, err := RunAllProbes()
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != len(ExpectedPackages()) {
		t.Fatalf("executed %d package probes, want %d", len(results), len(ExpectedPackages()))
	}
}
