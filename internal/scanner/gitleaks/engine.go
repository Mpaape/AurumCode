// Package gitleaks is the secrets engine of the scanner registry: gitleaks
// over the reviewed commit range, not the final tree. A credential committed
// in one commit of a pull request and removed in a later one has already
// leaked (it is in the history the merge publishes), so the engine runs
// `gitleaks git --log-opts=<base>..<head>` and never falls back to a
// whole-tree scan when the range is missing.
//
// The secret itself never leaves this package: the report is decoded into
// a struct that has no field for gitleaks' Secret, Match, Line, commit
// message, author or e-mail, and gitleaks is asked to redact them anyway.
// A finding carries the rule id, the file, the line, the commit and the
// rule's own description from the pinned rule base.
package gitleaks

import (
	"context"
	"fmt"
	"sort"

	"github.com/Mpaape/AurumCode/internal/scanner"
)

const (
	// Name is the engine's registered name and its typed origin.
	Name = "gitleaks"
	// Category is the gate source every secrets engine answers to.
	Category = "secrets"
	// binary is the executable looked up on PATH.
	binary = "gitleaks"
	// PinnedVersion is the only gitleaks version whose bundled rule base is
	// the pinned one (.board/bootstrap/locks/scanners.yml,
	// secrets_scanner_version). Another version is a failed scan.
	PinnedVersion = "v8.30.1"
	// RulebaseID and RulebaseSHA256 name the rule base bundled in
	// PinnedVersion (secrets_rulebase_id, secrets_rulebase_sha256).
	RulebaseID     = "gitleaks-default-config-v8.30.1"
	RulebaseSHA256 = "sha256:e163e53b9e7e8a8511e77271e2b323ed057759542a6d988258afe3a1fa329caf"
	// RulePrefix prefixes every gitleaks finding's rule id.
	RulePrefix = "gitleaks:"
	// RuleIgnoreFilePresent is the finding a committed .gitleaksignore
	// becomes under a central policy (see ignoreFileFinding).
	RuleIgnoreFilePresent = RulePrefix + "ignore-file-present"
	// severity: gitleaks has no native severity; every leak is an error and
	// the scanner entry's fail_on decides what blocks.
	severity = "error"
	// sideRight: a leak is content the reviewed range added.
	sideRight = "RIGHT"
)

func init() {
	scanner.Register(scanner.Engine{
		Scanner:  Engine{},
		Category: Category,
		Origin:   Name,
		Validate: validateOptions,
		// git reads a checkout owned by another user only through the CI
		// job's safe.directory entry; no other git configuration passes.
		Environment: scanner.Environment{Extra: scanner.GitSafeDirectories},
	})
}

// Engine runs gitleaks.
type Engine struct{}

// Name is the engine's registered name.
func (Engine) Name() string { return Name }

// Identity is the engine identity recorded in Report.Version: the binary
// version and the digest of the rule base it bundles. It changes whenever
// either changes, so a result digest that includes it never reuses a
// verdict computed with another rule base.
func Identity(version string) string {
	return fmt.Sprintf("gitleaks %s rulebase %s %s", version, RulebaseID, RulebaseSHA256)
}

// Run scans the commits of req.Range in the repository at req.Root. Under
// a central policy (TrustPolicy) inline `gitleaks:allow` comments do not
// suppress a finding and a .gitleaksignore at the root is itself a
// finding. Any failure is an error for the executor's single inconclusive
// rule; a missing binary keeps exec.ErrNotFound.
func (Engine) Run(ctx context.Context, req scanner.Request) (scanner.Report, error) {
	run := req.Command
	if run == nil {
		return scanner.Report{}, fmt.Errorf("gitleaks: nil command runner")
	}
	version, err := binaryVersion(ctx, run, req.Root)
	if err != nil {
		return scanner.Report{}, err
	}
	if err := verifyRange(ctx, run, req.Root, req.Range); err != nil {
		return scanner.Report{}, err
	}
	policy := req.Trust == scanner.TrustPolicy
	findings, err := scanRange(ctx, run, req.Root, req.Range, policy, version)
	if err != nil {
		return scanner.Report{}, err
	}
	if policy {
		if f, present := ignoreFileFinding(req.Root); present {
			findings = append(findings, f)
		}
	}
	sortFindings(findings)
	return scanner.Report{Findings: findings, Complete: true, Version: Identity(version)}, nil
}

// validateOptions: the engine takes no option. The rule base is the pinned
// one; a repository cannot select another through the configuration.
func validateOptions(opts scanner.Options) error {
	for key := range opts {
		return fmt.Errorf("unknown option %q (gitleaks takes no option)", key)
	}
	return nil
}

func sortFindings(findings []scanner.Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.RuleID != b.RuleID {
			return a.RuleID < b.RuleID
		}
		return a.Message < b.Message
	})
}
