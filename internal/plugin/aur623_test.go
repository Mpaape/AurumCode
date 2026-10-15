package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// aur623Canary is a secret-shaped value the issues step must redact by
// itself (the SARIF below is hand-written, not produced by the ingest).
var aur623Canary = "pass" + "word=" + "canario-aur623-4e2a"

// aur623Fake is an in-memory GitHub: one repository, its labels and issues.
// It forces a page size of 2 so every listing crosses pages, and records
// every request so a test can prove that no write happened.
type aur623Fake struct {
	mu         sync.Mutex
	visibility string // "" answers 500 on GET repo
	private    bool
	labels     map[string]bool
	issues     []*aur623Issue
	comments   map[int][]string
	calls      []string
	dropLabel  bool
	limited    int // POST /issues answered with the rate limiter first
}

// aur623Oversize mirrors GitHub's 422 for a title over 256 or a body over
// 65536 characters.
func aur623Oversize(in map[string]any) bool {
	t, _ := in["title"].(string)
	b, _ := in["body"].(string)
	return len([]rune(t)) > 256 || len([]rune(b)) > 65536
}

type aur623Issue struct {
	Number int      `json:"number"`
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	State  string   `json:"state"`
	Reason string   `json:"state_reason"`
	Labels []string `json:"-"`
}

func newAUR623Fake(visibility string, private bool) *aur623Fake {
	return &aur623Fake{visibility: visibility, private: private, labels: map[string]bool{}, comments: map[int][]string{}}
}

func (f *aur623Fake) writes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var w []string
	for _, c := range f.calls {
		if !strings.HasPrefix(c, "GET ") {
			w = append(w, c)
		}
	}
	return w
}

func (f *aur623Fake) wire(is *aur623Issue) map[string]any {
	labels := []map[string]string{}
	for _, l := range is.Labels {
		labels = append(labels, map[string]string{"name": l})
	}
	return map[string]any{"number": is.Number, "title": is.Title, "body": is.Body, "state": is.State, "state_reason": is.Reason, "labels": labels}
}

var aur623Route = regexp.MustCompile(`^/repos/dono/repo(/.*)?$`)

func (f *aur623Fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	m := aur623Route.FindStringSubmatch(r.URL.Path)
	if m == nil {
		http.NotFound(w, r)
		return
	}
	var in map[string]any
	_ = json.NewDecoder(r.Body).Decode(&in)
	route := m[1]
	send := func(code int, v any) { w.WriteHeader(code); _ = json.NewEncoder(w).Encode(v) }
	switch {
	case r.Method == "GET" && route == "":
		if f.visibility == "" {
			send(500, map[string]string{})
			return
		}
		send(200, map[string]any{"visibility": f.visibility, "private": f.private})
	case r.Method == "GET" && strings.HasPrefix(route, "/labels/"):
		if f.labels[strings.TrimPrefix(route, "/labels/")] {
			send(200, map[string]string{})
			return
		}
		send(404, map[string]string{})
	case r.Method == "POST" && route == "/labels":
		f.labels[fmt.Sprint(in["name"])] = true
		send(201, map[string]string{})
	case r.Method == "GET" && route == "/issues":
		f.list(w, r, send)
	case (r.Method == "POST" || r.Method == "PATCH") && aur623Oversize(in):
		send(422, map[string]string{})
	case r.Method == "POST" && route == "/issues" && f.limited > 0:
		f.limited--
		w.Header().Set("Retry-After", "0")
		send(403, map[string]string{})
	case r.Method == "POST" && route == "/issues":
		is := &aur623Issue{Number: len(f.issues) + 1, Title: fmt.Sprint(in["title"]), Body: fmt.Sprint(in["body"]), State: "open"}
		if !f.dropLabel {
			for _, l := range in["labels"].([]any) {
				is.Labels = append(is.Labels, fmt.Sprint(l))
			}
		}
		f.issues = append(f.issues, is)
		send(201, f.wire(is))
	default:
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		f.patchOrComment(r.Method, route, page, in, send)
	}
}

func (f *aur623Fake) list(_ http.ResponseWriter, r *http.Request, send func(int, any)) {
	label := r.URL.Query().Get("labels")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	var match []map[string]any
	for _, is := range f.issues {
		for _, l := range is.Labels {
			if l == label {
				match = append(match, f.wire(is))
			}
		}
	}
	const size = 2
	lo, hi := (page-1)*size, page*size
	if lo > len(match) {
		lo = len(match)
	}
	if hi > len(match) {
		hi = len(match)
	}
	send(200, append([]map[string]any{}, match[lo:hi]...))
}

var aur623IssueRoute = regexp.MustCompile(`^/issues/(\d+)(/comments)?$`)

// commentPage serves an issue's comments 2 per page, oldest first.
func (f *aur623Fake) commentPage(n, page int) []map[string]string {
	all := f.comments[n]
	lo, hi := (page-1)*2, page*2
	if lo > len(all) {
		lo = len(all)
	}
	if hi > len(all) {
		hi = len(all)
	}
	out := []map[string]string{}
	for _, c := range all[lo:hi] {
		out = append(out, map[string]string{"body": c})
	}
	return out
}

func (f *aur623Fake) patchOrComment(method, route string, page int, in map[string]any, send func(int, any)) {
	m := aur623IssueRoute.FindStringSubmatch(route)
	if m == nil {
		send(404, map[string]string{})
		return
	}
	n, _ := strconv.Atoi(m[1])
	if n < 1 || n > len(f.issues) {
		send(404, map[string]string{})
		return
	}
	is := f.issues[n-1]
	switch {
	case method == "GET" && m[2] != "":
		send(200, f.commentPage(n, page))
	case method == "POST" && m[2] != "":
		f.comments[n] = append(f.comments[n], fmt.Sprint(in["body"]))
		send(201, map[string]string{})
	case method == "PATCH" && m[2] == "":
		if v, ok := in["title"]; ok {
			is.Title = fmt.Sprint(v)
		}
		if v, ok := in["body"]; ok {
			is.Body = fmt.Sprint(v)
		}
		if v, ok := in["state"]; ok {
			is.State = fmt.Sprint(v)
		}
		if v, ok := in["state_reason"]; ok {
			is.Reason = fmt.Sprint(v)
		}
		send(200, f.wire(is))
	default:
		send(404, map[string]string{})
	}
}

// aur623Result is one SARIF result of the hand-written document.
type aur623Result struct{ fp, rule, path, text string }

func aur623SARIF(t *testing.T, dir, name string, ok bool, results ...aur623Result) string {
	t.Helper()
	rs := []map[string]any{}
	for i, r := range results {
		rs = append(rs, map[string]any{
			"ruleId": r.rule, "level": "error", "message": map[string]string{"text": r.text},
			"locations":           []map[string]any{{"physicalLocation": map[string]any{"artifactLocation": map[string]string{"uri": r.path}, "region": map[string]int{"startLine": 10 + i}}}},
			"partialFingerprints": map[string]string{FingerprintKey: r.fp},
			"properties":          map[string]string{"security-severity": "8.0"},
		})
	}
	doc := map[string]any{"version": "2.1.0", "runs": []map[string]any{{
		"tool":              map[string]any{"driver": map[string]any{"name": "AurumCode plugin " + name}},
		"automationDetails": map[string]string{"id": Category(name) + "/"},
		"invocations":       []map[string]bool{{"executionSuccessful": ok}},
		"results":           rs,
	}}}
	data, _ := json.Marshal(doc)
	p := filepath.Join(dir, "plugin-"+name+".sarif")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func aur623Sync(t *testing.T, fake *aur623Fake, sarif string) (int, string, string) {
	t.Helper()
	srv := httptest.NewServer(fake)
	defer srv.Close()
	api := NewGitHubIssues(srv.URL, "token-falso", "dono/repo")
	var out, errb bytes.Buffer
	rc := RunIssues(context.Background(), api, IssuesOptions{Name: "vvah", SARIF: sarif, Repo: "dono/repo"}, "pt-BR", &out, &errb)
	return rc, out.String(), errb.String()
}

var (
	aur623A = aur623Result{"caso-a", "plugin/vvah/cwe-89", "app.py", "Consulta SQL concatenada " + aur623Canary + " @alguem <!-- x -->"}
	aur623B = aur623Result{"caso-b", "plugin/vvah/cwe-79", "web/view.py", "Saida sem escape"}
	aur623C = aur623Result{"caso-c", "plugin/vvah/cwe-22", "files.py", "Caminho montado da entrada"}
)

// AC-001: two findings, no issues: two issues with marker, label and a
// readable pt-BR body; the secret redacted; stdout only counts.
func TestAUR623OpensIssuesWithMarker(t *testing.T) {
	fake := newAUR623Fake("private", true)
	sarif := aur623SARIF(t, t.TempDir(), "vvah", true, aur623A, aur623B)
	rc, out, errb := aur623Sync(t, fake, sarif)
	if rc != 0 || len(fake.issues) != 2 {
		t.Fatalf("rc=%d issues=%d stderr=%q", rc, len(fake.issues), errb)
	}
	for i, r := range []aur623Result{aur623A, aur623B} {
		is := fake.issues[i]
		first, _, _ := strings.Cut(is.Body, "\n")
		if first != "<!-- aurumcode-plugin:vvah:"+r.fp+" -->" || len(is.Labels) != 1 || is.Labels[0] != "aurumcode-plugin:vvah" {
			t.Fatalf("issue %d: marker %q labels %v", is.Number, first, is.Labels)
		}
		for _, want := range []string{"| regra | " + r.rule + " |", "| arquivo | " + r.path + " |", "| linha | " + strconv.Itoa(10+i) + " |", "| severidade | alta (8.0) |", "### Descrição (redigida)", "### Sugestão"} {
			if !strings.Contains(is.Body, want) {
				t.Fatalf("issue %d body lacks %q:\n%s", is.Number, want, is.Body)
			}
		}
		if !strings.Contains(is.Title, r.rule) || !strings.Contains(is.Title, r.path) || strings.Contains(is.Title, "Consulta") || strings.Contains(is.Title, "Saida") {
			t.Fatalf("title must carry only structured fields: %q", is.Title)
		}
	}
	if strings.Contains(fake.issues[0].Body, "canario-aur623") || !strings.Contains(fake.issues[0].Body, "[REDACTED]") {
		t.Fatalf("secret not redacted:\n%s", fake.issues[0].Body)
	}
	if !fake.labels["aurumcode-plugin:vvah"] {
		t.Fatal("label not created before the issues")
	}
	for _, leak := range []string{"app.py", "Consulta", "canario", "cwe-89"} {
		if strings.Contains(out+errb, leak) {
			t.Fatalf("log cites finding text %q: %q", leak, out+errb)
		}
	}
	if !strings.Contains(out, "abertas 2, mantidas 0, fechadas 0, reabertas 0") {
		t.Fatalf("counts line: %q", out)
	}
}

// AC-002: next run with A and C (B gone): A kept without duplicate, B
// closed with a comment, C opened; then B returns and is reopened. The
// listing crosses pages (the fake serves 2 per page).
func TestAUR623SyncKeepsClosesReopens(t *testing.T) {
	fake := newAUR623Fake("private", true)
	dir := t.TempDir()
	if rc, _, e := aur623Sync(t, fake, aur623SARIF(t, dir, "vvah", true, aur623A, aur623B)); rc != 0 {
		t.Fatalf("first run rc=%d %s", rc, e)
	}
	rc, out, e := aur623Sync(t, fake, aur623SARIF(t, dir, "vvah", true, aur623A, aur623C))
	if rc != 0 || !strings.Contains(out, "abertas 1, mantidas 1, fechadas 1, reabertas 0") {
		t.Fatalf("second run rc=%d out=%q err=%q", rc, out, e)
	}
	if len(fake.issues) != 3 || fake.issues[0].State != "open" || fake.issues[1].State != "closed" || fake.issues[2].State != "open" {
		t.Fatalf("states after second run: %+v", fake.issues)
	}
	if len(fake.comments[2]) != 1 || !strings.Contains(fake.comments[2][0], "varredura conclusiva") {
		t.Fatalf("closed issue without the comment: %v", fake.comments)
	}
	rc, out, e = aur623Sync(t, fake, aur623SARIF(t, dir, "vvah", true, aur623A, aur623B, aur623C))
	if rc != 0 || !strings.Contains(out, "abertas 0, mantidas 2, fechadas 0, reabertas 1") || fake.issues[1].State != "open" || len(fake.issues) != 3 {
		t.Fatalf("third run rc=%d out=%q err=%q issues=%+v", rc, out, e, fake.issues)
	}
	before := len(fake.writes())
	if rc, out, _ := aur623Sync(t, fake, aur623SARIF(t, dir, "vvah", true, aur623A, aur623B, aur623C)); rc != 0 || !strings.Contains(out, "mantidas 3") {
		t.Fatalf("idempotent run rc=%d out=%q", rc, out)
	}
	if after := len(fake.writes()); after != before {
		t.Fatalf("unchanged findings wrote %d times", after-before)
	}
}

// AC-002: an issue without the plugin's marker on its first line (a forged
// marker further down, or another plugin's) is never touched; a conclusive
// zero closes the plugin's open issues.
func TestAUR623ForeignIssuesUntouchedAndZeroCloses(t *testing.T) {
	fake := newAUR623Fake("internal", true)
	fake.issues = []*aur623Issue{
		{Number: 1, State: "open", Labels: []string{"aurumcode-plugin:vvah"}, Body: "texto\n<!-- aurumcode-plugin:vvah:caso-z -->"},
		{Number: 2, State: "open", Labels: []string{"aurumcode-plugin:vvah"}, Body: "<!-- aurumcode-plugin:outro:caso-z -->"},
		{Number: 3, State: "open", Labels: []string{"aurumcode-plugin:vvah"}, Body: "<!-- aurumcode-plugin:vvah:caso-z -->\nantigo"},
	}
	rc, out, e := aur623Sync(t, fake, aur623SARIF(t, t.TempDir(), "vvah", true))
	if rc != 0 || !strings.Contains(out, "fechadas 1") {
		t.Fatalf("rc=%d out=%q err=%q", rc, out, e)
	}
	if fake.issues[0].State != "open" || fake.issues[1].State != "open" || fake.issues[2].State != "closed" {
		t.Fatalf("states: %+v", fake.issues)
	}
}

// AC-001: an issue the API created without the label fails the sync: it
// would be duplicated by every following run.
func TestAUR623DroppedLabelFails(t *testing.T) {
	fake := newAUR623Fake("private", true)
	fake.dropLabel = true
	if rc, _, _ := aur623Sync(t, fake, aur623SARIF(t, t.TempDir(), "vvah", true, aur623A)); rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
}

// AC-003: public repository, unreadable visibility or a private flag that
// disagrees: exit 1 and no write. Inconclusive scan (no SARIF, a failed
// run, another plugin's category): no request at all, open issues stay.
func TestAUR623RefusesPublicAndInconclusive(t *testing.T) {
	cases := map[string]*aur623Fake{
		"public":     newAUR623Fake("public", false),
		"unreadable": newAUR623Fake("", false),
		"flag":       newAUR623Fake("private", false),
	}
	for name, fake := range cases {
		rc, _, e := aur623Sync(t, fake, aur623SARIF(t, t.TempDir(), "vvah", true, aur623A))
		if rc != 1 || len(fake.writes()) != 0 || len(fake.issues) != 0 {
			t.Fatalf("%s: rc=%d writes=%v", name, rc, fake.writes())
		}
		if !strings.Contains(e, "nada foi publicado") {
			t.Fatalf("%s: stderr %q", name, e)
		}
	}
	dir := t.TempDir()
	inconclusive := map[string]string{
		"missing":   filepath.Join(dir, "nao-existe.sarif"),
		"failed":    aur623SARIF(t, t.TempDir(), "vvah", false),
		"other":     aur623SARIF(t, t.TempDir(), "outro", true),
		"malformed": filepath.Join(dir, "ruim.sarif"),
	}
	if err := os.WriteFile(inconclusive["malformed"], []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, sarif := range inconclusive {
		if name == "other" {
			_ = os.Rename(sarif, filepath.Join(filepath.Dir(sarif), "plugin-vvah.sarif"))
			sarif = filepath.Join(filepath.Dir(sarif), "plugin-vvah.sarif")
		}
		fake := newAUR623Fake("private", true)
		fake.issues = []*aur623Issue{{Number: 1, State: "open", Labels: []string{"aurumcode-plugin:vvah"}, Body: IssueMarker("vvah", "caso-a") + "\n"}}
		rc, _, e := aur623Sync(t, fake, sarif)
		if rc != 1 || len(fake.calls) != 0 || fake.issues[0].State != "open" {
			t.Fatalf("%s: rc=%d calls=%v state=%s", name, rc, fake.calls, fake.issues[0].State)
		}
		if !strings.Contains(e, "nenhuma issue foi tocada") {
			t.Fatalf("%s: stderr %q", name, e)
		}
	}
}

// AC-003: a malformed invocation is a usage error, before any request.
func TestAUR623UsageBeforeAnyRequest(t *testing.T) {
	fake := newAUR623Fake("private", true)
	srv := httptest.NewServer(fake)
	defer srv.Close()
	var out, errb bytes.Buffer
	for _, o := range []IssuesOptions{{Name: "vvah", SARIF: "x", Repo: "."}, {Name: "", SARIF: "x", Repo: "dono/repo"}, {Name: "vvah", Repo: "dono/repo"}} {
		if rc := RunIssues(context.Background(), NewGitHubIssues(srv.URL, "", o.Repo), o, "pt-BR", &out, &errb); rc != 2 {
			t.Fatalf("%+v: rc=%d", o, rc)
		}
	}
	if len(fake.calls) != 0 {
		t.Fatalf("calls: %v", fake.calls)
	}
}

// AC-001: a path too long for GitHub's title and a description too long
// for its body are fitted, never a sync that fails every night; the
// fitted body is stable (a second run keeps it, no write).
func TestAUR623OversizeFindingFits(t *testing.T) {
	fake := newAUR623Fake("private", true)
	long := aur623Result{"caso-longo", "plugin/vvah/cwe-89", strings.Repeat("diretorio/", 30) + "app.py", strings.Repeat("descricao longa ", 5000) + "```fim"}
	sarif := aur623SARIF(t, t.TempDir(), "vvah", true, long)
	rc, out, e := aur623Sync(t, fake, sarif)
	if rc != 0 || len(fake.issues) != 1 {
		t.Fatalf("rc=%d out=%q err=%q", rc, out, e)
	}
	is := fake.issues[0]
	if !strings.Contains(is.Title, "…") || !strings.HasSuffix(is.Title, "app.py:10 (severidade alta)") || !strings.Contains(is.Body, "cortada no limite") {
		t.Fatalf("title %q / body not fitted", is.Title)
	}
	if strings.Count(is.Body, "```") != 2 {
		t.Fatal("a backtick of the description closed the code block")
	}
	before := len(fake.writes())
	if rc, _, _ := aur623Sync(t, fake, sarif); rc != 0 || len(fake.writes()) != before {
		t.Fatalf("fitted body is not stable: rc=%d writes %d -> %d", rc, before, len(fake.writes()))
	}
}

// AC-001: the rate limiter (403 with Retry-After) is waited and the write
// is sent again, never duplicated.
func TestAUR623RateLimitRetried(t *testing.T) {
	fake := newAUR623Fake("private", true)
	fake.limited = 2
	rc, _, e := aur623Sync(t, fake, aur623SARIF(t, t.TempDir(), "vvah", true, aur623A))
	if rc != 0 || len(fake.issues) != 1 {
		t.Fatalf("rc=%d issues=%d err=%q", rc, len(fake.issues), e)
	}
}

// AC-002: only an issue the plugin closed (its closed marker is the last
// comment) is reopened. One a person closed (no marker after the plugin's
// reopen comment, or "not planned") stays closed, counted as dismissed,
// with no write on it.
func TestAUR623HumanCloseStands(t *testing.T) {
	fake := newAUR623Fake("private", true)
	dir := t.TempDir()
	marker := func(fp string) string { return IssueMarker("vvah", fp) + "\n" }
	fake.issues = []*aur623Issue{
		{Number: 1, State: "closed", Reason: "completed", Labels: []string{"aurumcode-plugin:vvah"}, Body: marker("caso-a")},
		{Number: 2, State: "closed", Reason: "not_planned", Labels: []string{"aurumcode-plugin:vvah"}, Body: marker("caso-b")},
		{Number: 3, State: "closed", Reason: "completed", Labels: []string{"aurumcode-plugin:vvah"}, Body: marker("caso-c")},
	}
	fake.comments[1] = []string{closeComment("vvah"), "O achado voltou numa varredura conclusiva do plugin; a issue foi reaberta pelo AurumCode."}
	fake.comments[2] = []string{closeComment("vvah")}
	fake.comments[3] = []string{"x", "y", closeComment("vvah")}
	rc, out, e := aur623Sync(t, fake, aur623SARIF(t, dir, "vvah", true, aur623A, aur623B, aur623C))
	if rc != 0 || !strings.Contains(out, "abertas 0, mantidas 0, fechadas 0, reabertas 1, dispensadas por pessoa 2") {
		t.Fatalf("rc=%d out=%q err=%q", rc, out, e)
	}
	if fake.issues[0].State != "closed" || fake.issues[1].State != "closed" || fake.issues[2].State != "open" {
		t.Fatalf("states: %+v", fake.issues)
	}
	for _, w := range fake.writes() {
		if strings.Contains(w, "/issues/1") || strings.Contains(w, "/issues/2") {
			t.Fatalf("write on a dismissed issue: %v", fake.writes())
		}
	}
	if len(fake.comments[1]) != 2 || len(fake.comments[2]) != 1 {
		t.Fatal("a dismissed issue got a comment")
	}
}
