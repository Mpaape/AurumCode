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

const schemaPath409 = ".board/schemas/fake-scm-offline-profile.schema.json"
const profilePath409 = ".board/oci/profiles/fake-scm-offline-v1.json"
const lockPath409 = ".board/locks/oci/fake-scm-offline-v1.lock.json"
const registryPath409 = ".board/oci/profiles/registry.v1.json"
const containerSchemaPath409 = ".board/schemas/container-profile.schema.json"
const registryLockPath409 = ".board/locks/oci/registry-v1.lock.json"
const goUnitSchemaPath409 = ".board/schemas/go-unit-offline-profile.schema.json"
const goUnitLockPath409 = ".board/locks/oci/go-unit-offline-v1.lock.json"
const goGitSchemaPath409 = ".board/schemas/go-git-offline-profile.schema.json"
const goGitLockPath409 = ".board/locks/oci/go-git-offline-v1.lock.json"
const fakeProviderSchemaPath409 = ".board/schemas/fake-provider-profile.schema.json"
const fakeProviderLockPath409 = ".board/locks/oci/fake-provider-v1.lock.json"
const parserWorkerSchemaPath409 = ".board/schemas/parser-worker-profile.schema.json"
const parserWorkerLockPath409 = ".board/locks/oci/parser-worker-v1.lock.json"
const sqliteSchemaPath409 = ".board/schemas/sqlite-offline-profile.schema.json"
const sqliteLockPath409 = ".board/locks/oci/sqlite-offline-v1.lock.json"
const docsToolSchemaPath409 = ".board/schemas/docs-tool-offline-profile.schema.json"
const docsToolLockPath409 = ".board/locks/oci/docs-tool-offline-v1.lock.json"
const ociConformanceSchemaPath409 = ".board/schemas/oci-conformance-profile.schema.json"
const ociConformanceLockPath409 = ".board/locks/oci/oci-conformance-v1.lock.json"
const profileKey409 = "fake-scm-offline-v1"

type profile409 struct {
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
	ScmBackend          string            `json:"scm_backend"`
	RealScmBinary       string            `json:"real_scm_binary"`
	FakeScm             string            `json:"fake_scm"`
	FakeScmCGO          bool              `json:"fake_scm_cgo"`
	FakeScmRoot         string            `json:"fake_scm_root"`
	EventSet            string            `json:"event_set"`
	EventRoot           string            `json:"event_root"`
	ResponseSet         string            `json:"response_set"`
	RepositoryMode      string            `json:"repository_mode"`
	RepositoryRoot      string            `json:"repository_root"`
	RemoteOrigin        string            `json:"remote_origin"`
	RemoteProtocols     []string          `json:"remote_protocols"`
	CustomTransport     string            `json:"custom_transport"`
	CredentialHelper    string            `json:"credential_helper"`
	Askpass             string            `json:"askpass"`
	Hooks               string            `json:"hooks"`
	Submodules          string            `json:"submodules"`
	URLRewriting        string            `json:"url_rewriting"`
	Publication         string            `json:"publication"`
	ExternalDestination string            `json:"external_destination"`
	Token               string            `json:"token"`
	CredentialSources   string            `json:"credential_sources"`
	HostCheckout        string            `json:"host_checkout"`
	HostFilesystem      string            `json:"host_filesystem"`
	Subprocess          string            `json:"subprocess"`
	MaxEvents           int               `json:"max_events"`
	MaxEventBytes       int               `json:"max_event_bytes"`
	MaxResponses        int               `json:"max_responses"`
	MaxResponseBytes    int               `json:"max_response_bytes"`
	MaxRepositoryBytes  int               `json:"max_repository_bytes"`
	ScmTimeoutSeconds   int               `json:"scm_timeout_seconds"`
	Environment         map[string]string `json:"environment"`
	Command             []string          `json:"command"`
}

type lock409 struct {
	Schema  string `json:"schema"`
	Version int    `json:"version"`
	Profile string `json:"profile"`
	Image   string `json:"image"`
}

type entry409 struct {
	Key            string `json:"key"`
	Schema         string `json:"schema"`
	SchemaDigest   string `json:"schema_digest"`
	Lock           string `json:"lock"`
	LockDigest     string `json:"lock_digest"`
	ImageSetDigest string `json:"image_set_digest"`
}

type registry409 struct {
	Schema   string     `json:"schema"`
	Version  int        `json:"version"`
	Profiles []entry409 `json:"profiles"`
}

var digest409 = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// The fake SCM is a pure-Go, cgo-free implementation, so the plan needs a Go toolchain
// and Bash and no version-control binary at all. It is pinned to the locally present
// bootstrap image by immutable digest; a mutable tag, a bare name, or any other
// repository denies before the plan is used. The bootstrap image carries no `git`, no
// `git-upload-pack`, no `git-receive-pack`, no `ssh` and no `curl`, so the transports
// this profile denies are not merely unused: they are absent from the pinned image.
var image409 = regexp.MustCompile(`^aurum-bootstrap-go-bash@sha256:[0-9a-f]{64}$`)

// The fake engine, its pinned event log and the simulated repositories live under three
// bounded tmpfs directories and nowhere else. Every expression is anchored on both ends,
// so a checkout path, an absolute host path, a trailing slash, a look-alike sibling and a
// traversal that starts inside the bounded root and climbs out of it are all denied.
var fakeScmRoot409 = regexp.MustCompile(`^/tmp/aurum-fake-scm-engine$`)
var eventRoot409 = regexp.MustCompile(`^/tmp/aurum-fake-scm-events$`)
var repositoryRoot409 = regexp.MustCompile(`^/tmp/aurum-fake-scm-repos$`)

// The plan has no origin at all. The expression is anchored on both ends, so an `https://`
// clone URL, an `scp`-style `git@host:path` remote, an `ssh://` URL, a `file://` URL that
// reaches outside the simulated tree and an `ext::` command remote are denied alike, and
// none of them can hide behind a prefix or a suffix of the admitted value.
var remoteOrigin409 = regexp.MustCompile(`^absent$`)

// Credential shapes are assembled from fragments so this source file itself never
// carries a literal that the runner's input gate would read as a real secret.
var credentialShape409 = regexp.MustCompile(`(sk` + `-[A-Za-z0-9_-]{20,}|AKIA` + `[0-9A-Z]{16}|gh` + `[pousr]_[A-Za-z0-9]{20,}|-----BEGIN [A-Z ]*PRIVATE KEY-----)`)

var registryKeys409 = []string{"bootstrap-readonly-v1", "docs-tool-offline-v1", "fake-provider-v1", "fake-scm-offline-v1", "go-git-offline-v1", "go-unit-offline-v1", "oci-conformance-v1", "parser-worker-v1", "registry-v1", "sqlite-offline-v1"}
var requiredProfileKeys409 = []string{"schema", "version", "profile", "lock", "lock_digest", "network", "user", "cap_drop", "cap_add", "mounts", "devices", "sockets", "pull", "tmpfs", "checkout_readonly", "read_only_rootfs", "no_new_privileges", "privileged", "timeout_seconds", "memory_mb", "cpu_millis", "pids_limit", "tmpfs_mb", "stdout_limit_bytes", "stderr_limit_bytes", "max_input_files", "max_input_bytes", "module_cache", "module_cache_read_only", "scm_backend", "real_scm_binary", "fake_scm", "fake_scm_cgo", "fake_scm_root", "event_set", "event_root", "response_set", "repository_mode", "repository_root", "remote_origin", "remote_protocols", "custom_transport", "credential_helper", "askpass", "hooks", "submodules", "url_rewriting", "publication", "external_destination", "token", "credential_sources", "host_checkout", "host_filesystem", "subprocess", "max_events", "max_event_bytes", "max_responses", "max_response_bytes", "max_repository_bytes", "scm_timeout_seconds", "environment", "command"}
var requiredEnvironmentKeys409 = []string{"GOPROXY", "GOSUMDB", "GONOSUMDB", "GOTOOLCHAIN", "CGO_ENABLED", "AURUM_FAKE_SCM_ROOT", "AURUM_FAKE_SCM_EVENT_ROOT", "AURUM_FAKE_SCM_REPOSITORY_ROOT", "GIT_CONFIG_NOSYSTEM", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM", "GIT_TERMINAL_PROMPT", "GIT_ASKPASS", "GIT_ALLOW_PROTOCOL", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"}
var schemaDocumentKeys409 = []string{"$schema", "$id", "title", "type", "additionalProperties", "required", "properties"}
var expectedCommand409 = []string{"go", "test", "-mod=readonly", "./..."}

// The only transport the plan admits is the in-process fake one. This is an allowlist
// compared element by element, so adding `https`, `ssh`, `file` or `ext` to it is a plan
// change and is rejected exactly like enabling publication.
var expectedRemoteProtocols409 = []string{"fake"}

// The safety constants are pinned here, in Go, and not only in the schema bytes. A
// reviewer who relaxes the schema document and recomputes its `schema_digest` in the
// registry moves both files consistently; this map is the third copy that does not move
// with them, so the relaxation is still rejected.
var profileSchemaConstants409 = map[string]any{
	"schema":                 "aurum.fake-scm-offline-profile",
	"version":                1,
	"profile":                profileKey409,
	"lock":                   lockPath409,
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
	"scm_backend":            "fake-local",
	"real_scm_binary":        "denied",
	"fake_scm":               "digest-pinned",
	"fake_scm_cgo":           false,
	"fake_scm_root":          "/tmp/aurum-fake-scm-engine",
	"event_set":              "digest-pinned",
	"event_root":             "/tmp/aurum-fake-scm-events",
	"response_set":           "digest-pinned",
	"repository_mode":        "ephemeral",
	"repository_root":        "/tmp/aurum-fake-scm-repos",
	"remote_origin":          "absent",
	"remote_protocols":       expectedRemoteProtocols409,
	"custom_transport":       "denied",
	"credential_helper":      "denied",
	"askpass":                "denied",
	"hooks":                  "denied",
	"submodules":             "denied",
	"url_rewriting":          "denied",
	"publication":            "denied",
	"external_destination":   "denied",
	"token":                  "absent",
	"credential_sources":     "denied",
	"host_checkout":          "denied",
	"host_filesystem":        "denied",
	"subprocess":             "denied",
	"max_events":             1024,
	"max_event_bytes":        65536,
	"max_responses":          64,
	"max_response_bytes":     65536,
	"max_repository_bytes":   33554432,
	"scm_timeout_seconds":    5,
}

var environmentSchemaConstants409 = map[string]any{
	"GOPROXY":                        "off",
	"GOSUMDB":                        "off",
	"GONOSUMDB":                      "*",
	"GOTOOLCHAIN":                    "local",
	"CGO_ENABLED":                    "0",
	"AURUM_FAKE_SCM_ROOT":            "/tmp/aurum-fake-scm-engine",
	"AURUM_FAKE_SCM_EVENT_ROOT":      "/tmp/aurum-fake-scm-events",
	"AURUM_FAKE_SCM_REPOSITORY_ROOT": "/tmp/aurum-fake-scm-repos",
	// Even if a future image did carry a version-control binary, the plan neutralizes
	// every configuration channel it could read a credential helper, an askpass program,
	// an `insteadOf` rewrite or an `ext::` transport from.
	"GIT_CONFIG_NOSYSTEM": "1",
	"GIT_CONFIG_GLOBAL":   "/dev/null",
	"GIT_CONFIG_SYSTEM":   "/dev/null",
	"GIT_TERMINAL_PROMPT": "0",
	"GIT_ASKPASS":         "/bin/false",
	"GIT_ALLOW_PROTOCOL":  "none",
	"HTTP_PROXY":          "",
	"HTTPS_PROXY":         "",
	"NO_PROXY":            "*",
}

func hash409(b []byte) string { h := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(h[:]) }

// rejectDuplicateJSON409 refuses a document whose last-wins duplicate key would let a
// registry publish one plan to a reader and another to the runner.
func rejectDuplicateJSON409(b []byte) error {
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

func decode409(b []byte, out any) error {
	if err := rejectDuplicateJSON409(b); err != nil {
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

func exactKeys409(raw map[string]json.RawMessage, expected []string) bool {
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

func exactStringSet409(actual, expected []string) bool {
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

// exactStringList409 compares an ordered list element by element. It is used for the
// command and for the transport allowlist, so neither a reordering, nor an extra element,
// nor a missing one can pass as the declared value.
func exactStringList409(actual, expected []string) bool {
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

func schemaConst409(raw json.RawMessage, expected any) bool {
	var spec map[string]json.RawMessage
	if decode409(raw, &spec) != nil || !exactKeys409(spec, []string{"const"}) {
		return false
	}
	var actual any
	if decode409(spec["const"], &actual) != nil {
		return false
	}
	actualJSON, actualErr := json.Marshal(actual)
	expectedJSON, expectedErr := json.Marshal(expected)
	return actualErr == nil && expectedErr == nil && bytes.Equal(actualJSON, expectedJSON)
}

func validateSchemaInvariant409(schema []byte) bool {
	var raw map[string]json.RawMessage
	if decode409(schema, &raw) != nil || !exactKeys409(raw, schemaDocumentKeys409) {
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
	if decode409(schema, &doc) != nil ||
		doc.Schema != "https://json-schema.org/draft/2020-12/schema" ||
		doc.ID != "https://aurumcode.dev/schemas/fake-scm-offline-profile.schema.json" ||
		doc.Title != "AurumCode offline fake SCM profile" ||
		doc.Type != "object" || doc.AdditionalProperties ||
		!exactStringSet409(doc.Required, requiredProfileKeys409) ||
		!exactKeys409(doc.Properties, requiredProfileKeys409) {
		return false
	}
	for key, expected := range profileSchemaConstants409 {
		if !schemaConst409(doc.Properties[key], expected) {
			return false
		}
	}
	var digestSpec map[string]json.RawMessage
	if decode409(doc.Properties["lock_digest"], &digestSpec) != nil || !exactKeys409(digestSpec, []string{"type", "pattern"}) {
		return false
	}
	var digestRule struct {
		Type    string `json:"type"`
		Pattern string `json:"pattern"`
	}
	if decode409(doc.Properties["lock_digest"], &digestRule) != nil || digestRule.Type != "string" || digestRule.Pattern != "^sha256:[0-9a-f]{64}$" {
		return false
	}
	if !schemaConst409(doc.Properties["command"], expectedCommand409) {
		return false
	}
	var envRaw map[string]json.RawMessage
	if decode409(doc.Properties["environment"], &envRaw) != nil || !exactKeys409(envRaw, []string{"type", "additionalProperties", "required", "properties"}) {
		return false
	}
	var envSpec struct {
		Type                 string                     `json:"type"`
		AdditionalProperties bool                       `json:"additionalProperties"`
		Required             []string                   `json:"required"`
		Properties           map[string]json.RawMessage `json:"properties"`
	}
	if decode409(doc.Properties["environment"], &envSpec) != nil || envSpec.Type != "object" || envSpec.AdditionalProperties ||
		!exactStringSet409(envSpec.Required, requiredEnvironmentKeys409) ||
		!exactKeys409(envSpec.Properties, requiredEnvironmentKeys409) {
		return false
	}
	for key, expected := range environmentSchemaConstants409 {
		if !schemaConst409(envSpec.Properties[key], expected) {
			return false
		}
	}
	return true
}

func read409(root, path string) ([]byte, error) {
	if strings.Contains(path, "..") || strings.Contains(path, "//") {
		return nil, fmt.Errorf("unsafe path")
	}
	info, err := os.Lstat(filepath.Join(root, path))
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("missing")
	}
	return os.ReadFile(filepath.Join(root, path))
}

// ValidateProfileAUR409 is the side-effect-free plan loader for fake-scm-offline-v1. It
// never invokes a container engine, never opens a socket, never spawns a subprocess,
// never runs a version-control command, and never reads anything outside the documents
// this card owns.
func ValidateProfileAUR409(root string) (profile409, lock409, string) {
	sb, err := read409(root, schemaPath409)
	if err != nil {
		return profile409{}, lock409{}, "schema-missing"
	}
	if !validateSchemaInvariant409(sb) {
		return profile409{}, lock409{}, "schema-invalid"
	}
	pb, err := read409(root, profilePath409)
	if err != nil {
		return profile409{}, lock409{}, "profile-missing"
	}
	var raw map[string]json.RawMessage
	if decode409(pb, &raw) != nil || !exactKeys409(raw, requiredProfileKeys409) {
		return profile409{}, lock409{}, "schema-invalid"
	}
	var envRaw map[string]json.RawMessage
	if decode409(raw["environment"], &envRaw) != nil || !exactKeys409(envRaw, requiredEnvironmentKeys409) {
		return profile409{}, lock409{}, "schema-invalid"
	}
	var p profile409
	if decode409(pb, &p) != nil {
		return p, lock409{}, "schema-invalid"
	}
	// Credential shapes are refused before any other plan rule, so a value that could
	// authenticate against a real forge is denied even when it arrives through a field
	// whose exact value is otherwise pinned by a constant below.
	for _, value := range p.Environment {
		if credentialShape409.MatchString(value) {
			return p, lock409{}, "credential-present"
		}
	}
	// The fake engine, the event log and the response set are admitted only while they
	// are content-addressed. Their bytes belong to a dependent implementation card; what
	// this card pins is that an unpinned or tag-referenced input can never be resolved,
	// so a replayed event or response cannot change under a fixed plan.
	if p.FakeScm != "digest-pinned" || p.EventSet != "digest-pinned" || p.ResponseSet != "digest-pinned" ||
		!digest409.MatchString(p.LockDigest) {
		return p, lock409{}, "digest-invalid"
	}
	// Every bound is an exact constant, never a minimum: a value larger than the one
	// declared here is as unsafe as a value of zero, because the simulated repositories
	// have to fit inside the bounded tmpfs that holds them.
	if p.Schema != "aurum.fake-scm-offline-profile" || p.Version != 1 || p.Profile != profileKey409 ||
		p.Lock != lockPath409 ||
		p.Network != "none" || p.User != "65534:65534" || p.CapDrop != "ALL" || p.CapAdd != "none" ||
		p.Mounts != "none" || p.Devices != "none" || p.Sockets != "none" ||
		p.Pull != "never" || p.Tmpfs != "rw,nosuid,nodev" ||
		!p.CheckoutReadonly || !p.ReadOnlyRootfs || !p.NoNewPrivileges || p.Privileged ||
		p.TimeoutSeconds != 60 || p.MemoryMB != 256 || p.CPUMillis != 1000 || p.PidsLimit != 128 ||
		p.TmpfsMB != 128 || p.StdoutLimitBytes != 65536 || p.StderrLimitBytes != 65536 ||
		p.MaxInputFiles != 10000 || p.MaxInputBytes != 67108864 ||
		p.ModuleCache != "/go/pkg/mod" || !p.ModuleCacheReadOnly ||
		p.ScmBackend != "fake-local" || p.RealScmBinary != "denied" ||
		p.FakeScmCGO || !fakeScmRoot409.MatchString(p.FakeScmRoot) ||
		!eventRoot409.MatchString(p.EventRoot) ||
		p.RepositoryMode != "ephemeral" || !repositoryRoot409.MatchString(p.RepositoryRoot) ||
		!remoteOrigin409.MatchString(p.RemoteOrigin) ||
		!exactStringList409(p.RemoteProtocols, expectedRemoteProtocols409) ||
		p.CustomTransport != "denied" || p.CredentialHelper != "denied" || p.Askpass != "denied" ||
		p.Hooks != "denied" || p.Submodules != "denied" || p.URLRewriting != "denied" ||
		p.Publication != "denied" || p.ExternalDestination != "denied" ||
		p.Token != "absent" || p.CredentialSources != "denied" ||
		p.HostCheckout != "denied" || p.HostFilesystem != "denied" || p.Subprocess != "denied" ||
		p.MaxEvents != 1024 || p.MaxEventBytes != 65536 ||
		p.MaxResponses != 64 || p.MaxResponseBytes != 65536 ||
		p.MaxRepositoryBytes != 33554432 || p.ScmTimeoutSeconds != 5 ||
		!exactStringList409(p.Command, expectedCommand409) {
		return p, lock409{}, "unsafe-plan"
	}
	// Independently of the exact constants above, the simulated repositories have to fit
	// inside the declared tmpfs, and the pinned event log and response set have to fit
	// inside the declared input bound. A plan whose replay data cannot be held by its own
	// bounded filesystem would have to spill somewhere the plan never bounded.
	if p.MaxRepositoryBytes <= 0 || p.TmpfsMB <= 0 || p.MaxRepositoryBytes > p.TmpfsMB*1024*1024 {
		return p, lock409{}, "unsafe-plan"
	}
	if p.MaxEvents <= 0 || p.MaxEventBytes <= 0 ||
		p.MaxEvents > p.MaxInputFiles || p.MaxEvents*p.MaxEventBytes > p.MaxInputBytes {
		return p, lock409{}, "unsafe-plan"
	}
	if p.MaxResponses <= 0 || p.MaxResponseBytes <= 0 ||
		p.MaxResponses > p.MaxInputFiles || p.MaxResponses*p.MaxResponseBytes > p.MaxInputBytes {
		return p, lock409{}, "unsafe-plan"
	}
	// The three bounded roots are distinct directories and none of them contains another.
	// A fake engine that lived inside the simulated repository tree, or an event log that
	// sat under the engine, would make a pinned input writable at replay time.
	for _, pair := range [][2]string{
		{p.FakeScmRoot, p.EventRoot},
		{p.FakeScmRoot, p.RepositoryRoot},
		{p.EventRoot, p.RepositoryRoot},
	} {
		if pair[0] == pair[1] ||
			strings.HasPrefix(pair[0], pair[1]+"/") || strings.HasPrefix(pair[1], pair[0]+"/") {
			return p, lock409{}, "unsafe-plan"
		}
	}
	for key, expected := range environmentSchemaConstants409 {
		if p.Environment[key] != expected.(string) {
			return p, lock409{}, "unsafe-plan"
		}
	}
	lb, err := read409(root, lockPath409)
	if err != nil {
		return p, lock409{}, "lock-missing"
	}
	var l lock409
	if decode409(lb, &l) != nil || l.Schema != "aurum.oci-image-lock" || l.Version != 1 ||
		l.Profile != p.Profile || !image409.MatchString(l.Image) {
		return p, l, "mutable-input"
	}
	if hash409(lb) != p.LockDigest {
		return p, l, "lock-digest-mismatch"
	}
	return p, l, "valid"
}

// ValidateRegistryAUR409 resolves the canonical registry and admits exactly the ten
// registered keys. It is fail-closed: an unknown key, a duplicate, an out-of-order
// entry, or a digest that does not match the bytes on disk denies without any engine.
func ValidateRegistryAUR409(root string) (string, string) {
	rb, err := read409(root, registryPath409)
	if err != nil {
		return "", "registry-missing"
	}
	var r registry409
	if decode409(rb, &r) != nil || r.Schema != "aurum.profile-registry" || r.Version != 1 {
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
		case profileKey409:
			found = true
			if x.Schema != schemaPath409 || x.Lock != lockPath409 {
				return "", "unsafe-plan"
			}
			sb, err := read409(root, schemaPath409)
			if err != nil {
				return "", "schema-missing"
			}
			if x.SchemaDigest != hash409(sb) {
				return "", "schema-digest-mismatch"
			}
			_, l, code := ValidateProfileAUR409(root)
			if code != "valid" {
				return "", code
			}
			lb, err := read409(root, lockPath409)
			if err != nil {
				return "", "lock-missing"
			}
			if x.LockDigest != hash409(lb) {
				return "", "lock-digest-mismatch"
			}
			if x.ImageSetDigest != hash409([]byte(l.Image)) {
				return "", "unsafe-plan"
			}
		case "bootstrap-readonly-v1", "registry-v1":
			if x.Schema != containerSchemaPath409 || x.Lock != registryLockPath409 {
				return "", "unsafe-plan"
			}
			sb, err := read409(root, containerSchemaPath409)
			if err != nil {
				return "", "schema-missing"
			}
			if x.SchemaDigest != hash409(sb) {
				return "", "schema-digest-mismatch"
			}
			lb, err := read409(root, registryLockPath409)
			if err != nil {
				return "", "lock-missing"
			}
			if x.LockDigest != hash409(lb) {
				return "", "lock-digest-mismatch"
			}
			var l lock409
			if decode409(lb, &l) != nil || l.Schema != "aurum.oci-image-lock" || l.Version != 1 {
				return "", "mutable-input"
			}
			if x.ImageSetDigest != hash409([]byte(l.Image)) {
				return "", "unsafe-plan"
			}
		case "go-unit-offline-v1":
			// Owned by AUR-403. Its documents are not materialized for this
			// acceptance, so only the declared paths and digest shape are checked; the
			// fake SCM can never silently re-point the Go unit plan.
			if x.Schema != goUnitSchemaPath409 || x.Lock != goUnitLockPath409 {
				return "", "unsafe-plan"
			}
			if !digest409.MatchString(x.SchemaDigest) || !digest409.MatchString(x.LockDigest) || !digest409.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "go-git-offline-v1":
			// Owned by AUR-404, checked under the same neighbour rule as AUR-403's key.
			// That profile pins the image that does carry a real `git`; this one is a
			// different key with a different lock and cannot borrow it.
			if x.Schema != goGitSchemaPath409 || x.Lock != goGitLockPath409 {
				return "", "unsafe-plan"
			}
			if !digest409.MatchString(x.SchemaDigest) || !digest409.MatchString(x.LockDigest) || !digest409.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "fake-provider-v1":
			// Owned by AUR-405, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != fakeProviderSchemaPath409 || x.Lock != fakeProviderLockPath409 {
				return "", "unsafe-plan"
			}
			if !digest409.MatchString(x.SchemaDigest) || !digest409.MatchString(x.LockDigest) || !digest409.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "parser-worker-v1":
			// Owned by AUR-406, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != parserWorkerSchemaPath409 || x.Lock != parserWorkerLockPath409 {
				return "", "unsafe-plan"
			}
			if !digest409.MatchString(x.SchemaDigest) || !digest409.MatchString(x.LockDigest) || !digest409.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "sqlite-offline-v1":
			// Owned by AUR-407, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != sqliteSchemaPath409 || x.Lock != sqliteLockPath409 {
				return "", "unsafe-plan"
			}
			if !digest409.MatchString(x.SchemaDigest) || !digest409.MatchString(x.LockDigest) || !digest409.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "docs-tool-offline-v1":
			// Owned by AUR-408, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != docsToolSchemaPath409 || x.Lock != docsToolLockPath409 {
				return "", "unsafe-plan"
			}
			if !digest409.MatchString(x.SchemaDigest) || !digest409.MatchString(x.LockDigest) || !digest409.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "oci-conformance-v1":
			// Owned by AUR-410, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != ociConformanceSchemaPath409 || x.Lock != ociConformanceLockPath409 {
				return "", "unsafe-plan"
			}
			if !digest409.MatchString(x.SchemaDigest) || !digest409.MatchString(x.LockDigest) || !digest409.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		default:
			return "", "profile-unregistered"
		}
	}
	if !found || !sort.StringsAreSorted(keys) || !exactStringSet409(keys, registryKeys409) {
		return "", "profile-unregistered"
	}
	return hash409(rb), "valid"
}

func writeMutation409(root, path string, data []byte) error {
	full := filepath.Join(root, path)
	if err := os.Chmod(full, 0600); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0600)
}

func replaceOnce409(root, path, from, to string) error {
	b, err := read409(root, path)
	if err != nil {
		return err
	}
	if !bytes.Contains(b, []byte(from)) {
		return fmt.Errorf("mutation anchor %q absent from %s", from, path)
	}
	return writeMutation409(root, path, bytes.Replace(b, []byte(from), []byte(to), 1))
}

// dropKey409 removes one required key from the profile document, which is how a
// declared bound goes missing in practice.
func dropKey409(root, key string) error {
	pb, err := read409(root, profilePath409)
	if err != nil {
		return err
	}
	var p map[string]any
	if err := decode409(pb, &p); err != nil {
		return err
	}
	if _, ok := p[key]; !ok {
		return fmt.Errorf("mutation anchor %q absent from %s", key, profilePath409)
	}
	delete(p, key)
	nb, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return writeMutation409(root, profilePath409, nb)
}

func TestAUR409(t *testing.T) {
	root := os.Getenv("AURUMCODE_ROOT")
	if root == "" {
		t.Fatal("AURUMCODE_ROOT is required")
	}
	mutation := os.Getenv("AURUM_A409_MUTATION")
	expected := mutation
	switch mutation {
	case "":
	case "duplicate-profile":
		rb, err := read409(root, registryPath409)
		if err != nil {
			t.Fatalf("registry read: %v", err)
		}
		var r registry409
		if decode409(rb, &r) != nil {
			t.Fatal("registry decode")
		}
		var clone entry409
		for _, x := range r.Profiles {
			if x.Key == profileKey409 {
				clone = x
			}
		}
		if clone.Key == "" {
			t.Fatal("mutation target absent: " + profileKey409)
		}
		// A second document for the same key, with different bytes.
		clone.LockDigest = "sha256:" + strings.Repeat("0", 64)
		r.Profiles = append(r.Profiles, clone)
		nb, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("registry marshal: %v", err)
		}
		if err := writeMutation409(root, registryPath409, nb); err != nil {
			t.Fatalf("mutate registry: %v", err)
		}
	case "mutable-input":
		lb, err := read409(root, lockPath409)
		if err != nil {
			t.Fatalf("lock read: %v", err)
		}
		lb = bytes.Replace(lb,
			[]byte("aurum-bootstrap-go-bash@sha256:503ac356fa6bca4bad56fade87b5479ff371bd446ffb9c7db91f211323c7c73e"),
			[]byte("aurum-bootstrap-go-bash:1.21"), 1)
		if !bytes.Contains(lb, []byte("aurum-bootstrap-go-bash:1.21")) {
			t.Fatal("mutation anchor absent from lock")
		}
		if err := writeMutation409(root, lockPath409, lb); err != nil {
			t.Fatalf("mutate lock: %v", err)
		}
		pb, err := read409(root, profilePath409)
		if err != nil {
			t.Fatalf("profile read: %v", err)
		}
		var p map[string]any
		if decode409(pb, &p) != nil {
			t.Fatal("profile decode")
		}
		p["lock_digest"] = hash409(lb)
		nb, err := json.Marshal(p)
		if err != nil {
			t.Fatalf("profile marshal: %v", err)
		}
		if err := writeMutation409(root, profilePath409, nb); err != nil {
			t.Fatalf("mutate profile: %v", err)
		}
	case "unsafe-plan":
		if err := replaceOnce409(root, profilePath409, `"network": "none"`, `"network": "host"`); err != nil {
			t.Fatalf("mutate network: %v", err)
		}
	case "mount-enabled":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"mounts": "none"`, `"mounts": "bind"`); err != nil {
			t.Fatalf("mutate mounts: %v", err)
		}
	case "socket-enabled":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"sockets": "none"`, `"sockets": "unix"`); err != nil {
			t.Fatalf("mutate sockets: %v", err)
		}
	case "device-enabled":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"devices": "none"`, `"devices": "/dev/fuse"`); err != nil {
			t.Fatalf("mutate devices: %v", err)
		}
	case "root-user":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"user": "65534:65534"`, `"user": "0:0"`); err != nil {
			t.Fatalf("mutate user: %v", err)
		}
	case "capability-added":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"cap_add": "none"`, `"cap_add": "SYS_ADMIN"`); err != nil {
			t.Fatalf("mutate cap_add: %v", err)
		}
	case "real-scm-backend":
		expected = "unsafe-plan"
		// The backend swapped for a real version-control implementation.
		if err := replaceOnce409(root, profilePath409, `"scm_backend": "fake-local"`, `"scm_backend": "git"`); err != nil {
			t.Fatalf("mutate scm backend: %v", err)
		}
	case "real-scm-binary-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"real_scm_binary": "denied"`, `"real_scm_binary": "git"`); err != nil {
			t.Fatalf("mutate real scm binary: %v", err)
		}
	case "remote-origin-https":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"remote_origin": "absent"`, `"remote_origin": "https://forge.example.com/aurum/aurumcode.git"`); err != nil {
			t.Fatalf("mutate remote origin: %v", err)
		}
	case "remote-origin-scp":
		expected = "unsafe-plan"
		// The `scp`-style remote that carries no scheme at all.
		if err := replaceOnce409(root, profilePath409, `"remote_origin": "absent"`, `"remote_origin": "git@forge.example.com:aurum/aurumcode.git"`); err != nil {
			t.Fatalf("mutate remote origin: %v", err)
		}
	case "remote-origin-ssh":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"remote_origin": "absent"`, `"remote_origin": "ssh://forge.example.com/aurum/aurumcode.git"`); err != nil {
			t.Fatalf("mutate remote origin: %v", err)
		}
	case "remote-origin-file":
		expected = "unsafe-plan"
		// A `file://` remote that reaches outside the simulated tree entirely.
		if err := replaceOnce409(root, profilePath409, `"remote_origin": "absent"`, `"remote_origin": "file:///etc"`); err != nil {
			t.Fatalf("mutate remote origin: %v", err)
		}
	case "remote-origin-ext":
		expected = "unsafe-plan"
		// The `ext::` transport runs an arbitrary command as the remote helper.
		if err := replaceOnce409(root, profilePath409, `"remote_origin": "absent"`, `"remote_origin": "ext::sh -c cat"`); err != nil {
			t.Fatalf("mutate remote origin: %v", err)
		}
	case "remote-origin-absent-prefix":
		expected = "unsafe-plan"
		// A value that merely starts with the admitted one. The expression is anchored at
		// the end as well, so the prefix does not admit it.
		if err := replaceOnce409(root, profilePath409, `"remote_origin": "absent"`, `"remote_origin": "absent-but-really-https://forge.example.com/x.git"`); err != nil {
			t.Fatalf("mutate remote origin: %v", err)
		}
	case "protocol-allowlist-widened":
		expected = "unsafe-plan"
		// A transport added to the allowlist is a plan change, not a detail: the
		// allowlist is compared element by element, so widening it denies.
		if err := replaceOnce409(root, profilePath409, `"remote_protocols": ["fake"]`, `"remote_protocols": ["fake", "https"]`); err != nil {
			t.Fatalf("mutate transport allowlist: %v", err)
		}
	case "protocol-allowlist-ext":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"remote_protocols": ["fake"]`, `"remote_protocols": ["ext"]`); err != nil {
			t.Fatalf("mutate transport allowlist: %v", err)
		}
	case "custom-transport-allowed":
		expected = "unsafe-plan"
		// A custom `upload-pack`/`receive-pack` pair is a remote code path by another name.
		if err := replaceOnce409(root, profilePath409, `"custom_transport": "denied"`, `"custom_transport": "upload-pack"`); err != nil {
			t.Fatalf("mutate custom transport: %v", err)
		}
	case "credential-helper-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"credential_helper": "denied"`, `"credential_helper": "store"`); err != nil {
			t.Fatalf("mutate credential helper: %v", err)
		}
	case "askpass-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"askpass": "denied"`, `"askpass": "allowed"`); err != nil {
			t.Fatalf("mutate askpass: %v", err)
		}
	case "hooks-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"hooks": "denied"`, `"hooks": "allowed"`); err != nil {
			t.Fatalf("mutate hooks: %v", err)
		}
	case "submodule-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"submodules": "denied"`, `"submodules": "recursive"`); err != nil {
			t.Fatalf("mutate submodules: %v", err)
		}
	case "url-rewriting-allowed":
		expected = "unsafe-plan"
		// `insteadOf` rewriting turns an admitted URL into a different one after validation.
		if err := replaceOnce409(root, profilePath409, `"url_rewriting": "denied"`, `"url_rewriting": "insteadOf"`); err != nil {
			t.Fatalf("mutate url rewriting: %v", err)
		}
	case "publication-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"publication": "denied"`, `"publication": "push"`); err != nil {
			t.Fatalf("mutate publication: %v", err)
		}
	case "external-destination-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"external_destination": "denied"`, `"external_destination": "allowed"`); err != nil {
			t.Fatalf("mutate external destination: %v", err)
		}
	case "token-present":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"token": "absent"`, `"token": "present"`); err != nil {
			t.Fatalf("mutate token: %v", err)
		}
	case "credential-source":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"credential_sources": "denied"`, `"credential_sources": "environment"`); err != nil {
			t.Fatalf("mutate credential sources: %v", err)
		}
	case "host-checkout-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"host_checkout": "denied"`, `"host_checkout": "read-write"`); err != nil {
			t.Fatalf("mutate host checkout: %v", err)
		}
	case "host-filesystem-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"host_filesystem": "denied"`, `"host_filesystem": "read-only"`); err != nil {
			t.Fatalf("mutate host filesystem: %v", err)
		}
	case "subprocess-allowed":
		expected = "unsafe-plan"
		// A subprocess is how a real `git` would be reached even without a declared backend.
		if err := replaceOnce409(root, profilePath409, `"subprocess": "denied"`, `"subprocess": "allowed"`); err != nil {
			t.Fatalf("mutate subprocess: %v", err)
		}
	case "cgo-fake-scm":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"fake_scm_cgo": false`, `"fake_scm_cgo": true`); err != nil {
			t.Fatalf("mutate fake scm linkage: %v", err)
		}
	case "persistent-repository":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"repository_mode": "ephemeral"`, `"repository_mode": "persistent"`); err != nil {
			t.Fatalf("mutate repository mode: %v", err)
		}
	case "repository-root-host":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"repository_root": "/tmp/aurum-fake-scm-repos"`, `"repository_root": "/workspace"`); err != nil {
			t.Fatalf("mutate repository root: %v", err)
		}
	case "repository-root-traversal":
		expected = "unsafe-plan"
		// A root that starts inside the bounded tmpfs and climbs out of it. The anchored
		// expression denies it for the same reason it denies an absolute host path.
		if err := replaceOnce409(root, profilePath409, `"repository_root": "/tmp/aurum-fake-scm-repos"`, `"repository_root": "/tmp/aurum-fake-scm-repos/../../etc"`); err != nil {
			t.Fatalf("mutate repository root: %v", err)
		}
	case "repository-root-trailing-slash":
		expected = "unsafe-plan"
		// A trailing slash is a different string, and a plan is admitted only for the
		// exact declared directory.
		if err := replaceOnce409(root, profilePath409, `"repository_root": "/tmp/aurum-fake-scm-repos"`, `"repository_root": "/tmp/aurum-fake-scm-repos/"`); err != nil {
			t.Fatalf("mutate repository root: %v", err)
		}
	case "repository-root-lookalike":
		expected = "unsafe-plan"
		// A sibling directory whose name merely starts with the bounded root. The
		// expression is anchored at the end as well, so the prefix does not admit it.
		if err := replaceOnce409(root, profilePath409, `"repository_root": "/tmp/aurum-fake-scm-repos"`, `"repository_root": "/tmp/aurum-fake-scm-repos-host"`); err != nil {
			t.Fatalf("mutate repository root: %v", err)
		}
	case "event-root-escape":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"event_root": "/tmp/aurum-fake-scm-events"`, `"event_root": "/workspace/.board"`); err != nil {
			t.Fatalf("mutate event root: %v", err)
		}
	case "fake-scm-root-escape":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"fake_scm_root": "/tmp/aurum-fake-scm-engine"`, `"fake_scm_root": "/usr/local/bin"`); err != nil {
			t.Fatalf("mutate fake scm root: %v", err)
		}
	case "unbounded-repository":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"max_repository_bytes": 33554432`, `"max_repository_bytes": 0`); err != nil {
			t.Fatalf("mutate repository bound: %v", err)
		}
	case "oversized-repository":
		expected = "unsafe-plan"
		// A ceiling far larger than the tmpfs that has to hold the simulated repositories.
		if err := replaceOnce409(root, profilePath409, `"max_repository_bytes": 33554432`, `"max_repository_bytes": 34359738368`); err != nil {
			t.Fatalf("mutate repository bound: %v", err)
		}
	case "oversized-event-log":
		expected = "unsafe-plan"
		// An event log whose declared entries cannot fit the declared input bound.
		if err := replaceOnce409(root, profilePath409, `"max_events": 1024`, `"max_events": 2048`); err != nil {
			t.Fatalf("mutate event bound: %v", err)
		}
	case "unbounded-response":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"max_response_bytes": 65536`, `"max_response_bytes": 0`); err != nil {
			t.Fatalf("mutate response bound: %v", err)
		}
	case "unlimited-tmpfs":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"tmpfs_mb": 128`, `"tmpfs_mb": 0`); err != nil {
			t.Fatalf("mutate tmpfs bound: %v", err)
		}
	case "askpass-environment":
		expected = "unsafe-plan"
		// The askpass channel re-opened through the environment instead of the plan field.
		if err := replaceOnce409(root, profilePath409, `"GIT_ASKPASS": "/bin/false"`, `"GIT_ASKPASS": "/usr/bin/ssh-askpass"`); err != nil {
			t.Fatalf("mutate askpass environment: %v", err)
		}
	case "protocol-environment":
		expected = "unsafe-plan"
		// The `ext::` transport re-enabled through the environment.
		if err := replaceOnce409(root, profilePath409, `"GIT_ALLOW_PROTOCOL": "none"`, `"GIT_ALLOW_PROTOCOL": "ext"`); err != nil {
			t.Fatalf("mutate protocol environment: %v", err)
		}
	case "system-config-environment":
		expected = "unsafe-plan"
		// System configuration re-enabled: that is where a credential helper and an
		// `insteadOf` rewrite would come from.
		if err := replaceOnce409(root, profilePath409, `"GIT_CONFIG_NOSYSTEM": "1"`, `"GIT_CONFIG_NOSYSTEM": "0"`); err != nil {
			t.Fatalf("mutate system config environment: %v", err)
		}
	case "global-config-environment":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"GIT_CONFIG_GLOBAL": "/dev/null"`, `"GIT_CONFIG_GLOBAL": "/tmp/aurum-fake-scm-engine/gitconfig"`); err != nil {
			t.Fatalf("mutate global config environment: %v", err)
		}
	case "terminal-prompt-environment":
		expected = "unsafe-plan"
		if err := replaceOnce409(root, profilePath409, `"GIT_TERMINAL_PROMPT": "0"`, `"GIT_TERMINAL_PROMPT": "1"`); err != nil {
			t.Fatalf("mutate terminal prompt environment: %v", err)
		}
	case "unpinned-fake-scm":
		expected = "digest-invalid"
		if err := replaceOnce409(root, profilePath409, `"fake_scm": "digest-pinned"`, `"fake_scm": "unpinned"`); err != nil {
			t.Fatalf("mutate fake scm: %v", err)
		}
	case "unpinned-event-set":
		expected = "digest-invalid"
		if err := replaceOnce409(root, profilePath409, `"event_set": "digest-pinned"`, `"event_set": "unpinned"`); err != nil {
			t.Fatalf("mutate event set: %v", err)
		}
	case "mutable-response-set":
		expected = "digest-invalid"
		// The response set referenced by a mutable tag instead of content.
		if err := replaceOnce409(root, profilePath409, `"response_set": "digest-pinned"`, `"response_set": "latest"`); err != nil {
			t.Fatalf("mutate response set: %v", err)
		}
	case "missing-scm-timeout":
		expected = "schema-invalid"
		if err := dropKey409(root, "scm_timeout_seconds"); err != nil {
			t.Fatalf("mutate scm timeout: %v", err)
		}
	case "real-credential":
		expected = "credential-present"
		// Assembled here, never written in this source file, so the runner's input
		// gate never sees a credential-shaped literal in a materialized path.
		fake := "gh" + "p_" + strings.Repeat("A", 36)
		if !credentialShape409.MatchString(fake) {
			t.Fatal("mutation vector is not credential shaped")
		}
		if err := replaceOnce409(root, profilePath409, `"HTTP_PROXY": ""`, `"HTTP_PROXY": "`+fake+`"`); err != nil {
			t.Fatalf("mutate environment: %v", err)
		}
	default:
		t.Fatalf("unknown mutation: %s", mutation)
	}

	digest, code := ValidateRegistryAUR409(root)
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
	registryBytes := mustRead409(t, root, registryPath409)
	if strings.Contains(string(registryBytes), "latest") {
		t.Fatal("mutable tag in registry")
	}
	// No materialized document this card owns may carry a credential shape.
	for _, path := range []string{registryPath409, profilePath409, lockPath409, schemaPath409} {
		if credentialShape409.Match(mustRead409(t, root, path)) {
			t.Fatalf("credential shape in %s", path)
		}
	}
	fmt.Printf("registry_digest=%s\n", digest)
}

func mustRead409(t *testing.T, root, path string) []byte {
	t.Helper()
	b, err := read409(root, path)
	if err != nil {
		t.Fatalf("unreadable %s: %v", path, err)
	}
	return b
}
