package contracts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var digestAUR407 = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// ContractAUR407 pins the public contract of the sqlite-offline-v1 entry: the three
// documents this card publishes exist, parse, and declare exactly the key, schema id
// and lock path the registry advertises, exactly once. It reads nothing else, opens no
// database and starts no engine.
//
// The registry's total arity is deliberately not asserted here. This card owns the
// sqlite-offline-v1 key, not the size of the registry; the exact registered key set is
// asserted by tests/unit/AUR-407.go, which this card also owns and which the next
// profile card must extend when it registers an eighth key.
func ContractAUR407(t *testing.T) {
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

	schema := read(".board/schemas/sqlite-offline-profile.schema.json")
	if schema["$id"] != "https://aurumcode.dev/schemas/sqlite-offline-profile.schema.json" {
		t.Fatalf("schema publishes the wrong $id: %v", schema["$id"])
	}
	if schema["additionalProperties"] != false {
		t.Fatal("schema does not close its property set")
	}

	profile := read(".board/oci/profiles/sqlite-offline-v1.json")
	for key, want := range map[string]any{
		"schema":               "aurum.sqlite-offline-profile",
		"profile":              "sqlite-offline-v1",
		"lock":                 ".board/locks/oci/sqlite-offline-v1.lock.json",
		"network":              "none",
		"pull":                 "never",
		"mounts":               "none",
		"devices":              "none",
		"sockets":              "none",
		"sqlite_driver":        "digest-pinned",
		"database_mode":        "ephemeral",
		"database_root":        "/tmp/aurum-sqlite-state",
		"host_database":        "denied",
		"extension_loading":    "denied",
		"remote_uri":           "denied",
		"implicit_persistence": "denied",
		"attach_database":      "denied",
		"journal_mode":         "memory",
	} {
		if profile[key] != want {
			t.Fatalf("profile key %s is %v, want %v", key, profile[key], want)
		}
	}
	for key, want := range map[string]float64{
		"timeout_seconds":       60,
		"tmpfs_mb":              128,
		"query_timeout_seconds": 5,
		"busy_timeout_ms":       2000,
		"max_database_bytes":    33554432,
		"max_open_connections":  4,
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
	// A cgo driver would link a native SQLite library into the plan. The published
	// bytes deny it, and the pinned image carries neither that library nor a C
	// compiler, so the two agree.
	if profile["sqlite_driver_cgo"] != false {
		t.Fatalf("profile admits a cgo SQLite driver: %v", profile["sqlite_driver_cgo"])
	}
	if profile["cap_drop"] != "ALL" || profile["cap_add"] != "none" {
		t.Fatalf("profile does not drop every capability: %v/%v", profile["cap_drop"], profile["cap_add"])
	}
	environment, ok := profile["environment"].(map[string]any)
	if !ok {
		t.Fatalf("profile does not publish an environment object: %v", profile["environment"])
	}
	if environment["CGO_ENABLED"] != "0" {
		t.Fatalf("profile does not disable cgo: %v", environment["CGO_ENABLED"])
	}
	if environment["AURUM_SQLITE_DATABASE_ROOT"] != "/tmp/aurum-sqlite-state" {
		t.Fatalf("profile points the database off the bounded tmpfs: %v", environment["AURUM_SQLITE_DATABASE_ROOT"])
	}
	if environment["SQLITE_TMPDIR"] != "/tmp/aurum-sqlite-state" {
		t.Fatalf("profile points SQLite temporary storage off the bounded tmpfs: %v", environment["SQLITE_TMPDIR"])
	}

	lock := read(".board/locks/oci/sqlite-offline-v1.lock.json")
	if lock["schema"] != "aurum.oci-image-lock" || lock["profile"] != "sqlite-offline-v1" {
		t.Fatalf("lock does not bind to sqlite-offline-v1: %v", lock)
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
		if entry["key"] != "sqlite-offline-v1" {
			continue
		}
		found++
		if entry["schema"] != ".board/schemas/sqlite-offline-profile.schema.json" ||
			entry["lock"] != ".board/locks/oci/sqlite-offline-v1.lock.json" {
			t.Fatalf("registry entry points at foreign documents: %v", entry)
		}
		for _, field := range []string{"schema_digest", "lock_digest", "image_set_digest"} {
			value, ok := entry[field].(string)
			if !ok || !digestAUR407.MatchString(value) {
				t.Fatalf("registry entry field %s is not a sha256 digest: %v", field, entry[field])
			}
		}
	}
	if found != 1 {
		t.Fatalf("registry publishes sqlite-offline-v1 %d times, want exactly 1", found)
	}
}
