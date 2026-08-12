package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type registryEntryAUR409 struct {
	Key            string `json:"key"`
	Schema         string `json:"schema"`
	SchemaDigest   string `json:"schema_digest"`
	Lock           string `json:"lock"`
	LockDigest     string `json:"lock_digest"`
	ImageSetDigest string `json:"image_set_digest"`
}

func digestAUR409Bytes(b []byte) string {
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}

// IntegrationAUR409 crosses the registry entry with the bytes it claims to bind: schema
// digest, lock digest and image-set digest must all be re-derivable from the documents
// on disk, and the profile document must point at the same lock the registry points at.
// A registry that advertises a digest it cannot reproduce fails here before any consumer
// can resolve a repository.
func IntegrationAUR409(t *testing.T) {
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
		Profiles []registryEntryAUR409 `json:"profiles"`
	}
	if err := json.Unmarshal(registryBytes, &registry); err != nil {
		t.Fatalf("registry is not decodable: %v", err)
	}

	var entry *registryEntryAUR409
	for i := range registry.Profiles {
		if registry.Profiles[i].Key == "fake-scm-offline-v1" {
			entry = &registry.Profiles[i]
		}
	}
	if entry == nil {
		t.Fatal("registry has no fake-scm-offline-v1 entry")
	}

	schemaBytes := read(entry.Schema)
	if got := digestAUR409Bytes(schemaBytes); got != entry.SchemaDigest {
		t.Fatalf("schema digest mismatch: registry %s, bytes %s", entry.SchemaDigest, got)
	}
	lockBytes := read(entry.Lock)
	if got := digestAUR409Bytes(lockBytes); got != entry.LockDigest {
		t.Fatalf("lock digest mismatch: registry %s, bytes %s", entry.LockDigest, got)
	}

	var lock struct {
		Profile string `json:"profile"`
		Image   string `json:"image"`
	}
	if err := json.Unmarshal(lockBytes, &lock); err != nil {
		t.Fatalf("lock is not decodable: %v", err)
	}
	if lock.Profile != "fake-scm-offline-v1" {
		t.Fatalf("lock binds to %s", lock.Profile)
	}
	if got := digestAUR409Bytes([]byte(lock.Image)); got != entry.ImageSetDigest {
		t.Fatalf("image set digest mismatch: registry %s, image %s", entry.ImageSetDigest, got)
	}

	profileBytes := read(".board/oci/profiles/fake-scm-offline-v1.json")
	var profile struct {
		Lock               string            `json:"lock"`
		LockDigest         string            `json:"lock_digest"`
		TmpfsMB            int               `json:"tmpfs_mb"`
		FakeScmRoot        string            `json:"fake_scm_root"`
		EventRoot          string            `json:"event_root"`
		RepositoryRoot     string            `json:"repository_root"`
		RemoteOrigin       string            `json:"remote_origin"`
		RemoteProtocols    []string          `json:"remote_protocols"`
		MaxEvents          int               `json:"max_events"`
		MaxEventBytes      int               `json:"max_event_bytes"`
		MaxResponses       int               `json:"max_responses"`
		MaxResponseBytes   int               `json:"max_response_bytes"`
		MaxInputBytes      int               `json:"max_input_bytes"`
		MaxRepositoryBytes int               `json:"max_repository_bytes"`
		Environment        map[string]string `json:"environment"`
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
	// bounded, and the repository root is exactly where a simulated clone would land.
	for _, pair := range []struct {
		variable string
		declared string
	}{
		{"AURUM_FAKE_SCM_ROOT", profile.FakeScmRoot},
		{"AURUM_FAKE_SCM_EVENT_ROOT", profile.EventRoot},
		{"AURUM_FAKE_SCM_REPOSITORY_ROOT", profile.RepositoryRoot},
	} {
		if profile.Environment[pair.variable] != pair.declared {
			t.Fatalf("%s %q is not the declared root %q", pair.variable, profile.Environment[pair.variable], pair.declared)
		}
	}
	// The three bounded roots must be distinct directories, and none of them may contain
	// another: a fake engine that lived inside the simulated repository tree, or an event
	// log that sat under the engine, would make a pinned input writable at replay time.
	for _, pair := range [][2]string{
		{profile.FakeScmRoot, profile.EventRoot},
		{profile.FakeScmRoot, profile.RepositoryRoot},
		{profile.EventRoot, profile.RepositoryRoot},
	} {
		if pair[0] == pair[1] ||
			strings.HasPrefix(pair[0], pair[1]+"/") || strings.HasPrefix(pair[1], pair[0]+"/") {
			t.Fatalf("profile declares overlapping roots: engine=%q events=%q repositories=%q",
				profile.FakeScmRoot, profile.EventRoot, profile.RepositoryRoot)
		}
	}
	// There is no origin and no transport other than the in-process fake one, so no
	// declared value can reach a forge even if a consumer resolved it verbatim.
	if profile.RemoteOrigin != "absent" {
		t.Fatalf("profile declares a remote origin: %q", profile.RemoteOrigin)
	}
	if len(profile.RemoteProtocols) != 1 || profile.RemoteProtocols[0] != "fake" {
		t.Fatalf("profile declares reachable transports: %v", profile.RemoteProtocols)
	}
	// The simulated repositories have to fit inside the tmpfs that holds them; otherwise
	// the plan would have to spill outside its own bound.
	if profile.MaxRepositoryBytes <= 0 || profile.TmpfsMB <= 0 {
		t.Fatalf("profile declares an unbounded repository: max_repository_bytes=%d tmpfs_mb=%d",
			profile.MaxRepositoryBytes, profile.TmpfsMB)
	}
	if profile.MaxRepositoryBytes > profile.TmpfsMB*1024*1024 {
		t.Fatalf("max_repository_bytes %d exceeds the %d MiB bounded tmpfs",
			profile.MaxRepositoryBytes, profile.TmpfsMB)
	}
	// And the pinned event log and response set have to fit inside the declared input
	// bound, because both are replayed from materialized inputs.
	if profile.MaxEvents <= 0 || profile.MaxEventBytes <= 0 {
		t.Fatalf("profile declares an unbounded event log: max_events=%d max_event_bytes=%d",
			profile.MaxEvents, profile.MaxEventBytes)
	}
	if profile.MaxEvents*profile.MaxEventBytes > profile.MaxInputBytes {
		t.Fatalf("an event log of %d entries of %d bytes exceeds the %d byte input bound",
			profile.MaxEvents, profile.MaxEventBytes, profile.MaxInputBytes)
	}
	if profile.MaxResponses <= 0 || profile.MaxResponseBytes <= 0 {
		t.Fatalf("profile declares an unbounded response set: max_responses=%d max_response_bytes=%d",
			profile.MaxResponses, profile.MaxResponseBytes)
	}
	if profile.MaxResponses*profile.MaxResponseBytes > profile.MaxInputBytes {
		t.Fatalf("a response set of %d responses of %d bytes exceeds the %d byte input bound",
			profile.MaxResponses, profile.MaxResponseBytes, profile.MaxInputBytes)
	}
}
