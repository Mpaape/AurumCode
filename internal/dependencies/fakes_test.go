package dependencies

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// fakeOSV serves testdata/osv-fake.json as the OSV /v1/query API: the key
// is "<ecosystem>/<name>@<version>" ("<ecosystem>/<name>" for a whole
// package). A key the fixture lacks answers with no advisories, as OSV does.
type fakeOSV struct {
	mu      sync.Mutex
	server  *httptest.Server
	queries []string
	status  int
	date    time.Time
}

func newFakeOSV(t *testing.T, fixture string) *fakeOSV {
	t.Helper()
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	var answers map[string]json.RawMessage
	if err := json.Unmarshal(data, &answers); err != nil {
		t.Fatal(err)
	}
	f := &fakeOSV{status: http.StatusOK}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q osvQuery
		if r.URL.Path != "/v1/query" || json.NewDecoder(r.Body).Decode(&q) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		key := q.Package.Ecosystem + "/" + q.Package.Name
		if q.Version != "" {
			key += "@" + q.Version
		}
		f.mu.Lock()
		f.queries = append(f.queries, key)
		status, date := f.status, f.date
		f.mu.Unlock()
		if !date.IsZero() {
			w.Header().Set("Date", date.UTC().Format(http.TimeFormat))
		}
		if status != http.StatusOK {
			http.Error(w, "unavailable", status)
			return
		}
		body, ok := answers[key]
		if !ok {
			body = json.RawMessage(`{}`)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeOSV) source() OSV { return OSV{BaseURL: f.server.URL, Client: f.server.Client()} }

func (f *fakeOSV) asked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.queries...)
}

// fakeModel answers the select prompt with manifests and the extract
// prompt with changes, as JSON.
type fakeModel struct {
	manifests  []string
	changes    []Change
	suspicions []map[string]any
	err        error
}

func (m fakeModel) CompleteMessages(_ context.Context, msgs []llm.Message, _ llm.Options) (llm.Response, error) {
	if m.err != nil {
		return llm.Response{}, m.err
	}
	var body any
	if strings.Contains(msgs[0].Content, `{"suspicions"`) {
		body = map[string]any{"suspicions": m.suspicions}
	} else if strings.Contains(msgs[0].Content, `{"manifests"`) {
		body = map[string]any{"manifests": m.manifests}
	} else {
		body = map[string]any{"changes": m.changes}
	}
	data, _ := json.Marshal(body)
	return llm.Response{Text: string(data)}, nil
}

// fakeExtractor is the scanner's extraction per manifest; a manifest absent
// from pkgs is not recognized. missing makes the scanner absent.
type fakeExtractor struct {
	pkgs    map[string][]Package
	missing bool
}

func (x fakeExtractor) Extract(_ context.Context, _ string, paths []string) (map[string][]Package, error) {
	if x.missing {
		return nil, errors.Join(ErrScannerMissing, errors.New("osv-scanner not in PATH"))
	}
	out := map[string][]Package{}
	for _, p := range paths {
		if pkgs, ok := x.pkgs[p]; ok {
			out[p] = pkgs
		}
	}
	return out, nil
}

// fileDiff is one file of a diff whose hunk holds lines as written ("+",
// "-" or " " prefixed).
func fileDiff(path string, lines ...string) types.DiffFile {
	return types.DiffFile{Path: path, Hunks: []types.DiffHunk{{OldStart: 1, NewStart: 1, Lines: lines}}}
}

func diffOf(files ...types.DiffFile) *types.Diff { return &types.Diff{Files: files} }

// npmBump is a lockfile bumping minimist from base to head, beside a source
// file the model must not read as a manifest.
func npmBump(base, head string) *types.Diff {
	return diffOf(
		fileDiff("app/package-lock.json", `     "node_modules/minimist": {`, `-      "version": "`+base+`",`, `+      "version": "`+head+`",`),
		fileDiff("app/index.js", `+const x = require("minimist");`),
	)
}

func minimist(base, head string) Change {
	return Change{Manifest: "app/package-lock.json", Ecosystem: "npm", Name: "minimist", Base: base, Head: head}
}

func findingsOf(r Report, status Status) []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Status == status {
			out = append(out, f)
		}
	}
	return out
}

// fakeRegistry serves fixed metadata per package name; fail makes it
// unreachable.
type fakeRegistry struct {
	meta map[string]Metadata
	fail bool
}

func (r fakeRegistry) Metadata(_ context.Context, c Change) (Metadata, error) {
	if r.fail {
		return nil, errors.New("registry unreachable")
	}
	m, ok := r.meta[c.Name]
	if !ok {
		return nil, ErrNoSystem
	}
	return m, nil
}
