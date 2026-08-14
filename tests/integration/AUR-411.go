package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type registryEntryAUR411 struct {
	Key            string `json:"key"`
	Schema         string `json:"schema"`
	SchemaDigest   string `json:"schema_digest"`
	Lock           string `json:"lock"`
	LockDigest     string `json:"lock_digest"`
	ImageSetDigest string `json:"image_set_digest"`
}

type partitionAUR411 struct {
	Runtime   string `json:"runtime"`
	Toolchain string `json:"toolchain"`
	CacheRoot string `json:"cache_root"`
	CacheMode string `json:"cache_mode"`
	Telemetry string `json:"telemetry"`
}

func digestAUR411Bytes(b []byte) string {
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}

// IntegrationAUR411 crosses the registry entry with the bytes it claims to bind: schema
// digest, lock digest and image-set digest must all be re-derivable from the documents on
// disk, and the profile document must point at the same lock the registry points at. It
// then crosses the two halves of the polyglot plan that live in different parts of the
// document — the declared partitions, the published language list and the environment
// spelling of the bounded cache directories — because a plan whose halves disagree has
// isolation only on paper.
func IntegrationAUR411(t *testing.T) {
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
		Profiles []registryEntryAUR411 `json:"profiles"`
	}
	if err := json.Unmarshal(registryBytes, &registry); err != nil {
		t.Fatalf("registry is not decodable: %v", err)
	}

	var entry *registryEntryAUR411
	for i := range registry.Profiles {
		if registry.Profiles[i].Key == "polyglot-toolchain-v1" {
			entry = &registry.Profiles[i]
		}
	}
	if entry == nil {
		t.Fatal("registry has no polyglot-toolchain-v1 entry")
	}

	schemaBytes := read(entry.Schema)
	if got := digestAUR411Bytes(schemaBytes); got != entry.SchemaDigest {
		t.Fatalf("schema digest mismatch: registry %s, bytes %s", entry.SchemaDigest, got)
	}
	lockBytes := read(entry.Lock)
	if got := digestAUR411Bytes(lockBytes); got != entry.LockDigest {
		t.Fatalf("lock digest mismatch: registry %s, bytes %s", entry.LockDigest, got)
	}

	var lock struct {
		Profile string `json:"profile"`
		Image   string `json:"image"`
	}
	if err := json.Unmarshal(lockBytes, &lock); err != nil {
		t.Fatalf("lock is not decodable: %v", err)
	}
	if lock.Profile != "polyglot-toolchain-v1" {
		t.Fatalf("lock binds to %s", lock.Profile)
	}
	if got := digestAUR411Bytes([]byte(lock.Image)); got != entry.ImageSetDigest {
		t.Fatalf("image set digest mismatch: registry %s, image %s", entry.ImageSetDigest, got)
	}

	profileBytes := read(".board/oci/profiles/polyglot-toolchain-v1.json")
	var profile struct {
		Lock                     string                     `json:"lock"`
		LockDigest               string                     `json:"lock_digest"`
		TmpfsMB                  int                        `json:"tmpfs_mb"`
		TimeoutSeconds           int                        `json:"timeout_seconds"`
		MaxInputFiles            int                        `json:"max_input_files"`
		MaxInputBytes            int                        `json:"max_input_bytes"`
		PolyglotRoot             string                     `json:"polyglot_root"`
		ToolchainSource          string                     `json:"toolchain_source"`
		ToolchainMaterialization string                     `json:"toolchain_materialization"`
		ToolchainInstall         string                     `json:"toolchain_install"`
		ToolchainBootstrap       string                     `json:"toolchain_bootstrap"`
		PackageInstall           string                     `json:"package_install"`
		PackageRegistry          string                     `json:"package_registry"`
		PackageInstalls          int                        `json:"package_installs"`
		CacheMode                string                     `json:"cache_mode"`
		Telemetry                string                     `json:"telemetry"`
		Egress                   string                     `json:"egress"`
		HostToolchain            string                     `json:"host_toolchain"`
		HostCache                string                     `json:"host_cache"`
		Subprocess               string                     `json:"subprocess"`
		UnregisteredLanguage     string                     `json:"unregistered_language"`
		Languages                []string                   `json:"languages"`
		Toolchains               map[string]partitionAUR411 `json:"toolchains"`
		MaxLanguages             int                        `json:"max_languages"`
		MaxPackages              int                        `json:"max_packages"`
		MaxPackageBytes          int                        `json:"max_package_bytes"`
		MaxCacheBytes            int                        `json:"max_cache_bytes"`
		ResolveTimeoutSeconds    int                        `json:"resolve_timeout_seconds"`
		Environment              map[string]string          `json:"environment"`
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
	// The published list and the partition object are two spellings of the same set. A
	// language listed with no partition is a partition nobody validates; a partition
	// nobody lists is a plan reachable through a name the list never admits.
	if len(profile.Languages) == 0 {
		t.Fatal("profile publishes no language partition")
	}
	if len(profile.Languages) != len(profile.Toolchains) {
		t.Fatalf("profile lists %d languages and publishes %d partitions",
			len(profile.Languages), len(profile.Toolchains))
	}
	if !sort.StringsAreSorted(profile.Languages) {
		t.Fatalf("profile publishes an unsorted language list: %v", profile.Languages)
	}
	if len(profile.Languages) > profile.MaxLanguages {
		t.Fatalf("profile lists %d languages above its own ceiling of %d",
			len(profile.Languages), profile.MaxLanguages)
	}
	listed := map[string]bool{}
	for _, language := range profile.Languages {
		if listed[language] {
			t.Fatalf("profile lists %q twice", language)
		}
		listed[language] = true
		if _, ok := profile.Toolchains[language]; !ok {
			t.Fatalf("language %q has no partition", language)
		}
	}
	for language := range profile.Toolchains {
		if !listed[language] {
			t.Fatalf("partition %q is not listed in languages", language)
		}
	}
	// Each partition is isolated: its own bounded cache directory under the polyglot root,
	// its own runtime, read-only, telemetry denied and content-addressed. Two partitions
	// that shared a cache root or a runtime would be one partition wearing two names.
	roots := map[string]bool{}
	runtimes := map[string]bool{}
	for _, language := range profile.Languages {
		partition := profile.Toolchains[language]
		if partition.Runtime == "" || partition.CacheRoot == "" {
			t.Fatalf("partition %q is incomplete: %+v", language, partition)
		}
		if partition.Toolchain != "digest-pinned" {
			t.Fatalf("partition %q is not content-addressed: %q", language, partition.Toolchain)
		}
		if partition.CacheMode != profile.CacheMode || partition.CacheMode != "read-only" {
			t.Fatalf("partition %q publishes a writable cache: %q", language, partition.CacheMode)
		}
		if partition.Telemetry != profile.Telemetry || partition.Telemetry != "denied" {
			t.Fatalf("partition %q re-opens telemetry: %q", language, partition.Telemetry)
		}
		want := profile.PolyglotRoot + "/" + language
		if partition.CacheRoot != want {
			t.Fatalf("partition %q caches in %q, not in its own bounded root %q",
				language, partition.CacheRoot, want)
		}
		if !strings.HasPrefix(partition.CacheRoot, profile.PolyglotRoot+"/") {
			t.Fatalf("partition %q caches outside the polyglot root: %q", language, partition.CacheRoot)
		}
		if roots[partition.CacheRoot] {
			t.Fatalf("partition %q shares its cache root %q with another partition", language, partition.CacheRoot)
		}
		if runtimes[partition.Runtime] {
			t.Fatalf("partition %q shares its runtime %q with another partition", language, partition.Runtime)
		}
		roots[partition.CacheRoot] = true
		runtimes[partition.Runtime] = true
	}
	// No partition cache may contain another one, and none may contain the polyglot root.
	for outer := range roots {
		if outer == profile.PolyglotRoot || strings.HasPrefix(profile.PolyglotRoot, outer+"/") {
			t.Fatalf("cache root %q reaches the polyglot root %q", outer, profile.PolyglotRoot)
		}
		for inner := range roots {
			if inner != outer && strings.HasPrefix(inner, outer+"/") {
				t.Fatalf("cache root %q is nested inside %q", inner, outer)
			}
		}
	}
	// The environment spelling of a bounded directory and the declared one are the same
	// directory. If they ever disagree, a package manager reads or writes a path the plan
	// never bounded, and the partition's cache is exactly where that would land.
	for _, pair := range []struct {
		variable string
		declared string
	}{
		{"AURUM_POLYGLOT_ROOT", profile.PolyglotRoot},
		{"NUGET_PACKAGES", profile.Toolchains["dotnet"].CacheRoot},
		{"CARGO_HOME", profile.Toolchains["rust"].CacheRoot},
	} {
		if profile.Environment[pair.variable] != pair.declared {
			t.Fatalf("%s %q is not the declared root %q", pair.variable, profile.Environment[pair.variable], pair.declared)
		}
	}
	// Resolving a partition may never create one.
	if profile.ToolchainSource != "digest-pinned" {
		t.Fatalf("profile does not pin its toolchain source: %q", profile.ToolchainSource)
	}
	if profile.ToolchainMaterialization != "pre-materialized" {
		t.Fatalf("profile materializes a toolchain on demand: %q", profile.ToolchainMaterialization)
	}
	for _, denial := range []struct {
		field string
		value string
	}{
		{"toolchain_install", profile.ToolchainInstall},
		{"toolchain_bootstrap", profile.ToolchainBootstrap},
		{"package_install", profile.PackageInstall},
		{"package_registry", profile.PackageRegistry},
		{"egress", profile.Egress},
		{"host_toolchain", profile.HostToolchain},
		{"host_cache", profile.HostCache},
		{"subprocess", profile.Subprocess},
		{"unregistered_language", profile.UnregisteredLanguage},
	} {
		if denial.value != "denied" {
			t.Fatalf("profile does not deny %s: %q", denial.field, denial.value)
		}
	}
	// This card publishes a document; the plan declares that nothing is ever installed.
	if profile.PackageInstalls != 0 {
		t.Fatalf("profile declares %d package installs", profile.PackageInstalls)
	}
	// The eight partition caches coexist inside the one bounded tmpfs, so their combined
	// ceiling has to fit it. A ceiling that fits alone but not eight times over would have
	// to spill outside the plan's own bound.
	if profile.MaxCacheBytes <= 0 || profile.TmpfsMB <= 0 {
		t.Fatalf("profile declares an unbounded cache: max_cache_bytes=%d tmpfs_mb=%d",
			profile.MaxCacheBytes, profile.TmpfsMB)
	}
	if profile.MaxCacheBytes > profile.TmpfsMB*1024*1024 {
		t.Fatalf("max_cache_bytes %d exceeds the %d MiB bounded tmpfs",
			profile.MaxCacheBytes, profile.TmpfsMB)
	}
	if len(profile.Languages)*profile.MaxCacheBytes > profile.TmpfsMB*1024*1024 {
		t.Fatalf("%d partitions of %d bytes exceed the %d MiB bounded tmpfs",
			len(profile.Languages), profile.MaxCacheBytes, profile.TmpfsMB)
	}
	// And the package corpus has to fit inside the declared input bound, because every
	// cached package is a materialized input.
	if profile.MaxPackages <= 0 || profile.MaxPackageBytes <= 0 {
		t.Fatalf("profile declares an unbounded package set: max_packages=%d max_package_bytes=%d",
			profile.MaxPackages, profile.MaxPackageBytes)
	}
	if profile.MaxPackages > profile.MaxInputFiles {
		t.Fatalf("a package set of %d entries exceeds the %d file input bound",
			profile.MaxPackages, profile.MaxInputFiles)
	}
	if profile.MaxPackages*profile.MaxPackageBytes > profile.MaxInputBytes {
		t.Fatalf("a package set of %d packages of %d bytes exceeds the %d byte input bound",
			profile.MaxPackages, profile.MaxPackageBytes, profile.MaxInputBytes)
	}
	// A per-language deadline that outlived the run deadline would not bound anything.
	if profile.ResolveTimeoutSeconds <= 0 || profile.ResolveTimeoutSeconds > profile.TimeoutSeconds {
		t.Fatalf("resolve_timeout_seconds %d is not bounded by timeout_seconds %d",
			profile.ResolveTimeoutSeconds, profile.TimeoutSeconds)
	}
}
