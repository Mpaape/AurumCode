package browserproof_test

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/qa/browserproof"
)

func writeSite(t *testing.T, files map[string]string) string {
	t.Helper()

	root := t.TempDir()
	for name, body := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	return root
}

// TestScriptedDoubleCannotForgeSuccess proves the test double cannot hand out a
// verdict on its own: every observation must be corroborated by the local server
// ledger, so a double that lies or never navigates is refused as inconclusive.
func TestScriptedDoubleCannotForgeSuccess(t *testing.T) {
	cases := []struct {
		name    string
		variant string
		forge   browserproof.ForgeMode
	}{
		{"claims a page the artifact does not have", "missing-symbol-page", browserproof.ForgeClaimSuccess},
		{"claims a page it never navigated to", "nominal", browserproof.ForgeWithoutFetch},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			driver := browserproof.NewScriptedDriver()
			driver.Forge = tc.forge
			// The double claims exactly what the assertion wants, so only the
			// server ledger stands between the forgery and a "proved" verdict.
			driver.ForgedText = symbolText

			result, err := browserproof.New(driver).Run(context.Background(), scriptedRequest(fixtureDir(t, tc.variant)))
			if err == nil || result.Proved {
				t.Fatalf("forged observation accepted as proof: %+v", result)
			}
			if result.Outcome != browserproof.OutcomeInconclusive {
				t.Errorf("outcome = %q, want inconclusive", result.Outcome)
			}
			if result.Code != browserproof.CodeObservationUnsupported {
				t.Errorf("code = %q, want %q", result.Code, browserproof.CodeObservationUnsupported)
			}
		})
	}
}

// fetchingLiar navigates for real, so the server ledger records exactly the
// status and byte count it reports back, and then invents the selector, the text
// on screen and the links that decide reachability.
type fetchingLiar struct {
	text     string
	linkFrom string
	links    []string
}

func (d *fetchingLiar) Fingerprint(context.Context) (browserproof.DriverFingerprint, error) {
	return browserproof.DriverFingerprint{
		Kind:   browserproof.ExternalDriverKind,
		Digest: liarDigest,
	}, nil
}

func (d *fetchingLiar) Navigate(ctx context.Context, url, _ string) (browserproof.PageObservation, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return browserproof.PageObservation{}, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return browserproof.PageObservation{}, err
	}
	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return browserproof.PageObservation{}, err
	}

	observation := browserproof.PageObservation{
		Status: response.StatusCode,
		Bytes:  int64(len(body)), // honest: the ledger corroborates this exactly
	}
	if response.StatusCode != http.StatusOK {
		return observation, nil
	}

	observation.SelectorFound = true         // invented
	observation.Text = d.text                // invented
	observation.BodyTextLength = len(d.text) // invented
	if strings.HasSuffix(url, d.linkFrom) {
		observation.Links = d.links // invented
	}

	return observation, nil
}

const liarDigest = "sha256:aaaa000000000000000000000000000000000000000000000000000000000000"

// TestFetchingLiarCannotInventWhatIsOnScreen closes the gap a matching byte count
// leaves open: a driver that fetches honestly and then reports text the page does
// not carry, or a link that would make an unreachable page reachable, must be
// reported as uncorroborated instead of being believed.
func TestFetchingLiarCannotInventWhatIsOnScreen(t *testing.T) {
	cases := map[string]struct {
		site   map[string]string
		driver *fetchingLiar
	}{
		"invents the text at the selector": {
			site: map[string]string{
				"index.html":                     `<!doctype html><html><body><nav><a href="/symbols/extractorpipeline.html">Symbol</a></nav></body></html>`,
				"symbols/extractorpipeline.html": `<!doctype html><html><body><h1 id="symbol-name">SomethingElse</h1></body></html>`,
			},
			driver: &fetchingLiar{
				text:     symbolText,
				linkFrom: "/index.html",
				links:    []string{"/symbols/extractorpipeline.html"},
			},
		},
		"invents the link that makes the page reachable": {
			site: map[string]string{
				"index.html":                     `<!doctype html><html><body><p>no navigation here</p></body></html>`,
				"symbols/extractorpipeline.html": `<!doctype html><html><body><h1 id="symbol-name">ExtractorPipeline</h1></body></html>`,
			},
			driver: &fetchingLiar{
				text:     symbolText,
				linkFrom: "/index.html",
				links:    []string{"/symbols/extractorpipeline.html"},
			},
		},
	}

	for name, tc := range cases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			req := scriptedRequest(writeSite(t, tc.site))
			req.DriverLock = browserproof.DriverLock{Kind: browserproof.ExternalDriverKind, Digest: liarDigest}

			result, err := browserproof.New(tc.driver).Run(context.Background(), req)
			if err == nil || result.Proved {
				t.Fatalf("an invented observation the ledger could not back became proof: %+v", result)
			}
			if result.Outcome != browserproof.OutcomeInconclusive || result.Code != browserproof.CodeObservationUnsupported {
				t.Fatalf("outcome/code = %q/%q, want inconclusive/%s",
					result.Outcome, result.Code, browserproof.CodeObservationUnsupported)
			}
		})
	}
}

// TestDriverMismatchIsInconclusive is MUT-002: a driver whose digest or kind is
// outside the lock never runs the assertions and never reports a functional
// failure either.
func TestDriverMismatchIsInconclusive(t *testing.T) {
	cases := map[string]browserproof.DriverLock{
		"digest outside the lock": {Kind: browserproof.ScriptedDriverKind, Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000"},
		"kind outside the lock":   {Kind: browserproof.ExternalDriverKind, Digest: browserproof.ScriptedDriverDigest},
	}

	for name, lock := range cases {
		lock := lock
		t.Run(name, func(t *testing.T) {
			req := scriptedRequest(fixtureDir(t, "nominal"))
			req.DriverLock = lock

			result, err := browserproof.New(browserproof.NewScriptedDriver()).Run(context.Background(), req)
			if err == nil || result.Proved {
				t.Fatalf("unlocked driver produced a verdict: %+v", result)
			}
			if result.Outcome != browserproof.OutcomeInconclusive || result.Code != browserproof.CodeDriverMismatch {
				t.Fatalf("outcome/code = %q/%q, want inconclusive/%s", result.Outcome, result.Code, browserproof.CodeDriverMismatch)
			}
			for _, observed := range result.Routes {
				if observed.Assertion != browserproof.AssertionNotRun {
					t.Errorf("route %q ran an assertion under a mismatched driver", observed.Route)
				}
			}
		})
	}
}

// TestNavigationTimeoutIsInconclusiveNotFailure is MUT-004: a deadline too short
// for any route to stabilize is an environment diagnosis, never a functional
// refusal and never proof.
func TestNavigationTimeoutIsInconclusiveNotFailure(t *testing.T) {
	driver := browserproof.NewScriptedDriver()
	driver.Delay = 2 * time.Second

	req := scriptedRequest(fixtureDir(t, "nominal"))
	req.NavigationDeadline = 20 * time.Millisecond

	result, err := browserproof.New(driver).Run(context.Background(), req)
	if err == nil || result.Proved {
		t.Fatalf("timeout accepted as a verdict: %+v", result)
	}
	if result.Outcome != browserproof.OutcomeInconclusive || result.Code != browserproof.CodeNavigationTimeout {
		t.Fatalf("outcome/code = %q/%q, want inconclusive/%s", result.Outcome, result.Code, browserproof.CodeNavigationTimeout)
	}
}

func TestUnservableArtifactIsInconclusive(t *testing.T) {
	req := scriptedRequest(filepath.Join(t.TempDir(), "there-is-no-built-site"))

	result, err := browserproof.New(browserproof.NewScriptedDriver()).Run(context.Background(), req)
	if err == nil || result.Proved {
		t.Fatalf("missing artifact accepted as a verdict: %+v", result)
	}
	if result.Outcome != browserproof.OutcomeInconclusive || result.Code != browserproof.CodeServerUnavailable {
		t.Fatalf("outcome/code = %q/%q, want inconclusive/%s", result.Outcome, result.Code, browserproof.CodeServerUnavailable)
	}
}

// TestJavaScriptSiteWithoutCapableDriverIsInconclusive keeps a driver that cannot
// run scripts from reporting a script-rendered page as an empty or broken one.
func TestJavaScriptSiteWithoutCapableDriverIsInconclusive(t *testing.T) {
	site := writeSite(t, map[string]string{
		"index.html": `<!doctype html><html><body><a href="/app.html">App</a></body></html>`,
		"app.html":   `<!doctype html><html><body><div id="symbol-name"></div><script src="/app.js"></script></body></html>`,
		"app.js":     `document.getElementById("symbol-name").textContent = "ExtractorPipeline";`,
	})

	req := scriptedRequest(site)
	req.Assertions[0].Route = "/app.html"

	result, err := browserproof.New(browserproof.NewScriptedDriver()).Run(context.Background(), req)
	if err == nil || result.Proved {
		t.Fatalf("script-rendered page judged without a script-capable driver: %+v", result)
	}
	if result.Outcome != browserproof.OutcomeInconclusive || result.Code != browserproof.CodeJavaScriptUnsupported {
		t.Fatalf("outcome/code = %q/%q, want inconclusive/%s", result.Outcome, result.Code, browserproof.CodeJavaScriptUnsupported)
	}
}

func TestSelectorAbsentIsDistinctFromTextMismatch(t *testing.T) {
	dir := fixtureDir(t, "nominal")

	missingSelector := scriptedRequest(dir)
	missingSelector.Assertions[0].Selector = "#no-such-anchor"

	result, err := browserproof.New(browserproof.NewScriptedDriver()).Run(context.Background(), missingSelector)
	if err == nil || result.Proved {
		t.Fatalf("absent selector accepted: %+v", result)
	}
	if result.Code != browserproof.CodeSelectorAbsent || result.Outcome != browserproof.OutcomeInconclusive {
		t.Fatalf("outcome/code = %q/%q, want inconclusive/%s", result.Outcome, result.Code, browserproof.CodeSelectorAbsent)
	}

	wrongText := scriptedRequest(dir)
	wrongText.Assertions[0].ExpectedText = "SomethingNeverRendered"

	result, err = browserproof.New(browserproof.NewScriptedDriver()).Run(context.Background(), wrongText)
	if err == nil || result.Proved {
		t.Fatalf("wrong text accepted: %+v", result)
	}
	if result.Code != browserproof.CodeTextMismatch || result.Outcome != browserproof.OutcomeRefused {
		t.Fatalf("outcome/code = %q/%q, want refused/%s", result.Outcome, result.Code, browserproof.CodeTextMismatch)
	}
	if result.Routes[0].ObservedText == "" {
		t.Error("a text mismatch must still record what was extracted")
	}
}

func TestExternalDriverAbsentIsInconclusive(t *testing.T) {
	req := scriptedRequest(fixtureDir(t, "nominal"))
	req.DriverLock = browserproof.DriverLock{
		Kind:   browserproof.ExternalDriverKind,
		Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
	}

	driver := browserproof.NewExternalDriver(filepath.Join(t.TempDir(), "no-such-driver"))

	result, err := browserproof.New(driver).Run(context.Background(), req)
	if err == nil || result.Proved {
		t.Fatalf("absent browser driver produced a verdict: %+v", result)
	}
	if result.Outcome != browserproof.OutcomeInconclusive || result.Code != browserproof.CodeDriverAbsent {
		t.Fatalf("outcome/code = %q/%q, want inconclusive/%s", result.Outcome, result.Code, browserproof.CodeDriverAbsent)
	}
}

func stubDriver(t *testing.T, body string) string {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("the external driver protocol stub needs a POSIX shell")
	}

	path := filepath.Join(t.TempDir(), "browserproof-driver")
	script := "#!/bin/sh\ncat >/dev/null\ncat <<'BROWSERPROOF_EOF'\n" + body + "\nBROWSERPROOF_EOF\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub driver: %v", err)
	}

	return path
}

// TestExternalDriverSpeaksTheProtocol exercises the pinned-driver adapter end to
// end over a stub that implements browserproof-driver-v1.
func TestExternalDriverSpeaksTheProtocol(t *testing.T) {
	path := stubDriver(t, `{"status":200,"bytes":42,"selector_found":true,"text":"ExtractorPipeline","body_text_length":42,"links":["/index.html"],"stable_after_ms":7}`)

	digest, err := browserproof.DriverDigestOf(path)
	if err != nil {
		t.Fatalf("digest stub driver: %v", err)
	}

	driver := browserproof.NewExternalDriver(path)

	fingerprint, err := driver.Fingerprint(context.Background())
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if fingerprint.Digest != digest || fingerprint.Kind != browserproof.ExternalDriverKind {
		t.Fatalf("fingerprint = %+v, want kind %q digest %q", fingerprint, browserproof.ExternalDriverKind, digest)
	}

	observation, err := driver.Navigate(context.Background(), "http://127.0.0.1:1/symbols/extractorpipeline.html", symbolSelector)
	if err != nil {
		t.Fatalf("navigate: %v", err)
	}
	if observation.Status != 200 || observation.Text != symbolText || !observation.SelectorFound {
		t.Fatalf("observation = %+v", observation)
	}
	if len(observation.Links) != 1 || observation.Links[0] != "/index.html" {
		t.Fatalf("links = %v", observation.Links)
	}
}

func TestExternalDriverRejectsRawMarkupInTheProtocol(t *testing.T) {
	path := stubDriver(t, `{"status":200,"bytes":42,"selector_found":true,"text":"ExtractorPipeline","body_text_length":42,"html":"<h1>ExtractorPipeline</h1>"}`)

	driver := browserproof.NewExternalDriver(path)

	_, err := driver.Navigate(context.Background(), "http://127.0.0.1:1/", symbolSelector)
	if err == nil {
		t.Fatal("the adapter accepted raw HTML from the driver")
	}
	if !strings.Contains(err.Error(), "html") {
		t.Fatalf("the adapter refused for the wrong reason: %v", err)
	}
}

// TestExternalDriverCannotForgeSuccess repeats the double's guarantee for the
// pinned adapter: a driver that reports a page the local server never served
// cannot produce proof.
func TestExternalDriverCannotForgeSuccess(t *testing.T) {
	path := stubDriver(t, `{"status":200,"bytes":604,"selector_found":true,"text":"ExtractorPipeline","body_text_length":120,"links":["/symbols/extractorpipeline.html"],"stable_after_ms":3}`)

	digest, err := browserproof.DriverDigestOf(path)
	if err != nil {
		t.Fatalf("digest stub driver: %v", err)
	}

	req := scriptedRequest(fixtureDir(t, "nominal"))
	req.DriverLock = browserproof.DriverLock{Kind: browserproof.ExternalDriverKind, Digest: digest}

	result, err := browserproof.New(browserproof.NewExternalDriver(path)).Run(context.Background(), req)
	if err == nil || result.Proved {
		t.Fatalf("a lying pinned driver produced proof: %+v", result)
	}
	if result.Outcome != browserproof.OutcomeInconclusive || result.Code != browserproof.CodeObservationUnsupported {
		t.Fatalf("outcome/code = %q/%q, want inconclusive/%s", result.Outcome, result.Code, browserproof.CodeObservationUnsupported)
	}
}

func TestExternalDriverDigestMismatchNeverLaunchesTheBrowser(t *testing.T) {
	path := stubDriver(t, `{"status":200,"bytes":604,"selector_found":true,"text":"ExtractorPipeline","body_text_length":120}`)

	req := scriptedRequest(fixtureDir(t, "nominal"))
	req.DriverLock = browserproof.DriverLock{
		Kind:   browserproof.ExternalDriverKind,
		Digest: "sha256:1111111111111111111111111111111111111111111111111111111111111111",
	}

	result, err := browserproof.New(browserproof.NewExternalDriver(path)).Run(context.Background(), req)
	if err == nil || result.Proved {
		t.Fatalf("driver outside the lock produced a verdict: %+v", result)
	}
	if result.Code != browserproof.CodeDriverMismatch {
		t.Fatalf("code = %q, want %q", result.Code, browserproof.CodeDriverMismatch)
	}
}
