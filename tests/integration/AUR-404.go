package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type registryEntryAUR404 struct {
	Key            string `json:"key"`
	Schema         string `json:"schema"`
	SchemaDigest   string `json:"schema_digest"`
	Lock           string `json:"lock"`
	LockDigest     string `json:"lock_digest"`
	ImageSetDigest string `json:"image_set_digest"`
}

func digestAUR404(b []byte) string { h := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(h[:]) }

// IntegrationAUR404 crosses the registry entry with the bytes it claims to bind:
// schema digest, lock digest and image-set digest must all be re-derivable from the
// documents on disk. A registry that advertises a digest it cannot reproduce fails
// here before any consumer can act on the plan.
func IntegrationAUR404(t *testing.T) {
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
		Profiles []registryEntryAUR404 `json:"profiles"`
	}
	if err := json.Unmarshal(registryBytes, &registry); err != nil {
		t.Fatalf("registry is not decodable: %v", err)
	}

	var entry *registryEntryAUR404
	for i := range registry.Profiles {
		if registry.Profiles[i].Key == "go-git-offline-v1" {
			entry = &registry.Profiles[i]
		}
	}
	if entry == nil {
		t.Fatal("registry has no go-git-offline-v1 entry")
	}

	schemaBytes := read(entry.Schema)
	if got := digestAUR404(schemaBytes); got != entry.SchemaDigest {
		t.Fatalf("schema digest mismatch: registry %s, bytes %s", entry.SchemaDigest, got)
	}
	lockBytes := read(entry.Lock)
	if got := digestAUR404(lockBytes); got != entry.LockDigest {
		t.Fatalf("lock digest mismatch: registry %s, bytes %s", entry.LockDigest, got)
	}

	var lock struct {
		Profile string `json:"profile"`
		Image   string `json:"image"`
	}
	if err := json.Unmarshal(lockBytes, &lock); err != nil {
		t.Fatalf("lock is not decodable: %v", err)
	}
	if lock.Profile != "go-git-offline-v1" {
		t.Fatalf("lock binds to %s", lock.Profile)
	}
	if got := digestAUR404([]byte(lock.Image)); got != entry.ImageSetDigest {
		t.Fatalf("image set digest mismatch: registry %s, image %s", entry.ImageSetDigest, got)
	}

	profileBytes := read(".board/oci/profiles/go-git-offline-v1.json")
	var profile struct {
		Lock       string `json:"lock"`
		LockDigest string `json:"lock_digest"`
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
}
