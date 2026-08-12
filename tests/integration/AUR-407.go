package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type registryEntryAUR407 struct {
	Key            string `json:"key"`
	Schema         string `json:"schema"`
	SchemaDigest   string `json:"schema_digest"`
	Lock           string `json:"lock"`
	LockDigest     string `json:"lock_digest"`
	ImageSetDigest string `json:"image_set_digest"`
}

func digestAUR407(b []byte) string {
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}

// IntegrationAUR407 crosses the registry entry with the bytes it claims to bind: schema
// digest, lock digest and image-set digest must all be re-derivable from the documents
// on disk, and the profile document must point at the same lock the registry points at.
// A registry that advertises a digest it cannot reproduce fails here before any consumer
// can open the state store.
func IntegrationAUR407(t *testing.T) {
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
		Profiles []registryEntryAUR407 `json:"profiles"`
	}
	if err := json.Unmarshal(registryBytes, &registry); err != nil {
		t.Fatalf("registry is not decodable: %v", err)
	}

	var entry *registryEntryAUR407
	for i := range registry.Profiles {
		if registry.Profiles[i].Key == "sqlite-offline-v1" {
			entry = &registry.Profiles[i]
		}
	}
	if entry == nil {
		t.Fatal("registry has no sqlite-offline-v1 entry")
	}

	schemaBytes := read(entry.Schema)
	if got := digestAUR407(schemaBytes); got != entry.SchemaDigest {
		t.Fatalf("schema digest mismatch: registry %s, bytes %s", entry.SchemaDigest, got)
	}
	lockBytes := read(entry.Lock)
	if got := digestAUR407(lockBytes); got != entry.LockDigest {
		t.Fatalf("lock digest mismatch: registry %s, bytes %s", entry.LockDigest, got)
	}

	var lock struct {
		Profile string `json:"profile"`
		Image   string `json:"image"`
	}
	if err := json.Unmarshal(lockBytes, &lock); err != nil {
		t.Fatalf("lock is not decodable: %v", err)
	}
	if lock.Profile != "sqlite-offline-v1" {
		t.Fatalf("lock binds to %s", lock.Profile)
	}
	if got := digestAUR407([]byte(lock.Image)); got != entry.ImageSetDigest {
		t.Fatalf("image set digest mismatch: registry %s, image %s", entry.ImageSetDigest, got)
	}

	profileBytes := read(".board/oci/profiles/sqlite-offline-v1.json")
	var profile struct {
		Lock             string            `json:"lock"`
		LockDigest       string            `json:"lock_digest"`
		TmpfsMB          int               `json:"tmpfs_mb"`
		DatabaseRoot     string            `json:"database_root"`
		MaxDatabaseBytes int               `json:"max_database_bytes"`
		Environment      map[string]string `json:"environment"`
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
	// The environment roots and the declared root are three spellings of the same
	// directory. If they ever disagree, one of them reaches a filesystem the plan never
	// bounded, and SQLITE_TMPDIR is exactly where an implicit spill file would land.
	if profile.Environment["AURUM_SQLITE_DATABASE_ROOT"] != profile.DatabaseRoot {
		t.Fatalf("AURUM_SQLITE_DATABASE_ROOT %q is not the declared database root %q",
			profile.Environment["AURUM_SQLITE_DATABASE_ROOT"], profile.DatabaseRoot)
	}
	if profile.Environment["SQLITE_TMPDIR"] != profile.DatabaseRoot {
		t.Fatalf("SQLITE_TMPDIR %q is not the declared database root %q",
			profile.Environment["SQLITE_TMPDIR"], profile.DatabaseRoot)
	}
	// The ephemeral store has to fit inside the tmpfs that holds it; otherwise the plan
	// would have to spill outside its own bound.
	if profile.MaxDatabaseBytes <= 0 || profile.TmpfsMB <= 0 {
		t.Fatalf("profile declares an unbounded store: max_database_bytes=%d tmpfs_mb=%d",
			profile.MaxDatabaseBytes, profile.TmpfsMB)
	}
	if profile.MaxDatabaseBytes > profile.TmpfsMB*1024*1024 {
		t.Fatalf("max_database_bytes %d exceeds the %d MiB bounded tmpfs",
			profile.MaxDatabaseBytes, profile.TmpfsMB)
	}
}
