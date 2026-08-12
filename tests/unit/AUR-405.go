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

const schemaPath405 = ".board/schemas/fake-provider-profile.schema.json"
const profilePath405 = ".board/oci/profiles/fake-provider-v1.json"
const lockPath405 = ".board/locks/oci/fake-provider-v1.lock.json"
const registryPath405 = ".board/oci/profiles/registry.v1.json"
const containerSchemaPath405 = ".board/schemas/container-profile.schema.json"
const registryLockPath405 = ".board/locks/oci/registry-v1.lock.json"
const goUnitSchemaPath405 = ".board/schemas/go-unit-offline-profile.schema.json"
const goUnitLockPath405 = ".board/locks/oci/go-unit-offline-v1.lock.json"
const goGitSchemaPath405 = ".board/schemas/go-git-offline-profile.schema.json"
const goGitLockPath405 = ".board/locks/oci/go-git-offline-v1.lock.json"
const parserWorkerSchemaPath405 = ".board/schemas/parser-worker-profile.schema.json"
const parserWorkerLockPath405 = ".board/locks/oci/parser-worker-v1.lock.json"
const sqliteSchemaPath405 = ".board/schemas/sqlite-offline-profile.schema.json"
const sqliteLockPath405 = ".board/locks/oci/sqlite-offline-v1.lock.json"
const docsToolSchemaPath405 = ".board/schemas/docs-tool-offline-profile.schema.json"
const docsToolLockPath405 = ".board/locks/oci/docs-tool-offline-v1.lock.json"
const profileKey405 = "fake-provider-v1"

type profile405 struct {
	Schema                string            `json:"schema"`
	Version               int               `json:"version"`
	Profile               string            `json:"profile"`
	Lock                  string            `json:"lock"`
	LockDigest            string            `json:"lock_digest"`
	Network               string            `json:"network"`
	User                  string            `json:"user"`
	CapDrop               string            `json:"cap_drop"`
	CapAdd                string            `json:"cap_add"`
	Mounts                string            `json:"mounts"`
	Devices               string            `json:"devices"`
	Pull                  string            `json:"pull"`
	Tmpfs                 string            `json:"tmpfs"`
	CheckoutReadonly      bool              `json:"checkout_readonly"`
	ReadOnlyRootfs        bool              `json:"read_only_rootfs"`
	NoNewPrivileges       bool              `json:"no_new_privileges"`
	Privileged            bool              `json:"privileged"`
	TimeoutSeconds        int               `json:"timeout_seconds"`
	MemoryMB              int               `json:"memory_mb"`
	CPUMillis             int               `json:"cpu_millis"`
	PidsLimit             int               `json:"pids_limit"`
	TmpfsMB               int               `json:"tmpfs_mb"`
	StdoutLimitBytes      int               `json:"stdout_limit_bytes"`
	StderrLimitBytes      int               `json:"stderr_limit_bytes"`
	MaxInputFiles         int               `json:"max_input_files"`
	MaxInputBytes         int               `json:"max_input_bytes"`
	ModuleCache           string            `json:"module_cache"`
	ModuleCacheReadOnly   bool              `json:"module_cache_read_only"`
	ProviderEndpoint      string            `json:"provider_endpoint"`
	DNS                   string            `json:"dns"`
	Egress                string            `json:"egress"`
	APIKey                string            `json:"api_key"`
	CredentialSources     string            `json:"credential_sources"`
	ResponseScripts       string            `json:"response_scripts"`
	ResponseScriptsRoot   string            `json:"response_scripts_root"`
	RequestTimeoutSeconds int               `json:"request_timeout_seconds"`
	MaxResponseBytes      int               `json:"max_response_bytes"`
	MaxResponses          int               `json:"max_responses"`
	Environment           map[string]string `json:"environment"`
	Command               []string          `json:"command"`
}

type lock405 struct {
	Schema  string `json:"schema"`
	Version int    `json:"version"`
	Profile string `json:"profile"`
	Image   string `json:"image"`
}

type entry405 struct {
	Key            string `json:"key"`
	Schema         string `json:"schema"`
	SchemaDigest   string `json:"schema_digest"`
	Lock           string `json:"lock"`
	LockDigest     string `json:"lock_digest"`
	ImageSetDigest string `json:"image_set_digest"`
}

type registry405 struct {
	Schema   string     `json:"schema"`
	Version  int        `json:"version"`
	Profiles []entry405 `json:"profiles"`
}

var digest405 = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// The fake provider needs a Go toolchain and Bash and no Git, so it is pinned to the
// same locally present bootstrap image by immutable digest. A mutable tag, a bare
// name, or any other repository denies before the plan is used.
var image405 = regexp.MustCompile(`^aurum-bootstrap-go-bash@sha256:[0-9a-f]{64}$`)

// The only endpoint this plan may reach is the loopback address of its own network
// namespace, on the pinned port and API path. Any host, scheme or port beyond that is
// an egress the plan cannot open.
var endpoint405 = regexp.MustCompile(`^http://127\.0\.0\.1:8080/v1$`)

// Credential shapes are assembled from fragments so this source file itself never
// carries a literal that the runner's input gate would read as a real secret.
var credentialShape405 = regexp.MustCompile(`(sk` + `-[A-Za-z0-9_-]{20,}|AKIA` + `[0-9A-Z]{16}|gh` + `[pousr]_[A-Za-z0-9]{20,}|-----BEGIN [A-Z ]*PRIVATE KEY-----)`)

var registryKeys405 = []string{"bootstrap-readonly-v1", "docs-tool-offline-v1", "fake-provider-v1", "go-git-offline-v1", "go-unit-offline-v1", "parser-worker-v1", "registry-v1", "sqlite-offline-v1"}
var requiredProfileKeys405 = []string{"schema", "version", "profile", "lock", "lock_digest", "network", "user", "cap_drop", "cap_add", "mounts", "devices", "pull", "tmpfs", "checkout_readonly", "read_only_rootfs", "no_new_privileges", "privileged", "timeout_seconds", "memory_mb", "cpu_millis", "pids_limit", "tmpfs_mb", "stdout_limit_bytes", "stderr_limit_bytes", "max_input_files", "max_input_bytes", "module_cache", "module_cache_read_only", "provider_endpoint", "dns", "egress", "api_key", "credential_sources", "response_scripts", "response_scripts_root", "request_timeout_seconds", "max_response_bytes", "max_responses", "environment", "command"}
var requiredEnvironmentKeys405 = []string{"GOPROXY", "GOSUMDB", "GONOSUMDB", "GOTOOLCHAIN", "OPENAI_API_KEY", "OPENAI_BASE_URL", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"}
var schemaDocumentKeys405 = []string{"$schema", "$id", "title", "type", "additionalProperties", "required", "properties"}
var expectedCommand405 = []string{"go", "test", "-mod=readonly", "./..."}

var profileSchemaConstants405 = map[string]any{
	"schema":                  "aurum.fake-provider-profile",
	"version":                 1,
	"profile":                 profileKey405,
	"lock":                    lockPath405,
	"network":                 "none",
	"user":                    "65534:65534",
	"cap_drop":                "ALL",
	"cap_add":                 "none",
	"mounts":                  "none",
	"devices":                 "none",
	"pull":                    "never",
	"tmpfs":                   "rw,nosuid,nodev",
	"checkout_readonly":       true,
	"read_only_rootfs":        true,
	"no_new_privileges":       true,
	"privileged":              false,
	"timeout_seconds":         60,
	"memory_mb":               256,
	"cpu_millis":              1000,
	"pids_limit":              128,
	"tmpfs_mb":                128,
	"stdout_limit_bytes":      65536,
	"stderr_limit_bytes":      65536,
	"max_input_files":         10000,
	"max_input_bytes":         67108864,
	"module_cache":            "/go/pkg/mod",
	"module_cache_read_only":  true,
	"provider_endpoint":       "http://127.0.0.1:8080/v1",
	"dns":                     "denied",
	"egress":                  "denied",
	"api_key":                 "absent",
	"credential_sources":      "denied",
	"response_scripts":        "digest-pinned",
	"response_scripts_root":   "/tmp/aurum-fake-provider",
	"request_timeout_seconds": 5,
	"max_response_bytes":      65536,
	"max_responses":           64,
}

var environmentSchemaConstants405 = map[string]any{
	"GOPROXY":         "off",
	"GOSUMDB":         "off",
	"GONOSUMDB":       "*",
	"GOTOOLCHAIN":     "local",
	"OPENAI_API_KEY":  "",
	"OPENAI_BASE_URL": "http://127.0.0.1:8080/v1",
	"HTTP_PROXY":      "",
	"HTTPS_PROXY":     "",
	"NO_PROXY":        "*",
}

func hash405(b []byte) string { h := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(h[:]) }

// rejectDuplicateJSON405 refuses a document whose last-wins duplicate key would let a
// registry publish one plan to a reader and another to the runner.
func rejectDuplicateJSON405(b []byte) error {
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

func decode405(b []byte, out any) error {
	if err := rejectDuplicateJSON405(b); err != nil {
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

func exactKeys405(raw map[string]json.RawMessage, expected []string) bool {
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

func exactStringSet405(actual, expected []string) bool {
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

func exactCommand405(command []string) bool {
	if len(command) != len(expectedCommand405) {
		return false
	}
	for i, token := range expectedCommand405 {
		if command[i] != token {
			return false
		}
	}
	return true
}

func schemaConst405(raw json.RawMessage, expected any) bool {
	var spec map[string]json.RawMessage
	if decode405(raw, &spec) != nil || !exactKeys405(spec, []string{"const"}) {
		return false
	}
	var actual any
	if decode405(spec["const"], &actual) != nil {
		return false
	}
	actualJSON, actualErr := json.Marshal(actual)
	expectedJSON, expectedErr := json.Marshal(expected)
	return actualErr == nil && expectedErr == nil && bytes.Equal(actualJSON, expectedJSON)
}

func validateSchemaInvariant405(schema []byte) bool {
	var raw map[string]json.RawMessage
	if decode405(schema, &raw) != nil || !exactKeys405(raw, schemaDocumentKeys405) {
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
	if decode405(schema, &doc) != nil ||
		doc.Schema != "https://json-schema.org/draft/2020-12/schema" ||
		doc.ID != "https://aurumcode.dev/schemas/fake-provider-profile.schema.json" ||
		doc.Title != "AurumCode offline fake provider profile" ||
		doc.Type != "object" || doc.AdditionalProperties ||
		!exactStringSet405(doc.Required, requiredProfileKeys405) ||
		!exactKeys405(doc.Properties, requiredProfileKeys405) {
		return false
	}
	for key, expected := range profileSchemaConstants405 {
		if !schemaConst405(doc.Properties[key], expected) {
			return false
		}
	}
	var digestSpec map[string]json.RawMessage
	if decode405(doc.Properties["lock_digest"], &digestSpec) != nil || !exactKeys405(digestSpec, []string{"type", "pattern"}) {
		return false
	}
	var digestRule struct {
		Type    string `json:"type"`
		Pattern string `json:"pattern"`
	}
	if decode405(doc.Properties["lock_digest"], &digestRule) != nil || digestRule.Type != "string" || digestRule.Pattern != "^sha256:[0-9a-f]{64}$" {
		return false
	}
	if !schemaConst405(doc.Properties["command"], expectedCommand405) {
		return false
	}
	var envRaw map[string]json.RawMessage
	if decode405(doc.Properties["environment"], &envRaw) != nil || !exactKeys405(envRaw, []string{"type", "additionalProperties", "required", "properties"}) {
		return false
	}
	var envSpec struct {
		Type                 string                     `json:"type"`
		AdditionalProperties bool                       `json:"additionalProperties"`
		Required             []string                   `json:"required"`
		Properties           map[string]json.RawMessage `json:"properties"`
	}
	if decode405(doc.Properties["environment"], &envSpec) != nil || envSpec.Type != "object" || envSpec.AdditionalProperties ||
		!exactStringSet405(envSpec.Required, requiredEnvironmentKeys405) ||
		!exactKeys405(envSpec.Properties, requiredEnvironmentKeys405) {
		return false
	}
	for key, expected := range environmentSchemaConstants405 {
		if !schemaConst405(envSpec.Properties[key], expected) {
			return false
		}
	}
	return true
}

func read405(root, path string) ([]byte, error) {
	if strings.Contains(path, "..") || strings.Contains(path, "//") {
		return nil, fmt.Errorf("unsafe path")
	}
	info, err := os.Lstat(filepath.Join(root, path))
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("missing")
	}
	return os.ReadFile(filepath.Join(root, path))
}

// ValidateProfileAUR405 is the side-effect-free plan loader for fake-provider-v1.
// It never invokes a container engine, never opens a socket, and never reads
// anything outside the documents this card owns.
func ValidateProfileAUR405(root string) (profile405, lock405, string) {
	sb, err := read405(root, schemaPath405)
	if err != nil {
		return profile405{}, lock405{}, "schema-missing"
	}
	if !validateSchemaInvariant405(sb) {
		return profile405{}, lock405{}, "schema-invalid"
	}
	pb, err := read405(root, profilePath405)
	if err != nil {
		return profile405{}, lock405{}, "profile-missing"
	}
	var raw map[string]json.RawMessage
	if decode405(pb, &raw) != nil || !exactKeys405(raw, requiredProfileKeys405) {
		return profile405{}, lock405{}, "schema-invalid"
	}
	var envRaw map[string]json.RawMessage
	if decode405(raw["environment"], &envRaw) != nil || !exactKeys405(envRaw, requiredEnvironmentKeys405) {
		return profile405{}, lock405{}, "schema-invalid"
	}
	var p profile405
	if decode405(pb, &p) != nil {
		return p, lock405{}, "schema-invalid"
	}
	// Credential shapes are refused before any other plan rule, so a value that could
	// authenticate against a real provider is denied even when it arrives through a
	// field whose exact value is otherwise pinned by a constant below.
	if credentialShape405.MatchString(p.APIKey) {
		return p, lock405{}, "credential-present"
	}
	for _, value := range p.Environment {
		if credentialShape405.MatchString(value) {
			return p, lock405{}, "credential-present"
		}
	}
	// A response script set is admitted only while it is content-addressed. The
	// script bytes belong to a dependent implementation card; what this card pins is
	// that an unpinned script set can never be resolved.
	if p.ResponseScripts != "digest-pinned" || !digest405.MatchString(p.LockDigest) {
		return p, lock405{}, "digest-invalid"
	}
	if p.Schema != "aurum.fake-provider-profile" || p.Version != 1 || p.Profile != profileKey405 ||
		p.Lock != lockPath405 ||
		p.Network != "none" || p.User != "65534:65534" || p.CapDrop != "ALL" || p.CapAdd != "none" ||
		p.Mounts != "none" || p.Devices != "none" || p.Pull != "never" || p.Tmpfs != "rw,nosuid,nodev" ||
		!p.CheckoutReadonly || !p.ReadOnlyRootfs || !p.NoNewPrivileges || p.Privileged ||
		p.TimeoutSeconds != 60 || p.MemoryMB != 256 || p.CPUMillis != 1000 || p.PidsLimit != 128 ||
		p.TmpfsMB != 128 || p.StdoutLimitBytes != 65536 || p.StderrLimitBytes != 65536 ||
		p.MaxInputFiles != 10000 || p.MaxInputBytes != 67108864 ||
		p.ModuleCache != "/go/pkg/mod" || !p.ModuleCacheReadOnly ||
		!endpoint405.MatchString(p.ProviderEndpoint) ||
		p.DNS != "denied" || p.Egress != "denied" ||
		p.APIKey != "absent" || p.CredentialSources != "denied" ||
		p.ResponseScriptsRoot != "/tmp/aurum-fake-provider" ||
		p.RequestTimeoutSeconds != 5 || p.MaxResponseBytes != 65536 || p.MaxResponses != 64 ||
		!exactCommand405(p.Command) {
		return p, lock405{}, "unsafe-plan"
	}
	for key, expected := range environmentSchemaConstants405 {
		if p.Environment[key] != expected.(string) {
			return p, lock405{}, "unsafe-plan"
		}
	}
	lb, err := read405(root, lockPath405)
	if err != nil {
		return p, lock405{}, "lock-missing"
	}
	var l lock405
	if decode405(lb, &l) != nil || l.Schema != "aurum.oci-image-lock" || l.Version != 1 ||
		l.Profile != p.Profile || !image405.MatchString(l.Image) {
		return p, l, "mutable-input"
	}
	if hash405(lb) != p.LockDigest {
		return p, l, "lock-digest-mismatch"
	}
	return p, l, "valid"
}

// ValidateRegistryAUR405 resolves the canonical registry and admits exactly the eight
// registered keys. It is fail-closed: an unknown key, a duplicate, an out-of-order
// entry, or a digest that does not match the bytes on disk denies without any engine.
func ValidateRegistryAUR405(root string) (string, string) {
	rb, err := read405(root, registryPath405)
	if err != nil {
		return "", "registry-missing"
	}
	var r registry405
	if decode405(rb, &r) != nil || r.Schema != "aurum.profile-registry" || r.Version != 1 {
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
		case profileKey405:
			found = true
			if x.Schema != schemaPath405 || x.Lock != lockPath405 {
				return "", "unsafe-plan"
			}
			sb, err := read405(root, schemaPath405)
			if err != nil {
				return "", "schema-missing"
			}
			if x.SchemaDigest != hash405(sb) {
				return "", "schema-digest-mismatch"
			}
			_, l, code := ValidateProfileAUR405(root)
			if code != "valid" {
				return "", code
			}
			lb, err := read405(root, lockPath405)
			if err != nil {
				return "", "lock-missing"
			}
			if x.LockDigest != hash405(lb) {
				return "", "lock-digest-mismatch"
			}
			if x.ImageSetDigest != hash405([]byte(l.Image)) {
				return "", "unsafe-plan"
			}
		case "bootstrap-readonly-v1", "registry-v1":
			if x.Schema != containerSchemaPath405 || x.Lock != registryLockPath405 {
				return "", "unsafe-plan"
			}
			sb, err := read405(root, containerSchemaPath405)
			if err != nil {
				return "", "schema-missing"
			}
			if x.SchemaDigest != hash405(sb) {
				return "", "schema-digest-mismatch"
			}
			lb, err := read405(root, registryLockPath405)
			if err != nil {
				return "", "lock-missing"
			}
			if x.LockDigest != hash405(lb) {
				return "", "lock-digest-mismatch"
			}
			var l lock405
			if decode405(lb, &l) != nil || l.Schema != "aurum.oci-image-lock" || l.Version != 1 {
				return "", "mutable-input"
			}
			if x.ImageSetDigest != hash405([]byte(l.Image)) {
				return "", "unsafe-plan"
			}
		case "go-unit-offline-v1":
			// Owned by AUR-403. Its documents are not materialized for this
			// acceptance, so only the declared paths and digest shape are checked; the
			// fake provider can never silently re-point the Go unit plan.
			if x.Schema != goUnitSchemaPath405 || x.Lock != goUnitLockPath405 {
				return "", "unsafe-plan"
			}
			if !digest405.MatchString(x.SchemaDigest) || !digest405.MatchString(x.LockDigest) || !digest405.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "go-git-offline-v1":
			// Owned by AUR-404, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != goGitSchemaPath405 || x.Lock != goGitLockPath405 {
				return "", "unsafe-plan"
			}
			if !digest405.MatchString(x.SchemaDigest) || !digest405.MatchString(x.LockDigest) || !digest405.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "parser-worker-v1":
			// Owned by AUR-406, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != parserWorkerSchemaPath405 || x.Lock != parserWorkerLockPath405 {
				return "", "unsafe-plan"
			}
			if !digest405.MatchString(x.SchemaDigest) || !digest405.MatchString(x.LockDigest) || !digest405.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "sqlite-offline-v1":
			// Owned by AUR-407, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != sqliteSchemaPath405 || x.Lock != sqliteLockPath405 {
				return "", "unsafe-plan"
			}
			if !digest405.MatchString(x.SchemaDigest) || !digest405.MatchString(x.LockDigest) || !digest405.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "docs-tool-offline-v1":
			// Owned by AUR-408, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != docsToolSchemaPath405 || x.Lock != docsToolLockPath405 {
				return "", "unsafe-plan"
			}
			if !digest405.MatchString(x.SchemaDigest) || !digest405.MatchString(x.LockDigest) || !digest405.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		default:
			return "", "profile-unregistered"
		}
	}
	if !found || !sort.StringsAreSorted(keys) || !exactStringSet405(keys, registryKeys405) {
		return "", "profile-unregistered"
	}
	return hash405(rb), "valid"
}

func writeMutation405(root, path string, data []byte) error {
	full := filepath.Join(root, path)
	if err := os.Chmod(full, 0600); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0600)
}

func replaceOnce405(root, path, from, to string) error {
	b, err := read405(root, path)
	if err != nil {
		return err
	}
	if !bytes.Contains(b, []byte(from)) {
		return fmt.Errorf("mutation anchor %q absent from %s", from, path)
	}
	return writeMutation405(root, path, bytes.Replace(b, []byte(from), []byte(to), 1))
}

// dropKey405 removes one required key from the profile document, which is how a
// declared bound goes missing in practice.
func dropKey405(root, key string) error {
	pb, err := read405(root, profilePath405)
	if err != nil {
		return err
	}
	var p map[string]any
	if err := decode405(pb, &p); err != nil {
		return err
	}
	if _, ok := p[key]; !ok {
		return fmt.Errorf("mutation anchor %q absent from %s", key, profilePath405)
	}
	delete(p, key)
	nb, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return writeMutation405(root, profilePath405, nb)
}

func TestAUR405(t *testing.T) {
	root := os.Getenv("AURUMCODE_ROOT")
	if root == "" {
		t.Fatal("AURUMCODE_ROOT is required")
	}
	mutation := os.Getenv("AURUM_A405_MUTATION")
	expected := mutation
	switch mutation {
	case "":
	case "duplicate-profile":
		rb, err := read405(root, registryPath405)
		if err != nil {
			t.Fatalf("registry read: %v", err)
		}
		var r registry405
		if decode405(rb, &r) != nil {
			t.Fatal("registry decode")
		}
		var clone entry405
		for _, x := range r.Profiles {
			if x.Key == profileKey405 {
				clone = x
			}
		}
		if clone.Key == "" {
			t.Fatal("mutation target absent: " + profileKey405)
		}
		// A second document for the same key, with different bytes.
		clone.LockDigest = "sha256:" + strings.Repeat("0", 64)
		r.Profiles = append(r.Profiles, clone)
		nb, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("registry marshal: %v", err)
		}
		if err := writeMutation405(root, registryPath405, nb); err != nil {
			t.Fatalf("mutate registry: %v", err)
		}
	case "mutable-input":
		lb, err := read405(root, lockPath405)
		if err != nil {
			t.Fatalf("lock read: %v", err)
		}
		lb = bytes.Replace(lb,
			[]byte("aurum-bootstrap-go-bash@sha256:503ac356fa6bca4bad56fade87b5479ff371bd446ffb9c7db91f211323c7c73e"),
			[]byte("aurum-bootstrap-go-bash:1.21"), 1)
		if !bytes.Contains(lb, []byte("aurum-bootstrap-go-bash:1.21")) {
			t.Fatal("mutation anchor absent from lock")
		}
		if err := writeMutation405(root, lockPath405, lb); err != nil {
			t.Fatalf("mutate lock: %v", err)
		}
		pb, err := read405(root, profilePath405)
		if err != nil {
			t.Fatalf("profile read: %v", err)
		}
		var p map[string]any
		if decode405(pb, &p) != nil {
			t.Fatal("profile decode")
		}
		p["lock_digest"] = hash405(lb)
		nb, err := json.Marshal(p)
		if err != nil {
			t.Fatalf("profile marshal: %v", err)
		}
		if err := writeMutation405(root, profilePath405, nb); err != nil {
			t.Fatalf("mutate profile: %v", err)
		}
	case "unsafe-plan":
		if err := replaceOnce405(root, profilePath405, `"network": "none"`, `"network": "host"`); err != nil {
			t.Fatalf("mutate network: %v", err)
		}
	case "external-endpoint":
		expected = "unsafe-plan"
		if err := replaceOnce405(root, profilePath405, `"provider_endpoint": "http://127.0.0.1:8080/v1"`, `"provider_endpoint": "https://api.openai.com/v1"`); err != nil {
			t.Fatalf("mutate endpoint: %v", err)
		}
	case "external-base-url":
		expected = "unsafe-plan"
		if err := replaceOnce405(root, profilePath405, `"OPENAI_BASE_URL": "http://127.0.0.1:8080/v1"`, `"OPENAI_BASE_URL": "https://api.openai.com/v1"`); err != nil {
			t.Fatalf("mutate base url: %v", err)
		}
	case "real-credential":
		expected = "credential-present"
		// Assembled here, never written in this source file, so the runner's input
		// gate never sees a credential-shaped literal in a materialized path.
		fake := "sk" + "-" + strings.Repeat("A", 32)
		if !credentialShape405.MatchString(fake) {
			t.Fatal("mutation vector is not credential shaped")
		}
		if err := replaceOnce405(root, profilePath405, `"api_key": "absent"`, `"api_key": "`+fake+`"`); err != nil {
			t.Fatalf("mutate api key: %v", err)
		}
	case "credential-source":
		expected = "unsafe-plan"
		if err := replaceOnce405(root, profilePath405, `"credential_sources": "denied"`, `"credential_sources": "environment"`); err != nil {
			t.Fatalf("mutate credential source: %v", err)
		}
	case "unbounded-response":
		expected = "unsafe-plan"
		if err := replaceOnce405(root, profilePath405, `"max_response_bytes": 65536`, `"max_response_bytes": 0`); err != nil {
			t.Fatalf("mutate response bound: %v", err)
		}
	case "unpinned-script":
		expected = "digest-invalid"
		if err := replaceOnce405(root, profilePath405, `"response_scripts": "digest-pinned"`, `"response_scripts": "unpinned"`); err != nil {
			t.Fatalf("mutate response scripts: %v", err)
		}
	case "missing-timeout":
		expected = "schema-invalid"
		if err := dropKey405(root, "timeout_seconds"); err != nil {
			t.Fatalf("mutate timeout: %v", err)
		}
	case "egress-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce405(root, profilePath405, `"egress": "denied"`, `"egress": "allowed"`); err != nil {
			t.Fatalf("mutate egress: %v", err)
		}
	default:
		t.Fatalf("unknown mutation: %s", mutation)
	}

	digest, code := ValidateRegistryAUR405(root)
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
	registryBytes := mustRead405(t, root, registryPath405)
	if strings.Contains(string(registryBytes), "latest") {
		t.Fatal("mutable tag in registry")
	}
	// No materialized document this card owns may carry a credential shape.
	for _, path := range []string{registryPath405, profilePath405, lockPath405, schemaPath405} {
		if credentialShape405.Match(mustRead405(t, root, path)) {
			t.Fatalf("credential shape in %s", path)
		}
	}
	fmt.Printf("registry_digest=%s\n", digest)
}

func mustRead405(t *testing.T, root, path string) []byte {
	t.Helper()
	b, err := read405(root, path)
	if err != nil {
		t.Fatalf("unreadable %s: %v", path, err)
	}
	return b
}
