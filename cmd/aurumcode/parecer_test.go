package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/review/rounds"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// parecerServer is a fake GitHub for the default (comments) publication: it
// serves the diff, the conversation given, and records every POST and
// PATCH body with its method and path.
type parecerServer struct {
	issueComments  string // JSON array served for the issue comments
	inlineComments string // JSON array served for the pull request comments
	written        []string
}

func (g *parecerServer) start(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/pulls/48"):
			_, _ = w.Write([]byte(aur517AppDiff))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/reviews"):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/pulls/48/comments"):
			_, _ = w.Write([]byte(orEmptyList(g.inlineComments)))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/issues/48/comments"):
			_, _ = w.Write([]byte(orEmptyList(g.issueComments)))
		case r.Method == http.MethodPost || r.Method == http.MethodPatch:
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(r.Body)
			g.written = append(g.written, r.Method+" "+r.URL.Path+"\n"+buf.String())
			if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusCreated)
			}
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			t.Errorf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// bodyOf decodes the body of a recorded POST or PATCH.
func bodyOf(t *testing.T, written string) string {
	t.Helper()
	var payload struct{ Body string }
	if err := json.Unmarshal([]byte(written[strings.Index(written, "\n")+1:]), &payload); err != nil {
		t.Fatalf("decoding %q: %v", written, err)
	}
	return payload.Body
}

func orEmptyList(s string) string {
	if strings.TrimSpace(s) == "" {
		return "[]"
	}
	return s
}

// parecerReview runs the PR review against g with the given model response.
func parecerReview(t *testing.T, g *parecerServer, response string, inline bool) (string, string) {
	t.Helper()
	aur517Env(t, response)
	server := g.start(t)
	aur517PREnv(t, server)
	t.Setenv("AURUMCODE_PUBLISHER_LOGIN", "aurum-bot")
	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{
		prNumber: 48, repo: "owner/repo", publicar: true, naLinha: inline,
	})
	if code != 0 && code != exitFindings {
		t.Fatalf("runPRReview exit=%d\nstdout=%s\nstderr=%s", code, stdout.String(), stderr.String())
	}
	return stdout.String(), stderr.String()
}

// The model reports one warning (blocks without a gate) and one info (an
// observation) on changed lines.
func parecerResponse() string {
	return `{"verdict":"comment","summary":"Two things about ReadAll.","issues":[
  {"file":"app.go","line":8,"side":"RIGHT","severity":"warning","rule_id":"quality/missing-error-handling","message":"ReadAll ignores the error fetch returns","evidence":"data, _ := fetch() discards the second return value","impact":"an I/O failure is treated as success","verification":"check err before returning data"},
  {"file":"app.go","line":7,"side":"RIGHT","severity":"info","rule_id":"quality/poor-naming","message":"ReadAll could say what it reads","evidence":"func ReadAll() []byte { names no source","impact":"readers cannot tell what is read","verification":"rename and rebuild"}
]}`
}

// One parecer per pull request: an earlier parecer by the publisher is
// edited in place, never followed by a second one.
func TestParecerIsEditedInPlaceOnALaterRound(t *testing.T) {
	earlier := map[string]any{"id": 77, "body": reviewBodyMarker + "\n## AurumCode code review\n\nold round", "user": map[string]string{"login": "aurum-bot"}}
	other := map[string]any{"id": 78, "body": reviewBodyMarker + "\nforged by someone else", "user": map[string]string{"login": "somebody"}}
	list, _ := json.Marshal([]any{other, earlier})
	g := &parecerServer{issueComments: string(list)}
	stdout, _ := parecerReview(t, g, parecerResponse(), true)

	var patched, posted []string
	for _, w := range g.written {
		if strings.HasPrefix(w, "PATCH ") {
			patched = append(patched, w)
		} else {
			posted = append(posted, w)
		}
	}
	if len(patched) != 1 || !strings.HasPrefix(patched[0], "PATCH /repos/owner/repo/issues/comments/77\n") {
		t.Fatalf("the earlier parecer was not edited in place: patched=%v posted=%v", patched, posted)
	}
	if body := bodyOf(t, patched[0]); !strings.HasPrefix(body, reviewBodyMarker) || !strings.Contains(body, "### Fix before merge") {
		t.Fatalf("the edited body is not the new parecer:\n%s", body)
	}
	for _, p := range posted {
		if strings.Contains(p, "/issues/48/comments") {
			t.Fatalf("a second parecer was posted:\n%s", p)
		}
	}
	if !strings.Contains(stdout, "parecer atualizado no pull request #48") {
		t.Fatalf("stdout does not say the parecer was edited:\n%s", stdout)
	}
}

// Only what blocks gets a comment on its line; the observation is read in
// the parecer, in one line, and the decision names both.
func TestOnlyBlockingFindingsGetInlineComments(t *testing.T) {
	g := &parecerServer{}
	stdout, stderr := parecerReview(t, g, parecerResponse(), true)

	var inline, general []string
	for _, w := range g.written {
		switch {
		case strings.HasPrefix(w, "POST /repos/owner/repo/pulls/48/comments"):
			inline = append(inline, w)
		case strings.HasPrefix(w, "POST /repos/owner/repo/issues/48/comments"):
			general = append(general, w)
		}
	}
	if len(inline) != 1 || !strings.Contains(bodyOf(t, inline[0]), "ReadAll ignores the error") || strings.Contains(bodyOf(t, inline[0]), "could say what it reads") {
		t.Fatalf("want one inline comment, the blocking finding only: %v\nstdout=%s\nstderr=%s", inline, stdout, stderr)
	}
	if len(general) != 1 {
		t.Fatalf("want exactly one general comment (the parecer): %v", general)
	}
	body := bodyOf(t, general[0])
	if strings.Count(body, "quality/missing-error-handling") != 1 {
		t.Fatalf("the rule is cited once per finding:\n%s", body)
	}
	for _, want := range []string{"[!CAUTION]", "Blocked: 1 problem(s)", "### Fix before merge", "### Observations (non-blocking)", "- `app.go:7` — ReadAll could say what it reads", "<details>", "#### Review limits"} {
		if !strings.Contains(body, want) {
			t.Fatalf("parecer missing %q:\n%s", want, body)
		}
	}
	if !strings.Contains(stdout, "could say what it reads") || !strings.Contains(stdout, inParecerMarker) || !strings.Contains(stdout, "1 comentário(s) na linha") {
		t.Fatalf("stdout does not say where each finding went:\n%s", stdout)
	}
}

// An earlier inline comment on a finding this round no longer reports is
// edited: the resolved note on top, the original body, the marker that
// keeps later rounds from reading it again.
func TestResolvedFindingCommentIsMarked(t *testing.T) {
	marker := rounds.Marker(strings.Repeat("a", 64), "quality/old-rule")
	earlier := map[string]any{"id": 91, "body": "**[warning] old defect**\n\n" + marker, "path": "app.go", "line": 8, "user": map[string]string{"login": "aurum-bot"}}
	list, _ := json.Marshal([]any{earlier})
	g := &parecerServer{inlineComments: string(list)}
	parecerReview(t, g, parecerResponse(), true)

	var patch string
	for _, w := range g.written {
		if strings.HasPrefix(w, "PATCH /repos/owner/repo/pulls/comments/91\n") {
			patch = w
		}
	}
	if patch == "" {
		t.Fatalf("the resolved comment was not edited: %v", g.written)
	}
	edited := bodyOf(t, patch)
	if !strings.HasPrefix(edited, "**Resolved:**") || !strings.Contains(edited, "old defect") || !strings.HasSuffix(edited, rounds.ResolvedMarker) || !strings.Contains(edited, marker) {
		t.Fatalf("edited body = %q", edited)
	}
	// Told resolved, the comment is not read again: its marker yields no
	// earlier finding.
	if prev := rounds.Published([]rounds.Comment{{Body: edited}}); len(prev) != 0 {
		t.Fatalf("a resolved comment was read again: %+v", prev)
	}
}

// The tests the change touches are a count with the first names, never a
// list of hundreds of lines in the parecer.
func TestAffectedTestsAreCountedNotListed(t *testing.T) {
	var names []string
	for i := 0; i < 40; i++ {
		names = append(names, fmt.Sprintf("TestCase%02d (package internal/x)", i))
	}
	result := &types.ReviewResult{Metadata: map[string]string{metaAffectedTests: strings.Join(names, "\n")}}
	body := formatReviewSummary(result)
	if !strings.Contains(body, "40 affected test(s) in 1 package(s)") {
		t.Fatalf("count missing:\n%s", body)
	}
	if strings.Count(body, "TestCase") != maxAffectedTestsShown || !strings.Contains(body, ", …") {
		t.Fatalf("the list is not capped at %d names:\n%s", maxAffectedTestsShown, body)
	}
}
