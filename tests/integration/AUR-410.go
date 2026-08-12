package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type registryEntryAUR410 struct {
	Key            string `json:"key"`
	Schema         string `json:"schema"`
	SchemaDigest   string `json:"schema_digest"`
	Lock           string `json:"lock"`
	LockDigest     string `json:"lock_digest"`
	ImageSetDigest string `json:"image_set_digest"`
}

func digestAUR410Bytes(b []byte) string {
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}

// IntegrationAUR410 crosses the registry entry with the bytes it claims to bind: schema
// digest, lock digest and image-set digest must all be re-derivable from the documents on
// disk, and the profile document must point at the same lock the registry points at. It
// then crosses the two halves of the orchestrator/probe separation that live in different
// parts of the document — the declared roots, the environment spelling of those roots and
// the probe's writable set — because a plan whose halves disagree has a boundary only on
// paper.
func IntegrationAUR410(t *testing.T) {
	root := os.Getenv("AURUMCODE_ROOT")
	if root == "" {
		t.Fatal("AURUMCODE_ROOT is required")
	}
	read := func(path string) []byte {
		b, err := os.ReadFile(filepath.Join(root, path))
		if err != nil || len(b) == 0 {
			t.Fatalf("unreadable %s", path)
		}
		return b
	}

	registryBytes := read(".board/oci/profiles/registry.v1.json")
	var registry struct {
		Profiles []registryEntryAUR410 `json:"profiles"`
	}
	if err := json.Unmarshal(registryBytes, &registry); err != nil {
		t.Fatalf("registry is not decodable: %v", err)
	}

	var entry *registryEntryAUR410
	for i := range registry.Profiles {
		if registry.Profiles[i].Key == "oci-conformance-v1" {
			entry = &registry.Profiles[i]
		}
	}
	if entry == nil {
		t.Fatal("registry has no oci-conformance-v1 entry")
	}

	schemaBytes := read(entry.Schema)
	if got := digestAUR410Bytes(schemaBytes); got != entry.SchemaDigest {
		t.Fatalf("schema digest mismatch: registry %s, bytes %s", entry.SchemaDigest, got)
	}
	lockBytes := read(entry.Lock)
	if got := digestAUR410Bytes(lockBytes); got != entry.LockDigest {
		t.Fatalf("lock digest mismatch: registry %s, bytes %s", entry.LockDigest, got)
	}

	var lock struct {
		Profile string `json:"profile"`
		Image   string `json:"image"`
	}
	if err := json.Unmarshal(lockBytes, &lock); err != nil {
		t.Fatalf("lock is not decodable: %v", err)
	}
	if lock.Profile != "oci-conformance-v1" {
		t.Fatalf("lock binds to %s", lock.Profile)
	}
	if got := digestAUR410Bytes([]byte(lock.Image)); got != entry.ImageSetDigest {
		t.Fatalf("image set digest mismatch: registry %s, image %s", entry.ImageSetDigest, got)
	}

	profileBytes := read(".board/oci/profiles/oci-conformance-v1.json")
	var profile struct {
		Lock                string            `json:"lock"`
		LockDigest          string            `json:"lock_digest"`
		TmpfsMB             int               `json:"tmpfs_mb"`
		TimeoutSeconds      int               `json:"timeout_seconds"`
		MaxInputFiles       int               `json:"max_input_files"`
		MaxInputBytes       int               `json:"max_input_bytes"`
		Orchestrator        string            `json:"orchestrator"`
		OrchestratorRoot    string            `json:"orchestrator_root"`
		Probe               string            `json:"probe"`
		ProbeRoot           string            `json:"probe_root"`
		ProbeOutput         string            `json:"probe_output"`
		ProbeVerdict        string            `json:"probe_verdict_authority"`
		ProbeWritableRoots  []string          `json:"probe_writable_roots"`
		VerdictAuthority    string            `json:"verdict_authority"`
		ReportRoot          string            `json:"report_root"`
		SupportedEngines    []string          `json:"supported_engines"`
		UnsupportedEngine   string            `json:"unsupported_engine"`
		EngineInvocations   int               `json:"engine_invocations"`
		MaxProbes           int               `json:"max_probes"`
		MaxProbeBytes       int               `json:"max_probe_bytes"`
		MaxReportBytes      int               `json:"max_report_bytes"`
		ProbeTimeoutSeconds int               `json:"probe_timeout_seconds"`
		Environment         map[string]string `json:"environment"`
	}
	if err := json.Unmarshal(profileBytes, &profile); err != nil {
		t.Fatalf("profile is not decodable: %v", err)
	}
	if profile.Lock != entry.Lock {
		t.Fatalf("profile points at %s while the registry points at %s", profile.Lock, entry.Lock)
	}
	if profile.LockDigest != entry.LockDigest {
		t.Fatalf("profile lock digest %s is not the registry lock digest %s", profile.LockDigest, entry.LockDigest)
	}
	// The environment roots and the declared roots are two spellings of the same three
	// directories. If they ever disagree, one of them reaches a filesystem the plan never
	// bounded, and the probe root is exactly where untrusted output would land.
	for _, pair := range []struct {
		variable string
		declared string
	}{
		{"AURUM_OCI_ORCHESTRATOR_ROOT", profile.OrchestratorRoot},
		{"AURUM_OCI_PROBE_ROOT", profile.ProbeRoot},
		{"AURUM_OCI_REPORT_ROOT", profile.ReportRoot},
	} {
		if profile.Environment[pair.variable] != pair.declared {
			t.Fatalf("%s %q is not the declared root %q", pair.variable, profile.Environment[pair.variable], pair.declared)
		}
	}
	// The three bounded roots must be distinct directories, and none of them may contain
	// another: a probe root under the orchestrator root, or a report root under the probe
	// root, would put the untrusted side inside the trusted side's tree.
	for _, pair := range [][2]string{
		{profile.OrchestratorRoot, profile.ProbeRoot},
		{profile.OrchestratorRoot, profile.ReportRoot},
		{profile.ProbeRoot, profile.ReportRoot},
	} {
		if pair[0] == pair[1] ||
			strings.HasPrefix(pair[0], pair[1]+"/") || strings.HasPrefix(pair[1], pair[0]+"/") {
			t.Fatalf("profile declares overlapping roots: orchestrator=%q probes=%q reports=%q",
				profile.OrchestratorRoot, profile.ProbeRoot, profile.ReportRoot)
		}
	}
	// The probe writes inside its own root and nowhere the orchestrator reads. A writable
	// root that is, or contains, the orchestrator root or the report root would let the
	// untrusted side write its own verdict.
	if len(profile.ProbeWritableRoots) == 0 {
		t.Fatal("profile publishes no writable root for the probe")
	}
	probeRootWritable := false
	for _, writable := range profile.ProbeWritableRoots {
		if writable == "" {
			t.Fatal("profile publishes an empty writable root")
		}
		if writable == profile.ProbeRoot {
			probeRootWritable = true
		}
		for _, guarded := range []string{profile.OrchestratorRoot, profile.ReportRoot} {
			if guarded == writable || strings.HasPrefix(guarded, writable+"/") {
				t.Fatalf("writable root %q reaches the orchestrator-owned root %q", writable, guarded)
			}
		}
	}
	if !probeRootWritable {
		t.Fatalf("the probe root %q is not in the probe's writable set %v", profile.ProbeRoot, profile.ProbeWritableRoots)
	}
	// The boundary has to exist and the verdict has to sit on the trusted side of it.
	if profile.Orchestrator == profile.Probe {
		t.Fatalf("orchestrator and probe carry the same trust label: %q", profile.Probe)
	}
	if profile.VerdictAuthority != "orchestrator" || profile.ProbeVerdict != "denied" ||
		profile.ProbeOutput != "untrusted-observation" {
		t.Fatalf("the probe can reach the verdict: authority=%q probe_authority=%q output=%q",
			profile.VerdictAuthority, profile.ProbeVerdict, profile.ProbeOutput)
	}
	// The engine allowlist is closed, non-empty and free of repetition, and an engine
	// outside it is denied rather than tolerated.
	if len(profile.SupportedEngines) == 0 {
		t.Fatal("profile publishes no supported engine")
	}
	if profile.UnsupportedEngine != "denied" {
		t.Fatalf("profile tolerates an unsupported engine: %q", profile.UnsupportedEngine)
	}
	named := map[string]bool{}
	for _, engine := range profile.SupportedEngines {
		if engine != "docker" && engine != "podman" {
			t.Fatalf("profile names an unsupported engine: %q", engine)
		}
		if named[engine] {
			t.Fatalf("profile names %q twice", engine)
		}
		named[engine] = true
	}
	// This card publishes a document; the plan declares that no engine is reached at all.
	if profile.EngineInvocations != 0 {
		t.Fatalf("profile declares %d engine invocations", profile.EngineInvocations)
	}
	// The rendered report has to fit inside the tmpfs that holds it; otherwise the plan
	// would have to spill outside its own bound.
	if profile.MaxReportBytes <= 0 || profile.TmpfsMB <= 0 {
		t.Fatalf("profile declares an unbounded report: max_report_bytes=%d tmpfs_mb=%d",
			profile.MaxReportBytes, profile.TmpfsMB)
	}
	if profile.MaxReportBytes > profile.TmpfsMB*1024*1024 {
		t.Fatalf("max_report_bytes %d exceeds the %d MiB bounded tmpfs",
			profile.MaxReportBytes, profile.TmpfsMB)
	}
	// And the probe corpus has to fit inside the declared input bound, because every
	// probe is a materialized input.
	if profile.MaxProbes <= 0 || profile.MaxProbeBytes <= 0 {
		t.Fatalf("profile declares an unbounded probe set: max_probes=%d max_probe_bytes=%d",
			profile.MaxProbes, profile.MaxProbeBytes)
	}
	if profile.MaxProbes > profile.MaxInputFiles {
		t.Fatalf("a probe set of %d entries exceeds the %d file input bound",
			profile.MaxProbes, profile.MaxInputFiles)
	}
	if profile.MaxProbes*profile.MaxProbeBytes > profile.MaxInputBytes {
		t.Fatalf("a probe set of %d probes of %d bytes exceeds the %d byte input bound",
			profile.MaxProbes, profile.MaxProbeBytes, profile.MaxInputBytes)
	}
	// A per-probe deadline that outlived the run deadline would not bound anything.
	if profile.ProbeTimeoutSeconds <= 0 || profile.ProbeTimeoutSeconds > profile.TimeoutSeconds {
		t.Fatalf("probe_timeout_seconds %d is not bounded by timeout_seconds %d",
			profile.ProbeTimeoutSeconds, profile.TimeoutSeconds)
	}
}
