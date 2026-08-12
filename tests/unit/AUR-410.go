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

const schemaPath410 = ".board/schemas/oci-conformance-profile.schema.json"
const profilePath410 = ".board/oci/profiles/oci-conformance-v1.json"
const lockPath410 = ".board/locks/oci/oci-conformance-v1.lock.json"
const registryPath410 = ".board/oci/profiles/registry.v1.json"
const containerSchemaPath410 = ".board/schemas/container-profile.schema.json"
const registryLockPath410 = ".board/locks/oci/registry-v1.lock.json"
const goUnitSchemaPath410 = ".board/schemas/go-unit-offline-profile.schema.json"
const goUnitLockPath410 = ".board/locks/oci/go-unit-offline-v1.lock.json"
const goGitSchemaPath410 = ".board/schemas/go-git-offline-profile.schema.json"
const goGitLockPath410 = ".board/locks/oci/go-git-offline-v1.lock.json"
const fakeProviderSchemaPath410 = ".board/schemas/fake-provider-profile.schema.json"
const fakeProviderLockPath410 = ".board/locks/oci/fake-provider-v1.lock.json"
const parserWorkerSchemaPath410 = ".board/schemas/parser-worker-profile.schema.json"
const parserWorkerLockPath410 = ".board/locks/oci/parser-worker-v1.lock.json"
const sqliteSchemaPath410 = ".board/schemas/sqlite-offline-profile.schema.json"
const sqliteLockPath410 = ".board/locks/oci/sqlite-offline-v1.lock.json"
const docsToolSchemaPath410 = ".board/schemas/docs-tool-offline-profile.schema.json"
const docsToolLockPath410 = ".board/locks/oci/docs-tool-offline-v1.lock.json"
const fakeScmSchemaPath410 = ".board/schemas/fake-scm-offline-profile.schema.json"
const fakeScmLockPath410 = ".board/locks/oci/fake-scm-offline-v1.lock.json"
const profileKey410 = "oci-conformance-v1"

// The bounded size of one untrusted probe observation. It is the same value the plan
// publishes as max_probe_bytes, and it is applied to the observation before the
// observation is hashed, so an unbounded probe cannot make the orchestrator allocate.
const maxObservationBytes410 = 65536

type profile410 struct {
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
	Sockets               string            `json:"sockets"`
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
	Orchestrator          string            `json:"orchestrator"`
	OrchestratorRoot      string            `json:"orchestrator_root"`
	Probe                 string            `json:"probe"`
	ProbeRoot             string            `json:"probe_root"`
	ProbeImageSet         string            `json:"probe_image_set"`
	ProbeCGO              bool              `json:"probe_cgo"`
	ProbeOutput           string            `json:"probe_output"`
	ProbeVerdictAuthority string            `json:"probe_verdict_authority"`
	ProbeWritableRoots    []string          `json:"probe_writable_roots"`
	VerdictAuthority      string            `json:"verdict_authority"`
	ReportMode            string            `json:"report_mode"`
	ReportRoot            string            `json:"report_root"`
	SupportedEngines      []string          `json:"supported_engines"`
	UnsupportedEngine     string            `json:"unsupported_engine"`
	EngineInvocations     int               `json:"engine_invocations"`
	EngineSocket          string            `json:"engine_socket"`
	EngineAPI             string            `json:"engine_api"`
	HostEngine            string            `json:"host_engine"`
	Egress                string            `json:"egress"`
	HostCheckout          string            `json:"host_checkout"`
	HostFilesystem        string            `json:"host_filesystem"`
	Subprocess            string            `json:"subprocess"`
	MaxProbes             int               `json:"max_probes"`
	MaxProbeBytes         int               `json:"max_probe_bytes"`
	MaxReportBytes        int               `json:"max_report_bytes"`
	ProbeTimeoutSeconds   int               `json:"probe_timeout_seconds"`
	Environment           map[string]string `json:"environment"`
	Command               []string          `json:"command"`
}

type lock410 struct {
	Schema  string `json:"schema"`
	Version int    `json:"version"`
	Profile string `json:"profile"`
	Image   string `json:"image"`
}

type entry410 struct {
	Key            string `json:"key"`
	Schema         string `json:"schema"`
	SchemaDigest   string `json:"schema_digest"`
	Lock           string `json:"lock"`
	LockDigest     string `json:"lock_digest"`
	ImageSetDigest string `json:"image_set_digest"`
}

type registry410 struct {
	Schema   string     `json:"schema"`
	Version  int        `json:"version"`
	Profiles []entry410 `json:"profiles"`
}

// Every expression below is anchored with \A and \z, which in RE2 are the beginning and
// the end of the *text*, never the beginning and the end of a line. `^`/`$` would be the
// same by default in Go, but the intent is written explicitly here because a value that
// carries an embedded newline is exactly the shape a multiline anchor would admit.
var digest410 = regexp.MustCompile(`\Asha256:[0-9a-f]{64}\z`)

// The conformance orchestrator is pure Go with cgo disabled and needs a Go toolchain and
// Bash and nothing else. It is pinned to the locally present bootstrap image by immutable
// digest; a mutable tag, a bare name, or any other repository denies before the plan is
// used. Neither locally present image carries a container-engine CLI at all, and the
// bootstrap image additionally carries no `git`, no `ssh` and no `curl`; the busybox
// applets `wget`, `nc` and `ping` are present in it and reach nothing, because the plan
// runs with a single interface, `lo`.
var image410 = regexp.MustCompile(`\Aaurum-bootstrap-go-bash@sha256:[0-9a-f]{64}\z`)

// The trusted orchestrator, the untrusted probes and the rendered reports live under
// three bounded tmpfs directories and nowhere else. Every expression is anchored at both
// ends, so the checkout, an absolute host path, a trailing slash, a look-alike sibling, a
// traversal that starts inside the bounded root and climbs out of it, a value with an
// embedded newline, a leading space, an upper-cased spelling, a Unicode look-alike and
// the empty string are all denied alike.
var orchestratorRoot410 = regexp.MustCompile(`\A/tmp/aurum-oci-orchestrator\z`)
var probeRoot410 = regexp.MustCompile(`\A/tmp/aurum-oci-probes\z`)
var reportRoot410 = regexp.MustCompile(`\A/tmp/aurum-oci-reports\z`)

// The closed set of engines this profile may ever name. It is a denylist-free allowlist:
// an engine outside it is refused even when the plan document lists it, so widening the
// published list is not enough to make an unsupported engine admissible.
var supportedEngine410 = regexp.MustCompile(`\A(docker|podman)\z`)

// Credential shapes are assembled from fragments so this source file itself never
// carries a literal that the runner's input gate would read as a real secret.
var credentialShape410 = regexp.MustCompile(`(sk` + `-[A-Za-z0-9_-]{20,}|AKIA` + `[0-9A-Z]{16}|gh` + `[pousr]_[A-Za-z0-9]{20,}|-----BEGIN [A-Z ]*PRIVATE KEY-----)`)

var registryKeys410 = []string{"bootstrap-readonly-v1", "docs-tool-offline-v1", "fake-provider-v1", "fake-scm-offline-v1", "go-git-offline-v1", "go-unit-offline-v1", "oci-conformance-v1", "parser-worker-v1", "registry-v1", "sqlite-offline-v1"}
var requiredProfileKeys410 = []string{"schema", "version", "profile", "lock", "lock_digest", "network", "user", "cap_drop", "cap_add", "mounts", "devices", "sockets", "pull", "tmpfs", "checkout_readonly", "read_only_rootfs", "no_new_privileges", "privileged", "timeout_seconds", "memory_mb", "cpu_millis", "pids_limit", "tmpfs_mb", "stdout_limit_bytes", "stderr_limit_bytes", "max_input_files", "max_input_bytes", "module_cache", "module_cache_read_only", "orchestrator", "orchestrator_root", "probe", "probe_root", "probe_image_set", "probe_cgo", "probe_output", "probe_verdict_authority", "probe_writable_roots", "verdict_authority", "report_mode", "report_root", "supported_engines", "unsupported_engine", "engine_invocations", "engine_socket", "engine_api", "host_engine", "egress", "host_checkout", "host_filesystem", "subprocess", "max_probes", "max_probe_bytes", "max_report_bytes", "probe_timeout_seconds", "environment", "command"}
var requiredEnvironmentKeys410 = []string{"GOPROXY", "GOSUMDB", "GONOSUMDB", "GOTOOLCHAIN", "CGO_ENABLED", "AURUM_OCI_ORCHESTRATOR_ROOT", "AURUM_OCI_PROBE_ROOT", "AURUM_OCI_REPORT_ROOT", "DOCKER_HOST", "CONTAINER_HOST", "DOCKER_CONFIG", "DOCKER_API_VERSION", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"}
var schemaDocumentKeys410 = []string{"$schema", "$id", "title", "type", "additionalProperties", "required", "properties"}
var expectedCommand410 = []string{"go", "test", "-mod=readonly", "./..."}

// The two engines the conformance suite is allowed to target, compared element by
// element. Adding `nerdctl`, `ctr` or `crictl` to it is a plan change and is rejected
// exactly like enabling the network.
var expectedSupportedEngines410 = []string{"docker", "podman"}

// The probe's writable set is exactly its own bounded root, compared element by element.
// Adding the orchestrator root, the report root, or a parent of either, is rejected both
// by this equality and, independently of it, by the containment rule in the loader.
var expectedProbeWritableRoots410 = []string{"/tmp/aurum-oci-probes"}

// The safety constants are pinned here, in Go, and not only in the schema bytes. A
// reviewer who relaxes the schema document and recomputes its `schema_digest` in the
// registry moves both files consistently; this map is the third copy that does not move
// with them, so the relaxation is still rejected.
var profileSchemaConstants410 = map[string]any{
	"schema":                  "aurum.oci-conformance-profile",
	"version":                 1,
	"profile":                 profileKey410,
	"lock":                    lockPath410,
	"network":                 "none",
	"user":                    "65534:65534",
	"cap_drop":                "ALL",
	"cap_add":                 "none",
	"mounts":                  "none",
	"devices":                 "none",
	"sockets":                 "none",
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
	"orchestrator":            "trusted",
	"orchestrator_root":       "/tmp/aurum-oci-orchestrator",
	"probe":                   "untrusted",
	"probe_root":              "/tmp/aurum-oci-probes",
	"probe_image_set":         "digest-pinned",
	"probe_cgo":               false,
	"probe_output":            "untrusted-observation",
	"probe_verdict_authority": "denied",
	"probe_writable_roots":    expectedProbeWritableRoots410,
	"verdict_authority":       "orchestrator",
	"report_mode":             "ephemeral",
	"report_root":             "/tmp/aurum-oci-reports",
	"supported_engines":       expectedSupportedEngines410,
	"unsupported_engine":      "denied",
	"engine_invocations":      0,
	"engine_socket":           "denied",
	"engine_api":              "denied",
	"host_engine":             "denied",
	"egress":                  "denied",
	"host_checkout":           "denied",
	"host_filesystem":         "denied",
	"subprocess":              "denied",
	"max_probes":              1024,
	"max_probe_bytes":         65536,
	"max_report_bytes":        1048576,
	"probe_timeout_seconds":   5,
}

var environmentSchemaConstants410 = map[string]any{
	"GOPROXY":                     "off",
	"GOSUMDB":                     "off",
	"GONOSUMDB":                   "*",
	"GOTOOLCHAIN":                 "local",
	"CGO_ENABLED":                 "0",
	"AURUM_OCI_ORCHESTRATOR_ROOT": "/tmp/aurum-oci-orchestrator",
	"AURUM_OCI_PROBE_ROOT":        "/tmp/aurum-oci-probes",
	"AURUM_OCI_REPORT_ROOT":       "/tmp/aurum-oci-reports",
	// Every channel through which a container engine would be addressed is closed by
	// value, not merely unused: an empty endpoint names no engine, and the client
	// configuration directory is /dev/null, so no credential store, no context and no
	// remote API version can be read from the plan.
	"DOCKER_HOST":        "",
	"CONTAINER_HOST":     "",
	"DOCKER_CONFIG":      "/dev/null",
	"DOCKER_API_VERSION": "",
	"HTTP_PROXY":         "",
	"HTTPS_PROXY":        "",
	"NO_PROXY":           "*",
}

func hash410(b []byte) string { h := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(h[:]) }

// rejectDuplicateJSON410 refuses a document whose last-wins duplicate key would let a
// registry publish one plan to a reader and another to the runner.
func rejectDuplicateJSON410(b []byte) error {
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

func decode410(b []byte, out any) error {
	if err := rejectDuplicateJSON410(b); err != nil {
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

func exactKeys410(raw map[string]json.RawMessage, expected []string) bool {
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

func exactStringSet410(actual, expected []string) bool {
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

// exactStringList410 compares an ordered list element by element. It is used for the
// command, for the engine allowlist and for the probe's writable set, so neither a
// reordering, nor an extra element, nor a missing one can pass as the declared value.
func exactStringList410(actual, expected []string) bool {
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

func containsString410(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func schemaConst410(raw json.RawMessage, expected any) bool {
	var spec map[string]json.RawMessage
	if decode410(raw, &spec) != nil || !exactKeys410(spec, []string{"const"}) {
		return false
	}
	var actual any
	if decode410(spec["const"], &actual) != nil {
		return false
	}
	actualJSON, actualErr := json.Marshal(actual)
	expectedJSON, expectedErr := json.Marshal(expected)
	return actualErr == nil && expectedErr == nil && bytes.Equal(actualJSON, expectedJSON)
}

func validateSchemaInvariant410(schema []byte) bool {
	var raw map[string]json.RawMessage
	if decode410(schema, &raw) != nil || !exactKeys410(raw, schemaDocumentKeys410) {
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
	if decode410(schema, &doc) != nil ||
		doc.Schema != "https://json-schema.org/draft/2020-12/schema" ||
		doc.ID != "https://aurumcode.dev/schemas/oci-conformance-profile.schema.json" ||
		doc.Title != "AurumCode OCI conformance profile" ||
		doc.Type != "object" || doc.AdditionalProperties ||
		!exactStringSet410(doc.Required, requiredProfileKeys410) ||
		!exactKeys410(doc.Properties, requiredProfileKeys410) {
		return false
	}
	for key, expected := range profileSchemaConstants410 {
		if !schemaConst410(doc.Properties[key], expected) {
			return false
		}
	}
	var digestSpec map[string]json.RawMessage
	if decode410(doc.Properties["lock_digest"], &digestSpec) != nil || !exactKeys410(digestSpec, []string{"type", "pattern"}) {
		return false
	}
	var digestRule struct {
		Type    string `json:"type"`
		Pattern string `json:"pattern"`
	}
	if decode410(doc.Properties["lock_digest"], &digestRule) != nil || digestRule.Type != "string" || digestRule.Pattern != "^sha256:[0-9a-f]{64}$" {
		return false
	}
	if !schemaConst410(doc.Properties["command"], expectedCommand410) {
		return false
	}
	var envRaw map[string]json.RawMessage
	if decode410(doc.Properties["environment"], &envRaw) != nil || !exactKeys410(envRaw, []string{"type", "additionalProperties", "required", "properties"}) {
		return false
	}
	var envSpec struct {
		Type                 string                     `json:"type"`
		AdditionalProperties bool                       `json:"additionalProperties"`
		Required             []string                   `json:"required"`
		Properties           map[string]json.RawMessage `json:"properties"`
	}
	if decode410(doc.Properties["environment"], &envSpec) != nil || envSpec.Type != "object" || envSpec.AdditionalProperties ||
		!exactStringSet410(envSpec.Required, requiredEnvironmentKeys410) ||
		!exactKeys410(envSpec.Properties, requiredEnvironmentKeys410) {
		return false
	}
	for key, expected := range environmentSchemaConstants410 {
		if !schemaConst410(envSpec.Properties[key], expected) {
			return false
		}
	}
	return true
}

func read410(root, path string) ([]byte, error) {
	if strings.Contains(path, "..") || strings.Contains(path, "//") {
		return nil, fmt.Errorf("unsafe path")
	}
	info, err := os.Lstat(filepath.Join(root, path))
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("missing")
	}
	return os.ReadFile(filepath.Join(root, path))
}

// EngineSupportedAUR410 answers whether the plan admits one named engine. The published
// list is consulted only after the name has passed the closed expression above, so a
// widened list can never make `nerdctl`, `ctr` or `crictl` admissible, and neither an
// upper-cased spelling, a leading space, an embedded newline nor the empty string can
// impersonate an admitted name.
func EngineSupportedAUR410(p profile410, name string) bool {
	if p.UnsupportedEngine != "denied" || len(p.SupportedEngines) == 0 {
		return false
	}
	if !supportedEngine410.MatchString(name) {
		return false
	}
	return containsString410(p.SupportedEngines, name)
}

// ValidateProfileAUR410 is the side-effect-free plan loader for oci-conformance-v1. It
// never invokes a container engine, never opens an engine socket, never opens a network
// socket, never spawns a subprocess, and never reads anything outside the documents this
// card owns.
func ValidateProfileAUR410(root string) (profile410, lock410, string) {
	sb, err := read410(root, schemaPath410)
	if err != nil {
		return profile410{}, lock410{}, "schema-missing"
	}
	if !validateSchemaInvariant410(sb) {
		return profile410{}, lock410{}, "schema-invalid"
	}
	pb, err := read410(root, profilePath410)
	if err != nil {
		return profile410{}, lock410{}, "profile-missing"
	}
	var raw map[string]json.RawMessage
	if decode410(pb, &raw) != nil || !exactKeys410(raw, requiredProfileKeys410) {
		return profile410{}, lock410{}, "schema-invalid"
	}
	var envRaw map[string]json.RawMessage
	if decode410(raw["environment"], &envRaw) != nil || !exactKeys410(envRaw, requiredEnvironmentKeys410) {
		return profile410{}, lock410{}, "schema-invalid"
	}
	var p profile410
	if decode410(pb, &p) != nil {
		return p, lock410{}, "schema-invalid"
	}
	// Credential shapes are refused before any other plan rule, so a value that could
	// authenticate against a registry or an engine API is denied even when it arrives
	// through a field whose exact value is otherwise pinned by a constant below.
	for _, value := range p.Environment {
		if credentialShape410.MatchString(value) {
			return p, lock410{}, "credential-present"
		}
	}
	// The probe image set is admitted only while it is content-addressed. Its bytes
	// belong to a dependent implementation card; what this card pins is that an unpinned
	// or tag-referenced image can never be resolved, so a probe cannot change under a
	// fixed plan.
	if p.ProbeImageSet != "digest-pinned" || !digest410.MatchString(p.LockDigest) {
		return p, lock410{}, "digest-invalid"
	}
	// Every bound is an exact constant, never a minimum: a value larger than the one
	// declared here is as unsafe as a value of zero, because the rendered report has to
	// fit inside the bounded tmpfs that holds it.
	if p.Schema != "aurum.oci-conformance-profile" || p.Version != 1 || p.Profile != profileKey410 ||
		p.Lock != lockPath410 ||
		p.Network != "none" || p.User != "65534:65534" || p.CapDrop != "ALL" || p.CapAdd != "none" ||
		p.Mounts != "none" || p.Devices != "none" || p.Sockets != "none" ||
		p.Pull != "never" || p.Tmpfs != "rw,nosuid,nodev" ||
		!p.CheckoutReadonly || !p.ReadOnlyRootfs || !p.NoNewPrivileges || p.Privileged ||
		p.TimeoutSeconds != 60 || p.MemoryMB != 256 || p.CPUMillis != 1000 || p.PidsLimit != 128 ||
		p.TmpfsMB != 128 || p.StdoutLimitBytes != 65536 || p.StderrLimitBytes != 65536 ||
		p.MaxInputFiles != 10000 || p.MaxInputBytes != 67108864 ||
		p.ModuleCache != "/go/pkg/mod" || !p.ModuleCacheReadOnly ||
		p.Orchestrator != "trusted" || !orchestratorRoot410.MatchString(p.OrchestratorRoot) ||
		p.Probe != "untrusted" || !probeRoot410.MatchString(p.ProbeRoot) || p.ProbeCGO ||
		p.ProbeOutput != "untrusted-observation" || p.ProbeVerdictAuthority != "denied" ||
		!exactStringList410(p.ProbeWritableRoots, expectedProbeWritableRoots410) ||
		p.VerdictAuthority != "orchestrator" ||
		p.ReportMode != "ephemeral" || !reportRoot410.MatchString(p.ReportRoot) ||
		!exactStringList410(p.SupportedEngines, expectedSupportedEngines410) ||
		p.UnsupportedEngine != "denied" || p.EngineInvocations != 0 ||
		p.EngineSocket != "denied" || p.EngineAPI != "denied" || p.HostEngine != "denied" ||
		p.Egress != "denied" || p.HostCheckout != "denied" || p.HostFilesystem != "denied" ||
		p.Subprocess != "denied" ||
		p.MaxProbes != 1024 || p.MaxProbeBytes != 65536 ||
		p.MaxReportBytes != 1048576 || p.ProbeTimeoutSeconds != 5 ||
		!exactStringList410(p.Command, expectedCommand410) {
		return p, lock410{}, "unsafe-plan"
	}
	// The trust boundary has to exist. Independently of the constants above, the
	// orchestrator and the probe can never carry the same trust label, and the side that
	// issues the verdict can never be the untrusted one: a plan in which both sides are
	// "trusted" would have no boundary left to enforce.
	if p.Orchestrator == p.Probe || p.VerdictAuthority == "probe" || p.ProbeVerdictAuthority != "denied" {
		return p, lock410{}, "unsafe-plan"
	}
	// The engine allowlist is closed. Independently of the exact list above, every named
	// engine has to be one this profile supports, and no engine may be named twice.
	if len(p.SupportedEngines) == 0 {
		return p, lock410{}, "unsafe-plan"
	}
	namedEngines := map[string]bool{}
	for _, engine := range p.SupportedEngines {
		if !supportedEngine410.MatchString(engine) || namedEngines[engine] {
			return p, lock410{}, "unsafe-plan"
		}
		namedEngines[engine] = true
	}
	// The probe writes only inside its own bounded root. Independently of the exact list
	// above, no declared writable root may be, or contain, a root the orchestrator reads:
	// a probe that could write the orchestrator root or the report root would be writing
	// its own verdict. `/tmp` as a writable root is refused by this rule alone.
	if len(p.ProbeWritableRoots) == 0 || !containsString410(p.ProbeWritableRoots, p.ProbeRoot) {
		return p, lock410{}, "unsafe-plan"
	}
	for _, writable := range p.ProbeWritableRoots {
		if writable == "" {
			return p, lock410{}, "unsafe-plan"
		}
		for _, guarded := range []string{p.OrchestratorRoot, p.ReportRoot} {
			if guarded == writable || strings.HasPrefix(guarded, writable+"/") {
				return p, lock410{}, "unsafe-plan"
			}
		}
	}
	// Independently of the exact constants above, the rendered report has to fit inside
	// the declared tmpfs, and the probe set has to fit inside the declared input bound. A
	// plan whose report or whose probe corpus overflows its own bound would have to spill
	// somewhere the plan never bounded.
	if p.MaxReportBytes <= 0 || p.TmpfsMB <= 0 || p.MaxReportBytes > p.TmpfsMB*1024*1024 {
		return p, lock410{}, "unsafe-plan"
	}
	if p.MaxProbes <= 0 || p.MaxProbeBytes <= 0 ||
		p.MaxProbes > p.MaxInputFiles || p.MaxProbes*p.MaxProbeBytes > p.MaxInputBytes {
		return p, lock410{}, "unsafe-plan"
	}
	if p.ProbeTimeoutSeconds <= 0 || p.ProbeTimeoutSeconds > p.TimeoutSeconds {
		return p, lock410{}, "unsafe-plan"
	}
	// The three bounded roots are distinct directories and none of them contains another.
	// A probe root that sat under the orchestrator root, or a report root that sat under
	// the probe root, would put the untrusted side inside the trusted side's tree.
	for _, pair := range [][2]string{
		{p.OrchestratorRoot, p.ProbeRoot},
		{p.OrchestratorRoot, p.ReportRoot},
		{p.ProbeRoot, p.ReportRoot},
	} {
		if pair[0] == pair[1] ||
			strings.HasPrefix(pair[0], pair[1]+"/") || strings.HasPrefix(pair[1], pair[0]+"/") {
			return p, lock410{}, "unsafe-plan"
		}
	}
	for key, expected := range environmentSchemaConstants410 {
		if p.Environment[key] != expected.(string) {
			return p, lock410{}, "unsafe-plan"
		}
	}
	lb, err := read410(root, lockPath410)
	if err != nil {
		return p, lock410{}, "lock-missing"
	}
	var l lock410
	if decode410(lb, &l) != nil || l.Schema != "aurum.oci-image-lock" || l.Version != 1 ||
		l.Profile != p.Profile || !image410.MatchString(l.Image) {
		return p, l, "mutable-input"
	}
	if hash410(lb) != p.LockDigest {
		return p, l, "lock-digest-mismatch"
	}
	return p, l, "valid"
}

// ValidateRegistryAUR410 resolves the canonical registry and admits exactly the ten
// registered keys. It is fail-closed: an unknown key, a duplicate, an out-of-order
// entry, or a digest that does not match the bytes on disk denies without any engine.
func ValidateRegistryAUR410(root string) (string, string) {
	rb, err := read410(root, registryPath410)
	if err != nil {
		return "", "registry-missing"
	}
	var r registry410
	if decode410(rb, &r) != nil || r.Schema != "aurum.profile-registry" || r.Version != 1 {
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
		case profileKey410:
			found = true
			if x.Schema != schemaPath410 || x.Lock != lockPath410 {
				return "", "unsafe-plan"
			}
			sb, err := read410(root, schemaPath410)
			if err != nil {
				return "", "schema-missing"
			}
			if x.SchemaDigest != hash410(sb) {
				return "", "schema-digest-mismatch"
			}
			_, l, code := ValidateProfileAUR410(root)
			if code != "valid" {
				return "", code
			}
			lb, err := read410(root, lockPath410)
			if err != nil {
				return "", "lock-missing"
			}
			if x.LockDigest != hash410(lb) {
				return "", "lock-digest-mismatch"
			}
			if x.ImageSetDigest != hash410([]byte(l.Image)) {
				return "", "unsafe-plan"
			}
		case "bootstrap-readonly-v1", "registry-v1":
			if x.Schema != containerSchemaPath410 || x.Lock != registryLockPath410 {
				return "", "unsafe-plan"
			}
			sb, err := read410(root, containerSchemaPath410)
			if err != nil {
				return "", "schema-missing"
			}
			if x.SchemaDigest != hash410(sb) {
				return "", "schema-digest-mismatch"
			}
			lb, err := read410(root, registryLockPath410)
			if err != nil {
				return "", "lock-missing"
			}
			if x.LockDigest != hash410(lb) {
				return "", "lock-digest-mismatch"
			}
			var l lock410
			if decode410(lb, &l) != nil || l.Schema != "aurum.oci-image-lock" || l.Version != 1 {
				return "", "mutable-input"
			}
			if x.ImageSetDigest != hash410([]byte(l.Image)) {
				return "", "unsafe-plan"
			}
		case "go-unit-offline-v1":
			// Owned by AUR-403. Its documents are not materialized for this
			// acceptance, so only the declared paths and digest shape are checked; the
			// conformance profile can never silently re-point the Go unit plan.
			if x.Schema != goUnitSchemaPath410 || x.Lock != goUnitLockPath410 {
				return "", "unsafe-plan"
			}
			if !digest410.MatchString(x.SchemaDigest) || !digest410.MatchString(x.LockDigest) || !digest410.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "go-git-offline-v1":
			// Owned by AUR-404, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != goGitSchemaPath410 || x.Lock != goGitLockPath410 {
				return "", "unsafe-plan"
			}
			if !digest410.MatchString(x.SchemaDigest) || !digest410.MatchString(x.LockDigest) || !digest410.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "fake-provider-v1":
			// Owned by AUR-405, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != fakeProviderSchemaPath410 || x.Lock != fakeProviderLockPath410 {
				return "", "unsafe-plan"
			}
			if !digest410.MatchString(x.SchemaDigest) || !digest410.MatchString(x.LockDigest) || !digest410.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "parser-worker-v1":
			// Owned by AUR-406, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != parserWorkerSchemaPath410 || x.Lock != parserWorkerLockPath410 {
				return "", "unsafe-plan"
			}
			if !digest410.MatchString(x.SchemaDigest) || !digest410.MatchString(x.LockDigest) || !digest410.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "sqlite-offline-v1":
			// Owned by AUR-407, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != sqliteSchemaPath410 || x.Lock != sqliteLockPath410 {
				return "", "unsafe-plan"
			}
			if !digest410.MatchString(x.SchemaDigest) || !digest410.MatchString(x.LockDigest) || !digest410.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "docs-tool-offline-v1":
			// Owned by AUR-408, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != docsToolSchemaPath410 || x.Lock != docsToolLockPath410 {
				return "", "unsafe-plan"
			}
			if !digest410.MatchString(x.SchemaDigest) || !digest410.MatchString(x.LockDigest) || !digest410.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		case "fake-scm-offline-v1":
			// Owned by AUR-409, checked under the same neighbour rule as AUR-403's key.
			if x.Schema != fakeScmSchemaPath410 || x.Lock != fakeScmLockPath410 {
				return "", "unsafe-plan"
			}
			if !digest410.MatchString(x.SchemaDigest) || !digest410.MatchString(x.LockDigest) || !digest410.MatchString(x.ImageSetDigest) {
				return "", "digest-invalid"
			}
		default:
			return "", "profile-unregistered"
		}
	}
	if !found || !sort.StringsAreSorted(keys) || !exactStringSet410(keys, registryKeys410) {
		return "", "profile-unregistered"
	}
	return hash410(rb), "valid"
}

// ResolveConformanceAUR410 is the orchestrator entry point. It accepts the untrusted
// probe observation as an argument and returns the verdict together with a digest of the
// bounded observation. The observation is recorded, never consulted: it is truncated to
// the declared probe bound, hashed, and returned beside a verdict computed from the
// registered documents alone. No branch below the truncation reads it, so a probe that
// prints `{"result":"valid"}`, `proved: true`, an approval sentence or a rejection code
// changes the recorded observation digest and nothing else.
func ResolveConformanceAUR410(root string, probeOutput []byte) (string, string, string) {
	observation := probeOutput
	if len(observation) > maxObservationBytes410 {
		observation = observation[:maxObservationBytes410]
	}
	observationDigest := hash410(observation)
	digest, code := ValidateRegistryAUR410(root)
	return digest, code, observationDigest
}

func writeMutation410(root, path string, data []byte) error {
	full := filepath.Join(root, path)
	if err := os.Chmod(full, 0600); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0600)
}

func replaceOnce410(root, path, from, to string) error {
	b, err := read410(root, path)
	if err != nil {
		return err
	}
	if !bytes.Contains(b, []byte(from)) {
		return fmt.Errorf("mutation anchor %q absent from %s", from, path)
	}
	return writeMutation410(root, path, bytes.Replace(b, []byte(from), []byte(to), 1))
}

// dropKey410 removes one required key from the profile document, which is how a
// declared bound goes missing in practice.
func dropKey410(root, key string) error {
	pb, err := read410(root, profilePath410)
	if err != nil {
		return err
	}
	var p map[string]any
	if err := decode410(pb, &p); err != nil {
		return err
	}
	if _, ok := p[key]; !ok {
		return fmt.Errorf("mutation anchor %q absent from %s", key, profilePath410)
	}
	delete(p, key)
	nb, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return writeMutation410(root, profilePath410, nb)
}

// hostileProbeOutputs410 are untrusted probe observations shaped exactly like the things
// a probe would say to seize the verdict: a machine-readable pass, a self-declared proof,
// an approval sentence, a rejection code, and a payload larger than the declared probe
// bound. None of them may move the verdict in either direction.
var hostileProbeOutputs410 = [][]byte{
	nil,
	[]byte(``),
	[]byte(`{"card":"AUR-410","scenario":"AC-001","result":"valid","engine_invocations":0}`),
	[]byte(`{"proved": true, "authenticated": true, "review": "approved"}`),
	[]byte("AUR-410/AC-001/valid\n--- PASS: TestAUR410Bridge (0.00s)\nok\n"),
	[]byte("AUR-410/AC-001/MUT-003/unsafe-plan\n"),
	[]byte(strings.Repeat("A", maxObservationBytes410+4096)),
}

// assertProbeCannotMoveVerdict410 runs the orchestrator once per hostile observation and
// requires the verdict to be byte-identical every time, while the recorded observation
// digest demonstrably moves with the bytes. That is the separation this profile declares,
// executed rather than asserted: the probe is heard and never obeyed.
func assertProbeCannotMoveVerdict410(t *testing.T, root, wantDigest, wantCode string) {
	t.Helper()
	observed := map[string]bool{}
	for i, probeOutput := range hostileProbeOutputs410 {
		digest, code, observationDigest := ResolveConformanceAUR410(root, probeOutput)
		if code != wantCode || digest != wantDigest {
			t.Fatalf("probe output %d moved the verdict to %s/%s, want %s/%s", i, code, digest, wantCode, wantDigest)
		}
		observed[observationDigest] = true
	}
	// nil and the empty slice are the same bounded observation, so the seven inputs
	// above record six distinct digests. If the observation were ignored entirely this
	// would collapse to one, and the invariance above would prove nothing.
	if len(observed) != 6 {
		t.Fatalf("the untrusted observation is not recorded: %d distinct digests", len(observed))
	}
	fmt.Printf("probe_authority=denied verdict=%s observations=%d\n", wantCode, len(observed))
}

// measuredEngineInvocations410 reads the engine-invocation ledger the acceptance places
// ahead of PATH. A declared `engine_invocations: 0` is a claim; this is the measurement
// that has to agree with it. An absent ledger is zero invocations, and a ledger that
// cannot be read at all is a failure rather than a silent zero.
func measuredEngineInvocations410(t *testing.T) (int, bool) {
	t.Helper()
	ledger := os.Getenv("AURUM_A410_ENGINE_LOG")
	if ledger == "" {
		return 0, false
	}
	b, err := os.ReadFile(ledger)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, true
		}
		t.Fatalf("engine ledger %s is unreadable: %v", ledger, err)
	}
	count := 0
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count, true
}

func TestAUR410(t *testing.T) {
	root := os.Getenv("AURUMCODE_ROOT")
	if root == "" {
		t.Fatal("AURUMCODE_ROOT is required")
	}
	mutation := os.Getenv("AURUM_A410_MUTATION")
	expected := mutation
	switch mutation {
	case "":
	case "duplicate-profile":
		rb, err := read410(root, registryPath410)
		if err != nil {
			t.Fatalf("registry read: %v", err)
		}
		var r registry410
		if decode410(rb, &r) != nil {
			t.Fatal("registry decode")
		}
		var clone entry410
		for _, x := range r.Profiles {
			if x.Key == profileKey410 {
				clone = x
			}
		}
		if clone.Key == "" {
			t.Fatal("mutation target absent: " + profileKey410)
		}
		// A second document for the same key, with different bytes.
		clone.LockDigest = "sha256:" + strings.Repeat("0", 64)
		r.Profiles = append(r.Profiles, clone)
		nb, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("registry marshal: %v", err)
		}
		if err := writeMutation410(root, registryPath410, nb); err != nil {
			t.Fatalf("mutate registry: %v", err)
		}
	case "mutable-input":
		lb, err := read410(root, lockPath410)
		if err != nil {
			t.Fatalf("lock read: %v", err)
		}
		lb = bytes.Replace(lb,
			[]byte("aurum-bootstrap-go-bash@sha256:503ac356fa6bca4bad56fade87b5479ff371bd446ffb9c7db91f211323c7c73e"),
			[]byte("aurum-bootstrap-go-bash:1.21"), 1)
		if !bytes.Contains(lb, []byte("aurum-bootstrap-go-bash:1.21")) {
			t.Fatal("mutation anchor absent from lock")
		}
		if err := writeMutation410(root, lockPath410, lb); err != nil {
			t.Fatalf("mutate lock: %v", err)
		}
		pb, err := read410(root, profilePath410)
		if err != nil {
			t.Fatalf("profile read: %v", err)
		}
		var p map[string]any
		if decode410(pb, &p) != nil {
			t.Fatal("profile decode")
		}
		p["lock_digest"] = hash410(lb)
		nb, err := json.Marshal(p)
		if err != nil {
			t.Fatalf("profile marshal: %v", err)
		}
		if err := writeMutation410(root, profilePath410, nb); err != nil {
			t.Fatalf("mutate profile: %v", err)
		}
	case "unsafe-plan":
		if err := replaceOnce410(root, profilePath410, `"network": "none"`, `"network": "host"`); err != nil {
			t.Fatalf("mutate network: %v", err)
		}
	case "mount-enabled":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"mounts": "none"`, `"mounts": "bind"`); err != nil {
			t.Fatalf("mutate mounts: %v", err)
		}
	case "socket-enabled":
		expected = "unsafe-plan"
		// The engine socket is the single mount that turns a conformance probe into
		// full control of the host engine.
		if err := replaceOnce410(root, profilePath410, `"sockets": "none"`, `"sockets": "/var/run/docker.sock"`); err != nil {
			t.Fatalf("mutate sockets: %v", err)
		}
	case "device-enabled":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"devices": "none"`, `"devices": "/dev/fuse"`); err != nil {
			t.Fatalf("mutate devices: %v", err)
		}
	case "root-user":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"user": "65534:65534"`, `"user": "0:0"`); err != nil {
			t.Fatalf("mutate user: %v", err)
		}
	case "privileged-enabled":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"privileged": false`, `"privileged": true`); err != nil {
			t.Fatalf("mutate privileged: %v", err)
		}
	case "capability-added":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"cap_add": "none"`, `"cap_add": "SYS_ADMIN"`); err != nil {
			t.Fatalf("mutate cap_add: %v", err)
		}
	case "capability-kept":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"cap_drop": "ALL"`, `"cap_drop": "NET_RAW"`); err != nil {
			t.Fatalf("mutate cap_drop: %v", err)
		}
	case "writable-rootfs":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"read_only_rootfs": true`, `"read_only_rootfs": false`); err != nil {
			t.Fatalf("mutate read only rootfs: %v", err)
		}
	case "new-privileges-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"no_new_privileges": true`, `"no_new_privileges": false`); err != nil {
			t.Fatalf("mutate no new privileges: %v", err)
		}
	case "writable-checkout":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"checkout_readonly": true`, `"checkout_readonly": false`); err != nil {
			t.Fatalf("mutate checkout readonly: %v", err)
		}
	case "egress-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"egress": "denied"`, `"egress": "allowed"`); err != nil {
			t.Fatalf("mutate egress: %v", err)
		}
	case "probe-output-authoritative":
		expected = "unsafe-plan"
		// The probe's own output promoted from observation to authority. This is the
		// single field that decides whether a conformance run is a measurement or a
		// self-declared result.
		if err := replaceOnce410(root, profilePath410, `"probe_output": "untrusted-observation"`, `"probe_output": "authoritative"`); err != nil {
			t.Fatalf("mutate probe output: %v", err)
		}
	case "probe-output-trusted":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"probe_output": "untrusted-observation"`, `"probe_output": "trusted-result"`); err != nil {
			t.Fatalf("mutate probe output: %v", err)
		}
	case "probe-verdict-advisory":
		expected = "unsafe-plan"
		// An "advisory" verdict from the untrusted side is still a verdict from the
		// untrusted side.
		if err := replaceOnce410(root, profilePath410, `"probe_verdict_authority": "denied"`, `"probe_verdict_authority": "advisory"`); err != nil {
			t.Fatalf("mutate probe verdict authority: %v", err)
		}
	case "verdict-authority-probe":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"verdict_authority": "orchestrator"`, `"verdict_authority": "probe"`); err != nil {
			t.Fatalf("mutate verdict authority: %v", err)
		}
	case "probe-trusted":
		expected = "unsafe-plan"
		// The boundary collapsed from the probe's side.
		if err := replaceOnce410(root, profilePath410, `"probe": "untrusted"`, `"probe": "trusted"`); err != nil {
			t.Fatalf("mutate probe trust: %v", err)
		}
	case "orchestrator-untrusted":
		expected = "unsafe-plan"
		// The boundary collapsed from the orchestrator's side.
		if err := replaceOnce410(root, profilePath410, `"orchestrator": "trusted"`, `"orchestrator": "untrusted"`); err != nil {
			t.Fatalf("mutate orchestrator trust: %v", err)
		}
	case "probe-writes-orchestrator-root":
		expected = "unsafe-plan"
		// The probe granted write access to the directory the orchestrator reads.
		if err := replaceOnce410(root, profilePath410, `"probe_writable_roots": ["/tmp/aurum-oci-probes"]`, `"probe_writable_roots": ["/tmp/aurum-oci-probes", "/tmp/aurum-oci-orchestrator"]`); err != nil {
			t.Fatalf("mutate probe writable roots: %v", err)
		}
	case "probe-writes-report-root":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"probe_writable_roots": ["/tmp/aurum-oci-probes"]`, `"probe_writable_roots": ["/tmp/aurum-oci-probes", "/tmp/aurum-oci-reports"]`); err != nil {
			t.Fatalf("mutate probe writable roots: %v", err)
		}
	case "probe-writes-parent-root":
		expected = "unsafe-plan"
		// A writable root that contains all three bounded roots. It names neither the
		// orchestrator root nor the report root, and the containment rule denies it.
		if err := replaceOnce410(root, profilePath410, `"probe_writable_roots": ["/tmp/aurum-oci-probes"]`, `"probe_writable_roots": ["/tmp"]`); err != nil {
			t.Fatalf("mutate probe writable roots: %v", err)
		}
	case "probe-writable-roots-empty":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"probe_writable_roots": ["/tmp/aurum-oci-probes"]`, `"probe_writable_roots": []`); err != nil {
			t.Fatalf("mutate probe writable roots: %v", err)
		}
	case "engine-list-widened":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"supported_engines": ["docker", "podman"]`, `"supported_engines": ["docker", "nerdctl", "podman"]`); err != nil {
			t.Fatalf("mutate supported engines: %v", err)
		}
	case "engine-list-unsupported":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"supported_engines": ["docker", "podman"]`, `"supported_engines": ["ctr"]`); err != nil {
			t.Fatalf("mutate supported engines: %v", err)
		}
	case "engine-list-empty":
		expected = "unsafe-plan"
		// No engine at all is not a safe default: it is a plan that can never be
		// executed and whose allowlist proves nothing.
		if err := replaceOnce410(root, profilePath410, `"supported_engines": ["docker", "podman"]`, `"supported_engines": []`); err != nil {
			t.Fatalf("mutate supported engines: %v", err)
		}
	case "engine-list-duplicated":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"supported_engines": ["docker", "podman"]`, `"supported_engines": ["docker", "docker"]`); err != nil {
			t.Fatalf("mutate supported engines: %v", err)
		}
	case "unsupported-engine-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"unsupported_engine": "denied"`, `"unsupported_engine": "allowed"`); err != nil {
			t.Fatalf("mutate unsupported engine: %v", err)
		}
	case "engine-invoked":
		expected = "unsafe-plan"
		// The declared invocation count moved away from zero. This card publishes a
		// document; a plan that declares an engine call is a different plan.
		if err := replaceOnce410(root, profilePath410, `"engine_invocations": 0`, `"engine_invocations": 1`); err != nil {
			t.Fatalf("mutate engine invocations: %v", err)
		}
	case "engine-socket-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"engine_socket": "denied"`, `"engine_socket": "/var/run/docker.sock"`); err != nil {
			t.Fatalf("mutate engine socket: %v", err)
		}
	case "engine-api-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"engine_api": "denied"`, `"engine_api": "tcp://127.0.0.1:2375"`); err != nil {
			t.Fatalf("mutate engine api: %v", err)
		}
	case "host-engine-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"host_engine": "denied"`, `"host_engine": "allowed"`); err != nil {
			t.Fatalf("mutate host engine: %v", err)
		}
	case "host-checkout-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"host_checkout": "denied"`, `"host_checkout": "read-write"`); err != nil {
			t.Fatalf("mutate host checkout: %v", err)
		}
	case "host-filesystem-allowed":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"host_filesystem": "denied"`, `"host_filesystem": "read-only"`); err != nil {
			t.Fatalf("mutate host filesystem: %v", err)
		}
	case "subprocess-allowed":
		expected = "unsafe-plan"
		// A subprocess is how an engine CLI would be reached even without a declared
		// socket or API endpoint.
		if err := replaceOnce410(root, profilePath410, `"subprocess": "denied"`, `"subprocess": "allowed"`); err != nil {
			t.Fatalf("mutate subprocess: %v", err)
		}
	case "cgo-probe":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"probe_cgo": false`, `"probe_cgo": true`); err != nil {
			t.Fatalf("mutate probe linkage: %v", err)
		}
	case "persistent-report":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"report_mode": "ephemeral"`, `"report_mode": "persistent"`); err != nil {
			t.Fatalf("mutate report mode: %v", err)
		}
	case "orchestrator-root-host":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"orchestrator_root": "/tmp/aurum-oci-orchestrator"`, `"orchestrator_root": "/workspace"`); err != nil {
			t.Fatalf("mutate orchestrator root: %v", err)
		}
	case "probe-root-traversal":
		expected = "unsafe-plan"
		// A root that starts inside the bounded tmpfs and climbs out of it. The anchored
		// expression denies it for the same reason it denies an absolute host path.
		if err := replaceOnce410(root, profilePath410, `"probe_root": "/tmp/aurum-oci-probes"`, `"probe_root": "/tmp/aurum-oci-probes/../../etc"`); err != nil {
			t.Fatalf("mutate probe root: %v", err)
		}
	case "probe-root-trailing-slash":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"probe_root": "/tmp/aurum-oci-probes"`, `"probe_root": "/tmp/aurum-oci-probes/"`); err != nil {
			t.Fatalf("mutate probe root: %v", err)
		}
	case "probe-root-lookalike":
		expected = "unsafe-plan"
		// A sibling directory whose name merely starts with the bounded root.
		if err := replaceOnce410(root, profilePath410, `"probe_root": "/tmp/aurum-oci-probes"`, `"probe_root": "/tmp/aurum-oci-probes-host"`); err != nil {
			t.Fatalf("mutate probe root: %v", err)
		}
	case "probe-root-newline":
		expected = "unsafe-plan"
		// The admitted value followed by a newline and a second copy of itself. A
		// multiline anchor would accept this; \A and \z do not.
		if err := replaceOnce410(root, profilePath410, `"probe_root": "/tmp/aurum-oci-probes"`, `"probe_root": "/tmp/aurum-oci-probes\n/tmp/aurum-oci-probes"`); err != nil {
			t.Fatalf("mutate probe root: %v", err)
		}
	case "probe-root-trailing-newline":
		expected = "unsafe-plan"
		// The admitted value with a single trailing newline and nothing else.
		if err := replaceOnce410(root, profilePath410, `"probe_root": "/tmp/aurum-oci-probes"`, `"probe_root": "/tmp/aurum-oci-probes\n"`); err != nil {
			t.Fatalf("mutate probe root: %v", err)
		}
	case "probe-root-leading-space":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"probe_root": "/tmp/aurum-oci-probes"`, `"probe_root": " /tmp/aurum-oci-probes"`); err != nil {
			t.Fatalf("mutate probe root: %v", err)
		}
	case "probe-root-uppercase":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"probe_root": "/tmp/aurum-oci-probes"`, `"probe_root": "/TMP/AURUM-OCI-PROBES"`); err != nil {
			t.Fatalf("mutate probe root: %v", err)
		}
	case "probe-root-homoglyph":
		expected = "unsafe-plan"
		// The `o` of `probes` replaced by the Cyrillic small letter O (U+043E). The path
		// reads identically and is a different directory.
		if err := replaceOnce410(root, profilePath410, `"probe_root": "/tmp/aurum-oci-probes"`, "\"probe_root\": \"/tmp/aurum-oci-prоbes\""); err != nil {
			t.Fatalf("mutate probe root: %v", err)
		}
	case "probe-root-empty":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"probe_root": "/tmp/aurum-oci-probes"`, `"probe_root": ""`); err != nil {
			t.Fatalf("mutate probe root: %v", err)
		}
	case "report-root-escape":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"report_root": "/tmp/aurum-oci-reports"`, `"report_root": "/workspace/.board"`); err != nil {
			t.Fatalf("mutate report root: %v", err)
		}
	case "unbounded-report":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"max_report_bytes": 1048576`, `"max_report_bytes": 0`); err != nil {
			t.Fatalf("mutate report bound: %v", err)
		}
	case "oversized-report":
		expected = "unsafe-plan"
		// A ceiling far larger than the tmpfs that has to hold the rendered report.
		if err := replaceOnce410(root, profilePath410, `"max_report_bytes": 1048576`, `"max_report_bytes": 34359738368`); err != nil {
			t.Fatalf("mutate report bound: %v", err)
		}
	case "oversized-probe-set":
		expected = "unsafe-plan"
		// A probe corpus whose declared entries cannot fit the declared input bound.
		if err := replaceOnce410(root, profilePath410, `"max_probes": 1024`, `"max_probes": 2048`); err != nil {
			t.Fatalf("mutate probe bound: %v", err)
		}
	case "unbounded-probe-bytes":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"max_probe_bytes": 65536`, `"max_probe_bytes": 0`); err != nil {
			t.Fatalf("mutate probe bound: %v", err)
		}
	case "unbounded-probe-timeout":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"probe_timeout_seconds": 5`, `"probe_timeout_seconds": 0`); err != nil {
			t.Fatalf("mutate probe timeout: %v", err)
		}
	case "unlimited-tmpfs":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"tmpfs_mb": 128`, `"tmpfs_mb": 0`); err != nil {
			t.Fatalf("mutate tmpfs bound: %v", err)
		}
	case "engine-socket-environment":
		expected = "unsafe-plan"
		// The engine endpoint re-opened through the environment instead of the plan
		// field: DOCKER_HOST is all a client needs to reach the host daemon.
		if err := replaceOnce410(root, profilePath410, `"DOCKER_HOST": ""`, `"DOCKER_HOST": "unix:///var/run/docker.sock"`); err != nil {
			t.Fatalf("mutate docker host environment: %v", err)
		}
	case "container-host-environment":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"CONTAINER_HOST": ""`, `"CONTAINER_HOST": "unix:///run/podman/podman.sock"`); err != nil {
			t.Fatalf("mutate container host environment: %v", err)
		}
	case "engine-config-environment":
		expected = "unsafe-plan"
		// A client configuration directory is where a registry credential store and an
		// engine context would be read from.
		if err := replaceOnce410(root, profilePath410, `"DOCKER_CONFIG": "/dev/null"`, `"DOCKER_CONFIG": "/tmp/aurum-oci-probes/docker"`); err != nil {
			t.Fatalf("mutate docker config environment: %v", err)
		}
	case "proxy-environment":
		expected = "unsafe-plan"
		// The egress channel re-opened through the environment.
		if err := replaceOnce410(root, profilePath410, `"HTTPS_PROXY": ""`, `"HTTPS_PROXY": "http://127.0.0.1:3128"`); err != nil {
			t.Fatalf("mutate proxy environment: %v", err)
		}
	case "no-proxy-environment":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"NO_PROXY": "*"`, `"NO_PROXY": ""`); err != nil {
			t.Fatalf("mutate no proxy environment: %v", err)
		}
	case "cgo-environment":
		expected = "unsafe-plan"
		if err := replaceOnce410(root, profilePath410, `"CGO_ENABLED": "0"`, `"CGO_ENABLED": "1"`); err != nil {
			t.Fatalf("mutate cgo environment: %v", err)
		}
	case "unpinned-probe-image-set":
		expected = "digest-invalid"
		if err := replaceOnce410(root, profilePath410, `"probe_image_set": "digest-pinned"`, `"probe_image_set": "unpinned"`); err != nil {
			t.Fatalf("mutate probe image set: %v", err)
		}
	case "mutable-probe-image-set":
		expected = "digest-invalid"
		// The probe image set referenced by a mutable tag instead of content.
		if err := replaceOnce410(root, profilePath410, `"probe_image_set": "digest-pinned"`, `"probe_image_set": "latest"`); err != nil {
			t.Fatalf("mutate probe image set: %v", err)
		}
	case "missing-pids-limit":
		expected = "schema-invalid"
		if err := dropKey410(root, "pids_limit"); err != nil {
			t.Fatalf("mutate pids limit: %v", err)
		}
	case "missing-probe-timeout":
		expected = "schema-invalid"
		if err := dropKey410(root, "probe_timeout_seconds"); err != nil {
			t.Fatalf("mutate probe timeout: %v", err)
		}
	case "missing-supported-engines":
		expected = "schema-invalid"
		if err := dropKey410(root, "supported_engines"); err != nil {
			t.Fatalf("mutate supported engines: %v", err)
		}
	case "real-credential":
		expected = "credential-present"
		// Assembled here, never written in this source file, so the runner's input
		// gate never sees a credential-shaped literal in a materialized path.
		fake := "gh" + "p_" + strings.Repeat("A", 36)
		if !credentialShape410.MatchString(fake) {
			t.Fatal("mutation vector is not credential shaped")
		}
		if err := replaceOnce410(root, profilePath410, `"HTTP_PROXY": ""`, `"HTTP_PROXY": "`+fake+`"`); err != nil {
			t.Fatalf("mutate environment: %v", err)
		}
	default:
		t.Fatalf("unknown mutation: %s", mutation)
	}

	digest, code := ValidateRegistryAUR410(root)
	if mutation != "" {
		if code != expected {
			t.Fatalf("mutation %s got %s, want %s", mutation, code, expected)
		}
		if digest != "" {
			t.Fatalf("mutation %s returned a registry digest", mutation)
		}
		// A hostile probe observation may not rescue a rejected plan either. The same
		// invariance is required under the mutant as under the nominal document.
		assertProbeCannotMoveVerdict410(t, root, "", expected)
		fmt.Printf("mutation=%s code=%s\n", mutation, code)
		return
	}
	fmt.Printf("registry_code=%s\n", code)
	if code != "valid" || digest == "" {
		t.Fatalf("registry result=%s/%s", code, digest)
	}
	assertProbeCannotMoveVerdict410(t, root, digest, "valid")

	p, _, profileCode := ValidateProfileAUR410(root)
	if profileCode != "valid" {
		t.Fatalf("profile result=%s", profileCode)
	}
	// The engine allowlist is executed, not merely compared. Both supported engines
	// resolve; every unsupported name, and every impersonation of a supported one, does
	// not.
	for _, engine := range []string{"docker", "podman"} {
		if !EngineSupportedAUR410(p, engine) {
			t.Fatalf("supported engine %q is rejected", engine)
		}
	}
	for _, engine := range []string{"", "nerdctl", "ctr", "crictl", "runc", "buildah", "Docker", "DOCKER", " docker", "docker ", "docker\n", "docker\npodman", "docker;podman", "/usr/bin/docker", "podman-remote"} {
		if EngineSupportedAUR410(p, engine) {
			t.Fatalf("unsupported engine %q is admitted", engine)
		}
	}
	// The declared engine-invocation count and the measured one have to agree. The
	// ledger is written by shims the acceptance places ahead of PATH, so this is a
	// measurement of the whole run and not a restatement of the document.
	if measured, ok := measuredEngineInvocations410(t); ok {
		if measured != p.EngineInvocations {
			t.Fatalf("engine invocations declared %d, measured %d", p.EngineInvocations, measured)
		}
		fmt.Printf("engine_invocations_declared=%d engine_invocations_measured=%d\n", p.EngineInvocations, measured)
	}
	registryBytes := mustRead410(t, root, registryPath410)
	if strings.Contains(string(registryBytes), "latest") {
		t.Fatal("mutable tag in registry")
	}
	// No materialized document this card owns may carry a credential shape.
	for _, path := range []string{registryPath410, profilePath410, lockPath410, schemaPath410} {
		if credentialShape410.Match(mustRead410(t, root, path)) {
			t.Fatalf("credential shape in %s", path)
		}
	}
	fmt.Printf("registry_digest=%s\n", digest)
}

func mustRead410(t *testing.T, root, path string) []byte {
	t.Helper()
	b, err := read410(root, path)
	if err != nil {
		t.Fatalf("unreadable %s: %v", path, err)
	}
	return b
}
