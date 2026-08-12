package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type registryEntryAUR406 struct {
	Key            string `json:"key"`
	Schema         string `json:"schema"`
	SchemaDigest   string `json:"schema_digest"`
	Lock           string `json:"lock"`
	LockDigest     string `json:"lock_digest"`
	ImageSetDigest string `json:"image_set_digest"`
}

func digestAUR406(b []byte) string {
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}

// IntegrationAUR406 crosses the registry entry with the bytes it claims to bind:
// schema digest, lock digest and image-set digest must all be re-derivable from the
// documents on disk, and the profile document must point at the same lock the
// registry points at. A registry that advertises a digest it cannot reproduce fails
// here before any consumer can act on the plan.
func IntegrationAUR406(t *testing.T) {
	root := os.Getenv("AURUMCODE_ROOT")
	if root == "" {
		t.Fatal("AURUMCODE_ROOT is required")
	}
	read := func(path string) []byte {
		b, err := os.ReadFile(filepath.Join(root, path))
		if err != nil || len(b) == 0 {
			t.Fatalf("unreadable %s", path)
		}
		return b
	}

	registryBytes := read(".board/oci/profiles/registry.v1.json")
	var registry struct {
		Profiles []registryEntryAUR406 `json:"profiles"`
	}
	if err := json.Unmarshal(registryBytes, &registry); err != nil {
		t.Fatalf("registry is not decodable: %v", err)
	}

	var entry *registryEntryAUR406
	for i := range registry.Profiles {
		if registry.Profiles[i].Key == "parser-worker-v1" {
			entry = &registry.Profiles[i]
		}
	}
	if entry == nil {
		t.Fatal("registry has no parser-worker-v1 entry")
	}

	schemaBytes := read(entry.Schema)
	if got := digestAUR406(schemaBytes); got != entry.SchemaDigest {
		t.Fatalf("schema digest mismatch: registry %s, bytes %s", entry.SchemaDigest, got)
	}
	lockBytes := read(entry.Lock)
	if got := digestAUR406(lockBytes); got != entry.LockDigest {
		t.Fatalf("lock digest mismatch: registry %s, bytes %s", entry.LockDigest, got)
	}

	var lock struct {
		Profile string `json:"profile"`
		Image   string `json:"image"`
	}
	if err := json.Unmarshal(lockBytes, &lock); err != nil {
		t.Fatalf("lock is not decodable: %v", err)
	}
	if lock.Profile != "parser-worker-v1" {
		t.Fatalf("lock binds to %s", lock.Profile)
	}
	if got := digestAUR406([]byte(lock.Image)); got != entry.ImageSetDigest {
		t.Fatalf("image set digest mismatch: registry %s, image %s", entry.ImageSetDigest, got)
	}

	profileBytes := read(".board/oci/profiles/parser-worker-v1.json")
	var profile struct {
		Lock           string            `json:"lock"`
		LockDigest     string            `json:"lock_digest"`
		WorkerRoot     string            `json:"worker_root"`
		GrammarSetRoot string            `json:"grammar_set_root"`
		Environment    map[string]string `json:"environment"`
	}
	if err := json.Unmarshal(profileBytes, &profile); err != nil {
		t.Fatalf("profile is not decodable: %v", err)
	}
	if profile.Lock != entry.Lock {
		t.Fatalf("profile points at %s while the registry points at %s", profile.Lock, entry.Lock)
	}
	if profile.LockDigest != entry.LockDigest {
		t.Fatalf("profile lock digest %s is not the registry lock digest %s", profile.LockDigest, entry.LockDigest)
	}
	// The environment roots and the declared roots are two spellings of the same two
	// directories. If they ever disagree, one of them reaches a filesystem the plan
	// never bounded.
	if profile.Environment["AURUM_PARSER_WORKER_ROOT"] != profile.WorkerRoot {
		t.Fatalf("AURUM_PARSER_WORKER_ROOT %q is not the declared worker root %q",
			profile.Environment["AURUM_PARSER_WORKER_ROOT"], profile.WorkerRoot)
	}
	if profile.Environment["AURUM_PARSER_GRAMMAR_ROOT"] != profile.GrammarSetRoot {
		t.Fatalf("AURUM_PARSER_GRAMMAR_ROOT %q is not the declared grammar root %q",
			profile.Environment["AURUM_PARSER_GRAMMAR_ROOT"], profile.GrammarSetRoot)
	}
}
