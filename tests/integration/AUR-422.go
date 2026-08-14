package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	unit "github.com/Mpaape/AurumCode/tests/unit"
)

// IntegrationAUR422 binds the characterization loader to the real runner: every
// ceiling, floor, pattern and diagnostic the loader hardcodes is re-extracted
// from the .board/bin/oci-run source, and the plan derived from the real
// checked-in documents is checked against the flags the runner would pass.
// Nothing here starts an engine.

func runnerIntAUR422(t *testing.T, src, name string) int {
	t.Helper()
	m := regexp.MustCompile(`(?m)^readonly ` + regexp.QuoteMeta(name) + `=([0-9]+)$`).FindStringSubmatch(src)
	if m == nil {
		t.Fatalf("runner source no longer declares %s", name)
	}
	v, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("runner constant %s is not an integer: %v", name, err)
	}
	return v
}

func runnerStrAUR422(t *testing.T, src, name string) string {
	t.Helper()
	m := regexp.MustCompile(`(?m)^readonly ` + regexp.QuoteMeta(name) + `='([^']*)'$`).FindStringSubmatch(src)
	if m == nil {
		t.Fatalf("runner source no longer declares %s", name)
	}
	return m[1]
}

func IntegrationAUR422(t *testing.T) {
	root := os.Getenv("AURUMCODE_ROOT")
	if root == "" {
		t.Fatal("AURUMCODE_ROOT is required")
	}
	runnerBytes, err := os.ReadFile(filepath.Join(root, ".board/bin/oci-run"))
	if err != nil || len(runnerBytes) == 0 {
		t.Fatal("runner source unreadable: .board/bin/oci-run")
	}
	src := string(runnerBytes)

	for name, want := range map[string]int{
		"ceiling_timeout_seconds": unit.CeilingTimeoutAUR422,
		"ceiling_memory_mb":       unit.CeilingMemoryMBAUR422,
		"ceiling_cpu_millis":      unit.CeilingCPUMillisAUR422,
		"ceiling_pids_limit":      unit.CeilingPidsAUR422,
		"ceiling_tmpfs_mb":        unit.CeilingTmpfsMBAUR422,
		"ceiling_output_limit":    unit.CeilingOutputAUR422,
		"ceiling_input_files":     unit.CeilingInputFilesAUR422,
		"ceiling_input_bytes":     unit.CeilingInputBytesAUR422,
		"floor_timeout_seconds":   unit.FloorTimeoutAUR422,
		"floor_memory_mb":         unit.FloorMemoryMBAUR422,
		"floor_cpu_millis":        unit.FloorCPUMillisAUR422,
		"floor_pids_limit":        unit.FloorPidsAUR422,
		"floor_tmpfs_mb":          unit.FloorTmpfsMBAUR422,
		"floor_output_limit":      unit.FloorOutputAUR422,
	} {
		if got := runnerIntAUR422(t, src, name); got != want {
			t.Fatalf("loader constant diverges from runner %s: loader=%d runner=%d", name, want, got)
		}
	}

	if runnerStrAUR422(t, src, "bootstrap_profile") != unit.ProfileKeyAUR422 {
		t.Fatal("runner bootstrap profile name diverges")
	}
	if runnerStrAUR422(t, src, "required_tmpfs_options") != unit.RequiredTmpfsAUR422 {
		t.Fatal("runner tmpfs options diverge from the loader")
	}
	if runnerStrAUR422(t, src, "digest_pinned_image_pattern") != unit.ImagePatternAUR422 {
		t.Fatal("runner image pattern diverges from the loader")
	}
	if runnerStrAUR422(t, src, "non_root_user_pattern") != unit.UserPatternAUR422 {
		t.Fatal("runner user pattern diverges from the loader")
	}

	// die 78 stays intact: exactly the three registration diagnostics, reached
	// only when the profile document, the registry, or the lock is absent.
	die78 := regexp.MustCompile(`die 78 `).FindAllStringIndex(src, -1)
	if len(die78) != 3 {
		t.Fatalf("runner die 78 diagnostic count changed: %d", len(die78))
	}
	for _, needle := range []string{
		"profile is not registered",
		"profile registry is absent",
		"has no lock at",
	} {
		if !regexp.MustCompile(regexp.QuoteMeta(needle)).MatchString(src) {
			t.Fatalf("runner die 78 diagnostic missing: %s", needle)
		}
	}

	read := func(path string) []byte {
		b, err := os.ReadFile(filepath.Join(root, path))
		if err != nil || len(b) == 0 {
			t.Fatalf("required input unreadable: %s", path)
		}
		return b
	}
	plan, code := unit.ResolveBootstrapPlanAUR422(
		read(unit.RegistryPathAUR422), read(unit.SchemaPathAUR422),
		read(unit.ProfilePathAUR422), read(unit.LockPathAUR422))
	if code != "valid" {
		t.Fatalf("nominal documents do not derive a plan: %s", code)
	}
	if plan.Tmpfs != "/tmp:exec,rw,nosuid,nodev,size=512m" {
		t.Fatalf("derived tmpfs flag diverges from the runner: %s", plan.Tmpfs)
	}
	if plan.Memory != "256m" || plan.CPUs != "1.000" {
		t.Fatalf("derived resource flags diverge from the runner: %s/%s", plan.Memory, plan.CPUs)
	}
	if plan.Network != "none" || !plan.ReadOnlyRootfs || !plan.NoNewPrivileges ||
		plan.CapDrop != "ALL" || plan.User == "" || plan.User[0] == '0' {
		t.Fatal("derived plan lost a hardening guarantee")
	}
	if plan.EngineInvocations != 0 {
		t.Fatalf("plan derivation invoked an engine %d time(s)", plan.EngineInvocations)
	}
	fmt.Printf("integration runner_bindings=ok plan_image=%s engine_invocations=0\n", plan.Image)
}
