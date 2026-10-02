// AUR-548: Semgrep, a multi-language SAST pass, runs over the WHOLE
// reviewed tree (never just the diff) and its findings are deterministic
// evidence the model can explain but never remove or downgrade (AC-005):
// they are produced entirely by Go code, from Semgrep's own JSON report,
// strictly AFTER the model call -- the model's JSON response is never
// consulted to decide whether a Semgrep finding exists or what severity it
// carries, and runSASTPass never reads result.Issues.
//
// quality_gates.sast's own gate decision is DELIBERATELY independent of
// AUR-519's evaluateGate/GateConfig (policygate.go): a repository or
// policy can enable SAST with no `gate:` section declared at all, but
// evaluateGate returns Active=false the instant gate.Declared() is false,
// and its threshold loop only ever runs when gate.fail_on is set --
// neither condition has anything to do with quality_gates.sast. Rather
// than overload evaluateGate's signature (which every existing AUR-519/
// 520/521/537/538/543 test already calls, and which sibling cards
// AUR-549/550 touch concurrently), applySASTGate folds its own,
// independently computed decision into the SAME gateDecision struct
// evaluateGate produces, in place. From publishPolicyGateStatus's,
// writeComplianceArtifacts' and the exit-code section's point of view
// (main.go/pr.go) there is exactly one gate decision for this run, now
// possibly informed by two independent, additive sources -- no other file
// needs to change to learn about a SAST breach.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mpaape/AurumCode/internal/analysis"
	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/render"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// sastTimeout bounds one Semgrep invocation: generous enough for a real
// registry-pack scan in CI, short enough that a hung or misbehaving
// executable never stalls a run (or a test).
const sastTimeout = 120 * time.Second

// sastReasonUnavailable/sastReasonError/sastReasonInvalid are AUR-548's
// own stable, non-model-authored inconclusive-reason tokens -- the same
// kind of token gateReasonProviderFailure (aur537.go) and this file's
// sibling "degraded_parse"/"partial_coverage" tokens (main.go) already
// are. AC-003 treats all three Semgrep failure modes identically (the
// policy's own gate.inconclusive decides what happens next), but distinct
// tokens keep the published reason line and the audit record specific
// about which failure occurred: the binary was never found, it ran and
// failed, or it ran and produced something that was not a trustworthy
// Semgrep report.
const (
	sastReasonUnavailable = "sast_unavailable"
	sastReasonError       = "sast_execution_error"
	sastReasonInvalid     = "sast_invalid_output"
	// sastReasonUnverifiedCheckout is --pr's own reason (pr.go): the local
	// checkout is not verified as the pull request's own head
	// (codebaseContextMismatch/verifiedCleanCheckoutReason, AUR-515/536
	// -- a different repository, a divergent HEAD, an unclean tree, or
	// simply unverifiable). Semgrep is never invoked in this case: a
	// stale or unrelated checkout must never be scanned under the
	// reviewed pull request's name.
	sastReasonUnverifiedCheckout = "sast_unverified_checkout"
)

// realSemgrepRunner execs the "semgrep" binary from PATH -- the one
// production implementation of this seam. It reads no provider-contributed
// text and decides nothing from the reviewed repository's own content; a
// scan is always `semgrep scan --json --config <pack>... .` inside dir.
func realSemgrepRunner(ctx context.Context, dir string, args ...string) (stdout, stderr string, err error) {
	cmd := exec.CommandContext(ctx, "semgrep", args...)
	cmd.Dir = dir
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

// runSASTPass is runReview's (and, wiring permitting, runPRReview's)
// shared entry point for quality_gates.sast. When sast.IsEnabled() is
// false it is a complete no-op (AC-004: nothing changes without an
// explicit "enabled: true"). Otherwise it runs Semgrep over root through
// run (realSemgrepRunner in production; a fake executable on PATH in
// tests, injected here exactly like analysis.Runner.Vet's own
// commandRunner) and converts its report into gate-ready issues.
//
// reason is "" on a trustworthy scan -- including a genuinely clean one
// with zero findings -- or one of this file's sastReason* tokens
// otherwise; issues is always nil on a failed scan, never a partial or
// substitute finding set (AUR-548/MUT-001: treating a failed execution as
// zero findings is exactly the defect this split guards against -- a
// caller that forgets to check reason and only looks at len(issues)==0
// cannot tell "clean" apart from "failed" by construction, since issues is
// nil in both the clean-zero-findings and the failed cases; checking
// reason is mandatory and is exactly what applySASTGate does).
//
// policyOrigin is true when this run's effective quality_gates.sast
// section came from a central policy (sastOrigin == gateOriginPolicy at
// the call site): the scanned tree is then the pull request AUTHOR's own
// content, and two author-controlled bypasses must not be able to silence
// a policy-mandated finding --
//   - a `# nosemgrep` comment: countered by passing Semgrep's own
//     --disable-nosem flag (analysis.Runner.Semgrep);
//   - a repository-committed .semgrepignore: countered by scanning a
//     sanitized COPY of root with that file stripped (sastScanRoot,
//     below) instead of root itself, since the production checkout is
//     mounted read-only and this file cannot simply be deleted in place.
//
// filter redacts every Semgrep-sourced Message before it is ever
// published (Semgrep's own fixed rule text is normally safe, but a rule's
// message can echo configuration/metadata the redaction filter's own
// canary patterns are meant to catch, and this pass is held to the same
// bar as every other publication sink in this codebase).
func runSASTPass(ctx context.Context, root string, sast *config.SastConfig, policyOrigin bool, filter *redaction.Filter, run func(ctx context.Context, dir string, args ...string) (stdout, stderr string, err error)) (issues []types.ReviewIssue, reason string) {
	if !sast.IsEnabled() {
		return nil, ""
	}
	scanCtx, cancel := context.WithTimeout(ctx, sastTimeout)
	defer cancel()

	scanRoot, cleanup, sanitizeErr := sastScanRoot(root, policyOrigin)
	if sanitizeErr == nil {
		defer cleanup()
	} else {
		// Best-effort: a filesystem problem preparing the sanitized copy
		// falls back to scanning root as-is rather than failing the whole
		// pass closed on an unrelated I/O error -- --disable-nosem below
		// still applies regardless, which is the more common bypass.
		scanRoot = root
	}

	findings, err := analysis.NewRunner().Semgrep(scanCtx, scanRoot, sast.Packs(), policyOrigin, run)
	if err != nil {
		switch {
		case errors.Is(err, exec.ErrNotFound):
			return nil, sastReasonUnavailable
		case errors.Is(err, analysis.ErrSemgrepNoResults), errors.Is(err, analysis.ErrSemgrepReportedErrors), strings.Contains(err.Error(), "invalid JSON"):
			return nil, sastReasonInvalid
		default:
			return nil, sastReasonError
		}
	}
	issues = make([]types.ReviewIssue, 0, len(findings))
	for _, f := range findings {
		message := fmt.Sprintf("%s (rule %s)", f.Message, f.RuleID)
		if filter != nil {
			message = filter.Redact(message)
		}
		issues = append(issues, types.ReviewIssue{
			File:     f.Path,
			Line:     f.Line,
			Side:     f.Side,
			Severity: f.Severity,
			RuleID:   f.RuleID,
			Message:  message,
		})
	}
	return issues, ""
}

// sastScanRoot returns the directory Semgrep should actually scan.
// policyOrigin == false (a repository's own opt-in, scanning its own
// tree) always scans root directly: there is no author/policy trust
// boundary to defend there. policyOrigin == true scans root directly too,
// UNLESS root contains a top-level ".semgrepignore" file, in which case it
// copies root into a fresh temporary directory with that file (and ".git",
// irrelevant to a content scan) excluded, so a repository-committed
// .semgrepignore can never hide a policy-mandated finding. The returned
// cleanup removes that temporary directory; it is a no-op when no copy was
// made. A non-nil error means no copy was attempted or completed, and the
// caller falls back to scanning root unchanged.
func sastScanRoot(root string, policyOrigin bool) (scanRoot string, cleanup func(), err error) {
	noop := func() {}
	if !policyOrigin {
		return root, noop, nil
	}
	if _, statErr := os.Stat(filepath.Join(root, ".semgrepignore")); statErr != nil {
		return root, noop, nil
	}
	tmp, mkErr := os.MkdirTemp("", "aurumcode-sast-*")
	if mkErr != nil {
		return root, noop, mkErr
	}
	if cpErr := copyTreeExcluding(root, tmp, map[string]bool{".semgrepignore": true, ".git": true}); cpErr != nil {
		_ = os.RemoveAll(tmp)
		return root, noop, cpErr
	}
	return tmp, func() { _ = os.RemoveAll(tmp) }, nil
}

// copyTreeExcluding recursively copies src into dst, skipping any file or
// directory whose base name is a key of excludeNames. Used only for a
// small, test-sized checkout (see sastScanRoot); it is a plain, dependency
// -free Go walk rather than shelling out to "cp", which may not exist in
// every sealed execution environment this binary runs in.
func copyTreeExcluding(src, dst string, excludeNames map[string]bool) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		if excludeNames[d.Name()] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if mkErr := os.MkdirAll(filepath.Dir(target), 0700); mkErr != nil {
			return mkErr
		}
		return os.WriteFile(target, data, 0600)
	})
}

// sastInconclusiveNotice renders the declared-limitation text runReview/
// runPRReview add to result.Limitations for a Semgrep run that could not
// produce trustworthy findings, mirroring modelInvalidOutputNotice's
// (passes.go) shape and placement: a Limitations entry, never a finding.
func sastInconclusiveNotice(language, reason string) string {
	if language == "pt-BR" || language == "pt" {
		return fmt.Sprintf("SAST (Semgrep) inconclusivo: a varredura não produziu resultado confiável (%s); nenhum achado determinístico do Semgrep foi publicado nesta execução.", reason)
	}
	return fmt.Sprintf("SAST (Semgrep) inconclusive: the scan did not produce a trustworthy result (%s); no Semgrep finding was published for this run.", reason)
}

// applySASTGate folds quality_gates.sast's own, independent decision into
// an already-computed gateDecision (evaluateGate's return value,
// policygate.go) IN PLACE. It never calls or changes evaluateGate: SAST's
// Active/Fail/Breach/Inconclusive are computed from sast/issues/reason
// alone, so a config with quality_gates.sast but NO `gate:` section at all
// still publishes a status, withholds approval and writes an audit/SARIF
// record naming the breach -- exactly the publication path AUR-519/520/
// 521 already built, now driven by a second, independent gate. Calling
// this when sast.IsEnabled() is false is a no-op (d is left exactly as
// evaluateGate returned it).
//
//   - sast is the already precedence-resolved SastConfig
//     (repoCfg.QualityGates.Sast, after config.ApplyCentralPolicy already
//     picked policy-over-repo wholesale when a policy is in play -- see
//     that function's own comment: there is only ever one effective SAST
//     section per run, never a repo-and-policy merge, so this function
//     takes no separate "accepted origin" filter).
//   - origin names, for the published line only, whether this run's
//     effective section came from the central policy or the repository's
//     own opt-in (the same test runReview/runPRReview already does for
//     AUR-519's gateOrigin: centralCfg != nil means "policy").
//   - issues is runSASTPass's own, Semgrep-sourced slice ONLY -- never
//     result.Issues or any other slice the model's JSON response could
//     have contributed to -- so a model reply that tries to recite,
//     contradict or omit a Semgrep finding has no bearing on this
//     decision (AC-005: the model may explain, never remove or
//     downgrade a Semgrep finding).
//   - reason is runSASTPass's own inconclusive-reason token ("" for a
//     trustworthy scan, including zero findings).
func applySASTGate(d *gateDecision, sast *config.SastConfig, origin string, issues []types.ReviewIssue, reason string) error {
	if !sast.IsEnabled() {
		return nil
	}
	d.Active = true
	if reason != "" {
		d.Inconclusive = true
		d.Lines = append(d.Lines, fmt.Sprintf("SAST (semgrep, origem %s) inconclusivo (%s)", origin, reason))
		return nil
	}
	rank, name, err := sast.Threshold()
	if err != nil {
		// config.Parse already validates fail_on_severity before any run
		// reaches here; this is defensive only.
		return err
	}
	for _, issue := range issues {
		issueRank, ok := severityRankOf(issue.Severity)
		if !ok || issueRank < rank {
			continue
		}
		d.Fail = true
		d.Breach = true
		d.Lines = append(d.Lines, fmt.Sprintf("%s: %s (severidade %s, limiar %s, origem %s)", issue.RuleID, issue.Message, issue.Severity, name, origin))
		d.BlockingFindings = append(d.BlockingFindings, render.AuditFinding{
			RuleID:   issue.RuleID,
			Path:     issue.File,
			Line:     issue.Line,
			Severity: issue.Severity,
		})
	}
	return nil
}
