package contracts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
)

// ContractAUR422 proves that the checked-in schema, profile and lock speak the
// same contract the runner enforces: identical required key sets, identical
// hardening constants, and an explicitly characterized set of numeric
// divergences. It reads repository documents only; no engine is involved.

// The exact key set .board/bin/oci-run enforces via require_exact_keys.
var runnerProfileKeysAUR422 = []string{
	"schema", "version", "profile", "lock", "lock_digest", "network", "user",
	"cap_drop", "cap_add", "mounts", "devices", "pull", "tmpfs",
	"read_only_rootfs", "no_new_privileges", "privileged", "timeout_seconds",
	"memory_mb", "cpu_millis", "pids_limit", "tmpfs_mb", "stdout_limit_bytes",
	"stderr_limit_bytes", "max_input_files", "max_input_bytes",
}

var runnerLockKeysAUR422 = []string{"schema", "version", "profile", "image"}

func readJSONMapAUR422(t *testing.T, root, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, path))
	if err != nil || len(b) == 0 {
		t.Fatalf("required input unreadable: %s", path)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("invalid JSON document: %s: %v", path, err)
	}
	return m
}

func sortedKeysAUR422(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sameSetAUR422(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	w := append([]string(nil), want...)
	sort.Strings(w)
	for i := range got {
		if got[i] != w[i] {
			return false
		}
	}
	return true
}

func schemaConstAUR422(t *testing.T, props map[string]any, key string) any {
	t.Helper()
	prop, ok := props[key].(map[string]any)
	if !ok {
		t.Fatalf("schema property missing: %s", key)
	}
	value, ok := prop["const"]
	if !ok {
		t.Fatalf("schema property %s declares no const", key)
	}
	return value
}

func ContractAUR422(t *testing.T) {
	root := os.Getenv("AURUMCODE_ROOT")
	if root == "" {
		t.Fatal("AURUMCODE_ROOT is required")
	}
	schema := readJSONMapAUR422(t, root, ".board/schemas/container-profile.schema.json")
	profile := readJSONMapAUR422(t, root, ".board/oci/profiles/bootstrap-readonly-v1.json")
	lock := readJSONMapAUR422(t, root, ".board/locks/oci/bootstrap-readonly-v1.lock.json")

	// The schema's required list and the runner's exact key set must be the
	// same 25 keys; a key present in one gate but not the other is unenforced.
	requiredRaw, ok := schema["required"].([]any)
	if !ok {
		t.Fatal("schema declares no required list")
	}
	required := make([]string, 0, len(requiredRaw))
	for _, r := range requiredRaw {
		s, ok := r.(string)
		if !ok {
			t.Fatal("schema required list is not all strings")
		}
		required = append(required, s)
	}
	sort.Strings(required)
	if !sameSetAUR422(required, runnerProfileKeysAUR422) {
		t.Fatalf("schema required set diverges from the runner key set: %v", required)
	}

	// The profile document must declare exactly the runner's key set. The
	// schema additionally allows an optional checkout_readonly the runner
	// refuses as unknown; the materialized profile must not declare it.
	if !sameSetAUR422(sortedKeysAUR422(profile), runnerProfileKeysAUR422) {
		t.Fatalf("profile key set diverges from the runner key set: %v", sortedKeysAUR422(profile))
	}

	// Hardening constants must be pinned identically in schema and profile.
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema declares no properties")
	}
	for key, want := range map[string]any{
		"schema":            "aurum.container-profile",
		"profile":           "bootstrap-readonly-v1",
		"lock":              ".board/locks/oci/bootstrap-readonly-v1.lock.json",
		"network":           "none",
		"cap_drop":          "ALL",
		"cap_add":           "none",
		"mounts":            "none",
		"devices":           "none",
		"pull":              "never",
		"tmpfs":             "rw,nosuid,nodev",
		"read_only_rootfs":  true,
		"no_new_privileges": true,
		"privileged":        false,
	} {
		if schemaConstAUR422(t, props, key) != want {
			t.Fatalf("schema const diverges for %s", key)
		}
		if profile[key] != want {
			t.Fatalf("profile value diverges from pinned const for %s: %v", key, profile[key])
		}
	}

	// Numeric bounds: characterize the exact divergence set between the schema
	// maxima and the materialized profile. Today that set is exactly
	// {tmpfs_mb} (schema maximum 128, materialized 512, runner ceiling 1024).
	// Any new divergence, or the silent disappearance of this one, is RED.
	divergent := []string{}
	for _, key := range []string{
		"timeout_seconds", "memory_mb", "cpu_millis", "pids_limit", "tmpfs_mb",
		"stdout_limit_bytes", "stderr_limit_bytes", "max_input_files", "max_input_bytes",
	} {
		prop, ok := props[key].(map[string]any)
		if !ok {
			t.Fatalf("schema property missing: %s", key)
		}
		max, ok := prop["maximum"].(float64)
		if !ok {
			t.Fatalf("schema property %s declares no maximum", key)
		}
		min, ok := prop["minimum"].(float64)
		if !ok {
			t.Fatalf("schema property %s declares no minimum", key)
		}
		value, ok := profile[key].(float64)
		if !ok {
			t.Fatalf("profile value for %s is not a number", key)
		}
		if value < min {
			t.Fatalf("profile %s is below the schema minimum", key)
		}
		if value > max {
			divergent = append(divergent, key)
		}
	}
	if len(divergent) != 1 || divergent[0] != "tmpfs_mb" {
		t.Fatalf("schema/profile numeric divergence set changed: %v", divergent)
	}
	if profile["tmpfs_mb"].(float64) != 512 {
		t.Fatalf("characterized tmpfs_mb divergence changed: %v", profile["tmpfs_mb"])
	}

	// Lock: exactly the runner's key set, the schema's own $defs identity, and
	// an image reference matching the schema's own pinned-digest pattern.
	if !sameSetAUR422(sortedKeysAUR422(lock), runnerLockKeysAUR422) {
		t.Fatalf("lock key set diverges from the runner key set: %v", sortedKeysAUR422(lock))
	}
	defs, ok := schema["$defs"].(map[string]any)
	if !ok {
		t.Fatal("schema declares no $defs")
	}
	lockSchema, ok := defs["ociImageLock"].(map[string]any)
	if !ok {
		t.Fatal("schema declares no ociImageLock definition")
	}
	lockProps, ok := lockSchema["properties"].(map[string]any)
	if !ok {
		t.Fatal("ociImageLock declares no properties")
	}
	if schemaConstAUR422(t, lockProps, "schema") != "aurum.oci-image-lock" ||
		schemaConstAUR422(t, lockProps, "profile") != "bootstrap-readonly-v1" {
		t.Fatal("ociImageLock identity consts diverge")
	}
	if lock["schema"] != "aurum.oci-image-lock" || lock["profile"] != "bootstrap-readonly-v1" ||
		lock["version"].(float64) != 1 {
		t.Fatal("lock identity diverges from the schema definition")
	}
	imageProp, ok := lockProps["image"].(map[string]any)
	if !ok {
		t.Fatal("ociImageLock declares no image property")
	}
	patternText, ok := imageProp["pattern"].(string)
	if !ok {
		t.Fatal("ociImageLock image property declares no pattern")
	}
	pattern, err := regexp.Compile(patternText)
	if err != nil {
		t.Fatalf("ociImageLock image pattern does not compile: %v", err)
	}
	image, ok := lock["image"].(string)
	if !ok || !pattern.MatchString(image) {
		t.Fatalf("lock image is not pinned per the schema's own pattern: %v", lock["image"])
	}
}
