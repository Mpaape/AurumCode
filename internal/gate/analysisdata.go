// The analysis-data artifact's age/digest gate. ApplyAnalysisDataGate runs
// once, right before the Dependency-Track gate, and returns a partial
// decision the pipeline merges into the run's Result (Result.Merge), like
// every other contributor.
//
// It is a complete no-op -- no network call, no gate line, no audit field --
// unless the effective config declares `analysis_data` (repository or central
// policy; ApplyCentralPolicy has already resolved precedence).
package gate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Mpaape/AurumCode/internal/artifacts"
	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/gate/facts"
	"github.com/Mpaape/AurumCode/internal/gate/reasons"
)

// Seams for tests: a local fake GitHub server and a fixed clock. Production
// never overrides them.
var (
	AnalysisDataAPIBase = artifacts.DefaultAPIBase
	AnalysisDataNow     = time.Now
	AnalysisDataCache   = func() string {
		if d, err := os.UserCacheDir(); err == nil {
			return filepath.Join(d, "aurumcode", "analysis-data")
		}
		return filepath.Join(os.TempDir(), "aurumcode-analysis-data")
	}
)

// AnalysisDataReviewKinds are the artifact file kinds a review downloads
// today. The OSV copy (kind "osv") is large and nothing in the review reads
// it yet; its set digest is still verified through the manifest.
var AnalysisDataReviewKinds = []string{"scanners"}

// ApplyAnalysisDataGate resolves the artifact when cfg is declared. An
// unusable outcome is inconclusive (the pipeline applies the mode) and
// carries the reason code;
// a usable one adds no gate line and returns the audit facts.
func ApplyAnalysisDataGate(ctx context.Context, language string, cfg *config.AnalysisDataConfig) (result Result, reason string, audit *facts.AnalysisDataAudit) {
	if !cfg.Declared() {
		return Result{}, "", nil
	}
	out := artifacts.Resolve(ctx, artifacts.Options{
		APIBase:    AnalysisDataAPIBase,
		Repository: cfg.EffectiveRepository(),
		Token:      os.Getenv("GITHUB_TOKEN"),
		CacheDir:   AnalysisDataCache(),
		MaxAgeDays: cfg.EffectiveMaxAgeDays(),
		Now:        AnalysisDataNow,
		Kinds:      AnalysisDataReviewKinds,
	})
	if !out.Usable {
		result.Active = true
		result.Inconclusive = true
		line := "analysis_data: revisão inconclusiva (%s): %s"
		result.addLine(fmt.Sprintf(line, reasons.Text(language, out.Reason), out.Detail), fmt.Sprintf(line, out.Reason, out.Detail))
		return result, out.Reason, nil
	}
	if out.Source == artifacts.SourceCache {
		// Usable, but the operator must see the release listing was
		// unreachable: not a failure, never silent.
		result.Active = true
		result.Lines = append(result.Lines, fmt.Sprintf(
			"analysis_data: usando cópia em cache (%s): a listagem de releases estava indisponível; idade e digests verificados", out.Tag))
	}
	return result, "", &facts.AnalysisDataAudit{
		Digest:      out.Digest,
		GeneratedAt: out.GeneratedAt.UTC().Format(time.RFC3339),
		Tag:         out.Tag,
		Source:      out.Source,
	}
}
