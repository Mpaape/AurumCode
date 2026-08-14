package contracts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var digestAUR406 = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// ContractAUR406 pins the public contract of the parser-worker-v1 entry: the three
// documents this card publishes exist, parse, and declare exactly the key, schema id
// and lock path the registry advertises, exactly once. It reads nothing else and
// starts no engine.
//
// The registry's total arity is deliberately not asserted here. This card owns the
// parser-worker-v1 key, not the size of the registry; the exact registered key set is
// asserted by tests/unit/AUR-406.go, which this card also owns and which the next
// profile card must extend when it registers a seventh key.
func ContractAUR406(t *testing.T) {
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

	schema := read(".board/schemas/parser-worker-profile.schema.json")
	if schema["$id"] != "https://aurumcode.dev/schemas/parser-worker-profile.schema.json" {
		t.Fatalf("schema publishes the wrong $id: %v", schema["$id"])
	}
	if schema["additionalProperties"] != false {
		t.Fatal("schema does not close its property set")
	}

	profile := read(".board/oci/profiles/parser-worker-v1.json")
	for key, want := range map[string]any{
		"schema":           "aurum.parser-worker-profile",
		"profile":          "parser-worker-v1",
		"lock":             ".board/locks/oci/parser-worker-v1.lock.json",
		"network":          "none",
		"pull":             "never",
		"mounts":           "none",
		"devices":          "none",
		"sockets":          "none",
		"worker":           "digest-pinned",
		"worker_root":      "/tmp/aurum-parser-worker",
		"grammar_set":      "digest-pinned",
		"grammar_set_root": "/tmp/aurum-parser-grammars",
		"code_execution":   "denied",
		"host_filesystem":  "denied",
		"subprocess":       "denied",
		"blob_source":      "stdin",
	} {
		if profile[key] != want {
			t.Fatalf("profile key %s is %v, want %v", key, profile[key], want)
		}
	}
	for key, want := range map[string]float64{
		"timeout_seconds":       60,
		"parse_timeout_seconds": 5,
		"max_blob_bytes":        1048576,
		"max_blobs":             256,
		"stdout_limit_bytes":    65536,
		"stderr_limit_bytes":    65536,
	} {
		if profile[key] != want {
			t.Fatalf("profile bound %s is %v, want %v", key, profile[key], want)
		}
	}
	if profile["privileged"] != false {
		t.Fatalf("profile does not deny privileged execution: %v", profile["privileged"])
	}
	if profile["cap_drop"] != "ALL" || profile["cap_add"] != "none" {
		t.Fatalf("profile does not drop every capability: %v/%v", profile["cap_drop"], profile["cap_add"])
	}
	environment, ok := profile["environment"].(map[string]any)
	if !ok {
		t.Fatalf("profile does not publish an environment object: %v", profile["environment"])
	}
	if environment["AURUM_PARSER_WORKER_ROOT"] != "/tmp/aurum-parser-worker" {
		t.Fatalf("profile points the worker off the bounded tmpfs: %v", environment["AURUM_PARSER_WORKER_ROOT"])
	}
	if environment["AURUM_PARSER_GRAMMAR_ROOT"] != "/tmp/aurum-parser-grammars" {
		t.Fatalf("profile points the grammar set off the bounded tmpfs: %v", environment["AURUM_PARSER_GRAMMAR_ROOT"])
	}

	lock := read(".board/locks/oci/parser-worker-v1.lock.json")
	if lock["schema"] != "aurum.oci-image-lock" || lock["profile"] != "parser-worker-v1" {
		t.Fatalf("lock does not bind to parser-worker-v1: %v", lock)
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
		if entry["key"] != "parser-worker-v1" {
			continue
		}
		found++
		if entry["schema"] != ".board/schemas/parser-worker-profile.schema.json" ||
			entry["lock"] != ".board/locks/oci/parser-worker-v1.lock.json" {
			t.Fatalf("registry entry points at foreign documents: %v", entry)
		}
		for _, field := range []string{"schema_digest", "lock_digest", "image_set_digest"} {
			value, ok := entry[field].(string)
			if !ok || !digestAUR406.MatchString(value) {
				t.Fatalf("registry entry field %s is not a sha256 digest: %v", field, entry[field])
			}
		}
	}
	if found != 1 {
		t.Fatalf("registry publishes parser-worker-v1 %d times, want exactly 1", found)
	}
}
