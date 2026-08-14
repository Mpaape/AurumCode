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

const schemaPath411 = ".board/schemas/polyglot-toolchain-profile.schema.json"
const profilePath411 = ".board/oci/profiles/polyglot-toolchain-v1.json"
const lockPath411 = ".board/locks/oci/polyglot-toolchain-v1.lock.json"
const registryPath411 = ".board/oci/profiles/registry.v1.json"
const containerSchemaPath411 = ".board/schemas/container-profile.schema.json"
const registryLockPath411 = ".board/locks/oci/registry-v1.lock.json"
const goUnitSchemaPath411 = ".board/schemas/go-unit-offline-profile.schema.json"
const goUnitLockPath411 = ".board/locks/oci/go-unit-offline-v1.lock.json"
const goGitSchemaPath411 = ".board/schemas/go-git-offline-profile.schema.json"
const goGitLockPath411 = ".board/locks/oci/go-git-offline-v1.lock.json"
const fakeProviderSchemaPath411 = ".board/schemas/fake-provider-profile.schema.json"
const fakeProviderLockPath411 = ".board/locks/oci/fake-provider-v1.lock.json"
const parserWorkerSchemaPath411 = ".board/schemas/parser-worker-profile.schema.json"
const parserWorkerLockPath411 = ".board/locks/oci/parser-worker-v1.lock.json"
const sqliteSchemaPath411 = ".board/schemas/sqlite-offline-profile.schema.json"
const sqliteLockPath411 = ".board/locks/oci/sqlite-offline-v1.lock.json"
const docsToolSchemaPath411 = ".board/schemas/docs-tool-offline-profile.schema.json"
const docsToolLockPath411 = ".board/locks/oci/docs-tool-offline-v1.lock.json"
const fakeScmSchemaPath411 = ".board/schemas/fake-scm-offline-profile.schema.json"
const fakeScmLockPath411 = ".board/locks/oci/fake-scm-offline-v1.lock.json"
const ociConformanceSchemaPath411 = ".board/schemas/oci-conformance-profile.schema.json"
const ociConformanceLockPath411 = ".board/locks/oci/oci-conformance-v1.lock.json"
const profileKey411 = "polyglot-toolchain-v1"

// The two control names the acceptance measures beside the eight declared runtimes. One
// is created on PATH immediately before the measurement and the other can never exist, so
// a probe that always answers "present" and a probe that always answers "absent" are both
// caught. They are the proof that the availability measurement is live; without them an
// inert probe would silently decide every partition's outcome.
const availabilityControlPresent411 = "aurum-control-present-411"
const availabilityControlAbsent411 = "aurum-control-absent-411"

type partition411 struct {
	Runtime   string `json:"runtime"`
	Toolchain string `json:"toolchain"`
	CacheRoot string `json:"cache_root"`
	CacheMode string `json:"cache_mode"`
	Telemetry string `json:"telemetry"`
}

type profile411 struct {
	Schema                   string                  `json:"schema"`
	Version                  int                     `json:"version"`
	Profile                  string                  `json:"profile"`
	Lock                     string                  `json:"lock"`
	LockDigest               string                  `json:"lock_digest"`
	Network                  string                  `json:"network"`
	User                     string                  `json:"user"`
	CapDrop                  string                  `json:"cap_drop"`
	CapAdd                   string                  `json:"cap_add"`
	Mounts                   string                  `json:"mounts"`
	Devices                  string                  `json:"devices"`
	Sockets                  string                  `json:"sockets"`
	Pull                     string                  `json:"pull"`
	Tmpfs                    string                  `json:"tmpfs"`
	CheckoutReadonly         bool                    `json:"checkout_readonly"`
	ReadOnlyRootfs           bool                    `json:"read_only_rootfs"`
	NoNewPrivileges          bool                    `json:"no_new_privileges"`
	Privileged               bool                    `json:"privileged"`
	TimeoutSeconds           int                     `json:"timeout_seconds"`
	MemoryMB                 int                     `json:"memory_mb"`
	CPUMillis                int                     `json:"cpu_millis"`
	PidsLimit                int                     `json:"pids_limit"`
	TmpfsMB                  int                     `json:"tmpfs_mb"`
	StdoutLimitBytes         int                     `json:"stdout_limit_bytes"`
	StderrLimitBytes         int                     `json:"stderr_limit_bytes"`
	MaxInputFiles            int                     `json:"max_input_files"`
	MaxInputBytes            int                     `json:"max_input_bytes"`
	ModuleCache              string                  `json:"module_cache"`
	ModuleCacheReadOnly      bool                    `json:"module_cache_read_only"`
	PolyglotRoot             string                  `json:"polyglot_root"`
	ToolchainSource          string                  `json:"toolchain_source"`
	ToolchainMaterialization string                  `json:"toolchain_materialization"`
	ToolchainInstall         string                  `json:"toolchain_install"`
	ToolchainBootstrap       string                  `json:"toolchain_bootstrap"`
	PackageInstall           string                  `json:"package_install"`
	PackageRegistry          string                  `json:"package_registry"`
	PackageInstalls          int                     `json:"package_installs"`
	CacheMode                string                  `json:"cache_mode"`
	Telemetry                string                  `json:"telemetry"`
	Egress                   string                  `json:"egress"`
	HostToolchain            string                  `json:"host_toolchain"`
	HostCache                string                  `json:"host_cache"`
	HostFilesystem           string                  `json:"host_filesystem"`
	Subprocess               string                  `json:"subprocess"`
	UnregisteredLanguage     string                  `json:"unregistered_language"`
	Languages                []string                `json:"languages"`
	Toolchains               map[string]partition411 `json:"toolchains"`
	MaxLanguages             int                     `json:"max_languages"`
	MaxPackages              int                     `json:"max_packages"`
	MaxPackageBytes          int                     `json:"max_package_bytes"`
	MaxCacheBytes            int                     `json:"max_cache_bytes"`
	ResolveTimeoutSeconds    int                     `json:"resolve_timeout_seconds"`
	Environment              map[string]string       `json:"environment"`
	Command                  []string                `json:"command"`
}

type lock411 struct {
	Schema  string `json:"schema"`
	Version int    `json:"version"`
	Profile string `json:"profile"`
	Image   string `json:"image"`
}

type entry411 struct {
	Key            string `json:"key"`
	Schema         string `json:"schema"`
	SchemaDigest   string `json:"schema_digest"`
	Lock           string `json:"lock"`
	LockDigest     string `json:"lock_digest"`
	ImageSetDigest string `json:"image_set_digest"`
}

type registry411 struct {
	Schema   string     `json:"schema"`
	Version  int        `json:"version"`
	Profiles []entry411 `json:"profiles"`
}

// languagePlan411 is the isolated offline plan one declared language resolves to. It is
// returned only for a partition whose toolchain is pinned, whose cache is bounded and
// read-only, whose telemetry is denied, and whose runtime was actually measured present.
type languagePlan411 struct {
	Language       string
	Runtime        string
	CacheRoot      string
	CacheMode      string
	Telemetry      string
	Network        string
	PackageInstall string
}

// Every expression below is anchored with \A and \z, which in RE2 are the beginning and
// the end of the *text*, never the beginning and the end of a line. `^`/`$` would be the
// same by default in Go, but the intent is written explicitly here because a value that
// carries an embedded newline is exactly the shape a multiline anchor would admit.
var digest411 = regexp.MustCompile(`\Asha256:[0-9a-f]{64}\z`)

// The polyglot plan is pure document validation with cgo disabled and needs a Go toolchain
// and Bash and nothing else. It is pinned to the locally present bootstrap image by
// immutable digest; a mutable tag, a bare name, or any other repository denies before the
// plan is used.
var image411 = regexp.MustCompile(`\Aaurum-bootstrap-go-bash@sha256:[0-9a-f]{64}\z`)

// The single bounded tmpfs root that holds every language partition, and the closed set of
// per-partition cache roots beneath it. Both are anchored at both ends, so the checkout, an
// absolute host path, a trailing slash, a look-alike sibling, a traversal that starts
// inside the bounded root and climbs out of it, a value with an embedded newline, a leading
// space, an upper-cased spelling, a Unicode look-alike and the empty string are all denied.
var polyglotRoot411 = regexp.MustCompile(`\A/tmp/aurum-polyglot\z`)
var cacheRoot411 = regexp.MustCompile(`\A/tmp/aurum-polyglot/(bash|c-cpp|dotnet|javascript|powershell|python|rust|typescript)\z`)

// The closed set of language partitions this profile may ever name, and the closed set of
// runtime commands they may ever resolve to. Both are denylist-free allowlists: a name
// outside them is refused even when the plan document lists it, so widening the published
// `languages` array is not enough to make an unregistered language admissible.
var language411 = regexp.MustCompile(`\A(bash|c-cpp|dotnet|javascript|powershell|python|rust|typescript)\z`)
var runtime411 = regexp.MustCompile(`\A(bash|cargo|cc|dotnet|node|pwsh|python3|tsc)\z`)

// Credential shapes are assembled from fragments so this source file itself never
// carries a literal that the runner's input gate would read as a real secret.
var credentialShape411 = regexp.MustCompile(`(sk` + `-[A-Za-z0-9_-]{20,}|AKIA` + `[0-9A-Z]{16}|gh` + `[pousr]_[A-Za-z0-9]{20,}|-----BEGIN [A-Z ]*PRIVATE KEY-----)`)

var registryKeys411 = []string{"bootstrap-readonly-v1", "docs-tool-offline-v1", "fake-provider-v1", "fake-scm-offline-v1", "go-git-offline-v1", "go-unit-offline-v1", "oci-conformance-v1", "parser-worker-v1", "polyglot-toolchain-v1", "registry-v1", "sqlite-offline-v1"}
var requiredProfileKeys411 = []string{"schema", "version", "profile", "lock", "lock_digest", "network", "user", "cap_drop", "cap_add", "mounts", "devices", "sockets", "pull", "tmpfs", "checkout_readonly", "read_only_rootfs", "no_new_privileges", "privileged", "timeout_seconds", "memory_mb", "cpu_millis", "pids_limit", "tmpfs_mb", "stdout_limit_bytes", "stderr_limit_bytes", "max_input_files", "max_input_bytes", "module_cache", "module_cache_read_only", "polyglot_root", "toolchain_source", "toolchain_materialization", "toolchain_install", "toolchain_bootstrap", "package_install", "package_registry", "package_installs", "cache_mode", "telemetry", "egress", "host_toolchain", "host_cache", "host_filesystem", "subprocess", "unregistered_language", "languages", "toolchains", "max_languages", "max_packages", "max_package_bytes", "max_cache_bytes", "resolve_timeout_seconds", "environment", "command"}
var requiredEnvironmentKeys411 = []string{"GOPROXY", "GOSUMDB", "GONOSUMDB", "GOTOOLCHAIN", "CGO_ENABLED", "AURUM_POLYGLOT_ROOT", "PIP_NO_INDEX", "PIP_DISABLE_PIP_VERSION_CHECK", "NPM_CONFIG_OFFLINE", "NPM_CONFIG_REGISTRY", "NUGET_PACKAGES", "DOTNET_CLI_TELEMETRY_OPTOUT", "DOTNET_NOLOGO", "CARGO_HOME", "CARGO_NET_OFFLINE", "POWERSHELL_TELEMETRY_OPTOUT", "POWERSHELL_UPDATECHECK", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"}
var schemaDocumentKeys411 = []string{"$schema", "$id", "title", "type", "additionalProperties", "required", "properties"}
var expectedCommand411 = []string{"go", "test", "-mod=readonly", "./..."}

// The eight language partitions, in canonical ascending order, compared element by
// element. Adding a ninth language, dropping one, reordering them or naming one twice is a
// plan change and is rejected exactly like enabling the network.
var expectedLanguages411 = []string{"bash", "c-cpp", "dotnet", "javascript", "powershell", "python", "rust", "typescript"}

// The runtime command each partition resolves to. The map is the third copy of the
// language/runtime binding: it does not move when the schema document and its recomputed
// `schema_digest` move together.
var expectedRuntimes411 = map[string]string{
	"bash":       "bash",
	"c-cpp":      "cc",
	"dotnet":     "dotnet",
	"javascript": "node",
	"powershell": "pwsh",
	"python":     "python3",
	"rust":       "cargo",
	"typescript": "tsc",
}

// The safety constants are pinned here, in Go, and not only in the schema bytes. A
// reviewer who relaxes the schema document and recomputes its `schema_digest` in the
// registry moves both files consistently; this map is the third copy that does not move
// with them, so the relaxation is still rejected.
var profileSchemaConstants411 = map[string]any{
	"schema":                    "aurum.polyglot-toolchain-profile",
	"version":                   1,
	"profile":                   profileKey411,
	"lock":                      lockPath411,
	"network":                   "none",
	"user":                      "65534:65534",
	"cap_drop":                  "ALL",
	"cap_add":                   "none",
	"mounts":                    "none",
	"devices":                   "none",
	"sockets":                   "none",
	"pull":                      "never",
	"tmpfs":                     "rw,nosuid,nodev",
	"checkout_readonly":         true,
	"read_only_rootfs":          true,
	"no_new_privileges":         true,
	"privileged":                false,
	"timeout_seconds":           60,
	"memory_mb":                 256,
	"cpu_millis":                1000,
	"pids_limit":                128,
	"tmpfs_mb":                  128,
	"stdout_limit_bytes":        65536,
	"stderr_limit_bytes":        65536,
	"max_input_files":           10000,
	"max_input_bytes":           67108864,
	"module_cache":              "/go/pkg/mod",
	"module_cache_read_only":    true,
	"polyglot_root":             "/tmp/aurum-polyglot",
	"toolchain_source":          "digest-pinned",
	"toolchain_materialization": "pre-materialized",
	"toolchain_install":         "denied",
	"toolchain_bootstrap":       "denied",
	"package_install":           "denied",
	"package_registry":          "denied",
	"package_installs":          0,
	"cache_mode":                "read-only",
	"telemetry":                 "denied",
	"egress":                    "denied",
	"host_toolchain":            "denied",
	"host_cache":                "denied",
	"host_filesystem":           "denied",
	"subprocess":                "denied",
	"unregistered_language":     "denied",
	"languages":                 expectedLanguages411,
	"max_languages":             8,
	"max_packages":              4096,
	"max_package_bytes":         16384,
	"max_cache_bytes":           8388608,
	"resolve_timeout_seconds":   5,
}

// Every channel through which a package manager would reach an index, a registry or a
// telemetry endpoint is closed by value, not merely unused. Each is pinned by constant, so
// reopening any one of them is a proved mutant.
var environmentSchemaConstants411 = map[string]any{
	"GOPROXY":                       "off",
	"GOSUMDB":                       "off",
	"GONOSUMDB":                     "*",
	"GOTOOLCHAIN":                   "local",
	"CGO_ENABLED":                   "0",
	"AURUM_POLYGLOT_ROOT":           "/tmp/aurum-polyglot",
	"PIP_NO_INDEX":                  "1",
	"PIP_DISABLE_PIP_VERSION_CHECK": "1",
	"NPM_CONFIG_OFFLINE":            "true",
	"NPM_CONFIG_REGISTRY":           "",
	"NUGET_PACKAGES":                "/tmp/aurum-polyglot/dotnet",
	"DOTNET_CLI_TELEMETRY_OPTOUT":   "1",
	"DOTNET_NOLOGO":                 "1",
	"CARGO_HOME":                    "/tmp/aurum-polyglot/rust",
	"CARGO_NET_OFFLINE":             "true",
	"POWERSHELL_TELEMETRY_OPTOUT":   "1",
	"POWERSHELL_UPDATECHECK":        "Off",
	"HTTP_PROXY":                    "",
	"HTTPS_PROXY":                   "",
	"NO_PROXY":                      "*",
}

// The eight partitions pinned in Go as whole objects. This is the copy the schema document
// cannot move: relaxing one partition in the schema bytes and recomputing the registry's
// schema_digest leaves this map behind, and the plan is still rejected.
var toolchainSchemaConstants411 = map[string]any{
	"bash":       map[string]any{"runtime": "bash", "toolchain": "digest-pinned", "cache_root": "/tmp/aurum-polyglot/bash", "cache_mode": "read-only", "telemetry": "denied"},
	"c-cpp":      map[string]any{"runtime": "cc", "toolchain": "digest-pinned", "cache_root": "/tmp/aurum-polyglot/c-cpp", "cache_mode": "read-only", "telemetry": "denied"},
	"dotnet":     map[string]any{"runtime": "dotnet", "toolchain": "digest-pinned", "cache_root": "/tmp/aurum-polyglot/dotnet", "cache_mode": "read-only", "telemetry": "denied"},
	"javascript": map[string]any{"runtime": "node", "toolchain": "digest-pinned", "cache_root": "/tmp/aurum-polyglot/javascript", "cache_mode": "read-only", "telemetry": "denied"},
	"powershell": map[string]any{"runtime": "pwsh", "toolchain": "digest-pinned", "cache_root": "/tmp/aurum-polyglot/powershell", "cache_mode": "read-only", "telemetry": "denied"},
	"python":     map[string]any{"runtime": "python3", "toolchain": "digest-pinned", "cache_root": "/tmp/aurum-polyglot/python", "cache_mode": "read-only", "telemetry": "denied"},
	"rust":       map[string]any{"runtime": "cargo", "toolchain": "digest-pinned", "cache_root": "/tmp/aurum-polyglot/rust", "cache_mode": "read-only", "telemetry": "denied"},
	"typescript": map[string]any{"runtime": "tsc", "toolchain": "digest-pinned", "cache_root": "/tmp/aurum-polyglot/typescript", "cache_mode": "read-only", "telemetry": "denied"},
}

func hash411(b []byte) string { h := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(h[:]) }

// rejectDuplicateJSON411 refuses a document whose last-wins duplicate key would let a
// registry publish one plan to a reader and another to the runner.
func rejectDuplicateJSON411(b []byte) error {
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

func decode411(b []byte, out any) error {
	if err := rejectDuplicateJSON411(b); err != nil {
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

func exactKeys411(raw map[string]json.RawMessage, expected []string) bool {
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

func exactStringSet411(actual, expected []string) bool {
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

// exactStringList411 compares an ordered list element by element. It is used for the
// command and for the language partition list, so neither a reordering, nor an extra
// element, nor a missing one can pass as the declared value.
func exactStringList411(actual, expected []string) bool {
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

func containsString411(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func schemaConst411(raw json.RawMessage, expected any) bool {
	var spec map[string]json.RawMessage
	if decode411(raw, &spec) != nil || !exactKeys411(spec, []string{"const"}) {
		return false
	}
	var actual any
	if decode411(spec["const"], &actual) != nil {
		return false
	}
	actualJSON, actualErr := json.Marshal(actual)
	expectedJSON, expectedErr := json.Marshal(expected)
	return actualErr == nil && expectedErr == nil && bytes.Equal(actualJSON, expectedJSON)
}

func validateSchemaInvariant411(schema []byte) bool {
	var raw map[string]json.RawMessage
	if decode411(schema, &raw) != nil || !exactKeys411(raw, schemaDocumentKeys411) {
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
	if decode411(schema, &doc) != nil ||
		doc.Schema != "https://json-schema.org/draft/2020-12/schema" ||
		doc.ID != "https://aurumcode.dev/schemas/polyglot-toolchain-profile.schema.json" ||
		doc.Title != "AurumCode polyglot toolchain profile" ||
		doc.Type != "object" || doc.AdditionalProperties ||
		!exactStringSet411(doc.Required, requiredProfileKeys411) ||
		!exactKeys411(doc.Properties, requiredProfileKeys411) {
		return false
	}
	for key, expected := range profileSchemaConstants411 {
		if !schemaConst411(doc.Properties[key], expected) {
			return false
		}
	}
	var digestSpec map[string]json.RawMessage
	if decode411(doc.Properties["lock_digest"], &digestSpec) != nil || !exactKeys411(digestSpec, []string{"type", "pattern"}) {
		return false
	}
	var digestRule struct {
		Type    string `json:"type"`
		Pattern string `json:"pattern"`
	}
	if decode411(doc.Properties["lock_digest"], &digestRule) != nil || digestRule.Type != "string" || digestRule.Pattern != "^sha256:[0-9a-f]{64}$" {
		return false
	}
	if !schemaConst411(doc.Properties["command"], expectedCommand411) {
		return false
	}
	// The eight partitions are a closed object whose every member is pinned as a whole.
	var toolchainRaw map[string]json.RawMessage
	if decode411(doc.Properties["toolchains"], &toolchainRaw) != nil || !exactKeys411(toolchainRaw, []string{"type", "additionalProperties", "required", "properties"}) {
		return false
	}
	var toolchainSpec struct {
		Type                 string                     `json:"type"`
		AdditionalProperties bool                       `json:"additionalProperties"`
		Required             []string                   `json:"required"`
		Properties           map[string]json.RawMessage `json:"properties"`
	}
	if decode411(doc.Properties["toolchains"], &toolchainSpec) != nil || toolchainSpec.Type != "object" || toolchainSpec.AdditionalProperties ||
		!exactStringSet411(toolchainSpec.Required, expectedLanguages411) ||
		!exactKeys411(toolchainSpec.Properties, expectedLanguages411) {
		return false
	}
	for key, expected := range toolchainSchemaConstants411 {
		if !schemaConst411(toolchainSpec.Properties[key], expected) {
			return false
		}
	}
	var envRaw map[string]json.RawMessage
	if decode411(doc.Properties["environment"], &envRaw) != nil || !exactKeys411(envRaw, []string{"type", "additionalProperties", "required", "properties"}) {
		return false
	}
	var envSpec struct {
		Type                 string                     `json:"type"`
		AdditionalProperties bool                       `json:"additionalProperties"`
		Required             []string                   `json:"required"`
		Properties           map[string]json.RawMessage `json:"properties"`
	}
	if decode411(doc.Properties["environment"], &envSpec) != nil || envSpec.Type != "object" || envSpec.AdditionalProperties ||
		!exactStringSet411(envSpec.Required, requiredEnvironmentKeys411) ||
		!exactKeys411(envSpec.Properties, requiredEnvironmentKeys411) {
		return false
	}
	for key, expected := range environmentSchemaConstants411 {
		if !schemaConst411(envSpec.Properties[key], expected) {
			return false
		}
	}
	return true
}

func read411(root, path string) ([]byte, error) {
	if strings.Contains(path, "..") || strings.Contains(path, "//") {
		return nil, fmt.Errorf("unsafe path")
	}
	info, err := os.Lstat(filepath.Join(root, path))
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("missing")
	}
	return os.ReadFile(filepath.Join(root, path))
}

// ResolveLanguageAUR411 resolves one declared language to its isolated offline plan.
//
// `availability` is the *measured* set of runtime commands: a key is present only when the
// caller actually observed that runtime, and its value says whether the observation found
// it. A runtime that was never observed is not "absent" and is certainly not "available" --
// it is unmeasured, and it denies with its own code. Absence of evidence is never read as
// evidence, in either direction.
//
// The function invokes nothing: it never executes a runtime, never spawns a subprocess,
// never installs a package and never touches the network. It answers from the registered
// document and the measurement it is handed.
func ResolveLanguageAUR411(p profile411, language string, availability map[string]bool) (languagePlan411, string) {
	var zero languagePlan411
	// A plan that tolerates an unregistered language has no partition boundary left, so
	// no language resolves under it at all.
	if p.UnregisteredLanguage != "denied" {
		return zero, "unsafe-plan"
	}
	// The closed expression is consulted before the published list, so widening
	// `languages` can never make `java`, `ruby` or `go` admissible, and neither an
	// upper-cased spelling, a leading space, an embedded newline, a trailing slash, a
	// Unicode look-alike nor the empty string can impersonate a declared partition.
	if !language411.MatchString(language) || !containsString411(p.Languages, language) {
		return zero, "language-unregistered"
	}
	part, ok := p.Toolchains[language]
	if !ok {
		return zero, "language-unregistered"
	}
	// The toolchain is admitted only while it is content-addressed, at both the plan
	// level and the partition level.
	if p.ToolchainSource != "digest-pinned" || part.Toolchain != "digest-pinned" {
		return zero, "unpinned-toolchain"
	}
	// The partition is isolated: its cache is the one bounded directory that belongs to
	// this language, it is read-only, and its telemetry is denied. The partition's own
	// spelling of the shared policy must agree with the plan's, because a partition that
	// disagreed would be a hole in a policy the plan claims is global.
	if !runtime411.MatchString(part.Runtime) || part.Runtime != expectedRuntimes411[language] ||
		!cacheRoot411.MatchString(part.CacheRoot) || part.CacheRoot != p.PolyglotRoot+"/"+language ||
		part.CacheMode != "read-only" || part.CacheMode != p.CacheMode ||
		part.Telemetry != "denied" || part.Telemetry != p.Telemetry {
		return zero, "unsafe-plan"
	}
	// Resolving a partition may never create one. A plan that could install a toolchain,
	// install a package, bootstrap a runtime, reach a package registry or open the
	// network is not the plan this key publishes.
	if p.ToolchainInstall != "denied" || p.ToolchainBootstrap != "denied" ||
		p.PackageInstall != "denied" || p.PackageRegistry != "denied" ||
		p.ToolchainMaterialization != "pre-materialized" ||
		p.Network != "none" || p.Egress != "denied" ||
		p.HostToolchain != "denied" || p.HostCache != "denied" || p.Subprocess != "denied" {
		return zero, "unsafe-plan"
	}
	// The measurement, and only the measurement, decides whether this partition can run.
	// The plan never installs, so a runtime that is not already materialized can never
	// become available: the honest answer is to fail closed, not to promise a plan that
	// would have to create what it denies.
	available, measured := availability[part.Runtime]
	if !measured {
		return zero, "toolchain-unmeasured"
	}
	if !available {
		return zero, "toolchain-unavailable"
	}
	return languagePlan411{
		Language:       language,
		Runtime:        part.Runtime,
		CacheRoot:      part.CacheRoot,
		CacheMode:      part.CacheMode,
		Telemetry:      part.Telemetry,
		Network:        p.Network,
		PackageInstall: p.PackageInstall,
	}, "valid"
}

// ResolvePolyglotAUR411 is the entry point a consumer would call. It validates the whole
// registry and then the profile, and resolves a language only from a plan both admitted.
//
// This is where the measurement is put in its place: the availability argument can decide
// that an admitted partition is unavailable, and it can never decide that a rejected plan
// is usable. A measurement reporting all eight runtimes present cannot rescue a plan the
// loader refused, because the refusal is returned before the measurement is ever read.
func ResolvePolyglotAUR411(root, language string, availability map[string]bool) (languagePlan411, string) {
	if _, code := ValidateRegistryAUR411(root); code != "valid" {
		return languagePlan411{}, code
	}
	p, _, code := ValidateProfileAUR411(root)
	if code != "valid" {
		return languagePlan411{}, code
	}
	return ResolveLanguageAUR411(p, language, availability)
}

// ValidateProfileAUR411 is the side-effect-free plan loader for polyglot-toolchain-v1. It
// never installs a toolchain, never installs a package, never invokes a runtime, never
// spawns a subprocess, never opens a network socket, and never reads anything outside the
// documents this card owns.
func ValidateProfileAUR411(root string) (profile411, lock411, string) {
	sb, err := read411(root, schemaPath411)
	if err != nil {
		return profile411{}, lock411{}, "schema-missing"
	}
	if !validateSchemaInvariant411(sb) {
		return profile411{}, lock411{}, "schema-invalid"
	}
	pb, err := read411(root, profilePath411)
	if err != nil {
		return profile411{}, lock411{}, "profile-missing"
	}
	var raw map[string]json.RawMessage
	if decode411(pb, &raw) != nil || !exactKeys411(raw, requiredProfileKeys411) {
		return profile411{}, lock411{}, "schema-invalid"
	}
	var envRaw map[string]json.RawMessage
	if decode411(raw["environment"], &envRaw) != nil || !exactKeys411(envRaw, requiredEnvironmentKeys411) {
		return profile411{}, lock411{}, "schema-invalid"
	}
	var toolchainRaw map[string]json.RawMessage
	if decode411(raw["toolchains"], &toolchainRaw) != nil || !exactKeys411(toolchainRaw, expectedLanguages411) {
		return profile411{}, lock411{}, "schema-invalid"
	}
	var p profile411
	if decode411(pb, &p) != nil {
		return p, lock411{}, "schema-invalid"
	}
	// Credential shapes are refused before any other plan rule, so a value that could
	// authenticate against a package registry is denied even when it arrives through a
	// field whose exact value is otherwise pinned by a constant below.
	for _, value := range p.Environment {
		if credentialShape411.MatchString(value) {
			return p, lock411{}, "credential-present"
		}
	}
	// Every toolchain is admitted only while it is content-addressed. The toolchain bytes
	// belong to a dependent implementation card; what this card pins is that an unpinned
	// or tag-referenced toolchain can never be resolved, so a partition cannot change
	// under a fixed plan.
	if p.ToolchainSource != "digest-pinned" || !digest411.MatchString(p.LockDigest) {
		return p, lock411{}, "digest-invalid"
	}
	for _, language := range expectedLanguages411 {
		if p.Toolchains[language].Toolchain != "digest-pinned" {
			return p, lock411{}, "digest-invalid"
		}
	}
	// Every bound is an exact constant, never a minimum: a value larger than the one
	// declared here is as unsafe as a value of zero, because every partition cache has to
	// fit inside the one bounded tmpfs that holds all eight at once.
	if p.Schema != "aurum.polyglot-toolchain-profile" || p.Version != 1 || p.Profile != profileKey411 ||
		p.Lock != lockPath411 ||
		p.Network != "none" || p.User != "65534:65534" || p.CapDrop != "ALL" || p.CapAdd != "none" ||
		p.Mounts != "none" || p.Devices != "none" || p.Sockets != "none" ||
		p.Pull != "never" || p.Tmpfs != "rw,nosuid,nodev" ||
		!p.CheckoutReadonly || !p.ReadOnlyRootfs || !p.NoNewPrivileges || p.Privileged ||
		p.TimeoutSeconds != 60 || p.MemoryMB != 256 || p.CPUMillis != 1000 || p.PidsLimit != 128 ||
		p.TmpfsMB != 128 || p.StdoutLimitBytes != 65536 || p.StderrLimitBytes != 65536 ||
		p.MaxInputFiles != 10000 || p.MaxInputBytes != 67108864 ||
		p.ModuleCache != "/go/pkg/mod" || !p.ModuleCacheReadOnly ||
		!polyglotRoot411.MatchString(p.PolyglotRoot) ||
		p.ToolchainMaterialization != "pre-materialized" ||
		p.ToolchainInstall != "denied" || p.ToolchainBootstrap != "denied" ||
		p.PackageInstall != "denied" || p.PackageRegistry != "denied" || p.PackageInstalls != 0 ||
		p.CacheMode != "read-only" || p.Telemetry != "denied" || p.Egress != "denied" ||
		p.HostToolchain != "denied" || p.HostCache != "denied" || p.HostFilesystem != "denied" ||
		p.Subprocess != "denied" || p.UnregisteredLanguage != "denied" ||
		!exactStringList411(p.Languages, expectedLanguages411) ||
		p.MaxLanguages != 8 || p.MaxPackages != 4096 || p.MaxPackageBytes != 16384 ||
		p.MaxCacheBytes != 8388608 || p.ResolveTimeoutSeconds != 5 ||
		!exactStringList411(p.Command, expectedCommand411) {
		return p, lock411{}, "unsafe-plan"
	}
	// The partition set has to exist and be closed: every declared language must be one
	// this profile registers, no language may be named twice, the published order must be
	// canonically ascending, and the `languages` array and the `toolchains` object must
	// describe the same set -- a language listed in one and missing from the other is a
	// partition nobody validates.
	//
	// These checks are defence in depth, not the defence. The pinned equality
	// `exactStringList411(p.Languages, expectedLanguages411)` above fixes the array to the
	// same eight names in the same order, and the exact `toolchains` key set required when
	// the document was decoded fixes the object to those same eight, so no document that
	// reaches here can fail them and no current mutant isolates them. They stand for a
	// dependent card that builds either side from something other than the pinned table.
	if len(p.Languages) == 0 || len(p.Languages) != len(p.Toolchains) || len(p.Languages) > p.MaxLanguages {
		return p, lock411{}, "unsafe-plan"
	}
	if !sort.StringsAreSorted(p.Languages) {
		return p, lock411{}, "unsafe-plan"
	}
	namedLanguages := map[string]bool{}
	for _, language := range p.Languages {
		if !language411.MatchString(language) || namedLanguages[language] {
			return p, lock411{}, "unsafe-plan"
		}
		namedLanguages[language] = true
		if _, ok := p.Toolchains[language]; !ok {
			return p, lock411{}, "unsafe-plan"
		}
	}
	for language := range p.Toolchains {
		if !namedLanguages[language] {
			return p, lock411{}, "unsafe-plan"
		}
	}
	// Each partition is isolated from every other: two partitions may never share a cache
	// root and may never share a runtime -- either collision would silently merge two
	// languages the plan promises to keep apart -- and no cache root may be, or contain,
	// the polyglot root itself.
	//
	// What actually refuses a collision is the pair of pinned equalities below,
	// `part.Runtime != expectedRuntimes411[language]` and
	// `part.CacheRoot != p.PolyglotRoot+"/"+language`: expectedRuntimes411 is injective
	// over the eight languages and each cache root is derived from its own language, so a
	// document that clears them cannot collide. The distinctness and containment rules
	// below are therefore defence in depth -- evaluated on every partition, never the
	// reason a document is refused, and isolated by no current mutant. This was checked by
	// deletion: with the distinctness block removed, shared-runtime and shared-cache-root
	// stay rejected and the acceptance stays at exit 0.
	seenRoots := map[string]bool{}
	seenRuntimes := map[string]bool{}
	for _, language := range p.Languages {
		part := p.Toolchains[language]
		if part.Runtime == "" || part.CacheRoot == "" {
			return p, lock411{}, "unsafe-plan"
		}
		if !runtime411.MatchString(part.Runtime) || part.Runtime != expectedRuntimes411[language] {
			return p, lock411{}, "unsafe-plan"
		}
		if !cacheRoot411.MatchString(part.CacheRoot) || part.CacheRoot != p.PolyglotRoot+"/"+language {
			return p, lock411{}, "unsafe-plan"
		}
		if part.CacheMode != p.CacheMode || part.CacheMode != "read-only" ||
			part.Telemetry != p.Telemetry || part.Telemetry != "denied" {
			return p, lock411{}, "unsafe-plan"
		}
		if seenRoots[part.CacheRoot] || seenRuntimes[part.Runtime] {
			return p, lock411{}, "unsafe-plan"
		}
		seenRoots[part.CacheRoot] = true
		seenRuntimes[part.Runtime] = true
		if part.CacheRoot == p.PolyglotRoot || strings.HasPrefix(p.PolyglotRoot, part.CacheRoot+"/") ||
			!strings.HasPrefix(part.CacheRoot, p.PolyglotRoot+"/") {
			return p, lock411{}, "unsafe-plan"
		}
		for _, other := range p.Languages {
			if other == language {
				continue
			}
			otherRoot := p.Toolchains[other].CacheRoot
			if strings.HasPrefix(otherRoot, part.CacheRoot+"/") {
				return p, lock411{}, "unsafe-plan"
			}
		}
	}
	// The caches of every declared partition coexist inside the one bounded tmpfs, so their
	// combined ceiling has to fit it. A per-partition ceiling that fits alone but not eight
	// times over would have to spill somewhere the plan never bounded.
	//
	// Defence in depth again: `p.MaxCacheBytes != 8388608` above is a pinned equality that
	// refuses unbounded-cache, oversized-cache and aggregate-cache-overflow before either
	// relation below is reached, so no current mutant isolates them. Checked by deletion:
	// with both relations removed, all three stay rejected and the acceptance stays at
	// exit 0.
	if p.MaxCacheBytes <= 0 || p.TmpfsMB <= 0 || p.MaxCacheBytes > p.TmpfsMB*1024*1024 {
		return p, lock411{}, "unsafe-plan"
	}
	if len(p.Languages)*p.MaxCacheBytes > p.TmpfsMB*1024*1024 {
		return p, lock411{}, "unsafe-plan"
	}
	// And the package corpus has to fit inside the declared input bound, because every
	// cached package is a materialized input. Same standing: `p.MaxPackages != 4096` above
	// refuses oversized-package-set first, so this relation is verified and never the
	// reason for a rejection any current mutant produces.
	if p.MaxPackages <= 0 || p.MaxPackageBytes <= 0 ||
		p.MaxPackages > p.MaxInputFiles || p.MaxPackages*p.MaxPackageBytes > p.MaxInputBytes {
		return p, lock411{}, "unsafe-plan"
	}
	if p.ResolveTimeoutSeconds <= 0 || p.ResolveTimeoutSeconds > p.TimeoutSeconds {
		return p, lock411{}, "unsafe-plan"
	}
	for key, expected := range environmentSchemaConstants411 {
		if p.Environment[key] != expected.(string) {
			return p, lock411{}, "unsafe-plan"
		}
	}
	// The environment spelling of a bounded directory and the declared one are the same
	// directory. If they ever disagree, a package manager reads or writes a path the plan
	// never bounded.
	if p.Environment["AURUM_POLYGLOT_ROOT"] != p.PolyglotRoot ||
		p.Environment["NUGET_PACKAGES"] != p.Toolchains["dotnet"].CacheRoot ||
		p.Environment["CARGO_HOME"] != p.Toolchains["rust"].CacheRoot {
		return p, lock411{}, "unsafe-plan"
	}
	lb, err := read411(root, lockPath411)
	if err != nil {
		return p, lock411{}, "lock-missing"
	}
	var l lock411
	if decode411(lb, &l) != nil || l.Schema != "aurum.oci-image-lock" || l.Version != 1 ||
		l.Profile != p.Profile || !image411.MatchString(l.Image) {
		return p, l, "mutable-input"
	}
	if hash411(lb) != p.LockDigest {
		return p, l, "lock-digest-mismatch"
	}
	return p, l, "valid"
}

// ValidateRegistryAUR411 resolves the canonical registry and admits exactly the eleven
// registered keys. It is fail-closed: an unknown key, a duplicate, an out-of-order
// entry, or a digest that does not match the bytes on disk denies without any engine.
func ValidateRegistryAUR411(root string) (string, string) {
	rb, err := read411(root, registryPath411)
	if err != nil {
		return "", "registry-missing"
	}
	var r registry411
	if decode411(rb, &r) != nil || r.Schema != "aurum.profile-registry" || r.Version != 1 {
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
		case profileKey411:
			found = true
			if x.Schema != schemaPath411 || x.Lock != lockPath411 {
				return "", "unsafe-plan"
			}
			sb, err := read411(root, schemaPath411)
			if err != nil {
				return "", "schema-missing"
			}
			if x.SchemaDigest != hash411(sb) {
				return "", "schema-digest-mismatch"
			}
			_, l, code := ValidateProfileAUR411(root)
			if code != "valid" {
				return "", code
			}
			lb, err := read411(root, lockPath411)
			if err != nil {
				return "", "lock-missing"
			}
			if x.LockDigest != hash411(lb) {
				return "", "lock-digest-mismatch"
			}
			if x.ImageSetDigest != hash411([]byte(l.Image)) {
				return "", "unsafe-plan"
			}
		case "bootstrap-readonly-v1", "registry-v1":
			if x.Schema != containerSchemaPath411 || x.Lock != registryLockPath411 {
				return "", "unsafe-plan"
			}
			sb, err := read411(root, containerSchemaPath411)
			if err != nil {
				return "", "schema-missing"
			}
			if x.SchemaDigest != hash411(sb) {
				return "", "schema-digest-mismatch"
			}
			lb, err := read411(root, registryLockPath411)
			if err != nil {
				return "", "lock-missing"
			}
			if x.LockDigest != hash411(lb) {
				return "", "lock-digest-mismatch"
			}
			var l lock411
			if decode411(lb, &l) != nil || l.Schema != "aurum.oci-image-lock" || l.Version != 1 {
				return "", "mutable-input"
			}
			if x.ImageSetDigest != hash411([]byte(l.Image)) {
				return "", "unsafe-plan"
			}
		case "go-unit-offline-v1":
			// Owned by AUR-403. Its documents are not materialized for this
			// acceptance, so only the declared paths and digest shape are checked; the
			// polyglot profile can never silently re-point the Go unit plan.
			if x.Schema != goUnitSchemaPath411 || x.Lock != goUnitLockPath411 {
				return "", "unsafe-plan"
			}
			if !digest411.MatchString(x.SchemaDigest) || !digest411.MatchString(x.LockDigest) || !digest411.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "go-git-offline-v1":
			// Owned by AUR-404, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != goGitSchemaPath411 || x.Lock != goGitLockPath411 {
				return "", "unsafe-plan"
			}
			if !digest411.MatchString(x.SchemaDigest) || !digest411.MatchString(x.LockDigest) || !digest411.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "fake-provider-v1":
			// Owned by AUR-405, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != fakeProviderSchemaPath411 || x.Lock != fakeProviderLockPath411 {
				return "", "unsafe-plan"
			}
			if !digest411.MatchString(x.SchemaDigest) || !digest411.MatchString(x.LockDigest) || !digest411.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "parser-worker-v1":
			// Owned by AUR-406, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != parserWorkerSchemaPath411 || x.Lock != parserWorkerLockPath411 {
				return "", "unsafe-plan"
			}
			if !digest411.MatchString(x.SchemaDigest) || !digest411.MatchString(x.LockDigest) || !digest411.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "sqlite-offline-v1":
			// Owned by AUR-407, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != sqliteSchemaPath411 || x.Lock != sqliteLockPath411 {
				return "", "unsafe-plan"
			}
			if !digest411.MatchString(x.SchemaDigest) || !digest411.MatchString(x.LockDigest) || !digest411.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "docs-tool-offline-v1":
			// Owned by AUR-408, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != docsToolSchemaPath411 || x.Lock != docsToolLockPath411 {
				return "", "unsafe-plan"
			}
			if !digest411.MatchString(x.SchemaDigest) || !digest411.MatchString(x.LockDigest) || !digest411.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "fake-scm-offline-v1":
			// Owned by AUR-409, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != fakeScmSchemaPath411 || x.Lock != fakeScmLockPath411 {
				return "", "unsafe-plan"
			}
			if !digest411.MatchString(x.SchemaDigest) || !digest411.MatchString(x.LockDigest) || !digest411.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "oci-conformance-v1":
			// Owned by AUR-410, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != ociConformanceSchemaPath411 || x.Lock != ociConformanceLockPath411 {
				return "", "unsafe-plan"
			}
			if !digest411.MatchString(x.SchemaDigest) || !digest411.MatchString(x.LockDigest) || !digest411.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		default:
			return "", "profile-unregistered"
		}
	}
	if !found || !sort.StringsAreSorted(keys) || !exactStringSet411(keys, registryKeys411) {
		return "", "profile-unregistered"
	}
	return hash411(rb), "valid"
}

func writeMutation411(root, path string, data []byte) error {
	full := filepath.Join(root, path)
	if err := os.Chmod(full, 0600); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0600)
}

func replaceOnce411(root, path, from, to string) error {
	b, err := read411(root, path)
	if err != nil {
		return err
	}
	if !bytes.Contains(b, []byte(from)) {
		return fmt.Errorf("mutation anchor %q absent from %s", from, path)
	}
	return writeMutation411(root, path, bytes.Replace(b, []byte(from), []byte(to), 1))
}

// dropKey411 removes one required key from the profile document, which is how a
// declared bound goes missing in practice.
func dropKey411(root, key string) error {
	pb, err := read411(root, profilePath411)
	if err != nil {
		return err
	}
	var p map[string]any
	if err := decode411(pb, &p); err != nil {
		return err
	}
	if _, ok := p[key]; !ok {
		return fmt.Errorf("mutation anchor %q absent from %s", key, profilePath411)
	}
	delete(p, key)
	nb, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return writeMutation411(root, profilePath411, nb)
}

// syntheticAvailability411 builds a measurement that was never taken from a machine. It is
// used to exercise the resolution logic in all three of its states without claiming that
// any locally present image carries the eight runtimes; the acceptance's real measurement
// is handled separately and is never replaced by this.
func syntheticAvailability411(available bool) map[string]bool {
	out := map[string]bool{}
	for _, language := range expectedLanguages411 {
		out[expectedRuntimes411[language]] = available
	}
	return out
}

// assertResolutionIsDriven411 proves the availability argument is load-bearing before any
// measurement is trusted to it. The same profile is resolved three times over the same
// eight languages: with every runtime measured present, with every runtime measured
// absent, and with nothing measured at all. If the argument were ignored the three would
// agree, and a real measurement afterwards would be decorative.
func assertResolutionIsDriven411(t *testing.T, p profile411) {
	t.Helper()
	for _, probe := range []struct {
		label        string
		availability map[string]bool
		want         string
	}{
		{"all-present", syntheticAvailability411(true), "valid"},
		{"all-absent", syntheticAvailability411(false), "toolchain-unavailable"},
		{"unmeasured", map[string]bool{}, "toolchain-unmeasured"},
	} {
		roots := map[string]bool{}
		runtimes := map[string]bool{}
		for _, language := range expectedLanguages411 {
			plan, code := ResolveLanguageAUR411(p, language, probe.availability)
			if code != probe.want {
				t.Fatalf("%s: language %s resolved %s, want %s", probe.label, language, code, probe.want)
			}
			if probe.want != "valid" {
				if plan != (languagePlan411{}) {
					t.Fatalf("%s: language %s returned a plan while denying", probe.label, language)
				}
				continue
			}
			// A nominal language produces an isolated offline plan: its own cache root,
			// its own runtime, read-only, telemetry denied, no network, no install.
			if plan.Language != language || plan.Runtime != expectedRuntimes411[language] ||
				plan.CacheRoot != p.PolyglotRoot+"/"+language ||
				plan.CacheMode != "read-only" || plan.Telemetry != "denied" ||
				plan.Network != "none" || plan.PackageInstall != "denied" {
				t.Fatalf("%s: language %s resolved a non-isolated plan: %+v", probe.label, language, plan)
			}
			if roots[plan.CacheRoot] || runtimes[plan.Runtime] {
				t.Fatalf("%s: language %s shares a cache root or runtime with another partition", probe.label, language)
			}
			roots[plan.CacheRoot] = true
			runtimes[plan.Runtime] = true
		}
		if probe.want == "valid" && (len(roots) != len(expectedLanguages411) || len(runtimes) != len(expectedLanguages411)) {
			t.Fatalf("%s: %d roots and %d runtimes for %d languages", probe.label, len(roots), len(runtimes), len(expectedLanguages411))
		}
	}
	fmt.Printf("resolution_driven=true languages=%d\n", len(expectedLanguages411))
}

// unregisteredLanguages411 are the names that must never resolve: another language
// entirely, the runtime command mistaken for the partition name, and every impersonation
// of a declared partition -- upper-cased, space-padded, newline-carrying, slash-suffixed,
// prefix-sharing, Cyrillic-homoglyph and empty.
var unregisteredLanguages411 = []string{
	"", "java", "ruby", "perl", "go", "golang", "sh", "cpp", "js", "ts", "csharp",
	"python3", "node", "pwsh", "cargo", "tsc", "cc",
	"Bash", "BASH", "Python", " bash", "bash ", "bash\n", "bash\npython", "bash/", "/bash",
	"bash-host", "pythonx", "python-host", "rust;python", "pythоn",
}

// availabilityObservation411 is one line of the measurement the acceptance takes on the
// pristine PATH, before any shim exists.
type availabilityObservation411 struct {
	name    string
	present bool
}

// readAvailabilityLedger411 reads the measurement the acceptance wrote. Every failure mode
// is a hard failure and never a silent default: an unset variable, an unreadable file, a
// malformed line, a missing name, an extra name and a duplicate all stop the test. An
// absent ledger is not "nothing was available" and it is not "everything was available";
// it is a measurement that did not happen, and a test that treated it as either would be
// reporting a result it never observed.
func readAvailabilityLedger411(t *testing.T) map[string]bool {
	t.Helper()
	ledger := os.Getenv("AURUM_A411_AVAILABILITY_LOG")
	if ledger == "" {
		t.Fatal("AURUM_A411_AVAILABILITY_LOG is required: the toolchain availability measurement did not run")
	}
	b, err := os.ReadFile(ledger)
	if err != nil {
		t.Fatalf("availability ledger %s is unreadable: %v", ledger, err)
	}
	observations := []availabilityObservation411{}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 || (fields[1] != "present" && fields[1] != "absent") {
			t.Fatalf("availability ledger line %q is malformed", line)
		}
		observations = append(observations, availabilityObservation411{name: fields[0], present: fields[1] == "present"})
	}
	measured := map[string]bool{}
	for _, observation := range observations {
		if _, duplicate := measured[observation.name]; duplicate {
			t.Fatalf("availability ledger records %s twice", observation.name)
		}
		measured[observation.name] = observation.present
	}
	// The two controls prove the probe is live in both directions. A probe hard-wired to
	// "present" fails on the control that cannot exist; a probe hard-wired to "absent"
	// fails on the control the acceptance placed on PATH immediately before measuring.
	present, ok := measured[availabilityControlPresent411]
	if !ok || !present {
		t.Fatalf("availability probe is inert: control %s was not observed present", availabilityControlPresent411)
	}
	absent, ok := measured[availabilityControlAbsent411]
	if !ok || absent {
		t.Fatalf("availability probe is inert: control %s was not observed absent", availabilityControlAbsent411)
	}
	delete(measured, availabilityControlPresent411)
	delete(measured, availabilityControlAbsent411)
	// Exactly the eight declared runtimes, no more and no fewer. A ledger that skipped one
	// would hand the loader an unmeasured runtime and quietly change which code it returns.
	if len(measured) != len(expectedLanguages411) {
		t.Fatalf("availability ledger records %d runtimes, want %d", len(measured), len(expectedLanguages411))
	}
	for _, language := range expectedLanguages411 {
		if _, ok := measured[expectedRuntimes411[language]]; !ok {
			t.Fatalf("availability ledger never measured the %s runtime %s", language, expectedRuntimes411[language])
		}
	}
	return measured
}

// measuredPackageInstalls411 reads the package-install ledger the acceptance places ahead
// of PATH. A declared `package_installs: 0` is a claim; this is the measurement that has to
// agree with it. Every failure mode is hard: an unset variable and an unreadable or absent
// file are a measurement that did not happen, never a zero.
func measuredPackageInstalls411(t *testing.T) int {
	t.Helper()
	ledger := os.Getenv("AURUM_A411_INSTALL_LOG")
	if ledger == "" {
		t.Fatal("AURUM_A411_INSTALL_LOG is required: the package-install measurement did not run")
	}
	b, err := os.ReadFile(ledger)
	if err != nil {
		t.Fatalf("package-install ledger %s is unreadable: %v", ledger, err)
	}
	count := 0
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func TestAUR411(t *testing.T) {
	root := os.Getenv("AURUMCODE_ROOT")
	if root == "" {
		t.Fatal("AURUMCODE_ROOT is required")
	}
	mutation := os.Getenv("AURUM_A411_MUTATION")
	expected := mutation
	switch mutation {
	case "":
	case "duplicate-profile":
		rb, err := read411(root, registryPath411)
		if err != nil {
			t.Fatalf("registry read: %v", err)
		}
		var r registry411
		if decode411(rb, &r) != nil {
			t.Fatal("registry decode")
		}
		var clone entry411
		for _, x := range r.Profiles {
			if x.Key == profileKey411 {
				clone = x
			}
		}
		if clone.Key == "" {
			t.Fatal("mutation target absent: " + profileKey411)
		}
		// A second document for the same key, with different bytes.
		clone.LockDigest = "sha256:" + strings.Repeat("0", 64)
		r.Profiles = append(r.Profiles, clone)
		nb, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("registry marshal: %v", err)
		}
		if err := writeMutation411(root, registryPath411, nb); err != nil {
			t.Fatalf("mutate registry: %v", err)
		}
	case "mutable-input":
		lb, err := read411(root, lockPath411)
		if err != nil {
			t.Fatalf("lock read: %v", err)
		}
		lb = bytes.Replace(lb,
			[]byte("aurum-bootstrap-go-bash@sha256:503ac356fa6bca4bad56fade87b5479ff371bd446ffb9c7db91f211323c7c73e"),
			[]byte("aurum-bootstrap-go-bash:1.21"), 1)
		if !bytes.Contains(lb, []byte("aurum-bootstrap-go-bash:1.21")) {
			t.Fatal("mutation anchor absent from lock")
		}
		if err := writeMutation411(root, lockPath411, lb); err != nil {
			t.Fatalf("mutate lock: %v", err)
		}
		pb, err := read411(root, profilePath411)
		if err != nil {
			t.Fatalf("profile read: %v", err)
		}
		var p map[string]any
		if decode411(pb, &p) != nil {
			t.Fatal("profile decode")
		}
		p["lock_digest"] = hash411(lb)
		nb, err := json.Marshal(p)
		if err != nil {
			t.Fatalf("profile marshal: %v", err)
		}
		if err := writeMutation411(root, profilePath411, nb); err != nil {
			t.Fatalf("mutate profile: %v", err)
		}
	case "unsafe-plan":
		if err := replaceOnce411(root, profilePath411, `"network": "none"`, `"network": "host"`); err != nil {
			t.Fatalf("mutate network: %v", err)
		}
	case "mount-enabled":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"mounts": "none"`, `"mounts": "bind"`); err != nil {
			t.Fatalf("mutate mounts: %v", err)
		}
	case "socket-enabled":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"sockets": "none"`, `"sockets": "/var/run/docker.sock"`); err != nil {
			t.Fatalf("mutate sockets: %v", err)
		}
	case "device-enabled":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"devices": "none"`, `"devices": "/dev/fuse"`); err != nil {
			t.Fatalf("mutate devices: %v", err)
		}
	case "root-user":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"user": "65534:65534"`, `"user": "0:0"`); err != nil {
			t.Fatalf("mutate user: %v", err)
		}
	case "privileged-enabled":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"privileged": false`, `"privileged": true`); err != nil {
			t.Fatalf("mutate privileged: %v", err)
		}
	case "capability-added":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"cap_add": "none"`, `"cap_add": "SYS_ADMIN"`); err != nil {
			t.Fatalf("mutate cap_add: %v", err)
		}
	case "capability-kept":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"cap_drop": "ALL"`, `"cap_drop": "NET_RAW"`); err != nil {
			t.Fatalf("mutate cap_drop: %v", err)
		}
	case "writable-rootfs":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"read_only_rootfs": true`, `"read_only_rootfs": false`); err != nil {
			t.Fatalf("mutate read only rootfs: %v", err)
		}
	case "new-privileges-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"no_new_privileges": true`, `"no_new_privileges": false`); err != nil {
			t.Fatalf("mutate no new privileges: %v", err)
		}
	case "writable-checkout":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"checkout_readonly": true`, `"checkout_readonly": false`); err != nil {
			t.Fatalf("mutate checkout readonly: %v", err)
		}
	case "egress-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"egress": "denied"`, `"egress": "allowed"`); err != nil {
			t.Fatalf("mutate egress: %v", err)
		}
	case "toolchain-install-allowed":
		expected = "unsafe-plan"
		// The single field that turns a profile which pins already-materialized
		// toolchains into one that can fetch and create them.
		if err := replaceOnce411(root, profilePath411, `"toolchain_install": "denied"`, `"toolchain_install": "allowed"`); err != nil {
			t.Fatalf("mutate toolchain install: %v", err)
		}
	case "toolchain-bootstrap-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"toolchain_bootstrap": "denied"`, `"toolchain_bootstrap": "allowed"`); err != nil {
			t.Fatalf("mutate toolchain bootstrap: %v", err)
		}
	case "materialization-on-demand":
		expected = "unsafe-plan"
		// "on-demand" is an install with a friendlier name.
		if err := replaceOnce411(root, profilePath411, `"toolchain_materialization": "pre-materialized"`, `"toolchain_materialization": "on-demand"`); err != nil {
			t.Fatalf("mutate materialization: %v", err)
		}
	case "package-install-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"package_install": "denied"`, `"package_install": "allowed"`); err != nil {
			t.Fatalf("mutate package install: %v", err)
		}
	case "package-registry-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"package_registry": "denied"`, `"package_registry": "https://registry.npmjs.org/"`); err != nil {
			t.Fatalf("mutate package registry: %v", err)
		}
	case "package-installs-nonzero":
		expected = "unsafe-plan"
		// The declared install count moved away from zero. This card publishes a
		// document; a plan that declares an install is a different plan.
		if err := replaceOnce411(root, profilePath411, `"package_installs": 0`, `"package_installs": 1`); err != nil {
			t.Fatalf("mutate package installs: %v", err)
		}
	case "telemetry-enabled":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"telemetry": "denied",`, `"telemetry": "enabled",`); err != nil {
			t.Fatalf("mutate telemetry: %v", err)
		}
	case "partition-telemetry-enabled":
		expected = "unsafe-plan"
		// One partition opts back into telemetry while the plan-level policy still
		// denies it. A global policy with one exempt partition is not a policy.
		if err := replaceOnce411(root, profilePath411, `"cache_root": "/tmp/aurum-polyglot/python", "cache_mode": "read-only", "telemetry": "denied"`, `"cache_root": "/tmp/aurum-polyglot/python", "cache_mode": "read-only", "telemetry": "enabled"`); err != nil {
			t.Fatalf("mutate partition telemetry: %v", err)
		}
	case "cache-writable":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"cache_mode": "read-only",`, `"cache_mode": "read-write",`); err != nil {
			t.Fatalf("mutate cache mode: %v", err)
		}
	case "partition-cache-writable":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"cache_root": "/tmp/aurum-polyglot/rust", "cache_mode": "read-only"`, `"cache_root": "/tmp/aurum-polyglot/rust", "cache_mode": "read-write"`); err != nil {
			t.Fatalf("mutate partition cache mode: %v", err)
		}
	case "host-toolchain-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"host_toolchain": "denied"`, `"host_toolchain": "allowed"`); err != nil {
			t.Fatalf("mutate host toolchain: %v", err)
		}
	case "host-cache-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"host_cache": "denied"`, `"host_cache": "read-only"`); err != nil {
			t.Fatalf("mutate host cache: %v", err)
		}
	case "host-filesystem-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"host_filesystem": "denied"`, `"host_filesystem": "read-only"`); err != nil {
			t.Fatalf("mutate host filesystem: %v", err)
		}
	case "subprocess-allowed":
		expected = "unsafe-plan"
		// A subprocess is how a package manager would be reached even without a declared
		// registry endpoint.
		if err := replaceOnce411(root, profilePath411, `"subprocess": "denied"`, `"subprocess": "allowed"`); err != nil {
			t.Fatalf("mutate subprocess: %v", err)
		}
	case "unregistered-language-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"unregistered_language": "denied"`, `"unregistered_language": "allowed"`); err != nil {
			t.Fatalf("mutate unregistered language: %v", err)
		}
	case "language-list-widened":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"languages": ["bash", "c-cpp", "dotnet", "javascript", "powershell", "python", "rust", "typescript"]`, `"languages": ["bash", "c-cpp", "dotnet", "java", "javascript", "powershell", "python", "rust", "typescript"]`); err != nil {
			t.Fatalf("mutate languages: %v", err)
		}
	case "language-list-shrunk":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"languages": ["bash", "c-cpp", "dotnet", "javascript", "powershell", "python", "rust", "typescript"]`, `"languages": ["bash", "c-cpp", "dotnet", "javascript", "powershell", "python", "rust"]`); err != nil {
			t.Fatalf("mutate languages: %v", err)
		}
	case "language-list-unsorted":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"languages": ["bash", "c-cpp", "dotnet", "javascript", "powershell", "python", "rust", "typescript"]`, `"languages": ["c-cpp", "bash", "dotnet", "javascript", "powershell", "python", "rust", "typescript"]`); err != nil {
			t.Fatalf("mutate languages: %v", err)
		}
	case "language-list-duplicated":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"languages": ["bash", "c-cpp", "dotnet", "javascript", "powershell", "python", "rust", "typescript"]`, `"languages": ["bash", "bash", "c-cpp", "dotnet", "javascript", "powershell", "python", "rust"]`); err != nil {
			t.Fatalf("mutate languages: %v", err)
		}
	case "language-list-empty":
		expected = "unsafe-plan"
		// No language at all is not a safe default: it is a plan that can never be
		// executed and whose partition set proves nothing.
		if err := replaceOnce411(root, profilePath411, `"languages": ["bash", "c-cpp", "dotnet", "javascript", "powershell", "python", "rust", "typescript"]`, `"languages": []`); err != nil {
			t.Fatalf("mutate languages: %v", err)
		}
	case "language-partition-mismatch":
		expected = "unsafe-plan"
		// The published list and the partition object describe different sets: `java` is
		// listed and has no partition, `typescript` has a partition nobody lists.
		if err := replaceOnce411(root, profilePath411, `"languages": ["bash", "c-cpp", "dotnet", "javascript", "powershell", "python", "rust", "typescript"]`, `"languages": ["bash", "c-cpp", "dotnet", "java", "javascript", "powershell", "python", "rust"]`); err != nil {
			t.Fatalf("mutate languages: %v", err)
		}
	case "shared-runtime":
		expected = "unsafe-plan"
		// Two partitions collapse onto one runtime. The anchored expression admits
		// `node` for both; the pinned runtime equality is what refuses it, and the
		// distinctness rule behind it never gets the chance.
		if err := replaceOnce411(root, profilePath411, `"typescript": {"runtime": "tsc"`, `"typescript": {"runtime": "node"`); err != nil {
			t.Fatalf("mutate runtime: %v", err)
		}
	case "shared-cache-root":
		expected = "unsafe-plan"
		// Two partitions collapse onto one cache directory. The anchored expression
		// admits the value for both; the pinned cache-root equality is what refuses it,
		// and the distinctness rule behind it never gets the chance.
		if err := replaceOnce411(root, profilePath411, `"cache_root": "/tmp/aurum-polyglot/typescript"`, `"cache_root": "/tmp/aurum-polyglot/javascript"`); err != nil {
			t.Fatalf("mutate cache root: %v", err)
		}
	case "runtime-remapped":
		expected = "unsafe-plan"
		// A partition keeps its name and points at another language's runtime.
		if err := replaceOnce411(root, profilePath411, `"python": {"runtime": "python3"`, `"python": {"runtime": "bash"`); err != nil {
			t.Fatalf("mutate runtime: %v", err)
		}
	case "polyglot-root-host":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"polyglot_root": "/tmp/aurum-polyglot"`, `"polyglot_root": "/workspace"`); err != nil {
			t.Fatalf("mutate polyglot root: %v", err)
		}
	case "polyglot-root-traversal":
		expected = "unsafe-plan"
		// A root that starts inside the bounded tmpfs and climbs out of it. The anchored
		// expression denies it for the same reason it denies an absolute host path.
		if err := replaceOnce411(root, profilePath411, `"polyglot_root": "/tmp/aurum-polyglot"`, `"polyglot_root": "/tmp/aurum-polyglot/../../etc"`); err != nil {
			t.Fatalf("mutate polyglot root: %v", err)
		}
	case "polyglot-root-trailing-slash":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"polyglot_root": "/tmp/aurum-polyglot"`, `"polyglot_root": "/tmp/aurum-polyglot/"`); err != nil {
			t.Fatalf("mutate polyglot root: %v", err)
		}
	case "polyglot-root-lookalike":
		expected = "unsafe-plan"
		// A sibling directory whose name merely starts with the bounded root.
		if err := replaceOnce411(root, profilePath411, `"polyglot_root": "/tmp/aurum-polyglot"`, `"polyglot_root": "/tmp/aurum-polyglot-host"`); err != nil {
			t.Fatalf("mutate polyglot root: %v", err)
		}
	case "polyglot-root-newline":
		expected = "unsafe-plan"
		// The admitted value followed by a newline and a second copy of itself. A
		// multiline anchor would accept this; \A and \z do not.
		if err := replaceOnce411(root, profilePath411, `"polyglot_root": "/tmp/aurum-polyglot"`, `"polyglot_root": "/tmp/aurum-polyglot\n/tmp/aurum-polyglot"`); err != nil {
			t.Fatalf("mutate polyglot root: %v", err)
		}
	case "polyglot-root-trailing-newline":
		expected = "unsafe-plan"
		// The admitted value with a single trailing newline and nothing else.
		if err := replaceOnce411(root, profilePath411, `"polyglot_root": "/tmp/aurum-polyglot"`, `"polyglot_root": "/tmp/aurum-polyglot\n"`); err != nil {
			t.Fatalf("mutate polyglot root: %v", err)
		}
	case "polyglot-root-leading-space":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"polyglot_root": "/tmp/aurum-polyglot"`, `"polyglot_root": " /tmp/aurum-polyglot"`); err != nil {
			t.Fatalf("mutate polyglot root: %v", err)
		}
	case "polyglot-root-uppercase":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"polyglot_root": "/tmp/aurum-polyglot"`, `"polyglot_root": "/TMP/AURUM-POLYGLOT"`); err != nil {
			t.Fatalf("mutate polyglot root: %v", err)
		}
	case "polyglot-root-homoglyph":
		expected = "unsafe-plan"
		// The first `o` of `polyglot` replaced by the Cyrillic small letter O (U+043E).
		// The path reads identically and is a different directory.
		if err := replaceOnce411(root, profilePath411, `"polyglot_root": "/tmp/aurum-polyglot"`, "\"polyglot_root\": \"/tmp/aurum-pоlyglot\""); err != nil {
			t.Fatalf("mutate polyglot root: %v", err)
		}
	case "polyglot-root-empty":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"polyglot_root": "/tmp/aurum-polyglot"`, `"polyglot_root": ""`); err != nil {
			t.Fatalf("mutate polyglot root: %v", err)
		}
	case "cache-root-escape":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"cache_root": "/tmp/aurum-polyglot/python"`, `"cache_root": "/workspace/.board"`); err != nil {
			t.Fatalf("mutate cache root: %v", err)
		}
	case "cache-root-traversal":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"cache_root": "/tmp/aurum-polyglot/python"`, `"cache_root": "/tmp/aurum-polyglot/python/../../../etc"`); err != nil {
			t.Fatalf("mutate cache root: %v", err)
		}
	case "cache-root-trailing-slash":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"cache_root": "/tmp/aurum-polyglot/python"`, `"cache_root": "/tmp/aurum-polyglot/python/"`); err != nil {
			t.Fatalf("mutate cache root: %v", err)
		}
	case "cache-root-lookalike":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"cache_root": "/tmp/aurum-polyglot/python"`, `"cache_root": "/tmp/aurum-polyglot/python-host"`); err != nil {
			t.Fatalf("mutate cache root: %v", err)
		}
	case "cache-root-newline":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"cache_root": "/tmp/aurum-polyglot/python"`, `"cache_root": "/tmp/aurum-polyglot/python\n/tmp/aurum-polyglot/python"`); err != nil {
			t.Fatalf("mutate cache root: %v", err)
		}
	case "cache-root-trailing-newline":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"cache_root": "/tmp/aurum-polyglot/python"`, `"cache_root": "/tmp/aurum-polyglot/python\n"`); err != nil {
			t.Fatalf("mutate cache root: %v", err)
		}
	case "cache-root-leading-space":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"cache_root": "/tmp/aurum-polyglot/python"`, `"cache_root": " /tmp/aurum-polyglot/python"`); err != nil {
			t.Fatalf("mutate cache root: %v", err)
		}
	case "cache-root-uppercase":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"cache_root": "/tmp/aurum-polyglot/python"`, `"cache_root": "/TMP/AURUM-POLYGLOT/PYTHON"`); err != nil {
			t.Fatalf("mutate cache root: %v", err)
		}
	case "cache-root-homoglyph":
		expected = "unsafe-plan"
		// The `o` of `python` replaced by the Cyrillic small letter O (U+043E).
		if err := replaceOnce411(root, profilePath411, `"cache_root": "/tmp/aurum-polyglot/python"`, "\"cache_root\": \"/tmp/aurum-polyglot/pythоn\""); err != nil {
			t.Fatalf("mutate cache root: %v", err)
		}
	case "cache-root-empty":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"cache_root": "/tmp/aurum-polyglot/python"`, `"cache_root": ""`); err != nil {
			t.Fatalf("mutate cache root: %v", err)
		}
	case "cache-root-outside-polyglot-root":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"cache_root": "/tmp/aurum-polyglot/python"`, `"cache_root": "/tmp/aurum-other/python"`); err != nil {
			t.Fatalf("mutate cache root: %v", err)
		}
	case "unbounded-cache":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"max_cache_bytes": 8388608`, `"max_cache_bytes": 0`); err != nil {
			t.Fatalf("mutate cache bound: %v", err)
		}
	case "oversized-cache":
		expected = "unsafe-plan"
		// A ceiling far larger than the tmpfs that has to hold the partition.
		if err := replaceOnce411(root, profilePath411, `"max_cache_bytes": 8388608`, `"max_cache_bytes": 34359738368`); err != nil {
			t.Fatalf("mutate cache bound: %v", err)
		}
	case "aggregate-cache-overflow":
		expected = "unsafe-plan"
		// 32 MiB fits the 128 MiB tmpfs on its own, and eight of them do not. The pinned
		// `max_cache_bytes` equality is what refuses it; the aggregate relation behind it
		// never gets the chance.
		if err := replaceOnce411(root, profilePath411, `"max_cache_bytes": 8388608`, `"max_cache_bytes": 33554432`); err != nil {
			t.Fatalf("mutate cache bound: %v", err)
		}
	case "oversized-package-set":
		expected = "unsafe-plan"
		// A package corpus whose declared entries cannot fit the declared input bound.
		if err := replaceOnce411(root, profilePath411, `"max_packages": 4096`, `"max_packages": 8192`); err != nil {
			t.Fatalf("mutate package bound: %v", err)
		}
	case "unbounded-package-bytes":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"max_package_bytes": 16384`, `"max_package_bytes": 0`); err != nil {
			t.Fatalf("mutate package bound: %v", err)
		}
	case "unbounded-resolve-timeout":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"resolve_timeout_seconds": 5`, `"resolve_timeout_seconds": 0`); err != nil {
			t.Fatalf("mutate resolve timeout: %v", err)
		}
	case "resolve-timeout-exceeds-run":
		expected = "unsafe-plan"
		// A per-language deadline that outlives the run deadline bounds nothing.
		if err := replaceOnce411(root, profilePath411, `"resolve_timeout_seconds": 5`, `"resolve_timeout_seconds": 600`); err != nil {
			t.Fatalf("mutate resolve timeout: %v", err)
		}
	case "max-languages-lowered":
		expected = "unsafe-plan"
		// The declared partition count exceeds the plan's own ceiling for it.
		if err := replaceOnce411(root, profilePath411, `"max_languages": 8`, `"max_languages": 4`); err != nil {
			t.Fatalf("mutate max languages: %v", err)
		}
	case "unlimited-tmpfs":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"tmpfs_mb": 128`, `"tmpfs_mb": 0`); err != nil {
			t.Fatalf("mutate tmpfs bound: %v", err)
		}
	case "pip-index-reopened":
		expected = "unsafe-plan"
		// The Python index re-opened through the environment instead of the plan field.
		if err := replaceOnce411(root, profilePath411, `"PIP_NO_INDEX": "1"`, `"PIP_NO_INDEX": "0"`); err != nil {
			t.Fatalf("mutate pip environment: %v", err)
		}
	case "npm-offline-disabled":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"NPM_CONFIG_OFFLINE": "true"`, `"NPM_CONFIG_OFFLINE": "false"`); err != nil {
			t.Fatalf("mutate npm environment: %v", err)
		}
	case "npm-registry-reopened":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"NPM_CONFIG_REGISTRY": ""`, `"NPM_CONFIG_REGISTRY": "https://registry.npmjs.org/"`); err != nil {
			t.Fatalf("mutate npm environment: %v", err)
		}
	case "nuget-cache-escape":
		expected = "unsafe-plan"
		// The dotnet package cache pointed outside the partition it belongs to. The
		// cross-check between the environment spelling and the declared root refuses it.
		if err := replaceOnce411(root, profilePath411, `"NUGET_PACKAGES": "/tmp/aurum-polyglot/dotnet"`, `"NUGET_PACKAGES": "/workspace/.nuget"`); err != nil {
			t.Fatalf("mutate nuget environment: %v", err)
		}
	case "dotnet-telemetry-reopened":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"DOTNET_CLI_TELEMETRY_OPTOUT": "1"`, `"DOTNET_CLI_TELEMETRY_OPTOUT": "0"`); err != nil {
			t.Fatalf("mutate dotnet environment: %v", err)
		}
	case "cargo-offline-disabled":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"CARGO_NET_OFFLINE": "true"`, `"CARGO_NET_OFFLINE": "false"`); err != nil {
			t.Fatalf("mutate cargo environment: %v", err)
		}
	case "cargo-home-escape":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"CARGO_HOME": "/tmp/aurum-polyglot/rust"`, `"CARGO_HOME": "/workspace/.cargo"`); err != nil {
			t.Fatalf("mutate cargo environment: %v", err)
		}
	case "powershell-telemetry-reopened":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"POWERSHELL_TELEMETRY_OPTOUT": "1"`, `"POWERSHELL_TELEMETRY_OPTOUT": "0"`); err != nil {
			t.Fatalf("mutate powershell environment: %v", err)
		}
	case "powershell-updatecheck-reopened":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"POWERSHELL_UPDATECHECK": "Off"`, `"POWERSHELL_UPDATECHECK": "Default"`); err != nil {
			t.Fatalf("mutate powershell environment: %v", err)
		}
	case "polyglot-root-environment":
		expected = "unsafe-plan"
		// The environment spelling of the bounded root and the declared one disagree.
		if err := replaceOnce411(root, profilePath411, `"AURUM_POLYGLOT_ROOT": "/tmp/aurum-polyglot"`, `"AURUM_POLYGLOT_ROOT": "/tmp/aurum-polyglot-host"`); err != nil {
			t.Fatalf("mutate polyglot root environment: %v", err)
		}
	case "proxy-environment":
		expected = "unsafe-plan"
		// The egress channel re-opened through the environment.
		if err := replaceOnce411(root, profilePath411, `"HTTPS_PROXY": ""`, `"HTTPS_PROXY": "http://127.0.0.1:3128"`); err != nil {
			t.Fatalf("mutate proxy environment: %v", err)
		}
	case "no-proxy-environment":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"NO_PROXY": "*"`, `"NO_PROXY": ""`); err != nil {
			t.Fatalf("mutate no proxy environment: %v", err)
		}
	case "cgo-environment":
		expected = "unsafe-plan"
		if err := replaceOnce411(root, profilePath411, `"CGO_ENABLED": "0"`, `"CGO_ENABLED": "1"`); err != nil {
			t.Fatalf("mutate cgo environment: %v", err)
		}
	case "unpinned-toolchain-source":
		expected = "digest-invalid"
		if err := replaceOnce411(root, profilePath411, `"toolchain_source": "digest-pinned"`, `"toolchain_source": "unpinned"`); err != nil {
			t.Fatalf("mutate toolchain source: %v", err)
		}
	case "mutable-toolchain-source":
		expected = "digest-invalid"
		// The toolchain set referenced by a mutable tag instead of content.
		if err := replaceOnce411(root, profilePath411, `"toolchain_source": "digest-pinned"`, `"toolchain_source": "latest"`); err != nil {
			t.Fatalf("mutate toolchain source: %v", err)
		}
	case "unpinned-partition-toolchain":
		expected = "digest-invalid"
		// One partition alone drops its content addressing while the plan-level field
		// still claims every toolchain is pinned.
		if err := replaceOnce411(root, profilePath411, `"javascript": {"runtime": "node", "toolchain": "digest-pinned"`, `"javascript": {"runtime": "node", "toolchain": "latest"`); err != nil {
			t.Fatalf("mutate partition toolchain: %v", err)
		}
	case "missing-pids-limit":
		expected = "schema-invalid"
		if err := dropKey411(root, "pids_limit"); err != nil {
			t.Fatalf("mutate pids limit: %v", err)
		}
	case "missing-resolve-timeout":
		expected = "schema-invalid"
		if err := dropKey411(root, "resolve_timeout_seconds"); err != nil {
			t.Fatalf("mutate resolve timeout: %v", err)
		}
	case "missing-languages":
		expected = "schema-invalid"
		if err := dropKey411(root, "languages"); err != nil {
			t.Fatalf("mutate languages: %v", err)
		}
	case "missing-toolchains":
		expected = "schema-invalid"
		if err := dropKey411(root, "toolchains"); err != nil {
			t.Fatalf("mutate toolchains: %v", err)
		}
	case "real-credential":
		expected = "credential-present"
		// Assembled here, never written in this source file, so the runner's input
		// gate never sees a credential-shaped literal in a materialized path.
		fake := "gh" + "p_" + strings.Repeat("A", 36)
		if !credentialShape411.MatchString(fake) {
			t.Fatal("mutation vector is not credential shaped")
		}
		if err := replaceOnce411(root, profilePath411, `"HTTP_PROXY": ""`, `"HTTP_PROXY": "`+fake+`"`); err != nil {
			t.Fatalf("mutate environment: %v", err)
		}
	default:
		t.Fatalf("unknown mutation: %s", mutation)
	}

	digest, code := ValidateRegistryAUR411(root)
	if mutation != "" {
		if code != expected {
			t.Fatalf("mutation %s got %s, want %s", mutation, code, expected)
		}
		if digest != "" {
			t.Fatalf("mutation %s returned a registry digest", mutation)
		}
		// A measurement that reports every runtime present may not rescue a rejected
		// plan. The same invariance is required under the mutant as under the nominal
		// document: the refusal is returned before the measurement is ever read, so no
		// language resolves and the reported code does not change.
		everything := syntheticAvailability411(true)
		for _, language := range expectedLanguages411 {
			plan, resolveCode := ResolvePolyglotAUR411(root, language, everything)
			if resolveCode != expected {
				t.Fatalf("mutation %s: language %s resolved %s, want %s", mutation, language, resolveCode, expected)
			}
			if plan != (languagePlan411{}) {
				t.Fatalf("mutation %s: language %s returned a plan from a rejected registry", mutation, language)
			}
		}
		fmt.Printf("mutation=%s code=%s\n", mutation, code)
		return
	}
	fmt.Printf("registry_code=%s\n", code)
	if code != "valid" || digest == "" {
		t.Fatalf("registry result=%s/%s", code, digest)
	}

	p, _, profileCode := ValidateProfileAUR411(root)
	if profileCode != "valid" {
		t.Fatalf("profile result=%s", profileCode)
	}

	// The availability argument is proved load-bearing with three synthetic measurements
	// before any real one is consulted. These three do not observe a machine and never
	// claim to; they prove the resolver answers from the measurement it is handed.
	assertResolutionIsDriven411(t, p)

	// Every unregistered name and every impersonation of a declared partition fails
	// closed, whatever the measurement says. An all-present measurement is used here
	// precisely so that availability cannot be what rejects them.
	everything := syntheticAvailability411(true)
	for _, language := range unregisteredLanguages411 {
		plan, resolveCode := ResolveLanguageAUR411(p, language, everything)
		if resolveCode != "language-unregistered" {
			t.Fatalf("unregistered language %q resolved %s", language, resolveCode)
		}
		if plan != (languagePlan411{}) {
			t.Fatalf("unregistered language %q returned a plan", language)
		}
	}
	fmt.Printf("unregistered_languages_denied=%d\n", len(unregisteredLanguages411))

	// Now the real measurement, taken by the acceptance on the pristine PATH before any
	// shim existed. What it measures, and all it measures, is whether each declared
	// runtime command resolves through PATH inside this execution image. That is the only
	// claim made from it: a runtime measured present resolves to an isolated offline
	// plan, and a runtime measured absent fails closed, because this plan never installs
	// and so can never create what the image does not already carry.
	measured := readAvailabilityLedger411(t)
	availableCount := 0
	for _, language := range expectedLanguages411 {
		runtimeName := expectedRuntimes411[language]
		// Through the gated entry point, so the nominal path exercises the same door the
		// mutants are refused at.
		plan, resolveCode := ResolvePolyglotAUR411(root, language, measured)
		if measured[runtimeName] {
			availableCount++
			if resolveCode != "valid" {
				t.Fatalf("%s runtime %s measured present but resolved %s", language, runtimeName, resolveCode)
			}
			if plan.CacheRoot != p.PolyglotRoot+"/"+language || plan.CacheMode != "read-only" ||
				plan.Telemetry != "denied" || plan.Network != "none" || plan.PackageInstall != "denied" {
				t.Fatalf("%s resolved a non-isolated plan: %+v", language, plan)
			}
			continue
		}
		if resolveCode != "toolchain-unavailable" {
			t.Fatalf("%s runtime %s measured absent but resolved %s", language, runtimeName, resolveCode)
		}
		if plan != (languagePlan411{}) {
			t.Fatalf("%s returned a plan for an absent runtime", language)
		}
	}
	fmt.Printf("toolchains_measured=%d toolchains_present=%d toolchains_absent=%d\n",
		len(measured), availableCount, len(measured)-availableCount)

	// The declared install count and the measured one have to agree. The ledger is
	// written by shims the acceptance places ahead of PATH, so this is a measurement of
	// the whole run and not a restatement of the document. What it measures is an
	// invocation of one of those entrypoints resolved through PATH; a caller that reached
	// a package manager by absolute path would not appear in it, and this test does not
	// claim otherwise.
	installs := measuredPackageInstalls411(t)
	if installs != p.PackageInstalls {
		t.Fatalf("package installs declared %d, measured %d", p.PackageInstalls, installs)
	}
	fmt.Printf("package_installs_declared=%d package_installs_measured=%d\n", p.PackageInstalls, installs)

	registryBytes := mustRead411(t, root, registryPath411)
	if strings.Contains(string(registryBytes), "latest") {
		t.Fatal("mutable tag in registry")
	}
	// No materialized document this card owns may carry a credential shape.
	for _, path := range []string{registryPath411, profilePath411, lockPath411, schemaPath411} {
		if credentialShape411.Match(mustRead411(t, root, path)) {
			t.Fatalf("credential shape in %s", path)
		}
	}
	fmt.Printf("registry_digest=%s\n", digest)
}

func mustRead411(t *testing.T, root, path string) []byte {
	t.Helper()
	b, err := read411(root, path)
	if err != nil {
		t.Fatalf("unreadable %s: %v", path, err)
	}
	return b
}
