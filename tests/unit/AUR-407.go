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

const schemaPath407 = ".board/schemas/sqlite-offline-profile.schema.json"
const profilePath407 = ".board/oci/profiles/sqlite-offline-v1.json"
const lockPath407 = ".board/locks/oci/sqlite-offline-v1.lock.json"
const registryPath407 = ".board/oci/profiles/registry.v1.json"
const containerSchemaPath407 = ".board/schemas/container-profile.schema.json"
const registryLockPath407 = ".board/locks/oci/registry-v1.lock.json"
const goUnitSchemaPath407 = ".board/schemas/go-unit-offline-profile.schema.json"
const goUnitLockPath407 = ".board/locks/oci/go-unit-offline-v1.lock.json"
const goGitSchemaPath407 = ".board/schemas/go-git-offline-profile.schema.json"
const goGitLockPath407 = ".board/locks/oci/go-git-offline-v1.lock.json"
const fakeProviderSchemaPath407 = ".board/schemas/fake-provider-profile.schema.json"
const fakeProviderLockPath407 = ".board/locks/oci/fake-provider-v1.lock.json"
const parserWorkerSchemaPath407 = ".board/schemas/parser-worker-profile.schema.json"
const parserWorkerLockPath407 = ".board/locks/oci/parser-worker-v1.lock.json"
const docsToolSchemaPath407 = ".board/schemas/docs-tool-offline-profile.schema.json"
const docsToolLockPath407 = ".board/locks/oci/docs-tool-offline-v1.lock.json"
const fakeScmSchemaPath407 = ".board/schemas/fake-scm-offline-profile.schema.json"
const fakeScmLockPath407 = ".board/locks/oci/fake-scm-offline-v1.lock.json"
const profileKey407 = "sqlite-offline-v1"

type profile407 struct {
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
	SQLiteDriver        string            `json:"sqlite_driver"`
	SQLiteDriverCGO     bool              `json:"sqlite_driver_cgo"`
	DatabaseMode        string            `json:"database_mode"`
	DatabaseRoot        string            `json:"database_root"`
	HostDatabase        string            `json:"host_database"`
	ExtensionLoading    string            `json:"extension_loading"`
	RemoteURI           string            `json:"remote_uri"`
	ImplicitPersistence string            `json:"implicit_persistence"`
	AttachDatabase      string            `json:"attach_database"`
	JournalMode         string            `json:"journal_mode"`
	MaxDatabaseBytes    int               `json:"max_database_bytes"`
	MaxOpenConnections  int               `json:"max_open_connections"`
	BusyTimeoutMS       int               `json:"busy_timeout_ms"`
	QueryTimeoutSeconds int               `json:"query_timeout_seconds"`
	Environment         map[string]string `json:"environment"`
	Command             []string          `json:"command"`
}

type lock407 struct {
	Schema  string `json:"schema"`
	Version int    `json:"version"`
	Profile string `json:"profile"`
	Image   string `json:"image"`
}

type entry407 struct {
	Key            string `json:"key"`
	Schema         string `json:"schema"`
	SchemaDigest   string `json:"schema_digest"`
	Lock           string `json:"lock"`
	LockDigest     string `json:"lock_digest"`
	ImageSetDigest string `json:"image_set_digest"`
}

type registry407 struct {
	Schema   string     `json:"schema"`
	Version  int        `json:"version"`
	Profiles []entry407 `json:"profiles"`
}

var digest407 = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// The state store runs a pure-Go, cgo-free SQLite driver, so the plan needs a Go
// toolchain and Bash and neither a `sqlite3` binary nor a native SQLite library. It is
// pinned to the locally present bootstrap image by immutable digest; a mutable tag, a
// bare name, or any other repository denies before the plan is used.
var image407 = regexp.MustCompile(`^aurum-bootstrap-go-bash@sha256:[0-9a-f]{64}$`)

// The ephemeral database lives under the bounded tmpfs and nowhere else. The expression
// is anchored on both ends, so a checkout path, an absolute host path, and a traversal
// that starts inside the bounded root and climbs out of it are all denied.
var databaseRoot407 = regexp.MustCompile(`^/tmp/aurum-sqlite-state$`)

// Credential shapes are assembled from fragments so this source file itself never
// carries a literal that the runner's input gate would read as a real secret.
var credentialShape407 = regexp.MustCompile(`(sk` + `-[A-Za-z0-9_-]{20,}|AKIA` + `[0-9A-Z]{16}|gh` + `[pousr]_[A-Za-z0-9]{20,}|-----BEGIN [A-Z ]*PRIVATE KEY-----)`)

var registryKeys407 = []string{"bootstrap-readonly-v1", "docs-tool-offline-v1", "fake-provider-v1", "fake-scm-offline-v1", "go-git-offline-v1", "go-unit-offline-v1", "parser-worker-v1", "registry-v1", "sqlite-offline-v1"}
var requiredProfileKeys407 = []string{"schema", "version", "profile", "lock", "lock_digest", "network", "user", "cap_drop", "cap_add", "mounts", "devices", "sockets", "pull", "tmpfs", "checkout_readonly", "read_only_rootfs", "no_new_privileges", "privileged", "timeout_seconds", "memory_mb", "cpu_millis", "pids_limit", "tmpfs_mb", "stdout_limit_bytes", "stderr_limit_bytes", "max_input_files", "max_input_bytes", "module_cache", "module_cache_read_only", "sqlite_driver", "sqlite_driver_cgo", "database_mode", "database_root", "host_database", "extension_loading", "remote_uri", "implicit_persistence", "attach_database", "journal_mode", "max_database_bytes", "max_open_connections", "busy_timeout_ms", "query_timeout_seconds", "environment", "command"}
var requiredEnvironmentKeys407 = []string{"GOPROXY", "GOSUMDB", "GONOSUMDB", "GOTOOLCHAIN", "CGO_ENABLED", "AURUM_SQLITE_DATABASE_ROOT", "SQLITE_TMPDIR", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"}
var schemaDocumentKeys407 = []string{"$schema", "$id", "title", "type", "additionalProperties", "required", "properties"}
var expectedCommand407 = []string{"go", "test", "-mod=readonly", "./..."}

// The safety constants are pinned here, in Go, and not only in the schema bytes. A
// reviewer who relaxes the schema document and recomputes its `schema_digest` in the
// registry moves both files consistently; this map is the third copy that does not move
// with them, so the relaxation is still rejected.
var profileSchemaConstants407 = map[string]any{
	"schema":                 "aurum.sqlite-offline-profile",
	"version":                1,
	"profile":                profileKey407,
	"lock":                   lockPath407,
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
	"sqlite_driver":          "digest-pinned",
	"sqlite_driver_cgo":      false,
	"database_mode":          "ephemeral",
	"database_root":          "/tmp/aurum-sqlite-state",
	"host_database":          "denied",
	"extension_loading":      "denied",
	"remote_uri":             "denied",
	"implicit_persistence":   "denied",
	"attach_database":        "denied",
	"journal_mode":           "memory",
	"max_database_bytes":     33554432,
	"max_open_connections":   4,
	"busy_timeout_ms":        2000,
	"query_timeout_seconds":  5,
}

var environmentSchemaConstants407 = map[string]any{
	"GOPROXY":                    "off",
	"GOSUMDB":                    "off",
	"GONOSUMDB":                  "*",
	"GOTOOLCHAIN":                "local",
	"CGO_ENABLED":                "0",
	"AURUM_SQLITE_DATABASE_ROOT": "/tmp/aurum-sqlite-state",
	"SQLITE_TMPDIR":              "/tmp/aurum-sqlite-state",
	"HTTP_PROXY":                 "",
	"HTTPS_PROXY":                "",
	"NO_PROXY":                   "*",
}

func hash407(b []byte) string { h := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(h[:]) }

// rejectDuplicateJSON407 refuses a document whose last-wins duplicate key would let a
// registry publish one plan to a reader and another to the runner.
func rejectDuplicateJSON407(b []byte) error {
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

func decode407(b []byte, out any) error {
	if err := rejectDuplicateJSON407(b); err != nil {
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

func exactKeys407(raw map[string]json.RawMessage, expected []string) bool {
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

func exactStringSet407(actual, expected []string) bool {
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

func exactCommand407(command []string) bool {
	if len(command) != len(expectedCommand407) {
		return false
	}
	for i, token := range expectedCommand407 {
		if command[i] != token {
			return false
		}
	}
	return true
}

func schemaConst407(raw json.RawMessage, expected any) bool {
	var spec map[string]json.RawMessage
	if decode407(raw, &spec) != nil || !exactKeys407(spec, []string{"const"}) {
		return false
	}
	var actual any
	if decode407(spec["const"], &actual) != nil {
		return false
	}
	actualJSON, actualErr := json.Marshal(actual)
	expectedJSON, expectedErr := json.Marshal(expected)
	return actualErr == nil && expectedErr == nil && bytes.Equal(actualJSON, expectedJSON)
}

func validateSchemaInvariant407(schema []byte) bool {
	var raw map[string]json.RawMessage
	if decode407(schema, &raw) != nil || !exactKeys407(raw, schemaDocumentKeys407) {
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
	if decode407(schema, &doc) != nil ||
		doc.Schema != "https://json-schema.org/draft/2020-12/schema" ||
		doc.ID != "https://aurumcode.dev/schemas/sqlite-offline-profile.schema.json" ||
		doc.Title != "AurumCode offline SQLite state store profile" ||
		doc.Type != "object" || doc.AdditionalProperties ||
		!exactStringSet407(doc.Required, requiredProfileKeys407) ||
		!exactKeys407(doc.Properties, requiredProfileKeys407) {
		return false
	}
	for key, expected := range profileSchemaConstants407 {
		if !schemaConst407(doc.Properties[key], expected) {
			return false
		}
	}
	var digestSpec map[string]json.RawMessage
	if decode407(doc.Properties["lock_digest"], &digestSpec) != nil || !exactKeys407(digestSpec, []string{"type", "pattern"}) {
		return false
	}
	var digestRule struct {
		Type    string `json:"type"`
		Pattern string `json:"pattern"`
	}
	if decode407(doc.Properties["lock_digest"], &digestRule) != nil || digestRule.Type != "string" || digestRule.Pattern != "^sha256:[0-9a-f]{64}$" {
		return false
	}
	if !schemaConst407(doc.Properties["command"], expectedCommand407) {
		return false
	}
	var envRaw map[string]json.RawMessage
	if decode407(doc.Properties["environment"], &envRaw) != nil || !exactKeys407(envRaw, []string{"type", "additionalProperties", "required", "properties"}) {
		return false
	}
	var envSpec struct {
		Type                 string                     `json:"type"`
		AdditionalProperties bool                       `json:"additionalProperties"`
		Required             []string                   `json:"required"`
		Properties           map[string]json.RawMessage `json:"properties"`
	}
	if decode407(doc.Properties["environment"], &envSpec) != nil || envSpec.Type != "object" || envSpec.AdditionalProperties ||
		!exactStringSet407(envSpec.Required, requiredEnvironmentKeys407) ||
		!exactKeys407(envSpec.Properties, requiredEnvironmentKeys407) {
		return false
	}
	for key, expected := range environmentSchemaConstants407 {
		if !schemaConst407(envSpec.Properties[key], expected) {
			return false
		}
	}
	return true
}

func read407(root, path string) ([]byte, error) {
	if strings.Contains(path, "..") || strings.Contains(path, "//") {
		return nil, fmt.Errorf("unsafe path")
	}
	info, err := os.Lstat(filepath.Join(root, path))
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("missing")
	}
	return os.ReadFile(filepath.Join(root, path))
}

// ValidateProfileAUR407 is the side-effect-free plan loader for sqlite-offline-v1. It
// never invokes a container engine, never opens a socket, never spawns a subprocess,
// never opens a database, and never reads anything outside the documents this card owns.
func ValidateProfileAUR407(root string) (profile407, lock407, string) {
	sb, err := read407(root, schemaPath407)
	if err != nil {
		return profile407{}, lock407{}, "schema-missing"
	}
	if !validateSchemaInvariant407(sb) {
		return profile407{}, lock407{}, "schema-invalid"
	}
	pb, err := read407(root, profilePath407)
	if err != nil {
		return profile407{}, lock407{}, "profile-missing"
	}
	var raw map[string]json.RawMessage
	if decode407(pb, &raw) != nil || !exactKeys407(raw, requiredProfileKeys407) {
		return profile407{}, lock407{}, "schema-invalid"
	}
	var envRaw map[string]json.RawMessage
	if decode407(raw["environment"], &envRaw) != nil || !exactKeys407(envRaw, requiredEnvironmentKeys407) {
		return profile407{}, lock407{}, "schema-invalid"
	}
	var p profile407
	if decode407(pb, &p) != nil {
		return p, lock407{}, "schema-invalid"
	}
	// Credential shapes are refused before any other plan rule, so a value that could
	// authenticate against a real service is denied even when it arrives through a
	// field whose exact value is otherwise pinned by a constant below.
	for _, value := range p.Environment {
		if credentialShape407.MatchString(value) {
			return p, lock407{}, "credential-present"
		}
	}
	// The SQLite driver is admitted only while it is content-addressed. Its bytes belong
	// to a dependent implementation card; what this card pins is that an unpinned driver
	// can never be resolved.
	if p.SQLiteDriver != "digest-pinned" || !digest407.MatchString(p.LockDigest) {
		return p, lock407{}, "digest-invalid"
	}
	// Every bound is an exact constant, never a minimum: a value larger than the one
	// declared here is as unsafe as a value of zero, because the ephemeral database has
	// to fit inside the bounded tmpfs that holds it.
	if p.Schema != "aurum.sqlite-offline-profile" || p.Version != 1 || p.Profile != profileKey407 ||
		p.Lock != lockPath407 ||
		p.Network != "none" || p.User != "65534:65534" || p.CapDrop != "ALL" || p.CapAdd != "none" ||
		p.Mounts != "none" || p.Devices != "none" || p.Sockets != "none" ||
		p.Pull != "never" || p.Tmpfs != "rw,nosuid,nodev" ||
		!p.CheckoutReadonly || !p.ReadOnlyRootfs || !p.NoNewPrivileges || p.Privileged ||
		p.TimeoutSeconds != 60 || p.MemoryMB != 256 || p.CPUMillis != 1000 || p.PidsLimit != 128 ||
		p.TmpfsMB != 128 || p.StdoutLimitBytes != 65536 || p.StderrLimitBytes != 65536 ||
		p.MaxInputFiles != 10000 || p.MaxInputBytes != 67108864 ||
		p.ModuleCache != "/go/pkg/mod" || !p.ModuleCacheReadOnly ||
		p.SQLiteDriverCGO ||
		p.DatabaseMode != "ephemeral" || !databaseRoot407.MatchString(p.DatabaseRoot) ||
		p.HostDatabase != "denied" || p.ExtensionLoading != "denied" || p.RemoteURI != "denied" ||
		p.ImplicitPersistence != "denied" || p.AttachDatabase != "denied" ||
		p.JournalMode != "memory" ||
		p.MaxDatabaseBytes != 33554432 || p.MaxOpenConnections != 4 ||
		p.BusyTimeoutMS != 2000 || p.QueryTimeoutSeconds != 5 ||
		!exactCommand407(p.Command) {
		return p, lock407{}, "unsafe-plan"
	}
	// Independently of the exact constants above, the declared database ceiling has to
	// fit inside the declared tmpfs. A plan whose store cannot be held by its own bounded
	// filesystem would have to spill somewhere the plan never bounded.
	if p.MaxDatabaseBytes <= 0 || p.TmpfsMB <= 0 || p.MaxDatabaseBytes > p.TmpfsMB*1024*1024 {
		return p, lock407{}, "unsafe-plan"
	}
	for key, expected := range environmentSchemaConstants407 {
		if p.Environment[key] != expected.(string) {
			return p, lock407{}, "unsafe-plan"
		}
	}
	lb, err := read407(root, lockPath407)
	if err != nil {
		return p, lock407{}, "lock-missing"
	}
	var l lock407
	if decode407(lb, &l) != nil || l.Schema != "aurum.oci-image-lock" || l.Version != 1 ||
		l.Profile != p.Profile || !image407.MatchString(l.Image) {
		return p, l, "mutable-input"
	}
	if hash407(lb) != p.LockDigest {
		return p, l, "lock-digest-mismatch"
	}
	return p, l, "valid"
}

// ValidateRegistryAUR407 resolves the canonical registry and admits exactly the nine
// registered keys. It is fail-closed: an unknown key, a duplicate, an out-of-order
// entry, or a digest that does not match the bytes on disk denies without any engine.
func ValidateRegistryAUR407(root string) (string, string) {
	rb, err := read407(root, registryPath407)
	if err != nil {
		return "", "registry-missing"
	}
	var r registry407
	if decode407(rb, &r) != nil || r.Schema != "aurum.profile-registry" || r.Version != 1 {
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
		case profileKey407:
			found = true
			if x.Schema != schemaPath407 || x.Lock != lockPath407 {
				return "", "unsafe-plan"
			}
			sb, err := read407(root, schemaPath407)
			if err != nil {
				return "", "schema-missing"
			}
			if x.SchemaDigest != hash407(sb) {
				return "", "schema-digest-mismatch"
			}
			_, l, code := ValidateProfileAUR407(root)
			if code != "valid" {
				return "", code
			}
			lb, err := read407(root, lockPath407)
			if err != nil {
				return "", "lock-missing"
			}
			if x.LockDigest != hash407(lb) {
				return "", "lock-digest-mismatch"
			}
			if x.ImageSetDigest != hash407([]byte(l.Image)) {
				return "", "unsafe-plan"
			}
		case "bootstrap-readonly-v1", "registry-v1":
			if x.Schema != containerSchemaPath407 || x.Lock != registryLockPath407 {
				return "", "unsafe-plan"
			}
			sb, err := read407(root, containerSchemaPath407)
			if err != nil {
				return "", "schema-missing"
			}
			if x.SchemaDigest != hash407(sb) {
				return "", "schema-digest-mismatch"
			}
			lb, err := read407(root, registryLockPath407)
			if err != nil {
				return "", "lock-missing"
			}
			if x.LockDigest != hash407(lb) {
				return "", "lock-digest-mismatch"
			}
			var l lock407
			if decode407(lb, &l) != nil || l.Schema != "aurum.oci-image-lock" || l.Version != 1 {
				return "", "mutable-input"
			}
			if x.ImageSetDigest != hash407([]byte(l.Image)) {
				return "", "unsafe-plan"
			}
		case "go-unit-offline-v1":
			// Owned by AUR-403. Its documents are not materialized for this
			// acceptance, so only the declared paths and digest shape are checked; the
			// state store can never silently re-point the Go unit plan.
			if x.Schema != goUnitSchemaPath407 || x.Lock != goUnitLockPath407 {
				return "", "unsafe-plan"
			}
			if !digest407.MatchString(x.SchemaDigest) || !digest407.MatchString(x.LockDigest) || !digest407.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "go-git-offline-v1":
			// Owned by AUR-404, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != goGitSchemaPath407 || x.Lock != goGitLockPath407 {
				return "", "unsafe-plan"
			}
			if !digest407.MatchString(x.SchemaDigest) || !digest407.MatchString(x.LockDigest) || !digest407.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "fake-provider-v1":
			// Owned by AUR-405, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != fakeProviderSchemaPath407 || x.Lock != fakeProviderLockPath407 {
				return "", "unsafe-plan"
			}
			if !digest407.MatchString(x.SchemaDigest) || !digest407.MatchString(x.LockDigest) || !digest407.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "parser-worker-v1":
			// Owned by AUR-406, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != parserWorkerSchemaPath407 || x.Lock != parserWorkerLockPath407 {
				return "", "unsafe-plan"
			}
			if !digest407.MatchString(x.SchemaDigest) || !digest407.MatchString(x.LockDigest) || !digest407.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "docs-tool-offline-v1":
			// Owned by AUR-408, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != docsToolSchemaPath407 || x.Lock != docsToolLockPath407 {
				return "", "unsafe-plan"
			}
			if !digest407.MatchString(x.SchemaDigest) || !digest407.MatchString(x.LockDigest) || !digest407.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "fake-scm-offline-v1":
			// Owned by AUR-409, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != fakeScmSchemaPath407 || x.Lock != fakeScmLockPath407 {
				return "", "unsafe-plan"
			}
			if !digest407.MatchString(x.SchemaDigest) || !digest407.MatchString(x.LockDigest) || !digest407.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		default:
			return "", "profile-unregistered"
		}
	}
	if !found || !sort.StringsAreSorted(keys) || !exactStringSet407(keys, registryKeys407) {
		return "", "profile-unregistered"
	}
	return hash407(rb), "valid"
}

func writeMutation407(root, path string, data []byte) error {
	full := filepath.Join(root, path)
	if err := os.Chmod(full, 0600); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0600)
}

func replaceOnce407(root, path, from, to string) error {
	b, err := read407(root, path)
	if err != nil {
		return err
	}
	if !bytes.Contains(b, []byte(from)) {
		return fmt.Errorf("mutation anchor %q absent from %s", from, path)
	}
	return writeMutation407(root, path, bytes.Replace(b, []byte(from), []byte(to), 1))
}

// dropKey407 removes one required key from the profile document, which is how a
// declared bound goes missing in practice.
func dropKey407(root, key string) error {
	pb, err := read407(root, profilePath407)
	if err != nil {
		return err
	}
	var p map[string]any
	if err := decode407(pb, &p); err != nil {
		return err
	}
	if _, ok := p[key]; !ok {
		return fmt.Errorf("mutation anchor %q absent from %s", key, profilePath407)
	}
	delete(p, key)
	nb, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return writeMutation407(root, profilePath407, nb)
}

func TestAUR407(t *testing.T) {
	root := os.Getenv("AURUMCODE_ROOT")
	if root == "" {
		t.Fatal("AURUMCODE_ROOT is required")
	}
	mutation := os.Getenv("AURUM_A407_MUTATION")
	expected := mutation
	switch mutation {
	case "":
	case "duplicate-profile":
		rb, err := read407(root, registryPath407)
		if err != nil {
			t.Fatalf("registry read: %v", err)
		}
		var r registry407
		if decode407(rb, &r) != nil {
			t.Fatal("registry decode")
		}
		var clone entry407
		for _, x := range r.Profiles {
			if x.Key == profileKey407 {
				clone = x
			}
		}
		if clone.Key == "" {
			t.Fatal("mutation target absent: " + profileKey407)
		}
		// A second document for the same key, with different bytes.
		clone.LockDigest = "sha256:" + strings.Repeat("0", 64)
		r.Profiles = append(r.Profiles, clone)
		nb, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("registry marshal: %v", err)
		}
		if err := writeMutation407(root, registryPath407, nb); err != nil {
			t.Fatalf("mutate registry: %v", err)
		}
	case "mutable-input":
		lb, err := read407(root, lockPath407)
		if err != nil {
			t.Fatalf("lock read: %v", err)
		}
		lb = bytes.Replace(lb,
			[]byte("aurum-bootstrap-go-bash@sha256:503ac356fa6bca4bad56fade87b5479ff371bd446ffb9c7db91f211323c7c73e"),
			[]byte("aurum-bootstrap-go-bash:1.21"), 1)
		if !bytes.Contains(lb, []byte("aurum-bootstrap-go-bash:1.21")) {
			t.Fatal("mutation anchor absent from lock")
		}
		if err := writeMutation407(root, lockPath407, lb); err != nil {
			t.Fatalf("mutate lock: %v", err)
		}
		pb, err := read407(root, profilePath407)
		if err != nil {
			t.Fatalf("profile read: %v", err)
		}
		var p map[string]any
		if decode407(pb, &p) != nil {
			t.Fatal("profile decode")
		}
		p["lock_digest"] = hash407(lb)
		nb, err := json.Marshal(p)
		if err != nil {
			t.Fatalf("profile marshal: %v", err)
		}
		if err := writeMutation407(root, profilePath407, nb); err != nil {
			t.Fatalf("mutate profile: %v", err)
		}
	case "unsafe-plan":
		if err := replaceOnce407(root, profilePath407, `"network": "none"`, `"network": "host"`); err != nil {
			t.Fatalf("mutate network: %v", err)
		}
	case "mount-enabled":
		expected = "unsafe-plan"
		if err := replaceOnce407(root, profilePath407, `"mounts": "none"`, `"mounts": "bind"`); err != nil {
			t.Fatalf("mutate mounts: %v", err)
		}
	case "socket-enabled":
		expected = "unsafe-plan"
		if err := replaceOnce407(root, profilePath407, `"sockets": "none"`, `"sockets": "unix"`); err != nil {
			t.Fatalf("mutate sockets: %v", err)
		}
	case "device-enabled":
		expected = "unsafe-plan"
		if err := replaceOnce407(root, profilePath407, `"devices": "none"`, `"devices": "/dev/fuse"`); err != nil {
			t.Fatalf("mutate devices: %v", err)
		}
	case "root-user":
		expected = "unsafe-plan"
		if err := replaceOnce407(root, profilePath407, `"user": "65534:65534"`, `"user": "0:0"`); err != nil {
			t.Fatalf("mutate user: %v", err)
		}
	case "capability-added":
		expected = "unsafe-plan"
		if err := replaceOnce407(root, profilePath407, `"cap_add": "none"`, `"cap_add": "SYS_ADMIN"`); err != nil {
			t.Fatalf("mutate cap_add: %v", err)
		}
	case "host-database-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce407(root, profilePath407, `"host_database": "denied"`, `"host_database": "read-only"`); err != nil {
			t.Fatalf("mutate host database: %v", err)
		}
	case "extension-loading-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce407(root, profilePath407, `"extension_loading": "denied"`, `"extension_loading": "allowed"`); err != nil {
			t.Fatalf("mutate extension loading: %v", err)
		}
	case "remote-uri-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce407(root, profilePath407, `"remote_uri": "denied"`, `"remote_uri": "allowed"`); err != nil {
			t.Fatalf("mutate remote uri: %v", err)
		}
	case "implicit-persistence-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce407(root, profilePath407, `"implicit_persistence": "denied"`, `"implicit_persistence": "allowed"`); err != nil {
			t.Fatalf("mutate implicit persistence: %v", err)
		}
	case "attach-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce407(root, profilePath407, `"attach_database": "denied"`, `"attach_database": "allowed"`); err != nil {
			t.Fatalf("mutate attach: %v", err)
		}
	case "persistent-journal":
		expected = "unsafe-plan"
		if err := replaceOnce407(root, profilePath407, `"journal_mode": "memory"`, `"journal_mode": "wal"`); err != nil {
			t.Fatalf("mutate journal mode: %v", err)
		}
	case "database-root-escape":
		expected = "unsafe-plan"
		if err := replaceOnce407(root, profilePath407, `"database_root": "/tmp/aurum-sqlite-state"`, `"database_root": "/workspace/.board"`); err != nil {
			t.Fatalf("mutate database root: %v", err)
		}
	case "database-root-traversal":
		expected = "unsafe-plan"
		// A root that starts inside the bounded tmpfs and climbs out of it. The anchored
		// expression denies it for the same reason it denies an absolute host path.
		if err := replaceOnce407(root, profilePath407, `"database_root": "/tmp/aurum-sqlite-state"`, `"database_root": "/tmp/aurum-sqlite-state/../../etc"`); err != nil {
			t.Fatalf("mutate database root: %v", err)
		}
	case "cgo-driver":
		expected = "unsafe-plan"
		if err := replaceOnce407(root, profilePath407, `"sqlite_driver_cgo": false`, `"sqlite_driver_cgo": true`); err != nil {
			t.Fatalf("mutate driver linkage: %v", err)
		}
	case "unbounded-database":
		expected = "unsafe-plan"
		if err := replaceOnce407(root, profilePath407, `"max_database_bytes": 33554432`, `"max_database_bytes": 0`); err != nil {
			t.Fatalf("mutate database bound: %v", err)
		}
	case "oversized-database":
		expected = "unsafe-plan"
		// A ceiling far larger than the tmpfs that has to hold the store.
		if err := replaceOnce407(root, profilePath407, `"max_database_bytes": 33554432`, `"max_database_bytes": 34359738368`); err != nil {
			t.Fatalf("mutate database bound: %v", err)
		}
	case "unlimited-tmpfs":
		expected = "unsafe-plan"
		if err := replaceOnce407(root, profilePath407, `"tmpfs_mb": 128`, `"tmpfs_mb": 0`); err != nil {
			t.Fatalf("mutate tmpfs bound: %v", err)
		}
	case "unpinned-driver":
		expected = "digest-invalid"
		if err := replaceOnce407(root, profilePath407, `"sqlite_driver": "digest-pinned"`, `"sqlite_driver": "unpinned"`); err != nil {
			t.Fatalf("mutate driver: %v", err)
		}
	case "missing-query-timeout":
		expected = "schema-invalid"
		if err := dropKey407(root, "query_timeout_seconds"); err != nil {
			t.Fatalf("mutate query timeout: %v", err)
		}
	case "real-credential":
		expected = "credential-present"
		// Assembled here, never written in this source file, so the runner's input
		// gate never sees a credential-shaped literal in a materialized path.
		fake := "sk" + "-" + strings.Repeat("A", 32)
		if !credentialShape407.MatchString(fake) {
			t.Fatal("mutation vector is not credential shaped")
		}
		if err := replaceOnce407(root, profilePath407, `"HTTP_PROXY": ""`, `"HTTP_PROXY": "`+fake+`"`); err != nil {
			t.Fatalf("mutate environment: %v", err)
		}
	default:
		t.Fatalf("unknown mutation: %s", mutation)
	}

	digest, code := ValidateRegistryAUR407(root)
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
	registryBytes := mustRead407(t, root, registryPath407)
	if strings.Contains(string(registryBytes), "latest") {
		t.Fatal("mutable tag in registry")
	}
	// No materialized document this card owns may carry a credential shape.
	for _, path := range []string{registryPath407, profilePath407, lockPath407, schemaPath407} {
		if credentialShape407.Match(mustRead407(t, root, path)) {
			t.Fatalf("credential shape in %s", path)
		}
	}
	fmt.Printf("registry_digest=%s\n", digest)
}

func mustRead407(t *testing.T, root, path string) []byte {
	t.Helper()
	b, err := read407(root, path)
	if err != nil {
		t.Fatalf("unreadable %s: %v", path, err)
	}
	return b
}
