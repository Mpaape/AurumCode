package integration

// Integration program for card AUR-471, selector IntegrationAUR471.
//
// tests/unit/AUR-471.go proves the policy contract with in-memory YAML.
// This program proves the same contract against REAL files on a REAL temp
// filesystem: the exact .aurumcode/iso25010-weights.yml layout a team
// would commit, including the historical thresholds/static_signals
// sections that must stay forward-compatible, and the refusal of a policy
// that tries to switch off secret redaction.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	policy "github.com/Mpaape/AurumCode/internal/config/policy"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func IntegrationAUR471(t *testing.T) {
	t.Run("RealWeightsFileDrivesEvaluation", testAUR471RealWeightsFile)
	t.Run("HistoricalSectionsStayCompatible", testAUR471HistoricalSections)
	t.Run("RealFileRefusesRedactionOff", testAUR471RealFileRefusesRedaction)
	t.Run("RealZeroConfigFileAbsent", testAUR471RealZeroConfig)
}

func writeRealPolicy(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".aurumcode")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "iso25010-weights.yml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return root
}

const realWeightsHeader = `# ISO/IEC 25010 Quality Characteristics Weights
weights:
  functionality: 0.15
  reliability: 0.15
  usability: 0.10
  efficiency: 0.12
  maintainability: 0.18
  portability: 0.08
  security: 0.17
  compatibility: 0.05
`

func testAUR471RealWeightsFile(t *testing.T) {
	root := writeRealPolicy(t, realWeightsHeader)
	p, err := policy.Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if p == nil || len(p.Weights) != 8 {
		t.Fatalf("want 8 weights, got %+v", p)
	}
	ev, err := policy.Evaluate(p, types.ISOScores{Functionality: 80, Security: 40})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(ev.PerCharacteristic) != 8 {
		t.Fatalf("want 8 rows, got %d", len(ev.PerCharacteristic))
	}
	out, err := policy.Render(&types.ReviewResult{Verdict: "pass"}, ev)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("render is not valid JSON: %v", err)
	}
	if _, ok := decoded["iso_policy"]; !ok {
		t.Fatalf("rendered review must carry iso_policy when a policy is declared: %s", out)
	}
	if _, ok := decoded["aggregate"]; !ok {
		if sub, ok := decoded["iso_policy"].(map[string]any); !ok || sub["aggregate"] == nil {
			t.Fatalf("iso_policy must carry the aggregate: %s", out)
		}
	}
}

func testAUR471HistoricalSections(t *testing.T) {
	root := writeRealPolicy(t, realWeightsHeader+`thresholds:
  excellent: 90
  good: 75
static_signals:
  complexity_increase: -5
`)
	p, err := policy.Load(root)
	if err != nil {
		t.Fatalf("historical sections must stay forward-compatible: %v", err)
	}
	if p == nil || len(p.Weights) != 8 {
		t.Fatalf("weights not parsed: %+v", p)
	}
}

func testAUR471RealFileRefusesRedaction(t *testing.T) {
	root := writeRealPolicy(t, realWeightsHeader+"redaction: disabled\n")
	_, err := policy.Load(root)
	if err == nil {
		t.Fatal("a real policy disabling redaction must be refused")
	}
	if !contains471(err.Error(), "redaction") || !contains471(err.Error(), "refused-clause") {
		t.Fatalf("refusal must be named, got %v", err)
	}
}

func testAUR471RealZeroConfig(t *testing.T) {
	root := t.TempDir()
	p, err := policy.Load(root)
	if err != nil || p != nil {
		t.Fatalf("absent file must be (nil, nil), got %+v, %v", p, err)
	}
}

func contains471(h, n string) bool {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return true
		}
	}
	return false
}
