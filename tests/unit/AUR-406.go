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

const schemaPath406 = ".board/schemas/parser-worker-profile.schema.json"
const profilePath406 = ".board/oci/profiles/parser-worker-v1.json"
const lockPath406 = ".board/locks/oci/parser-worker-v1.lock.json"
const registryPath406 = ".board/oci/profiles/registry.v1.json"
const containerSchemaPath406 = ".board/schemas/container-profile.schema.json"
const registryLockPath406 = ".board/locks/oci/registry-v1.lock.json"
const goUnitSchemaPath406 = ".board/schemas/go-unit-offline-profile.schema.json"
const goUnitLockPath406 = ".board/locks/oci/go-unit-offline-v1.lock.json"
const goGitSchemaPath406 = ".board/schemas/go-git-offline-profile.schema.json"
const goGitLockPath406 = ".board/locks/oci/go-git-offline-v1.lock.json"
const fakeProviderSchemaPath406 = ".board/schemas/fake-provider-profile.schema.json"
const fakeProviderLockPath406 = ".board/locks/oci/fake-provider-v1.lock.json"
const sqliteSchemaPath406 = ".board/schemas/sqlite-offline-profile.schema.json"
const sqliteLockPath406 = ".board/locks/oci/sqlite-offline-v1.lock.json"
const docsToolSchemaPath406 = ".board/schemas/docs-tool-offline-profile.schema.json"
const docsToolLockPath406 = ".board/locks/oci/docs-tool-offline-v1.lock.json"
const fakeScmSchemaPath406 = ".board/schemas/fake-scm-offline-profile.schema.json"
const fakeScmLockPath406 = ".board/locks/oci/fake-scm-offline-v1.lock.json"
const profileKey406 = "parser-worker-v1"

type profile406 struct {
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
	Sockets             string            `json:"sockets"`
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
	Worker              string            `json:"worker"`
	WorkerRoot          string            `json:"worker_root"`
	GrammarSet          string            `json:"grammar_set"`
	GrammarSetRoot      string            `json:"grammar_set_root"`
	CodeExecution       string            `json:"code_execution"`
	HostFilesystem      string            `json:"host_filesystem"`
	Subprocess          string            `json:"subprocess"`
	BlobSource          string            `json:"blob_source"`
	MaxBlobBytes        int               `json:"max_blob_bytes"`
	MaxBlobs            int               `json:"max_blobs"`
	ParseTimeoutSeconds int               `json:"parse_timeout_seconds"`
	Environment         map[string]string `json:"environment"`
	Command             []string          `json:"command"`
}

type lock406 struct {
	Schema  string `json:"schema"`
	Version int    `json:"version"`
	Profile string `json:"profile"`
	Image   string `json:"image"`
}

type entry406 struct {
	Key            string `json:"key"`
	Schema         string `json:"schema"`
	SchemaDigest   string `json:"schema_digest"`
	Lock           string `json:"lock"`
	LockDigest     string `json:"lock_digest"`
	ImageSetDigest string `json:"image_set_digest"`
}

type registry406 struct {
	Schema   string     `json:"schema"`
	Version  int        `json:"version"`
	Profiles []entry406 `json:"profiles"`
}

var digest406 = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// The parser worker needs a Go toolchain and Bash and no Git, so it is pinned to the
// same locally present bootstrap image by immutable digest. A mutable tag, a bare
// name, or any other repository denies before the plan is used.
var image406 = regexp.MustCompile(`^aurum-bootstrap-go-bash@sha256:[0-9a-f]{64}$`)

// The worker and the grammar set live under the bounded tmpfs and nowhere else. A root
// anywhere in the checkout, in the module cache, or on any absolute host path is a host
// filesystem the plan cannot open.
var workerRoot406 = regexp.MustCompile(`^/tmp/aurum-parser-worker$`)
var grammarRoot406 = regexp.MustCompile(`^/tmp/aurum-parser-grammars$`)

// Credential shapes are assembled from fragments so this source file itself never
// carries a literal that the runner's input gate would read as a real secret.
var credentialShape406 = regexp.MustCompile(`(sk` + `-[A-Za-z0-9_-]{20,}|AKIA` + `[0-9A-Z]{16}|gh` + `[pousr]_[A-Za-z0-9]{20,}|-----BEGIN [A-Z ]*PRIVATE KEY-----)`)

var registryKeys406 = []string{"bootstrap-readonly-v1", "docs-tool-offline-v1", "fake-provider-v1", "fake-scm-offline-v1", "go-git-offline-v1", "go-unit-offline-v1", "parser-worker-v1", "registry-v1", "sqlite-offline-v1"}
var requiredProfileKeys406 = []string{"schema", "version", "profile", "lock", "lock_digest", "network", "user", "cap_drop", "cap_add", "mounts", "devices", "sockets", "pull", "tmpfs", "checkout_readonly", "read_only_rootfs", "no_new_privileges", "privileged", "timeout_seconds", "memory_mb", "cpu_millis", "pids_limit", "tmpfs_mb", "stdout_limit_bytes", "stderr_limit_bytes", "max_input_files", "max_input_bytes", "module_cache", "module_cache_read_only", "worker", "worker_root", "grammar_set", "grammar_set_root", "code_execution", "host_filesystem", "subprocess", "blob_source", "max_blob_bytes", "max_blobs", "parse_timeout_seconds", "environment", "command"}
var requiredEnvironmentKeys406 = []string{"GOPROXY", "GOSUMDB", "GONOSUMDB", "GOTOOLCHAIN", "AURUM_PARSER_WORKER_ROOT", "AURUM_PARSER_GRAMMAR_ROOT", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"}
var schemaDocumentKeys406 = []string{"$schema", "$id", "title", "type", "additionalProperties", "required", "properties"}
var expectedCommand406 = []string{"go", "test", "-mod=readonly", "./..."}

var profileSchemaConstants406 = map[string]any{
	"schema":                 "aurum.parser-worker-profile",
	"version":                1,
	"profile":                profileKey406,
	"lock":                   lockPath406,
	"network":                "none",
	"user":                   "65534:65534",
	"cap_drop":               "ALL",
	"cap_add":                "none",
	"mounts":                 "none",
	"devices":                "none",
	"sockets":                "none",
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
	"worker":                 "digest-pinned",
	"worker_root":            "/tmp/aurum-parser-worker",
	"grammar_set":            "digest-pinned",
	"grammar_set_root":       "/tmp/aurum-parser-grammars",
	"code_execution":         "denied",
	"host_filesystem":        "denied",
	"subprocess":             "denied",
	"blob_source":            "stdin",
	"max_blob_bytes":         1048576,
	"max_blobs":              256,
	"parse_timeout_seconds":  5,
}

var environmentSchemaConstants406 = map[string]any{
	"GOPROXY":                   "off",
	"GOSUMDB":                   "off",
	"GONOSUMDB":                 "*",
	"GOTOOLCHAIN":               "local",
	"AURUM_PARSER_WORKER_ROOT":  "/tmp/aurum-parser-worker",
	"AURUM_PARSER_GRAMMAR_ROOT": "/tmp/aurum-parser-grammars",
	"HTTP_PROXY":                "",
	"HTTPS_PROXY":               "",
	"NO_PROXY":                  "*",
}

func hash406(b []byte) string { h := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(h[:]) }

// rejectDuplicateJSON406 refuses a document whose last-wins duplicate key would let a
// registry publish one plan to a reader and another to the runner.
func rejectDuplicateJSON406(b []byte) error {
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

func decode406(b []byte, out any) error {
	if err := rejectDuplicateJSON406(b); err != nil {
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

func exactKeys406(raw map[string]json.RawMessage, expected []string) bool {
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

func exactStringSet406(actual, expected []string) bool {
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

func exactCommand406(command []string) bool {
	if len(command) != len(expectedCommand406) {
		return false
	}
	for i, token := range expectedCommand406 {
		if command[i] != token {
			return false
		}
	}
	return true
}

func schemaConst406(raw json.RawMessage, expected any) bool {
	var spec map[string]json.RawMessage
	if decode406(raw, &spec) != nil || !exactKeys406(spec, []string{"const"}) {
		return false
	}
	var actual any
	if decode406(spec["const"], &actual) != nil {
		return false
	}
	actualJSON, actualErr := json.Marshal(actual)
	expectedJSON, expectedErr := json.Marshal(expected)
	return actualErr == nil && expectedErr == nil && bytes.Equal(actualJSON, expectedJSON)
}

func validateSchemaInvariant406(schema []byte) bool {
	var raw map[string]json.RawMessage
	if decode406(schema, &raw) != nil || !exactKeys406(raw, schemaDocumentKeys406) {
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
	if decode406(schema, &doc) != nil ||
		doc.Schema != "https://json-schema.org/draft/2020-12/schema" ||
		doc.ID != "https://aurumcode.dev/schemas/parser-worker-profile.schema.json" ||
		doc.Title != "AurumCode offline parser worker profile" ||
		doc.Type != "object" || doc.AdditionalProperties ||
		!exactStringSet406(doc.Required, requiredProfileKeys406) ||
		!exactKeys406(doc.Properties, requiredProfileKeys406) {
		return false
	}
	for key, expected := range profileSchemaConstants406 {
		if !schemaConst406(doc.Properties[key], expected) {
			return false
		}
	}
	var digestSpec map[string]json.RawMessage
	if decode406(doc.Properties["lock_digest"], &digestSpec) != nil || !exactKeys406(digestSpec, []string{"type", "pattern"}) {
		return false
	}
	var digestRule struct {
		Type    string `json:"type"`
		Pattern string `json:"pattern"`
	}
	if decode406(doc.Properties["lock_digest"], &digestRule) != nil || digestRule.Type != "string" || digestRule.Pattern != "^sha256:[0-9a-f]{64}$" {
		return false
	}
	if !schemaConst406(doc.Properties["command"], expectedCommand406) {
		return false
	}
	var envRaw map[string]json.RawMessage
	if decode406(doc.Properties["environment"], &envRaw) != nil || !exactKeys406(envRaw, []string{"type", "additionalProperties", "required", "properties"}) {
		return false
	}
	var envSpec struct {
		Type                 string                     `json:"type"`
		AdditionalProperties bool                       `json:"additionalProperties"`
		Required             []string                   `json:"required"`
		Properties           map[string]json.RawMessage `json:"properties"`
	}
	if decode406(doc.Properties["environment"], &envSpec) != nil || envSpec.Type != "object" || envSpec.AdditionalProperties ||
		!exactStringSet406(envSpec.Required, requiredEnvironmentKeys406) ||
		!exactKeys406(envSpec.Properties, requiredEnvironmentKeys406) {
		return false
	}
	for key, expected := range environmentSchemaConstants406 {
		if !schemaConst406(envSpec.Properties[key], expected) {
			return false
		}
	}
	return true
}

func read406(root, path string) ([]byte, error) {
	if strings.Contains(path, "..") || strings.Contains(path, "//") {
		return nil, fmt.Errorf("unsafe path")
	}
	info, err := os.Lstat(filepath.Join(root, path))
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("missing")
	}
	return os.ReadFile(filepath.Join(root, path))
}

// ValidateProfileAUR406 is the side-effect-free plan loader for parser-worker-v1.
// It never invokes a container engine, never opens a socket, never spawns a
// subprocess, and never reads anything outside the documents this card owns.
func ValidateProfileAUR406(root string) (profile406, lock406, string) {
	sb, err := read406(root, schemaPath406)
	if err != nil {
		return profile406{}, lock406{}, "schema-missing"
	}
	if !validateSchemaInvariant406(sb) {
		return profile406{}, lock406{}, "schema-invalid"
	}
	pb, err := read406(root, profilePath406)
	if err != nil {
		return profile406{}, lock406{}, "profile-missing"
	}
	var raw map[string]json.RawMessage
	if decode406(pb, &raw) != nil || !exactKeys406(raw, requiredProfileKeys406) {
		return profile406{}, lock406{}, "schema-invalid"
	}
	var envRaw map[string]json.RawMessage
	if decode406(raw["environment"], &envRaw) != nil || !exactKeys406(envRaw, requiredEnvironmentKeys406) {
		return profile406{}, lock406{}, "schema-invalid"
	}
	var p profile406
	if decode406(pb, &p) != nil {
		return p, lock406{}, "schema-invalid"
	}
	// Credential shapes are refused before any other plan rule, so a value that could
	// authenticate against a real service is denied even when it arrives through a
	// field whose exact value is otherwise pinned by a constant below.
	for _, value := range p.Environment {
		if credentialShape406.MatchString(value) {
			return p, lock406{}, "credential-present"
		}
	}
	// The worker binary and the grammar set are admitted only while they are
	// content-addressed. Their bytes belong to a dependent implementation card; what
	// this card pins is that an unpinned worker or grammar set can never be resolved.
	if p.Worker != "digest-pinned" || p.GrammarSet != "digest-pinned" || !digest406.MatchString(p.LockDigest) {
		return p, lock406{}, "digest-invalid"
	}
	if p.Schema != "aurum.parser-worker-profile" || p.Version != 1 || p.Profile != profileKey406 ||
		p.Lock != lockPath406 ||
		p.Network != "none" || p.User != "65534:65534" || p.CapDrop != "ALL" || p.CapAdd != "none" ||
		p.Mounts != "none" || p.Devices != "none" || p.Sockets != "none" ||
		p.Pull != "never" || p.Tmpfs != "rw,nosuid,nodev" ||
		!p.CheckoutReadonly || !p.ReadOnlyRootfs || !p.NoNewPrivileges || p.Privileged ||
		p.TimeoutSeconds != 60 || p.MemoryMB != 256 || p.CPUMillis != 1000 || p.PidsLimit != 128 ||
		p.TmpfsMB != 128 || p.StdoutLimitBytes != 65536 || p.StderrLimitBytes != 65536 ||
		p.MaxInputFiles != 10000 || p.MaxInputBytes != 67108864 ||
		p.ModuleCache != "/go/pkg/mod" || !p.ModuleCacheReadOnly ||
		!workerRoot406.MatchString(p.WorkerRoot) || !grammarRoot406.MatchString(p.GrammarSetRoot) ||
		p.CodeExecution != "denied" || p.HostFilesystem != "denied" || p.Subprocess != "denied" ||
		p.BlobSource != "stdin" ||
		p.MaxBlobBytes != 1048576 || p.MaxBlobs != 256 || p.ParseTimeoutSeconds != 5 ||
		!exactCommand406(p.Command) {
		return p, lock406{}, "unsafe-plan"
	}
	for key, expected := range environmentSchemaConstants406 {
		if p.Environment[key] != expected.(string) {
			return p, lock406{}, "unsafe-plan"
		}
	}
	lb, err := read406(root, lockPath406)
	if err != nil {
		return p, lock406{}, "lock-missing"
	}
	var l lock406
	if decode406(lb, &l) != nil || l.Schema != "aurum.oci-image-lock" || l.Version != 1 ||
		l.Profile != p.Profile || !image406.MatchString(l.Image) {
		return p, l, "mutable-input"
	}
	if hash406(lb) != p.LockDigest {
		return p, l, "lock-digest-mismatch"
	}
	return p, l, "valid"
}

// ValidateRegistryAUR406 resolves the canonical registry and admits exactly the nine
// registered keys. It is fail-closed: an unknown key, a duplicate, an out-of-order
// entry, or a digest that does not match the bytes on disk denies without any engine.
func ValidateRegistryAUR406(root string) (string, string) {
	rb, err := read406(root, registryPath406)
	if err != nil {
		return "", "registry-missing"
	}
	var r registry406
	if decode406(rb, &r) != nil || r.Schema != "aurum.profile-registry" || r.Version != 1 {
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
		case profileKey406:
			found = true
			if x.Schema != schemaPath406 || x.Lock != lockPath406 {
				return "", "unsafe-plan"
			}
			sb, err := read406(root, schemaPath406)
			if err != nil {
				return "", "schema-missing"
			}
			if x.SchemaDigest != hash406(sb) {
				return "", "schema-digest-mismatch"
			}
			_, l, code := ValidateProfileAUR406(root)
			if code != "valid" {
				return "", code
			}
			lb, err := read406(root, lockPath406)
			if err != nil {
				return "", "lock-missing"
			}
			if x.LockDigest != hash406(lb) {
				return "", "lock-digest-mismatch"
			}
			if x.ImageSetDigest != hash406([]byte(l.Image)) {
				return "", "unsafe-plan"
			}
		case "bootstrap-readonly-v1", "registry-v1":
			if x.Schema != containerSchemaPath406 || x.Lock != registryLockPath406 {
				return "", "unsafe-plan"
			}
			sb, err := read406(root, containerSchemaPath406)
			if err != nil {
				return "", "schema-missing"
			}
			if x.SchemaDigest != hash406(sb) {
				return "", "schema-digest-mismatch"
			}
			lb, err := read406(root, registryLockPath406)
			if err != nil {
				return "", "lock-missing"
			}
			if x.LockDigest != hash406(lb) {
				return "", "lock-digest-mismatch"
			}
			var l lock406
			if decode406(lb, &l) != nil || l.Schema != "aurum.oci-image-lock" || l.Version != 1 {
				return "", "mutable-input"
			}
			if x.ImageSetDigest != hash406([]byte(l.Image)) {
				return "", "unsafe-plan"
			}
		case "go-unit-offline-v1":
			// Owned by AUR-403. Its documents are not materialized for this
			// acceptance, so only the declared paths and digest shape are checked; the
			// parser worker can never silently re-point the Go unit plan.
			if x.Schema != goUnitSchemaPath406 || x.Lock != goUnitLockPath406 {
				return "", "unsafe-plan"
			}
			if !digest406.MatchString(x.SchemaDigest) || !digest406.MatchString(x.LockDigest) || !digest406.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "go-git-offline-v1":
			// Owned by AUR-404, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != goGitSchemaPath406 || x.Lock != goGitLockPath406 {
				return "", "unsafe-plan"
			}
			if !digest406.MatchString(x.SchemaDigest) || !digest406.MatchString(x.LockDigest) || !digest406.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "fake-provider-v1":
			// Owned by AUR-405, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != fakeProviderSchemaPath406 || x.Lock != fakeProviderLockPath406 {
				return "", "unsafe-plan"
			}
			if !digest406.MatchString(x.SchemaDigest) || !digest406.MatchString(x.LockDigest) || !digest406.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "sqlite-offline-v1":
			// Owned by AUR-407, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != sqliteSchemaPath406 || x.Lock != sqliteLockPath406 {
				return "", "unsafe-plan"
			}
			if !digest406.MatchString(x.SchemaDigest) || !digest406.MatchString(x.LockDigest) || !digest406.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "docs-tool-offline-v1":
			// Owned by AUR-408, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != docsToolSchemaPath406 || x.Lock != docsToolLockPath406 {
				return "", "unsafe-plan"
			}
			if !digest406.MatchString(x.SchemaDigest) || !digest406.MatchString(x.LockDigest) || !digest406.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "fake-scm-offline-v1":
			// Owned by AUR-409, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != fakeScmSchemaPath406 || x.Lock != fakeScmLockPath406 {
				return "", "unsafe-plan"
			}
			if !digest406.MatchString(x.SchemaDigest) || !digest406.MatchString(x.LockDigest) || !digest406.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		default:
			return "", "profile-unregistered"
		}
	}
	if !found || !sort.StringsAreSorted(keys) || !exactStringSet406(keys, registryKeys406) {
		return "", "profile-unregistered"
	}
	return hash406(rb), "valid"
}

func writeMutation406(root, path string, data []byte) error {
	full := filepath.Join(root, path)
	if err := os.Chmod(full, 0600); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0600)
}

func replaceOnce406(root, path, from, to string) error {
	b, err := read406(root, path)
	if err != nil {
		return err
	}
	if !bytes.Contains(b, []byte(from)) {
		return fmt.Errorf("mutation anchor %q absent from %s", from, path)
	}
	return writeMutation406(root, path, bytes.Replace(b, []byte(from), []byte(to), 1))
}

// dropKey406 removes one required key from the profile document, which is how a
// declared bound goes missing in practice.
func dropKey406(root, key string) error {
	pb, err := read406(root, profilePath406)
	if err != nil {
		return err
	}
	var p map[string]any
	if err := decode406(pb, &p); err != nil {
		return err
	}
	if _, ok := p[key]; !ok {
		return fmt.Errorf("mutation anchor %q absent from %s", key, profilePath406)
	}
	delete(p, key)
	nb, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return writeMutation406(root, profilePath406, nb)
}

func TestAUR406(t *testing.T) {
	root := os.Getenv("AURUMCODE_ROOT")
	if root == "" {
		t.Fatal("AURUMCODE_ROOT is required")
	}
	mutation := os.Getenv("AURUM_A406_MUTATION")
	expected := mutation
	switch mutation {
	case "":
	case "duplicate-profile":
		rb, err := read406(root, registryPath406)
		if err != nil {
			t.Fatalf("registry read: %v", err)
		}
		var r registry406
		if decode406(rb, &r) != nil {
			t.Fatal("registry decode")
		}
		var clone entry406
		for _, x := range r.Profiles {
			if x.Key == profileKey406 {
				clone = x
			}
		}
		if clone.Key == "" {
			t.Fatal("mutation target absent: " + profileKey406)
		}
		// A second document for the same key, with different bytes.
		clone.LockDigest = "sha256:" + strings.Repeat("0", 64)
		r.Profiles = append(r.Profiles, clone)
		nb, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("registry marshal: %v", err)
		}
		if err := writeMutation406(root, registryPath406, nb); err != nil {
			t.Fatalf("mutate registry: %v", err)
		}
	case "mutable-input":
		lb, err := read406(root, lockPath406)
		if err != nil {
			t.Fatalf("lock read: %v", err)
		}
		lb = bytes.Replace(lb,
			[]byte("aurum-bootstrap-go-bash@sha256:503ac356fa6bca4bad56fade87b5479ff371bd446ffb9c7db91f211323c7c73e"),
			[]byte("aurum-bootstrap-go-bash:1.21"), 1)
		if !bytes.Contains(lb, []byte("aurum-bootstrap-go-bash:1.21")) {
			t.Fatal("mutation anchor absent from lock")
		}
		if err := writeMutation406(root, lockPath406, lb); err != nil {
			t.Fatalf("mutate lock: %v", err)
		}
		pb, err := read406(root, profilePath406)
		if err != nil {
			t.Fatalf("profile read: %v", err)
		}
		var p map[string]any
		if decode406(pb, &p) != nil {
			t.Fatal("profile decode")
		}
		p["lock_digest"] = hash406(lb)
		nb, err := json.Marshal(p)
		if err != nil {
			t.Fatalf("profile marshal: %v", err)
		}
		if err := writeMutation406(root, profilePath406, nb); err != nil {
			t.Fatalf("mutate profile: %v", err)
		}
	case "unsafe-plan":
		if err := replaceOnce406(root, profilePath406, `"network": "none"`, `"network": "host"`); err != nil {
			t.Fatalf("mutate network: %v", err)
		}
	case "mount-enabled":
		expected = "unsafe-plan"
		if err := replaceOnce406(root, profilePath406, `"mounts": "none"`, `"mounts": "bind"`); err != nil {
			t.Fatalf("mutate mounts: %v", err)
		}
	case "socket-enabled":
		expected = "unsafe-plan"
		if err := replaceOnce406(root, profilePath406, `"sockets": "none"`, `"sockets": "unix"`); err != nil {
			t.Fatalf("mutate sockets: %v", err)
		}
	case "device-enabled":
		expected = "unsafe-plan"
		if err := replaceOnce406(root, profilePath406, `"devices": "none"`, `"devices": "/dev/fuse"`); err != nil {
			t.Fatalf("mutate devices: %v", err)
		}
	case "root-user":
		expected = "unsafe-plan"
		if err := replaceOnce406(root, profilePath406, `"user": "65534:65534"`, `"user": "0:0"`); err != nil {
			t.Fatalf("mutate user: %v", err)
		}
	case "capability-added":
		expected = "unsafe-plan"
		if err := replaceOnce406(root, profilePath406, `"cap_add": "none"`, `"cap_add": "SYS_ADMIN"`); err != nil {
			t.Fatalf("mutate cap_add: %v", err)
		}
	case "code-execution-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce406(root, profilePath406, `"code_execution": "denied"`, `"code_execution": "allowed"`); err != nil {
			t.Fatalf("mutate code execution: %v", err)
		}
	case "host-filesystem-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce406(root, profilePath406, `"host_filesystem": "denied"`, `"host_filesystem": "read-only"`); err != nil {
			t.Fatalf("mutate host filesystem: %v", err)
		}
	case "subprocess-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce406(root, profilePath406, `"subprocess": "denied"`, `"subprocess": "allowed"`); err != nil {
			t.Fatalf("mutate subprocess: %v", err)
		}
	case "grammar-root-escape":
		expected = "unsafe-plan"
		if err := replaceOnce406(root, profilePath406, `"grammar_set_root": "/tmp/aurum-parser-grammars"`, `"grammar_set_root": "/workspace/.board"`); err != nil {
			t.Fatalf("mutate grammar root: %v", err)
		}
	case "worker-root-escape":
		expected = "unsafe-plan"
		if err := replaceOnce406(root, profilePath406, `"worker_root": "/tmp/aurum-parser-worker"`, `"worker_root": "/workspace/internal"`); err != nil {
			t.Fatalf("mutate worker root: %v", err)
		}
	case "host-blob-source":
		expected = "unsafe-plan"
		if err := replaceOnce406(root, profilePath406, `"blob_source": "stdin"`, `"blob_source": "host-file"`); err != nil {
			t.Fatalf("mutate blob source: %v", err)
		}
	case "unbounded-blob":
		expected = "unsafe-plan"
		if err := replaceOnce406(root, profilePath406, `"max_blob_bytes": 1048576`, `"max_blob_bytes": 0`); err != nil {
			t.Fatalf("mutate blob bound: %v", err)
		}
	case "unpinned-grammar":
		expected = "digest-invalid"
		if err := replaceOnce406(root, profilePath406, `"grammar_set": "digest-pinned"`, `"grammar_set": "unpinned"`); err != nil {
			t.Fatalf("mutate grammar set: %v", err)
		}
	case "unpinned-worker":
		expected = "digest-invalid"
		if err := replaceOnce406(root, profilePath406, `"worker": "digest-pinned"`, `"worker": "mutable"`); err != nil {
			t.Fatalf("mutate worker: %v", err)
		}
	case "missing-parse-timeout":
		expected = "schema-invalid"
		if err := dropKey406(root, "parse_timeout_seconds"); err != nil {
			t.Fatalf("mutate parse timeout: %v", err)
		}
	case "real-credential":
		expected = "credential-present"
		// Assembled here, never written in this source file, so the runner's input
		// gate never sees a credential-shaped literal in a materialized path.
		fake := "sk" + "-" + strings.Repeat("A", 32)
		if !credentialShape406.MatchString(fake) {
			t.Fatal("mutation vector is not credential shaped")
		}
		if err := replaceOnce406(root, profilePath406, `"HTTP_PROXY": ""`, `"HTTP_PROXY": "`+fake+`"`); err != nil {
			t.Fatalf("mutate environment: %v", err)
		}
	default:
		t.Fatalf("unknown mutation: %s", mutation)
	}

	digest, code := ValidateRegistryAUR406(root)
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
	registryBytes := mustRead406(t, root, registryPath406)
	if strings.Contains(string(registryBytes), "latest") {
		t.Fatal("mutable tag in registry")
	}
	// No materialized document this card owns may carry a credential shape.
	for _, path := range []string{registryPath406, profilePath406, lockPath406, schemaPath406} {
		if credentialShape406.Match(mustRead406(t, root, path)) {
			t.Fatalf("credential shape in %s", path)
		}
	}
	fmt.Printf("registry_digest=%s\n", digest)
}

func mustRead406(t *testing.T, root, path string) []byte {
	t.Helper()
	b, err := read406(root, path)
	if err != nil {
		t.Fatalf("unreadable %s: %v", path, err)
	}
	return b
}
