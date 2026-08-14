package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type registryEntryAUR408 struct {
	Key            string `json:"key"`
	Schema         string `json:"schema"`
	SchemaDigest   string `json:"schema_digest"`
	Lock           string `json:"lock"`
	LockDigest     string `json:"lock_digest"`
	ImageSetDigest string `json:"image_set_digest"`
}

func digestAUR408Bytes(b []byte) string {
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}

// IntegrationAUR408 crosses the registry entry with the bytes it claims to bind: schema
// digest, lock digest and image-set digest must all be re-derivable from the documents
// on disk, and the profile document must point at the same lock the registry points at.
// A registry that advertises a digest it cannot reproduce fails here before any consumer
// can render a document.
func IntegrationAUR408(t *testing.T) {
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
		Profiles []registryEntryAUR408 `json:"profiles"`
	}
	if err := json.Unmarshal(registryBytes, &registry); err != nil {
		t.Fatalf("registry is not decodable: %v", err)
	}

	var entry *registryEntryAUR408
	for i := range registry.Profiles {
		if registry.Profiles[i].Key == "docs-tool-offline-v1" {
			entry = &registry.Profiles[i]
		}
	}
	if entry == nil {
		t.Fatal("registry has no docs-tool-offline-v1 entry")
	}

	schemaBytes := read(entry.Schema)
	if got := digestAUR408Bytes(schemaBytes); got != entry.SchemaDigest {
		t.Fatalf("schema digest mismatch: registry %s, bytes %s", entry.SchemaDigest, got)
	}
	lockBytes := read(entry.Lock)
	if got := digestAUR408Bytes(lockBytes); got != entry.LockDigest {
		t.Fatalf("lock digest mismatch: registry %s, bytes %s", entry.LockDigest, got)
	}

	var lock struct {
		Profile string `json:"profile"`
		Image   string `json:"image"`
	}
	if err := json.Unmarshal(lockBytes, &lock); err != nil {
		t.Fatalf("lock is not decodable: %v", err)
	}
	if lock.Profile != "docs-tool-offline-v1" {
		t.Fatalf("lock binds to %s", lock.Profile)
	}
	if got := digestAUR408Bytes([]byte(lock.Image)); got != entry.ImageSetDigest {
		t.Fatalf("image set digest mismatch: registry %s, image %s", entry.ImageSetDigest, got)
	}

	profileBytes := read(".board/oci/profiles/docs-tool-offline-v1.json")
	var profile struct {
		Lock             string            `json:"lock"`
		LockDigest       string            `json:"lock_digest"`
		TmpfsMB          int               `json:"tmpfs_mb"`
		GeneratorRoot    string            `json:"generator_root"`
		FixtureRoot      string            `json:"fixture_root"`
		OutputRoot       string            `json:"output_root"`
		MaxDocuments     int               `json:"max_documents"`
		MaxDocumentBytes int               `json:"max_document_bytes"`
		MaxInputBytes    int               `json:"max_input_bytes"`
		MaxOutputBytes   int               `json:"max_output_bytes"`
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
	// The environment roots and the declared roots are two spellings of the same three
	// directories. If they ever disagree, one of them reaches a filesystem the plan never
	// bounded, and the output root is exactly where a rendered file would land.
	for _, pair := range []struct {
		variable string
		declared string
	}{
		{"AURUM_DOCS_GENERATOR_ROOT", profile.GeneratorRoot},
		{"AURUM_DOCS_FIXTURE_ROOT", profile.FixtureRoot},
		{"AURUM_DOCS_OUTPUT_ROOT", profile.OutputRoot},
	} {
		if profile.Environment[pair.variable] != pair.declared {
			t.Fatalf("%s %q is not the declared root %q", pair.variable, profile.Environment[pair.variable], pair.declared)
		}
	}
	// The three bounded roots must be distinct directories: a generator that writes into
	// its own fixture tree, or an output tree that shadows the pinned generator, would
	// make the pinned inputs mutable at render time.
	if profile.GeneratorRoot == profile.FixtureRoot ||
		profile.GeneratorRoot == profile.OutputRoot ||
		profile.FixtureRoot == profile.OutputRoot {
		t.Fatalf("profile declares overlapping roots: generator=%q fixtures=%q output=%q",
			profile.GeneratorRoot, profile.FixtureRoot, profile.OutputRoot)
	}
	// The rendered output has to fit inside the tmpfs that holds it; otherwise the plan
	// would have to spill outside its own bound.
	if profile.MaxOutputBytes <= 0 || profile.TmpfsMB <= 0 {
		t.Fatalf("profile declares an unbounded output: max_output_bytes=%d tmpfs_mb=%d",
			profile.MaxOutputBytes, profile.TmpfsMB)
	}
	if profile.MaxOutputBytes > profile.TmpfsMB*1024*1024 {
		t.Fatalf("max_output_bytes %d exceeds the %d MiB bounded tmpfs",
			profile.MaxOutputBytes, profile.TmpfsMB)
	}
	// And the declared corpus has to fit inside the declared input bound.
	if profile.MaxDocuments <= 0 || profile.MaxDocumentBytes <= 0 {
		t.Fatalf("profile declares an unbounded corpus: max_documents=%d max_document_bytes=%d",
			profile.MaxDocuments, profile.MaxDocumentBytes)
	}
	if profile.MaxDocuments*profile.MaxDocumentBytes > profile.MaxInputBytes {
		t.Fatalf("a corpus of %d documents of %d bytes exceeds the %d byte input bound",
			profile.MaxDocuments, profile.MaxDocumentBytes, profile.MaxInputBytes)
	}
}
