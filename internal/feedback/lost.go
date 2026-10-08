package feedback

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// Comment is the part of a GitHub issue or pull request comment the loop
// reads.
type Comment struct {
	ID                int64  `json:"id"`
	Body              string `json:"body"`
	HTMLURL           string `json:"html_url"`
	IssueURL          string `json:"issue_url"`
	AuthorAssociation string `json:"author_association"`
}

// lostCommand is the comment command that reports a defect the gate let
// through: "/aurum perdeu [<sha>] <descricao>".
const lostCommand = "/aurum perdeu"

// trustedAssociations are the authors whose report is accepted. Anyone else
// could inject text into the model prompt and the policy pull request.
var trustedAssociations = map[string]bool{"OWNER": true, "MEMBER": true, "COLLABORATOR": true}

var shaRE = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// PullHead resolves the head commit of pull request number in repo; ok is
// false when the number is an issue, not a pull request.
type PullHead func(repo string, number int) (sha string, ok bool, err error)

// Rejected names a comment that was not turned into a signal, and why.
type Rejected struct {
	Source string `json:"origem"`
	Reason string `json:"motivo"`
}

// Escaped turns the /aurum perdeu comments of repo into escaped-defect
// signals with the commit and the description. The commit is the one named
// in the command or, on a pull request, its head; without a commit the
// report is rejected, never guessed.
func Escaped(filter *redaction.Filter, repo string, comments []Comment, head PullHead) ([]Signal, []Rejected, error) {
	if filter == nil {
		filter = redaction.NewFilter()
	}
	var out []Signal
	var rejected []Rejected
	for _, c := range comments {
		rest, ok := lostArguments(c.Body)
		if !ok {
			continue
		}
		reject := func(reason string) {
			rejected = append(rejected, Rejected{Source: filter.Redact(c.HTMLURL), Reason: reason})
		}
		if !trustedAssociations[strings.ToUpper(c.AuthorAssociation)] {
			reject("autor sem associação ao repositório")
			continue
		}
		sha, desc := "", rest
		if fields := strings.Fields(rest); len(fields) > 0 && shaRE.MatchString(strings.ToLower(fields[0])) {
			sha = strings.ToLower(fields[0])
			desc = strings.TrimSpace(strings.TrimPrefix(rest, fields[0]))
		}
		if desc == "" {
			reject("sem descrição")
			continue
		}
		if sha == "" {
			number, numOK := issueNumber(c.IssueURL)
			if !numOK {
				reject("sem commit")
				continue
			}
			headSHA, isPR, err := head(repo, number)
			if err != nil {
				return nil, nil, err
			}
			if !isPR || headSHA == "" {
				reject("sem commit: em issue, informe o SHA")
				continue
			}
			sha = headSHA
		}
		out = append(out, newSignal(filter, KindEscaped, repo, "comment:"+strconv.FormatInt(c.ID, 10), Signal{
			SHA:         sha,
			Description: desc,
			Source:      c.HTMLURL,
		}))
	}
	return out, rejected, nil
}

// lostArguments returns the text after the command when a line of body
// starts with it.
func lostArguments(body string) (string, bool) {
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, lostCommand+" ") || t == lostCommand {
			return strings.TrimSpace(strings.TrimPrefix(t, lostCommand)), true
		}
	}
	return "", false
}

func issueNumber(issueURL string) (int, bool) {
	i := strings.LastIndex(issueURL, "/")
	if i < 0 {
		return 0, false
	}
	n, err := strconv.Atoi(issueURL[i+1:])
	return n, err == nil && n > 0
}

// CandidateCase is a candidate for the AUR-523 corpus generated from an
// escaped defect. A human completes it (file, line, label) before it joins
// the corpus; until then it is evidence, not ground truth.
type CandidateCase struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Label       string `json:"label"`
	Repo        string `json:"repo"`
	Commit      string `json:"commit"`
	Description string `json:"descricao"`
	Source      string `json:"origem"`
	Signal      string `json:"sinal"`
}

// Candidates builds one candidate case per escaped-defect signal; the case
// id is the signal id, so a rerun can never duplicate it.
func Candidates(signals []Signal) []CandidateCase {
	var out []CandidateCase
	for _, s := range signals {
		if s.Kind != KindEscaped {
			continue
		}
		out = append(out, CandidateCase{
			ID: "escapado-" + s.ID, Status: "candidato", Label: "defect",
			Repo: s.Repo, Commit: s.SHA, Description: s.Description, Source: s.Source, Signal: s.ID,
		})
	}
	return out
}
