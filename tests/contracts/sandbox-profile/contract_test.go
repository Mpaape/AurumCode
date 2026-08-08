package sandboxprofile_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/sandbox/profile"
	"gopkg.in/yaml.v3"
)

type fixtureFile struct {
	SchemaPath string        `yaml:"schema_path"`
	Lock       string        `yaml:"lock"`
	Cases      []fixtureCase `yaml:"cases"`
}

type fixtureCase struct {
	Name     string `yaml:"name"`
	Expected string `yaml:"expected"`
	Profile  string `yaml:"profile"`
	Lock     string `yaml:"lock"`
}

func TestContractAUR006(t *testing.T) {
	root := repoRootAUR006(t)
	fixtureBytes, err := os.ReadFile(filepath.Join(root, "tests/specs/AUR-006/cases.yaml"))
	if err != nil {
		t.Fatalf("AUR-006 fixture unreadable: %v", err)
	}
	var fixture fixtureFile
	if err := yaml.Unmarshal(fixtureBytes, &fixture); err != nil {
		t.Fatalf("AUR-006 fixture is not valid YAML: %v", err)
	}
	if len(fixture.Cases) != 11 {
		t.Fatalf("AUR-006 fixture has %d cases, want 11", len(fixture.Cases))
	}

	schema, err := os.ReadFile(filepath.Join(root, fixture.SchemaPath))
	if err != nil {
		t.Fatalf("AUR-006 schema unreadable: %v", err)
	}
	defaultLock := []byte(fixture.Lock)

	var nominal profile.ValidationResult
	for _, testCase := range fixture.Cases {
		t.Run(testCase.Name, func(t *testing.T) {
			lock := defaultLock
			if testCase.Lock != "" {
				lock = []byte(testCase.Lock)
			}
			result := profile.ValidateBootstrapProfile([]byte(testCase.Profile), schema, lock)
			if result.Code != testCase.Expected {
				t.Fatalf("code=%q, want %q (status=%q)", result.Code, testCase.Expected, result.Status)
			}
			if result.EngineInvocations != 0 {
				t.Fatalf("engine_invocations=%d, want 0", result.EngineInvocations)
			}
			if testCase.Name == "nominal" {
				nominal = result
				if result.Status != "valid" {
					t.Fatalf("nominal status=%q, want valid", result.Status)
				}
			}
			if testCase.Name != "nominal" && result.Status == "valid" {
				t.Fatalf("hostile case was accepted")
			}
		})
	}

	replay := profile.ValidateBootstrapProfile([]byte(fixture.Cases[0].Profile), schema, defaultLock)
	if replay != nominal {
		t.Fatalf("replay result differs: first=%+v replay=%+v", nominal, replay)
	}
}

func TestAUR006RejectsUnknownAndDuplicateFields(t *testing.T) {
	root := repoRootAUR006(t)
	schema, err := os.ReadFile(filepath.Join(root, ".board/schemas/container-profile.schema.json"))
	if err != nil {
		t.Fatalf("schema unreadable: %v", err)
	}
	lock := []byte(`{"schema":"aurum.oci-image-lock","version":1,"profile":"bootstrap-readonly-v1","image":"bash@sha256:ae4668c2560999e65e89532cd2ad1b6688bb23298189f0bd229ef80fa4bd0831"}`)

	profileJSON := `{"schema":"aurum.container-profile","version":1,"profile":"bootstrap-readonly-v1","lock":".board/locks/oci/bootstrap-readonly-v1.lock.json","lock_digest":"sha256:cf03c2e277e797e34c7f01db944e20e98a46cd74f1351846cb86d6d1f908e98c","network":"none","user":"65534:65534","cap_drop":"ALL","cap_add":"none","mounts":"none","devices":"none","pull":"never","tmpfs":"rw,noexec,nosuid,nodev","read_only_rootfs":true,"no_new_privileges":true,"privileged":false,"timeout_seconds":120,"memory_mb":256,"cpu_millis":1000,"pids_limit":128,"tmpfs_mb":32,"stdout_limit_bytes":65536,"stderr_limit_bytes":65536,"max_input_files":10000,"max_input_bytes":67108864}`
	unknown := profile.ValidateBootstrapProfile([]byte(profileJSON[:len(profileJSON)-1]+`,"unexpected":true}`), schema, lock)
	if unknown.Code != "schema_invalid" || unknown.EngineInvocations != 0 {
		t.Fatalf("unknown field result=%+v", unknown)
	}
	duplicate := profile.ValidateBootstrapProfile([]byte(`{"schema":"aurum.container-profile","schema":"aurum.container-profile"}`), schema, lock)
	if duplicate.Code != "schema_invalid" || duplicate.EngineInvocations != 0 {
		t.Fatalf("duplicate field result=%+v", duplicate)
	}
	missing := profile.ValidateBootstrapProfile([]byte(strings.Replace(profileJSON, `,"privileged":false`, "", 1)), schema, lock)
	if missing.Code != "schema_invalid" || missing.EngineInvocations != 0 {
		t.Fatalf("missing required field result=%+v", missing)
	}
}

func TestAUR006RejectsMalformedValuesAndUnsafeImages(t *testing.T) {
	root := repoRootAUR006(t)
	schema, err := os.ReadFile(filepath.Join(root, ".board/schemas/container-profile.schema.json"))
	if err != nil {
		t.Fatalf("schema unreadable: %v", err)
	}
	lock, err := os.ReadFile(filepath.Join(root, "tests/specs/AUR-006/cases.yaml"))
	if err != nil {
		t.Fatalf("fixture unreadable: %v", err)
	}
	var fixture fixtureFile
	if err := yaml.Unmarshal(lock, &fixture); err != nil {
		t.Fatalf("AUR-006 fixture is not valid YAML: %v", err)
	}
	profileJSON := strings.ReplaceAll(fixture.Cases[0].Profile, " ", "")
	defaultLock := []byte(fixture.Lock)

	malformed := []struct {
		name string
		old  string
		new  string
	}{
		{name: "mounts object", old: `"mounts":"none"`, new: `"mounts":{}`},
		{name: "mounts null", old: `"mounts":"none"`, new: `"mounts":null`},
		{name: "devices array", old: `"devices":"none"`, new: `"devices":[]`},
		{name: "capability array", old: `"cap_add":"none"`, new: `"cap_add":[]`},
		{name: "network object", old: `"network":"none"`, new: `"network":{}`},
		{name: "network null", old: `"network":"none"`, new: `"network":null`},
		{name: "resource string", old: `"timeout_seconds":120`, new: `"timeout_seconds":"120"`},
		{name: "resource null", old: `"timeout_seconds":120`, new: `"timeout_seconds":null`},
		{name: "unknown profile", old: `"profile":"bootstrap-readonly-v1"`, new: `"profile":"other-profile"`},
	}
	for _, testCase := range malformed {
		t.Run(testCase.name, func(t *testing.T) {
			document := replaceAUR006(t, profileJSON, testCase.old, testCase.new)
			result := profile.ValidateBootstrapProfile([]byte(document), schema, defaultLock)
			if result.Code != "schema_invalid" || result.Status != "invalid" || result.EngineInvocations != 0 {
				t.Fatalf("result=%+v, want structural rejection", result)
			}
		})
	}

	malformedImageLock := []byte(strings.Replace(string(defaultLock),
		`"image": "bash@sha256:ae4668c2560999e65e89532cd2ad1b6688bb23298189f0bd229ef80fa4bd0831"`,
		`"image": null`, 1))
	malformedImageDocument := replaceAUR006(t, profileJSON,
		`"lock_digest":"sha256:cf03c2e277e797e34c7f01db944e20e98a46cd74f1351846cb86d6d1f908e98c"`,
		`"lock_digest":"`+digestAUR006(malformedImageLock)+`"`)
	malformedImageResult := profile.ValidateBootstrapProfile([]byte(malformedImageDocument), schema, malformedImageLock)
	if malformedImageResult.Code != "lock_manifest_invalid" || malformedImageResult.Status != "invalid" || malformedImageResult.EngineInvocations != 0 {
		t.Fatalf("null image result=%+v, want lock_manifest_invalid", malformedImageResult)
	}

	for _, image := range []string{
		"foo//bar@sha256:ae4668c2560999e65e89532cd2ad1b6688bb23298189f0bd229ef80fa4bd0831",
		"foo/../bar@sha256:ae4668c2560999e65e89532cd2ad1b6688bb23298189f0bd229ef80fa4bd0831",
		"foo..bar@sha256:ae4668c2560999e65e89532cd2ad1b6688bb23298189f0bd229ef80fa4bd0831",
		"foo/@sha256:ae4668c2560999e65e89532cd2ad1b6688bb23298189f0bd229ef80fa4bd0831",
	} {
		name := strings.ReplaceAll(image, "/", "_")
		name = strings.ReplaceAll(name, "@", "_")
		t.Run("unsafe image "+name, func(t *testing.T) {
			mutatedLock := []byte(strings.Replace(string(defaultLock),
				"bash@sha256:ae4668c2560999e65e89532cd2ad1b6688bb23298189f0bd229ef80fa4bd0831",
				image, 1))
			document := replaceAUR006(t, profileJSON,
				`"lock_digest":"sha256:cf03c2e277e797e34c7f01db944e20e98e46cd74f1351846cb86d6d1f908e98c"`,
				`"lock_digest":"`+digestAUR006(mutatedLock)+`"`)
			result := profile.ValidateBootstrapProfile([]byte(document), schema, mutatedLock)
			if result.Code != "image_digest_required" || result.Status != "invalid" || result.EngineInvocations != 0 {
				t.Fatalf("result=%+v, want image_digest_required", result)
			}
		})
	}

	malformedSchema := strings.Replace(string(schema),
		`"type": "integer", "minimum": 1, "maximum": 120`,
		`"minimum": 1, "maximum": 120`, 1)
	result := profile.ValidateBootstrapProfile([]byte(profileJSON), []byte(malformedSchema), defaultLock)
	if result.Code != "schema_invalid" || result.Status != "invalid" || result.EngineInvocations != 0 {
		t.Fatalf("malformed schema result=%+v, want structural rejection", result)
	}
}

func replaceAUR006(t *testing.T, source, old, replacement string) string {
	t.Helper()
	if strings.Count(source, old) != 1 {
		t.Fatalf("replacement %q occurred %d times", old, strings.Count(source, old))
	}
	return strings.Replace(source, old, replacement, 1)
}

func digestAUR006(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func repoRootAUR006(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("AUR-006 caller path unavailable")
	}
	for dir := filepath.Dir(source); ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("AUR-006 go.mod not found")
		}
	}
}
