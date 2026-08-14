package contracts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var digestAUR409 = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// ContractAUR409 pins the public contract of the fake-scm-offline-v1 entry: the three
// documents this card publishes exist, parse, and declare exactly the key, schema id
// and lock path the registry advertises, exactly once. It reads nothing else, runs no
// version-control command and starts no engine.
//
// The registry's total arity is deliberately not asserted here. This card owns the
// fake-scm-offline-v1 key, not the size of the registry; the exact registered key set is
// asserted by tests/unit/AUR-409.go, which this card also owns and which the next
// profile card must extend when it registers a tenth key.
func ContractAUR409(t *testing.T) {
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

	schema := read(".board/schemas/fake-scm-offline-profile.schema.json")
	if schema["$id"] != "https://aurumcode.dev/schemas/fake-scm-offline-profile.schema.json" {
		t.Fatalf("schema publishes the wrong $id: %v", schema["$id"])
	}
	if schema["additionalProperties"] != false {
		t.Fatal("schema does not close its property set")
	}

	profile := read(".board/oci/profiles/fake-scm-offline-v1.json")
	for key, want := range map[string]any{
		"schema":               "aurum.fake-scm-offline-profile",
		"profile":              "fake-scm-offline-v1",
		"lock":                 ".board/locks/oci/fake-scm-offline-v1.lock.json",
		"network":              "none",
		"pull":                 "never",
		"mounts":               "none",
		"devices":              "none",
		"sockets":              "none",
		"scm_backend":          "fake-local",
		"real_scm_binary":      "denied",
		"fake_scm":             "digest-pinned",
		"fake_scm_root":        "/tmp/aurum-fake-scm-engine",
		"event_set":            "digest-pinned",
		"event_root":           "/tmp/aurum-fake-scm-events",
		"response_set":         "digest-pinned",
		"repository_mode":      "ephemeral",
		"repository_root":      "/tmp/aurum-fake-scm-repos",
		"remote_origin":        "absent",
		"custom_transport":     "denied",
		"credential_helper":    "denied",
		"askpass":              "denied",
		"hooks":                "denied",
		"submodules":           "denied",
		"url_rewriting":        "denied",
		"publication":          "denied",
		"external_destination": "denied",
		"token":                "absent",
		"credential_sources":   "denied",
		"host_checkout":        "denied",
		"host_filesystem":      "denied",
		"subprocess":           "denied",
	} {
		if profile[key] != want {
			t.Fatalf("profile key %s is %v, want %v", key, profile[key], want)
		}
	}
	for key, want := range map[string]float64{
		"timeout_seconds":      60,
		"tmpfs_mb":             128,
		"scm_timeout_seconds":  5,
		"max_events":           1024,
		"max_event_bytes":      65536,
		"max_responses":        64,
		"max_response_bytes":   65536,
		"max_repository_bytes": 33554432,
		"stdout_limit_bytes":   65536,
		"stderr_limit_bytes":   65536,
	} {
		if profile[key] != want {
			t.Fatalf("profile bound %s is %v, want %v", key, profile[key], want)
		}
	}
	if profile["privileged"] != false {
		t.Fatalf("profile does not deny privileged execution: %v", profile["privileged"])
	}
	// A cgo fake SCM would link a native version-control library into the plan. The
	// published bytes deny it, and the pinned image carries no git, no git-upload-pack,
	// no git-receive-pack, no ssh and no curl, so the two agree.
	if profile["fake_scm_cgo"] != false {
		t.Fatalf("profile admits a cgo fake SCM: %v", profile["fake_scm_cgo"])
	}
	if profile["cap_drop"] != "ALL" || profile["cap_add"] != "none" {
		t.Fatalf("profile does not drop every capability: %v/%v", profile["cap_drop"], profile["cap_add"])
	}
	protocols, ok := profile["remote_protocols"].([]any)
	if !ok {
		t.Fatalf("profile does not publish a transport allowlist: %v", profile["remote_protocols"])
	}
	if len(protocols) != 1 || protocols[0] != "fake" {
		t.Fatalf("profile publishes an unexpected transport allowlist: %v", protocols)
	}
	environment, ok := profile["environment"].(map[string]any)
	if !ok {
		t.Fatalf("profile does not publish an environment object: %v", profile["environment"])
	}
	if environment["CGO_ENABLED"] != "0" {
		t.Fatalf("profile does not disable cgo: %v", environment["CGO_ENABLED"])
	}
	if environment["AURUM_FAKE_SCM_ROOT"] != "/tmp/aurum-fake-scm-engine" {
		t.Fatalf("profile points the fake engine off the bounded tmpfs: %v", environment["AURUM_FAKE_SCM_ROOT"])
	}
	if environment["AURUM_FAKE_SCM_EVENT_ROOT"] != "/tmp/aurum-fake-scm-events" {
		t.Fatalf("profile points the event log off the bounded tmpfs: %v", environment["AURUM_FAKE_SCM_EVENT_ROOT"])
	}
	if environment["AURUM_FAKE_SCM_REPOSITORY_ROOT"] != "/tmp/aurum-fake-scm-repos" {
		t.Fatalf("profile points the repositories off the bounded tmpfs: %v", environment["AURUM_FAKE_SCM_REPOSITORY_ROOT"])
	}
	// Every configuration channel a version-control client could read a credential
	// helper, an askpass program, an `insteadOf` rewrite or an `ext::` transport from is
	// closed in the published bytes as well, not only in the plan fields above.
	for key, want := range map[string]any{
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_CONFIG_GLOBAL":   "/dev/null",
		"GIT_CONFIG_SYSTEM":   "/dev/null",
		"GIT_TERMINAL_PROMPT": "0",
		"GIT_ASKPASS":         "/bin/false",
		"GIT_ALLOW_PROTOCOL":  "none",
	} {
		if environment[key] != want {
			t.Fatalf("environment %s is %v, want %v", key, environment[key], want)
		}
	}

	lock := read(".board/locks/oci/fake-scm-offline-v1.lock.json")
	if lock["schema"] != "aurum.oci-image-lock" || lock["profile"] != "fake-scm-offline-v1" {
		t.Fatalf("lock does not bind to fake-scm-offline-v1: %v", lock)
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
		if entry["key"] != "fake-scm-offline-v1" {
			continue
		}
		found++
		if entry["schema"] != ".board/schemas/fake-scm-offline-profile.schema.json" ||
			entry["lock"] != ".board/locks/oci/fake-scm-offline-v1.lock.json" {
			t.Fatalf("registry entry points at foreign documents: %v", entry)
		}
		for _, field := range []string{"schema_digest", "lock_digest", "image_set_digest"} {
			value, ok := entry[field].(string)
			if !ok || !digestAUR409.MatchString(value) {
				t.Fatalf("registry entry field %s is not a sha256 digest: %v", field, entry[field])
			}
		}
	}
	if found != 1 {
		t.Fatalf("registry publishes fake-scm-offline-v1 %d times, want exactly 1", found)
	}
}
