package contracts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var digestAUR410 = regexp.MustCompile(`\Asha256:[0-9a-f]{64}\z`)

// ContractAUR410 pins the public contract of the oci-conformance-v1 entry: the three
// documents this card publishes exist, parse, and declare exactly the key, schema id and
// lock path the registry advertises, exactly once. It reads nothing else, invokes no
// container engine and starts no probe.
//
// The registry's total arity is deliberately not asserted here. This card owns the
// oci-conformance-v1 key, not the size of the registry; the exact registered key set is
// asserted by tests/unit/AUR-410.go, which this card also owns and which the next profile
// card must extend when it registers an eleventh key.
func ContractAUR410(t *testing.T) {
	root := os.Getenv("AURUMCODE_ROOT")
	if root == "" {
		t.Fatal("AURUMCODE_ROOT is required")
	}
	read := func(path string) map[string]any {
		b, err := os.ReadFile(filepath.Join(root, path))
		if err != nil || len(b) == 0 {
			t.Fatalf("unreadable %s", path)
		}
		var doc map[string]any
		if json.Unmarshal(b, &doc) != nil {
			t.Fatalf("invalid JSON %s", path)
		}
		return doc
	}

	schema := read(".board/schemas/oci-conformance-profile.schema.json")
	if schema["$id"] != "https://aurumcode.dev/schemas/oci-conformance-profile.schema.json" {
		t.Fatalf("schema publishes the wrong $id: %v", schema["$id"])
	}
	if schema["additionalProperties"] != false {
		t.Fatal("schema does not close its property set")
	}

	profile := read(".board/oci/profiles/oci-conformance-v1.json")
	for key, want := range map[string]any{
		"schema":                  "aurum.oci-conformance-profile",
		"profile":                 "oci-conformance-v1",
		"lock":                    ".board/locks/oci/oci-conformance-v1.lock.json",
		"network":                 "none",
		"pull":                    "never",
		"user":                    "65534:65534",
		"cap_drop":                "ALL",
		"cap_add":                 "none",
		"mounts":                  "none",
		"devices":                 "none",
		"sockets":                 "none",
		"tmpfs":                   "rw,nosuid,nodev",
		"orchestrator":            "trusted",
		"orchestrator_root":       "/tmp/aurum-oci-orchestrator",
		"probe":                   "untrusted",
		"probe_root":              "/tmp/aurum-oci-probes",
		"probe_image_set":         "digest-pinned",
		"probe_output":            "untrusted-observation",
		"probe_verdict_authority": "denied",
		"verdict_authority":       "orchestrator",
		"report_mode":             "ephemeral",
		"report_root":             "/tmp/aurum-oci-reports",
		"unsupported_engine":      "denied",
		"engine_socket":           "denied",
		"engine_api":              "denied",
		"host_engine":             "denied",
		"egress":                  "denied",
		"host_checkout":           "denied",
		"host_filesystem":         "denied",
		"subprocess":              "denied",
	} {
		if profile[key] != want {
			t.Fatalf("profile key %s is %v, want %v", key, profile[key], want)
		}
	}
	for key, want := range map[string]float64{
		"timeout_seconds":       60,
		"memory_mb":             256,
		"cpu_millis":            1000,
		"pids_limit":            128,
		"tmpfs_mb":              128,
		"stdout_limit_bytes":    65536,
		"stderr_limit_bytes":    65536,
		"max_probes":            1024,
		"max_probe_bytes":       65536,
		"max_report_bytes":      1048576,
		"probe_timeout_seconds": 5,
		// Declared zero, and measured zero by the acceptance: this card publishes a
		// document and never reaches an engine.
		"engine_invocations": 0,
	} {
		if profile[key] != want {
			t.Fatalf("profile bound %s is %v, want %v", key, profile[key], want)
		}
	}
	for key, want := range map[string]bool{
		"checkout_readonly":      true,
		"read_only_rootfs":       true,
		"no_new_privileges":      true,
		"module_cache_read_only": true,
		"privileged":             false,
		// A cgo probe would link a native engine client into the plan. The published
		// bytes deny it, and neither locally present image carries an engine CLI at all.
		"probe_cgo": false,
	} {
		if profile[key] != want {
			t.Fatalf("profile flag %s is %v, want %v", key, profile[key], want)
		}
	}
	engines, ok := profile["supported_engines"].([]any)
	if !ok {
		t.Fatalf("profile does not publish an engine allowlist: %v", profile["supported_engines"])
	}
	if len(engines) != 2 || engines[0] != "docker" || engines[1] != "podman" {
		t.Fatalf("profile publishes an unexpected engine allowlist: %v", engines)
	}
	writable, ok := profile["probe_writable_roots"].([]any)
	if !ok {
		t.Fatalf("profile does not publish the probe's writable set: %v", profile["probe_writable_roots"])
	}
	if len(writable) != 1 || writable[0] != "/tmp/aurum-oci-probes" {
		t.Fatalf("profile widens the probe's writable set: %v", writable)
	}
	environment, ok := profile["environment"].(map[string]any)
	if !ok {
		t.Fatalf("profile does not publish an environment object: %v", profile["environment"])
	}
	// Every channel through which a container engine would be addressed is closed in the
	// published bytes as well, not only in the plan fields above.
	for key, want := range map[string]any{
		"CGO_ENABLED":                 "0",
		"AURUM_OCI_ORCHESTRATOR_ROOT": "/tmp/aurum-oci-orchestrator",
		"AURUM_OCI_PROBE_ROOT":        "/tmp/aurum-oci-probes",
		"AURUM_OCI_REPORT_ROOT":       "/tmp/aurum-oci-reports",
		"DOCKER_HOST":                 "",
		"CONTAINER_HOST":              "",
		"DOCKER_CONFIG":               "/dev/null",
		"DOCKER_API_VERSION":          "",
		"HTTP_PROXY":                  "",
		"HTTPS_PROXY":                 "",
		"NO_PROXY":                    "*",
	} {
		if environment[key] != want {
			t.Fatalf("environment %s is %v, want %v", key, environment[key], want)
		}
	}

	lock := read(".board/locks/oci/oci-conformance-v1.lock.json")
	if lock["schema"] != "aurum.oci-image-lock" || lock["profile"] != "oci-conformance-v1" {
		t.Fatalf("lock does not bind to oci-conformance-v1: %v", lock)
	}
	image, ok := lock["image"].(string)
	if !ok || len(image) == 0 {
		t.Fatal("lock does not publish an image")
	}
	if image != "aurum-bootstrap-go-bash@sha256:503ac356fa6bca4bad56fade87b5479ff371bd446ffb9c7db91f211323c7c73e" {
		t.Fatalf("lock publishes an unpinned or foreign image: %s", image)
	}

	registry := read(".board/oci/profiles/registry.v1.json")
	entries, ok := registry["profiles"].([]any)
	if !ok || len(entries) == 0 {
		t.Fatalf("registry does not publish a profile list: %v", registry["profiles"])
	}
	found := 0
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			t.Fatal("registry entry is not an object")
		}
		if entry["key"] != "oci-conformance-v1" {
			continue
		}
		found++
		if entry["schema"] != ".board/schemas/oci-conformance-profile.schema.json" ||
			entry["lock"] != ".board/locks/oci/oci-conformance-v1.lock.json" {
			t.Fatalf("registry entry points at foreign documents: %v", entry)
		}
		for _, field := range []string{"schema_digest", "lock_digest", "image_set_digest"} {
			value, ok := entry[field].(string)
			if !ok || !digestAUR410.MatchString(value) {
				t.Fatalf("registry entry field %s is not a sha256 digest: %v", field, entry[field])
			}
		}
	}
	if found != 1 {
		t.Fatalf("registry publishes oci-conformance-v1 %d times, want exactly 1", found)
	}
}
