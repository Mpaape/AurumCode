package pipeline

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
	"gopkg.in/yaml.v3"
)

// pagesBlockStart/End mirror the delimiters the scaffold writes around its
// generated listing. The assertions below read only inside that block: prose a
// consumer wrote above it is none of this test's business.
const (
	pagesBlockStart = "<!-- aurumcode:pages:start -->"
	pagesBlockEnd   = "<!-- aurumcode:pages:end -->"
)

var markdownDestinationRE = regexp.MustCompile(`\]\(([^)]*)\)`)

// unsetEnvForTest removes a variable for the duration of the test and restores it
// afterwards.
//
// t.Setenv(name, "") cannot be used for this: it SETS the variable to the empty
// string, which this code reads through os.LookupEnv as "the caller explicitly
// asked for the root". GITHUB_REPOSITORY is also genuinely populated on an
// Actions runner, so a test that only clears it locally would exercise a
// different ladder rung in CI than on a laptop.
func unsetEnvForTest(t *testing.T, name string) {
	t.Helper()

	previous, existed := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("unset %s: %v", name, err)
	}

	t.Cleanup(func() {
		if !existed {
			_ = os.Unsetenv(name)
			return
		}
		_ = os.Setenv(name, previous)
	})
}

// rootAbsoluteDestinations returns every root-absolute markdown link destination
// inside the generated page block.
//
// It fails the test when the block is missing or holds no root-absolute
// destination at all. That is deliberate: a harness that "passes" because it
// examined zero cases is worse than no harness, and every caller below asserts a
// property *of each* destination, which an empty slice satisfies vacuously.
func rootAbsoluteDestinations(t *testing.T, index string) []string {
	t.Helper()

	start := strings.Index(index, pagesBlockStart)
	end := strings.Index(index, pagesBlockEnd)
	if start < 0 || end < 0 || end < start {
		t.Fatalf("index.md has no generated page block (%s ... %s):\n%s",
			pagesBlockStart, pagesBlockEnd, index)
	}

	block := index[start:end]

	var destinations []string
	for _, match := range markdownDestinationRE.FindAllStringSubmatch(block, -1) {
		if strings.HasPrefix(match[1], "/") {
			destinations = append(destinations, match[1])
		}
	}

	if len(destinations) == 0 {
		t.Fatalf("generated page block lists no root-absolute link, so every "+
			"per-link assertion below would pass without examining anything:\n%s", block)
	}

	return destinations
}

// runScaffoldPipeline drives the real pipeline over the shared fixture and
// returns what production code left on disk. Only the external extractor is a
// double; discovery, normalization (which writes the permalink) and the scaffold
// all run for real.
func runScaffoldPipeline(t *testing.T, config *ExtractorPipelineConfig) (index, reference, siteConfig string) {
	t.Helper()

	configPath := filepath.Join(config.DocsDir, "_config.yml")

	// Invariant that makes the _config.yml assertions non-vacuous: if the
	// fixture already shipped a config, a later "baseurl is right" assertion
	// would be proving something this test wrote.
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("fixture must not ship a _config.yml, else the assertions below "+
			"would read the test's own bytes (stat err = %v)", err)
	}

	p := NewExtractorPipeline(config, site.NewMockRunner(), nil)
	if err := p.RegisterExtractor(&fileWritingExtractor{
		lang:  extractors.LanguageGo,
		files: map[string]string{"ledger.md": gomarkdocOutput},
	}); err != nil {
		t.Fatalf("RegisterExtractor failed: %v", err)
	}

	if err := p.Run(context.Background()); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	return readGenerated(t, filepath.Join(config.DocsDir, "index.md")),
		readGeneratedOptional(t, filepath.Join(config.DocsDir, "reference.md")),
		readGenerated(t, configPath)
}

// parsedBaseURL reads the baseurl key out of the _config.yml production wrote.
// The provenance comment is checked first: it is the proof that these bytes came
// from scaffold.writeConfig and not from the test.
func parsedBaseURL(t *testing.T, siteConfig string) (value string, present bool) {
	t.Helper()

	const provenance = "# Minimal Jekyll configuration written by AurumCode."
	if !strings.Contains(siteConfig, provenance) {
		t.Fatalf("_config.yml lacks the provenance line %q, so it was not written "+
			"by the production scaffold:\n%s", provenance, siteConfig)
	}

	var parsed struct {
		BaseURL *string `yaml:"baseurl"`
	}
	if err := yaml.Unmarshal([]byte(siteConfig), &parsed); err != nil {
		t.Fatalf("_config.yml is not valid YAML (%v):\n%s", err, siteConfig)
	}

	if parsed.BaseURL == nil {
		return "", false
	}
	return *parsed.BaseURL, true
}

// TestExtractorPipeline_Run_UnderBasePath_LinksResolve is the regression test for
// the defect a browser proved: published under a project Pages base path, 100% of
// the documentation links on the index answered 404 because the scaffold emitted
// the permalink raw. kramdown copies a markdown destination into the href
// verbatim - it knows nothing about Jekyll's baseurl - so the prefix has to be in
// the markdown itself.
//
// The two effects are asserted separately on purpose: the link prefix is what
// fixes the 404, and the baseurl key is what fixes the theme's assets. The proof
// run showed a site with baseurl declared and STILL 404 on every link, so one
// assertion cannot stand in for the other.
func TestExtractorPipeline_Run_UnderBasePath_LinksResolve(t *testing.T) {
	_, config := newScaffoldFixture(t)
	config.BaseURL = "/widgetlib"
	config.BaseURLDeclared = true

	index, _, siteConfig := runScaffoldPipeline(t, config)

	// AUR-484 option (b): single page, so its link is inlined into index.md;
	// no reference.md is written.
	// Effect 1: the link the browser follows.
	if !strings.Contains(index, "](/widgetlib/go/ledger/)") {
		t.Errorf("index.md does not link to /widgetlib/go/ledger/: every documentation "+
			"link would answer 404 under the base path:\n%s", index)
	}
	if strings.Contains(index, "](/go/ledger/)") {
		t.Errorf("index.md still carries the unprefixed destination /go/ledger/:\n%s", index)
	}

	destinations := rootAbsoluteDestinations(t, index)
	for _, destination := range destinations {
		if !strings.HasPrefix(destination, "/widgetlib/") {
			t.Errorf("root-absolute destination %q is not under the base path /widgetlib/; "+
				"it would answer 404", destination)
		}
	}

	// Effect 2: the key the theme resolves its assets through.
	value, present := parsedBaseURL(t, siteConfig)
	if !present {
		t.Fatalf("_config.yml declares no baseurl: the theme's stylesheet and every "+
			"relative_url would resolve off the base path:\n%s", siteConfig)
	}
	if value != "/widgetlib" {
		t.Errorf("_config.yml baseurl = %q, want %q", value, "/widgetlib")
	}

	// Cross-check: the two independent effects have to agree, or the site is
	// internally inconsistent even though both assertions above passed.
	for _, destination := range destinations {
		if !strings.HasPrefix(destination, value+"/") {
			t.Errorf("destination %q is not under the declared baseurl %q", destination, value)
		}
	}
}

// TestExtractorPipeline_Run_DefaultProjectPagesSite is the exact scenario the
// browser proof refuted, reproduced with ZERO caller configuration.
//
// A repository that enables GitHub Pages gets owner.github.io/<repo>/ by
// default. Before this change the index linked "/go/ledger/", which on that host
// is a different path than "/widgetlib/go/ledger/" and answered 404 for 100% of
// the documentation links. Nothing but GITHUB_REPOSITORY, which every Actions
// run sets, is needed to know better.
func TestExtractorPipeline_Run_DefaultProjectPagesSite(t *testing.T) {
	unsetEnvForTest(t, "GITHUB_REPOSITORY")
	t.Setenv("GITHUB_REPOSITORY", "owner/widgetlib")

	_, config := newScaffoldFixture(t)
	// The caller declares nothing at all: this is the out-of-the-box path.
	if config.BaseURLDeclared {
		t.Fatal("fixture must not declare a base path; this case is about deriving one")
	}

	index, _, siteConfig := runScaffoldPipeline(t, config)

	// AUR-484 option (b): single page, so its link is inlined into index.md.
	if !strings.Contains(index, "](/widgetlib/go/ledger/)") {
		t.Errorf("index.md does not link to /widgetlib/go/ledger/ although GITHUB_REPOSITORY "+
			"is owner/widgetlib: this is the 404 the browser proof recorded:\n%s", index)
	}

	for _, destination := range rootAbsoluteDestinations(t, index) {
		if !strings.HasPrefix(destination, "/widgetlib/") {
			t.Errorf("destination %q would answer 404 on owner.github.io/widgetlib/", destination)
		}
	}

	value, present := parsedBaseURL(t, siteConfig)
	if !present || value != "/widgetlib" {
		t.Errorf("_config.yml baseurl = (%q, present=%t), want (%q, true)", value, present, "/widgetlib")
	}
}

// TestExtractorPipeline_Run_PublishedAtRoot_IsUnchanged protects everyone who
// publishes at a domain root. The base path work must not put a prefix - or a
// stray double slash - in front of a link that was already correct.
//
// The "declared empty" row is the one that makes a custom domain servable: the
// caller has to be able to say "root" in a way that outranks any derivation.
func TestExtractorPipeline_Run_PublishedAtRoot_IsUnchanged(t *testing.T) {
	testCases := []struct {
		name        string
		baseURL     string
		declared    bool
		seedConfig  string
		wantBaseKey bool
	}{
		{name: "caller declared root explicitly", baseURL: "", declared: true},
		{name: "caller declared nothing", baseURL: "", declared: false},
		{name: "caller declared a bare slash", baseURL: "/", declared: true},
		{
			name:       "existing config declares root and caller is silent",
			declared:   false,
			seedConfig: "# hand written\nbaseurl: \"\"\ntitle: \"custom domain\"\n",
		},
	}

	if len(testCases) == 0 {
		t.Fatal("no cases: the loop below would assert nothing")
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// GITHUB_REPOSITORY is genuinely set on Actions runners. Left alone,
			// derivation would prefix these links there and not here, and the
			// suite would mean something different in CI than on a laptop.
			unsetEnvForTest(t, "GITHUB_REPOSITORY")

			_, config := newScaffoldFixture(t)
			config.BaseURL = tc.baseURL
			config.BaseURLDeclared = tc.declared

			var index, reference string
			if tc.seedConfig != "" {
				if err := os.MkdirAll(config.DocsDir, 0o755); err != nil {
					t.Fatalf("create docs dir: %v", err)
				}
				configPath := filepath.Join(config.DocsDir, "_config.yml")
				if err := os.WriteFile(configPath, []byte(tc.seedConfig), 0o644); err != nil {
					t.Fatalf("seed _config.yml: %v", err)
				}

				p := NewExtractorPipeline(config, site.NewMockRunner(), nil)
				if err := p.RegisterExtractor(&fileWritingExtractor{
					lang:  extractors.LanguageGo,
					files: map[string]string{"ledger.md": gomarkdocOutput},
				}); err != nil {
					t.Fatalf("RegisterExtractor failed: %v", err)
				}
				if err := p.Run(context.Background()); err != nil {
					t.Fatalf("Run failed: %v", err)
				}
				index = readGenerated(t, filepath.Join(config.DocsDir, "index.md"))
				reference = readGeneratedOptional(t, filepath.Join(config.DocsDir, "reference.md"))
			} else {
				var siteConfig string
				index, reference, siteConfig = runScaffoldPipeline(t, config)

				// A greenfield root publisher must get no baseurl key at all.
				// An empty-but-present key is the shape that a normalizer
				// returning "/" instead of "" would produce, and it makes the
				// theme emit "//assets/...", which the browser reads as a
				// different host.
				if _, present := parsedBaseURL(t, siteConfig); present {
					t.Errorf("_config.yml declares a baseurl although the site publishes "+
						"at the root:\n%s", siteConfig)
				}
			}

			_ = reference // AUR-484 option (b): single page, no reference.md written
			if !strings.Contains(index, "](/go/ledger/)") {
				t.Errorf("index.md lost the root-published destination /go/ledger/:\n%s", index)
			}
			for _, destination := range rootAbsoluteDestinations(t, index) {
				if strings.HasPrefix(destination, "//") {
					t.Errorf("destination %q starts with a double slash: a browser reads "+
						"that as a protocol-relative URL and leaves the site", destination)
				}
			}
		})
	}
}

// missingBaseURLWarningMarker is the phrase that identifies the one diagnostic
// under test here, so the "stays silent" cases can distinguish it from the
// routine "[Pipeline] Publishing under base path ..." line that a base-path run
// also emits. Asserting "logged nothing at all" would pass for the wrong reason.
const missingBaseURLWarningMarker = "declares no baseurl"

// runPipelineCapturingLog drives the real pipeline over the shared fixture and
// returns everything production wrote to the standard logger.
//
// The seed is written before the run so the pipeline meets a consumer-owned
// _config.yml, which is the only situation the warning exists for: writeConfig
// refuses to overwrite that file, so the run prefixes the links but cannot add
// the baseurl key the theme's assets resolve through.
func runPipelineCapturingLog(t *testing.T, config *ExtractorPipelineConfig, seedConfig string) (logged, index, reference, siteConfig string) {
	t.Helper()

	configPath := filepath.Join(config.DocsDir, "_config.yml")

	if seedConfig != "" {
		if err := os.MkdirAll(config.DocsDir, 0o755); err != nil {
			t.Fatalf("create docs dir: %v", err)
		}
		if err := os.WriteFile(configPath, []byte(seedConfig), 0o644); err != nil {
			t.Fatalf("seed _config.yml: %v", err)
		}
	} else if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("fixture must not ship a _config.yml when no seed was asked for "+
			"(stat err = %v)", err)
	}

	var captured bytes.Buffer
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&captured)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	}()

	p := NewExtractorPipeline(config, site.NewMockRunner(), nil)
	if err := p.RegisterExtractor(&fileWritingExtractor{
		lang:  extractors.LanguageGo,
		files: map[string]string{"ledger.md": gomarkdocOutput},
	}); err != nil {
		t.Fatalf("RegisterExtractor failed: %v", err)
	}

	if err := p.Run(context.Background()); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	logged = captured.String()
	if logged == "" {
		t.Fatal("production logged nothing at all: the capture is not wired to the " +
			"logger the pipeline uses, so every assertion below would be vacuous")
	}

	return logged, readGenerated(t, filepath.Join(config.DocsDir, "index.md")),
		readGeneratedOptional(t, filepath.Join(config.DocsDir, "reference.md")),
		readGenerated(t, configPath)
}

// TestExtractorPipeline_ConsumerConfigWithoutBaseURL_IsWarned covers the half of
// the fix that production code cannot deliver on its own.
//
// A repository that already ships its own _config.yml keeps it: writeConfig
// never overwrites a consumer file. So the run prefixes every generated link and
// leaves the config without a baseurl key. The links then work while the theme's
// stylesheet, its favicon and every other relative_url asset resolve off the
// base path and 404. Nothing detects that from the outside - the pages render,
// just unstyled - so the run has to say which line to add, and it has to say the
// line, not merely that "something is wrong".
func TestExtractorPipeline_ConsumerConfigWithoutBaseURL_IsWarned(t *testing.T) {
	unsetEnvForTest(t, "GITHUB_REPOSITORY")

	const seed = "# hand written by the consumer\ntitle: \"Widget Library\"\ntheme: minima\n"

	_, config := newScaffoldFixture(t)
	config.BaseURL = "/widgetlib"
	config.BaseURLDeclared = true

	logged, index, _, siteConfig := runPipelineCapturingLog(t, config, seed)

	// Precondition, asserted rather than assumed: this is genuinely the
	// half-configured site. If production had overwritten the consumer's file,
	// or had failed to prefix the links, the warning would be about nothing.
	if siteConfig != seed {
		t.Fatalf("production rewrote the consumer's _config.yml, so the warning under "+
			"test guards a situation that no longer happens:\ngot:\n%s\nwant:\n%s",
			siteConfig, seed)
	}
	if _, declared := configuredBaseURL(filepath.Join(config.DocsDir, "_config.yml")); declared {
		t.Fatalf("the seeded _config.yml declares a baseurl after all:\n%s", siteConfig)
	}
	// AUR-484 option (b): single page, so its link is inlined into index.md.
	if !strings.Contains(index, "](/widgetlib/go/ledger/)") {
		t.Fatalf("links were not prefixed, so this is not the links-work-assets-404 "+
			"case the warning describes:\n%s", index)
	}

	if !strings.Contains(logged, missingBaseURLWarningMarker) {
		t.Fatalf("the run published a site whose links use /widgetlib and whose theme "+
			"assets do not, and said nothing about it. Logged:\n%s", logged)
	}

	// The remediation has to be copy-pasteable. A warning that says "check your
	// baseurl" leaves the consumer to guess the value, and guessing "widgetlib"
	// or "/widgetlib/" produces a differently broken site.
	const remediation = `baseurl: "/widgetlib"`
	if !strings.Contains(logged, remediation) {
		t.Errorf("the warning does not name the exact line to add (%s); a consumer "+
			"cannot act on it. Logged:\n%s", remediation, logged)
	}

	// It also has to name the file, because the pipeline writes into DocsDir,
	// which is not necessarily where the consumer thinks their config lives.
	wantPath := filepath.ToSlash(filepath.Join(config.DocsDir, "_config.yml"))
	if !strings.Contains(logged, wantPath) {
		t.Errorf("the warning does not name the file to edit (%s). Logged:\n%s", wantPath, logged)
	}
}

// TestExtractorPipeline_ConsumerConfigBaseURLWarning_StaysSilent is the converse.
//
// A warning that fires on a correctly configured site is noise, and noise is how
// a real warning gets ignored. Each row below is a site that is already whole,
// for a different reason.
func TestExtractorPipeline_ConsumerConfigBaseURLWarning_StaysSilent(t *testing.T) {
	testCases := []struct {
		name       string
		baseURL    string
		declared   bool
		seedConfig string
		why        string
	}{
		{
			name:       "consumer config already declares the baseurl",
			baseURL:    "/widgetlib",
			declared:   true,
			seedConfig: "title: \"Widget Library\"\nbaseurl: \"/widgetlib\"\n",
			why:        "the assets already resolve through the same prefix as the links",
		},
		{
			name:       "site publishes at the root",
			baseURL:    "",
			declared:   true,
			seedConfig: "title: \"Widget Library\"\ntheme: minima\n",
			why:        "there is no prefix for the assets to be missing",
		},
		{
			name:     "scaffold wrote the config itself",
			baseURL:  "/widgetlib",
			declared: true,
			why:      "writeConfig put the baseurl key in the file it created",
		},
	}

	if len(testCases) == 0 {
		t.Fatal("no cases: the loop below would assert nothing")
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			unsetEnvForTest(t, "GITHUB_REPOSITORY")

			_, config := newScaffoldFixture(t)
			config.BaseURL = tc.baseURL
			config.BaseURLDeclared = tc.declared

			logged, _, _, siteConfig := runPipelineCapturingLog(t, config, tc.seedConfig)

			if strings.Contains(logged, missingBaseURLWarningMarker) {
				t.Errorf("warned about a missing baseurl although %s.\n_config.yml:\n%s\nlogged:\n%s",
					tc.why, siteConfig, logged)
			}
		})
	}
}

// TestNormalizeBasePath covers the shapes a caller really supplies. The scaffold's
// own applyBaseURL only trims a trailing slash, so everything else has to be
// resolved before the value reaches it.
func TestNormalizeBasePath(t *testing.T) {
	testCases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty", raw: "", want: ""},
		{name: "bare slash", raw: "/", want: ""},
		{name: "double slash only", raw: "//", want: ""},
		{name: "already canonical", raw: "/repo", want: "/repo"},
		{name: "trailing slash", raw: "/repo/", want: "/repo"},
		{name: "doubled trailing slash", raw: "/repo//", want: "/repo"},
		{name: "no leading slash", raw: "repo", want: "/repo"},
		{name: "surrounding whitespace", raw: "  /repo  ", want: "/repo"},
		{name: "nested path", raw: "/org/repo/", want: "/org/repo"},
		// configure-pages emits an absolute base_url; a consumer will wire it
		// straight through. Keeping the scheme and host would send every link
		// off to an absolute URL the site cannot control.
		{name: "absolute url from configure-pages", raw: "https://owner.github.io/repo", want: "/repo"},
		// A protocol-relative value makes the browser LEAVE the site.
		{name: "protocol relative", raw: "//evil.example.com", want: "/evil.example.com"},
		{name: "protocol relative with path", raw: "//evil.example.com/x", want: "/evil.example.com/x"},
	}

	if len(testCases) == 0 {
		t.Fatal("no cases: the loop below would assert nothing")
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeBasePath(tc.raw); got != tc.want {
				t.Errorf("normalizeBasePath(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// TestDeriveBasePathFromRepository covers the guess made when nobody declared
// anything. It is a guess: right for a project site, right for a user site, and
// wrong for a custom domain - which is why it sits at the bottom of the ladder.
func TestDeriveBasePathFromRepository(t *testing.T) {
	testCases := []struct {
		name       string
		repository string
		want       string
	}{
		{name: "project site", repository: "owner/widgetlib", want: "/widgetlib"},
		{name: "user site", repository: "owner/owner.github.io", want: ""},
		{name: "user site cased differently", repository: "Owner/owner.GITHUB.io", want: ""},
		{name: "org site", repository: "acme/acme.github.io", want: ""},
		{name: "not set", repository: "", want: ""},
		{name: "malformed without slash", repository: "widgetlib", want: ""},
		{name: "malformed empty name", repository: "owner/", want: ""},
	}

	if len(testCases) == 0 {
		t.Fatal("no cases: the loop below would assert nothing")
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := deriveBasePathFromRepository(tc.repository); got != tc.want {
				t.Errorf("deriveBasePathFromRepository(%q) = %q, want %q", tc.repository, got, tc.want)
			}
		})
	}
}

// TestExtractorPipeline_BasePathPrecedence pins the ladder. The interesting rung
// is the middle one: a baseurl already on disk is what Jekyll actually reads, so
// it beats a derivation from GITHUB_REPOSITORY, which is only a guess.
func TestExtractorPipeline_BasePathPrecedence(t *testing.T) {
	testCases := []struct {
		name       string
		declared   bool
		baseURL    string
		seedConfig string
		repository string
		want       string
	}{
		{
			name: "caller beats everything", declared: true, baseURL: "/from-caller",
			seedConfig: "baseurl: \"/from-caller\"\n", repository: "owner/from-env",
			want: "/from-caller",
		},
		{
			name:       "config on disk beats the derivation",
			seedConfig: "baseurl: \"/from-disk\"\n", repository: "owner/from-env",
			want: "/from-disk",
		},
		{
			name:       "config declaring root beats the derivation",
			seedConfig: "baseurl: \"\"\n", repository: "owner/from-env",
			want: "",
		},
		{
			name:       "derivation is used when nothing else says anything",
			repository: "owner/from-env", want: "/from-env",
		},
		{
			name:       "config without a baseurl key falls through to the derivation",
			seedConfig: "title: \"x\"\n", repository: "owner/from-env", want: "/from-env",
		},
		{name: "nothing at all means root", want: ""},
	}

	if len(testCases) == 0 {
		t.Fatal("no cases: the loop below would assert nothing")
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			unsetEnvForTest(t, "GITHUB_REPOSITORY")
			if tc.repository != "" {
				t.Setenv("GITHUB_REPOSITORY", tc.repository)
			}

			docsDir := t.TempDir()
			if tc.seedConfig != "" {
				if err := os.WriteFile(filepath.Join(docsDir, "_config.yml"),
					[]byte(tc.seedConfig), 0o644); err != nil {
					t.Fatalf("seed _config.yml: %v", err)
				}
			}

			p := NewExtractorPipeline(&ExtractorPipelineConfig{
				SourceDir:       t.TempDir(),
				OutputDir:       docsDir,
				DocsDir:         docsDir,
				BaseURL:         tc.baseURL,
				BaseURLDeclared: tc.declared,
			}, site.NewMockRunner(), nil)

			got, err := p.resolveBasePath(docsDir)
			if err != nil {
				t.Fatalf("resolveBasePath error = %v", err)
			}
			if got != tc.want {
				t.Errorf("resolveBasePath = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestExtractorPipeline_BasePathConflictIsFatal pins that two sources disagreeing
// stops the run. Silently picking one would publish a site whose links and whose
// theme assets resolve through different prefixes, and the run would still be
// green.
func TestExtractorPipeline_BasePathConflictIsFatal(t *testing.T) {
	unsetEnvForTest(t, "GITHUB_REPOSITORY")

	docsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(docsDir, "_config.yml"),
		[]byte("baseurl: \"/from-disk\"\n"), 0o644); err != nil {
		t.Fatalf("seed _config.yml: %v", err)
	}

	p := NewExtractorPipeline(&ExtractorPipelineConfig{
		SourceDir:       t.TempDir(),
		OutputDir:       docsDir,
		DocsDir:         docsDir,
		BaseURL:         "/from-caller",
		BaseURLDeclared: true,
	}, site.NewMockRunner(), nil)

	got, err := p.resolveBasePath(docsDir)
	if !errors.Is(err, ErrBasePathConflict) {
		t.Fatalf("resolveBasePath = (%q, %v), want ErrBasePathConflict", got, err)
	}
	if !strings.Contains(err.Error(), "/from-caller") || !strings.Contains(err.Error(), "/from-disk") {
		t.Errorf("conflict error names neither value, so an operator cannot act on it: %v", err)
	}
}
