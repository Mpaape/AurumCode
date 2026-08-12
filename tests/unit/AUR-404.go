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

const schemaPath404 = ".board/schemas/go-git-offline-profile.schema.json"
const profilePath404 = ".board/oci/profiles/go-git-offline-v1.json"
const lockPath404 = ".board/locks/oci/go-git-offline-v1.lock.json"
const registryPath404 = ".board/oci/profiles/registry.v1.json"
const containerSchemaPath404 = ".board/schemas/container-profile.schema.json"
const registryLockPath404 = ".board/locks/oci/registry-v1.lock.json"
const goUnitSchemaPath404 = ".board/schemas/go-unit-offline-profile.schema.json"
const goUnitLockPath404 = ".board/locks/oci/go-unit-offline-v1.lock.json"
const profileKey404 = "go-git-offline-v1"

type profile404 struct {
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
	GitFixture          string            `json:"git_fixture"`
	GitFixtureRoot      string            `json:"git_fixture_root"`
	HostCheckout        string            `json:"host_checkout"`
	CredentialHelpers   string            `json:"credential_helpers"`
	Hooks               string            `json:"hooks"`
	Signing             string            `json:"signing"`
	Environment         map[string]string `json:"environment"`
	Command             []string          `json:"command"`
}

type lock404 struct {
	Schema  string `json:"schema"`
	Version int    `json:"version"`
	Profile string `json:"profile"`
	Image   string `json:"image"`
}

type entry404 struct {
	Key            string `json:"key"`
	Schema         string `json:"schema"`
	SchemaDigest   string `json:"schema_digest"`
	Lock           string `json:"lock"`
	LockDigest     string `json:"lock_digest"`
	ImageSetDigest string `json:"image_set_digest"`
}

type registry404 struct {
	Schema   string     `json:"schema"`
	Version  int        `json:"version"`
	Profiles []entry404 `json:"profiles"`
}

var digest404 = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// The Git-capable image is pinned to the `golang` repository by immutable digest.
// A mutable tag, a bare name, or any other repository denies before the plan is used.
var image404 = regexp.MustCompile(`^golang@sha256:[0-9a-f]{64}$`)

var registryKeys404 = []string{"bootstrap-readonly-v1", "go-git-offline-v1", "go-unit-offline-v1", "registry-v1"}
var requiredProfileKeys404 = []string{"schema", "version", "profile", "lock", "lock_digest", "network", "user", "cap_drop", "cap_add", "mounts", "devices", "pull", "tmpfs", "checkout_readonly", "read_only_rootfs", "no_new_privileges", "privileged", "timeout_seconds", "memory_mb", "cpu_millis", "pids_limit", "tmpfs_mb", "stdout_limit_bytes", "stderr_limit_bytes", "max_input_files", "max_input_bytes", "module_cache", "module_cache_read_only", "git_fixture", "git_fixture_root", "host_checkout", "credential_helpers", "hooks", "signing", "environment", "command"}
var requiredEnvironmentKeys404 = []string{"GOPROXY", "GOSUMDB", "GONOSUMDB", "GOTOOLCHAIN", "GIT_CONFIG_NOSYSTEM", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM", "GIT_TERMINAL_PROMPT", "GIT_ASKPASS", "GIT_ALLOW_PROTOCOL", "GIT_CEILING_DIRECTORIES"}
var schemaDocumentKeys404 = []string{"$schema", "$id", "title", "type", "additionalProperties", "required", "properties"}
var expectedCommand404 = []string{"go", "test", "-mod=readonly", "./..."}

var profileSchemaConstants404 = map[string]any{
	"schema":                 "aurum.go-git-offline-profile",
	"version":                1,
	"profile":                profileKey404,
	"lock":                   lockPath404,
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
	"timeout_seconds":        120,
	"memory_mb":              256,
	"cpu_millis":             1000,
	"pids_limit":             128,
	"tmpfs_mb":               256,
	"stdout_limit_bytes":     65536,
	"stderr_limit_bytes":     65536,
	"max_input_files":        10000,
	"max_input_bytes":        67108864,
	"module_cache":           "/go/pkg/mod",
	"module_cache_read_only": true,
	"git_fixture":            "ephemeral",
	"git_fixture_root":       "/tmp/aurum-git-fixture",
	"host_checkout":          "denied",
	"credential_helpers":     "denied",
	"hooks":                  "denied",
	"signing":                "denied",
}

var environmentSchemaConstants404 = map[string]any{
	"GOPROXY":                 "off",
	"GOSUMDB":                 "off",
	"GONOSUMDB":               "*",
	"GOTOOLCHAIN":             "local",
	"GIT_CONFIG_NOSYSTEM":     "1",
	"GIT_CONFIG_GLOBAL":       "/dev/null",
	"GIT_CONFIG_SYSTEM":       "/dev/null",
	"GIT_TERMINAL_PROMPT":     "0",
	"GIT_ASKPASS":             "/bin/false",
	"GIT_ALLOW_PROTOCOL":      "none",
	"GIT_CEILING_DIRECTORIES": "/tmp",
}

func hash404(b []byte) string { h := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(h[:]) }

// rejectDuplicateJSON404 refuses a document whose last-wins duplicate key would let a
// registry publish one plan to a reader and another to the runner.
func rejectDuplicateJSON404(b []byte) error {
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

func decode404(b []byte, out any) error {
	if err := rejectDuplicateJSON404(b); err != nil {
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

func exactKeys404(raw map[string]json.RawMessage, expected []string) bool {
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

func exactStringSet404(actual, expected []string) bool {
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

func exactCommand404(command []string) bool {
	if len(command) != len(expectedCommand404) {
		return false
	}
	for i, token := range expectedCommand404 {
		if command[i] != token {
			return false
		}
	}
	return true
}

func schemaConst404(raw json.RawMessage, expected any) bool {
	var spec map[string]json.RawMessage
	if decode404(raw, &spec) != nil || !exactKeys404(spec, []string{"const"}) {
		return false
	}
	var actual any
	if decode404(spec["const"], &actual) != nil {
		return false
	}
	actualJSON, actualErr := json.Marshal(actual)
	expectedJSON, expectedErr := json.Marshal(expected)
	return actualErr == nil && expectedErr == nil && bytes.Equal(actualJSON, expectedJSON)
}

func validateSchemaInvariant404(schema []byte) bool {
	var raw map[string]json.RawMessage
	if decode404(schema, &raw) != nil || !exactKeys404(raw, schemaDocumentKeys404) {
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
	if decode404(schema, &doc) != nil ||
		doc.Schema != "https://json-schema.org/draft/2020-12/schema" ||
		doc.ID != "https://aurumcode.dev/schemas/go-git-offline-profile.schema.json" ||
		doc.Title != "AurumCode offline Go Git profile" ||
		doc.Type != "object" || doc.AdditionalProperties ||
		!exactStringSet404(doc.Required, requiredProfileKeys404) ||
		!exactKeys404(doc.Properties, requiredProfileKeys404) {
		return false
	}
	for key, expected := range profileSchemaConstants404 {
		if !schemaConst404(doc.Properties[key], expected) {
			return false
		}
	}
	var digestSpec map[string]json.RawMessage
	if decode404(doc.Properties["lock_digest"], &digestSpec) != nil || !exactKeys404(digestSpec, []string{"type", "pattern"}) {
		return false
	}
	var digestRule struct {
		Type    string `json:"type"`
		Pattern string `json:"pattern"`
	}
	if decode404(doc.Properties["lock_digest"], &digestRule) != nil || digestRule.Type != "string" || digestRule.Pattern != "^sha256:[0-9a-f]{64}$" {
		return false
	}
	if !schemaConst404(doc.Properties["command"], expectedCommand404) {
		return false
	}
	var envRaw map[string]json.RawMessage
	if decode404(doc.Properties["environment"], &envRaw) != nil || !exactKeys404(envRaw, []string{"type", "additionalProperties", "required", "properties"}) {
		return false
	}
	var envSpec struct {
		Type                 string                     `json:"type"`
		AdditionalProperties bool                       `json:"additionalProperties"`
		Required             []string                   `json:"required"`
		Properties           map[string]json.RawMessage `json:"properties"`
	}
	if decode404(doc.Properties["environment"], &envSpec) != nil || envSpec.Type != "object" || envSpec.AdditionalProperties ||
		!exactStringSet404(envSpec.Required, requiredEnvironmentKeys404) ||
		!exactKeys404(envSpec.Properties, requiredEnvironmentKeys404) {
		return false
	}
	for key, expected := range environmentSchemaConstants404 {
		if !schemaConst404(envSpec.Properties[key], expected) {
			return false
		}
	}
	return true
}

func read404(root, path string) ([]byte, error) {
	if strings.Contains(path, "..") || strings.Contains(path, "//") {
		return nil, fmt.Errorf("unsafe path")
	}
	info, err := os.Lstat(filepath.Join(root, path))
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("missing")
	}
	return os.ReadFile(filepath.Join(root, path))
}

// ValidateProfileAUR404 is the side-effect-free plan loader for go-git-offline-v1.
// It never invokes a container engine and never reads anything outside the
// documents this card owns.
func ValidateProfileAUR404(root string) (profile404, lock404, string) {
	sb, err := read404(root, schemaPath404)
	if err != nil {
		return profile404{}, lock404{}, "schema-missing"
	}
	if !validateSchemaInvariant404(sb) {
		return profile404{}, lock404{}, "schema-invalid"
	}
	pb, err := read404(root, profilePath404)
	if err != nil {
		return profile404{}, lock404{}, "profile-missing"
	}
	var raw map[string]json.RawMessage
	if decode404(pb, &raw) != nil || !exactKeys404(raw, requiredProfileKeys404) {
		return profile404{}, lock404{}, "schema-invalid"
	}
	var envRaw map[string]json.RawMessage
	if decode404(raw["environment"], &envRaw) != nil || !exactKeys404(envRaw, requiredEnvironmentKeys404) {
		return profile404{}, lock404{}, "schema-invalid"
	}
	var p profile404
	if decode404(pb, &p) != nil {
		return p, lock404{}, "schema-invalid"
	}
	if p.Schema != "aurum.go-git-offline-profile" || p.Version != 1 || p.Profile != profileKey404 ||
		p.Lock != lockPath404 || !digest404.MatchString(p.LockDigest) ||
		p.Network != "none" || p.User != "65534:65534" || p.CapDrop != "ALL" || p.CapAdd != "none" ||
		p.Mounts != "none" || p.Devices != "none" || p.Pull != "never" || p.Tmpfs != "rw,nosuid,nodev" ||
		!p.CheckoutReadonly || !p.ReadOnlyRootfs || !p.NoNewPrivileges || p.Privileged ||
		p.TimeoutSeconds != 120 || p.MemoryMB != 256 || p.CPUMillis != 1000 || p.PidsLimit != 128 ||
		p.TmpfsMB != 256 || p.StdoutLimitBytes != 65536 || p.StderrLimitBytes != 65536 ||
		p.MaxInputFiles != 10000 || p.MaxInputBytes != 67108864 ||
		p.ModuleCache != "/go/pkg/mod" || !p.ModuleCacheReadOnly ||
		p.GitFixture != "ephemeral" || p.GitFixtureRoot != "/tmp/aurum-git-fixture" ||
		p.HostCheckout != "denied" || p.CredentialHelpers != "denied" ||
		p.Hooks != "denied" || p.Signing != "denied" ||
		!exactCommand404(p.Command) {
		return p, lock404{}, "unsafe-plan"
	}
	for key, expected := range environmentSchemaConstants404 {
		if p.Environment[key] != expected.(string) {
			return p, lock404{}, "unsafe-plan"
		}
	}
	lb, err := read404(root, lockPath404)
	if err != nil {
		return p, lock404{}, "lock-missing"
	}
	var l lock404
	if decode404(lb, &l) != nil || l.Schema != "aurum.oci-image-lock" || l.Version != 1 ||
		l.Profile != p.Profile || !image404.MatchString(l.Image) {
		return p, l, "mutable-input"
	}
	if hash404(lb) != p.LockDigest {
		return p, l, "lock-digest-mismatch"
	}
	return p, l, "valid"
}

// ValidateRegistryAUR404 resolves the canonical registry and admits exactly the four
// registered keys. It is fail-closed: an unknown key, a duplicate, an out-of-order
// entry, or a digest that does not match the bytes on disk denies without any engine.
func ValidateRegistryAUR404(root string) (string, string) {
	rb, err := read404(root, registryPath404)
	if err != nil {
		return "", "registry-missing"
	}
	var r registry404
	if decode404(rb, &r) != nil || r.Schema != "aurum.profile-registry" || r.Version != 1 {
		return "", "schema-invalid"
	}
	// Shape first, content second. A key published twice is `duplicate-profile`
	// wherever the copies sit, so the reported code never depends on which copy the
	// loader happened to validate first.
	keys := make([]string, len(r.Profiles))
	seen := map[string]bool{}
	for i, x := range r.Profiles {
		if seen[x.Key] {
			return "", "duplicate-profile"
		}
		seen[x.Key] = true
		keys[i] = x.Key
	}
	for i := 1; i < len(keys); i++ {
		if keys[i-1] >= keys[i] {
			return "", "order-invalid"
		}
	}
	found := false
	for _, x := range r.Profiles {
		switch x.Key {
		case profileKey404:
			found = true
			if x.Schema != schemaPath404 || x.Lock != lockPath404 {
				return "", "unsafe-plan"
			}
			sb, err := read404(root, schemaPath404)
			if err != nil {
				return "", "schema-missing"
			}
			if x.SchemaDigest != hash404(sb) {
				return "", "schema-digest-mismatch"
			}
			_, l, code := ValidateProfileAUR404(root)
			if code != "valid" {
				return "", code
			}
			lb, err := read404(root, lockPath404)
			if err != nil {
				return "", "lock-missing"
			}
			if x.LockDigest != hash404(lb) {
				return "", "lock-digest-mismatch"
			}
			if x.ImageSetDigest != hash404([]byte(l.Image)) {
				return "", "unsafe-plan"
			}
		case "bootstrap-readonly-v1", "registry-v1":
			if x.Schema != containerSchemaPath404 || x.Lock != registryLockPath404 {
				return "", "unsafe-plan"
			}
			sb, err := read404(root, containerSchemaPath404)
			if err != nil {
				return "", "schema-missing"
			}
			if x.SchemaDigest != hash404(sb) {
				return "", "schema-digest-mismatch"
			}
			lb, err := read404(root, registryLockPath404)
			if err != nil {
				return "", "lock-missing"
			}
			if x.LockDigest != hash404(lb) {
				return "", "lock-digest-mismatch"
			}
			var l lock404
			if decode404(lb, &l) != nil || l.Schema != "aurum.oci-image-lock" || l.Version != 1 {
				return "", "mutable-input"
			}
			if x.ImageSetDigest != hash404([]byte(l.Image)) {
				return "", "unsafe-plan"
			}
		case "go-unit-offline-v1":
			// Owned by AUR-403. This card asserts only that the neighbouring entry
			// keeps its declared paths and digest shape; its bytes are not read here
			// so registering Git cannot silently re-point the Go unit plan.
			if x.Schema != goUnitSchemaPath404 || x.Lock != goUnitLockPath404 {
				return "", "unsafe-plan"
			}
			if !digest404.MatchString(x.SchemaDigest) || !digest404.MatchString(x.LockDigest) || !digest404.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		default:
			return "", "profile-unregistered"
		}
	}
	if !found || !sort.StringsAreSorted(keys) || !exactStringSet404(keys, registryKeys404) {
		return "", "profile-unregistered"
	}
	return hash404(rb), "valid"
}

func writeMutation404(root, path string, data []byte) error {
	full := filepath.Join(root, path)
	if err := os.Chmod(full, 0600); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0600)
}

func replaceOnce404(root, path, from, to string) error {
	b, err := read404(root, path)
	if err != nil {
		return err
	}
	if !bytes.Contains(b, []byte(from)) {
		return fmt.Errorf("mutation anchor %q absent from %s", from, path)
	}
	return writeMutation404(root, path, bytes.Replace(b, []byte(from), []byte(to), 1))
}

func TestAUR404(t *testing.T) {
	root := os.Getenv("AURUMCODE_ROOT")
	if root == "" {
		t.Fatal("AURUMCODE_ROOT is required")
	}
	mutation := os.Getenv("AURUM_A404_MUTATION")
	expected := mutation
	switch mutation {
	case "":
	case "duplicate-profile":
		rb, err := read404(root, registryPath404)
		if err != nil {
			t.Fatalf("registry read: %v", err)
		}
		var r registry404
		if decode404(rb, &r) != nil {
			t.Fatal("registry decode")
		}
		var clone entry404
		for _, x := range r.Profiles {
			if x.Key == profileKey404 {
				clone = x
			}
		}
		if clone.Key == "" {
			t.Fatal("mutation target absent: " + profileKey404)
		}
		// A second document for the same key, with different bytes.
		clone.LockDigest = "sha256:" + strings.Repeat("0", 64)
		r.Profiles = append(r.Profiles, clone)
		nb, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("registry marshal: %v", err)
		}
		if err := writeMutation404(root, registryPath404, nb); err != nil {
			t.Fatalf("mutate registry: %v", err)
		}
	case "mutable-input":
		lb, err := read404(root, lockPath404)
		if err != nil {
			t.Fatalf("lock read: %v", err)
		}
		lb = bytes.Replace(lb,
			[]byte("golang@sha256:4746d26432a9117a5f58e95cb9f954ddf0de128e9d5816886514199316e4a2fb"),
			[]byte("golang:1.21"), 1)
		if !bytes.Contains(lb, []byte("golang:1.21")) {
			t.Fatal("mutation anchor absent from lock")
		}
		if err := writeMutation404(root, lockPath404, lb); err != nil {
			t.Fatalf("mutate lock: %v", err)
		}
		pb, err := read404(root, profilePath404)
		if err != nil {
			t.Fatalf("profile read: %v", err)
		}
		var p map[string]any
		if decode404(pb, &p) != nil {
			t.Fatal("profile decode")
		}
		p["lock_digest"] = hash404(lb)
		nb, err := json.Marshal(p)
		if err != nil {
			t.Fatalf("profile marshal: %v", err)
		}
		if err := writeMutation404(root, profilePath404, nb); err != nil {
			t.Fatalf("mutate profile: %v", err)
		}
	case "unsafe-plan":
		if err := replaceOnce404(root, profilePath404, `"network": "none"`, `"network": "host"`); err != nil {
			t.Fatalf("mutate network: %v", err)
		}
	case "host-checkout":
		expected = "unsafe-plan"
		if err := replaceOnce404(root, profilePath404, `"host_checkout": "denied"`, `"host_checkout": "allowed"`); err != nil {
			t.Fatalf("mutate host checkout: %v", err)
		}
	case "hook-enabled":
		expected = "unsafe-plan"
		if err := replaceOnce404(root, profilePath404, `"hooks": "denied"`, `"hooks": "allowed"`); err != nil {
			t.Fatalf("mutate hooks: %v", err)
		}
	case "credential-helper":
		expected = "unsafe-plan"
		if err := replaceOnce404(root, profilePath404, `"credential_helpers": "denied"`, `"credential_helpers": "allowed"`); err != nil {
			t.Fatalf("mutate credential helper: %v", err)
		}
	case "askpass-helper":
		expected = "unsafe-plan"
		if err := replaceOnce404(root, profilePath404, `"GIT_ASKPASS": "/bin/false"`, `"GIT_ASKPASS": "/usr/bin/env"`); err != nil {
			t.Fatalf("mutate askpass helper: %v", err)
		}
	case "persistent-fixture":
		expected = "unsafe-plan"
		if err := replaceOnce404(root, profilePath404, `"git_fixture": "ephemeral"`, `"git_fixture": "persistent"`); err != nil {
			t.Fatalf("mutate fixture: %v", err)
		}
	default:
		t.Fatalf("unknown mutation: %s", mutation)
	}

	digest, code := ValidateRegistryAUR404(root)
	if mutation != "" {
		if code != expected {
			t.Fatalf("mutation %s got %s, want %s", mutation, code, expected)
		}
		if digest != "" {
			t.Fatalf("mutation %s returned a registry digest", mutation)
		}
		fmt.Printf("mutation=%s code=%s\n", mutation, code)
		return
	}
	fmt.Printf("registry_code=%s\n", code)
	if code != "valid" || digest == "" {
		t.Fatalf("registry result=%s/%s", code, digest)
	}
	if strings.Contains(string(mustRead404(t, root, registryPath404)), "latest") {
		t.Fatal("mutable tag in registry")
	}
	fmt.Printf("registry_digest=%s\n", digest)
}

func mustRead404(t *testing.T, root, path string) []byte {
	t.Helper()
	b, err := read404(root, path)
	if err != nil {
		t.Fatalf("unreadable %s: %v", path, err)
	}
	return b
}
