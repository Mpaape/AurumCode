package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/pipeline"
	"gopkg.in/yaml.v3"
)

// This file covers the functions that decide what the consumer of the Action
// sees: the verdict text, the machine-readable summary line, the exit status,
// and the configuration that is derived from the environment the composite
// action hands over.
//
// Every assertion is on an observable value - a returned struct field, an exact
// log line, or a raw process exit code. Nothing here asserts merely that a call
// did not panic.

// ---------------------------------------------------------------------------
// shared fixtures
// ---------------------------------------------------------------------------

func testConfig() *pipeline.ExtractorPipelineConfig {
	return &pipeline.ExtractorPipelineConfig{
		SourceDir: "/src",
		OutputDir: "/out",
		DocsDir:   "/out",
	}
}

func servableSite() siteStatus {
	return siteStatus{
		IndexPath:  "/out/index.md",
		ConfigPath: "/out/_config.yml",
		HasConfig:  true,
		IndexPages: 1,
	}
}

func unservableSite() siteStatus {
	return siteStatus{
		IndexPath:  "/out/index.md",
		ConfigPath: "/out/_config.yml",
	}
}

// ghostIndexSite is a fully formed scaffold - _config.yml exists, the index
// lists real pages - whose consumer-owned exclude list hides every one of
// them. Servable() alone cannot see this: it only asks for one listed page,
// not for one reachable one.
func ghostIndexSite() siteStatus {
	return siteStatus{
		IndexPath:     "/out/index.md",
		ConfigPath:    "/out/_config.yml",
		HasConfig:     true,
		IndexPages:    3,
		ExcludedPages: 3,
	}
}

// partiallyExcludedSite is servable - at least one listed page is reachable -
// but the consumer's exclude list hides some of the others. This is the
// boundary AllPagesExcluded must not cross: degraded, not fatal.
func partiallyExcludedSite() siteStatus {
	return siteStatus{
		IndexPath:     "/out/index.md",
		ConfigPath:    "/out/_config.yml",
		HasConfig:     true,
		IndexPages:    3,
		ExcludedPages: 1,
	}
}

func skip(lang extractors.Language, files int) pipeline.LanguageSkip {
	return pipeline.LanguageSkip{
		Language: lang,
		Reason:   pipeline.SkipToolUnavailable,
		Tool:     string(lang) + "doc",
		Files:    files,
	}
}

// captureLog redirects the standard logger for the duration of fn and returns
// everything report and its helpers wrote.
func captureLog(t *testing.T, fn func()) string {
	t.Helper()

	var buf bytes.Buffer
	prevWriter, prevFlags, prevPrefix := log.Writer(), log.Flags(), log.Prefix()
	log.SetOutput(&buf)
	log.SetFlags(0)
	log.SetPrefix("")
	t.Cleanup(func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
		log.SetPrefix(prevPrefix)
	})

	fn()

	return buf.String()
}

// stubFatal swaps the process-terminating call for a recorder so the failing
// branches of report can be inspected in-process. TestReportRawExitCode proves
// the same branches really do terminate the process.
func stubFatal(t *testing.T) *string {
	t.Helper()

	var recorded string
	prev := fatalf
	fatalf = func(format string, v ...interface{}) {
		recorded = fmt.Sprintf(format, v...)
	}
	t.Cleanup(func() { fatalf = prev })

	return &recorded
}

// ---------------------------------------------------------------------------
// report: verdict, summary and fatality
// ---------------------------------------------------------------------------

// reportScenario is shared by the in-process test and the subprocess exit-code
// test so both observe exactly the same input.
type reportScenario struct {
	runErr      error
	docs        []string
	site        siteStatus
	wantFatal   bool
	wantExit    int
	wantSummary string
	mustContain []string
	mustAbsent  []string
}

func reportScenarios() map[string]reportScenario {
	return map[string]reportScenario{
		// A structured failure with nothing on disk: the only honest outcome.
		"total-failure": {
			runErr: &pipeline.ExtractionError{
				Partial: false,
				Errors:  []error{errors.New("gomarkdoc exploded")},
			},
			docs:        nil,
			site:        unservableSite(),
			wantFatal:   true,
			wantExit:    1,
			wantSummary: "aurumcode: result=failed docs=0 skipped=0 failed=1 languages_skipped=none output=/out index_pages=0 index_pages_excluded=0 config=false",
			mustContain: []string{
				"❌ FAILED - no documentation was produced",
				"   - failed: gomarkdoc exploded",
				"❌ Pipeline failed:",
			},
			mustAbsent: []string{"✅", "result=ok", "result=partial"},
		},

		// An error that is not an *ExtractionError must not be softened into a
		// partial run just because report cannot introspect it.
		"opaque-failure": {
			runErr:      errors.New("boom"),
			docs:        nil,
			site:        unservableSite(),
			wantFatal:   true,
			wantExit:    1,
			wantSummary: "aurumcode: result=failed docs=0 skipped=0 failed=0 languages_skipped=none output=/out index_pages=0 index_pages_excluded=0 config=false",
			mustContain: []string{"❌ FAILED - no documentation was produced"},
			mustAbsent:  []string{"✅", "result=ok", "result=partial"},
		},

		// Markdown left over from an earlier run must not rescue a hard failure.
		"total-failure-with-stale-docs": {
			runErr: &pipeline.ExtractionError{
				Partial: false,
				Errors:  []error{errors.New("gomarkdoc exploded")},
			},
			docs:        []string{"/out/stale.md"},
			site:        servableSite(),
			wantFatal:   true,
			wantExit:    1,
			wantSummary: "aurumcode: result=failed docs=1 skipped=0 failed=1 languages_skipped=none output=/out index_pages=1 index_pages_excluded=0 config=true",
			mustAbsent:  []string{"✅", "result=ok"},
		},

		// Fail closed: the pipeline claims it produced something, the filesystem
		// disagrees, so the run is reported as failed and not as partial.
		"partial-without-docs": {
			runErr: &pipeline.ExtractionError{
				Partial: true,
				Skipped: []pipeline.LanguageSkip{skip("rust", 3)},
				Errors:  []error{errors.New("cpp extractor failed")},
			},
			docs:        nil,
			site:        unservableSite(),
			wantFatal:   true,
			wantExit:    1,
			wantSummary: "aurumcode: result=failed docs=0 skipped=1 failed=1 languages_skipped=rust output=/out index_pages=0 index_pages_excluded=0 config=false",
			mustContain: []string{"no markdown file exists under /out"},
			mustAbsent:  []string{"✅", "result=ok", "result=partial", "PARTIAL SUCCESS"},
		},

		// The degraded run. It exits 0, so the summary line is the only signal a
		// consumer has; it must never read as a complete run.
		"partial-with-docs": {
			runErr: &pipeline.ExtractionError{
				Partial: true,
				Skipped: []pipeline.LanguageSkip{skip("rust", 3), skip("csharp", 2)},
				Errors:  []error{errors.New("cpp extractor failed")},
			},
			docs:        []string{"/out/a.md", "/out/b.md"},
			site:        servableSite(),
			wantFatal:   false,
			wantExit:    0,
			wantSummary: "aurumcode: result=partial docs=2 skipped=2 failed=1 languages_skipped=rust,csharp output=/out index_pages=1 index_pages_excluded=0 config=true",
			mustContain: []string{
				"⚠️  PARTIAL SUCCESS - 2 language(s) skipped, 1 extraction error(s)",
				"   - failed: cpp extractor failed",
			},
			mustAbsent: []string{
				"✅ Documentation regeneration completed!",
				"result=ok",
			},
		},

		// No error and no output: not a success, and not a failure either.
		"empty": {
			runErr:      nil,
			docs:        nil,
			site:        unservableSite(),
			wantFatal:   false,
			wantExit:    0,
			wantSummary: "aurumcode: result=empty docs=0 skipped=0 failed=0 languages_skipped=none output=/out index_pages=0 index_pages_excluded=0 config=false",
			mustContain: []string{"⚠️  NO DOCUMENTATION PRODUCED - no supported source file was documented under /src"},
			mustAbsent:  []string{"✅", "result=ok", "result=partial"},
		},

		"complete": {
			runErr:      nil,
			docs:        []string{"/out/a.md"},
			site:        servableSite(),
			wantFatal:   false,
			wantExit:    0,
			wantSummary: "aurumcode: result=ok docs=1 skipped=0 failed=0 languages_skipped=none output=/out index_pages=1 index_pages_excluded=0 config=true",
			mustContain: []string{
				"✅ Documentation regeneration completed!",
				"🌐 Site scaffold:",
				"   - /out/a.md",
			},
			mustAbsent: []string{"result=partial", "would answer 404"},
		},

		// Markdown was produced but the artifact cannot be published. The run is
		// still result=ok, so the index/config counts are the only warning a
		// machine consumer gets - assert they are actually there.
		"complete-but-unpublishable": {
			runErr:      nil,
			docs:        []string{"/out/a.md"},
			site:        unservableSite(),
			wantFatal:   false,
			wantExit:    0,
			wantSummary: "aurumcode: result=ok docs=1 skipped=0 failed=0 languages_skipped=none output=/out index_pages=0 index_pages_excluded=0 config=false",
			mustContain: []string{
				"✅ Documentation regeneration completed!",
				"⚠️  /out/index.md lists no pages - a published site would answer 404 at its root",
				"⚠️  /out/_config.yml is missing - the published pages would be served as raw markdown",
			},
			mustAbsent: []string{"🌐 Site scaffold:"},
		},

		// The artifact is servable - the index lists real pages - but the
		// consumer's own exclude list hides some of them. This must stay
		// result=ok: at least one link works, so it is a warning, not a
		// build failure.
		"complete-with-some-pages-excluded": {
			runErr:      nil,
			docs:        []string{"/out/a.md", "/out/b.md", "/out/c.md"},
			site:        partiallyExcludedSite(),
			wantFatal:   false,
			wantExit:    0,
			wantSummary: "aurumcode: result=ok docs=3 skipped=0 failed=0 languages_skipped=none output=/out index_pages=3 index_pages_excluded=1 config=true",
			mustContain: []string{
				"✅ Documentation regeneration completed!",
				"🌐 Site scaffold:",
				"⚠️  /out/_config.yml excludes 1 of the 3 page(s) listed in /out/index.md - those links would answer 404",
			},
			mustAbsent: []string{"result=partial", "result=unpublishable"},
		},

		// The ghost-index defect this whole guard exists for: index.md exists,
		// _config.yml exists, every page the index lists is excluded by the
		// consumer's own config, and the run used to exit 0 anyway. It must now
		// fail loudly instead of reporting result=ok.
		"unpublishable-all-pages-excluded": {
			runErr:      nil,
			docs:        []string{"/out/a.md", "/out/b.md", "/out/c.md"},
			site:        ghostIndexSite(),
			wantFatal:   true,
			wantExit:    1,
			wantSummary: "aurumcode: result=unpublishable docs=3 skipped=0 failed=0 languages_skipped=none output=/out index_pages=3 index_pages_excluded=3 config=true",
			mustContain: []string{
				"⚠️  /out/_config.yml excludes 3 of the 3 page(s) listed in /out/index.md - those links would answer 404",
				"/out/index.md lists 3 page(s) but /out/_config.yml excludes all 3 of them",
				"every link a published site would carry answers 404",
			},
			mustAbsent: []string{"✅ Documentation regeneration completed!", "result=ok", "result=partial"},
		},

		// Same defect, but reached from the partial branch: some languages were
		// skipped AND every generated page is excluded. Both diagnostics must
		// show, and the run must still fail rather than downgrade to partial.
		"unpublishable-all-pages-excluded-partial": {
			runErr: &pipeline.ExtractionError{
				Partial: true,
				Skipped: []pipeline.LanguageSkip{skip("rust", 1)},
			},
			docs:        []string{"/out/a.md", "/out/b.md", "/out/c.md"},
			site:        ghostIndexSite(),
			wantFatal:   true,
			wantExit:    1,
			wantSummary: "aurumcode: result=unpublishable docs=3 skipped=1 failed=0 languages_skipped=rust output=/out index_pages=3 index_pages_excluded=3 config=true",
			mustContain: []string{
				"⚠️  PARTIAL SUCCESS - 1 language(s) skipped, 0 extraction error(s)",
				"/out/index.md lists 3 page(s) but /out/_config.yml excludes all 3 of them",
			},
			mustAbsent: []string{"✅", "result=ok", "result=partial"},
		},
	}
}

func TestReportVerdict(t *testing.T) {
	for name, sc := range reportScenarios() {
		sc := sc
		t.Run(name, func(t *testing.T) {
			recorded := stubFatal(t)

			out := captureLog(t, func() {
				report(sc.runErr, sc.docs, sc.site, testConfig())
			})

			if got := *recorded != ""; got != sc.wantFatal {
				t.Errorf("fatal = %v, want %v (recorded %q)", got, sc.wantFatal, *recorded)
			}

			if !strings.Contains(out, sc.wantSummary) {
				t.Errorf("summary line missing.\n want: %s\n  got:\n%s", sc.wantSummary, out)
			}

			// The fatal message is written by fatalf, not by the logger, so fold
			// it back in before checking the consumer-visible text.
			full := out + *recorded
			for _, want := range sc.mustContain {
				if !strings.Contains(full, want) {
					t.Errorf("output missing %q.\ngot:\n%s", want, full)
				}
			}
			for _, unwanted := range sc.mustAbsent {
				if strings.Contains(full, unwanted) {
					t.Errorf("output must not contain %q.\ngot:\n%s", unwanted, full)
				}
			}
		})
	}
}

// TestReportVerdictsAreDistinguishable pins the property the whole verdict
// exists for: a degraded run and a complete run cannot produce the same
// machine-readable token.
func TestReportVerdictsAreDistinguishable(t *testing.T) {
	stubFatal(t)
	tokens := map[string]string{}

	for name, sc := range reportScenarios() {
		out := captureLog(t, func() {
			report(sc.runErr, sc.docs, sc.site, testConfig())
		})

		idx := strings.Index(out, "aurumcode: result=")
		if idx < 0 {
			t.Fatalf("%s: no summary line in %q", name, out)
		}
		field := strings.Fields(out[idx:])[1]
		tokens[name] = field
	}

	if tokens["partial-with-docs"] == tokens["complete"] {
		t.Errorf("partial run and complete run share verdict %q", tokens["complete"])
	}
	if tokens["empty"] == tokens["complete"] {
		t.Errorf("empty run and complete run share verdict %q", tokens["complete"])
	}
	if tokens["partial-without-docs"] != "result=failed" {
		t.Errorf("partial run with no markdown = %q, want result=failed", tokens["partial-without-docs"])
	}

	want := map[string]string{
		"total-failure":                    "result=failed",
		"partial-with-docs":                "result=partial",
		"empty":                            "result=empty",
		"complete":                         "result=ok",
		"unpublishable-all-pages-excluded": "result=unpublishable",
		"unpublishable-all-pages-excluded-partial": "result=unpublishable",
	}
	for name, expected := range want {
		if tokens[name] != expected {
			t.Errorf("%s verdict = %q, want %q", name, tokens[name], expected)
		}
	}
}

// ---------------------------------------------------------------------------
// report: the raw exit code a caller actually receives
// ---------------------------------------------------------------------------

const reportCaseEnv = "AURUMCODE_TEST_REPORT_CASE"

// TestReportRawExitCode re-executes this test binary so report runs against the
// real log.Fatalf and the operating system reports the true exit status. This is
// the status the composite action's `run:` step observes.
func TestReportRawExitCode(t *testing.T) {
	if name := os.Getenv(reportCaseEnv); name != "" {
		sc, ok := reportScenarios()[name]
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown report scenario %q\n", name)
			os.Exit(97)
		}
		report(sc.runErr, sc.docs, sc.site, testConfig())
		// Only reached when report did not terminate the process.
		fmt.Println("report-returned")
		return
	}

	for name, sc := range reportScenarios() {
		sc := sc
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestReportRawExitCode$")
			cmd.Env = append(os.Environ(), reportCaseEnv+"="+name)
			out, err := cmd.CombinedOutput()

			exit := 0
			if err != nil {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatalf("running subprocess: %v\n%s", err, out)
				}
				exit = exitErr.ExitCode()
			}

			if exit != sc.wantExit {
				t.Errorf("exit code = %d, want %d\noutput:\n%s", exit, sc.wantExit, out)
			}

			text := string(out)
			if !strings.Contains(text, sc.wantSummary) {
				t.Errorf("subprocess summary missing.\n want: %s\n  got:\n%s", sc.wantSummary, text)
			}

			returned := strings.Contains(text, "report-returned")
			if returned == sc.wantFatal {
				t.Errorf("report returned = %v, want %v\noutput:\n%s", returned, !sc.wantFatal, text)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// summaryLine
// ---------------------------------------------------------------------------

func TestSummaryLine(t *testing.T) {
	tests := []struct {
		name    string
		result  string
		docs    []string
		site    siteStatus
		extract *pipeline.ExtractionError
		output  string
		want    string
	}{
		{
			name:   "no error and no docs",
			result: "empty",
			site:   unservableSite(),
			output: "/out",
			want:   "aurumcode: result=empty docs=0 skipped=0 failed=0 languages_skipped=none output=/out index_pages=0 index_pages_excluded=0 config=false",
		},
		{
			name:   "complete run reports servable site",
			result: "ok",
			docs:   []string{"a.md", "b.md", "c.md"},
			site:   servableSite(),
			output: "/out",
			want:   "aurumcode: result=ok docs=3 skipped=0 failed=0 languages_skipped=none output=/out index_pages=1 index_pages_excluded=0 config=true",
		},
		{
			name:   "skipped languages are listed in order",
			result: "partial",
			docs:   []string{"a.md"},
			site:   servableSite(),
			extract: &pipeline.ExtractionError{
				Partial: true,
				Skipped: []pipeline.LanguageSkip{skip("rust", 1), skip("csharp", 2), skip("powershell", 3)},
			},
			output: "/out",
			want:   "aurumcode: result=partial docs=1 skipped=3 failed=0 languages_skipped=rust,csharp,powershell output=/out index_pages=1 index_pages_excluded=0 config=true",
		},
		{
			// Errors without skips leave languages_skipped=none: the failure count
			// is the only evidence, so it must be non-zero.
			name:   "errors without skips",
			result: "failed",
			site:   unservableSite(),
			extract: &pipeline.ExtractionError{
				Errors: []error{errors.New("one"), errors.New("two")},
			},
			output: "/out",
			want:   "aurumcode: result=failed docs=0 skipped=0 failed=2 languages_skipped=none output=/out index_pages=0 index_pages_excluded=0 config=false",
		},
		{
			// Observed behaviour, not an endorsement: the line is space separated
			// and unquoted, so an output directory containing a space makes
			// `output=` unparseable by a field splitter.
			name:   "output dir with a space breaks field splitting",
			result: "ok",
			docs:   []string{"a.md"},
			site:   servableSite(),
			output: "/my out",
			want:   "aurumcode: result=ok docs=1 skipped=0 failed=0 languages_skipped=none output=/my out index_pages=1 index_pages_excluded=0 config=true",
		},
		{
			// The count that replaced the boolean: a scaffold whose exclude list
			// hides some, but not all, of the pages it lists.
			name:   "some pages excluded",
			result: "ok",
			docs:   []string{"a.md", "b.md", "c.md"},
			site:   partiallyExcludedSite(),
			output: "/out",
			want:   "aurumcode: result=ok docs=3 skipped=0 failed=0 languages_skipped=none output=/out index_pages=3 index_pages_excluded=1 config=true",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			config := testConfig()
			config.OutputDir = tc.output

			got := summaryLine(tc.result, tc.docs, tc.site, tc.extract, config)
			if got != tc.want {
				t.Errorf("summaryLine =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestSummaryLineSpaceInOutputDirIsAmbiguous(t *testing.T) {
	config := testConfig()
	config.OutputDir = "/my out"

	line := summaryLine("ok", []string{"a.md"}, servableSite(), nil, config)

	fields := strings.Fields(line)
	last := fields[len(fields)-1]
	if last != "config=true" {
		t.Fatalf("last field = %q, want config=true", last)
	}
	for _, f := range fields {
		if f == "out" {
			// The path fragment now looks like a bare token; documenting it here
			// so a future consumer does not parse this line with `awk '{print $N}'`.
			return
		}
	}
	t.Errorf("expected the space in the output dir to split into a bare field, got %q", line)
}

// ---------------------------------------------------------------------------
// resolveConfig / envBool / envOrDefault
// ---------------------------------------------------------------------------

// unsetEnv removes a variable for the duration of the test and restores it
// afterwards.
//
// t.Setenv(name, "") is not a substitute: it SETS the variable to the empty
// string. For every knob read with os.Getenv that is indistinguishable from
// absent, but AURUMCODE_BASE_URL is read with os.LookupEnv precisely so that
// "declared as the root" and "not declared" stay different, and setting it empty
// would silently mean the former in every test below. GITHUB_REPOSITORY has the
// same hazard from the other side: it is genuinely populated on an Actions
// runner, so leaving it in place would make these tests exercise a different
// code path in CI than on a laptop.
func unsetEnv(t *testing.T, name string) {
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

func clearPipelineEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		envSourceDir, envOutputDir, envDocsDir, envLanguages,
		envIncremental, envValidateJekyll, envDeployGHPages,
	} {
		t.Setenv(name, "")
	}

	// These two must be UNSET rather than emptied; see unsetEnv.
	unsetEnv(t, envBaseURL)
	unsetEnv(t, "GITHUB_REPOSITORY")
}

// TestResolveConfigBaseURLDistinguishesUnsetFromEmpty is the guard on the one
// distinction that decides whether a site on a custom domain is servable.
//
// A caller on a custom domain publishes at the root, but nothing in
// GITHUB_REPOSITORY says so - derivation would guess "/repo" and break every
// link. The only way out is for the caller to declare the root explicitly, which
// requires "set to empty" and "not set" to remain two different states all the
// way from the environment into the pipeline config. os.Getenv collapses them.
func TestResolveConfigBaseURLDistinguishesUnsetFromEmpty(t *testing.T) {
	testCases := []struct {
		name         string
		set          bool
		value        string
		wantValue    string
		wantDeclared bool
	}{
		{name: "not set", set: false, wantDeclared: false},
		{name: "set to empty means the root", set: true, value: "", wantValue: "", wantDeclared: true},
		{name: "set to a path", set: true, value: "/widgetlib", wantValue: "/widgetlib", wantDeclared: true},
		{name: "set to a bare slash", set: true, value: "/", wantValue: "/", wantDeclared: true},
	}

	if len(testCases) == 0 {
		t.Fatal("no cases: the loop below would assert nothing")
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			clearPipelineEnv(t)
			if tc.set {
				t.Setenv(envBaseURL, tc.value)
			}

			config, err := resolveConfig(false)
			if err != nil {
				t.Fatalf("resolveConfig error = %v", err)
			}

			if config.BaseURLDeclared != tc.wantDeclared {
				t.Errorf("BaseURLDeclared = %t, want %t (with %s set=%t value=%q)",
					config.BaseURLDeclared, tc.wantDeclared, envBaseURL, tc.set, tc.value)
			}
			if config.BaseURL != tc.wantValue {
				t.Errorf("BaseURL = %q, want %q", config.BaseURL, tc.wantValue)
			}
		})
	}
}

func TestResolveConfigDefaults(t *testing.T) {
	clearPipelineEnv(t)

	config, err := resolveConfig(true)
	if err != nil {
		t.Fatalf("resolveConfig error = %v", err)
	}

	wantSource, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}

	if config.SourceDir != wantSource {
		t.Errorf("SourceDir = %q, want %q", config.SourceDir, wantSource)
	}
	if config.OutputDir != ".aurumcode" {
		t.Errorf("OutputDir = %q, want %q", config.OutputDir, ".aurumcode")
	}
	if config.DocsDir != ".aurumcode" {
		t.Errorf("DocsDir = %q, want %q", config.DocsDir, ".aurumcode")
	}
	if len(config.Languages) != 0 {
		t.Errorf("Languages = %v, want empty", config.Languages)
	}
	if config.Incremental || config.ValidateJekyll || config.DeployGHPages {
		t.Errorf("boolean defaults = incremental:%v jekyll:%v ghpages:%v, want all false",
			config.Incremental, config.ValidateJekyll, config.DeployGHPages)
	}
	if !config.GenerateWelcome {
		t.Error("GenerateWelcome = false, want it to mirror the argument")
	}

	off, err := resolveConfig(false)
	if err != nil {
		t.Fatalf("resolveConfig error = %v", err)
	}
	if off.GenerateWelcome {
		t.Error("GenerateWelcome = true, want it to mirror the argument")
	}
}

func TestResolveConfigDirectoryPrecedence(t *testing.T) {
	src := t.TempDir()

	tests := []struct {
		name       string
		output     string
		docs       string
		wantOutput string
		wantDocs   string
	}{
		{
			name:       "docs dir inherits output dir",
			output:     "build/docs",
			wantOutput: "build/docs",
			wantDocs:   "build/docs",
		},
		{
			name:       "explicit docs dir wins over output dir",
			output:     "build/docs",
			docs:       "site",
			wantOutput: "build/docs",
			wantDocs:   "site",
		},
		{
			name:       "docs dir alone leaves the output default",
			docs:       "site",
			wantOutput: ".aurumcode",
			wantDocs:   "site",
		},
		{
			// envOrDefault trims, so whitespace is not a value.
			name:       "blank output dir falls back to the default",
			output:     "   ",
			wantOutput: ".aurumcode",
			wantDocs:   ".aurumcode",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			clearPipelineEnv(t)
			t.Setenv(envSourceDir, src)
			t.Setenv(envOutputDir, tc.output)
			t.Setenv(envDocsDir, tc.docs)

			config, err := resolveConfig(false)
			if err != nil {
				t.Fatalf("resolveConfig error = %v", err)
			}
			if config.OutputDir != tc.wantOutput {
				t.Errorf("OutputDir = %q, want %q", config.OutputDir, tc.wantOutput)
			}
			if config.DocsDir != tc.wantDocs {
				t.Errorf("DocsDir = %q, want %q", config.DocsDir, tc.wantDocs)
			}
			if config.SourceDir != src {
				t.Errorf("SourceDir = %q, want %q", config.SourceDir, src)
			}
		})
	}
}

func TestResolveConfigSourceDirIsAbsolute(t *testing.T) {
	clearPipelineEnv(t)
	t.Setenv(envSourceDir, ".")

	config, err := resolveConfig(false)
	if err != nil {
		t.Fatalf("resolveConfig error = %v", err)
	}
	if !filepath.IsAbs(config.SourceDir) {
		t.Errorf("SourceDir = %q, want an absolute path", config.SourceDir)
	}

	want, _ := filepath.Abs(".")
	if config.SourceDir != want {
		t.Errorf("SourceDir = %q, want %q", config.SourceDir, want)
	}
}

func TestResolveConfigRejectsBadSourceDir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	missing := filepath.Join(dir, "absent")

	tests := []struct {
		name     string
		source   string
		wantFrag string
	}{
		{"missing directory", missing, "source directory " + missing},
		{"regular file", file, "is not a directory"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			clearPipelineEnv(t)
			t.Setenv(envSourceDir, tc.source)

			config, err := resolveConfig(false)
			if err == nil {
				t.Fatalf("resolveConfig succeeded, want an error (config %+v)", config)
			}
			if config != nil {
				t.Errorf("config = %+v, want nil alongside the error", config)
			}
			if !strings.Contains(err.Error(), tc.wantFrag) {
				t.Errorf("error = %q, want it to mention %q", err, tc.wantFrag)
			}
		})
	}
}

// TestResolveConfigRejectsGarbageBooleans answers the question directly: a
// boolean knob set to something that is not a boolean is refused, not coerced
// to false and silently ignored.
func TestResolveConfigRejectsGarbageBooleans(t *testing.T) {
	src := t.TempDir()

	for _, name := range []string{envIncremental, envValidateJekyll, envDeployGHPages} {
		name := name
		t.Run(name, func(t *testing.T) {
			clearPipelineEnv(t)
			t.Setenv(envSourceDir, src)
			t.Setenv(name, "maybe")

			config, err := resolveConfig(false)
			if err == nil {
				t.Fatalf("resolveConfig succeeded with %s=maybe, want an error (config %+v)", name, config)
			}
			if config != nil {
				t.Errorf("config = %+v, want nil alongside the error", config)
			}

			want := name + ` must be one of true/false (got "maybe")`
			if err.Error() != want {
				t.Errorf("error = %q, want %q", err, want)
			}
		})
	}
}

// TestResolveConfigDeployGHPagesFailsClosed pins the fail-closed contract: the
// capability is absent from this build, so asking for it is an explicit error
// rather than a run that quietly publishes nothing.
func TestResolveConfigDeployGHPagesFailsClosed(t *testing.T) {
	src := t.TempDir()

	clearPipelineEnv(t)
	t.Setenv(envSourceDir, src)
	t.Setenv(envDeployGHPages, "true")

	config, err := resolveConfig(false)
	if err == nil {
		t.Fatalf("resolveConfig succeeded with %s=true, want an error (config %+v)", envDeployGHPages, config)
	}
	if config != nil {
		t.Errorf("config = %+v, want nil alongside the error", config)
	}
	if !strings.Contains(err.Error(), "gh-pages deployment is not implemented in this build") {
		t.Errorf("error = %q, want it to state the capability is missing", err)
	}

	// The opposite value is accepted and leaves the flag off.
	t.Setenv(envDeployGHPages, "false")
	config, err = resolveConfig(false)
	if err != nil {
		t.Fatalf("resolveConfig error = %v", err)
	}
	if config.DeployGHPages {
		t.Error("DeployGHPages = true, want false")
	}
}

func TestResolveConfigBooleansAndLanguages(t *testing.T) {
	src := t.TempDir()

	clearPipelineEnv(t)
	t.Setenv(envSourceDir, src)
	t.Setenv(envIncremental, "TRUE")
	t.Setenv(envValidateJekyll, " yes ")
	t.Setenv(envLanguages, " go , python ,, go ")

	config, err := resolveConfig(false)
	if err != nil {
		t.Fatalf("resolveConfig error = %v", err)
	}
	if !config.Incremental {
		t.Error("Incremental = false, want true for \"TRUE\"")
	}
	if !config.ValidateJekyll {
		t.Error("ValidateJekyll = false, want true for \" yes \"")
	}
	if want := []string{"go", "python", "go"}; !reflect.DeepEqual(config.Languages, want) {
		t.Errorf("Languages = %v, want %v", config.Languages, want)
	}
}

func TestEnvBool(t *testing.T) {
	const name = "AURUMCODE_TEST_BOOL"

	accepted := []struct {
		raw  string
		want bool
	}{
		{"", false},
		{"0", false},
		{"false", false},
		{"FALSE", false},
		{"  False  ", false},
		{"no", false},
		{"NO", false},
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{"  True  ", true},
		{"yes", true},
		{" Yes ", true},
	}

	for _, tc := range accepted {
		tc := tc
		t.Run("accepts "+safeName(tc.raw), func(t *testing.T) {
			t.Setenv(name, tc.raw)
			got, err := envBool(name)
			if err != nil {
				t.Fatalf("envBool(%q) error = %v", tc.raw, err)
			}
			if got != tc.want {
				t.Errorf("envBool(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}

	// These are all values a user could plausibly type. Every one is refused,
	// so a misconfigured knob stops the run instead of silently meaning false.
	rejected := []string{"maybe", "on", "off", "y", "n", "2", "-1", "TRUEISH", "true false", "1.0", "null", "None"}

	for _, raw := range rejected {
		raw := raw
		t.Run("rejects "+safeName(raw), func(t *testing.T) {
			t.Setenv(name, raw)
			got, err := envBool(name)
			if err == nil {
				t.Fatalf("envBool(%q) = %v with no error, want an error", raw, got)
			}
			if got {
				t.Errorf("envBool(%q) = true alongside an error, want false", raw)
			}

			want := name + ` must be one of true/false (got "` + raw + `")`
			if err.Error() != want {
				t.Errorf("error = %q, want %q", err, want)
			}
		})
	}
}

func TestEnvOrDefault(t *testing.T) {
	const name = "AURUMCODE_TEST_STRING"

	tests := []struct {
		raw      string
		fallback string
		want     string
	}{
		{"", "fb", "fb"},
		{"   ", "fb", "fb"},
		{"\t\n", "fb", "fb"},
		{"value", "fb", "value"},
		{"  value  ", "fb", "value"},
		{"a b", "fb", "a b"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(safeName(tc.raw), func(t *testing.T) {
			t.Setenv(name, tc.raw)
			if got := envOrDefault(name, tc.fallback); got != tc.want {
				t.Errorf("envOrDefault(%q, %q) = %q, want %q", tc.raw, tc.fallback, got, tc.want)
			}
		})
	}
}

// strconv renders a raw value for a subtest name without importing strconv for
// one call and without producing a name containing a slash or a space.
func safeName(raw string) string {
	if raw == "" {
		return "empty"
	}
	return strings.NewReplacer(" ", "_", "\t", "tab", "\n", "nl", "/", "-").Replace(raw)
}

// ---------------------------------------------------------------------------
// splitLanguages
// ---------------------------------------------------------------------------

func TestSplitLanguages(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"empty string", "", []string{}},
		{"only separators", ",,,", []string{}},
		{"only whitespace", "   ", []string{}},
		{"whitespace and separators", " , ,\t, ", []string{}},
		{"single value", "go", []string{"go"}},
		{"padding is trimmed", "  go  ", []string{"go"}},
		{"multiple values", "go,python,bash", []string{"go", "python", "bash"}},
		{"inner padding is trimmed", " go , python ", []string{"go", "python"}},
		{"empty fields are dropped", "go,,python", []string{"go", "python"}},
		{"trailing separator", "go,", []string{"go"}},
		{"leading separator", ",go", []string{"go"}},

		// Observed behaviour worth pinning: the function neither de-duplicates
		// nor validates nor canonicalises case. The pipeline matches language
		// names exactly against lowercase constants, so "Go" and "golang" filter
		// every file out and the run ends as result=empty with exit 0.
		{"duplicates are preserved", "go,go", []string{"go", "go"}},
		{"case is preserved verbatim", "Go", []string{"Go"}},
		{"unknown names are accepted", "cobol", []string{"cobol"}},
		{"space separated input is one token", "go python", []string{"go python"}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := splitLanguages(tc.raw)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("splitLanguages(%q) = %#v, want %#v", tc.raw, got, tc.want)
			}
		})
	}
}

// TestSplitLanguagesEmptyIsNonNil matters because the pipeline treats a
// zero-length language list as "extract everything"; a nil versus empty slice
// mix-up here would change which files are documented.
func TestSplitLanguagesEmptyIsNonNil(t *testing.T) {
	got := splitLanguages("")
	if got == nil {
		t.Fatal("splitLanguages(\"\") = nil, want an empty non-nil slice")
	}
	if len(got) != 0 {
		t.Fatalf("splitLanguages(\"\") = %v, want length 0", got)
	}
}

// TestSplitLanguagesUnknownNameDocumentsNothing shows the consequence of the
// missing validation at the layer that consumes it: an unrecognised name is not
// rejected, it simply removes every candidate file.
func TestSplitLanguagesUnknownNameDocumentsNothing(t *testing.T) {
	for _, raw := range []string{"Go", "golang", "cobol"} {
		langs := splitLanguages(raw)
		if len(langs) != 1 {
			t.Fatalf("splitLanguages(%q) = %v, want one entry", raw, langs)
		}

		lang := extractors.Language(langs[0])
		if lang == extractors.LanguageGo {
			t.Errorf("splitLanguages(%q) produced the canonical %q; expected the raw value to differ", raw, extractors.LanguageGo)
		}
	}
}

// ---------------------------------------------------------------------------
// collectMarkdown
// ---------------------------------------------------------------------------

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestCollectMarkdownMissingOutputDirIsNotAnError(t *testing.T) {
	config := &pipeline.ExtractorPipelineConfig{
		OutputDir: filepath.Join(t.TempDir(), "never-created"),
	}

	docs, err := collectMarkdown(config)
	if err != nil {
		t.Fatalf("collectMarkdown error = %v, want nil", err)
	}
	if len(docs) != 0 {
		t.Errorf("docs = %v, want none", docs)
	}
}

func TestCollectMarkdownEmptyDir(t *testing.T) {
	dir := t.TempDir()
	config := &pipeline.ExtractorPipelineConfig{OutputDir: dir, DocsDir: dir}

	docs, err := collectMarkdown(config)
	if err != nil {
		t.Fatalf("collectMarkdown error = %v", err)
	}
	if len(docs) != 0 {
		t.Errorf("docs = %v, want none", docs)
	}
}

func TestCollectMarkdownSelectsAndExcludes(t *testing.T) {
	out := t.TempDir()

	writeFile(t, filepath.Join(out, "a.md"), "a")
	writeFile(t, filepath.Join(out, "B.MD"), "b")       // case-insensitive extension
	writeFile(t, filepath.Join(out, "notes.txt"), "no") // wrong extension
	writeFile(t, filepath.Join(out, "index.md"), "landing")
	writeFile(t, filepath.Join(out, "sub", "c.md"), "c")
	writeFile(t, filepath.Join(out, "sub", "index.md"), "nested landing")

	config := &pipeline.ExtractorPipelineConfig{OutputDir: out, DocsDir: out}

	docs, err := collectMarkdown(config)
	if err != nil {
		t.Fatalf("collectMarkdown error = %v", err)
	}

	want := []string{
		filepath.ToSlash(filepath.Join(out, "B.MD")),
		filepath.ToSlash(filepath.Join(out, "a.md")),
		filepath.ToSlash(filepath.Join(out, "sub", "c.md")),
		filepath.ToSlash(filepath.Join(out, "sub", "index.md")),
	}
	if !reflect.DeepEqual(docs, want) {
		t.Errorf("docs =\n%#v\nwant\n%#v", docs, want)
	}

	for _, doc := range docs {
		if doc == filepath.ToSlash(filepath.Join(out, "index.md")) {
			t.Error("the site landing page must not be counted as generated documentation")
		}
	}
}

// TestCollectMarkdownLandingPageFollowsDocsDir pins the exclusion rule: the
// landing page that is skipped is the one under DocsDir, so when DocsDir points
// somewhere else an index.md inside OutputDir does count as a real document.
func TestCollectMarkdownLandingPageFollowsDocsDir(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "out")
	docsDir := filepath.Join(root, "site")

	writeFile(t, filepath.Join(out, "index.md"), "generated index")
	writeFile(t, filepath.Join(docsDir, "index.md"), "landing")

	config := &pipeline.ExtractorPipelineConfig{OutputDir: out, DocsDir: docsDir}
	docs, err := collectMarkdown(config)
	if err != nil {
		t.Fatalf("collectMarkdown error = %v", err)
	}

	want := []string{filepath.ToSlash(filepath.Join(out, "index.md"))}
	if !reflect.DeepEqual(docs, want) {
		t.Errorf("docs = %#v, want %#v", docs, want)
	}
}

// TestCollectMarkdownEmptyDocsDirFallsBackToOutputDir covers the branch that
// resolveConfig never produces but a direct caller can.
func TestCollectMarkdownEmptyDocsDirFallsBackToOutputDir(t *testing.T) {
	out := t.TempDir()
	writeFile(t, filepath.Join(out, "index.md"), "landing")
	writeFile(t, filepath.Join(out, "a.md"), "a")

	config := &pipeline.ExtractorPipelineConfig{OutputDir: out, DocsDir: ""}
	docs, err := collectMarkdown(config)
	if err != nil {
		t.Fatalf("collectMarkdown error = %v", err)
	}

	want := []string{filepath.ToSlash(filepath.Join(out, "a.md"))}
	if !reflect.DeepEqual(docs, want) {
		t.Errorf("docs = %#v, want %#v", docs, want)
	}
}

func TestCollectMarkdownSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevation on Windows")
	}

	root := t.TempDir()
	out := filepath.Join(root, "out")
	external := filepath.Join(root, "external")

	writeFile(t, filepath.Join(external, "real.md"), "real")
	writeFile(t, filepath.Join(external, "nested", "deep.md"), "deep")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// A symlink pointing at an existing markdown file.
	if err := os.Symlink(filepath.Join(external, "real.md"), filepath.Join(out, "linked.md")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	// A symlink pointing at a directory full of markdown.
	if err := os.Symlink(filepath.Join(external, "nested"), filepath.Join(out, "linkdir")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	// A symlink whose target does not exist.
	if err := os.Symlink(filepath.Join(external, "gone.md"), filepath.Join(out, "dangling.md")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	config := &pipeline.ExtractorPipelineConfig{OutputDir: out, DocsDir: out}
	docs, err := collectMarkdown(config)
	if err != nil {
		t.Fatalf("collectMarkdown error = %v", err)
	}

	// filepath.Walk does not follow symlinks: a symlinked file is counted by its
	// own name, a symlinked directory is never descended into, and a dangling
	// symlink is counted even though nothing can read it.
	want := []string{
		filepath.ToSlash(filepath.Join(out, "dangling.md")),
		filepath.ToSlash(filepath.Join(out, "linked.md")),
	}
	if !reflect.DeepEqual(docs, want) {
		t.Errorf("docs =\n%#v\nwant\n%#v", docs, want)
	}

	for _, doc := range docs {
		if strings.Contains(doc, "linkdir") {
			t.Errorf("symlinked directory was descended into: %s", doc)
		}
	}

	// The dangling entry is reported as a document but cannot be read - the
	// count in the summary line is therefore an upper bound, not a guarantee.
	if _, err := os.ReadFile(filepath.Join(out, "dangling.md")); err == nil {
		t.Error("expected the dangling symlink to be unreadable")
	}
}

func TestCollectMarkdownOutputDirIsAFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "single.md")
	writeFile(t, file, "content")

	config := &pipeline.ExtractorPipelineConfig{OutputDir: file, DocsDir: dir}
	docs, err := collectMarkdown(config)
	if err != nil {
		t.Fatalf("collectMarkdown error = %v", err)
	}

	want := []string{filepath.ToSlash(file)}
	if !reflect.DeepEqual(docs, want) {
		t.Errorf("docs = %#v, want %#v", docs, want)
	}
}

func TestCollectMarkdownPropagatesWalkErrors(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: permission bits do not block the walk")
	}

	out := t.TempDir()
	locked := filepath.Join(out, "locked")
	writeFile(t, filepath.Join(locked, "a.md"), "a")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	config := &pipeline.ExtractorPipelineConfig{OutputDir: out, DocsDir: out}
	docs, err := collectMarkdown(config)
	if err == nil {
		t.Fatalf("collectMarkdown succeeded with docs %v, want a permission error", docs)
	}
	if docs != nil {
		t.Errorf("docs = %v, want nil alongside the error", docs)
	}
}

// ---------------------------------------------------------------------------
// inspectSite / siteStatus
// ---------------------------------------------------------------------------

// docsUnder joins each relative name onto dir and returns the slash-form
// paths collectMarkdown would have produced, so tests can hand inspectSite
// exactly the shape main() does.
func docsUnder(dir string, names ...string) []string {
	docs := make([]string, 0, len(names))
	for _, name := range names {
		docs = append(docs, filepath.ToSlash(filepath.Join(dir, name)))
	}
	return docs
}

func TestInspectSite(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(t *testing.T, dir string)
		docs           []string
		wantConfig     bool
		wantIndexPages int
		wantExcluded   int
	}{
		{
			name: "missing directory, no docs",
		},
		{
			name: "empty directory, no docs",
			setup: func(t *testing.T, dir string) {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
			},
		},
		{
			// docs generated with no config at all: not servable, but the page
			// count still reflects what was actually produced.
			name:           "docs with no config",
			docs:           []string{"a.md", "b.md"},
			wantIndexPages: 2,
		},
		{
			name: "config only, no docs",
			setup: func(t *testing.T, dir string) {
				writeFile(t, filepath.Join(dir, "_config.yml"), "title: x")
			},
			wantConfig: true,
		},
		{
			name: "docs and a config with no exclude list",
			setup: func(t *testing.T, dir string) {
				writeFile(t, filepath.Join(dir, "_config.yml"), "title: x\n")
			},
			docs:           []string{"a.md", "b.md"},
			wantConfig:     true,
			wantIndexPages: 2,
		},
		{
			// A directory named index.md would make the site unservable if it
			// were still checked for existence; the redesigned status no longer
			// depends on that file's presence at all, only on what was
			// generated and what the config excludes.
			name: "index.md is a directory but does not affect the count",
			setup: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "index.md"), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				writeFile(t, filepath.Join(dir, "_config.yml"), "title: x")
			},
			docs:           []string{"a.md"},
			wantConfig:     true,
			wantIndexPages: 1,
		},
		{
			// The ghost-index defect this whole redesign exists for: the
			// consumer's own exclude list hides one of the two generated
			// pages. The old boolean pair would have reported index=true
			// config=true with no way to see the dead link.
			name: "exclude list hides one of two generated pages",
			setup: func(t *testing.T, dir string) {
				writeFile(t, filepath.Join(dir, "_config.yml"), "exclude:\n  - go\n")
			},
			docs:           []string{"go/pkg.md", "python/pkg.md"},
			wantConfig:     true,
			wantIndexPages: 2,
			wantExcluded:   1,
		},
		{
			// Every generated page excluded: the case report() must now fail on.
			name: "exclude list hides every generated page",
			setup: func(t *testing.T, dir string) {
				writeFile(t, filepath.Join(dir, "_config.yml"), "exclude:\n  - go\n  - python\n")
			},
			docs:           []string{"go/pkg.md", "python/pkg.md"},
			wantConfig:     true,
			wantIndexPages: 2,
			wantExcluded:   2,
		},
		{
			// An exclude entry matching a whole directory hides every page
			// under it, not just a page whose path equals the entry exactly.
			name: "exclude entry hides a page nested under it",
			setup: func(t *testing.T, dir string) {
				writeFile(t, filepath.Join(dir, "_config.yml"), "exclude:\n  - go\n")
			},
			docs:           []string{"go/nested/deep.md"},
			wantConfig:     true,
			wantIndexPages: 1,
			wantExcluded:   1,
		},
		{
			// A directory that merely starts with the same characters as an
			// exclude entry must not match: "go" must not hide "gopher/x.md".
			name: "exclude entry does not match on a bare prefix",
			setup: func(t *testing.T, dir string) {
				writeFile(t, filepath.Join(dir, "_config.yml"), "exclude:\n  - go\n")
			},
			docs:           []string{"gopher/x.md"},
			wantConfig:     true,
			wantIndexPages: 1,
			wantExcluded:   0,
		},
		{
			// A malformed config must not crash the run or the report; it
			// simply cannot say anything is excluded.
			name: "malformed config excludes nothing",
			setup: func(t *testing.T, dir string) {
				writeFile(t, filepath.Join(dir, "_config.yml"), "exclude: [unterminated")
			},
			docs:           []string{"a.md"},
			wantConfig:     true,
			wantIndexPages: 1,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			docsDir := filepath.Join(t.TempDir(), "docs")
			if tc.setup != nil {
				tc.setup(t, docsDir)
			}

			config := &pipeline.ExtractorPipelineConfig{OutputDir: docsDir, DocsDir: docsDir}
			got := inspectSite(config, docsUnder(docsDir, tc.docs...))

			if got.HasConfig != tc.wantConfig {
				t.Errorf("HasConfig = %v, want %v", got.HasConfig, tc.wantConfig)
			}
			if got.IndexPages != tc.wantIndexPages {
				t.Errorf("IndexPages = %d, want %d", got.IndexPages, tc.wantIndexPages)
			}
			if got.ExcludedPages != tc.wantExcluded {
				t.Errorf("ExcludedPages = %d, want %d", got.ExcludedPages, tc.wantExcluded)
			}

			wantServable := tc.wantConfig && tc.wantIndexPages > 0
			if got.Servable() != wantServable {
				t.Errorf("Servable() = %v, want %v", got.Servable(), wantServable)
			}

			wantIndexPath := filepath.ToSlash(filepath.Join(docsDir, "index.md"))
			wantConfigPath := filepath.ToSlash(filepath.Join(docsDir, "_config.yml"))
			if got.IndexPath != wantIndexPath {
				t.Errorf("IndexPath = %q, want %q", got.IndexPath, wantIndexPath)
			}
			if got.ConfigPath != wantConfigPath {
				t.Errorf("ConfigPath = %q, want %q", got.ConfigPath, wantConfigPath)
			}
		})
	}
}

func TestInspectSiteEmptyDocsDirFallsBackToOutputDir(t *testing.T) {
	out := t.TempDir()
	writeFile(t, filepath.Join(out, "_config.yml"), "title: x")

	got := inspectSite(&pipeline.ExtractorPipelineConfig{OutputDir: out, DocsDir: ""}, docsUnder(out, "a.md"))

	if got.IndexPath != filepath.ToSlash(filepath.Join(out, "index.md")) {
		t.Errorf("IndexPath = %q, want it derived from OutputDir", got.IndexPath)
	}
	if !got.Servable() {
		t.Errorf("Servable() = false, want true (config %v, index_pages %d)", got.HasConfig, got.IndexPages)
	}
}

// TestInspectSiteConfigSymlinkToNothingIsNotServable pins the one piece of
// filesystem-existence behaviour that survives the redesign: HasConfig still
// comes from os.Stat, so a _config.yml that is a dangling symlink still
// counts as absent even though docs were generated.
func TestInspectSiteConfigSymlinkToNothingIsNotServable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevation on Windows")
	}

	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	real := filepath.Join(root, "real")

	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// _config.yml is a symlink to nothing.
	if err := os.Symlink(filepath.Join(real, "absent.yml"), filepath.Join(docsDir, "_config.yml")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	got := inspectSite(&pipeline.ExtractorPipelineConfig{OutputDir: root, DocsDir: docsDir}, docsUnder(docsDir, "a.md"))

	if got.HasConfig {
		t.Error("HasConfig = true, want false for a dangling symlink")
	}
	if got.IndexPages != 1 {
		t.Errorf("IndexPages = %d, want 1 - docs were generated even though the site is not servable", got.IndexPages)
	}
	if got.Servable() {
		t.Error("Servable() = true, want false while _config.yml is unresolvable")
	}
}

func TestSiteStatusServable(t *testing.T) {
	tests := []struct {
		indexPages int
		config     bool
		want       bool
	}{
		{0, false, false},
		{1, false, false},
		{0, true, false},
		{1, true, true},
		{5, true, true},
	}

	for _, tc := range tests {
		got := siteStatus{IndexPages: tc.indexPages, HasConfig: tc.config}.Servable()
		if got != tc.want {
			t.Errorf("siteStatus{IndexPages:%d, HasConfig:%v}.Servable() = %v, want %v",
				tc.indexPages, tc.config, got, tc.want)
		}
	}

	// Servable() alone must not react to ExcludedPages: that split is what
	// AllPagesExcluded is for, and report relies on the two staying separate.
	allExcludedButServable := siteStatus{HasConfig: true, IndexPages: 3, ExcludedPages: 3}
	if !allExcludedButServable.Servable() {
		t.Error("Servable() = false for an all-excluded site, want true - " +
			"AllPagesExcluded is the guard that must catch this, not Servable")
	}
}

// TestSiteStatusAllPagesExcluded is the direct, unit-level pin on the
// predicate report uses to fail closed on a ghost index. It exists
// independently of the report()-level scenarios so the boundary - some
// excluded versus every page excluded - is provable without going through
// log capture and fatalf stubbing.
func TestSiteStatusAllPagesExcluded(t *testing.T) {
	tests := []struct {
		name          string
		indexPages    int
		excludedPages int
		want          bool
	}{
		{"no pages at all", 0, 0, false},
		{"none excluded", 3, 0, false},
		{"some excluded", 3, 1, false},
		{"all excluded", 3, 3, true},
		{"one page, excluded", 1, 1, true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := siteStatus{IndexPages: tc.indexPages, ExcludedPages: tc.excludedPages}.AllPagesExcluded()
			if got != tc.want {
				t.Errorf("AllPagesExcluded() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestIsRegularFile covers the predicate both site checks rely on.
func TestIsRegularFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.txt")
	writeFile(t, file, "x")

	if !isRegularFile(file) {
		t.Error("isRegularFile(regular file) = false, want true")
	}
	if isRegularFile(dir) {
		t.Error("isRegularFile(directory) = true, want false")
	}
	if isRegularFile(filepath.Join(dir, "absent")) {
		t.Error("isRegularFile(missing path) = true, want false")
	}
}

// ---------------------------------------------------------------------------
// the action manifest that feeds this binary
// ---------------------------------------------------------------------------
//
// resolveConfig above distinguishes "AURUMCODE_BASE_URL is unset" from
// "AURUMCODE_BASE_URL is set to the empty string" through os.LookupEnv, and the
// whole base-path fix rests on that distinction surviving the trip through
// action.yml. Nothing inside Go can observe that trip, so the manifest is
// asserted directly here: it is the file whose contents decide whether a
// consumer's documentation links resolve or 404, and it had no automated gate.

// actionBasePathSentinel is the value the manifest's default and the shell step
// must agree on. It is written once so the two assertions below cannot drift
// apart silently: a default of 'auto' compared against "AUTO" in the shell is
// the same defect as no sentinel at all.
const actionBasePathSentinel = "auto"

// actionInput mirrors one entry of action.yml's `inputs` map.
//
// Default is a *string on purpose: a missing `default:` key and `default: ”`
// are exactly the two states this test exists to tell apart, and a plain string
// would collapse them into the same zero value - the very confusion the sentinel
// was introduced to prevent.
type actionInput struct {
	Description string  `yaml:"description"`
	Required    bool    `yaml:"required"`
	Default     *string `yaml:"default"`
}

type actionStep struct {
	Name  string            `yaml:"name"`
	ID    string            `yaml:"id"`
	Shell string            `yaml:"shell"`
	Env   map[string]string `yaml:"env"`
	Run   string            `yaml:"run"`
}

type actionManifest struct {
	Inputs map[string]actionInput `yaml:"inputs"`
	Runs   struct {
		Using string       `yaml:"using"`
		Steps []actionStep `yaml:"steps"`
	} `yaml:"runs"`
}

// loadActionManifest parses the real action.yml this binary ships inside. The
// path is relative to the package directory, which is where `go test` runs.
func loadActionManifest(t *testing.T) actionManifest {
	t.Helper()

	const path = "../../action.yml"

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var manifest actionManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("%s is not valid YAML (%v); GitHub would refuse to run the action", path, err)
	}

	if len(manifest.Inputs) == 0 {
		t.Fatalf("%s declares no inputs, so the assertions below would examine nothing", path)
	}
	if len(manifest.Runs.Steps) == 0 {
		t.Fatalf("%s declares no steps, so the assertions below would examine nothing", path)
	}

	return manifest
}

func actionStepByID(t *testing.T, manifest actionManifest, id string) actionStep {
	t.Helper()

	for _, step := range manifest.Runs.Steps {
		if step.ID == id {
			return step
		}
	}

	ids := make([]string, 0, len(manifest.Runs.Steps))
	for _, step := range manifest.Runs.Steps {
		ids = append(ids, fmt.Sprintf("%q", step.ID))
	}
	t.Fatalf("action.yml has no step with id %q; step ids are %s", id, strings.Join(ids, ", "))
	return actionStep{}
}

// TestActionBasePathDefaultIsTheSentinel is the regression gate for the single
// line that decides whether every consumer's documentation links resolve.
//
// The generator derives the base path from GITHUB_REPOSITORY only when
// AURUMCODE_BASE_URL is *unset*. A composite action input always carries a
// value, so exporting it unconditionally would leave the variable always set and
// the derivation would never run - precisely the state that made 100% of the
// links on a project Pages site answer 404. The non-empty sentinel is what keeps
// "the caller touched nothing" and "the caller asked for the domain root" two
// different answers.
//
// Flipping this default back to ” reintroduces the defect for every consumer
// while every other test in this repository stays green.
func TestActionBasePathDefaultIsTheSentinel(t *testing.T) {
	manifest := loadActionManifest(t)

	input, ok := manifest.Inputs["base-path"]
	if !ok {
		names := make([]string, 0, len(manifest.Inputs))
		for name := range manifest.Inputs {
			names = append(names, name)
		}
		sort.Strings(names)
		t.Fatalf("action.yml declares no base-path input; inputs are %s", strings.Join(names, ", "))
	}

	if input.Default == nil {
		t.Fatal("base-path declares no default: GitHub hands the step an empty string, " +
			"the step exports AURUMCODE_BASE_URL='' and the generator reads that as " +
			"\"publish at the root\", so every documentation link on a project Pages " +
			"site answers 404")
	}

	if *input.Default != actionBasePathSentinel {
		t.Fatalf("base-path default = %q, want %q. An empty default is indistinguishable "+
			"from a caller who really asked for the root: os.LookupEnv reports the "+
			"variable as declared, resolveBasePath never derives from GITHUB_REPOSITORY, "+
			"and the 404 the fix removed comes back for every project Pages site.",
			*input.Default, actionBasePathSentinel)
	}
}

// TestActionExportsBaseURLOnlyOffTheSentinel pins the other half: the step has to
// export AURUMCODE_BASE_URL *conditionally*.
//
// A correct default with an unconditional export is just as broken - the
// variable would always be set, so the generator would never derive anything -
// and the two halves live hundreds of lines apart in a file no Go test read.
func TestActionExportsBaseURLOnlyOffTheSentinel(t *testing.T) {
	manifest := loadActionManifest(t)
	step := actionStepByID(t, manifest, "generate")

	// The input has to reach the shell, and it has to reach it through env:.
	// Interpolating ${{ inputs.base-path }} into the run: body would splice
	// caller-controlled text into the script itself.
	const wantEnvValue = "${{ inputs.base-path }}"
	if got := step.Env["BASE_PATH"]; got != wantEnvValue {
		t.Fatalf("generate step maps BASE_PATH = %q, want %q; the base-path input never "+
			"reaches the generator otherwise", got, wantEnvValue)
	}
	if strings.Contains(step.Run, "inputs.base-path") {
		t.Errorf("the run: body interpolates inputs.base-path directly instead of reading "+
			"the BASE_PATH environment variable:\n%s", step.Run)
	}

	// The guard is reconstructed from the sentinel the test above pinned, so
	// changing one side without the other fails here.
	wantGuard := `if [ "${BASE_PATH}" != "` + actionBasePathSentinel + `" ]; then`
	wantExport := `export ` + envBaseURL + `="${BASE_PATH}"`

	lines := strings.Split(step.Run, "\n")

	guardLine, closeLine := -1, -1
	var exportLines []int
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == wantGuard && guardLine < 0 {
			guardLine = i
		} else if trimmed == "fi" && guardLine >= 0 && closeLine < 0 {
			closeLine = i
		}
		if strings.Contains(trimmed, "export "+envBaseURL) {
			exportLines = append(exportLines, i)
		}
	}

	if guardLine < 0 {
		t.Fatalf("the generate step does not guard the export with %s; without it the "+
			"variable is always set and the generator never derives a base path from "+
			"GITHUB_REPOSITORY:\n%s", wantGuard, step.Run)
	}
	if closeLine < 0 {
		t.Fatalf("the %s guard is never closed with fi:\n%s", wantGuard, step.Run)
	}

	if len(exportLines) != 1 {
		t.Fatalf("found %d exports of %s in the generate step, want exactly 1:\n%s",
			len(exportLines), envBaseURL, step.Run)
	}

	if exportLines[0] <= guardLine || exportLines[0] >= closeLine {
		t.Fatalf("%s is exported outside the sentinel guard (export on line %d, guard on "+
			"line %d, fi on line %d): the variable would always be set and every project "+
			"Pages site would go back to 404:\n%s",
			envBaseURL, exportLines[0]+1, guardLine+1, closeLine+1, step.Run)
	}

	if got := strings.TrimSpace(lines[exportLines[0]]); got != wantExport {
		t.Errorf("export line = %q, want %q; the generator reads %s and nothing else",
			got, wantExport, envBaseURL)
	}
}
