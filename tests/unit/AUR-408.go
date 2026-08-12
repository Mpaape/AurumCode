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

const schemaPath408 = ".board/schemas/docs-tool-offline-profile.schema.json"
const profilePath408 = ".board/oci/profiles/docs-tool-offline-v1.json"
const lockPath408 = ".board/locks/oci/docs-tool-offline-v1.lock.json"
const registryPath408 = ".board/oci/profiles/registry.v1.json"
const containerSchemaPath408 = ".board/schemas/container-profile.schema.json"
const registryLockPath408 = ".board/locks/oci/registry-v1.lock.json"
const goUnitSchemaPath408 = ".board/schemas/go-unit-offline-profile.schema.json"
const goUnitLockPath408 = ".board/locks/oci/go-unit-offline-v1.lock.json"
const goGitSchemaPath408 = ".board/schemas/go-git-offline-profile.schema.json"
const goGitLockPath408 = ".board/locks/oci/go-git-offline-v1.lock.json"
const fakeProviderSchemaPath408 = ".board/schemas/fake-provider-profile.schema.json"
const fakeProviderLockPath408 = ".board/locks/oci/fake-provider-v1.lock.json"
const parserWorkerSchemaPath408 = ".board/schemas/parser-worker-profile.schema.json"
const parserWorkerLockPath408 = ".board/locks/oci/parser-worker-v1.lock.json"
const sqliteSchemaPath408 = ".board/schemas/sqlite-offline-profile.schema.json"
const sqliteLockPath408 = ".board/locks/oci/sqlite-offline-v1.lock.json"
const fakeScmSchemaPath408 = ".board/schemas/fake-scm-offline-profile.schema.json"
const fakeScmLockPath408 = ".board/locks/oci/fake-scm-offline-v1.lock.json"
const profileKey408 = "docs-tool-offline-v1"

type profile408 struct {
	Schema               string            `json:"schema"`
	Version              int               `json:"version"`
	Profile              string            `json:"profile"`
	Lock                 string            `json:"lock"`
	LockDigest           string            `json:"lock_digest"`
	Network              string            `json:"network"`
	User                 string            `json:"user"`
	CapDrop              string            `json:"cap_drop"`
	CapAdd               string            `json:"cap_add"`
	Mounts               string            `json:"mounts"`
	Devices              string            `json:"devices"`
	Sockets              string            `json:"sockets"`
	Pull                 string            `json:"pull"`
	Tmpfs                string            `json:"tmpfs"`
	CheckoutReadonly     bool              `json:"checkout_readonly"`
	ReadOnlyRootfs       bool              `json:"read_only_rootfs"`
	NoNewPrivileges      bool              `json:"no_new_privileges"`
	Privileged           bool              `json:"privileged"`
	TimeoutSeconds       int               `json:"timeout_seconds"`
	MemoryMB             int               `json:"memory_mb"`
	CPUMillis            int               `json:"cpu_millis"`
	PidsLimit            int               `json:"pids_limit"`
	TmpfsMB              int               `json:"tmpfs_mb"`
	StdoutLimitBytes     int               `json:"stdout_limit_bytes"`
	StderrLimitBytes     int               `json:"stderr_limit_bytes"`
	MaxInputFiles        int               `json:"max_input_files"`
	MaxInputBytes        int               `json:"max_input_bytes"`
	ModuleCache          string            `json:"module_cache"`
	ModuleCacheReadOnly  bool              `json:"module_cache_read_only"`
	Generator            string            `json:"generator"`
	GeneratorCGO         bool              `json:"generator_cgo"`
	GeneratorRoot        string            `json:"generator_root"`
	Renderer             string            `json:"renderer"`
	PluginSet            string            `json:"plugin_set"`
	NativeSiteTool       string            `json:"native_site_tool"`
	DynamicPlugin        string            `json:"dynamic_plugin"`
	RemoteFetch          string            `json:"remote_fetch"`
	SnippetExecution     string            `json:"snippet_execution"`
	SnippetLanguages     []string          `json:"snippet_languages"`
	FixtureSource        string            `json:"fixture_source"`
	FixtureRoot          string            `json:"fixture_root"`
	OutputMode           string            `json:"output_mode"`
	OutputRoot           string            `json:"output_root"`
	HostOutput           string            `json:"host_output"`
	HostFilesystem       string            `json:"host_filesystem"`
	Subprocess           string            `json:"subprocess"`
	MaxDocuments         int               `json:"max_documents"`
	MaxDocumentBytes     int               `json:"max_document_bytes"`
	MaxOutputBytes       int               `json:"max_output_bytes"`
	RenderTimeoutSeconds int               `json:"render_timeout_seconds"`
	Environment          map[string]string `json:"environment"`
	Command              []string          `json:"command"`
}

type lock408 struct {
	Schema  string `json:"schema"`
	Version int    `json:"version"`
	Profile string `json:"profile"`
	Image   string `json:"image"`
}

type entry408 struct {
	Key            string `json:"key"`
	Schema         string `json:"schema"`
	SchemaDigest   string `json:"schema_digest"`
	Lock           string `json:"lock"`
	LockDigest     string `json:"lock_digest"`
	ImageSetDigest string `json:"image_set_digest"`
}

type registry408 struct {
	Schema   string     `json:"schema"`
	Version  int        `json:"version"`
	Profiles []entry408 `json:"profiles"`
}

var digest408 = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// The documentation plan runs a pure-Go, cgo-free generator and renderer, so it needs a
// Go toolchain and Bash and no native site tool at all. It is pinned to the locally
// present bootstrap image by immutable digest; a mutable tag, a bare name, or any other
// repository denies before the plan is used.
var image408 = regexp.MustCompile(`^aurum-bootstrap-go-bash@sha256:[0-9a-f]{64}$`)

// The generator, its local fixtures and the rendered output live under three bounded
// tmpfs directories and nowhere else. Every expression is anchored on both ends, so a
// checkout path, an absolute host path, a trailing slash, a look-alike sibling and a
// traversal that starts inside the bounded root and climbs out of it are all denied.
var generatorRoot408 = regexp.MustCompile(`^/tmp/aurum-docs-tool$`)
var fixtureRoot408 = regexp.MustCompile(`^/tmp/aurum-docs-fixtures$`)
var outputRoot408 = regexp.MustCompile(`^/tmp/aurum-docs-output$`)

// Credential shapes are assembled from fragments so this source file itself never
// carries a literal that the runner's input gate would read as a real secret.
var credentialShape408 = regexp.MustCompile(`(sk` + `-[A-Za-z0-9_-]{20,}|AKIA` + `[0-9A-Z]{16}|gh` + `[pousr]_[A-Za-z0-9]{20,}|-----BEGIN [A-Z ]*PRIVATE KEY-----)`)

var registryKeys408 = []string{"bootstrap-readonly-v1", "docs-tool-offline-v1", "fake-provider-v1", "fake-scm-offline-v1", "go-git-offline-v1", "go-unit-offline-v1", "parser-worker-v1", "registry-v1", "sqlite-offline-v1"}
var requiredProfileKeys408 = []string{"schema", "version", "profile", "lock", "lock_digest", "network", "user", "cap_drop", "cap_add", "mounts", "devices", "sockets", "pull", "tmpfs", "checkout_readonly", "read_only_rootfs", "no_new_privileges", "privileged", "timeout_seconds", "memory_mb", "cpu_millis", "pids_limit", "tmpfs_mb", "stdout_limit_bytes", "stderr_limit_bytes", "max_input_files", "max_input_bytes", "module_cache", "module_cache_read_only", "generator", "generator_cgo", "generator_root", "renderer", "plugin_set", "native_site_tool", "dynamic_plugin", "remote_fetch", "snippet_execution", "snippet_languages", "fixture_source", "fixture_root", "output_mode", "output_root", "host_output", "host_filesystem", "subprocess", "max_documents", "max_document_bytes", "max_output_bytes", "render_timeout_seconds", "environment", "command"}
var requiredEnvironmentKeys408 = []string{"GOPROXY", "GOSUMDB", "GONOSUMDB", "GOTOOLCHAIN", "CGO_ENABLED", "AURUM_DOCS_GENERATOR_ROOT", "AURUM_DOCS_FIXTURE_ROOT", "AURUM_DOCS_OUTPUT_ROOT", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"}
var schemaDocumentKeys408 = []string{"$schema", "$id", "title", "type", "additionalProperties", "required", "properties"}
var expectedCommand408 = []string{"go", "test", "-mod=readonly", "./..."}

// The renderer emits every fenced block as inert text. This is the closed set of
// languages it will highlight; it is an allowlist, so widening it is a plan change and
// is rejected exactly like enabling execution.
var expectedSnippetLanguages408 = []string{"bash", "go", "json"}

// The safety constants are pinned here, in Go, and not only in the schema bytes. A
// reviewer who relaxes the schema document and recomputes its `schema_digest` in the
// registry moves both files consistently; this map is the third copy that does not move
// with them, so the relaxation is still rejected.
var profileSchemaConstants408 = map[string]any{
	"schema":                 "aurum.docs-tool-offline-profile",
	"version":                1,
	"profile":                profileKey408,
	"lock":                   lockPath408,
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
	"generator":              "digest-pinned",
	"generator_cgo":          false,
	"generator_root":         "/tmp/aurum-docs-tool",
	"renderer":               "digest-pinned",
	"plugin_set":             "digest-pinned",
	"native_site_tool":       "denied",
	"dynamic_plugin":         "denied",
	"remote_fetch":           "denied",
	"snippet_execution":      "denied",
	"snippet_languages":      expectedSnippetLanguages408,
	"fixture_source":         "local",
	"fixture_root":           "/tmp/aurum-docs-fixtures",
	"output_mode":            "ephemeral",
	"output_root":            "/tmp/aurum-docs-output",
	"host_output":            "denied",
	"host_filesystem":        "denied",
	"subprocess":             "denied",
	"max_documents":          64,
	"max_document_bytes":     1048576,
	"max_output_bytes":       33554432,
	"render_timeout_seconds": 5,
}

var environmentSchemaConstants408 = map[string]any{
	"GOPROXY":                   "off",
	"GOSUMDB":                   "off",
	"GONOSUMDB":                 "*",
	"GOTOOLCHAIN":               "local",
	"CGO_ENABLED":               "0",
	"AURUM_DOCS_GENERATOR_ROOT": "/tmp/aurum-docs-tool",
	"AURUM_DOCS_FIXTURE_ROOT":   "/tmp/aurum-docs-fixtures",
	"AURUM_DOCS_OUTPUT_ROOT":    "/tmp/aurum-docs-output",
	"HTTP_PROXY":                "",
	"HTTPS_PROXY":               "",
	"NO_PROXY":                  "*",
}

func hash408(b []byte) string { h := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(h[:]) }

// rejectDuplicateJSON408 refuses a document whose last-wins duplicate key would let a
// registry publish one plan to a reader and another to the runner.
func rejectDuplicateJSON408(b []byte) error {
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

func decode408(b []byte, out any) error {
	if err := rejectDuplicateJSON408(b); err != nil {
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

func exactKeys408(raw map[string]json.RawMessage, expected []string) bool {
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

func exactStringSet408(actual, expected []string) bool {
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

// exactStringList408 compares an ordered list element by element. It is used for the
// command and for the snippet allowlist, so neither a reordering, nor an extra element,
// nor a missing one can pass as the declared value.
func exactStringList408(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for i, token := range expected {
		if actual[i] != token {
			return false
		}
	}
	return true
}

func schemaConst408(raw json.RawMessage, expected any) bool {
	var spec map[string]json.RawMessage
	if decode408(raw, &spec) != nil || !exactKeys408(spec, []string{"const"}) {
		return false
	}
	var actual any
	if decode408(spec["const"], &actual) != nil {
		return false
	}
	actualJSON, actualErr := json.Marshal(actual)
	expectedJSON, expectedErr := json.Marshal(expected)
	return actualErr == nil && expectedErr == nil && bytes.Equal(actualJSON, expectedJSON)
}

func validateSchemaInvariant408(schema []byte) bool {
	var raw map[string]json.RawMessage
	if decode408(schema, &raw) != nil || !exactKeys408(raw, schemaDocumentKeys408) {
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
	if decode408(schema, &doc) != nil ||
		doc.Schema != "https://json-schema.org/draft/2020-12/schema" ||
		doc.ID != "https://aurumcode.dev/schemas/docs-tool-offline-profile.schema.json" ||
		doc.Title != "AurumCode offline documentation tool profile" ||
		doc.Type != "object" || doc.AdditionalProperties ||
		!exactStringSet408(doc.Required, requiredProfileKeys408) ||
		!exactKeys408(doc.Properties, requiredProfileKeys408) {
		return false
	}
	for key, expected := range profileSchemaConstants408 {
		if !schemaConst408(doc.Properties[key], expected) {
			return false
		}
	}
	var digestSpec map[string]json.RawMessage
	if decode408(doc.Properties["lock_digest"], &digestSpec) != nil || !exactKeys408(digestSpec, []string{"type", "pattern"}) {
		return false
	}
	var digestRule struct {
		Type    string `json:"type"`
		Pattern string `json:"pattern"`
	}
	if decode408(doc.Properties["lock_digest"], &digestRule) != nil || digestRule.Type != "string" || digestRule.Pattern != "^sha256:[0-9a-f]{64}$" {
		return false
	}
	if !schemaConst408(doc.Properties["command"], expectedCommand408) {
		return false
	}
	var envRaw map[string]json.RawMessage
	if decode408(doc.Properties["environment"], &envRaw) != nil || !exactKeys408(envRaw, []string{"type", "additionalProperties", "required", "properties"}) {
		return false
	}
	var envSpec struct {
		Type                 string                     `json:"type"`
		AdditionalProperties bool                       `json:"additionalProperties"`
		Required             []string                   `json:"required"`
		Properties           map[string]json.RawMessage `json:"properties"`
	}
	if decode408(doc.Properties["environment"], &envSpec) != nil || envSpec.Type != "object" || envSpec.AdditionalProperties ||
		!exactStringSet408(envSpec.Required, requiredEnvironmentKeys408) ||
		!exactKeys408(envSpec.Properties, requiredEnvironmentKeys408) {
		return false
	}
	for key, expected := range environmentSchemaConstants408 {
		if !schemaConst408(envSpec.Properties[key], expected) {
			return false
		}
	}
	return true
}

func read408(root, path string) ([]byte, error) {
	if strings.Contains(path, "..") || strings.Contains(path, "//") {
		return nil, fmt.Errorf("unsafe path")
	}
	info, err := os.Lstat(filepath.Join(root, path))
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("missing")
	}
	return os.ReadFile(filepath.Join(root, path))
}

// ValidateProfileAUR408 is the side-effect-free plan loader for docs-tool-offline-v1. It
// never invokes a container engine, never opens a socket, never spawns a subprocess,
// never renders a document, and never reads anything outside the documents this card
// owns.
func ValidateProfileAUR408(root string) (profile408, lock408, string) {
	sb, err := read408(root, schemaPath408)
	if err != nil {
		return profile408{}, lock408{}, "schema-missing"
	}
	if !validateSchemaInvariant408(sb) {
		return profile408{}, lock408{}, "schema-invalid"
	}
	pb, err := read408(root, profilePath408)
	if err != nil {
		return profile408{}, lock408{}, "profile-missing"
	}
	var raw map[string]json.RawMessage
	if decode408(pb, &raw) != nil || !exactKeys408(raw, requiredProfileKeys408) {
		return profile408{}, lock408{}, "schema-invalid"
	}
	var envRaw map[string]json.RawMessage
	if decode408(raw["environment"], &envRaw) != nil || !exactKeys408(envRaw, requiredEnvironmentKeys408) {
		return profile408{}, lock408{}, "schema-invalid"
	}
	var p profile408
	if decode408(pb, &p) != nil {
		return p, lock408{}, "schema-invalid"
	}
	// Credential shapes are refused before any other plan rule, so a value that could
	// authenticate against a real service is denied even when it arrives through a
	// field whose exact value is otherwise pinned by a constant below.
	for _, value := range p.Environment {
		if credentialShape408.MatchString(value) {
			return p, lock408{}, "credential-present"
		}
	}
	// Generator, renderer and plugin set are admitted only while they are
	// content-addressed. Their bytes belong to a dependent implementation card; what this
	// card pins is that an unpinned or tag-referenced tool can never be resolved.
	if p.Generator != "digest-pinned" || p.Renderer != "digest-pinned" || p.PluginSet != "digest-pinned" ||
		!digest408.MatchString(p.LockDigest) {
		return p, lock408{}, "digest-invalid"
	}
	// Every bound is an exact constant, never a minimum: a value larger than the one
	// declared here is as unsafe as a value of zero, because the rendered output has to
	// fit inside the bounded tmpfs that holds it.
	if p.Schema != "aurum.docs-tool-offline-profile" || p.Version != 1 || p.Profile != profileKey408 ||
		p.Lock != lockPath408 ||
		p.Network != "none" || p.User != "65534:65534" || p.CapDrop != "ALL" || p.CapAdd != "none" ||
		p.Mounts != "none" || p.Devices != "none" || p.Sockets != "none" ||
		p.Pull != "never" || p.Tmpfs != "rw,nosuid,nodev" ||
		!p.CheckoutReadonly || !p.ReadOnlyRootfs || !p.NoNewPrivileges || p.Privileged ||
		p.TimeoutSeconds != 60 || p.MemoryMB != 256 || p.CPUMillis != 1000 || p.PidsLimit != 128 ||
		p.TmpfsMB != 128 || p.StdoutLimitBytes != 65536 || p.StderrLimitBytes != 65536 ||
		p.MaxInputFiles != 10000 || p.MaxInputBytes != 67108864 ||
		p.ModuleCache != "/go/pkg/mod" || !p.ModuleCacheReadOnly ||
		p.GeneratorCGO || !generatorRoot408.MatchString(p.GeneratorRoot) ||
		p.NativeSiteTool != "denied" || p.DynamicPlugin != "denied" || p.RemoteFetch != "denied" ||
		p.SnippetExecution != "denied" || !exactStringList408(p.SnippetLanguages, expectedSnippetLanguages408) ||
		p.FixtureSource != "local" || !fixtureRoot408.MatchString(p.FixtureRoot) ||
		p.OutputMode != "ephemeral" || !outputRoot408.MatchString(p.OutputRoot) ||
		p.HostOutput != "denied" || p.HostFilesystem != "denied" || p.Subprocess != "denied" ||
		p.MaxDocuments != 64 || p.MaxDocumentBytes != 1048576 ||
		p.MaxOutputBytes != 33554432 || p.RenderTimeoutSeconds != 5 ||
		!exactStringList408(p.Command, expectedCommand408) {
		return p, lock408{}, "unsafe-plan"
	}
	// Independently of the exact constants above, the declared output ceiling has to fit
	// inside the declared tmpfs and the declared corpus has to fit inside the declared
	// input bound. A plan whose output or whose corpus cannot be held by its own bounded
	// filesystem would have to spill somewhere the plan never bounded.
	if p.MaxOutputBytes <= 0 || p.TmpfsMB <= 0 || p.MaxOutputBytes > p.TmpfsMB*1024*1024 {
		return p, lock408{}, "unsafe-plan"
	}
	if p.MaxDocuments <= 0 || p.MaxDocumentBytes <= 0 ||
		p.MaxDocuments > p.MaxInputFiles || p.MaxDocuments*p.MaxDocumentBytes > p.MaxInputBytes {
		return p, lock408{}, "unsafe-plan"
	}
	for key, expected := range environmentSchemaConstants408 {
		if p.Environment[key] != expected.(string) {
			return p, lock408{}, "unsafe-plan"
		}
	}
	lb, err := read408(root, lockPath408)
	if err != nil {
		return p, lock408{}, "lock-missing"
	}
	var l lock408
	if decode408(lb, &l) != nil || l.Schema != "aurum.oci-image-lock" || l.Version != 1 ||
		l.Profile != p.Profile || !image408.MatchString(l.Image) {
		return p, l, "mutable-input"
	}
	if hash408(lb) != p.LockDigest {
		return p, l, "lock-digest-mismatch"
	}
	return p, l, "valid"
}

// ValidateRegistryAUR408 resolves the canonical registry and admits exactly the nine
// registered keys. It is fail-closed: an unknown key, a duplicate, an out-of-order
// entry, or a digest that does not match the bytes on disk denies without any engine.
func ValidateRegistryAUR408(root string) (string, string) {
	rb, err := read408(root, registryPath408)
	if err != nil {
		return "", "registry-missing"
	}
	var r registry408
	if decode408(rb, &r) != nil || r.Schema != "aurum.profile-registry" || r.Version != 1 {
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
		case profileKey408:
			found = true
			if x.Schema != schemaPath408 || x.Lock != lockPath408 {
				return "", "unsafe-plan"
			}
			sb, err := read408(root, schemaPath408)
			if err != nil {
				return "", "schema-missing"
			}
			if x.SchemaDigest != hash408(sb) {
				return "", "schema-digest-mismatch"
			}
			_, l, code := ValidateProfileAUR408(root)
			if code != "valid" {
				return "", code
			}
			lb, err := read408(root, lockPath408)
			if err != nil {
				return "", "lock-missing"
			}
			if x.LockDigest != hash408(lb) {
				return "", "lock-digest-mismatch"
			}
			if x.ImageSetDigest != hash408([]byte(l.Image)) {
				return "", "unsafe-plan"
			}
		case "bootstrap-readonly-v1", "registry-v1":
			if x.Schema != containerSchemaPath408 || x.Lock != registryLockPath408 {
				return "", "unsafe-plan"
			}
			sb, err := read408(root, containerSchemaPath408)
			if err != nil {
				return "", "schema-missing"
			}
			if x.SchemaDigest != hash408(sb) {
				return "", "schema-digest-mismatch"
			}
			lb, err := read408(root, registryLockPath408)
			if err != nil {
				return "", "lock-missing"
			}
			if x.LockDigest != hash408(lb) {
				return "", "lock-digest-mismatch"
			}
			var l lock408
			if decode408(lb, &l) != nil || l.Schema != "aurum.oci-image-lock" || l.Version != 1 {
				return "", "mutable-input"
			}
			if x.ImageSetDigest != hash408([]byte(l.Image)) {
				return "", "unsafe-plan"
			}
		case "go-unit-offline-v1":
			// Owned by AUR-403. Its documents are not materialized for this
			// acceptance, so only the declared paths and digest shape are checked; the
			// documentation tool can never silently re-point the Go unit plan.
			if x.Schema != goUnitSchemaPath408 || x.Lock != goUnitLockPath408 {
				return "", "unsafe-plan"
			}
			if !digest408.MatchString(x.SchemaDigest) || !digest408.MatchString(x.LockDigest) || !digest408.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "go-git-offline-v1":
			// Owned by AUR-404, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != goGitSchemaPath408 || x.Lock != goGitLockPath408 {
				return "", "unsafe-plan"
			}
			if !digest408.MatchString(x.SchemaDigest) || !digest408.MatchString(x.LockDigest) || !digest408.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "fake-provider-v1":
			// Owned by AUR-405, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != fakeProviderSchemaPath408 || x.Lock != fakeProviderLockPath408 {
				return "", "unsafe-plan"
			}
			if !digest408.MatchString(x.SchemaDigest) || !digest408.MatchString(x.LockDigest) || !digest408.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "parser-worker-v1":
			// Owned by AUR-406, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != parserWorkerSchemaPath408 || x.Lock != parserWorkerLockPath408 {
				return "", "unsafe-plan"
			}
			if !digest408.MatchString(x.SchemaDigest) || !digest408.MatchString(x.LockDigest) || !digest408.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "sqlite-offline-v1":
			// Owned by AUR-407, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != sqliteSchemaPath408 || x.Lock != sqliteLockPath408 {
				return "", "unsafe-plan"
			}
			if !digest408.MatchString(x.SchemaDigest) || !digest408.MatchString(x.LockDigest) || !digest408.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "fake-scm-offline-v1":
			// Owned by AUR-409, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != fakeScmSchemaPath408 || x.Lock != fakeScmLockPath408 {
				return "", "unsafe-plan"
			}
			if !digest408.MatchString(x.SchemaDigest) || !digest408.MatchString(x.LockDigest) || !digest408.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		default:
			return "", "profile-unregistered"
		}
	}
	if !found || !sort.StringsAreSorted(keys) || !exactStringSet408(keys, registryKeys408) {
		return "", "profile-unregistered"
	}
	return hash408(rb), "valid"
}

func writeMutation408(root, path string, data []byte) error {
	full := filepath.Join(root, path)
	if err := os.Chmod(full, 0600); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0600)
}

func replaceOnce408(root, path, from, to string) error {
	b, err := read408(root, path)
	if err != nil {
		return err
	}
	if !bytes.Contains(b, []byte(from)) {
		return fmt.Errorf("mutation anchor %q absent from %s", from, path)
	}
	return writeMutation408(root, path, bytes.Replace(b, []byte(from), []byte(to), 1))
}

// dropKey408 removes one required key from the profile document, which is how a
// declared bound goes missing in practice.
func dropKey408(root, key string) error {
	pb, err := read408(root, profilePath408)
	if err != nil {
		return err
	}
	var p map[string]any
	if err := decode408(pb, &p); err != nil {
		return err
	}
	if _, ok := p[key]; !ok {
		return fmt.Errorf("mutation anchor %q absent from %s", key, profilePath408)
	}
	delete(p, key)
	nb, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return writeMutation408(root, profilePath408, nb)
}

func TestAUR408(t *testing.T) {
	root := os.Getenv("AURUMCODE_ROOT")
	if root == "" {
		t.Fatal("AURUMCODE_ROOT is required")
	}
	mutation := os.Getenv("AURUM_A408_MUTATION")
	expected := mutation
	switch mutation {
	case "":
	case "duplicate-profile":
		rb, err := read408(root, registryPath408)
		if err != nil {
			t.Fatalf("registry read: %v", err)
		}
		var r registry408
		if decode408(rb, &r) != nil {
			t.Fatal("registry decode")
		}
		var clone entry408
		for _, x := range r.Profiles {
			if x.Key == profileKey408 {
				clone = x
			}
		}
		if clone.Key == "" {
			t.Fatal("mutation target absent: " + profileKey408)
		}
		// A second document for the same key, with different bytes.
		clone.LockDigest = "sha256:" + strings.Repeat("0", 64)
		r.Profiles = append(r.Profiles, clone)
		nb, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("registry marshal: %v", err)
		}
		if err := writeMutation408(root, registryPath408, nb); err != nil {
			t.Fatalf("mutate registry: %v", err)
		}
	case "mutable-input":
		lb, err := read408(root, lockPath408)
		if err != nil {
			t.Fatalf("lock read: %v", err)
		}
		lb = bytes.Replace(lb,
			[]byte("aurum-bootstrap-go-bash@sha256:503ac356fa6bca4bad56fade87b5479ff371bd446ffb9c7db91f211323c7c73e"),
			[]byte("aurum-bootstrap-go-bash:1.21"), 1)
		if !bytes.Contains(lb, []byte("aurum-bootstrap-go-bash:1.21")) {
			t.Fatal("mutation anchor absent from lock")
		}
		if err := writeMutation408(root, lockPath408, lb); err != nil {
			t.Fatalf("mutate lock: %v", err)
		}
		pb, err := read408(root, profilePath408)
		if err != nil {
			t.Fatalf("profile read: %v", err)
		}
		var p map[string]any
		if decode408(pb, &p) != nil {
			t.Fatal("profile decode")
		}
		p["lock_digest"] = hash408(lb)
		nb, err := json.Marshal(p)
		if err != nil {
			t.Fatalf("profile marshal: %v", err)
		}
		if err := writeMutation408(root, profilePath408, nb); err != nil {
			t.Fatalf("mutate profile: %v", err)
		}
	case "unsafe-plan":
		if err := replaceOnce408(root, profilePath408, `"network": "none"`, `"network": "host"`); err != nil {
			t.Fatalf("mutate network: %v", err)
		}
	case "mount-enabled":
		expected = "unsafe-plan"
		if err := replaceOnce408(root, profilePath408, `"mounts": "none"`, `"mounts": "bind"`); err != nil {
			t.Fatalf("mutate mounts: %v", err)
		}
	case "socket-enabled":
		expected = "unsafe-plan"
		if err := replaceOnce408(root, profilePath408, `"sockets": "none"`, `"sockets": "unix"`); err != nil {
			t.Fatalf("mutate sockets: %v", err)
		}
	case "device-enabled":
		expected = "unsafe-plan"
		if err := replaceOnce408(root, profilePath408, `"devices": "none"`, `"devices": "/dev/fuse"`); err != nil {
			t.Fatalf("mutate devices: %v", err)
		}
	case "root-user":
		expected = "unsafe-plan"
		if err := replaceOnce408(root, profilePath408, `"user": "65534:65534"`, `"user": "0:0"`); err != nil {
			t.Fatalf("mutate user: %v", err)
		}
	case "capability-added":
		expected = "unsafe-plan"
		if err := replaceOnce408(root, profilePath408, `"cap_add": "none"`, `"cap_add": "SYS_ADMIN"`); err != nil {
			t.Fatalf("mutate cap_add: %v", err)
		}
	case "dynamic-plugin-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce408(root, profilePath408, `"dynamic_plugin": "denied"`, `"dynamic_plugin": "allowed"`); err != nil {
			t.Fatalf("mutate dynamic plugin: %v", err)
		}
	case "remote-fetch-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce408(root, profilePath408, `"remote_fetch": "denied"`, `"remote_fetch": "allowed"`); err != nil {
			t.Fatalf("mutate remote fetch: %v", err)
		}
	case "snippet-execution-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce408(root, profilePath408, `"snippet_execution": "denied"`, `"snippet_execution": "allowed"`); err != nil {
			t.Fatalf("mutate snippet execution: %v", err)
		}
	case "snippet-allowlist-widened":
		expected = "unsafe-plan"
		// A language added to the allowlist is a plan change, not a detail: the
		// allowlist is compared element by element, so widening it denies.
		if err := replaceOnce408(root, profilePath408, `"snippet_languages": ["bash", "go", "json"]`, `"snippet_languages": ["bash", "go", "json", "sh"]`); err != nil {
			t.Fatalf("mutate snippet allowlist: %v", err)
		}
	case "native-site-tool-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce408(root, profilePath408, `"native_site_tool": "denied"`, `"native_site_tool": "pandoc"`); err != nil {
			t.Fatalf("mutate native site tool: %v", err)
		}
	case "cgo-generator":
		expected = "unsafe-plan"
		if err := replaceOnce408(root, profilePath408, `"generator_cgo": false`, `"generator_cgo": true`); err != nil {
			t.Fatalf("mutate generator linkage: %v", err)
		}
	case "host-output-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce408(root, profilePath408, `"host_output": "denied"`, `"host_output": "read-write"`); err != nil {
			t.Fatalf("mutate host output: %v", err)
		}
	case "host-filesystem-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce408(root, profilePath408, `"host_filesystem": "denied"`, `"host_filesystem": "read-only"`); err != nil {
			t.Fatalf("mutate host filesystem: %v", err)
		}
	case "subprocess-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce408(root, profilePath408, `"subprocess": "denied"`, `"subprocess": "allowed"`); err != nil {
			t.Fatalf("mutate subprocess: %v", err)
		}
	case "persistent-output":
		expected = "unsafe-plan"
		if err := replaceOnce408(root, profilePath408, `"output_mode": "ephemeral"`, `"output_mode": "persistent"`); err != nil {
			t.Fatalf("mutate output mode: %v", err)
		}
	case "output-root-host":
		expected = "unsafe-plan"
		if err := replaceOnce408(root, profilePath408, `"output_root": "/tmp/aurum-docs-output"`, `"output_root": "/workspace/docs"`); err != nil {
			t.Fatalf("mutate output root: %v", err)
		}
	case "output-root-traversal":
		expected = "unsafe-plan"
		// A root that starts inside the bounded tmpfs and climbs out of it. The anchored
		// expression denies it for the same reason it denies an absolute host path.
		if err := replaceOnce408(root, profilePath408, `"output_root": "/tmp/aurum-docs-output"`, `"output_root": "/tmp/aurum-docs-output/../../etc"`); err != nil {
			t.Fatalf("mutate output root: %v", err)
		}
	case "output-root-trailing-slash":
		expected = "unsafe-plan"
		// A trailing slash is a different string, and a plan is admitted only for the
		// exact declared directory.
		if err := replaceOnce408(root, profilePath408, `"output_root": "/tmp/aurum-docs-output"`, `"output_root": "/tmp/aurum-docs-output/"`); err != nil {
			t.Fatalf("mutate output root: %v", err)
		}
	case "output-root-lookalike":
		expected = "unsafe-plan"
		// A sibling directory whose name merely starts with the bounded root. The
		// expression is anchored at the end as well, so the prefix does not admit it.
		if err := replaceOnce408(root, profilePath408, `"output_root": "/tmp/aurum-docs-output"`, `"output_root": "/tmp/aurum-docs-output-host"`); err != nil {
			t.Fatalf("mutate output root: %v", err)
		}
	case "fixture-root-escape":
		expected = "unsafe-plan"
		if err := replaceOnce408(root, profilePath408, `"fixture_root": "/tmp/aurum-docs-fixtures"`, `"fixture_root": "/workspace/docs"`); err != nil {
			t.Fatalf("mutate fixture root: %v", err)
		}
	case "remote-fixture-source":
		expected = "unsafe-plan"
		if err := replaceOnce408(root, profilePath408, `"fixture_source": "local"`, `"fixture_source": "remote"`); err != nil {
			t.Fatalf("mutate fixture source: %v", err)
		}
	case "generator-root-escape":
		expected = "unsafe-plan"
		if err := replaceOnce408(root, profilePath408, `"generator_root": "/tmp/aurum-docs-tool"`, `"generator_root": "/usr/local/bin"`); err != nil {
			t.Fatalf("mutate generator root: %v", err)
		}
	case "unbounded-output":
		expected = "unsafe-plan"
		if err := replaceOnce408(root, profilePath408, `"max_output_bytes": 33554432`, `"max_output_bytes": 0`); err != nil {
			t.Fatalf("mutate output bound: %v", err)
		}
	case "oversized-output":
		expected = "unsafe-plan"
		// A ceiling far larger than the tmpfs that has to hold the rendered output.
		if err := replaceOnce408(root, profilePath408, `"max_output_bytes": 33554432`, `"max_output_bytes": 34359738368`); err != nil {
			t.Fatalf("mutate output bound: %v", err)
		}
	case "oversized-corpus":
		expected = "unsafe-plan"
		// A corpus whose declared documents cannot fit the declared input bound.
		if err := replaceOnce408(root, profilePath408, `"max_documents": 64`, `"max_documents": 128`); err != nil {
			t.Fatalf("mutate corpus bound: %v", err)
		}
	case "unlimited-tmpfs":
		expected = "unsafe-plan"
		if err := replaceOnce408(root, profilePath408, `"tmpfs_mb": 128`, `"tmpfs_mb": 0`); err != nil {
			t.Fatalf("mutate tmpfs bound: %v", err)
		}
	case "unpinned-generator":
		expected = "digest-invalid"
		if err := replaceOnce408(root, profilePath408, `"generator": "digest-pinned"`, `"generator": "unpinned"`); err != nil {
			t.Fatalf("mutate generator: %v", err)
		}
	case "unpinned-renderer":
		expected = "digest-invalid"
		if err := replaceOnce408(root, profilePath408, `"renderer": "digest-pinned"`, `"renderer": "unpinned"`); err != nil {
			t.Fatalf("mutate renderer: %v", err)
		}
	case "mutable-plugin":
		expected = "digest-invalid"
		// The plugin set referenced by a mutable tag instead of content.
		if err := replaceOnce408(root, profilePath408, `"plugin_set": "digest-pinned"`, `"plugin_set": "latest"`); err != nil {
			t.Fatalf("mutate plugin set: %v", err)
		}
	case "missing-render-timeout":
		expected = "schema-invalid"
		if err := dropKey408(root, "render_timeout_seconds"); err != nil {
			t.Fatalf("mutate render timeout: %v", err)
		}
	case "real-credential":
		expected = "credential-present"
		// Assembled here, never written in this source file, so the runner's input
		// gate never sees a credential-shaped literal in a materialized path.
		fake := "sk" + "-" + strings.Repeat("A", 32)
		if !credentialShape408.MatchString(fake) {
			t.Fatal("mutation vector is not credential shaped")
		}
		if err := replaceOnce408(root, profilePath408, `"HTTP_PROXY": ""`, `"HTTP_PROXY": "`+fake+`"`); err != nil {
			t.Fatalf("mutate environment: %v", err)
		}
	default:
		t.Fatalf("unknown mutation: %s", mutation)
	}

	digest, code := ValidateRegistryAUR408(root)
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
	registryBytes := mustRead408(t, root, registryPath408)
	if strings.Contains(string(registryBytes), "latest") {
		t.Fatal("mutable tag in registry")
	}
	// No materialized document this card owns may carry a credential shape.
	for _, path := range []string{registryPath408, profilePath408, lockPath408, schemaPath408} {
		if credentialShape408.Match(mustRead408(t, root, path)) {
			t.Fatalf("credential shape in %s", path)
		}
	}
	fmt.Printf("registry_digest=%s\n", digest)
}

func mustRead408(t *testing.T, root, path string) []byte {
	t.Helper()
	b, err := read408(root, path)
	if err != nil {
		t.Fatalf("unreadable %s: %v", path, err)
	}
	return b
}
