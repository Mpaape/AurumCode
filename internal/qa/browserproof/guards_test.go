package browserproof_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/qa/browserproof"
)

// This file puts every refusal the harness owns under a test that fails when the
// refusal is deleted. A guard nobody exercises is a guard the next refactor
// removes in silence, and this package exists precisely to refuse things.

// tweakedDriver wraps the honest scripted double and rewrites one field of its
// observation, so a single corroboration guard carries the whole weight of a
// case instead of several of them overlapping.
type tweakedDriver struct {
	inner  *browserproof.ScriptedDriver
	runsJS bool
	tweak  func(route string, observation *browserproof.PageObservation)
}

func (d *tweakedDriver) Fingerprint(ctx context.Context) (browserproof.DriverFingerprint, error) {
	fingerprint, err := d.inner.Fingerprint(ctx)
	fingerprint.ExecutesJavaScript = d.runsJS

	return fingerprint, err
}

func (d *tweakedDriver) Navigate(ctx context.Context, rawURL, selector string) (browserproof.PageObservation, error) {
	observation, err := d.inner.Navigate(ctx, rawURL, selector)
	if err != nil {
		return observation, err
	}
	if d.tweak != nil {
		d.tweak(routeOf(rawURL), &observation)
	}

	return observation, nil
}

func tweaked(tweak func(route string, observation *browserproof.PageObservation)) *tweakedDriver {
	return &tweakedDriver{inner: browserproof.NewScriptedDriver(), tweak: tweak}
}

// rawFetchDriver reports only what a status line and a byte count can prove, and
// then whatever the test asks it to invent. It never reads the selector, so a
// case can reach the harness's own corroboration instead of tripping the
// double's parser first.
type rawFetchDriver struct {
	tweak func(route string, observation *browserproof.PageObservation)
}

func (d *rawFetchDriver) Fingerprint(context.Context) (browserproof.DriverFingerprint, error) {
	return browserproof.DriverFingerprint{
		Kind:   browserproof.ScriptedDriverKind,
		Digest: browserproof.ScriptedDriverDigest,
	}, nil
}

func (d *rawFetchDriver) Navigate(ctx context.Context, rawURL, _ string) (browserproof.PageObservation, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return browserproof.PageObservation{}, err
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return browserproof.PageObservation{}, err
	}
	defer func() { _ = response.Body.Close() }()

	body := make([]byte, 0, 1024)
	buffer := make([]byte, 32*1024)
	for {
		n, readErr := response.Body.Read(buffer)
		body = append(body, buffer[:n]...)
		if readErr != nil {
			break
		}
	}

	observation := browserproof.PageObservation{
		Status: response.StatusCode,
		Bytes:  int64(len(body)),
	}
	if d.tweak != nil {
		d.tweak(routeOf(rawURL), &observation)
	}

	return observation, nil
}

func routeOf(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	return parsed.Path
}

// singleRouteRequest points the crawl and the assertion at the same route, so a
// case about corroboration is not decided by reachability instead.
func singleRouteRequest(siteDir, route, selector, expected string) browserproof.RunRequest {
	req := scriptedRequest(siteDir)
	req.EntryRoute = route
	req.Assertions = []browserproof.RouteAssertion{{Route: route, Selector: selector, ExpectedText: expected}}

	return req
}

func mustRefuse(t *testing.T, result browserproof.BrowserProofResultV1, err error, outcome browserproof.Outcome, code string) {
	t.Helper()

	if err == nil || result.Proved {
		t.Fatalf("the harness produced a verdict it cannot back: %+v", result)
	}
	if result.Outcome != outcome || result.Code != code {
		t.Fatalf("outcome/code = %q/%q, want %q/%q", result.Outcome, result.Code, outcome, code)
	}
}

// TestOversizedArtifactIsNeverCorroboratedFromMemory covers the ledger budget:
// past maxLedgerBodyBytes the harness keeps no copy of a page, and a route it
// cannot check the driver against is reported as uncorroborated instead of being
// believed. Deleting either half — the drop in the server ledger or the nil-body
// refusal in corroborateContent — turns an unchecked observation into evidence.
func TestOversizedArtifactIsNeverCorroboratedFromMemory(t *testing.T) {
	const pageBytes = 4_000_000 // just under MaxPageBytes, five of them pass the ledger budget

	filler := strings.Repeat("documentation ", pageBytes/14)
	site := map[string]string{}

	var links strings.Builder
	for i := 1; i <= 5; i++ {
		route := fmt.Sprintf("p%d.html", i)
		links.WriteString(fmt.Sprintf(`<li><a href="/%s">page %d</a></li>`, route, i))
		site[route] = fmt.Sprintf(
			`<!doctype html><html><body><h1 id="symbol-name">%s</h1><p>%s</p></body></html>`,
			symbolText, filler)
	}
	site["index.html"] = `<!doctype html><html><body><ul>` + links.String() + `</ul></body></html>`

	req := scriptedRequest(writeSite(t, site))
	req.Assertions = []browserproof.RouteAssertion{{
		Route: "/p5.html", Selector: symbolSelector, ExpectedText: symbolText,
	}}

	result, err := browserproof.New(browserproof.NewScriptedDriver()).Run(context.Background(), req)
	mustRefuse(t, result, err, browserproof.OutcomeInconclusive, browserproof.CodeObservationUnsupported)

	if !strings.Contains(result.Detail, "kept no copy") {
		t.Fatalf("a route past the ledger budget must say the harness kept no copy to check against, got %q", result.Detail)
	}
}

// TestScriptRenderedPageIsNeverJudgedFromTheDOM keeps a script-capable driver
// from being taken at its word: the harness has no static ground truth for a
// page that reaches its rendered state through script, so it says so.
func TestScriptRenderedPageIsNeverJudgedFromTheDOM(t *testing.T) {
	site := writeSite(t, map[string]string{
		"app.html": `<!doctype html><html><body><h1 id="symbol-name">ExtractorPipeline</h1>` +
			`<script>document.title = "rendered";</script></body></html>`,
	})

	driver := tweaked(nil)
	driver.runsJS = true

	result, err := browserproof.New(driver).Run(
		context.Background(), singleRouteRequest(site, "/app.html", symbolSelector, symbolText))
	mustRefuse(t, result, err, browserproof.OutcomeInconclusive, browserproof.CodeObservationUnsupported)

	if !strings.Contains(result.Detail, "script") {
		t.Fatalf("the refusal must name the script it cannot corroborate, got %q", result.Detail)
	}
}

// TestDriverCannotReportTextOnAPageThatRendersNone closes the gap where a driver
// claims rendered text on a page whose served bytes render nothing at all.
func TestDriverCannotReportTextOnAPageThatRendersNone(t *testing.T) {
	site := writeSite(t, map[string]string{
		"blank.html": `<!doctype html><html><head><title>ExtractorPipeline</title></head><body></body></html>`,
	})

	driver := tweaked(func(_ string, observation *browserproof.PageObservation) {
		observation.BodyTextLength = len(symbolText) // invented: the body renders nothing
	})

	result, err := browserproof.New(driver).Run(
		context.Background(), singleRouteRequest(site, "/blank.html", symbolSelector, symbolText))
	mustRefuse(t, result, err, browserproof.OutcomeInconclusive, browserproof.CodeObservationUnsupported)

	if !strings.Contains(result.Detail, "renders none") {
		t.Fatalf("the refusal must name the empty render it could not corroborate, got %q", result.Detail)
	}
}

// TestUnsupportedSelectorIsARequestFaultNotAVerdict pins the refusal of a
// selector the harness cannot evaluate: it is the request that is wrong, and the
// artifact is neither proved nor refused because of it.
func TestUnsupportedSelectorIsARequestFaultNotAVerdict(t *testing.T) {
	site := writeSite(t, map[string]string{
		"page.html": `<!doctype html><html><body><div><p id="symbol-name">ExtractorPipeline</p></div></body></html>`,
	})

	// The double never parses the selector, so the refusal can only come from
	// the harness's own corroboration step.
	driver := &rawFetchDriver{tweak: func(_ string, observation *browserproof.PageObservation) {
		observation.SelectorFound = true
		observation.Text = symbolText
		observation.BodyTextLength = len(symbolText)
	}}

	result, err := browserproof.New(driver).Run(
		context.Background(), singleRouteRequest(site, "/page.html", "div > p", symbolText))
	mustRefuse(t, result, err, browserproof.OutcomeInconclusive, browserproof.CodeRequestInvalid)
}

// TestDriverCannotInventASelectorTheServedPageLacks is the forgery the whole
// package exists to refuse: without this check a driver that fetches honestly
// and then claims the selector and its text produces a proved verdict over a
// page that carries neither.
func TestDriverCannotInventASelectorTheServedPageLacks(t *testing.T) {
	site := writeSite(t, map[string]string{
		"page.html": `<!doctype html><html><body><p>no symbol heading here</p></body></html>`,
	})

	driver := &rawFetchDriver{tweak: func(_ string, observation *browserproof.PageObservation) {
		observation.SelectorFound = true // invented
		observation.Text = symbolText    // invented
		observation.BodyTextLength = len(symbolText)
	}}

	result, err := browserproof.New(driver).Run(
		context.Background(), singleRouteRequest(site, "/page.html", symbolSelector, symbolText))
	mustRefuse(t, result, err, browserproof.OutcomeInconclusive, browserproof.CodeObservationUnsupported)

	if !strings.Contains(result.Detail, "selector_found") {
		t.Fatalf("the refusal must name the selector claim it could not corroborate, got %q", result.Detail)
	}
}

// TestDriverReturningMarkupIsADriverFault keeps raw HTML out of the evidence at
// the seam where it would enter: a driver that answers with markup instead of
// rendered text is faulty, and its answer is not a verdict.
func TestDriverReturningMarkupIsADriverFault(t *testing.T) {
	driver := tweaked(func(_ string, observation *browserproof.PageObservation) {
		observation.Text = `<h1 id="symbol-name">ExtractorPipeline</h1>`
	})

	result, err := browserproof.New(driver).Run(context.Background(), scriptedRequest(fixtureDir(t, "nominal")))
	mustRefuse(t, result, err, browserproof.OutcomeInconclusive, browserproof.CodeDriverFault)

	for _, route := range result.Routes {
		if strings.Contains(route.ObservedText, "<") {
			t.Fatalf("markup reached the evidence: %q", route.ObservedText)
		}
	}
}

// TestDriverStatusMustMatchTheServedStatus stops a driver from turning a missing
// page into a delivered one: a 200 the server never sent is uncorroborated, not
// an absent target and certainly not proof.
func TestDriverStatusMustMatchTheServedStatus(t *testing.T) {
	site := writeSite(t, map[string]string{
		"index.html": `<!doctype html><html><body><p>index</p></body></html>`,
	})

	driver := &rawFetchDriver{tweak: func(_ string, observation *browserproof.PageObservation) {
		observation.Status = http.StatusOK // invented: the route does not exist
		observation.Bytes = 0              // keep the byte check out of this case
	}}

	result, err := browserproof.New(driver).Run(
		context.Background(), singleRouteRequest(site, "/gone.html", symbolSelector, symbolText))
	mustRefuse(t, result, err, browserproof.OutcomeInconclusive, browserproof.CodeObservationUnsupported)
}

// TestDriverByteCountMustMatchTheServedBytes is the last cheap corroboration
// before content checks: a driver whose byte count does not match the ledger read
// a different page from the one the harness served.
func TestDriverByteCountMustMatchTheServedBytes(t *testing.T) {
	driver := tweaked(func(_ string, observation *browserproof.PageObservation) {
		observation.Bytes++ // everything else about this observation is honest
	})

	result, err := browserproof.New(driver).Run(context.Background(), scriptedRequest(fixtureDir(t, "nominal")))
	mustRefuse(t, result, err, browserproof.OutcomeInconclusive, browserproof.CodeObservationUnsupported)

	if !strings.Contains(result.Detail, "bytes") {
		t.Fatalf("the refusal must name the byte count it could not corroborate, got %q", result.Detail)
	}
}

// TestOversizedPageIsAServerDiagnosisNotAnEmptyRender covers two refusals that
// must not be confused: the server refuses a page past MaxPageBytes, and Run
// reports a non-200 as an environment diagnosis instead of reading the empty
// body it got as a feature the artifact failed to render.
func TestOversizedPageIsAServerDiagnosisNotAnEmptyRender(t *testing.T) {
	site := writeSite(t, map[string]string{
		"big.html": `<!doctype html><html><body><h1 id="symbol-name">ExtractorPipeline</h1><p>` +
			strings.Repeat("x", browserproof.MaxPageBytes) + `</p></body></html>`,
	})

	result, err := browserproof.New(browserproof.NewScriptedDriver()).Run(
		context.Background(), singleRouteRequest(site, "/big.html", symbolSelector, symbolText))
	mustRefuse(t, result, err, browserproof.OutcomeInconclusive, browserproof.CodeServerUnavailable)

	if result.Routes[0].Status != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", result.Routes[0].Status, http.StatusRequestEntityTooLarge)
	}
}

// TestEnvironmentDiagnosisOutranksFunctionalRefusal pins the precedence rule: a
// run that could not judge one route has not refused the feature either, however
// the routes are ordered.
func TestEnvironmentDiagnosisOutranksFunctionalRefusal(t *testing.T) {
	req := scriptedRequest(fixtureDir(t, "nominal"))
	req.Assertions = []browserproof.RouteAssertion{
		// A functional refusal comes first, so an implementation that reports
		// the first refusal it finds reports this one.
		{Route: symbolRoute, Selector: symbolSelector, ExpectedText: "SomethingNeverRendered"},
		// The harness could not judge this one at all.
		{Route: "/symbols/index.html", Selector: "#no-such-anchor", ExpectedText: "Symbols"},
	}

	result, err := browserproof.New(browserproof.NewScriptedDriver()).Run(context.Background(), req)
	mustRefuse(t, result, err, browserproof.OutcomeInconclusive, browserproof.CodeSelectorAbsent)

	if result.Routes[0].Code != browserproof.CodeTextMismatch {
		t.Fatalf("the refused route must keep its own code, got %q", result.Routes[0].Code)
	}
}

// TestCrawlBudgetKeepsAFarPageUnreachable pins maxCrawlPages: past the budget the
// harness stops walking, and a page it never reached is reported unreachable
// instead of being assumed delivered.
func TestCrawlBudgetKeepsAFarPageUnreachable(t *testing.T) {
	const chain = 40

	site := map[string]string{
		"index.html": `<!doctype html><html><body><a href="/p1.html">next</a></body></html>`,
	}
	for i := 1; i <= chain; i++ {
		site[fmt.Sprintf("p%d.html", i)] = fmt.Sprintf(
			`<!doctype html><html><body><h1 id="symbol-name">%s</h1><a href="/p%d.html">next</a></body></html>`,
			symbolText, i+1)
	}
	site[fmt.Sprintf("p%d.html", chain+1)] = fmt.Sprintf(
		`<!doctype html><html><body><h1 id="symbol-name">%s</h1></body></html>`, symbolText)

	req := scriptedRequest(writeSite(t, site))
	req.Assertions = []browserproof.RouteAssertion{{
		Route: fmt.Sprintf("/p%d.html", chain), Selector: symbolSelector, ExpectedText: symbolText,
	}}

	result, err := browserproof.New(browserproof.NewScriptedDriver()).Run(context.Background(), req)
	mustRefuse(t, result, err, browserproof.OutcomeRefused, browserproof.CodeUnreachableRoute)
}

// TestRequestGuardsRefuseAnUnrunnableRun keeps a malformed request from being
// answered with a verdict about the artifact.
func TestRequestGuardsRefuseAnUnrunnableRun(t *testing.T) {
	cases := map[string]func(req *browserproof.RunRequest){
		"no artifact directory":           func(req *browserproof.RunRequest) { req.SiteDir = "   " },
		"no route assertion":              func(req *browserproof.RunRequest) { req.Assertions = nil },
		"driver lock without a kind":      func(req *browserproof.RunRequest) { req.DriverLock.Kind = "" },
		"driver lock without a digest":    func(req *browserproof.RunRequest) { req.DriverLock.Digest = "" },
		"assertion without a selector":    func(req *browserproof.RunRequest) { req.Assertions[0].Selector = "  " },
		"assertion without expected text": func(req *browserproof.RunRequest) { req.Assertions[0].ExpectedText = "" },
		"deadline that is not positive":   func(req *browserproof.RunRequest) { req.NavigationDeadline = 0 },
	}

	for name, mutate := range cases {
		mutate := mutate
		t.Run(name, func(t *testing.T) {
			req := scriptedRequest(fixtureDir(t, "nominal"))
			mutate(&req)

			result, err := browserproof.New(browserproof.NewScriptedDriver()).Run(context.Background(), req)
			mustRefuse(t, result, err, browserproof.OutcomeInconclusive, browserproof.CodeRequestInvalid)
		})
	}
}

// faultyFingerprintDriver fails for a reason that is not an absent browser.
type faultyFingerprintDriver struct{}

func (faultyFingerprintDriver) Fingerprint(context.Context) (browserproof.DriverFingerprint, error) {
	return browserproof.DriverFingerprint{}, errors.New("the driver refused to report its identity")
}

func (faultyFingerprintDriver) Navigate(context.Context, string, string) (browserproof.PageObservation, error) {
	return browserproof.PageObservation{}, errors.New("unreachable")
}

// TestUnidentifiableDriverIsAFaultNotAnAbsence keeps the two environment
// diagnoses apart: a driver that is installed but broken is not a missing one.
func TestUnidentifiableDriverIsAFaultNotAnAbsence(t *testing.T) {
	result, err := browserproof.New(faultyFingerprintDriver{}).Run(
		context.Background(), scriptedRequest(fixtureDir(t, "nominal")))
	mustRefuse(t, result, err, browserproof.OutcomeInconclusive, browserproof.CodeDriverFault)
}

// TestObservedTextAndDetailAreBounded keeps the evidence bounded at the source:
// a long rendered text and a long refusal detail are truncated before they reach
// a verdict, so the artifact cannot push unbounded content into the record.
func TestObservedTextAndDetailAreBounded(t *testing.T) {
	t.Run("observed text", func(t *testing.T) {
		long := symbolText + " " + strings.Repeat("documentation ", 200)
		site := writeSite(t, map[string]string{
			"page.html": `<!doctype html><html><body><h1 id="symbol-name">` + long + `</h1></body></html>`,
		})

		result, err := browserproof.New(browserproof.NewScriptedDriver()).Run(
			context.Background(), singleRouteRequest(site, "/page.html", symbolSelector, symbolText))
		if err != nil {
			t.Fatalf("a long but honest page must still be judged: %v", err)
		}

		observed := []rune(result.Routes[0].ObservedText)
		if len(observed) != 513 || observed[512] != '…' {
			t.Fatalf("observed text is %d runes and ends with %q, want 512 runes plus an ellipsis",
				len(observed), string(observed[len(observed)-1]))
		}
	})

	t.Run("refusal detail", func(t *testing.T) {
		route := "/" + strings.Repeat("long-route-segment/", 20) + "page.html"
		site := writeSite(t, map[string]string{
			"index.html": `<!doctype html><html><body><p>index</p></body></html>`,
		})

		driver := browserproof.NewScriptedDriver()
		driver.Forge = browserproof.ForgeWithoutFetch
		driver.ForgedText = symbolText

		result, err := browserproof.New(driver).Run(
			context.Background(), singleRouteRequest(site, route, symbolSelector, symbolText))
		mustRefuse(t, result, err, browserproof.OutcomeInconclusive, browserproof.CodeObservationUnsupported)

		detail := []rune(result.Detail)
		if len(detail) != 241 || detail[240] != '…' {
			t.Fatalf("detail is %d runes, want 240 runes plus an ellipsis: %q", len(detail), result.Detail)
		}
	})
}

// TestHiddenMarkupIsNotRenderedText keeps text a user never sees from standing in
// for a rendered page: a body whose only content is hidden renders nothing.
func TestHiddenMarkupIsNotRenderedText(t *testing.T) {
	cases := map[string]string{
		"head":     `<!doctype html><html><head><title>ExtractorPipeline</title></head><body></body></html>`,
		"style":    `<!doctype html><html><body><style>#symbol-name{color:red}</style></body></html>`,
		"noscript": `<!doctype html><html><body><noscript>ExtractorPipeline</noscript></body></html>`,
		"template": `<!doctype html><html><body><template>ExtractorPipeline</template></body></html>`,
	}

	for name, page := range cases {
		page := page
		t.Run(name, func(t *testing.T) {
			site := writeSite(t, map[string]string{"page.html": page})

			result, err := browserproof.New(browserproof.NewScriptedDriver()).Run(
				context.Background(), singleRouteRequest(site, "/page.html", symbolSelector, symbolText))
			mustRefuse(t, result, err, browserproof.OutcomeRefused, browserproof.CodeEmptyRender)
		})
	}
}

// TestScriptedDriverRefusesANonLoopbackAddress keeps a proof run inside the local
// artifact: the double dials the loopback interface or nothing at all.
func TestScriptedDriverRefusesANonLoopbackAddress(t *testing.T) {
	// 192.0.2.0/24 is the reserved TEST-NET-1 block: the guard must refuse it
	// before a packet is ever sent.
	_, err := browserproof.NewScriptedDriver().Navigate(
		context.Background(), "http://192.0.2.1:80/index.html", "")
	if err == nil {
		t.Fatal("the double dialed an address outside loopback")
	}
	if !strings.Contains(err.Error(), "non-loopback") {
		t.Fatalf("the double refused for the wrong reason: %v", err)
	}
}

// TestScriptedDriverRefusesAnOversizedPage keeps a page past MaxPageBytes from
// being read into the harness even when something else served it.
func TestScriptedDriverRefusesAnOversizedPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(strings.Repeat("x", browserproof.MaxPageBytes+1)))
	}))
	defer server.Close()

	_, err := browserproof.NewScriptedDriver().Navigate(context.Background(), server.URL+"/big.html", "")
	if err == nil {
		t.Fatal("the double read a page past the size ceiling")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("the double refused for the wrong reason: %v", err)
	}
}

// TestScriptedDriverRefusesAnUnsupportedSelector keeps the selector subset
// honest: a construct the harness cannot evaluate is refused instead of being
// silently read as a tag name that matches nothing.
func TestScriptedDriverRefusesAnUnsupportedSelector(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><body><div><p>ExtractorPipeline</p></div></body></html>`))
	}))
	defer server.Close()

	for _, selector := range []string{"div > p", "p[data-x]", "h1:first-child", "#a.b"} {
		_, err := browserproof.NewScriptedDriver().Navigate(context.Background(), server.URL+"/page.html", selector)
		if err == nil {
			t.Fatalf("selector %q was accepted", selector)
		}
	}
}

func canaryStub(t *testing.T, body string) string {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("the external driver protocol stub needs a POSIX shell")
	}

	path := filepath.Join(t.TempDir(), "browserproof-driver")
	if err := os.WriteFile(path, []byte("#!/bin/sh\ncat >/dev/null\n"+body+"\n"), 0o755); err != nil {
		t.Fatalf("write stub driver: %v", err)
	}

	return path
}

// TestExternalDriverInheritsNoSecret is the canary AUR-423 requires: the browser
// process runs without the runner's environment, so a secret in the runner never
// reaches the driver, the observation or the verdict.
func TestExternalDriverInheritsNoSecret(t *testing.T) {
	const canary = "AURUM-CANARY-9c1f2b7e"

	t.Setenv("AURUM_SECRET_CANARY", canary)

	path := canaryStub(t, `printf '{"status":200,"bytes":4,"selector_found":true,`+
		`"text":"seen:%s","body_text_length":4}' "${AURUM_SECRET_CANARY:-absent}"`)

	observation, err := browserproof.NewExternalDriver(path).Navigate(
		context.Background(), "http://127.0.0.1:1/index.html", symbolSelector)
	if err != nil {
		t.Fatalf("navigate: %v", err)
	}
	if strings.Contains(observation.Text, canary) {
		t.Fatalf("the runner's secret reached the driver: %q", observation.Text)
	}
	if observation.Text != "seen:absent" {
		t.Fatalf("the driver saw an environment it should not have: %q", observation.Text)
	}
}

// TestExternalDriverResponseIsCapped refuses a driver that floods the harness
// instead of answering it.
func TestExternalDriverResponseIsCapped(t *testing.T) {
	path := canaryStub(t, `printf '{"status":200,"bytes":4,"selector_found":true,"text":"'
for i in $(seq 1 2100); do printf 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'; done
printf '","body_text_length":4}'`)

	_, err := browserproof.NewExternalDriver(path).Navigate(
		context.Background(), "http://127.0.0.1:1/index.html", symbolSelector)
	if err == nil {
		t.Fatal("the adapter accepted a driver response past the response ceiling")
	}
}

// TestDriverDigestRefusesSomethingThatIsNotAnExecutable keeps a directory or a
// missing path from being fingerprinted as a pinned browser.
func TestDriverDigestRefusesSomethingThatIsNotAnExecutable(t *testing.T) {
	for name, path := range map[string]string{
		"a directory": t.TempDir(),
		"nothing":     filepath.Join(t.TempDir(), "no-such-driver"),
		"no path":     "",
	} {
		path := path
		t.Run(name, func(t *testing.T) {
			digest, err := browserproof.DriverDigestOf(path)
			if err == nil {
				t.Fatalf("digest %q was accepted as a pinned browser", digest)
			}
			if !errors.Is(err, browserproof.ErrDriverAbsent) {
				t.Fatalf("want ErrDriverAbsent, got %v", err)
			}
		})
	}
}

// TestValidateRejectsAVerdictThatContradictsItself covers the contract checks
// that no run currently produces: a verdict handed to the board from anywhere
// must still carry the evidence it claims.
func TestValidateRejectsAVerdictThatContradictsItself(t *testing.T) {
	refused := func(t *testing.T) browserproof.BrowserProofResultV1 {
		result := provedResult(t)
		result.Proved = false
		result.Outcome = browserproof.OutcomeRefused
		result.Code = browserproof.CodeTargetAbsent
		result.Routes[0].Assertion = browserproof.AssertionFail

		return result
	}

	cases := map[string]func(t *testing.T) browserproof.BrowserProofResultV1{
		"an outcome outside the contract": func(t *testing.T) browserproof.BrowserProofResultV1 {
			result := refused(t)
			result.Outcome = "mostly-fine"

			return result
		},
		"a route without a path": func(t *testing.T) browserproof.BrowserProofResultV1 {
			result := refused(t)
			result.Routes[0].Route = ""

			return result
		},
		"an assertion outside the contract": func(t *testing.T) browserproof.BrowserProofResultV1 {
			result := refused(t)
			result.Routes[0].Assertion = "probably-passed"

			return result
		},
		"proved without the selector being found": func(t *testing.T) browserproof.BrowserProofResultV1 {
			result := provedResult(t)
			result.Routes[0].SelectorFound = false

			return result
		},
		"proved over a route nothing reaches": func(t *testing.T) browserproof.BrowserProofResultV1 {
			result := provedResult(t)
			result.Routes[0].Reachable = false

			return result
		},
		"proved over a route that did not answer 200": func(t *testing.T) browserproof.BrowserProofResultV1 {
			result := provedResult(t)
			result.Routes[0].Status = http.StatusAccepted

			return result
		},
	}

	for name, build := range cases {
		build := build
		t.Run(name, func(t *testing.T) {
			result := build(t)
			if err := result.Validate(); err == nil {
				t.Fatalf("a self-contradicting verdict was accepted: %+v", result)
			}
		})
	}
}

// TestProvedVerdictSurvivesTheContract is the counterweight to every refusal
// above: the nominal run must still validate, so the guards refuse forgeries
// rather than everything.
func TestProvedVerdictSurvivesTheContract(t *testing.T) {
	result := provedResult(t)
	if err := result.Validate(); err != nil {
		t.Fatalf("a real verdict was refused by its own contract: %v", err)
	}
	if result.DeadlineMs != (10 * time.Second).Milliseconds() {
		t.Fatalf("the verdict lost the navigation deadline: %d", result.DeadlineMs)
	}
}
