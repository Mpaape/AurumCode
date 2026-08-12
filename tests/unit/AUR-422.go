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
	"strings"
	"testing"
)

// AUR-422 characterizes the already-materialized bootstrap-readonly-v1 profile:
// the loader below derives, from untrusted repository documents alone, the exact
// plan .board/bin/oci-run would execute, and returns a stable rejection code
// before any engine could be invoked. It never shells out and never pulls.

const (
	ProfileKeyAUR422      = "bootstrap-readonly-v1"
	ProfilePathAUR422     = ".board/oci/profiles/bootstrap-readonly-v1.json"
	LockPathAUR422        = ".board/locks/oci/bootstrap-readonly-v1.lock.json"
	SchemaPathAUR422      = ".board/schemas/container-profile.schema.json"
	RegistryPathAUR422    = ".board/oci/profiles/registry.v1.json"
	RequiredTmpfsAUR422   = "rw,nosuid,nodev"
	TmpfsDestinationAUR422 = "/tmp"
)

// Ceilings and floors characterized from .board/bin/oci-run; the integration
// lane re-extracts these from the runner source and refuses any drift.
const (
	CeilingTimeoutAUR422    = 900
	CeilingMemoryMBAUR422   = 4096
	CeilingCPUMillisAUR422  = 4000
	CeilingPidsAUR422       = 4096
	CeilingTmpfsMBAUR422    = 1024
	CeilingOutputAUR422     = 1048576
	CeilingInputFilesAUR422 = 100000
	CeilingInputBytesAUR422 = 268435456
	FloorTimeoutAUR422      = 1
	FloorMemoryMBAUR422     = 64
	FloorCPUMillisAUR422    = 100
	FloorPidsAUR422         = 16
	FloorTmpfsMBAUR422      = 8
	FloorOutputAUR422       = 4096
)

// Pattern literals identical to the runner's readonly variables.
const (
	ImagePatternAUR422 = `^[a-z0-9][a-z0-9._/-]*@sha256:[0-9a-f]{64}$`
	UserPatternAUR422  = `^[1-9][0-9]{2,6}:[1-9][0-9]{2,6}$`
)

var (
	imageDigestRe422 = regexp.MustCompile(ImagePatternAUR422)
	nonRootUserRe422 = regexp.MustCompile(UserPatternAUR422)
	shaDigestRe422   = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

type RegistryDocAUR422 struct {
	Schema   string                `json:"schema"`
	Version  *int                  `json:"version"`
	Profiles []RegistryEntryAUR422 `json:"profiles"`
}

type RegistryEntryAUR422 struct {
	Key            string `json:"key"`
	Schema         string `json:"schema"`
	SchemaDigest   string `json:"schema_digest"`
	Lock           string `json:"lock"`
	LockDigest     string `json:"lock_digest"`
	ImageSetDigest string `json:"image_set_digest"`
}

type LockDocAUR422 struct {
	Schema  string `json:"schema"`
	Version *int   `json:"version"`
	Profile string `json:"profile"`
	Image   string `json:"image"`
}

type ProfileDocAUR422 struct {
	Schema           string `json:"schema"`
	Version          *int   `json:"version"`
	Profile          string `json:"profile"`
	Lock             string `json:"lock"`
	LockDigest       string `json:"lock_digest"`
	Network          string `json:"network"`
	User             string `json:"user"`
	CapDrop          string `json:"cap_drop"`
	CapAdd           string `json:"cap_add"`
	Mounts           string `json:"mounts"`
	Devices          string `json:"devices"`
	Pull             string `json:"pull"`
	Tmpfs            string `json:"tmpfs"`
	ReadOnlyRootfs   *bool  `json:"read_only_rootfs"`
	NoNewPrivileges  *bool  `json:"no_new_privileges"`
	Privileged       *bool  `json:"privileged"`
	TimeoutSeconds   *int   `json:"timeout_seconds"`
	MemoryMB         *int   `json:"memory_mb"`
	CPUMillis        *int   `json:"cpu_millis"`
	PidsLimit        *int   `json:"pids_limit"`
	TmpfsMB          *int   `json:"tmpfs_mb"`
	StdoutLimitBytes *int   `json:"stdout_limit_bytes"`
	StderrLimitBytes *int   `json:"stderr_limit_bytes"`
	MaxInputFiles    *int   `json:"max_input_files"`
	MaxInputBytes    *int   `json:"max_input_bytes"`
}

// PlanAUR422 is the derived execution plan: exactly the flags oci-run passes to
// the engine, plus the digests binding the plan to its source documents.
// EngineInvocations is part of the contract: deriving a plan is always zero.
type PlanAUR422 struct {
	Image             string
	User              string
	Network           string
	CapDrop           string
	CapAdd            string
	ReadOnlyRootfs    bool
	NoNewPrivileges   bool
	Tmpfs             string
	Memory            string
	CPUs              string
	MemoryMB          int
	CPUMillis         int
	PidsLimit         int
	TimeoutSeconds    int
	TmpfsMB           int
	StdoutLimitBytes  int
	StderrLimitBytes  int
	MaxInputFiles     int
	MaxInputBytes     int
	ProfileDigest     string
	LockDigest        string
	EngineInvocations int
}

func sha422(b []byte) string {
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}

func decodeStrict422(data []byte, value any) bool {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(value) != nil {
		return false
	}
	var trailing any
	return d.Decode(&trailing) == io.EOF
}

func bounded422(value *int, floor, ceiling int) bool {
	return value != nil && *value >= floor && *value <= ceiling
}

// ResolveBootstrapPlanAUR422 resolves the bootstrap-readonly-v1 key against the
// registry document and derives the plan oci-run would execute. Validation order
// deliberately mirrors resolve_profile in .board/bin/oci-run: the lock is
// validated before the profile-to-lock digest binding, so a lock whose image
// reference became a mutable tag reports mutable-image, not lock-mismatch.
// Every failure returns a stable code with an empty plan; nothing here can
// reach the runner's die 78 and nothing here invokes an engine.
func ResolveBootstrapPlanAUR422(registry, schema, profileDoc, lockDoc []byte) (PlanAUR422, string) {
	var none PlanAUR422

	// Registry resolution: the key must be registered exactly once and its
	// entry must bind the canonical container-profile schema by content digest.
	var reg RegistryDocAUR422
	if !decodeStrict422(registry, &reg) {
		return none, "schema-invalid"
	}
	if reg.Schema != "aurum.profile-registry" || reg.Version == nil || *reg.Version != 1 {
		return none, "schema-invalid"
	}
	matches := 0
	var entry RegistryEntryAUR422
	for _, e := range reg.Profiles {
		if e.Key == ProfileKeyAUR422 {
			matches++
			entry = e
		}
	}
	if matches == 0 {
		return none, "profile-unregistered"
	}
	if matches > 1 {
		return none, "duplicate-profile"
	}
	if entry.Schema != SchemaPathAUR422 || strings.Contains(entry.Lock, "..") ||
		!strings.HasPrefix(entry.Lock, ".board/locks/oci/") || !strings.HasSuffix(entry.Lock, ".lock.json") {
		return none, "unsafe-plan"
	}
	if !shaDigestRe422.MatchString(entry.SchemaDigest) || !shaDigestRe422.MatchString(entry.LockDigest) ||
		!shaDigestRe422.MatchString(entry.ImageSetDigest) {
		return none, "digest-invalid"
	}
	if sha422(schema) != entry.SchemaDigest {
		return none, "schema-digest-mismatch"
	}

	// Lock document: validated first, exactly like the runner.
	var lock LockDocAUR422
	if !decodeStrict422(lockDoc, &lock) {
		return none, "schema-invalid"
	}
	if lock.Schema != "aurum.oci-image-lock" || lock.Version == nil || *lock.Version != 1 ||
		lock.Profile != ProfileKeyAUR422 {
		return none, "schema-invalid"
	}
	if !imageDigestRe422.MatchString(lock.Image) {
		return none, "mutable-image"
	}

	// Profile document identity and lock binding.
	var p ProfileDocAUR422
	if !decodeStrict422(profileDoc, &p) {
		return none, "schema-invalid"
	}
	if p.Schema != "aurum.container-profile" || p.Version == nil || *p.Version != 1 ||
		p.Profile != ProfileKeyAUR422 {
		return none, "schema-invalid"
	}
	if p.Lock != LockPathAUR422 {
		return none, "schema-invalid"
	}
	if !shaDigestRe422.MatchString(p.LockDigest) || p.LockDigest != sha422(lockDoc) {
		return none, "lock-mismatch"
	}

	// Hardening literals: any relaxation is a privileged plan and is refused
	// before the runner could ever assemble an engine command line.
	if p.Network != "none" || p.CapDrop != "ALL" || p.CapAdd != "none" ||
		p.Mounts != "none" || p.Devices != "none" || p.Pull != "never" {
		return none, "privileged-plan"
	}
	if p.ReadOnlyRootfs == nil || !*p.ReadOnlyRootfs ||
		p.NoNewPrivileges == nil || !*p.NoNewPrivileges ||
		p.Privileged == nil || *p.Privileged {
		return none, "privileged-plan"
	}
	if !nonRootUserRe422.MatchString(p.User) {
		return none, "privileged-plan"
	}
	if p.Tmpfs != RequiredTmpfsAUR422 {
		return none, "privileged-plan"
	}

	// Resource ceilings: an absent or unbounded limit is refused.
	if !bounded422(p.TimeoutSeconds, FloorTimeoutAUR422, CeilingTimeoutAUR422) ||
		!bounded422(p.MemoryMB, FloorMemoryMBAUR422, CeilingMemoryMBAUR422) ||
		!bounded422(p.CPUMillis, FloorCPUMillisAUR422, CeilingCPUMillisAUR422) ||
		!bounded422(p.PidsLimit, FloorPidsAUR422, CeilingPidsAUR422) ||
		!bounded422(p.TmpfsMB, FloorTmpfsMBAUR422, CeilingTmpfsMBAUR422) ||
		!bounded422(p.StdoutLimitBytes, FloorOutputAUR422, CeilingOutputAUR422) ||
		!bounded422(p.StderrLimitBytes, FloorOutputAUR422, CeilingOutputAUR422) ||
		!bounded422(p.MaxInputFiles, 1, CeilingInputFilesAUR422) ||
		!bounded422(p.MaxInputBytes, 1, CeilingInputBytesAUR422) {
		return none, "unbounded-limit"
	}

	return PlanAUR422{
		Image:             lock.Image,
		User:              p.User,
		Network:           p.Network,
		CapDrop:           p.CapDrop,
		CapAdd:            p.CapAdd,
		ReadOnlyRootfs:    *p.ReadOnlyRootfs,
		NoNewPrivileges:   *p.NoNewPrivileges,
		Tmpfs:             fmt.Sprintf("%s:exec,%s,size=%dm", TmpfsDestinationAUR422, RequiredTmpfsAUR422, *p.TmpfsMB),
		Memory:            fmt.Sprintf("%dm", *p.MemoryMB),
		CPUs:              fmt.Sprintf("%d.%03d", *p.CPUMillis/1000, *p.CPUMillis%1000),
		MemoryMB:          *p.MemoryMB,
		CPUMillis:         *p.CPUMillis,
		PidsLimit:         *p.PidsLimit,
		TimeoutSeconds:    *p.TimeoutSeconds,
		TmpfsMB:           *p.TmpfsMB,
		StdoutLimitBytes:  *p.StdoutLimitBytes,
		StderrLimitBytes:  *p.StderrLimitBytes,
		MaxInputFiles:     *p.MaxInputFiles,
		MaxInputBytes:     *p.MaxInputBytes,
		ProfileDigest:     sha422(profileDoc),
		LockDigest:        p.LockDigest,
		EngineInvocations: 0,
	}, "valid"
}

// mutantLabelAUR422 maps a stable rejection code to the declared skeptical
// mutation it falsifies, so a corrupted checked-in document makes the nominal
// acceptance fail with the exact card code.
func mutantLabelAUR422(code string) string {
	switch code {
	case "lock-mismatch":
		return "MUT-001/" + code
	case "mutable-image":
		return "MUT-002/" + code
	case "privileged-plan":
		return "MUT-003/" + code
	default:
		return code
	}
}

func mutateDocAUR422(t *testing.T, doc []byte, key string, value any) []byte {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(doc, &m); err != nil {
		t.Fatalf("mutation base document invalid: %v", err)
	}
	m[key] = value
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("mutation marshal failed: %v", err)
	}
	return out
}

func dropRegistryKeyAUR422(t *testing.T, registry []byte) []byte {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(registry, &m); err != nil {
		t.Fatalf("registry document invalid: %v", err)
	}
	profiles, ok := m["profiles"].([]any)
	if !ok {
		t.Fatal("registry profiles array missing")
	}
	kept := make([]any, 0, len(profiles))
	for _, raw := range profiles {
		entry, ok := raw.(map[string]any)
		if ok && entry["key"] == ProfileKeyAUR422 {
			continue
		}
		kept = append(kept, raw)
	}
	m["profiles"] = kept
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("registry mutation marshal failed: %v", err)
	}
	return out
}

func expectRejectionAUR422(t *testing.T, label, want string, registry, schema, profileDoc, lockDoc []byte) {
	t.Helper()
	plan, code := ResolveBootstrapPlanAUR422(registry, schema, profileDoc, lockDoc)
	if code != want {
		t.Fatalf("mutant %s expected code %s, got %s", label, want, code)
	}
	if plan != (PlanAUR422{}) {
		t.Fatalf("mutant %s leaked a derived plan", label)
	}
	fmt.Printf("mutation=%s code=%s engine_invocations=0\n", label, code)
}

func readInputAUR422(t *testing.T, root, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatalf("required input unreadable: %s: %v", path, err)
	}
	if len(b) == 0 {
		t.Fatalf("required input empty: %s", path)
	}
	return b
}

func altDigestAUR422(seed string) string {
	h := sha256.Sum256([]byte(seed))
	return "sha256:" + hex.EncodeToString(h[:])
}

func TestAUR422(t *testing.T) {
	root := os.Getenv("AURUMCODE_ROOT")
	if root == "" {
		t.Fatal("AURUMCODE_ROOT is required")
	}
	registry := readInputAUR422(t, root, RegistryPathAUR422)
	schema := readInputAUR422(t, root, SchemaPathAUR422)
	profileDoc := readInputAUR422(t, root, ProfilePathAUR422)
	lockDoc := readInputAUR422(t, root, LockPathAUR422)

	switch mutation := os.Getenv("AURUM_A422_MUTATION"); mutation {
	case "":
		// nominal below
	case "MUT-001":
		// Only the declared lock_digest changes; the lock itself does not.
		mutated := mutateDocAUR422(t, profileDoc, "lock_digest", altDigestAUR422("AUR-422/AC-001/MUT-001"))
		expectRejectionAUR422(t, "MUT-001", "lock-mismatch", registry, schema, mutated, lockDoc)
		return
	case "MUT-002":
		// The digest-pinned image reference becomes a mutable tag of the same
		// repository; the loader refuses before anything could pull.
		mutated := mutateDocAUR422(t, lockDoc, "image", "aurum-bootstrap-go-bash:latest")
		expectRejectionAUR422(t, "MUT-002", "mutable-image", registry, schema, profileDoc, mutated)
		return
	case "MUT-003":
		expectRejectionAUR422(t, "MUT-003", "privileged-plan", registry, schema,
			mutateDocAUR422(t, profileDoc, "network", "bridge"), lockDoc)
		expectRejectionAUR422(t, "MUT-003", "privileged-plan", registry, schema,
			mutateDocAUR422(t, profileDoc, "mounts", "/var/run/docker.sock"), lockDoc)
		expectRejectionAUR422(t, "MUT-003", "privileged-plan", registry, schema,
			mutateDocAUR422(t, profileDoc, "user", "0:0"), lockDoc)
		return
	default:
		t.Fatalf("unknown mutation selector: %s", mutation)
	}

	plan, code := ResolveBootstrapPlanAUR422(registry, schema, profileDoc, lockDoc)
	if code != "valid" {
		t.Fatalf("AUR-422/AC-001/%s", mutantLabelAUR422(code))
	}

	var lock LockDocAUR422
	if !decodeStrict422(lockDoc, &lock) {
		t.Fatal("lock document unreadable after resolution")
	}
	if plan.Image != lock.Image || !imageDigestRe422.MatchString(plan.Image) {
		t.Fatalf("plan image is not the lock's digest-pinned image: %s", plan.Image)
	}
	if plan.Network != "none" {
		t.Fatalf("plan does not deny network: %s", plan.Network)
	}
	if !plan.ReadOnlyRootfs {
		t.Fatal("plan does not enforce a read-only rootfs")
	}
	if plan.CapDrop != "ALL" || plan.CapAdd != "none" {
		t.Fatalf("plan does not drop all capabilities: drop=%s add=%s", plan.CapDrop, plan.CapAdd)
	}
	if !plan.NoNewPrivileges {
		t.Fatal("plan does not set no-new-privileges")
	}
	uidGid := strings.SplitN(plan.User, ":", 2)
	if len(uidGid) != 2 || uidGid[0] == "0" || uidGid[1] == "0" {
		t.Fatalf("plan user is not a non-root uid:gid pair: %s", plan.User)
	}
	if plan.User != "65534:65534" {
		t.Fatalf("plan user diverges from the materialized profile: %s", plan.User)
	}
	if plan.Tmpfs != fmt.Sprintf("/tmp:exec,%s,size=%dm", RequiredTmpfsAUR422, plan.TmpfsMB) {
		t.Fatalf("plan tmpfs diverges from the runner derivation: %s", plan.Tmpfs)
	}
	if plan.Memory != "256m" || plan.MemoryMB != 256 {
		t.Fatalf("plan memory ceiling diverges: %s/%d", plan.Memory, plan.MemoryMB)
	}
	if plan.CPUs != "1.000" || plan.CPUMillis != 1000 {
		t.Fatalf("plan cpu ceiling diverges: %s/%d", plan.CPUs, plan.CPUMillis)
	}
	if plan.TimeoutSeconds != 120 {
		t.Fatalf("plan wall-clock ceiling diverges: %d", plan.TimeoutSeconds)
	}
	if plan.PidsLimit != 128 {
		t.Fatalf("plan pids ceiling diverges: %d", plan.PidsLimit)
	}
	if plan.StdoutLimitBytes != 65536 || plan.StderrLimitBytes != 65536 {
		t.Fatalf("plan output ceilings diverge: %d/%d", plan.StdoutLimitBytes, plan.StderrLimitBytes)
	}
	if plan.TmpfsMB != 512 {
		t.Fatalf("plan tmpfs ceiling diverges: %d", plan.TmpfsMB)
	}
	if plan.MaxInputFiles != 10000 || plan.MaxInputBytes != 67108864 {
		t.Fatalf("plan input ceilings diverge: %d/%d", plan.MaxInputFiles, plan.MaxInputBytes)
	}
	if plan.ProfileDigest != sha422(profileDoc) || plan.LockDigest != sha422(lockDoc) {
		t.Fatal("plan digests do not bind the source documents")
	}
	if plan.EngineInvocations != 0 {
		t.Fatalf("plan derivation invoked an engine %d time(s)", plan.EngineInvocations)
	}

	// Embedded skeptical fixtures: every declared mutant plus the Given's
	// writable-rootfs and unbounded-stdout mutants must be refused with a
	// stable code, without reaching die 78 and without any engine call.
	expectRejectionAUR422(t, "MUT-001", "lock-mismatch", registry, schema,
		mutateDocAUR422(t, profileDoc, "lock_digest", altDigestAUR422("AUR-422/AC-001/MUT-001")), lockDoc)
	expectRejectionAUR422(t, "MUT-002", "mutable-image", registry, schema,
		profileDoc, mutateDocAUR422(t, lockDoc, "image", "aurum-bootstrap-go-bash:latest"))
	expectRejectionAUR422(t, "MUT-003", "privileged-plan", registry, schema,
		mutateDocAUR422(t, profileDoc, "network", "bridge"), lockDoc)
	expectRejectionAUR422(t, "MUT-003", "privileged-plan", registry, schema,
		mutateDocAUR422(t, profileDoc, "mounts", "/var/run/docker.sock"), lockDoc)
	expectRejectionAUR422(t, "MUT-003", "privileged-plan", registry, schema,
		mutateDocAUR422(t, profileDoc, "user", "0:0"), lockDoc)
	expectRejectionAUR422(t, "writable-rootfs", "privileged-plan", registry, schema,
		mutateDocAUR422(t, profileDoc, "read_only_rootfs", false), lockDoc)
	expectRejectionAUR422(t, "privileged-flag", "privileged-plan", registry, schema,
		mutateDocAUR422(t, profileDoc, "privileged", true), lockDoc)
	expectRejectionAUR422(t, "cap-add", "privileged-plan", registry, schema,
		mutateDocAUR422(t, profileDoc, "cap_add", "SYS_ADMIN"), lockDoc)
	expectRejectionAUR422(t, "unbounded-stdout", "unbounded-limit", registry, schema,
		mutateDocAUR422(t, profileDoc, "stdout_limit_bytes", 0), lockDoc)
	expectRejectionAUR422(t, "unregistered-key", "profile-unregistered",
		dropRegistryKeyAUR422(t, registry), schema, profileDoc, lockDoc)
	expectRejectionAUR422(t, "foreign-lock", "schema-invalid", registry, schema,
		mutateDocAUR422(t, profileDoc, "lock", ".board/locks/oci/registry-v1.lock.json"), lockDoc)

	fmt.Printf("plan image=%s user=%s network=%s tmpfs=%s timeout_s=%d memory_mb=%d cpu_millis=%d pids=%d stdout_b=%d stderr_b=%d profile_digest=%s lock_digest=%s engine_invocations=0\n",
		plan.Image, plan.User, plan.Network, plan.Tmpfs, plan.TimeoutSeconds, plan.MemoryMB,
		plan.CPUMillis, plan.PidsLimit, plan.StdoutLimitBytes, plan.StderrLimitBytes,
		plan.ProfileDigest, plan.LockDigest)
}
