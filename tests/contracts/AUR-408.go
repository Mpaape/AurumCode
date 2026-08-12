package contracts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var digestAUR408 = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// ContractAUR408 pins the public contract of the docs-tool-offline-v1 entry: the three
// documents this card publishes exist, parse, and declare exactly the key, schema id
// and lock path the registry advertises, exactly once. It reads nothing else, renders no
// document and starts no engine.
//
// The registry's total arity is deliberately not asserted here. This card owns the
// docs-tool-offline-v1 key, not the size of the registry; the exact registered key set is
// asserted by tests/unit/AUR-408.go, which this card also owns and which the next
// profile card must extend when it registers a ninth key.
func ContractAUR408(t *testing.T) {
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

	schema := read(".board/schemas/docs-tool-offline-profile.schema.json")
	if schema["$id"] != "https://aurumcode.dev/schemas/docs-tool-offline-profile.schema.json" {
		t.Fatalf("schema publishes the wrong $id: %v", schema["$id"])
	}
	if schema["additionalProperties"] != false {
		t.Fatal("schema does not close its property set")
	}

	profile := read(".board/oci/profiles/docs-tool-offline-v1.json")
	for key, want := range map[string]any{
		"schema":            "aurum.docs-tool-offline-profile",
		"profile":           "docs-tool-offline-v1",
		"lock":              ".board/locks/oci/docs-tool-offline-v1.lock.json",
		"network":           "none",
		"pull":              "never",
		"mounts":            "none",
		"devices":           "none",
		"sockets":           "none",
		"generator":         "digest-pinned",
		"renderer":          "digest-pinned",
		"plugin_set":        "digest-pinned",
		"native_site_tool":  "denied",
		"dynamic_plugin":    "denied",
		"remote_fetch":      "denied",
		"snippet_execution": "denied",
		"fixture_source":    "local",
		"fixture_root":      "/tmp/aurum-docs-fixtures",
		"generator_root":    "/tmp/aurum-docs-tool",
		"output_mode":       "ephemeral",
		"output_root":       "/tmp/aurum-docs-output",
		"host_output":       "denied",
		"host_filesystem":   "denied",
		"subprocess":        "denied",
	} {
		if profile[key] != want {
			t.Fatalf("profile key %s is %v, want %v", key, profile[key], want)
		}
	}
	for key, want := range map[string]float64{
		"timeout_seconds":        60,
		"tmpfs_mb":               128,
		"render_timeout_seconds": 5,
		"max_documents":          64,
		"max_document_bytes":     1048576,
		"max_output_bytes":       33554432,
		"stdout_limit_bytes":     65536,
		"stderr_limit_bytes":     65536,
	} {
		if profile[key] != want {
			t.Fatalf("profile bound %s is %v, want %v", key, profile[key], want)
		}
	}
	if profile["privileged"] != false {
		t.Fatalf("profile does not deny privileged execution: %v", profile["privileged"])
	}
	// A cgo generator would link a native site tool into the plan. The published bytes
	// deny it, and the pinned image carries no pandoc, mkdocs, jekyll, hugo,
	// asciidoctor, ruby, python or C compiler, so the two agree.
	if profile["generator_cgo"] != false {
		t.Fatalf("profile admits a cgo documentation generator: %v", profile["generator_cgo"])
	}
	if profile["cap_drop"] != "ALL" || profile["cap_add"] != "none" {
		t.Fatalf("profile does not drop every capability: %v/%v", profile["cap_drop"], profile["cap_add"])
	}
	languages, ok := profile["snippet_languages"].([]any)
	if !ok {
		t.Fatalf("profile does not publish a snippet allowlist: %v", profile["snippet_languages"])
	}
	if len(languages) != 3 || languages[0] != "bash" || languages[1] != "go" || languages[2] != "json" {
		t.Fatalf("profile publishes an unexpected snippet allowlist: %v", languages)
	}
	environment, ok := profile["environment"].(map[string]any)
	if !ok {
		t.Fatalf("profile does not publish an environment object: %v", profile["environment"])
	}
	if environment["CGO_ENABLED"] != "0" {
		t.Fatalf("profile does not disable cgo: %v", environment["CGO_ENABLED"])
	}
	if environment["AURUM_DOCS_OUTPUT_ROOT"] != "/tmp/aurum-docs-output" {
		t.Fatalf("profile points the rendered output off the bounded tmpfs: %v", environment["AURUM_DOCS_OUTPUT_ROOT"])
	}
	if environment["AURUM_DOCS_FIXTURE_ROOT"] != "/tmp/aurum-docs-fixtures" {
		t.Fatalf("profile points the fixtures off the bounded tmpfs: %v", environment["AURUM_DOCS_FIXTURE_ROOT"])
	}
	if environment["AURUM_DOCS_GENERATOR_ROOT"] != "/tmp/aurum-docs-tool" {
		t.Fatalf("profile points the generator off the bounded tmpfs: %v", environment["AURUM_DOCS_GENERATOR_ROOT"])
	}

	lock := read(".board/locks/oci/docs-tool-offline-v1.lock.json")
	if lock["schema"] != "aurum.oci-image-lock" || lock["profile"] != "docs-tool-offline-v1" {
		t.Fatalf("lock does not bind to docs-tool-offline-v1: %v", lock)
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
		if entry["key"] != "docs-tool-offline-v1" {
			continue
		}
		found++
		if entry["schema"] != ".board/schemas/docs-tool-offline-profile.schema.json" ||
			entry["lock"] != ".board/locks/oci/docs-tool-offline-v1.lock.json" {
			t.Fatalf("registry entry points at foreign documents: %v", entry)
		}
		for _, field := range []string{"schema_digest", "lock_digest", "image_set_digest"} {
			value, ok := entry[field].(string)
			if !ok || !digestAUR408.MatchString(value) {
				t.Fatalf("registry entry field %s is not a sha256 digest: %v", field, entry[field])
			}
		}
	}
	if found != 1 {
		t.Fatalf("registry publishes docs-tool-offline-v1 %d times, want exactly 1", found)
	}
}
