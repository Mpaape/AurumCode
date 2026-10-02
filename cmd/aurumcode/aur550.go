// AUR-550: quality_gates.ssor_dtrack, AUR-519's own breach/inconclusive
// gate semantics applied to an OWASP Dependency-Track v5 server instead of
// a model's own findings. applyDTrackGate runs once, right after
// evaluateGate in both runReview (main.go) and runPRReview (pr.go), and
// folds its outcome into the SAME gateDecision those two already publish
// through gateResult.Lines, writeComplianceArtifacts and
// publishPolicyGateStatus -- this card adds no second gate, no second
// writer, and no change at all to a run that never declares
// quality_gates.ssor_dtrack.enabled: true (cfg.Declared()'s own guard).
//
// It never generates a SBOM (AUR-549's own job): the file at
// cfg.SBOMGenerator.OutputFile is read as-is and uploaded verbatim.
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
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/dtrack"
	"github.com/Mpaape/AurumCode/internal/render"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// dtrackClockNow/dtrackSleeper are this card's own injectable seams for
// TestAUR550...'s fast polling tests. Production code never overrides
// them; the real HTTP transport is never mocked here -- a test points
// cfg.ServerAPIHost at an httptest.Server instead (dtrack.ValidateHost's
// own loopback exception exists exactly for that).
var (
	dtrackClockNow = time.Now
	dtrackSleeper  = time.Sleep
)

// dtrackSecretLookup/dtrackReadBOM abstract os.Getenv/os.ReadFile so a
// test can supply a fake environment and a fixture SBOM without touching
// the real process environment or filesystem lookup rules.
var (
	dtrackSecretLookup = os.Getenv
	dtrackReadBOM       = os.ReadFile
)

// Stable, non-server-authored reason tokens this function itself can add
// on top of internal/dtrack's own (dtrack.Reason*): a missing secret or
// an unreadable/misconfigured SBOM file is a local configuration problem,
// never a server response, but it must be just as inconclusive, never a
// silent approval.
const (
	gateReasonDTrackSecretMissing = "dtrack_secret_missing"
	gateReasonDTrackSBOMUnavailable = "dtrack_sbom_unavailable"
	gateReasonDTrackInvalidHost    = "dtrack_invalid_host"
)

// applyDTrackGate evaluates quality_gates.ssor_dtrack when cfg.Declared()
// (enabled: true); it is a complete no-op (returns a zero gateDecision,
// an empty reason and the SAME filter pointer) otherwise.
//
// inconclusiveMode is the SAME GateConfig.InconclusiveMode() the caller
// already computed for its own skill-based gate -- "block" fails the
// check outright on any dtrack inconclusive result, "warn"/"" (the same
// non-blocking default evaluateGate itself uses) only marks it
// Inconclusive. A breach is never downgraded by this mode: exactly like
// evaluateGate's own threshold loop, a real breach sets Fail
// unconditionally.
func applyDTrackGate(ctx context.Context, cfg config.DTrackGateConfig, inconclusiveMode string, filter *redaction.Filter) (result gateDecision, reason string, newFilter *redaction.Filter) {
	newFilter = filter
	if !cfg.Declared() {
		return gateDecision{}, "", filter
	}
	result.Active = true
	blockOnInconclusive := inconclusiveMode == "block"

	apiKey := dtrackSecretLookup(cfg.APIKeySecret)
	projectID := dtrackSecretLookup(cfg.ProjectIDSecret)
	if apiKey != "" {
		// AC-004: registered as an exact-value secret the instant it is
		// known, before this function (or its caller) can write a single
		// further byte that might carry it.
		newFilter = redaction.NewFilter(os.Getenv(redaction.CanaryEnv), apiKey)
	}
	if apiKey == "" || projectID == "" {
		result.Inconclusive = true
		result.Fail = blockOnInconclusive
		reason = gateReasonDTrackSecretMissing
		result.Lines = append(result.Lines, fmt.Sprintf(
			"ssor_dtrack: variável de ambiente %q (api_key_secret) ou %q (project_id_secret) não definida",
			cfg.APIKeySecret, cfg.ProjectIDSecret,
		))
		return result, reason, newFilter
	}

	bom, err := dtrackReadBOM(cfg.SBOMGenerator.OutputFile)
	if err != nil || len(bom) == 0 {
		result.Inconclusive = true
		result.Fail = blockOnInconclusive
		reason = gateReasonDTrackSBOMUnavailable
		result.Lines = append(result.Lines, fmt.Sprintf(
			"ssor_dtrack: SBOM em sbom_generator.output_file %q não pôde ser lido", cfg.SBOMGenerator.OutputFile,
		))
		return result, reason, newFilter
	}

	client, err := dtrack.NewClient(cfg.ServerAPIHost, apiKey)
	if err != nil {
		result.Inconclusive = true
		result.Fail = blockOnInconclusive
		reason = gateReasonDTrackInvalidHost
		result.Lines = append(result.Lines, "ssor_dtrack: server_api_host inválido")
		return result, reason, newFilter
	}
	client = client.WithClock(dtrackClockNow, dtrackSleeper)

	timeout := time.Duration(cfg.EffectiveTimeoutSeconds()) * time.Second
	interval := time.Duration(cfg.EffectivePollIntervalSeconds()) * time.Second
	outcome := dtrack.Run(ctx, client, projectID, bom, cfg.Thresholds.AsClientThresholds(), interval, timeout)

	switch {
	case outcome.Inconclusive:
		result.Inconclusive = true
		result.Fail = blockOnInconclusive
		reason = outcome.InconclusiveReason
		result.Lines = append(result.Lines, fmt.Sprintf("ssor_dtrack: revisão inconclusiva (%s)", outcome.InconclusiveReason))
	case outcome.Breach:
		result.Breach = true
		result.Fail = true
		for _, r := range outcome.Reasons {
			result.Lines = append(result.Lines, "ssor_dtrack: "+r)
		}
		result.BlockingFindings = append(result.BlockingFindings, render.AuditFinding{
			RuleID:   "ssor_dtrack",
			Path:     projectID,
			Severity: "error",
		})
	default:
		result.Lines = append(result.Lines, fmt.Sprintf(
			"ssor_dtrack: aprovado (critical=%d, high=%d, policy_violations=%d)",
			outcome.Critical, outcome.High, outcome.PolicyViolationsTotal,
		))
	}
	return result, reason, newFilter
}

// mergeDTrackGate folds dtrackResult into gateResult using the exact same
// fields evaluateGate's own caller already reads (Active/Fail/Breach/
// Inconclusive/Lines/BlockingFindings), and combines dtrackReason with an
// already-set gateInconclusiveReason by joining with a comma -- never
// replacing it -- so AUR-537's own single-reason assertions (e.g.
// "provider_failure") stay intact when ssor_dtrack is not declared, and a
// run where both fire publishes both reasons.
func mergeDTrackGate(gateResult gateDecision, gateInconclusiveReason string, dtrackResult gateDecision, dtrackReason string) (gateDecision, string) {
	if !dtrackResult.Active {
		return gateResult, gateInconclusiveReason
	}
	gateResult.Active = true
	gateResult.Fail = gateResult.Fail || dtrackResult.Fail
	gateResult.Breach = gateResult.Breach || dtrackResult.Breach
	gateResult.Inconclusive = gateResult.Inconclusive || dtrackResult.Inconclusive
	gateResult.Lines = append(gateResult.Lines, dtrackResult.Lines...)
	gateResult.BlockingFindings = append(gateResult.BlockingFindings, dtrackResult.BlockingFindings...)
	gateResult.AppliedExceptions = append(gateResult.AppliedExceptions, dtrackResult.AppliedExceptions...)
	if dtrackReason != "" {
		if gateInconclusiveReason != "" {
			gateInconclusiveReason += "," + dtrackReason
		} else {
			gateInconclusiveReason = dtrackReason
		}
	}
	return gateResult, gateInconclusiveReason
}

// wrapWriterWithFilter wraps dst with a second redaction.Writer layer
// using filter, so every write through the returned io.Writer is
// redacted by filter (which may carry a secret dst's own, earlier writer
// does not yet know about) before reaching dst itself. It returns dst
// unchanged (and a nil *redaction.Writer) if the wrap fails -- an
// unauthorized sink name or a nil dst, neither of which should ever
// happen with the two canonical sinks this card uses -- so the caller
// can Flush the returned *redaction.Writer before returning, when it is
// not nil.
func wrapWriterWithFilter(sink redaction.Sink, dst io.Writer, filter *redaction.Filter) (io.Writer, *redaction.Writer) {
	w, err := filter.NewWriter(sink, dst)
	if err != nil {
		return dst, nil
	}
	return w, w
}
