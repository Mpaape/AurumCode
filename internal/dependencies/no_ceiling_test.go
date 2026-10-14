package dependencies

// Issue #179: no size or page count makes the dependency check
// inconclusive. Each test passes the old ceiling on the default path.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
)

// osvPages serves pages pages of one vulnerability each; pad grows the
// first page's summary to that many bytes.
func osvPages(t *testing.T, pages, pad int, repeat bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q osvQuery
		_ = json.NewDecoder(r.Body).Decode(&q)
		n := 0
		if q.PageToken != "" {
			fmt.Sscanf(q.PageToken, "p%d", &n)
		}
		next := ""
		switch {
		case repeat:
			next = "p1"
		case n+1 < pages:
			next = fmt.Sprintf("p%d", n+1)
		}
		summary := "page"
		if n == 0 {
			summary = strings.Repeat("s", pad)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"vulns": []map[string]any{{"id": fmt.Sprintf("GHSA-%d", n), "summary": summary}}, "next_page_token": next})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestOSVPastTheOldPageAndBodyCeilingsIsReadWhole(t *testing.T) {
	srv := osvPages(t, 25, 17<<20, false)
	got, err := OSV{BaseURL: srv.URL, Client: srv.Client()}.Query(context.Background(), Query{Name: "minimist", Ecosystem: "npm", Version: "1.0.0"})
	if err != nil {
		t.Fatalf("25 pages, the first of 17 MiB: %v", err)
	}
	if len(got) != 25 {
		t.Fatalf("got %d vulnerabilities, want all 25 pages", len(got))
	}
}

func TestOSVRepeatedPageTokenIsALoudError(t *testing.T) {
	srv := osvPages(t, 0, 1, true)
	if _, err := (OSV{BaseURL: srv.URL, Client: srv.Client()}).Query(context.Background(), Query{Name: "x", Ecosystem: "npm", Version: "1"}); err == nil || !strings.Contains(err.Error(), "repeated a page token") {
		t.Fatalf("err = %v, want the repeated page token error", err)
	}
}

func TestRegistryAnswerPastTheOldCeilingIsRead(t *testing.T) {
	pad := strings.Repeat("r", 5<<20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"pad": pad})
	}))
	t.Cleanup(srv.Close)
	var out struct {
		Pad string `json:"pad"`
	}
	if err := (DepsDev{BaseURL: srv.URL, Client: srv.Client()}).get(context.Background(), srv.URL, &out); err != nil || out.Pad != pad {
		t.Fatalf("a 5 MiB registry answer: err=%v bytes=%d", err, len(out.Pad))
	}
}

// promptModel records what the extraction sent and answers like fakeModel.
type promptModel struct {
	fakeModel
	sent *string
	opts *llm.Options
}

func (m promptModel) CompleteMessages(ctx context.Context, msgs []llm.Message, opts llm.Options) (llm.Response, error) {
	*m.sent += msgs[len(msgs)-1].Content
	*m.opts = opts
	return m.fakeModel.CompleteMessages(ctx, msgs, opts)
}

func TestManifestPastTheOldCeilingIsSentWhole(t *testing.T) {
	line := `+    "pkg": "1.0.0",`
	var lines []string
	for n := 0; n <= (256<<10)/len(line)+1; n++ {
		lines = append(lines, line)
	}
	diff := diffOf(fileDiff("package.json", lines...))
	var sent string
	var opts llm.Options
	changes, err := extractChanges(context.Background(), promptModel{fakeModel: fakeModel{changes: []Change{minimist("1.0.0", "1.2.6")}}, sent: &sent, opts: &opts}, diff, []string{"package.json"})
	if err != nil || len(changes) != 1 {
		t.Fatalf("a manifest past 256 KiB: err=%v changes=%v", err, changes)
	}
	if len(sent) <= 256<<10 || !strings.Contains(sent, "=== package.json") {
		t.Fatalf("the manifest was not sent whole: %d bytes", len(sent))
	}
	if opts.MaxTokens != 0 {
		t.Fatalf("the answer is capped at %d tokens, want no cap", opts.MaxTokens)
	}
}

// A tree whose path list passes the old 256 KiB ceiling is still scanned:
// the check concludes instead of reporting the manifests omitted.
func TestScanPastTheOldPathListCeilingConcludes(t *testing.T) {
	osv := newFakeOSV(t, fixture)
	paths := []string{"go.mod"}
	for n := 0; len(strings.Join(paths, "    ")) <= 256<<10; n++ {
		paths = append(paths, fmt.Sprintf("src/deep/directory/file%06d.go", n))
	}
	scanner := fakeExtractor{pkgs: map[string][]Package{"go.mod": {{Ecosystem: "Go", Name: "example.org/mod", Version: "v1.0.0"}}}}
	r := Scan(context.Background(), ScanInputs{Paths: paths, Model: fakeModel{}, Source: osv.source(), Extractor: scanner})
	if r.Reason == ReasonManifestsOmitted || r.Inconclusive() {
		t.Fatalf("%d paths: reason %q %s", len(paths), r.Reason, r.Detail)
	}
}
