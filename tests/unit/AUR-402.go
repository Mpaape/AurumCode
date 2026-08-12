package unit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

type registryAUR402 struct {
	Schema   string        `json:"schema"`
	Version  int           `json:"version"`
	Profiles []entryAUR402 `json:"profiles"`
}
type entryAUR402 struct {
	Key            string `json:"key"`
	Schema         string `json:"schema"`
	SchemaDigest   string `json:"schema_digest"`
	Lock           string `json:"lock"`
	LockDigest     string `json:"lock_digest"`
	ImageSetDigest string `json:"image_set_digest"`
}
type lockAUR402 struct {
	Schema  string `json:"schema"`
	Version int    `json:"version"`
	Profile string `json:"profile"`
	Image   string `json:"image"`
}

var digestAUR402 = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var imageAUR402 = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]*@sha256:[0-9a-f]{64}$`)
var approvedImageAUR402 = regexp.MustCompile(`^aurum-bootstrap-go-bash@sha256:[0-9a-f]{64}$`)
var keyAUR402 = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]+$`)

func shaAUR402(b []byte) string { h := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(h[:]) }
func decodeSingleAUR402(data []byte, value any) bool {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(value) != nil {
		return false
	}
	var trailing any
	return d.Decode(&trailing) == io.EOF
}

func readAUR402(root, p string) ([]byte, string) {
	if (p != ".board/schemas/container-profile.schema.json" && p != ".board/locks/oci/registry-v1.lock.json") || strings.Contains(p, "..") || strings.Contains(p, "//") {
		return nil, "lock-path-invalid"
	}
	info, e := os.Lstat(filepath.Join(root, p))
	if e != nil || !info.Mode().IsRegular() {
		return nil, "lock-missing"
	}
	b, e := os.ReadFile(filepath.Join(root, p))
	if e != nil {
		return nil, "lock-missing"
	}
	return b, ""
}

// ValidateRegistryAUR402 is the canonical, side-effect-free registry loader used by every layer.
func ValidateRegistryAUR402(root string, document []byte) (string, string) {
	var r registryAUR402
	if !decodeSingleAUR402(document, &r) {
		return "", "schema-invalid"
	}
	if r.Schema != "aurum.profile-registry" || r.Version != 1 || len(r.Profiles) < 2 {
		return "", "schema-invalid"
	}
	keys := make([]string, len(r.Profiles))
	seen := map[string]bool{}
	for i, e := range r.Profiles {
		if !keyAUR402.MatchString(e.Key) {
			return "", "profile-unregistered"
		}
		if seen[e.Key] {
			return "", "duplicate-profile"
		}
		seen[e.Key] = true
		keys[i] = e.Key
		if i > 0 && keys[i-1] >= e.Key {
			return "", "order-invalid"
		}
		if e.Key != "bootstrap-readonly-v1" && e.Key != "registry-v1" && e.Key != "go-unit-offline-v1" && e.Key != "go-git-offline-v1" && e.Key != "fake-provider-v1" && e.Key != "parser-worker-v1" && e.Key != "sqlite-offline-v1" && e.Key != "docs-tool-offline-v1" {
			return "", "profile-unregistered"
		}
		// Registered by AUR-408, under the same neighbour rule as the state store
		// below: the documentation tool's own documents are validated by that card's
		// loader, so this loader checks only the declared paths and digest shape and
		// never reads a file outside its own materialized allowlist.
		if e.Key == "docs-tool-offline-v1" {
			if e.Schema != ".board/schemas/docs-tool-offline-profile.schema.json" || e.Lock != ".board/locks/oci/docs-tool-offline-v1.lock.json" {
				return "", "unsafe-plan"
			}
			if !digestAUR402.MatchString(e.SchemaDigest) || !digestAUR402.MatchString(e.LockDigest) || !digestAUR402.MatchString(e.ImageSetDigest) {
				return "", "digest-invalid"
			}
			continue
		}
		// Registered by AUR-407, under the same neighbour rule as the parser worker
		// below: the state store's own documents are validated by that card's loader, so
		// this loader checks only the declared paths and digest shape and never reads a
		// file outside its own materialized allowlist.
		if e.Key == "sqlite-offline-v1" {
			if e.Schema != ".board/schemas/sqlite-offline-profile.schema.json" || e.Lock != ".board/locks/oci/sqlite-offline-v1.lock.json" {
				return "", "unsafe-plan"
			}
			if !digestAUR402.MatchString(e.SchemaDigest) || !digestAUR402.MatchString(e.LockDigest) || !digestAUR402.MatchString(e.ImageSetDigest) {
				return "", "digest-invalid"
			}
			continue
		}
		// Registered by AUR-406, under the same neighbour rule as the fake provider and
		// the Git plan below: the parser worker's own documents are validated by that
		// card's loader, so this loader checks only the declared paths and digest shape
		// and never reads a file outside its own materialized allowlist.
		if e.Key == "parser-worker-v1" {
			if e.Schema != ".board/schemas/parser-worker-profile.schema.json" || e.Lock != ".board/locks/oci/parser-worker-v1.lock.json" {
				return "", "unsafe-plan"
			}
			if !digestAUR402.MatchString(e.SchemaDigest) || !digestAUR402.MatchString(e.LockDigest) || !digestAUR402.MatchString(e.ImageSetDigest) {
				return "", "digest-invalid"
			}
			continue
		}
		// Registered by AUR-405, under the same neighbour rule as the Git plan below:
		// the fake provider's own documents are validated by that card's loader, so
		// this loader checks only the declared paths and digest shape and never reads
		// a file outside its own materialized allowlist.
		if e.Key == "fake-provider-v1" {
			if e.Schema != ".board/schemas/fake-provider-profile.schema.json" || e.Lock != ".board/locks/oci/fake-provider-v1.lock.json" {
				return "", "unsafe-plan"
			}
			if !digestAUR402.MatchString(e.SchemaDigest) || !digestAUR402.MatchString(e.LockDigest) || !digestAUR402.MatchString(e.ImageSetDigest) {
				return "", "digest-invalid"
			}
			continue
		}
		// Registered by AUR-404. The Git plan's own documents are validated by that
		// card's loader; here only the declared paths and digest shape are checked, so
		// this loader never depends on files outside its own materialized allowlist.
		if e.Key == "go-git-offline-v1" {
			if e.Schema != ".board/schemas/go-git-offline-profile.schema.json" || e.Lock != ".board/locks/oci/go-git-offline-v1.lock.json" {
				return "", "unsafe-plan"
			}
			if !digestAUR402.MatchString(e.SchemaDigest) || !digestAUR402.MatchString(e.LockDigest) || !digestAUR402.MatchString(e.ImageSetDigest) {
				return "", "digest-invalid"
			}
			continue
		}
		if e.Key == "go-unit-offline-v1" {
			if e.Schema != ".board/schemas/go-unit-offline-profile.schema.json" || e.Lock != ".board/locks/oci/go-unit-offline-v1.lock.json" {
				return "", "unsafe-plan"
			}
			if !digestAUR402.MatchString(e.SchemaDigest) || !digestAUR402.MatchString(e.LockDigest) || !digestAUR402.MatchString(e.ImageSetDigest) {
				return "", "digest-invalid"
			}
			continue
		}
		if e.Schema != ".board/schemas/container-profile.schema.json" || strings.Contains(e.Schema, "..") || strings.Contains(e.Schema, "//") || e.Lock != ".board/locks/oci/registry-v1.lock.json" || strings.Contains(e.Lock, "..") || strings.Contains(e.Lock, "//") {
			return "", "unsafe-plan"
		}
		if !digestAUR402.MatchString(e.SchemaDigest) || !digestAUR402.MatchString(e.LockDigest) || !digestAUR402.MatchString(e.ImageSetDigest) {
			return "", "digest-invalid"
		}
		schema, se := readAUR402(root, e.Schema)
		if se != "" || shaAUR402(schema) != e.SchemaDigest {
			return "", "schema-digest-mismatch"
		}
		lock, le := readAUR402(root, e.Lock)
		if le != "" {
			return "", "lock-missing"
		}
		if shaAUR402(lock) != e.LockDigest {
			return "", "lock-digest-mismatch"
		}
		var l lockAUR402
		if !decodeSingleAUR402(lock, &l) || l.Schema != "aurum.oci-image-lock" || l.Version != 1 || l.Profile != "registry-v1" || !imageAUR402.MatchString(l.Image) {
			return "", "mutable-input"
		}
		if !approvedImageAUR402.MatchString(l.Image) {
			return "", "unsafe-plan"
		}
		if shaAUR402([]byte(l.Image)) != e.ImageSetDigest {
			return "", "unsafe-plan"
		}
	}
	if !sort.StringsAreSorted(keys) {
		return "", "order-invalid"
	}
	return shaAUR402(document), "valid"
}

func TestAUR402(t *testing.T) {
	root := os.Getenv("AURUMCODE_ROOT")
	if root == "" {
		t.Fatal("AURUMCODE_ROOT is required")
	}
	b, e := os.ReadFile(filepath.Join(root, ".board/oci/profiles/registry.v1.json"))
	if e != nil {
		t.Fatal(e)
	}
	digest, code := ValidateRegistryAUR402(root, b)
	if expected := os.Getenv("AURUM_A402_EXPECT_CODE"); expected != "" {
		if code != expected {
			t.Fatalf("expected %s, got %s", expected, code)
		}
		return
	}
	if code != "valid" || digest == "" {
		t.Fatalf("registry result=%s/%s", code, digest)
	}
	if strings.Contains(string(b), "latest") {
		t.Fatal("mutable tag in registry")
	}
	fmt.Printf("registry_digest=%s\n", digest)
}
