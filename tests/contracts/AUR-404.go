package contracts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ContractAUR404 pins the public contract of the go-git-offline-v1 entry: the three
// documents this card publishes exist, parse, and declare exactly the key, schema id
// and lock path the registry advertises. It reads nothing else and starts no engine.
func ContractAUR404(t *testing.T) {
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

	schema := read(".board/schemas/go-git-offline-profile.schema.json")
	if schema["$id"] != "https://aurumcode.dev/schemas/go-git-offline-profile.schema.json" {
		t.Fatalf("schema publishes the wrong $id: %v", schema["$id"])
	}
	if schema["additionalProperties"] != false {
		t.Fatal("schema does not close its property set")
	}

	profile := read(".board/oci/profiles/go-git-offline-v1.json")
	for key, want := range map[string]any{
		"schema":             "aurum.go-git-offline-profile",
		"profile":            "go-git-offline-v1",
		"lock":               ".board/locks/oci/go-git-offline-v1.lock.json",
		"network":            "none",
		"pull":               "never",
		"host_checkout":      "denied",
		"credential_helpers": "denied",
		"hooks":              "denied",
		"signing":            "denied",
		"git_fixture":        "ephemeral",
	} {
		if profile[key] != want {
			t.Fatalf("profile key %s is %v, want %v", key, profile[key], want)
		}
	}

	lock := read(".board/locks/oci/go-git-offline-v1.lock.json")
	if lock["schema"] != "aurum.oci-image-lock" || lock["profile"] != "go-git-offline-v1" {
		t.Fatalf("lock does not bind to go-git-offline-v1: %v", lock)
	}
	image, ok := lock["image"].(string)
	if !ok || len(image) == 0 {
		t.Fatal("lock does not publish an image")
	}
	if image != "golang@sha256:4746d26432a9117a5f58e95cb9f954ddf0de128e9d5816886514199316e4a2fb" {
		t.Fatalf("lock publishes an unpinned or foreign image: %s", image)
	}

	registry := read(".board/oci/profiles/registry.v1.json")
	entries, ok := registry["profiles"].([]any)
	// Extended by AUR-405 (fake-provider-v1, fifth key) and then by AUR-406
	// (parser-worker-v1, sixth key). The arity assertion is kept, not relaxed: every
	// rejection this check made before still fails, only the registered count moved
	// with the registry.
	if !ok || len(entries) != 6 {
		t.Fatalf("registry does not publish exactly six profiles: %v", registry["profiles"])
	}
	found := false
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			t.Fatal("registry entry is not an object")
		}
		if entry["key"] == "go-git-offline-v1" {
			found = true
			if entry["schema"] != ".board/schemas/go-git-offline-profile.schema.json" ||
				entry["lock"] != ".board/locks/oci/go-git-offline-v1.lock.json" {
				t.Fatalf("registry entry points at foreign documents: %v", entry)
			}
		}
	}
	if !found {
		t.Fatal("registry does not publish go-git-offline-v1")
	}
}
