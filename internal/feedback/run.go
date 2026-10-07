package feedback

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

var repoRE = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// ValidRepo reports whether s is an "owner/name" repository id.
func ValidRepo(s string) bool { return repoRE.MatchString(s) && !strings.Contains(s, "..") }

// Options is one run of the loop.
type Options struct {
	// GitHub reads the signals; PolicyGitHub (nil: GitHub) writes the
	// policy repository, so the read token never needs write access.
	GitHub       *GitHub
	PolicyGitHub *GitHub
	Repos        []string
	Policy       string
	Since        string
	Provider     llm.Provider
	Filter       *redaction.Filter
	// Before and After are the optional corpus reports for the body.
	Before, After *CorpusReport
	// Publish false builds the plan and writes nothing.
	Publish bool
}

// Result is what a run did.
type Result struct {
	Signals     int
	Fresh       int
	Notes       []string
	Plan        Plan
	Planned     bool
	PullRequest int
}

// Collect gathers the signals of every repository, deduplicated.
func Collect(o Options) ([]Signal, []Rejected, []string, error) {
	var all []Signal
	var rejected []Rejected
	var notes []string
	for _, repo := range o.Repos {
		if !ValidRepo(repo) {
			return nil, nil, nil, fmt.Errorf("repositório inválido %q (use owner/nome)", repo)
		}
		alerts, ok, err := o.GitHub.DismissedAlerts(repo)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("%s: alertas: %w", repo, err)
		}
		if !ok {
			notes = append(notes, repo+": code scanning indisponível; sem sinais de falso positivo")
		}
		all = append(all, FalsePositives(o.Filter, repo, alerts)...)
		runs, err := o.GitHub.AuditRuns(repo)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("%s: auditoria: %w", repo, err)
		}
		fixed, err := TruePositives(o.Filter, repo, runs, o.GitHub.ChangedLines)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("%s: diff entre auditorias: %w", repo, err)
		}
		all = append(all, fixed...)
		comments, err := o.GitHub.Comments(repo, o.Since)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("%s: comentários: %w", repo, err)
		}
		lost, rej, err := Escaped(o.Filter, repo, comments, o.GitHub.PullHead)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("%s: /aurum perdeu: %w", repo, err)
		}
		all = append(all, lost...)
		rejected = append(rejected, rej...)
	}
	return Dedupe(all), rejected, notes, nil
}

// Run collects, filters what the policy repository already carries, asks
// the model for proposals and opens (or updates) the single feedback pull
// request. Without fresh signals it writes nothing.
func Run(o Options) (Result, error) {
	if !ValidRepo(o.Policy) {
		return Result{}, fmt.Errorf("repositório da política inválido %q", o.Policy)
	}
	if o.Filter == nil {
		o.Filter = redaction.NewFilter()
	}
	signals, rejected, notes, err := Collect(o)
	if err != nil {
		return Result{}, err
	}
	res := Result{Signals: len(signals), Notes: notes}
	writer := o.PolicyGitHub
	if writer == nil {
		writer = o.GitHub
	}
	policy := PolicyRepo{GitHub: writer, Repo: o.Policy}
	base, err := policy.DefaultBranch()
	if err != nil {
		return res, err
	}
	number, open, known, err := proposedBefore(policy)
	if err != nil {
		return res, err
	}
	ref := base
	if open {
		ref = Branch
	}
	ledger, err := readLedger(policy, base)
	if err != nil {
		return res, err
	}
	branchLedger := Ledger{}.With(known)
	fresh := Fresh(signals, ledger, branchLedger)
	res.Fresh = len(fresh)
	if len(fresh) == 0 {
		return res, nil
	}
	skills, err := policySkills(policy, ref)
	if err != nil {
		return res, err
	}
	proposals, discarded, err := Propose(o.Provider, o.Filter, fresh, skills)
	if err != nil {
		return res, err
	}
	plan, ok := BuildPlan(Inputs{
		Signals: fresh, Ledger: ledger.With(branchLedger.toSignals()), Proposals: proposals, Discarded: discarded,
		Rejected: rejected, Skills: skills, Comparison: Compare(o.Before, o.After),
	})
	res.Plan, res.Planned = plan, ok
	if !ok || !o.Publish {
		return res, nil
	}
	res.PullRequest, err = policy.Publish(plan, base, number, open)
	return res, err
}

// proposedBefore finds the open feedback pull request, if any, and every
// signal id any feedback pull request (open, merged or rejected) already
// carried, read from the ledger at its head commit.
func proposedBefore(p PolicyRepo) (number int, open bool, known []Signal, err error) {
	pulls, err := p.Pulls()
	if err != nil {
		return 0, false, nil, err
	}
	for _, pr := range pulls {
		ref := pr.Head.SHA
		if pr.State == "open" {
			number, open, ref = pr.Number, true, Branch
		}
		if ref == "" {
			continue
		}
		l, err := readLedger(p, ref)
		if err != nil {
			return 0, false, nil, err
		}
		known = append(known, l.toSignals()...)
	}
	return number, open, known, nil
}

func (l Ledger) toSignals() []Signal {
	out := make([]Signal, 0, len(l.Signals))
	for _, id := range l.Signals {
		out = append(out, Signal{ID: id})
	}
	return out
}

func readLedger(p PolicyRepo, ref string) (Ledger, error) {
	content, _, found, err := p.File(LedgerPath, ref)
	if err != nil {
		return Ledger{}, err
	}
	return ParseLedger([]byte(content), found)
}

// policySkills reads the skills the policy's config.yml declares.
func policySkills(p PolicyRepo, ref string) (map[string]string, error) {
	raw, _, found, err := p.File(config.DefaultConfigPath, ref)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%s sem %s: não há política para propor", p.Repo, config.DefaultConfigPath)
	}
	cfg, err := config.Parse([]byte(raw), p.Repo+"/"+config.DefaultConfigPath)
	if err != nil {
		return nil, err
	}
	skills := map[string]string{}
	for _, path := range cfg.Review.Context.Skills {
		path = strings.TrimSpace(path)
		content, _, ok, err := p.File(path, ref)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("skill declarada %s não existe em %s", path, p.Repo)
		}
		skills[path] = content
	}
	return skills, nil
}
