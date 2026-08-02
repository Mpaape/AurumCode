// Package browserproof drives a headless browser over a built site served on
// loopback and reports whether the effect promised to a user is actually on the
// screen. It separates three verdicts that a passing test suite conflates: the
// artifact proved the behavior, the artifact refused it, or the environment
// could not judge. Only the first is proof, and it is never inferred from a
// green run, from a driver that says so, or from a status code alone.
package browserproof

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"
)

const (
	maxCrawlPages        = 32
	maxObservedTextRunes = 512
	maxDetailRunes       = 240
)

// DriverLock pins the browser that is allowed to produce a verdict.
type DriverLock struct {
	Kind   string `json:"kind"`
	Digest string `json:"digest"`
}

// RouteAssertion declares what a user must be able to see on one route.
type RouteAssertion struct {
	Route        string
	Selector     string
	ExpectedText string
}

// RunRequest is one proof run over one built artifact.
type RunRequest struct {
	Card               string
	SiteDir            string
	EntryRoute         string
	Assertions         []RouteAssertion
	DriverLock         DriverLock
	NavigationDeadline time.Duration
}

// BrowserProof runs assertions through a Driver against a locally served build.
type BrowserProof struct {
	driver Driver
}

func New(driver Driver) *BrowserProof {
	return &BrowserProof{driver: driver}
}

// stepError carries the refusal code of a failed step out of a helper.
type stepError struct {
	code   string
	detail string
}

func (e *stepError) Error() string { return e.code + ": " + e.detail }

// Run serves the artifact on loopback, checks the driver against the lock,
// walks every declared route and asserts the selector and its text. It returns
// a non-nil error for every outcome that is not proof, so a caller that drops
// the verdict still cannot read a refusal as success.
func (b *BrowserProof) Run(ctx context.Context, req RunRequest) (BrowserProofResultV1, error) {
	result := BrowserProofResultV1{Schema: SchemaBrowserProofResultV1, Card: req.Card}

	req, err := normalizeRequest(req)
	if err != nil {
		return finalize(result, CodeRequestInvalid, err.Error())
	}

	result.EntryRoute = req.EntryRoute
	result.DeadlineMs = req.NavigationDeadline.Milliseconds()
	result.Routes = make([]RouteObservationV1, 0, len(req.Assertions))
	for _, assertion := range req.Assertions {
		result.Routes = append(result.Routes, RouteObservationV1{
			Route:        assertion.Route,
			Selector:     assertion.Selector,
			ExpectedText: assertion.ExpectedText,
			Assertion:    AssertionNotRun,
		})
	}

	digest, err := artifactDigest(req.SiteDir)
	if err != nil {
		return finalize(result, CodeServerUnavailable, err.Error())
	}
	result.ArtifactDigest = digest

	server, err := startSiteServer(req.SiteDir)
	if err != nil {
		return finalize(result, CodeServerUnavailable, err.Error())
	}
	defer server.Close()

	fingerprint, err := b.driver.Fingerprint(ctx)
	if err != nil {
		if errors.Is(err, ErrDriverAbsent) {
			return finalize(result, CodeDriverAbsent, err.Error())
		}
		return finalize(result, CodeDriverFault, err.Error())
	}
	result.DriverKind = fingerprint.Kind
	result.DriverDigest = fingerprint.Digest
	result.DriverRunsJS = fingerprint.ExecutesJavaScript

	if fingerprint.Kind != req.DriverLock.Kind || fingerprint.Digest != req.DriverLock.Digest {
		return finalize(result, CodeDriverMismatch, fmt.Sprintf(
			"driver %s %s is outside the lock %s %s",
			fingerprint.Kind, fingerprint.Digest, req.DriverLock.Kind, req.DriverLock.Digest))
	}

	reachable, err := b.reachableRoutes(ctx, server, req, fingerprint.ExecutesJavaScript)
	if err != nil {
		return finalizeStep(result, err)
	}

	for i := range req.Assertions {
		assertion := req.Assertions[i]
		row := &result.Routes[i]

		observation, served, err := b.observe(
			ctx, server, assertion.Route, assertion.Selector, req.NavigationDeadline, fingerprint.ExecutesJavaScript)
		if err != nil {
			var step *stepError
			if errors.As(err, &step) {
				row.Code = step.code
			}
			return finalizeStep(result, err)
		}

		row.Status = served.Status
		row.ServedBytes = served.Bytes
		row.ServedDigest = served.Digest
		row.StableAfterMs = observation.StableAfter.Milliseconds()
		row.Reachable = reachable[assertion.Route]
		row.SelectorFound = observation.SelectorFound
		row.ObservedText = sanitizeText(observation.Text)

		switch {
		case served.Status == http.StatusNotFound || served.Status == http.StatusGone:
			row.Assertion, row.Code = AssertionFail, CodeTargetAbsent
		case served.Status != http.StatusOK:
			row.Code = CodeServerUnavailable
		case (served.NeedsScript || observation.RequiresJavaScript) && !fingerprint.ExecutesJavaScript:
			row.Code = CodeJavaScriptUnsupported
		case served.Bytes == 0 || observation.BodyTextLength == 0:
			row.Assertion, row.Code = AssertionFail, CodeEmptyRender
		case !row.Reachable:
			row.Assertion, row.Code = AssertionFail, CodeUnreachableRoute
		case !observation.SelectorFound:
			row.Code = CodeSelectorAbsent
		case !strings.Contains(row.ObservedText, assertion.ExpectedText):
			row.Assertion, row.Code = AssertionFail, CodeTextMismatch
		default:
			row.Assertion = AssertionPass
		}
	}

	// An environment diagnosis outranks a functional refusal: a run that could
	// not judge one route has not refused the feature either.
	var inconclusive, refused *RouteObservationV1
	for i := range result.Routes {
		row := &result.Routes[i]
		switch outcomeOfCode(row.Code) {
		case OutcomeInconclusive:
			if inconclusive == nil {
				inconclusive = row
			}
		case OutcomeRefused:
			if refused == nil {
				refused = row
			}
		}
	}

	switch {
	case inconclusive != nil:
		return finalize(result, inconclusive.Code, "route "+inconclusive.Route)
	case refused != nil:
		return finalize(result, refused.Code, "route "+refused.Route)
	default:
		return finalize(result, "", "")
	}
}

// reachableRoutes walks the site from the entry route, so a page that exists but
// that no navigation reaches is never counted as delivered.
func (b *BrowserProof) reachableRoutes(
	ctx context.Context,
	server *siteServer,
	req RunRequest,
	runsJS bool,
) (map[string]bool, error) {
	reachable := map[string]bool{req.EntryRoute: true}
	visited := map[string]bool{}
	queue := []string{req.EntryRoute}

	for len(queue) > 0 && len(visited) < maxCrawlPages {
		route := queue[0]
		queue = queue[1:]
		if visited[route] {
			continue
		}
		visited[route] = true

		observation, served, err := b.observe(ctx, server, route, "", req.NavigationDeadline, runsJS)
		if err != nil {
			return nil, err
		}
		if served.Status != http.StatusOK {
			continue
		}

		for _, href := range observation.Links {
			target, ok := resolveLink(route, href)
			if !ok {
				continue
			}
			reachable[target] = true
			if !visited[target] {
				queue = append(queue, target)
			}
		}
	}

	return reachable, nil
}

// observe navigates one route and corroborates what the driver reported against
// what the local server actually served. A driver cannot hand out an
// observation the server never backed.
func (b *BrowserProof) observe(
	ctx context.Context,
	server *siteServer,
	route, selector string,
	deadline time.Duration,
	runsJS bool,
) (PageObservation, ledgerEntry, error) {
	navigation, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	observation, err := b.driver.Navigate(navigation, server.URL(route), selector)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return observation, ledgerEntry{}, &stepError{
				code:   CodeNavigationTimeout,
				detail: fmt.Sprintf("route %s did not stabilize within %s", route, deadline),
			}
		case errors.Is(err, ErrDriverAbsent):
			return observation, ledgerEntry{}, &stepError{code: CodeDriverAbsent, detail: err.Error()}
		default:
			return observation, ledgerEntry{}, &stepError{code: CodeDriverFault, detail: err.Error()}
		}
	}

	if markupPattern.MatchString(observation.Text) {
		return observation, ledgerEntry{}, &stepError{
			code:   CodeDriverFault,
			detail: fmt.Sprintf("driver returned markup instead of text for route %s", route),
		}
	}

	served, ok := server.lastServed(route)
	switch {
	case !ok:
		return observation, served, &stepError{
			code:   CodeObservationUnsupported,
			detail: fmt.Sprintf("the local server never served route %s", route),
		}
	case served.Status != observation.Status:
		return observation, served, &stepError{
			code: CodeObservationUnsupported,
			detail: fmt.Sprintf("route %s: the driver reported %d, the server served %d",
				route, observation.Status, served.Status),
		}
	case served.Status == http.StatusOK && observation.Bytes > 0 && observation.Bytes != served.Bytes:
		return observation, served, &stepError{
			code: CodeObservationUnsupported,
			detail: fmt.Sprintf("route %s: the driver reported %d bytes, the server served %d",
				route, observation.Bytes, served.Bytes),
		}
	}

	if step := corroborateContent(observation, served, selector, runsJS); step != nil {
		return observation, served, step
	}

	return observation, served, nil
}

// corroborateContent checks the claims a status code and a byte count cannot
// reach — the selector, the text on screen and the links that make a page
// reachable — against the harness's own copy of the bytes it served. Matching
// bytes are not evidence that the driver read them: without this check a driver
// that fetches honestly and then invents its observation produces proof.
func corroborateContent(observation PageObservation, served ledgerEntry, selector string, runsJS bool) *stepError {
	if served.Status != http.StatusOK {
		return nil
	}
	if served.Body == nil {
		return &stepError{
			code:   CodeObservationUnsupported,
			detail: fmt.Sprintf("the harness kept no copy of route %s to check the observation against", served.Route),
		}
	}
	if served.NeedsScript {
		if runsJS {
			// A scripted page has no static ground truth, so the harness says it
			// cannot judge instead of taking the driver's word for the DOM.
			return &stepError{
				code: CodeObservationUnsupported,
				detail: fmt.Sprintf("route %s reaches its rendered state through script, which the harness cannot corroborate",
					served.Route),
			}
		}
		// A driver that runs no script is reported as such by Run instead.
		return nil
	}

	tokens := tokenizeHTML(string(served.Body))

	if observation.BodyTextLength > 0 && visibleText(tokens) == "" {
		return &stepError{
			code:   CodeObservationUnsupported,
			detail: fmt.Sprintf("route %s: the driver reported rendered text, the served page renders none", served.Route),
		}
	}

	servedLinks := map[string]bool{}
	for _, href := range extractLinks(tokens) {
		servedLinks[href] = true
	}
	for _, href := range observation.Links {
		if !servedLinks[href] {
			return &stepError{
				code:   CodeObservationUnsupported,
				detail: fmt.Sprintf("route %s: the driver reported a link the served page does not carry", served.Route),
			}
		}
	}

	if selector == "" {
		return nil
	}

	spec, err := parseSelector(selector)
	if err != nil {
		return &stepError{code: CodeRequestInvalid, detail: err.Error()}
	}

	text, found := selectorText(tokens, spec)
	if observation.SelectorFound != found {
		return &stepError{
			code: CodeObservationUnsupported,
			detail: fmt.Sprintf("route %s: the driver reported selector_found=%v, the served page says %v",
				served.Route, observation.SelectorFound, found),
		}
	}
	// The comparison uses the untruncated normalization on purpose: sanitizeText
	// bounds what reaches the evidence, and comparing a page against its own
	// elided copy would report every long page as uncorroborated.
	if found && !strings.Contains(collapseSpaces(text), normalizeText(observation.Text)) {
		return &stepError{
			code: CodeObservationUnsupported,
			detail: fmt.Sprintf("route %s: the driver reported text the served page does not carry at %s",
				served.Route, selector),
		}
	}

	return nil
}

func normalizeRequest(req RunRequest) (RunRequest, error) {
	if strings.TrimSpace(req.SiteDir) == "" {
		return req, errors.New("no artifact directory to serve")
	}
	if len(req.Assertions) == 0 {
		return req, errors.New("no route assertion was declared")
	}
	if len(req.Assertions) > MaxRoutes {
		return req, fmt.Errorf("%d routes declared, the harness accepts at most %d", len(req.Assertions), MaxRoutes)
	}
	if req.NavigationDeadline <= 0 {
		return req, errors.New("the navigation deadline must be positive")
	}
	if req.NavigationDeadline > MaxNavigationDeadline {
		return req, fmt.Errorf("navigation deadline %s exceeds the %s ceiling", req.NavigationDeadline, MaxNavigationDeadline)
	}
	if req.DriverLock.Kind == "" || req.DriverLock.Digest == "" {
		return req, errors.New("the driver lock must pin a kind and a digest")
	}

	assertions := make([]RouteAssertion, 0, len(req.Assertions))
	for i, assertion := range req.Assertions {
		if strings.TrimSpace(assertion.Selector) == "" {
			return req, fmt.Errorf("assertion %d declares no selector", i)
		}
		if strings.TrimSpace(assertion.ExpectedText) == "" {
			return req, fmt.Errorf("assertion %d declares no expected text", i)
		}
		assertion.Route = normalizeRoute(assertion.Route)
		assertions = append(assertions, assertion)
	}

	req.EntryRoute = normalizeRoute(req.EntryRoute)
	req.Assertions = assertions

	return req, nil
}

func finalizeStep(result BrowserProofResultV1, err error) (BrowserProofResultV1, error) {
	var step *stepError
	if errors.As(err, &step) {
		return finalize(result, step.code, step.detail)
	}

	return finalize(result, CodeDriverFault, err.Error())
}

// finalize is the only exit of Run: it sets the verdict, validates it against
// the published contract and downgrades anything it cannot substantiate.
func finalize(result BrowserProofResultV1, code, detail string) (BrowserProofResultV1, error) {
	result.Code = code
	result.Detail = sanitizeDetail(detail)
	result.Outcome = outcomeOfCode(code)
	result.Proved = result.Outcome == OutcomeProved

	if err := result.Validate(); err != nil {
		result.Proved = false
		result.Outcome = OutcomeInconclusive
		result.Code = CodeVerdictRejected
		result.Detail = sanitizeDetail(err.Error())
	}

	if result.Proved {
		return result, nil
	}

	return result, &ProofError{
		Card:    result.Card,
		Code:    result.Code,
		Outcome: result.Outcome,
		Detail:  result.Detail,
	}
}

// normalizeText strips control characters and collapses whitespace without
// bounding the result. It is what two texts are compared through; what reaches
// a verdict goes through sanitizeText instead.
func normalizeText(text string) string {
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)

	return strings.Join(strings.Fields(cleaned), " ")
}

func sanitizeText(text string) string {
	cleaned := normalizeText(text)

	runes := []rune(cleaned)
	if len(runes) > maxObservedTextRunes {
		return string(runes[:maxObservedTextRunes]) + "…"
	}

	return cleaned
}

func sanitizeDetail(detail string) string {
	cleaned := sanitizeText(detail)

	runes := []rune(cleaned)
	if len(runes) > maxDetailRunes {
		return string(runes[:maxDetailRunes]) + "…"
	}

	return cleaned
}
