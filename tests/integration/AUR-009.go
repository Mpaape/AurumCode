//go:build aur009_redaction

package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	redaction "github.com/Mpaape/AurumCode/internal/security/redaction"
	suite "github.com/Mpaape/AurumCode/tests/security/redactionsuite"
)

// IntegrationAUR009 executes the sealed vectors of tests/specs/AUR-009 across
// all seven sinks, proves the harness bounds are enforced, and replays the
// sweep to prove exit code, order and digest are preserved. It prints bounded
// observation lines only; raw sink content and the canary never reach the log.
func IntegrationAUR009(t *testing.T) {
	canary := os.Getenv(redaction.CanaryEnv)
	if canary == "" || strings.ContainsAny(canary, " \t\r\n\"'") {
		t.Fatal("AUR-009 integration requires a non-empty, quote-free, space-free AURUM_SECRET_CANARY")
	}

	casesPath := os.Getenv("AURUM_A009_CASES")
	if casesPath == "" {
		casesPath = filepath.Join("..", "specs", "AUR-009", "cases.yaml")
	}
	spec, err := suite.Load(casesPath, canary)
	if err != nil {
		t.Fatalf("sealed spec load failed: %v", err)
	}

	// The harness must refuse more than 64 vectors, more than 4 MiB of
	// expanded vector bytes, and a deadline above 30 seconds.
	boundsDir := t.TempDir()
	tooMany := filepath.Join(boundsDir, "too-many.yaml")
	if err := os.WriteFile(tooMany, suite.SyntheticSpec(suite.MaxVectors+1), 0o600); err != nil {
		t.Fatalf("bounds fixture write failed: %v", err)
	}
	if _, err := suite.Load(tooMany, canary); err == nil || !strings.Contains(err.Error(), "cases-too-many") {
		t.Errorf("harness accepted %d vectors: %v", suite.MaxVectors+1, err)
	}
	tooLarge := filepath.Join(boundsDir, "too-large.yaml")
	if err := os.WriteFile(tooLarge, suite.SyntheticOversizeSpec(), 0o600); err != nil {
		t.Fatalf("bounds fixture write failed: %v", err)
	}
	if _, err := suite.Load(tooLarge, canary); err == nil || !strings.Contains(err.Error(), "cases-too-large") {
		t.Errorf("harness accepted an oversized vector payload: %v", err)
	}
	slowSpec := strings.Replace(string(suite.SyntheticSpec(3)),
		"deadline_seconds: 30", "deadline_seconds: 31", 1)
	tooSlow := filepath.Join(boundsDir, "too-slow.yaml")
	if err := os.WriteFile(tooSlow, []byte(slowSpec), 0o600); err != nil {
		t.Fatalf("bounds fixture write failed: %v", err)
	}
	if _, err := suite.Load(tooSlow, canary); err == nil || !strings.Contains(err.Error(), "limits-out-of-contract") {
		t.Errorf("harness accepted a deadline above 30s: %v", err)
	}

	first, err := suite.Run(spec, canary)
	if err != nil {
		t.Fatalf("vector sweep failed to execute: %v", err)
	}
	second, err := suite.Run(spec, canary)
	if err != nil {
		t.Fatalf("replay sweep failed to execute: %v", err)
	}
	if first.Digest != second.Digest || len(first.Cases) != len(second.Cases) {
		t.Error("AUR009_REPLAY_DIVERGENT")
	}
	for i := range first.Cases {
		if second.Cases[i].Name != first.Cases[i].Name || second.Cases[i].Result != first.Cases[i].Result {
			t.Error("AUR009_REPLAY_DIVERGENT")
			break
		}
	}

	failed := false
	for _, res := range first.Cases {
		fmt.Printf("AUR009_OBSERVATION\t%s\t%s\t%s\t%d\t%s\n",
			res.Name, res.Kind, res.Result, res.Effects, res.ArtifactSHA256)
		if res.Leak {
			failed = true
			fmt.Printf("AUR009_LEAK\t%s\t%s\n", res.Channel, res.Name)
		}
		if res.Mismatch {
			failed = true
			fmt.Printf("AUR009_MISMATCH\t%s\t%s\n", res.Channel, res.Name)
		}
	}
	fmt.Printf("AUR009_DIGEST\t%s\n", first.Digest)
	if failed {
		t.Fatal("AUR-009 sink redaction diverged from the sealed vectors")
	}
}
