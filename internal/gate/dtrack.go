// AUR-550: quality_gates.ssor_dtrack, AUR-519's own breach/inconclusive
// gate semantics applied to an OWASP Dependency-Track v5 server instead of
// a model's own findings. ApplyDTrackGate runs once, right after
// EvaluateGate in both runReview (main.go) and runPRReview (pr.go), and
// folds its outcome into the SAME Result those two already publish
// through gateResult.Lines, writeComplianceArtifacts and
// publishPolicyGateStatus -- this card adds no second gate, no second
// writer, and no change at all to a run that never declares
// quality_gates.ssor_dtrack.enabled: true (cfg.Declared()'s own guard).
//
// It never generates a SBOM (AUR-549's own job): the file at
// cfg.SBOMOutputFile() (AUR-549's own sbom_generator.output_file, nested
// under this card's own ssor_dtrack section) is read as-is and uploaded
// verbatim.
//
// The API key (read from the environment variable NAMED by
// cfg.APIKeySecret -- never a literal) must never reach stdout, stderr,
// the published review body, the audit record or SARIF (AC-004). This
// function registers it as an exact-value secret in a freshly built
// redaction.Filter (the existing filter is a value type with no mutator;
// rebuilding is the only way to add a secret to it) and returns that new
// filter so the caller can keep using it for every write from this point
// on, including a second redaction.Writer layer wrapped around the
// caller's own stdout/stderr -- see the call sites in main.go/pr.go.
package gate

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/dtrack"
	"github.com/Mpaape/AurumCode/internal/gate/facts"
	"github.com/Mpaape/AurumCode/internal/gate/reasons"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// DTrackClockNow/DTrackSleeper are this card's own injectable seams for
// TestAUR550...'s fast polling tests. Production code never overrides
// them; the real HTTP transport is never mocked here -- a test points
// cfg.ServerAPIHost at an httptest.Server instead (dtrack.ValidateHost's
// own loopback exception exists exactly for that).
var (
	DTrackClockNow = time.Now
	DTrackSleeper  = time.Sleep
)

// DTrackSecretLookup/DTrackReadBOM abstract os.Getenv/os.ReadFile so a
// test can supply a fake environment and a fixture SBOM without touching
// the real process environment or filesystem lookup rules.
var (
	DTrackSecretLookup = os.Getenv
	DTrackReadBOM      = os.ReadFile
)

// Stable, non-server-authored reason tokens this function itself can add
// on top of internal/dtrack's own (dtrack.Reason*): a missing secret or
// an unreadable/misconfigured SBOM file is a local configuration problem,
// never a server response, but it must be just as inconclusive, never a
// silent approval.
const (
	ReasonDTrackSecretMissing   = "dtrack_secret_missing"
	ReasonDTrackSBOMUnavailable = "dtrack_sbom_unavailable"
	ReasonDTrackInvalidHost     = "dtrack_invalid_host"
)

// ApplyDTrackGate evaluates quality_gates.ssor_dtrack when cfg.Declared()
// (enabled: true); it is a complete no-op (returns a zero Result,
// an empty reason and the SAME filter pointer) otherwise.
//
// Every inconclusive outcome only marks the result Inconclusive; whether it
// fails is decided once, by the pipeline (ApplyInconclusiveMode). A breach
// is never downgraded: a real breach sets Fail unconditionally.
func ApplyDTrackGate(ctx context.Context, language string, cfg *config.SsorDtrackConfig, filter *redaction.Filter) (result Result, reason string, newFilter *redaction.Filter) {
	newFilter = filter
	if !cfg.Declared() {
		return Result{}, "", filter
	}
	result.Active = true

	apiKey := DTrackSecretLookup(cfg.APIKeySecret)
	projectID := DTrackSecretLookup(cfg.ProjectIDSecret)
	if apiKey != "" {
		// AC-004: registered as an exact-value secret the instant it is
		// known, before this function (or its caller) can write a single
		// further byte that might carry it.
		newFilter = redaction.NewFilter(os.Getenv(redaction.CanaryEnv), apiKey)
	}
	if apiKey == "" || projectID == "" {
		result.Inconclusive = true
		reason = ReasonDTrackSecretMissing
		result.Lines = append(result.Lines, fmt.Sprintf(
			"ssor_dtrack: revisão inconclusiva (%s): variável de ambiente %q (api_key_secret) ou %q (project_id_secret) não definida",
			reasons.Text(language, reason), cfg.APIKeySecret, cfg.ProjectIDSecret,
		))
		return result, reason, newFilter
	}

	bomPath := cfg.SBOMOutputFile()
	bom, err := DTrackReadBOM(bomPath)
	if bomPath == "" || err != nil || len(bom) == 0 {
		result.Inconclusive = true
		reason = ReasonDTrackSBOMUnavailable
		result.Lines = append(result.Lines, fmt.Sprintf(
			"ssor_dtrack: revisão inconclusiva (%s): SBOM em sbom_generator.output_file %q não pôde ser lido", reasons.Text(language, reason), bomPath,
		))
		return result, reason, newFilter
	}

	client, err := dtrack.NewClient(cfg.ServerAPIHost, apiKey)
	if err != nil {
		result.Inconclusive = true
		reason = ReasonDTrackInvalidHost
		result.Lines = append(result.Lines, fmt.Sprintf("ssor_dtrack: revisão inconclusiva (%s): server_api_host inválido", reasons.Text(language, reason)))
		return result, reason, newFilter
	}
	client = client.WithClock(DTrackClockNow, DTrackSleeper)

	timeout := time.Duration(cfg.EffectiveTimeoutSeconds()) * time.Second
	interval := time.Duration(cfg.EffectivePollIntervalSeconds()) * time.Second
	outcome := dtrack.Run(ctx, client, projectID, bom, clientThresholds(cfg.Thresholds), interval, timeout)

	for _, note := range outcome.Notes {
		result.Lines = append(result.Lines, "ssor_dtrack: "+note+": recálculo de métricas não permitido à chave; seguiu lendo até assentar")
	}
	switch {
	case outcome.Inconclusive:
		result.Inconclusive = true
		reason = outcome.InconclusiveReason
		result.Lines = append(result.Lines, fmt.Sprintf("ssor_dtrack: revisão inconclusiva (%s)", reasons.Text(language, outcome.InconclusiveReason)))
	case outcome.Breach:
		result.Breach = true
		result.Fail = true
		for _, r := range outcome.Reasons {
			result.Lines = append(result.Lines, "ssor_dtrack: "+r+" (origem "+OriginDTrack+")")
		}
		result.BlockingFindings = append(result.BlockingFindings, facts.AuditFinding{
			RuleID:   "ssor_dtrack",
			Path:     projectID,
			Severity: "error",
			Origin:   OriginDTrack,
		})
	default:
		result.Lines = append(result.Lines, fmt.Sprintf(
			"ssor_dtrack: aprovado (critical=%d, high=%d, policy_violations=%d)",
			outcome.Critical, outcome.High, outcome.PolicyViolationsTotal,
		))
	}
	return result, reason, newFilter
}

// WrapWriterWithFilter wraps dst with a second redaction.Writer layer
// using filter, so every write through the returned io.Writer is
// redacted by filter (which may carry a secret dst's own, earlier writer
// does not yet know about) before reaching dst itself. It returns dst
// unchanged (and a nil *redaction.Writer) if the wrap fails -- an
// unauthorized sink name or a nil dst, neither of which should ever
// happen with the two canonical sinks this card uses -- so the caller
// can Flush the returned *redaction.Writer before returning, when it is
// not nil.
func WrapWriterWithFilter(sink redaction.Sink, dst io.Writer, filter *redaction.Filter) (io.Writer, *redaction.Writer) {
	w, err := filter.NewWriter(sink, dst)
	if err != nil {
		return dst, nil
	}
	return w, w
}

// clientThresholds converts the configured thresholds to the client's own
// shape, so the configuration never depends on the client that applies it.
func clientThresholds(t config.SsorDtrackThresholds) dtrack.Thresholds {
	return dtrack.Thresholds{
		MaxCritical:      t.MaxCritical,
		MaxHigh:          t.MaxHigh,
		PolicyViolations: t.PolicyViolations,
	}
}
