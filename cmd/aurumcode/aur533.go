// AUR-533: the analysis-data artifact's age/digest gate. applyAnalysisDataGate
// runs once, right before the Dependency-Track gate in both runReview
// (main.go) and runPRReview (pr.go), and folds its outcome into the SAME
// gateDecision those two already publish, following aur550.go's
// applyDTrackGate/mergeDTrackGate pattern (mergeDTrackGate itself is reused:
// it only reads the gateDecision fields).
//
// It is a complete no-op -- no network call, no gate line, no audit field --
// unless the effective config declares `analysis_data` (repository or central
// policy; ApplyCentralPolicy has already resolved precedence).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Mpaape/AurumCode/internal/artifacts"
	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// Seams for tests: a local fake GitHub server and a fixed clock. Production
// never overrides them.
var (
	analysisDataAPIBase = artifacts.DefaultAPIBase
	analysisDataNow     = time.Now
	analysisDataCache   = func() string {
		if d, err := os.UserCacheDir(); err == nil {
			return filepath.Join(d, "aurumcode", "analysis-data")
		}
		return filepath.Join(os.TempDir(), "aurumcode-analysis-data")
	}
)

// analysisDataReviewKinds are the artifact file kinds a review downloads
// today. The OSV copy (kind "osv") is large and nothing in the review reads
// it yet; its set digest is still verified through the manifest.
var analysisDataReviewKinds = []string{"scanners"}

// analysisDataAudit is what AUR-521's audit record gets for a usable artifact.
type analysisDataAudit struct {
	Digest      string
	GeneratedAt string
	Tag         string
}

// applyAnalysisDataGate resolves the artifact when cfg is declared. An
// unusable outcome is inconclusive by the policy's mode ("block" fails the
// check, anything else only withholds approval) and carries the reason code;
// a usable one adds no gate line and returns the audit facts.
func applyAnalysisDataGate(ctx context.Context, cfg *config.AnalysisDataConfig, inconclusiveMode string) (result gateDecision, reason string, audit *analysisDataAudit) {
	if !cfg.Declared() {
		return gateDecision{}, "", nil
	}
	out := artifacts.Resolve(ctx, artifacts.Options{
		APIBase:    analysisDataAPIBase,
		Repository: cfg.EffectiveRepository(),
		Token:      os.Getenv("GITHUB_TOKEN"),
		CacheDir:   analysisDataCache(),
		MaxAgeDays: cfg.EffectiveMaxAgeDays(),
		Now:        analysisDataNow,
		Kinds:      analysisDataReviewKinds,
	})
	if !out.Usable {
		result.Active = true
		result.Inconclusive = true
		result.Fail = inconclusiveMode == "block"
		result.Lines = append(result.Lines, fmt.Sprintf("analysis_data: revisão inconclusiva (%s): %s", out.Reason, out.Detail))
		return result, out.Reason, nil
	}
	return gateDecision{}, "", &analysisDataAudit{
		Digest:      out.Digest,
		GeneratedAt: out.GeneratedAt.UTC().Format(time.RFC3339),
		Tag:         out.Tag,
	}
}

// addAnalysisDataToAuditFile adds the three analysis_data_* keys to the audit
// record render.WriteAuditRecord just wrote (render's AuditRecord has no field
// for them and lives outside this card's paths), re-redacting the result.
func addAnalysisDataToAuditFile(path string, a *analysisDataAudit, filter *redaction.Filter) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var rec map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rec); err != nil {
		return err
	}
	for k, v := range map[string]string{
		"analysis_data_digest":       a.Digest,
		"analysis_data_generated_at": a.GeneratedAt,
		"analysis_data_tag":          a.Tag,
	} {
		b, _ := json.Marshal(v)
		rec[k] = b
	}
	out, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(filter.Redact(string(out))+"\n"), 0o600)
}
