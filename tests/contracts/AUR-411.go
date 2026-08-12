package contracts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var digestAUR411 = regexp.MustCompile(`\Asha256:[0-9a-f]{64}\z`)

// The eight language partitions and the runtime command each one resolves to, in canonical
// ascending order. The contract lane pins the binding independently of the loader: a
// partition that quietly changed which runtime it points at would merge two languages the
// profile promises to keep apart.
var partitionsAUR411 = []struct {
	language string
	runtime  string
}{
	{"bash", "bash"},
	{"c-cpp", "cc"},
	{"dotnet", "dotnet"},
	{"javascript", "node"},
	{"powershell", "pwsh"},
	{"python", "python3"},
	{"rust", "cargo"},
	{"typescript", "tsc"},
}

// ContractAUR411 pins the public contract of the polyglot-toolchain-v1 entry: the three
// documents this card publishes exist, parse, and declare exactly the key, schema id and
// lock path the registry advertises, exactly once. It reads nothing else, installs no
// toolchain, installs no package and invokes no runtime.
//
// The registry's total arity is deliberately not asserted here. This card owns the
// polyglot-toolchain-v1 key, not the size of the registry; the exact registered key set is
// asserted by tests/unit/AUR-411.go, which this card also owns and which the next profile
// card must extend when it registers a twelfth key.
func ContractAUR411(t *testing.T) {
	root := os.Getenv("AURUMCODE_ROOT")
	if root == "" {
		t.Fatal("AURUMCODE_ROOT is required")
	}
	read := func(path string) map[string]any {
		b, err := os.ReadFile(filepath.Join(root, path))
		if err != nil || len(b) == 0 {
			t.Fatalf("unreadable %s", path)
		}
		var doc map[string]any
		if json.Unmarshal(b, &doc) != nil {
			t.Fatalf("invalid JSON %s", path)
		}
		return doc
	}

	schema := read(".board/schemas/polyglot-toolchain-profile.schema.json")
	if schema["$id"] != "https://aurumcode.dev/schemas/polyglot-toolchain-profile.schema.json" {
		t.Fatalf("schema publishes the wrong $id: %v", schema["$id"])
	}
	if schema["additionalProperties"] != false {
		t.Fatal("schema does not close its property set")
	}

	profile := read(".board/oci/profiles/polyglot-toolchain-v1.json")
	for key, want := range map[string]any{
		"schema":                    "aurum.polyglot-toolchain-profile",
		"profile":                   "polyglot-toolchain-v1",
		"lock":                      ".board/locks/oci/polyglot-toolchain-v1.lock.json",
		"network":                   "none",
		"pull":                      "never",
		"user":                      "65534:65534",
		"cap_drop":                  "ALL",
		"cap_add":                   "none",
		"mounts":                    "none",
		"devices":                   "none",
		"sockets":                   "none",
		"tmpfs":                     "rw,nosuid,nodev",
		"polyglot_root":             "/tmp/aurum-polyglot",
		"toolchain_source":          "digest-pinned",
		"toolchain_materialization": "pre-materialized",
		"toolchain_install":         "denied",
		"toolchain_bootstrap":       "denied",
		"package_install":           "denied",
		"package_registry":          "denied",
		"cache_mode":                "read-only",
		"telemetry":                 "denied",
		"egress":                    "denied",
		"host_toolchain":            "denied",
		"host_cache":                "denied",
		"host_filesystem":           "denied",
		"subprocess":                "denied",
		"unregistered_language":     "denied",
	} {
		if profile[key] != want {
			t.Fatalf("profile key %s is %v, want %v", key, profile[key], want)
		}
	}
	for key, want := range map[string]float64{
		"timeout_seconds":         60,
		"memory_mb":               256,
		"cpu_millis":              1000,
		"pids_limit":              128,
		"tmpfs_mb":                128,
		"stdout_limit_bytes":      65536,
		"stderr_limit_bytes":      65536,
		"max_languages":           8,
		"max_packages":            4096,
		"max_package_bytes":       16384,
		"max_cache_bytes":         8388608,
		"resolve_timeout_seconds": 5,
		// Declared zero, and measured zero by the acceptance: this card publishes a
		// document and never installs a toolchain or a package.
		"package_installs": 0,
	} {
		if profile[key] != want {
			t.Fatalf("profile bound %s is %v, want %v", key, profile[key], want)
		}
	}
	for key, want := range map[string]bool{
		"checkout_readonly":      true,
		"read_only_rootfs":       true,
		"no_new_privileges":      true,
		"module_cache_read_only": true,
		"privileged":             false,
	} {
		if profile[key] != want {
			t.Fatalf("profile flag %s is %v, want %v", key, profile[key], want)
		}
	}
	languages, ok := profile["languages"].([]any)
	if !ok {
		t.Fatalf("profile does not publish a language list: %v", profile["languages"])
	}
	if len(languages) != len(partitionsAUR411) {
		t.Fatalf("profile publishes %d languages, want %d", len(languages), len(partitionsAUR411))
	}
	for i, partition := range partitionsAUR411 {
		if languages[i] != partition.language {
			t.Fatalf("language %d is %v, want %s", i, languages[i], partition.language)
		}
	}
	toolchains, ok := profile["toolchains"].(map[string]any)
	if !ok {
		t.Fatalf("profile does not publish a partition object: %v", profile["toolchains"])
	}
	if len(toolchains) != len(partitionsAUR411) {
		t.Fatalf("profile publishes %d partitions, want %d", len(toolchains), len(partitionsAUR411))
	}
	// Each partition is pinned whole: its runtime, its content addressing, its own bounded
	// cache directory, a read-only cache and denied telemetry.
	for _, partition := range partitionsAUR411 {
		raw, ok := toolchains[partition.language].(map[string]any)
		if !ok {
			t.Fatalf("partition %s is not an object: %v", partition.language, toolchains[partition.language])
		}
		for key, want := range map[string]any{
			"runtime":    partition.runtime,
			"toolchain":  "digest-pinned",
			"cache_root": "/tmp/aurum-polyglot/" + partition.language,
			"cache_mode": "read-only",
			"telemetry":  "denied",
		} {
			if raw[key] != want {
				t.Fatalf("partition %s key %s is %v, want %v", partition.language, key, raw[key], want)
			}
		}
		if len(raw) != 5 {
			t.Fatalf("partition %s publishes %d keys, want 5", partition.language, len(raw))
		}
	}
	environment, ok := profile["environment"].(map[string]any)
	if !ok {
		t.Fatalf("profile does not publish an environment object: %v", profile["environment"])
	}
	// Every channel through which a package manager would reach an index, a registry or a
	// telemetry endpoint is closed in the published bytes as well, not only in the plan
	// fields above.
	for key, want := range map[string]any{
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
	} {
		if environment[key] != want {
			t.Fatalf("environment %s is %v, want %v", key, environment[key], want)
		}
	}

	lock := read(".board/locks/oci/polyglot-toolchain-v1.lock.json")
	if lock["schema"] != "aurum.oci-image-lock" || lock["profile"] != "polyglot-toolchain-v1" {
		t.Fatalf("lock does not bind to polyglot-toolchain-v1: %v", lock)
	}
	image, ok := lock["image"].(string)
	if !ok || len(image) == 0 {
		t.Fatal("lock does not publish an image")
	}
	if image != "aurum-bootstrap-go-bash@sha256:503ac356fa6bca4bad56fade87b5479ff371bd446ffb9c7db91f211323c7c73e" {
		t.Fatalf("lock publishes an unpinned or foreign image: %s", image)
	}

	registry := read(".board/oci/profiles/registry.v1.json")
	entries, ok := registry["profiles"].([]any)
	if !ok || len(entries) == 0 {
		t.Fatalf("registry does not publish a profile list: %v", registry["profiles"])
	}
	found := 0
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			t.Fatal("registry entry is not an object")
		}
		if entry["key"] != "polyglot-toolchain-v1" {
			continue
		}
		found++
		if entry["schema"] != ".board/schemas/polyglot-toolchain-profile.schema.json" ||
			entry["lock"] != ".board/locks/oci/polyglot-toolchain-v1.lock.json" {
			t.Fatalf("registry entry points at foreign documents: %v", entry)
		}
		for _, field := range []string{"schema_digest", "lock_digest", "image_set_digest"} {
			value, ok := entry[field].(string)
			if !ok || !digestAUR411.MatchString(value) {
				t.Fatalf("registry entry field %s is not a sha256 digest: %v", field, entry[field])
			}
		}
	}
	if found != 1 {
		t.Fatalf("registry publishes polyglot-toolchain-v1 %d times, want exactly 1", found)
	}
}
