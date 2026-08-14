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

const schemaPath403 = ".board/schemas/go-unit-offline-profile.schema.json"
const profilePath403 = ".board/oci/profiles/go-unit-offline-v1.json"
const lockPath403 = ".board/locks/oci/go-unit-offline-v1.lock.json"
const registryPath403 = ".board/oci/profiles/registry.v1.json"
const gitSchemaPath403 = ".board/schemas/go-git-offline-profile.schema.json"
const gitLockPath403 = ".board/locks/oci/go-git-offline-v1.lock.json"
const fakeProviderSchemaPath403 = ".board/schemas/fake-provider-profile.schema.json"
const fakeProviderLockPath403 = ".board/locks/oci/fake-provider-v1.lock.json"
const parserWorkerSchemaPath403 = ".board/schemas/parser-worker-profile.schema.json"
const parserWorkerLockPath403 = ".board/locks/oci/parser-worker-v1.lock.json"
const sqliteSchemaPath403 = ".board/schemas/sqlite-offline-profile.schema.json"
const sqliteLockPath403 = ".board/locks/oci/sqlite-offline-v1.lock.json"
const docsToolSchemaPath403 = ".board/schemas/docs-tool-offline-profile.schema.json"
const docsToolLockPath403 = ".board/locks/oci/docs-tool-offline-v1.lock.json"
const fakeScmSchemaPath403 = ".board/schemas/fake-scm-offline-profile.schema.json"
const fakeScmLockPath403 = ".board/locks/oci/fake-scm-offline-v1.lock.json"
const ociConformanceSchemaPath403 = ".board/schemas/oci-conformance-profile.schema.json"
const ociConformanceLockPath403 = ".board/locks/oci/oci-conformance-v1.lock.json"
const polyglotSchemaPath403 = ".board/schemas/polyglot-toolchain-profile.schema.json"
const polyglotLockPath403 = ".board/locks/oci/polyglot-toolchain-v1.lock.json"

type profile403 struct {
	Schema              string            `json:"schema"`
	Version             int               `json:"version"`
	Profile             string            `json:"profile"`
	Lock                string            `json:"lock"`
	LockDigest          string            `json:"lock_digest"`
	Network             string            `json:"network"`
	User                string            `json:"user"`
	CapDrop             string            `json:"cap_drop"`
	CapAdd              string            `json:"cap_add"`
	Mounts              string            `json:"mounts"`
	Devices             string            `json:"devices"`
	Pull                string            `json:"pull"`
	Tmpfs               string            `json:"tmpfs"`
	CheckoutReadonly    bool              `json:"checkout_readonly"`
	ReadOnlyRootfs      bool              `json:"read_only_rootfs"`
	NoNewPrivileges     bool              `json:"no_new_privileges"`
	Privileged          bool              `json:"privileged"`
	TimeoutSeconds      int               `json:"timeout_seconds"`
	MemoryMB            int               `json:"memory_mb"`
	CPUMillis           int               `json:"cpu_millis"`
	PidsLimit           int               `json:"pids_limit"`
	TmpfsMB             int               `json:"tmpfs_mb"`
	StdoutLimitBytes    int               `json:"stdout_limit_bytes"`
	StderrLimitBytes    int               `json:"stderr_limit_bytes"`
	MaxInputFiles       int               `json:"max_input_files"`
	MaxInputBytes       int               `json:"max_input_bytes"`
	ModuleCache         string            `json:"module_cache"`
	ModuleCacheReadOnly bool              `json:"module_cache_read_only"`
	Environment         map[string]string `json:"environment"`
	Command             []string          `json:"command"`
}
type lock403 struct {
	Schema  string `json:"schema"`
	Version int    `json:"version"`
	Profile string `json:"profile"`
	Image   string `json:"image"`
}
type entry403 struct {
	Key            string `json:"key"`
	Schema         string `json:"schema"`
	SchemaDigest   string `json:"schema_digest"`
	Lock           string `json:"lock"`
	LockDigest     string `json:"lock_digest"`
	ImageSetDigest string `json:"image_set_digest"`
}
type registry403 struct {
	Schema   string     `json:"schema"`
	Version  int        `json:"version"`
	Profiles []entry403 `json:"profiles"`
}

var digest403 = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var image403 = regexp.MustCompile(`^aurum-bootstrap-go-bash@sha256:[0-9a-f]{64}$`)
var requiredProfileKeys403 = []string{"schema", "version", "profile", "lock", "lock_digest", "network", "user", "cap_drop", "cap_add", "mounts", "devices", "pull", "tmpfs", "checkout_readonly", "read_only_rootfs", "no_new_privileges", "privileged", "timeout_seconds", "memory_mb", "cpu_millis", "pids_limit", "tmpfs_mb", "stdout_limit_bytes", "stderr_limit_bytes", "max_input_files", "max_input_bytes", "module_cache", "module_cache_read_only", "environment", "command"}
var requiredEnvironmentKeys403 = []string{"GOPROXY", "GOSUMDB", "GONOSUMDB", "GOTOOLCHAIN"}
var schemaDocumentKeys403 = []string{"$schema", "$id", "title", "type", "additionalProperties", "required", "properties"}
var profileSchemaConstants403 = map[string]any{
	"schema":                 "aurum.go-unit-offline-profile",
	"version":                1,
	"profile":                "go-unit-offline-v1",
	"lock":                   lockPath403,
	"network":                "none",
	"user":                   "65534:65534",
	"cap_drop":               "ALL",
	"cap_add":                "none",
	"mounts":                 "none",
	"devices":                "none",
	"pull":                   "never",
	"tmpfs":                  "rw,nosuid,nodev",
	"checkout_readonly":      true,
	"read_only_rootfs":       true,
	"no_new_privileges":      true,
	"privileged":             false,
	"timeout_seconds":        60,
	"memory_mb":              256,
	"cpu_millis":             1000,
	"pids_limit":             128,
	"tmpfs_mb":               128,
	"stdout_limit_bytes":     65536,
	"stderr_limit_bytes":     65536,
	"max_input_files":        10000,
	"max_input_bytes":        67108864,
	"module_cache":           "/go/pkg/mod",
	"module_cache_read_only": true,
}
var environmentSchemaConstants403 = map[string]any{
	"GOPROXY":     "off",
	"GOSUMDB":     "off",
	"GONOSUMDB":   "*",
	"GOTOOLCHAIN": "local",
}

func hash403(b []byte) string { h := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(h[:]) }
func rejectDuplicateJSON403(b []byte) error {
	d := json.NewDecoder(bytes.NewReader(b))
	var walk func() error
	walk = func() error {
		token, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				nameToken, err := d.Token()
				if err != nil {
					return err
				}
				name, ok := nameToken.(string)
				if !ok {
					return fmt.Errorf("object key is not a string")
				}
				if seen[name] {
					return fmt.Errorf("duplicate key %q", name)
				}
				seen[name] = true
				if err := walk(); err != nil {
					return err
				}
			}
			end, err := d.Token()
			if err != nil || end != json.Delim('}') {
				return fmt.Errorf("unterminated object")
			}
		case '[':
			for d.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			end, err := d.Token()
			if err != nil || end != json.Delim(']') {
				return fmt.Errorf("unterminated array")
			}
		default:
			return fmt.Errorf("unexpected delimiter")
		}
		return nil
	}
	if err := walk(); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}
func decode403(b []byte, out any) error {
	if err := rejectDuplicateJSON403(b); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	var tail any
	if d.Decode(&tail) != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}
func exactKeys403(raw map[string]json.RawMessage, expected []string) bool {
	if len(raw) != len(expected) {
		return false
	}
	for _, key := range expected {
		if _, ok := raw[key]; !ok {
			return false
		}
	}
	return true
}
func exactStringSet403(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	seen := map[string]bool{}
	for _, key := range actual {
		if seen[key] {
			return false
		}
		seen[key] = true
	}
	for _, key := range expected {
		if !seen[key] {
			return false
		}
	}
	return true
}
func exactCommand403(command []string) bool {
	return len(command) == 4 && command[0] == "go" && command[1] == "test" && command[2] == "-mod=readonly" && command[3] == "./..."
}
func schemaConst403(raw json.RawMessage, expected any) bool {
	var spec map[string]json.RawMessage
	if decode403(raw, &spec) != nil || !exactKeys403(spec, []string{"const"}) {
		return false
	}
	var actual any
	if decode403(spec["const"], &actual) != nil {
		return false
	}
	actualJSON, actualErr := json.Marshal(actual)
	expectedJSON, expectedErr := json.Marshal(expected)
	return actualErr == nil && expectedErr == nil && bytes.Equal(actualJSON, expectedJSON)
}
func validateSchemaInvariant403(schema []byte) bool {
	var raw map[string]json.RawMessage
	if decode403(schema, &raw) != nil || !exactKeys403(raw, schemaDocumentKeys403) {
		return false
	}
	var doc struct {
		Schema               string                     `json:"$schema"`
		ID                   string                     `json:"$id"`
		Title                string                     `json:"title"`
		Type                 string                     `json:"type"`
		AdditionalProperties bool                       `json:"additionalProperties"`
		Required             []string                   `json:"required"`
		Properties           map[string]json.RawMessage `json:"properties"`
	}
	if decode403(schema, &doc) != nil || doc.Schema != "https://json-schema.org/draft/2020-12/schema" || doc.ID != "https://aurumcode.dev/schemas/go-unit-offline-profile.schema.json" || doc.Title != "AurumCode offline Go unit profile" || doc.Type != "object" || doc.AdditionalProperties || !exactStringSet403(doc.Required, requiredProfileKeys403) || !exactKeys403(doc.Properties, requiredProfileKeys403) {
		return false
	}
	for key, expected := range profileSchemaConstants403 {
		if !schemaConst403(doc.Properties[key], expected) {
			return false
		}
	}
	var digestSpec map[string]json.RawMessage
	if decode403(doc.Properties["lock_digest"], &digestSpec) != nil || !exactKeys403(digestSpec, []string{"type", "pattern"}) {
		return false
	}
	var digestRule struct {
		Type    string `json:"type"`
		Pattern string `json:"pattern"`
	}
	if decode403(doc.Properties["lock_digest"], &digestRule) != nil || digestRule.Type != "string" || digestRule.Pattern != "^sha256:[0-9a-f]{64}$" {
		return false
	}
	if !schemaConst403(doc.Properties["command"], []string{"go", "test", "-mod=readonly", "./..."}) {
		return false
	}
	var envRaw map[string]json.RawMessage
	if decode403(doc.Properties["environment"], &envRaw) != nil || !exactKeys403(envRaw, []string{"type", "additionalProperties", "required", "properties"}) {
		return false
	}
	var envSpec struct {
		Type                 string                     `json:"type"`
		AdditionalProperties bool                       `json:"additionalProperties"`
		Required             []string                   `json:"required"`
		Properties           map[string]json.RawMessage `json:"properties"`
	}
	if decode403(doc.Properties["environment"], &envSpec) != nil || envSpec.Type != "object" || envSpec.AdditionalProperties || !exactStringSet403(envSpec.Required, requiredEnvironmentKeys403) || !exactKeys403(envSpec.Properties, requiredEnvironmentKeys403) {
		return false
	}
	for key, expected := range environmentSchemaConstants403 {
		if !schemaConst403(envSpec.Properties[key], expected) {
			return false
		}
	}
	return true
}
func read403(root, path string) ([]byte, error) {
	if strings.Contains(path, "..") || strings.Contains(path, "//") {
		return nil, fmt.Errorf("unsafe path")
	}
	i, e := os.Lstat(filepath.Join(root, path))
	if e != nil || !i.Mode().IsRegular() {
		return nil, fmt.Errorf("missing")
	}
	return os.ReadFile(filepath.Join(root, path))
}

func validateProfile403(root string) (profile403, lock403, string) {
	sb, e := read403(root, schemaPath403)
	if e != nil {
		return profile403{}, lock403{}, "schema-missing"
	}
	if !validateSchemaInvariant403(sb) {
		return profile403{}, lock403{}, "schema-invalid"
	}
	pb, e := read403(root, profilePath403)
	if e != nil {
		return profile403{}, lock403{}, "profile-missing"
	}
	var raw map[string]json.RawMessage
	if decode403(pb, &raw) != nil || !exactKeys403(raw, requiredProfileKeys403) {
		return profile403{}, lock403{}, "schema-invalid"
	}
	var envRaw map[string]json.RawMessage
	if decode403(raw["environment"], &envRaw) != nil || !exactKeys403(envRaw, requiredEnvironmentKeys403) {
		return profile403{}, lock403{}, "schema-invalid"
	}
	var p profile403
	if decode403(pb, &p) != nil {
		return p, lock403{}, "schema-invalid"
	}
	if p.Schema != "aurum.go-unit-offline-profile" || p.Version != 1 || p.Profile != "go-unit-offline-v1" || p.Lock != lockPath403 || !digest403.MatchString(p.LockDigest) || p.Network != "none" || p.User != "65534:65534" || p.CapDrop != "ALL" || p.CapAdd != "none" || p.Mounts != "none" || p.Devices != "none" || p.Pull != "never" || p.Tmpfs != "rw,nosuid,nodev" || !p.CheckoutReadonly || !p.ReadOnlyRootfs || !p.NoNewPrivileges || p.Privileged || p.TimeoutSeconds != 60 || p.MemoryMB != 256 || p.CPUMillis != 1000 || p.PidsLimit != 128 || p.TmpfsMB != 128 || p.StdoutLimitBytes != 65536 || p.StderrLimitBytes != 65536 || p.MaxInputFiles != 10000 || p.MaxInputBytes != 67108864 || p.ModuleCache != "/go/pkg/mod" || !p.ModuleCacheReadOnly || p.Environment["GOPROXY"] != "off" || p.Environment["GOSUMDB"] != "off" || p.Environment["GONOSUMDB"] != "*" || p.Environment["GOTOOLCHAIN"] != "local" || !exactCommand403(p.Command) {
		return p, lock403{}, "unsafe-plan"
	}
	lb, e := read403(root, lockPath403)
	if e != nil {
		return p, lock403{}, "lock-missing"
	}
	var l lock403
	if decode403(lb, &l) != nil || l.Schema != "aurum.oci-image-lock" || l.Version != 1 || l.Profile != p.Profile || !image403.MatchString(l.Image) {
		return p, l, "mutable-input"
	}
	if hash403(lb) != p.LockDigest {
		return p, l, "lock-digest-mismatch"
	}
	return p, l, "valid"
}
func validateRegistry403(root string) (string, string) {
	rb, e := read403(root, registryPath403)
	if e != nil {
		return "", "registry-missing"
	}
	var r registry403
	if decode403(rb, &r) != nil || r.Schema != "aurum.profile-registry" || r.Version != 1 {
		return "", "schema-invalid"
	}
	sb, e := read403(root, schemaPath403)
	if e != nil {
		return "", "schema-missing"
	}
	keys := make([]string, len(r.Profiles))
	seen := map[string]bool{}
	for i, x := range r.Profiles {
		keys[i] = x.Key
		if seen[x.Key] {
			return "", "duplicate-profile"
		}
		seen[x.Key] = true
		if i > 0 && keys[i-1] >= x.Key {
			return "", "order-invalid"
		}
		if x.Key != "bootstrap-readonly-v1" && x.Key != "fake-provider-v1" && x.Key != "go-unit-offline-v1" && x.Key != "go-git-offline-v1" && x.Key != "parser-worker-v1" && x.Key != "registry-v1" && x.Key != "sqlite-offline-v1" && x.Key != "docs-tool-offline-v1" && x.Key != "fake-scm-offline-v1" && x.Key != "oci-conformance-v1" && x.Key != "polyglot-toolchain-v1" {
			return "", "profile-unregistered"
		}
		if x.Key == "polyglot-toolchain-v1" {
			// Registered by AUR-411. Its documents are not materialized for this
			// acceptance either, so only the declared paths and digest shape are
			// checked: registering the polyglot profile can never re-point the Go
			// unit plan validated below.
			if x.Schema != polyglotSchemaPath403 || x.Lock != polyglotLockPath403 {
				return "", "unsafe-plan"
			}
			if !digest403.MatchString(x.SchemaDigest) || !digest403.MatchString(x.LockDigest) || !digest403.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		} else if x.Key == "oci-conformance-v1" {
			// Registered by AUR-410. Its documents are not materialized for this
			// acceptance either, so only the declared paths and digest shape are
			// checked: registering the conformance profile can never re-point the Go
			// unit plan validated below.
			if x.Schema != ociConformanceSchemaPath403 || x.Lock != ociConformanceLockPath403 {
				return "", "unsafe-plan"
			}
			if !digest403.MatchString(x.SchemaDigest) || !digest403.MatchString(x.LockDigest) || !digest403.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		} else if x.Key == "fake-scm-offline-v1" {
			// Registered by AUR-409. Its documents are not materialized for this
			// acceptance either, so only the declared paths and digest shape are
			// checked: registering the fake SCM can never re-point the Go unit plan
			// validated below.
			if x.Schema != fakeScmSchemaPath403 || x.Lock != fakeScmLockPath403 {
				return "", "unsafe-plan"
			}
			if !digest403.MatchString(x.SchemaDigest) || !digest403.MatchString(x.LockDigest) || !digest403.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		} else if x.Key == "docs-tool-offline-v1" {
			// Registered by AUR-408. Its documents are not materialized for this
			// acceptance either, so only the declared paths and digest shape are
			// checked: registering the documentation tool can never re-point the Go
			// unit plan validated below.
			if x.Schema != docsToolSchemaPath403 || x.Lock != docsToolLockPath403 {
				return "", "unsafe-plan"
			}
			if !digest403.MatchString(x.SchemaDigest) || !digest403.MatchString(x.LockDigest) || !digest403.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		} else if x.Key == "sqlite-offline-v1" {
			// Registered by AUR-407. Its documents are not materialized for this
			// acceptance either, so only the declared paths and digest shape are
			// checked: registering the state store can never re-point the Go unit
			// plan validated below.
			if x.Schema != sqliteSchemaPath403 || x.Lock != sqliteLockPath403 {
				return "", "unsafe-plan"
			}
			if !digest403.MatchString(x.SchemaDigest) || !digest403.MatchString(x.LockDigest) || !digest403.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		} else if x.Key == "parser-worker-v1" {
			// Registered by AUR-406. Its documents are not materialized for this
			// acceptance either, so only the declared paths and digest shape are
			// checked: registering the parser worker can never re-point the Go unit
			// plan validated below.
			if x.Schema != parserWorkerSchemaPath403 || x.Lock != parserWorkerLockPath403 {
				return "", "unsafe-plan"
			}
			if !digest403.MatchString(x.SchemaDigest) || !digest403.MatchString(x.LockDigest) || !digest403.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		} else if x.Key == "fake-provider-v1" {
			// Registered by AUR-405. Its documents are not materialized for this
			// acceptance either, so only the declared paths and digest shape are
			// checked: registering the fake provider can never re-point the Go unit
			// plan validated below.
			if x.Schema != fakeProviderSchemaPath403 || x.Lock != fakeProviderLockPath403 {
				return "", "unsafe-plan"
			}
			if !digest403.MatchString(x.SchemaDigest) || !digest403.MatchString(x.LockDigest) || !digest403.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		} else if x.Key == "go-git-offline-v1" {
			// Registered by AUR-404. Its documents are not materialized for this
			// acceptance, so only the declared paths and digest shape are checked:
			// registering Git can never re-point the Go unit plan validated below.
			if x.Schema != gitSchemaPath403 || x.Lock != gitLockPath403 {
				return "", "unsafe-plan"
			}
			if !digest403.MatchString(x.SchemaDigest) || !digest403.MatchString(x.LockDigest) || !digest403.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		} else if x.Key == "go-unit-offline-v1" {
			if x.Schema != schemaPath403 || x.Lock != lockPath403 {
				return "", "unsafe-plan"
			}
			if x.SchemaDigest != hash403(sb) {
				return "", "schema-digest-mismatch"
			}
			pb, _ := read403(root, profilePath403)
			if hash403(pb) == "" || x.SchemaDigest == "" {
				return "", "schema-invalid"
			}
			_, l, c := validateProfile403(root)
			if c != "valid" {
				return "", c
			}
			lb, _ := read403(root, lockPath403)
			if x.LockDigest != hash403(lb) || x.ImageSetDigest != hash403([]byte(l.Image)) {
				return "", "unsafe-plan"
			}
		} else if x.Schema != ".board/schemas/container-profile.schema.json" || x.Lock != ".board/locks/oci/registry-v1.lock.json" {
			return "", "unsafe-plan"
		} else if !digest403.MatchString(x.SchemaDigest) || !digest403.MatchString(x.LockDigest) || !digest403.MatchString(x.ImageSetDigest) {
			return "", "digest-invalid"
		}
	}
	// Extended by AUR-411 (polyglot-toolchain-v1, eleventh key). The arity stays exact and
	// the positional order stays complete: every key asserted before is still asserted at
	// the single index the canonical ascending order gives it, and the new key is pinned
	// at the only index it can occupy. `polyglot-` sorts after `parser-` -- `a` precedes
	// `o` in the first byte that differs -- and before `registry-`, so the two keys that
	// follow it move one index to the right; no key leaves the list, no arity becomes a
	// minimum and no equality becomes an inequality.
	if len(r.Profiles) != 11 || !sort.StringsAreSorted(keys) || keys[0] != "bootstrap-readonly-v1" || keys[1] != "docs-tool-offline-v1" || keys[2] != "fake-provider-v1" || keys[3] != "fake-scm-offline-v1" || keys[4] != "go-git-offline-v1" || keys[5] != "go-unit-offline-v1" || keys[6] != "oci-conformance-v1" || keys[7] != "parser-worker-v1" || keys[8] != "polyglot-toolchain-v1" || keys[9] != "registry-v1" || keys[10] != "sqlite-offline-v1" {
		return "", "profile-unregistered"
	}
	return hash403(rb), "valid"
}

func writeMutation403(root, path string, data []byte) error {
	full := filepath.Join(root, path)
	if err := os.Chmod(full, 0600); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0600)
}
func TestAUR403(t *testing.T) {
	root := os.Getenv("AURUMCODE_ROOT")
	if root == "" {
		t.Fatal("AURUMCODE_ROOT is required")
	}
	mutation := os.Getenv("AURUM_A403_MUTATION")
	if mutation == "mutable-input" {
		lb, _ := read403(root, lockPath403)
		lb = bytes.Replace(lb, []byte("aurum-bootstrap-go-bash@sha256:503ac356fa6bca4bad56fade87b5479ff371bd446ffb9c7db91f211323c7c73e"), []byte("aurum-bootstrap-go-bash:latest"), 1)
		if err := writeMutation403(root, lockPath403, lb); err != nil {
			t.Fatalf("mutate lock: %v", err)
		}
		pb, _ := read403(root, profilePath403)
		var p map[string]any
		if decode403(pb, &p) != nil {
			t.Fatal("profile decode")
		}
		p["lock_digest"] = hash403(lb)
		nb, _ := json.Marshal(p)
		if err := writeMutation403(root, profilePath403, nb); err != nil {
			t.Fatalf("mutate profile: %v", err)
		}
	}
	if mutation == "unsafe-plan" {
		pb, _ := read403(root, profilePath403)
		pb = bytes.Replace(pb, []byte(`"network": "none"`), []byte(`"network": "host"`), 1)
		if err := writeMutation403(root, profilePath403, pb); err != nil {
			t.Fatalf("mutate isolation: %v", err)
		}
	}
	if mutation == "missing-required" || mutation == "split-command" {
		pb, _ := read403(root, profilePath403)
		var p map[string]any
		if decode403(pb, &p) != nil {
			t.Fatal("profile decode")
		}
		if mutation == "missing-required" {
			delete(p, "privileged")
		} else {
			p["command"] = []string{"go test", "-mod=readonly", "./..."}
		}
		nb, _ := json.Marshal(p)
		if err := writeMutation403(root, profilePath403, nb); err != nil {
			t.Fatalf("mutate profile: %v", err)
		}
	}
	if mutation == "duplicate-profile" {
		rb, _ := read403(root, registryPath403)
		var r registry403
		if decode403(rb, &r) != nil {
			t.Fatal("registry decode")
		}
		r.Profiles = append(r.Profiles, r.Profiles[1])
		nb, _ := json.Marshal(r)
		if err := writeMutation403(root, registryPath403, nb); err != nil {
			t.Fatalf("mutate registry: %v", err)
		}
	}
	d, c := validateRegistry403(root)
	if mutation != "" {
		expected := mutation
		if mutation == "missing-required" {
			expected = "schema-invalid"
		}
		if mutation == "split-command" {
			expected = "unsafe-plan"
		}
		if c != expected {
			t.Fatalf("mutation %s got %s, want %s", mutation, c, expected)
		}
		fmt.Printf("mutation=%s code=%s\n", mutation, c)
		return
	}
	if c != "valid" || d == "" {
		t.Fatalf("registry result=%s/%s", c, d)
	}
	fmt.Printf("registry_digest=%s\n", d)
}
