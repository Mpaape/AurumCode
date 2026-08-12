package contracts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var digestAUR405 = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// ContractAUR405 pins the public contract of the fake-provider-v1 entry: the three
// documents this card publishes exist, parse, and declare exactly the key, schema id
// and lock path the registry advertises, exactly once. It reads nothing else and
// starts no engine.
//
// The registry's total arity is deliberately not asserted here. This card owns the
// fake-provider-v1 key, not the size of the registry; the exact registered key set is
// asserted by tests/unit/AUR-405.go, which this card also owns and which the next
// profile card must extend when it registers a sixth key.
func ContractAUR405(t *testing.T) {
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

	schema := read(".board/schemas/fake-provider-profile.schema.json")
	if schema["$id"] != "https://aurumcode.dev/schemas/fake-provider-profile.schema.json" {
		t.Fatalf("schema publishes the wrong $id: %v", schema["$id"])
	}
	if schema["additionalProperties"] != false {
		t.Fatal("schema does not close its property set")
	}

	profile := read(".board/oci/profiles/fake-provider-v1.json")
	for key, want := range map[string]any{
		"schema":                "aurum.fake-provider-profile",
		"profile":               "fake-provider-v1",
		"lock":                  ".board/locks/oci/fake-provider-v1.lock.json",
		"network":               "none",
		"pull":                  "never",
		"provider_endpoint":     "http://127.0.0.1:8080/v1",
		"dns":                   "denied",
		"egress":                "denied",
		"api_key":               "absent",
		"credential_sources":    "denied",
		"response_scripts":      "digest-pinned",
		"response_scripts_root": "/tmp/aurum-fake-provider",
	} {
		if profile[key] != want {
			t.Fatalf("profile key %s is %v, want %v", key, profile[key], want)
		}
	}
	for key, want := range map[string]float64{
		"timeout_seconds":         60,
		"request_timeout_seconds": 5,
		"max_response_bytes":      65536,
		"max_responses":           64,
		"stdout_limit_bytes":      65536,
		"stderr_limit_bytes":      65536,
	} {
		if profile[key] != want {
			t.Fatalf("profile bound %s is %v, want %v", key, profile[key], want)
		}
	}
	environment, ok := profile["environment"].(map[string]any)
	if !ok {
		t.Fatalf("profile does not publish an environment object: %v", profile["environment"])
	}
	if environment["OPENAI_API_KEY"] != "" {
		t.Fatal("profile publishes a non-empty OPENAI_API_KEY")
	}
	if environment["OPENAI_BASE_URL"] != "http://127.0.0.1:8080/v1" {
		t.Fatalf("profile points the adapter off the pinned local endpoint: %v", environment["OPENAI_BASE_URL"])
	}

	lock := read(".board/locks/oci/fake-provider-v1.lock.json")
	if lock["schema"] != "aurum.oci-image-lock" || lock["profile"] != "fake-provider-v1" {
		t.Fatalf("lock does not bind to fake-provider-v1: %v", lock)
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
		if entry["key"] != "fake-provider-v1" {
			continue
		}
		found++
		if entry["schema"] != ".board/schemas/fake-provider-profile.schema.json" ||
			entry["lock"] != ".board/locks/oci/fake-provider-v1.lock.json" {
			t.Fatalf("registry entry points at foreign documents: %v", entry)
		}
		for _, field := range []string{"schema_digest", "lock_digest", "image_set_digest"} {
			value, ok := entry[field].(string)
			if !ok || !digestAUR405.MatchString(value) {
				t.Fatalf("registry entry field %s is not a sha256 digest: %v", field, entry[field])
			}
		}
	}
	if found != 1 {
		t.Fatalf("registry publishes fake-provider-v1 %d times, want exactly 1", found)
	}
}
