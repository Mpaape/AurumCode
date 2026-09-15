package integration

// Integration program for card AUR-500, selector IntegrationAUR500.
//
// Exercises the repository-config selection path end to end at the filesystem
// seam: a real .aurumcode/config.yml on disk names a profile, the file is read
// and decoded, a review flag is layered on top, and internal/reviewprofile
// resolves the preset -- while the deterministic boundary values stay fixed
// across every built-in and an unknown configured name fails closed before any
// model call.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/reviewprofile"
	"gopkg.in/yaml.v3"
)

func IntegrationAUR500(t *testing.T) {
	t.Run("ConfigFileSelectsProfile", testAUR500ConfigFileSelectsProfile)
	t.Run("FlagOverridesConfiguredProfile", testAUR500FlagOverridesConfiguredProfile)
	t.Run("UnknownConfiguredProfileFailsClosed", testAUR500UnknownConfiguredProfileFailsClosed)
	t.Run("EveryProfileKeepsSafetyBoundaries", testAUR500EveryProfileKeepsSafetyBoundaries)
}

type repoConfig struct {
	Review struct {
		Profile string `yaml:"profile"`
	} `yaml:"review"`
}

func writeConfig(t *testing.T, profile string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".aurumcode")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	doc := "review:\n  profile: " + profile + "\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func configuredProfile(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".aurumcode", "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg repoConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg.Review.Profile
}

func testAUR500ConfigFileSelectsProfile(t *testing.T) {
	root := writeConfig(t, "seguranca")
	res, err := reviewprofile.Resolve(reviewprofile.Selection{Config: configuredProfile(t, root)})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !res.Applied || res.Profile.Name != "seguranca" {
		t.Fatalf("config-selected profile = %+v", res)
	}
	if !strings.Contains(res.Declared, "seguranca") {
		t.Fatalf("declaration %q does not name the entered profile", res.Declared)
	}
}

func testAUR500FlagOverridesConfiguredProfile(t *testing.T) {
	root := writeConfig(t, "performance")
	res, err := reviewprofile.Resolve(reviewprofile.Selection{
		Config: configuredProfile(t, root),
		Flag:   "solid",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.Profile.Name != "solid" {
		t.Fatalf("flag did not override config: %q", res.Profile.Name)
	}
}

func testAUR500UnknownConfiguredProfileFailsClosed(t *testing.T) {
	root := writeConfig(t, "desconhecido")
	_, err := reviewprofile.Resolve(reviewprofile.Selection{Config: configuredProfile(t, root)})
	if err == nil {
		t.Fatal("unknown configured profile resolved")
	}
	if !errors.Is(err, reviewprofile.ErrUnknownProfile) {
		t.Fatalf("error %v is not unknown-profile", err)
	}
	if !strings.Contains(err.Error(), "desconhecido") {
		t.Fatalf("error %q does not name the configured profile", err.Error())
	}
}

func testAUR500EveryProfileKeepsSafetyBoundaries(t *testing.T) {
	for _, name := range reviewprofile.Names() {
		res, err := reviewprofile.Resolve(reviewprofile.Selection{Flag: name})
		if err != nil {
			t.Fatalf("resolve %s: %v", name, err)
		}
		e := res.Effective
		if !e.SecurityPassEnabled || !e.RedactionEnabled {
			t.Fatalf("%s weakened a safety boundary: %+v", name, e)
		}
		if e.SeverityFloor != "" || e.FailOnThreshold != "" || e.CostCap != -1 {
			t.Fatalf("%s changed a non-profile knob: %+v", name, e)
		}
	}
}

// The test file is compiled as package integration by the acceptance bridge;
// keep the yaml import used even if the decode is trimmed.
var _ = yaml.Unmarshal
