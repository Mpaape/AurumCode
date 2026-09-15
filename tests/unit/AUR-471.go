package unit

// Unit program for card AUR-471, selector TestAUR471.
//
// Proves internal/config/policy in isolation, no CLI and no model:
//   - declared weights drive a per-characteristic and an aggregate score
//     that are coherent with the file (weight*score, rounded sum);
//   - invalid sum, negative weight and unknown characteristic each fail
//     with a NAMED error, so no model call can ever happen for them;
//   - the explicit policy is refused when it tries to disable the
//     deterministic security pass or secret redaction, naming the clause;
//   - absent file is a byte-for-byte no-op against json.Marshal.

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"

	policy "github.com/Mpaape/AurumCode/internal/config/policy"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func TestAUR471(t *testing.T) {
	t.Run("DeclaredWeightsReport", testAUR471DeclaredWeightsReport)
	t.Run("AggregateFollowsWeights", testAUR471AggregateFollowsWeights)
	t.Run("InvalidSumNamedError", testAUR471InvalidSumNamedError)
	t.Run("DuplicateWeightNamedError", testAUR471DuplicateWeightNamedError)
	t.Run("TildeSecurityPassRefused", testAUR471TildeSecurityPassRefused)
	t.Run("NegativeWeightNamedError", testAUR471NegativeWeightNamedError)
	t.Run("NonFiniteWeightNamedError", testAUR471NonFiniteWeightNamedError)
	t.Run("UnknownCharacteristicNamedError", testAUR471UnknownCharacteristicNamedError)
	t.Run("RefusedSecurityPassClause", testAUR471RefusedSecurityPassClause)
	t.Run("RefusedRedactionClause", testAUR471RefusedRedactionClause)
	t.Run("AliasedRedactionClauseRefused", testAUR471AliasedRedactionClauseRefused)
	t.Run("ConventionParsed", testAUR471ConventionParsed)
	t.Run("ZeroConfigByteIdentical", testAUR471ZeroConfigByteIdentical)
}

func writePolicy(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, ".aurumcode")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "iso25010-weights.yml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write policy: %v", err)
	}
}

const fullWeights = `weights:
  functionality: 0.15
  reliability: 0.15
  usability: 0.10
  efficiency: 0.12
  maintainability: 0.18
  portability: 0.08
  security: 0.17
  compatibility: 0.05
`

func sampleScores() types.ISOScores {
	return types.ISOScores{
		Functionality: 80, Reliability: 90, Usability: 70, Efficiency: 60,
		Maintainability: 100, Portability: 50, Security: 40, Compatibility: 30,
	}
}

func testAUR471DeclaredWeightsReport(t *testing.T) {
	root := t.TempDir()
	writePolicy(t, root, fullWeights)
	p, err := policy.Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if p == nil {
		t.Fatal("Load must return a policy for a declared file")
	}
	ev, err := policy.Evaluate(p, sampleScores())
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if ev == nil || !ev.Enabled {
		t.Fatal("Evaluate must return an enabled evaluation for a non-nil policy")
	}
	if len(ev.PerCharacteristic) != 8 {
		t.Fatalf("want 8 characteristics, got %d", len(ev.PerCharacteristic))
	}
	byName := map[policy.Characteristic]policy.CharacteristicScore{}
	for _, row := range ev.PerCharacteristic {
		byName[row.Characteristic] = row
	}
	security := byName[policy.Security]
	if security.Score != 40 || math.Abs(security.Weight-0.17) > 1e-12 {
		t.Fatalf("security row wrong: %+v", security)
	}
	if math.Abs(security.Contribution-6.8) > 1e-9 {
		t.Fatalf("security contribution = %v, want 6.8", security.Contribution)
	}
}

func testAUR471AggregateFollowsWeights(t *testing.T) {
	rootA := t.TempDir()
	writePolicy(t, rootA, fullWeights)
	// A second declaration that shifts 0.12 from maintainability to
	// security; the aggregate must move with the file, not stay fixed.
	const shifted = `weights:
  functionality: 0.15
  reliability: 0.15
  usability: 0.10
  efficiency: 0.12
  maintainability: 0.06
  portability: 0.08
  security: 0.29
  compatibility: 0.05
`
	rootB := t.TempDir()
	writePolicy(t, rootB, shifted)

	pA, err := policy.Load(rootA)
	if err != nil {
		t.Fatalf("Load A: %v", err)
	}
	pB, err := policy.Load(rootB)
	if err != nil {
		t.Fatalf("Load B: %v", err)
	}
	evA, _ := policy.Evaluate(pA, sampleScores())
	evB, _ := policy.Evaluate(pB, sampleScores())

	wantA := weighted(sampleScores(), map[policy.Characteristic]float64{
		policy.Functionality: 0.15, policy.Reliability: 0.15, policy.Usability: 0.10,
		policy.Efficiency: 0.12, policy.Maintainability: 0.18, policy.Portability: 0.08,
		policy.Security: 0.17, policy.Compatibility: 0.05,
	})
	if evA.Aggregate != wantA {
		t.Fatalf("aggregate A = %d, want %d", evA.Aggregate, wantA)
	}
	if evA.Aggregate == evB.Aggregate {
		t.Fatalf("aggregate did not follow the declared weights: A=%d B=%d", evA.Aggregate, evB.Aggregate)
	}
}

func weighted(s types.ISOScores, w map[policy.Characteristic]float64) int {
	exact := float64(s.Functionality)*w[policy.Functionality] +
		float64(s.Reliability)*w[policy.Reliability] +
		float64(s.Usability)*w[policy.Usability] +
		float64(s.Efficiency)*w[policy.Efficiency] +
		float64(s.Maintainability)*w[policy.Maintainability] +
		float64(s.Portability)*w[policy.Portability] +
		float64(s.Security)*w[policy.Security] +
		float64(s.Compatibility)*w[policy.Compatibility]
	return int(math.Round(exact))
}

func testAUR471InvalidSumNamedError(t *testing.T) {
	root := t.TempDir()
	writePolicy(t, root, `weights:
  functionality: 0.50
  reliability: 0.10
  usability: 0.10
  efficiency: 0.10
  maintainability: 0.10
  portability: 0.05
  security: 0.05
  compatibility: 0.05
`)
	_, err := policy.Load(root)
	if !errors.Is(err, policy.ErrWeightsSum) {
		t.Fatalf("want ErrWeightsSum, got %v", err)
	}
}

func testAUR471DuplicateWeightNamedError(t *testing.T) {
	root := t.TempDir()
	// A repeated characteristic would have its first declaration counted in
	// the raw sum but overwritten in the map, so a file whose effective sum
	// is 0.5 could pass. It must be refused, naming invalid-weight, with no
	// policy produced.
	writePolicy(t, root, `weights:
  functionality: 0.50
  functionality: 0.50
  reliability: 0.10
  usability: 0.10
  efficiency: 0.10
  maintainability: 0.10
  portability: 0.05
  security: 0.05
  compatibility: 0.05
`)
	p, err := policy.Load(root)
	if !errors.Is(err, policy.ErrInvalidWeight) {
		t.Fatalf("want ErrInvalidWeight for a duplicated weight, got %v", err)
	}
	if p != nil {
		t.Fatalf("Load must not return a policy for a duplicated weight, got %+v", p)
	}
	if err == nil || !contains471(err.Error(), "functionality") {
		t.Fatalf("refusal must name the duplicated characteristic, got %v", err)
	}
}

func testAUR471TildeSecurityPassRefused(t *testing.T) {
	root := t.TempDir()
	writePolicy(t, root, fullWeights+"security_pass: ~\n")
	_, err := policy.Load(root)
	if !errors.Is(err, policy.ErrRefusedClause) {
		t.Fatalf("want ErrRefusedClause for security_pass: ~, got %v", err)
	}
	if err == nil || !contains471(err.Error(), "security_pass") {
		t.Fatalf("refusal must name the clause, got %v", err)
	}
}

func testAUR471NegativeWeightNamedError(t *testing.T) {
	root := t.TempDir()
	writePolicy(t, root, `weights:
  functionality: 0.30
  reliability: 0.15
  usability: 0.10
  efficiency: 0.12
  maintainability: 0.18
  portability: 0.08
  security: -0.05
  compatibility: 0.12
`)
	_, err := policy.Load(root)
	if !errors.Is(err, policy.ErrNegativeWeight) {
		t.Fatalf("want ErrNegativeWeight, got %v", err)
	}
}

func testAUR471NonFiniteWeightNamedError(t *testing.T) {
	root := t.TempDir()
	writePolicy(t, root, `weights:
  functionality: .nan
  reliability: 0.15
  usability: 0.10
  efficiency: 0.12
  maintainability: 0.18
  portability: 0.08
  security: 0.17
  compatibility: 0.05
`)
	p, err := policy.Load(root)
	if !errors.Is(err, policy.ErrNonFiniteWeight) {
		t.Fatalf("want ErrNonFiniteWeight, got %v", err)
	}
	if p != nil {
		t.Fatalf("Load must not return a policy for a non-finite weight, got %+v", p)
	}
}

func testAUR471UnknownCharacteristicNamedError(t *testing.T) {
	root := t.TempDir()
	writePolicy(t, root, `weights:
  functionality: 0.15
  reliability: 0.15
  usability: 0.10
  efficiency: 0.12
  maintainability: 0.18
  portability: 0.08
  security: 0.17
  compatibility: 0.05
  agility: 0.10
`)
	_, err := policy.Load(root)
	if !errors.Is(err, policy.ErrUnknownCharacteristic) {
		t.Fatalf("want ErrUnknownCharacteristic, got %v", err)
	}
}

func testAUR471RefusedSecurityPassClause(t *testing.T) {
	root := t.TempDir()
	writePolicy(t, root, fullWeights+"security_pass: false\n")
	_, err := policy.Load(root)
	if !errors.Is(err, policy.ErrRefusedClause) {
		t.Fatalf("want ErrRefusedClause, got %v", err)
	}
	if err == nil || !contains471(err.Error(), "security_pass") {
		t.Fatalf("refusal must name the clause, got %v", err)
	}
}

func testAUR471RefusedRedactionClause(t *testing.T) {
	root := t.TempDir()
	writePolicy(t, root, fullWeights+"redaction: off\n")
	_, err := policy.Load(root)
	if !errors.Is(err, policy.ErrRefusedClause) {
		t.Fatalf("want ErrRefusedClause, got %v", err)
	}
	if err == nil || !contains471(err.Error(), "redaction") {
		t.Fatalf("refusal must name the clause, got %v", err)
	}
}

func testAUR471AliasedRedactionClauseRefused(t *testing.T) {
	root := t.TempDir()
	writePolicy(t, root, fullWeights+"off: &off false\nredaction: *off\n")
	_, err := policy.Load(root)
	if !errors.Is(err, policy.ErrRefusedClause) {
		t.Fatalf("want ErrRefusedClause for an aliased disabling value, got %v", err)
	}
	if err == nil || !contains471(err.Error(), "redaction") {
		t.Fatalf("refusal must name the redaction clause, got %v", err)
	}
}

func testAUR471ConventionParsed(t *testing.T) {
	root := t.TempDir()
	writePolicy(t, root, fullWeights+`convention:
  severity_limit:
    security: error
    portability: info
  blocking: [security, reliability]
  informative: [portability]
`)
	p, err := policy.Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if p.Convention.SeverityLimit[policy.Security] != policy.SeverityError {
		t.Fatalf("security limit = %q", p.Convention.SeverityLimit[policy.Security])
	}
	if len(p.Convention.Blocking) != 2 || p.Convention.Blocking[0] != policy.Security {
		t.Fatalf("blocking = %v", p.Convention.Blocking)
	}
}

func testAUR471ZeroConfigByteIdentical(t *testing.T) {
	root := t.TempDir()
	p, err := policy.Load(root)
	if err != nil {
		t.Fatalf("Load empty: %v", err)
	}
	if p != nil {
		t.Fatalf("missing file must be nil policy, got %+v", p)
	}
	ev, err := policy.Evaluate(p, sampleScores())
	if err != nil || ev != nil {
		t.Fatalf("Evaluate(nil) = %+v, %v; want nil, nil", ev, err)
	}
	result := &types.ReviewResult{
		Verdict:   "pass",
		Summary:   "unchanged",
		ISOScores: &types.ISOScores{Functionality: 80},
	}
	got, err := policy.Render(result, ev)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("zero-config Render is not byte-identical:\n got=%s\nwant=%s", got, want)
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
