// Round convergence of a pull request (internal/review/rounds): a model
// finding new in this round on code the earlier conclusive round already
// reviewed, unchanged since, is published as an observation instead of a
// new block. Security findings keep blocking; what an earlier round blocked
// on keeps blocking; without a trusted earlier round nothing changes.
package main

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/review/rounds"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// roundState is what convergence knows about this round and the earlier one.
type roundState struct {
	// earlier is the round recorded by this product's latest parecer.
	earlier    rounds.Round
	hasEarlier bool
	// head is the pull request head this round reviewed: diffHead, the
	// head read before the diff, when it is still the head after the
	// review; "" otherwise.
	head, diffHead string
	// late counts the findings published as observations by convergence.
	late int
}

// readEarlierRound reads the round marker of the latest parecer published
// by this product's login. Another author's comment is never read, and
// without a readable conversation or a known publisher there is none.
func (p *prReview) readEarlierRound() (rounds.Round, bool) {
	publisher := strings.TrimSpace(p.env().publisherLogin)
	if p.historyErr != nil || publisher == "" {
		return rounds.Round{}, false
	}
	var found rounds.Round
	ok := false
	for _, e := range p.historyEntries {
		if e.Kind != "comment" && e.Kind != "review" {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(e.Author), publisher) || !strings.Contains(e.Body, reviewBodyMarker) {
			continue
		}
		if r, has := rounds.ParseRoundMarker(e.Body); has {
			found, ok = r, true
		}
	}
	return found, ok
}

// convergeRounds resolves the head this round reviewed and, when an earlier
// round is trusted, publishes the late findings as observations. Any doubt
// (no earlier round, unreadable head, comparison refused or not a fast
// forward) leaves every finding as it is.
func (p *prReview) convergeRounds() {
	p.convergence.earlier, p.convergence.hasEarlier = p.readEarlierRound()
	// A head that moved while this round ran was not the one reviewed: it is
	// never recorded, and nothing is demoted against it.
	if head := p.currentHead(); head != "" && head == p.convergence.diffHead {
		p.convergence.head = head
	}
	if !p.convergence.hasEarlier {
		return
	}
	if path := p.trustSurfaceChanged(); path != "" {
		fmt.Fprintf(p.stderr, "aurumcode review: round convergence off: the pull request changes %s, which can publish as the reviewer\n", p.filter.Redact(path))
		return
	}
	if p.convergence.head == "" {
		fmt.Fprintln(p.stderr, "aurumcode review: round convergence off: the pull request head is unknown or moved during the review")
		return
	}
	changes, err := p.changesSinceEarlierRound()
	if err != nil {
		fmt.Fprintf(p.stderr, "aurumcode review: round convergence off: %s\n", p.filter.Redact(err.Error()))
		return
	}
	p.demoteLateFindings(changes)
}

// trustSurfaceChanged names a changed file that could let the pull request
// post as this product's login (a workflow) or change its configuration:
// the earlier round's record is then not trusted.
func (p *prReview) trustSurfaceChanged() string {
	for _, path := range diffPaths(p.diff) {
		clean := strings.TrimPrefix(strings.ToLower(path), "./")
		if strings.HasPrefix(clean, ".github/workflows/") || strings.HasPrefix(clean, ".github/actions/") || strings.HasPrefix(clean, ".aurumcode/") || clean == ".aurumcode.yml" || clean == ".aurumcode.yaml" {
			return path
		}
	}
	return ""
}

// currentHead is the pull request head read from the API, lower case; ""
// when it cannot be read.
func (p *prReview) currentHead() string {
	head, err := resolvePullRequestHeadSHA(p.ctx, p.client, p.owner, p.repoName, p.prNumber)
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(head))
}

// changesSinceEarlierRound is what changed between the earlier round's head
// and this one, refused when the earlier head is not an ancestor.
func (p *prReview) changesSinceEarlierRound() (rounds.Changes, error) {
	cmp, err := p.client.CompareCommits(p.ctx, p.owner, p.repoName, p.convergence.earlier.SHA, p.convergence.head)
	if err != nil {
		return rounds.Changes{}, err
	}
	files := make([]rounds.ChangedFile, 0, len(cmp.Files))
	for _, f := range cmp.Files {
		files = append(files, rounds.ChangedFile{Path: f.Filename, Patch: f.Patch})
	}
	changes, ok := rounds.NewChanges(cmp.Status, files)
	if !ok {
		return rounds.Changes{}, fmt.Errorf("the earlier round's commit %s is not a trusted ancestor of the head (status %q, %d file(s))", shortSHA(p.convergence.earlier.SHA), cmp.Status, len(cmp.Files))
	}
	return changes, nil
}

// demoteLateFindings turns the model's late findings into observations
// that say so. Only the model's own findings are read: deterministic
// evidence and the model's assessment of it follow the triage as before.
func (p *prReview) demoteLateFindings(changes rounds.Changes) {
	earlier := map[string]bool{}
	for _, fp := range p.convergence.earlier.Blocking {
		earlier[fp] = true
	}
	for _, prev := range rounds.Published(earlierComments(p.historyEntries, p.env().publisherLogin)) {
		earlier[prev.Fingerprint] = true
	}
	catalog := review.NewRulesLoader()
	if err := catalog.Load(); err != nil {
		catalog = nil
	}
	var idx []int
	var findings []rounds.Finding
	for i, issue := range p.result.Issues {
		if issue.Origin != "" || issue.Assessment != nil {
			continue
		}
		idx = append(idx, i)
		findings = append(findings, rounds.Finding{
			Fingerprint: roundFingerprint(p.diff, issue, p.filter),
			Path:        issue.File, Line: issue.Line, Left: strings.EqualFold(issue.Side, "LEFT"),
			Blocking: severityBlocks(issue.Severity), Exempt: p.alwaysBlocks(issue, catalog),
			Refs: rounds.Refs(issue.Evidence + "\n" + issue.Message + "\n" + issue.Impact),
		})
	}
	note := i18n.Format(p.reviewLanguage, "review.late_finding", shortSHA(p.convergence.earlier.SHA))
	for k, late := range rounds.LateFindings(findings, earlier, p.convergence.earlier, changes) {
		if !late {
			continue
		}
		issue := &p.result.Issues[idx[k]]
		issue.Severity, issue.Late = "info", true
		issue.Message = note + " " + issue.Message
		p.convergence.late++
	}
	if p.convergence.late > 0 {
		what := countText(p.convergence.late, "1 finding", "%d findings")
		fmt.Fprintf(p.stderr, "aurumcode review: %s new on code unchanged since the earlier round (%s) published as observations\n", what, shortSHA(p.convergence.earlier.SHA))
	}
}

// severityBlocks is the gate-free reading of a blocking severity.
func severityBlocks(severity string) bool {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "error", "warning", "critical":
		return true
	}
	return false
}

// alwaysBlocks names the findings convergence never turns into
// observations: a critical one; a security rule (a rule whose prefix
// before "/" or "#" names security, a catalog or skill rule of the security
// category, a secret or an injection); and every rule of the policy layer.
func (p *prReview) alwaysBlocks(issue types.ReviewIssue, catalog *review.RulesLoader) bool {
	id := strings.ToLower(strings.TrimSpace(issue.RuleID))
	prefix := id
	if i := strings.IndexAny(id, "/#"); i >= 0 {
		prefix = id[:i]
	}
	if strings.EqualFold(strings.TrimSpace(issue.Severity), "critical") || strings.Contains(prefix, "secur") || strings.Contains(prefix, "segur") ||
		strings.Contains(id, "secret") || strings.Contains(id, "segredo") || strings.Contains(id, "injection") || strings.Contains(id, "injec") {
		return true
	}
	if catalog != nil {
		if rule, ok := catalog.Get(issue.RuleID); ok && strings.EqualFold(rule.Category, "security") {
			return true
		}
	}
	if rule, ok := p.dynamicRules[issue.RuleID]; ok {
		if strings.EqualFold(rule.Category, "security") || rule.Origin == gateOriginPolicy {
			return true
		}
	}
	return false
}

// roundMarker records this round for the next one: the head it reviewed
// and the fingerprints it blocked on. A run that did not look everywhere
// carries the earlier round's record forward, so a later round never
// treats code as reviewed that no conclusive round saw.
func (p *prReview) roundMarker() string {
	if !p.conclusiveForRounds() || p.convergence.head == "" {
		if p.convergence.hasEarlier {
			return rounds.RoundMarker(p.convergence.earlier)
		}
		return ""
	}
	rule := p.blockingRule()
	record := rounds.Round{SHA: p.convergence.head, HasLocations: true}
	note := func(issue types.ReviewIssue, fp string) {
		if !rule.Blocks(issue) {
			return
		}
		record.Blocking = append(record.Blocking, fp)
		record.Locations = append(record.Locations, rounds.Location{Path: issue.File, Line: issue.Line})
	}
	for i, issue := range p.shown.Issues {
		if i < len(p.round.fingerprints) {
			note(issue, p.round.fingerprints[i])
		}
	}
	for _, issue := range append(append([]types.ReviewIssue(nil), p.shown.Collapsed...), p.shown.Absorbed...) {
		note(issue, roundFingerprint(p.diff, issue, p.filter))
	}
	return rounds.RoundMarker(record)
}

// lateNotice is the details note counting the late findings.
func lateNotice(late int, language string) string {
	if late == 0 {
		return ""
	}
	return fmt.Sprintf("#### %s\n\n- %s\n", i18n.Text(language, "review.round_heading"), i18n.Format(language, "review.round_late", late))
}
