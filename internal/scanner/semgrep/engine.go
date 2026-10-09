package semgrep

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/scanner"
)

const (
	// Name is the engine's registered name.
	Name = "semgrep"
	// Category is the gate source every SAST engine answers to.
	Category = "sast"
	// binary is the executable looked up on PATH.
	binary = "semgrep"
	// OptionRulePacks is the option naming the rule packs to scan with.
	OptionRulePacks = "rule_packs"
	// sideRight: a whole-tree scan has no removed-line concept.
	sideRight = "RIGHT"
)

// DefaultRulePacks is the default rule-pack selection when rule_packs is
// absent or empty.
var DefaultRulePacks = []string{"p/security-audit", "p/owasp-top-ten"}

func init() {
	scanner.Register(scanner.Engine{
		Scanner:  Engine{},
		Category: Category,
		// The legacy origin label keeps every existing gate line, audit
		// and SARIF byte-identical.
		Origin:   Category,
		Validate: validateOptions,
		// git reads the reviewed range for the change scope; in CI the
		// checkout belongs to another user, so only its safe.directory
		// entries reach the child.
		Environment: scanner.Environment{Extra: scanner.GitSafeDirectories},
	})
}

// Engine runs Semgrep.
type Engine struct{}

// Name is the engine's registered name.
func (Engine) Name() string { return Name }

// Run scans req.Root and keeps the findings on the lines req.Range added:
// the tree is scanned whole (a rule may need the surrounding files), but a
// finding on a line the pull request did not add is the repository's history
// and never judges this change, nor does a line of a path the review's
// `ignore` leaves out (req.Ignored). Under a central policy (TrustPolicy)
// the author's `# nosemgrep` comments and committed `.semgrepignore` files
// are ignored.
func (Engine) Run(ctx context.Context, req scanner.Request) (scanner.Report, error) {
	packs, err := RulePacks(req.Options)
	if err != nil {
		return scanner.Report{}, err
	}
	if req.Command == nil {
		return scanner.Report{}, errors.New("semgrep: nil command runner")
	}
	added, err := scanner.AddedLines(ctx, req.Command, req.Root, req.Range)
	if err != nil {
		return scanner.Report{}, fmt.Errorf("semgrep: %w", err)
	}
	added = added.Without(req.Ignored)
	findings, err := scan(ctx, req.Root, packs, req.Trust == scanner.TrustPolicy, req.Command, added)
	if err != nil {
		return scanner.Report{}, err
	}
	return scanner.Report{Findings: added.Keep(findings), Complete: true}, nil
}

// RulePacks reads the rule_packs option, defaulting to DefaultRulePacks.
func RulePacks(opts scanner.Options) ([]string, error) {
	raw, ok := opts[OptionRulePacks]
	if !ok || raw == nil {
		return append([]string(nil), DefaultRulePacks...), nil
	}
	var packs []string
	switch v := raw.(type) {
	case []string:
		packs = v
	case []any:
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s: %v is not a string", OptionRulePacks, item)
			}
			packs = append(packs, s)
		}
	default:
		return nil, fmt.Errorf("%s: must be a list of strings", OptionRulePacks)
	}
	out := make([]string, 0, len(packs))
	for _, p := range packs {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return append([]string(nil), DefaultRulePacks...), nil
	}
	return out, nil
}

// validateOptions refuses an unknown option and a rule pack that reads as a
// command-line flag.
func validateOptions(opts scanner.Options) error {
	for key := range opts {
		if key != OptionRulePacks {
			return fmt.Errorf("unknown option %q (accepted: %s)", key, OptionRulePacks)
		}
	}
	packs, err := RulePacks(opts)
	if err != nil {
		return err
	}
	for _, p := range packs {
		if strings.HasPrefix(p, "-") {
			return fmt.Errorf("%s: %q looks like a flag, not a rule pack", OptionRulePacks, p)
		}
	}
	return nil
}
