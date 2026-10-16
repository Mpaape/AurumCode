package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

const (
	convergeHead1 = "1111111111111111111111111111111111111111"
	convergeHead2 = "2222222222222222222222222222222222222222"
)

// convergeGitHub is the rounds fake GitHub plus the pull request head and
// the comparison between the rounds' heads.
type convergeGitHub struct {
	*aur494GitHub
	mu   sync.Mutex
	head string
	// heads, when set, answers the next metadata reads in order.
	heads    []string
	status   int // HTTP status of the comparison; 0 is 200
	compare  map[string]any
	compared []string
}

func (g *convergeGitHub) handler(t *testing.T) http.Handler {
	inner := g.aur494GitHub.handler(t)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/42" && strings.Contains(r.Header.Get("Accept"), "json"):
			if len(g.heads) > 0 {
				g.head, g.heads = g.heads[0], g.heads[1:]
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"head": map[string]string{"sha": g.head}, "base": map[string]string{"sha": "base"}})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/repos/owner/repo/compare/"):
			g.compared = append(g.compared, strings.TrimPrefix(r.URL.Path, "/repos/owner/repo/compare/"))
			if g.status != 0 {
				w.WriteHeader(g.status)
				return
			}
			_ = json.NewEncoder(w).Encode(g.compare)
		default:
			inner.ServeHTTP(w, r)
		}
	})
}

func convergeServer(t *testing.T) (*convergeGitHub, *httptest.Server) {
	g := &convergeGitHub{aur494GitHub: &aur494GitHub{}, head: convergeHead1}
	server := httptest.NewServer(g.handler(t))
	t.Cleanup(server.Close)
	return g, server
}

// convergeSecondRound sets the second round's head and comparison.
func (g *convergeGitHub) convergeSecondRound(status int, compare map[string]any) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.head, g.status, g.compare = convergeHead2, status, compare
}

const convergeLateRule = "quality/missing-error-handling"

func convergeIssue(line int, rule, severity string) string {
	return `{"file":"main.go","line":` + strconv.Itoa(line) + `,"severity":"` + severity + `","rule_id":"` + rule + `","message":"Mensagem do achado","impact":"impacto","evidence":"evidencia na linha","verification":"verificacao"}`
}

// TestRoundsConvergeLateFindingOnUnchangedCode covers the five decisions of
// a second round: a new finding on a line unchanged since the first round
// becomes an observation; the same with a security rule, a repeated
// unresolved finding, a changed line and an untrusted history all block.
func TestRoundsConvergeLateFindingOnUnchangedCode(t *testing.T) {
	unchanged := map[string]any{"status": "ahead", "files": []any{}}
	changedLine3 := map[string]any{"status": "ahead", "files": []any{map[string]any{"filename": "main.go", "status": "modified", "patch": "@@ -1,3 +1,3 @@\n package main\n var limite = 42\n-// velho\n+// fim\n"}}}
	ruleOf := func(id string) string {
		if id == "" {
			return convergeLateRule
		}
		return id
	}
	cases := []struct {
		name       string
		firstRule  string // "" : the first round reports nothing
		secondRule string
		status     int
		compare    map[string]any
		wantLate   bool
	}{
		{"novo em linha inalterada vira observação", "", convergeLateRule, 0, unchanged, true},
		{"regra de segurança continua bloqueando", "", "security/command-injection", 0, unchanged, false},
		{"achado repetido não resolvido continua bloqueando", convergeLateRule, convergeLateRule, 0, unchanged, false},
		{"linha alterada é tratada normalmente", "", convergeLateRule, 0, changedLine3, false},
		{"sem histórico confiável (comparação recusada)", "", convergeLateRule, http.StatusNotFound, nil, false},
		{"sem histórico confiável (force push)", "", convergeLateRule, 0, map[string]any{"status": "diverged", "files": []any{}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, server := convergeServer(t)
			first := aur494Response()
			if tc.firstRule != "" {
				first = aur494Response(convergeIssue(3, ruleOf(tc.firstRule), "warning"))
			}
			round1, _ := aur494Round(t, g.aur494GitHub, server, aur494DiffA, first)
			if !strings.Contains(round1.Body, "<!-- aurumcode:round "+convergeHead1+" ") {
				t.Fatalf("the first round did not record its head:\n%s", round1.Body)
			}
			g.convergeSecondRound(tc.status, tc.compare)
			round2, _ := aur494Round(t, g.aur494GitHub, server, aur494DiffA, aur494Response(convergeIssue(3, tc.secondRule, "warning")))
			if len(g.compared) != 1 || g.compared[0] != convergeHead1+"..."+convergeHead2 {
				t.Fatalf("comparisons = %v, want the first round's head against the second's", g.compared)
			}
			late := aur494InEitherLanguage(round2.Body, "review.late_finding", shortSHA(convergeHead1))
			blocks := round2.Event == "REQUEST_CHANGES"
			if late != tc.wantLate || blocks == tc.wantLate {
				t.Fatalf("late note = %v, event = %s; want late = %v\n%s", late, round2.Event, tc.wantLate, round2.Body)
			}
		})
	}
}

// TestRoundsConvergeDegradedRoundKeepsEarlierRecord: a round that did not
// look everywhere never records its own head as reviewed.
func TestRoundsConvergeDegradedRoundKeepsEarlierRecord(t *testing.T) {
	p := &prReview{reviewState: reviewState{model: modelProviderFailed}}
	if p.roundMarker() != "" {
		t.Fatal("a degraded first round recorded a head")
	}
	p.convergence.head, p.convergence.hasEarlier = convergeHead2, true
	p.convergence.earlier.SHA = convergeHead1
	if got := p.roundMarker(); !strings.Contains(got, convergeHead1) || strings.Contains(got, convergeHead2) {
		t.Fatalf("degraded round marker = %q, want the earlier round's head carried forward", got)
	}
}

// TestRoundsConvergeMarkerSurvivesRedaction: the round marker is read back
// from the conversation after redaction, so the filter must keep it.
func TestRoundsConvergeMarkerSurvivesRedaction(t *testing.T) {
	body := reviewBodyMarker + "\nparecer\n<!-- aurumcode:round " + convergeHead1 + " " + strings.Repeat("a", 64) + " -->\n"
	if redacted := redaction.NewFilter().Redact(body); redacted != body {
		t.Fatalf("redaction changed the round marker:\n%s", redacted)
	}
	if !strings.Contains(i18n.Format("pt-BR", "review.late_finding", "1111111"), "código já revisado") {
		t.Fatal("the late note does not say the code was already reviewed")
	}
}

// TestRoundsConvergeHeadMovedDuringReview: a push while the round ran means
// the recorded head was never reviewed: nothing is demoted, the moved head
// is not recorded, and the earlier record is carried forward.
func TestRoundsConvergeHeadMovedDuringReview(t *testing.T) {
	g, server := convergeServer(t)
	aur494Round(t, g.aur494GitHub, server, aur494DiffA, aur494Response())
	const head3 = "3333333333333333333333333333333333333333"
	g.convergeSecondRound(0, map[string]any{"status": "identical", "files": []any{}})
	g.mu.Lock()
	g.heads = []string{convergeHead2, head3}
	g.mu.Unlock()
	round2, _ := aur494Round(t, g.aur494GitHub, server, aur494DiffA, aur494Response(convergeIssue(3, convergeLateRule, "warning")))
	if round2.Event != "REQUEST_CHANGES" || len(g.compared) != 0 {
		t.Fatalf("event = %s, comparisons = %v: a moved head must not converge", round2.Event, g.compared)
	}
	if strings.Contains(round2.Body, head3) || !strings.Contains(round2.Body, "<!-- aurumcode:round "+convergeHead1+" ") {
		t.Fatalf("the moved head was recorded, or the earlier record was lost:\n%s", round2.Body)
	}
}

// TestRoundsConvergeExemptions: what convergence never turns into an
// observation.
func TestRoundsConvergeExemptions(t *testing.T) {
	p := &prReview{}
	p.dynamicRules = map[string]review.Rule{
		"policy/erro":    {ID: "policy/erro", Origin: gateOriginPolicy, Severity: "error"},
		"policy/aviso":   {ID: "policy/aviso", Origin: gateOriginPolicy, Severity: "warning"},
		"repo/erro":      {ID: "repo/erro", Origin: gateOriginRepo, Severity: "error"},
		"repo/seguranca": {ID: "repo/seguranca", Origin: gateOriginRepo, Severity: "warning", Category: "security"},
	}
	catalog := review.NewRulesLoader()
	if err := catalog.Load(); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		rule, severity string
		want           bool
	}{
		{"security/xss", "warning", true},
		{"quality/hardcoded-secret", "warning", true},
		{"quality/sql-injection-risk", "warning", true},
		{"quality/poor-naming", "critical", true},
		{"policy/erro", "warning", true},
		{"policy/aviso", "error", true},
		{"policy/aviso", "warning", true},
		{"security#x", "warning", true},
		{"seguranca#x", "warning", true},
		{"security-policy#x", "warning", true},
		{"repo-security/x", "warning", true},
		{"repo/erro", "error", false},
		{"repo/seguranca", "warning", true},
		{"quality/missing-error-handling", "error", false},
	}
	for _, c := range cases {
		if got := p.alwaysBlocks(types.ReviewIssue{RuleID: c.rule, Severity: c.severity}, catalog); got != c.want {
			t.Errorf("alwaysBlocks(%s, %s) = %v, want %v", c.rule, c.severity, got, c.want)
		}
	}
}

// TestRoundsConvergeOffWhenWorkflowChanges: a pull request that changes a
// workflow could post as the publisher login and forge the record.
func TestRoundsConvergeOffWhenWorkflowChanges(t *testing.T) {
	const workflowDiff = "diff --git a/.github/workflows/ci.yml b/.github/workflows/ci.yml\n@@ -1,1 +1,2 @@\n name: ci\n+on: push\n"
	g, server := convergeServer(t)
	aur494Round(t, g.aur494GitHub, server, aur494DiffA+workflowDiff, aur494Response())
	g.convergeSecondRound(0, map[string]any{"status": "ahead", "files": []any{}})
	round2, _ := aur494Round(t, g.aur494GitHub, server, aur494DiffA+workflowDiff, aur494Response(convergeIssue(3, convergeLateRule, "warning")))
	if round2.Event != "REQUEST_CHANGES" || len(g.compared) != 0 {
		t.Fatalf("event = %s, comparisons = %v: a workflow change must turn convergence off", round2.Event, g.compared)
	}
}
