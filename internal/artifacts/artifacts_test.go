package artifacts

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 2, 3, 17, 0, 0, time.UTC)

func zipBytes(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for n, c := range entries {
		f, _ := w.Create(n)
		_, _ = f.Write([]byte(c))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// fakeOSV serves the public layout: /ecosystems.txt and /<eco>/all.zip.
func fakeOSV(t *testing.T, ecos map[string][]byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ecosystems.txt" {
			names := make([]string, 0, len(ecos))
			for n := range ecos {
				names = append(names, n)
			}
			sort.Strings(names)
			_, _ = fmt.Fprintln(w, strings.Join(names, "\n"))
			return
		}
		for n, z := range ecos {
			if r.URL.Path == "/"+n+"/all.zip" {
				_, _ = w.Write(z)
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func scannersFile(t *testing.T) string {
	p := filepath.Join(t.TempDir(), "scanners.yml")
	if err := os.WriteFile(p, []byte("schema: bootstrap-lock-v1\nvuln_scanner_name: trivy\nvuln_scanner_version: 0.73.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func buildAt(t *testing.T, osvURL string, now time.Time) (string, *Manifest) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "dist")
	m, err := Build(context.Background(), BuildOptions{OSVBase: osvURL, ScannersFile: scannersFile(t), OutDir: dir, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	return dir, m
}

type fakeGitHub struct {
	*httptest.Server
	mu   sync.Mutex
	hits map[string]int
}

// serveReleases publishes each dist directory as a release tagged from its
// manifest. dirs are served at /dl/<tag>/<asset>.
func serveReleases(t *testing.T, dirs ...string) *fakeGitHub {
	t.Helper()
	g := &fakeGitHub{hits: map[string]int{}}
	type asset struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	}
	type rel struct {
		TagName   string  `json:"tag_name"`
		CreatedAt string  `json:"created_at"`
		Assets    []asset `json:"assets"`
	}
	var rels []rel
	byTag := map[string]string{}
	mux := http.NewServeMux()
	g.Server = httptest.NewServer(mux)
	t.Cleanup(g.Close)
	for _, d := range dirs {
		m, err := VerifyDir(d)
		if err != nil {
			t.Fatal(err)
		}
		tag, _ := m.Tag()
		gen, _ := m.GeneratedTime()
		byTag[tag] = d
		r := rel{TagName: tag, CreatedAt: gen.Format(time.RFC3339)}
		names := []string{ManifestName}
		for _, f := range m.Files {
			names = append(names, f.Path)
		}
		for _, n := range names {
			r.Assets = append(r.Assets, asset{Name: n, URL: g.URL + "/dl/" + tag + "/" + n})
		}
		rels = append(rels, r)
	}
	// Newest-first is not relied on: shuffle by serving oldest first.
	mux.HandleFunc("/repos/o/r/releases", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(rels)
	})
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		g.hits[r.URL.Path]++
		g.mu.Unlock()
		rest := strings.TrimPrefix(r.URL.Path, "/dl/")
		i := strings.LastIndex(rest, "/")
		d, ok := byTag[rest[:i]]
		if !ok {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(d, rest[i+1:]))
	})
	return g
}

func (g *fakeGitHub) payloadHits() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := 0
	for p, c := range g.hits {
		if !strings.HasSuffix(p, "/"+ManifestName) {
			n += c
		}
	}
	return n
}

func opts(g *fakeGitHub, now time.Time, maxAge int) Options {
	return Options{APIBase: g.URL, Repository: "o/r", CacheDir: "", MaxAgeDays: maxAge, Now: func() time.Time { return now }}
}

func baseEcos(t *testing.T) map[string][]byte {
	return map[string][]byte{
		"Alpha":  zipBytes(t, map[string]string{"A-1.json": `{"id":"A-1"}`}),
		"Beta:2": zipBytes(t, map[string]string{"B-1.json": `{"id":"B-1"}`}),
	}
}

// AC-001 support: a generated artifact is reproducible and self-verifying.
func TestAUR533BuildProducesVerifiableManifest(t *testing.T) {
	srv := fakeOSV(t, baseEcos(t))
	dir, m := buildAt(t, srv.URL, t0)
	if m.Schema != SchemaV1 || m.GeneratedAt != "2026-10-02T03:17:00Z" || !strings.HasPrefix(m.SetDigest, "sha256:") {
		t.Fatalf("manifest header: %+v", m)
	}
	if m.Scanners["vuln_scanner_version"] != "0.73.0" {
		t.Fatalf("scanner versions not recorded: %v", m.Scanners)
	}
	if _, err := VerifyDir(dir); err != nil {
		t.Fatal(err)
	}
	if tag, _ := m.Tag(); tag != "analysis-data/20261002T031700Z" {
		t.Fatalf("tag %q", tag)
	}
}

// AC-001 support: a source failure or a corrupt payload yields no artifact.
func TestAUR533BuildFailsClosedOnBadSource(t *testing.T) {
	bad := fakeOSV(t, map[string][]byte{"Alpha": []byte("not a zip")})
	_, err := Build(context.Background(), BuildOptions{OSVBase: bad.URL, OutDir: filepath.Join(t.TempDir(), "d")})
	if err == nil || !strings.Contains(err.Error(), "not a readable zip") {
		t.Fatalf("want corrupt-zip refusal, got %v", err)
	}
	empty := fakeOSV(t, map[string][]byte{})
	if _, err := Build(context.Background(), BuildOptions{OSVBase: empty.URL, OutDir: filepath.Join(t.TempDir(), "d")}); err == nil {
		t.Fatal("empty listing must not publish")
	}
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	if _, err := Build(context.Background(), BuildOptions{OSVBase: dead.URL, OutDir: filepath.Join(t.TempDir(), "d")}); err == nil {
		t.Fatal("unreachable source must not publish")
	}
}

// AC-005: an ecosystem the source starts publishing lands in the next
// artifact with no code change; the list is read from the source.
func TestAUR533NewEcosystemEntersNextArtifact(t *testing.T) {
	ecos := baseEcos(t)
	srv := fakeOSV(t, ecos)
	_, first := buildAt(t, srv.URL, t0)
	ecos["Brand New:eco"] = zipBytes(t, map[string]string{"N-1.json": `{"id":"N-1"}`})
	dir2, second := buildAt(t, srv.URL, t0.Add(24*time.Hour))
	has := func(m *Manifest, eco string) bool {
		for _, f := range m.Files {
			if f.Ecosystem == eco {
				return true
			}
		}
		return false
	}
	if has(first, "Brand New:eco") || !has(second, "Brand New:eco") {
		t.Fatalf("new ecosystem not picked up from the source listing: first=%v second=%v", first.Files, second.Files)
	}
	if _, err := VerifyDir(dir2); err != nil {
		t.Fatal(err)
	}
	for _, f := range second.Files {
		if strings.ContainsAny(f.Path, ": /") {
			t.Fatalf("unsafe asset name %q", f.Path)
		}
	}
}

// AC-002: within the age limit the artifact is used and its digest and date
// are exposed for the audit record.
func TestAUR533ResolveUsesFreshArtifactAndExposesAudit(t *testing.T) {
	srv := fakeOSV(t, baseEcos(t))
	old, _ := buildAt(t, srv.URL, t0.Add(-48*time.Hour))
	newer, m := buildAt(t, srv.URL, t0)
	g := serveReleases(t, old, newer)
	o := opts(g, t0.Add(3*24*time.Hour), 7)
	o.CacheDir = t.TempDir()
	out := Resolve(context.Background(), o)
	if !out.Usable {
		t.Fatalf("fresh artifact refused: %s", out.Message())
	}
	if out.Digest != m.SetDigest || !out.GeneratedAt.Equal(t0) {
		t.Fatalf("newest release not chosen: %+v", out)
	}
	if out.Source != SourceRemote {
		t.Fatalf("source = %q, want remote", out.Source)
	}
	a := out.Audit()
	if a["analysis_data_digest"] != m.SetDigest || a["analysis_data_generated_at"] != "2026-10-02T03:17:00Z" || a["analysis_data"] != "used" {
		t.Fatalf("audit fields: %v", a)
	}
	if _, err := VerifyDir(out.Dir); err != nil {
		t.Fatal(err)
	}
}

// AC-003: beyond max age the result is unusable with a reason, and no
// payload is downloaded.
func TestAUR533ResolveRefusesStaleArtifact(t *testing.T) {
	srv := fakeOSV(t, baseEcos(t))
	dir, _ := buildAt(t, srv.URL, t0)
	g := serveReleases(t, dir)
	o := opts(g, t0.Add(10*24*time.Hour), 7)
	o.CacheDir = t.TempDir()
	out := Resolve(context.Background(), o)
	if out.Usable || out.Reason != ReasonStale || !strings.Contains(out.Message(), "above max_age_days=7") {
		t.Fatalf("stale artifact must be unusable with its reason, got %+v", out)
	}
	if g.payloadHits() != 0 {
		t.Fatalf("stale artifact payload was downloaded (%d hits)", g.payloadHits())
	}
	if out.Audit()["analysis_data"] != "unusable" {
		t.Fatal("audit must record unusable")
	}
	// The same artifact is fine with a larger limit: the age is what decides.
	o2 := opts(g, t0.Add(10*24*time.Hour), 30)
	o2.CacheDir = t.TempDir()
	if !Resolve(context.Background(), o2).Usable {
		t.Fatal("artifact within a larger limit must be usable")
	}
}

// AC-004: a tampered payload or a tampered manifest is refused.
func TestAUR533ResolveRefusesDigestMismatch(t *testing.T) {
	srv := fakeOSV(t, baseEcos(t))
	for _, tc := range []string{"payload", "manifest-digest", "manifest-file-entry"} {
		t.Run(tc, func(t *testing.T) {
			dir, m := buildAt(t, srv.URL, t0)
			switch tc {
			case "payload":
				p := filepath.Join(dir, m.Files[0].Path)
				b, _ := os.ReadFile(p)
				b[len(b)-3] ^= 0xff
				_ = os.WriteFile(p, b, 0o644)
			case "manifest-digest":
				raw, _ := os.ReadFile(filepath.Join(dir, ManifestName))
				raw = bytes.Replace(raw, []byte(m.SetDigest), []byte("sha256:"+strings.Repeat("0", 64)), 1)
				_ = os.WriteFile(filepath.Join(dir, ManifestName), raw, 0o644)
			case "manifest-file-entry":
				raw, _ := os.ReadFile(filepath.Join(dir, ManifestName))
				raw = bytes.Replace(raw, []byte(m.Files[0].SHA256), []byte("sha256:"+strings.Repeat("1", 64)), 1)
				_ = os.WriteFile(filepath.Join(dir, ManifestName), raw, 0o644)
			}
			g := serveReleases2(t, dir, m)
			o := opts(g, t0.Add(time.Hour), 7)
			o.CacheDir = t.TempDir()
			out := Resolve(context.Background(), o)
			if out.Usable || out.Reason != ReasonDigestMismatch {
				t.Fatalf("tampered artifact must be refused as digest mismatch, got %+v", out)
			}
		})
	}
}

// serveReleases2 serves a directory that VerifyDir would reject (tampered):
// the tag comes from the untampered manifest m.
func serveReleases2(t *testing.T, dir string, m *Manifest) *fakeGitHub {
	t.Helper()
	g := &fakeGitHub{hits: map[string]int{}}
	tag, _ := m.Tag()
	mux := http.NewServeMux()
	g.Server = httptest.NewServer(mux)
	t.Cleanup(g.Close)
	mux.HandleFunc("/repos/o/r/releases", func(w http.ResponseWriter, r *http.Request) {
		assets := []map[string]string{{"name": ManifestName, "browser_download_url": g.URL + "/dl/" + ManifestName}}
		for _, f := range m.Files {
			assets = append(assets, map[string]string{"name": f.Path, "browser_download_url": g.URL + "/dl/" + f.Path})
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{"tag_name": tag, "created_at": m.GeneratedAt, "assets": assets}})
	})
	mux.Handle("/dl/", http.StripPrefix("/dl/", http.FileServer(http.Dir(dir))))
	return g
}

// Without network and without a cached artifact the answer is inconclusive,
// never approved; a cached copy is still held to the age limit.
func TestAUR533ResolveOfflineIsInconclusive(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	out := Resolve(context.Background(), Options{APIBase: dead.URL, Repository: "o/r", CacheDir: t.TempDir(), MaxAgeDays: 7, Now: func() time.Time { return t0 }})
	if out.Usable || out.Reason != ReasonUnavailable {
		t.Fatalf("offline without cache must be unavailable, got %+v", out)
	}

	srv := fakeOSV(t, baseEcos(t))
	dir, _ := buildAt(t, srv.URL, t0)
	g := serveReleases(t, dir)
	cache := t.TempDir()
	o := opts(g, t0.Add(time.Hour), 7)
	o.CacheDir = cache
	if !Resolve(context.Background(), o).Usable {
		t.Fatal("priming run must succeed")
	}
	off := Options{APIBase: dead.URL, Repository: "o/r", CacheDir: cache, MaxAgeDays: 7, Now: func() time.Time { return t0.Add(2 * time.Hour) }}
	if out := Resolve(context.Background(), off); !out.Usable || out.Source != SourceCache {
		t.Fatalf("fresh cached copy should serve offline, marked as cache: %+v", out)
	}
	off.Now = func() time.Time { return t0.Add(9 * 24 * time.Hour) }
	if out := Resolve(context.Background(), off); out.Usable || out.Reason != ReasonStale {
		t.Fatalf("stale cached copy must be refused, got %+v", out)
	}
	// A cached payload altered offline is refused too.
	off.Now = func() time.Time { return t0.Add(2 * time.Hour) }
	entries, _ := os.ReadDir(cache)
	cd := filepath.Join(cache, entries[0].Name())
	m, _ := VerifyDir(cd)
	_ = os.WriteFile(filepath.Join(cd, m.Files[0].Path), []byte("tampered"), 0o644)
	if out := Resolve(context.Background(), off); out.Usable || out.Reason != ReasonDigestMismatch {
		t.Fatalf("tampered cache must be refused, got %+v", out)
	}
}

func TestAUR533ResolveRejectsBadInputs(t *testing.T) {
	if out := Resolve(context.Background(), Options{MaxAgeDays: 0}); out.Usable || out.Reason != ReasonInvalid {
		t.Fatalf("non-positive max age: %+v", out)
	}
	srv := fakeOSV(t, baseEcos(t))
	dir, _ := buildAt(t, srv.URL, t0.Add(2*time.Hour))
	g := serveReleases(t, dir)
	o := opts(g, t0, 7) // artifact generated 2h in the "future"
	o.CacheDir = t.TempDir()
	if out := Resolve(context.Background(), o); out.Usable || out.Reason != ReasonInvalid {
		t.Fatalf("future-dated artifact must be refused: %+v", out)
	}
	if _, err := ParseManifest([]byte(`{"schema":"other"}`)); err == nil {
		t.Fatal("unknown schema accepted")
	}
	m := `{"schema":"` + SchemaV1 + `","generated_at":"2026-10-02T03:17:00Z","files":[{"path":"../x","sha256":"sha256:` + strings.Repeat("a", 64) + `","size":1}],"set_digest":"x"}`
	if _, err := ParseManifest([]byte(m)); err == nil {
		t.Fatal("path traversal accepted")
	}
}

// ---- AC-001: the scripts ----

func buildTool(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "analysis-data")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/analysis-data")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

type scriptRun struct {
	out      string
	err      error
	ghCalls  string
	distPath string
}

func runBuildThenPublish(t *testing.T, osvURL, testCmd string) scriptRun {
	t.Helper()
	bin := buildTool(t)
	work := t.TempDir()
	dist := filepath.Join(work, "dist")
	ghLog := filepath.Join(work, "gh.log")
	fakeGH := filepath.Join(work, "gh")
	_ = os.WriteFile(fakeGH, []byte("#!/usr/bin/env bash\necho \"$@\" >>"+ghLog+"\n"), 0o755)
	repoRoot, _ := filepath.Abs("../..")
	env := append(os.Environ(),
		"AURUM_OSV_BASE="+osvURL, "AURUM_ARTIFACTS_BIN="+bin, "AURUM_SCANNERS_FILE="+scannersFile(t),
		"AURUM_ARTIFACT_TEST_CMD="+testCmd, "AURUM_GH="+fakeGH)
	run := func(script string) (string, error) {
		c := exec.Command("bash", filepath.Join(repoRoot, "scripts/artifacts", script), dist)
		c.Env = env
		c.Dir = work
		b, err := c.CombinedOutput()
		return string(b), err
	}
	var r scriptRun
	r.distPath = dist
	r.out, r.err = run("build.sh")
	// The workflow runs publish only when the build step succeeded; the
	// script itself must also refuse without the marker.
	if r.err == nil {
		o, err := run("publish.sh")
		r.out += o
		r.err = err
	}
	b, _ := os.ReadFile(ghLog)
	r.ghCalls = string(b)
	return r
}

func TestAUR533PassingTestsPublishAndFailingTestsDoNot(t *testing.T) {
	srv := fakeOSV(t, baseEcos(t))
	ok := runBuildThenPublish(t, srv.URL, `test -f "$AURUM_ARTIFACT_DIR/manifest.json"`)
	if ok.err != nil {
		t.Fatalf("passing run failed: %v\n%s", ok.err, ok.out)
	}
	if !strings.Contains(ok.ghCalls, "release create analysis-data/") || !strings.Contains(ok.ghCalls, "--draft") || !strings.Contains(ok.ghCalls, "release edit analysis-data/") {
		t.Fatalf("publish did not run the release flow:\n%s", ok.ghCalls)
	}
	if !strings.Contains(ok.ghCalls, "manifest.json") || !strings.Contains(ok.ghCalls, "osv-Alpha.zip") || !strings.Contains(ok.ghCalls, "osv-Beta_2.zip") {
		t.Fatalf("assets missing from release create:\n%s", ok.ghCalls)
	}

	bad := runBuildThenPublish(t, srv.URL, `echo "test failed"; exit 1`)
	if bad.err == nil {
		t.Fatalf("failing artifact test must fail the run:\n%s", bad.out)
	}
	if bad.ghCalls != "" {
		t.Fatalf("a failing test must not publish; gh was called:\n%s", bad.ghCalls)
	}
	if _, err := os.Stat(bad.distPath + ".tests-passed"); err == nil {
		t.Fatal("tests-passed marker written despite failing tests")
	}
}

func TestAUR533PublishRefusesUntestedOrChangedArtifact(t *testing.T) {
	srv := fakeOSV(t, baseEcos(t))
	dir, _ := buildAt(t, srv.URL, time.Now())
	repoRoot, _ := filepath.Abs("../..")
	work := t.TempDir()
	ghLog := filepath.Join(work, "gh.log")
	fakeGH := filepath.Join(work, "gh")
	_ = os.WriteFile(fakeGH, []byte("#!/usr/bin/env bash\necho \"$@\" >>"+ghLog+"\n"), 0o755)
	pub := func() error {
		c := exec.Command("bash", filepath.Join(repoRoot, "scripts/artifacts/publish.sh"), dir)
		c.Env = append(os.Environ(), "AURUM_GH="+fakeGH)
		return c.Run()
	}
	if pub() == nil {
		t.Fatal("publish without the tests-passed marker must fail")
	}
	m, _ := VerifyDir(dir)
	tag, _ := m.Tag()
	_ = os.WriteFile(dir+".tests-passed", []byte(m.SetDigest+"\n"+tag+"\n"), 0o644)
	p := filepath.Join(dir, m.Files[0].Path)
	b, _ := os.ReadFile(p)
	_ = os.WriteFile(p, append(b, 'x'), 0o644)
	if pub() == nil {
		t.Fatal("publish of a file changed after the tests must fail")
	}
	if _, err := os.Stat(ghLog); err == nil {
		t.Fatal("gh must not be called for a refused publish")
	}
}

// TestGeneratedArtifact is the step the workflow runs against the artifact it
// just built (AURUM_ARTIFACT_DIR). Without that variable it is skipped.
func TestGeneratedArtifact(t *testing.T) {
	dir := os.Getenv("AURUM_ARTIFACT_DIR")
	if dir == "" {
		t.Skip("AURUM_ARTIFACT_DIR not set: only meaningful in the build workflow")
	}
	m, err := VerifyDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	gen, _ := m.GeneratedTime()
	if time.Since(gen) > time.Hour || time.Until(gen) > maxClockSkew {
		t.Fatalf("artifact is not freshly generated: %s", m.GeneratedAt)
	}
	eco := 0
	for _, f := range m.Files {
		if f.Kind != "osv" {
			continue
		}
		eco++
		zr, err := zip.OpenReader(filepath.Join(dir, f.Path))
		if err != nil {
			t.Fatalf("%s: %v", f.Path, err)
		}
		zr.Close()
	}
	if eco == 0 {
		t.Fatal("artifact has no OSV ecosystem")
	}
	if len(m.Scanners) == 0 {
		t.Fatal("artifact records no scanner versions")
	}
}

// The workflow must be scheduled, gate publishing on the tested build and pin
// every action to a commit SHA.
func TestAUR533WorkflowOrderingAndPins(t *testing.T) {
	raw, err := os.ReadFile("../../.github/workflows/analysis-data.yml")
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		On   map[string]any `yaml:"on"`
		Jobs map[string]struct {
			Needs           any    `yaml:"needs"`
			If              string `yaml:"if"`
			ContinueOnError any    `yaml:"continue-on-error"`
			Steps           []struct {
				Uses            string `yaml:"uses"`
				Run             string `yaml:"run"`
				If              string `yaml:"if"`
				ContinueOnError any    `yaml:"continue-on-error"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatal(err)
	}
	if _, ok := wf.On["schedule"]; !ok {
		t.Fatalf("workflow must be scheduled: %v", wf.On)
	}
	if _, ok := wf.On["workflow_dispatch"]; !ok {
		t.Fatalf("workflow must be scheduled and manual: %v", wf.On)
	}
	pub, ok := wf.Jobs["publish"]
	if !ok || fmt.Sprint(pub.Needs) != "build" || pub.If != "" {
		t.Fatalf("publish must depend on build with no override condition: %+v", pub)
	}
	pin := regexp.MustCompile(`@[0-9a-f]{40}( |$)`)
	var buildRuns, publishRuns string
	for name, j := range wf.Jobs {
		if j.ContinueOnError != nil {
			t.Fatalf("job %s: continue-on-error would let a failed test publish", name)
		}
		for _, s := range j.Steps {
			if s.ContinueOnError != nil || strings.Contains(s.If, "always()") || strings.Contains(s.If, "failure()") {
				t.Fatalf("job %s: step bypasses the failure gate: %+v", name, s)
			}
			if s.Uses != "" && !pin.MatchString(s.Uses) {
				t.Fatalf("job %s: action not pinned by SHA: %s", name, s.Uses)
			}
			if name == "build" {
				buildRuns += s.Run
			} else {
				publishRuns += s.Run
			}
		}
	}
	if !strings.Contains(buildRuns, "scripts/artifacts/build.sh") || !strings.Contains(publishRuns, "scripts/artifacts/publish.sh") {
		t.Fatal("workflow must run build.sh in build and publish.sh in publish")
	}
}

// The workflow's test step really runs against a freshly built artifact (and
// is not skipped).
func TestAUR533GeneratedArtifactStepRunsAgainstFreshBuild(t *testing.T) {
	srv := fakeOSV(t, baseEcos(t))
	dir, _ := buildAt(t, srv.URL, time.Now())
	step := func() ([]byte, error) {
		c := exec.Command("go", "test", "-count=1", "-v", "-run", "^TestGeneratedArtifact$", ".")
		c.Env = append(os.Environ(), "AURUM_ARTIFACT_DIR="+dir)
		return c.CombinedOutput()
	}
	out, err := step()
	if err != nil || !strings.Contains(string(out), "--- PASS: TestGeneratedArtifact") {
		t.Fatalf("generated-artifact step: %v\n%s", err, out)
	}
	m, _ := VerifyDir(dir)
	p := filepath.Join(dir, m.Files[0].Path)
	b, _ := os.ReadFile(p)
	_ = os.WriteFile(p, append(b, 'x'), 0o644)
	if out, err := step(); err == nil || !strings.Contains(string(out), "FAIL") {
		t.Fatalf("tampered artifact must fail the step:\n%s", out)
	}
}
