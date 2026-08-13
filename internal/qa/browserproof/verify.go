package browserproof

// This file is AUR-429's evolution of the package: the end-to-end docs
// verification. Run (browserproof.go) judges routes a caller already knows;
// VerifyDocs delivers what a user of `aurumcode docs verify` was promised —
// after publishing, open the home page, follow a link of the index and
// confirm the expected content is there, instead of trusting that the file
// was uploaded. It discovers which index link carries the content and then
// hands the navigation to Run, so every claim in the final verdict is backed
// by the same server-ledger corroboration the rest of this package enforces:
// discovery can pick a route, but only a fully corroborated Run can prove it.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SchemaDocsVerifyResultV1 identifies the docs-verification verdict contract.
const SchemaDocsVerifyResultV1 = "DocsVerifyResultV1"

// DocsVerifyRequest is one verification of one published documentation site.
type DocsVerifyRequest struct {
	// Card names the card or run this verdict is evidence for.
	Card string
	// SiteDir is the published HTML tree, served on loopback for the run.
	SiteDir string
	// PublishedURL is the public location the site is (or will be) published
	// under. It is recorded in the verdict; the navigation itself serves
	// SiteDir on loopback, because a proof run reaches no external network.
	PublishedURL string
	// IndexSelector and IndexText are what the home page must show.
	IndexSelector string
	IndexText     string
	// ContentSelector and ContentText are what a page reached through a link
	// of the index must show.
	ContentSelector    string
	ContentText        string
	DriverLock         DriverLock
	NavigationDeadline time.Duration
}

// DocsVerifyResultV1 is the published verdict of a docs verification. A
// proved verdict always embeds the full BrowserProofResultV1 that backs it.
type DocsVerifyResultV1 struct {
	Schema          string                `json:"schema"`
	Card            string                `json:"card,omitempty"`
	PublishedURL    string                `json:"published_url"`
	EntryRoute      string                `json:"entry_route"`
	FollowedLink    string                `json:"followed_link,omitempty"`
	FollowedRoute   string                `json:"followed_route,omitempty"`
	ExpectedContent string                `json:"expected_content"`
	Outcome         Outcome               `json:"outcome"`
	Proved          bool                  `json:"proved"`
	Code            string                `json:"code,omitempty"`
	Detail          string                `json:"detail,omitempty"`
	Proof           *BrowserProofResultV1 `json:"proof,omitempty"`
}

// Validate rejects any docs verdict that does not carry the evidence it
// claims. A caller must run it before trusting Proved.
func (r DocsVerifyResultV1) Validate() error {
	if r.Schema != SchemaDocsVerifyResultV1 {
		return fmt.Errorf("browserproof: schema %q is not %q", r.Schema, SchemaDocsVerifyResultV1)
	}
	switch r.Outcome {
	case OutcomeProved, OutcomeRefused, OutcomeInconclusive:
	default:
		return fmt.Errorf("browserproof: unknown outcome %q", r.Outcome)
	}
	if r.Proved != (r.Outcome == OutcomeProved) {
		return fmt.Errorf("browserproof: proved=%v contradicts outcome %q", r.Proved, r.Outcome)
	}
	if markupPattern.MatchString(r.Detail) || markupPattern.MatchString(r.FollowedLink) {
		return fmt.Errorf("%w: docs verdict", ErrEvidenceNotSanitized)
	}

	if !r.Proved {
		if r.Code == "" {
			return errors.New("browserproof: a docs verdict that is not proved must carry a refusal code")
		}
		return nil
	}

	if r.Code != "" {
		return fmt.Errorf("browserproof: proved docs verdict carries refusal code %q", r.Code)
	}
	if err := validatePublishedURL(r.PublishedURL); err != nil {
		return fmt.Errorf("browserproof: proved docs verdict: %w", err)
	}
	if r.EntryRoute == "" || r.FollowedLink == "" || r.FollowedRoute == "" {
		return errors.New("browserproof: proved docs verdict must record the link it followed")
	}
	if r.FollowedRoute == r.EntryRoute {
		return errors.New("browserproof: proved docs verdict never left the home page")
	}
	if strings.TrimSpace(r.ExpectedContent) == "" {
		return errors.New("browserproof: proved docs verdict names no expected content")
	}
	if r.Proof == nil {
		return errors.New("browserproof: proved docs verdict without a browser proof")
	}
	if err := r.Proof.Validate(); err != nil {
		return fmt.Errorf("browserproof: embedded proof: %w", err)
	}
	if !r.Proof.Proved {
		return errors.New("browserproof: proved docs verdict over an unproved browser proof")
	}
	// The embedded proof must be about exactly this navigation: the home page
	// first, the followed page last, asserting the expected content.
	if len(r.Proof.Routes) < 2 || r.Proof.Routes[0].Route != r.EntryRoute {
		return errors.New("browserproof: embedded proof does not open the home page")
	}
	followed := r.Proof.Routes[len(r.Proof.Routes)-1]
	if followed.Route != r.FollowedRoute || followed.ExpectedText != r.ExpectedContent {
		return errors.New("browserproof: embedded proof is not about the followed link and its expected content")
	}

	return nil
}

// ParseDocsVerifyResultV1 decodes a docs verdict and refuses anything it
// cannot substantiate, including unknown fields.
func ParseDocsVerifyResultV1(data []byte) (DocsVerifyResultV1, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var result DocsVerifyResultV1
	if err := decoder.Decode(&result); err != nil {
		return DocsVerifyResultV1{}, fmt.Errorf("browserproof: decode docs verdict: %w", err)
	}
	if err := result.Validate(); err != nil {
		return DocsVerifyResultV1{}, err
	}

	return result, nil
}

// VerifyDocs serves the published tree on loopback, opens the home page,
// follows a link of the index to a page that shows the expected content, and
// returns the corroborated verdict. Like Run, it returns a non-nil error for
// every outcome that is not proof.
func (b *BrowserProof) VerifyDocs(ctx context.Context, req DocsVerifyRequest) (DocsVerifyResultV1, error) {
	entry := normalizeRoute("/")
	result := DocsVerifyResultV1{
		Schema:          SchemaDocsVerifyResultV1,
		Card:            req.Card,
		PublishedURL:    strings.TrimSpace(req.PublishedURL),
		EntryRoute:      entry,
		ExpectedContent: req.ContentText,
	}

	if err := validateDocsRequest(req); err != nil {
		return finalizeDocs(result, CodeRequestInvalid, err.Error())
	}

	server, err := startSiteServer(req.SiteDir)
	if err != nil {
		return finalizeDocs(result, CodeServerUnavailable, err.Error())
	}
	defer server.Close()

	fingerprint, err := b.driver.Fingerprint(ctx)
	if err != nil {
		if errors.Is(err, ErrDriverAbsent) {
			return finalizeDocs(result, CodeDriverAbsent, err.Error())
		}
		return finalizeDocs(result, CodeDriverFault, err.Error())
	}
	if fingerprint.Kind != req.DriverLock.Kind || fingerprint.Digest != req.DriverLock.Digest {
		return finalizeDocs(result, CodeDriverMismatch, fmt.Sprintf(
			"driver %s %s is outside the lock %s %s",
			fingerprint.Kind, fingerprint.Digest, req.DriverLock.Kind, req.DriverLock.Digest))
	}

	// Open the home page. The observation is corroborated against the served
	// bytes by observe; the classification mirrors Run's, so a broken home
	// page is refused with the same code either way reports it.
	observation, served, err := b.observe(
		ctx, server, entry, req.IndexSelector, req.NavigationDeadline, fingerprint.ExecutesJavaScript)
	if err != nil {
		return finalizeDocsStep(result, err)
	}
	indexText := sanitizeText(observation.Text)
	switch {
	case served.Status == http.StatusNotFound || served.Status == http.StatusGone:
		return finalizeDocs(result, CodeTargetAbsent, "the home page answers "+fmt.Sprint(served.Status))
	case served.Status != http.StatusOK:
		return finalizeDocs(result, CodeServerUnavailable, fmt.Sprintf("the home page answers %d", served.Status))
	case (served.NeedsScript || observation.RequiresJavaScript) && !fingerprint.ExecutesJavaScript:
		return finalizeDocs(result, CodeJavaScriptUnsupported, "the home page needs a scripting engine")
	case served.Bytes == 0 || observation.BodyTextLength == 0:
		return finalizeDocs(result, CodeEmptyRender, "the home page renders no text")
	case !observation.SelectorFound:
		return finalizeDocs(result, CodeSelectorAbsent, "the home page has no "+req.IndexSelector)
	case !strings.Contains(indexText, req.IndexText):
		return finalizeDocs(result, CodeTextMismatch, fmt.Sprintf(
			"the home page shows %q, which does not contain %q", indexText, req.IndexText))
	}

	// Follow the index: the links come from the corroborated observation of
	// the home page, in the order the index carries them.
	type candidate struct{ href, route string }
	seen := map[string]bool{entry: true}
	candidates := make([]candidate, 0, len(observation.Links))
	for _, href := range observation.Links {
		route, ok := resolveLink(entry, href)
		if !ok || seen[route] {
			continue
		}
		seen[route] = true
		candidates = append(candidates, candidate{href: href, route: route})
	}
	if len(candidates) == 0 {
		return finalizeDocs(result, CodeUnreachableRoute, "the index carries no link to follow")
	}

	// Discovery picks the first linked page that shows the expected content,
	// using exactly the comparison Run will re-apply, over at most the crawl
	// budget. Discovery never decides the verdict: the chosen route is handed
	// to Run below and only its corroborated proof counts.
	chosen := candidate{}
	for i, cand := range candidates {
		if i >= maxCrawlPages {
			break
		}
		linked, linkedServed, err := b.observe(
			ctx, server, cand.route, req.ContentSelector, req.NavigationDeadline, fingerprint.ExecutesJavaScript)
		if err != nil {
			return finalizeDocsStep(result, err)
		}
		if linkedServed.Status != http.StatusOK || !linked.SelectorFound {
			continue
		}
		if strings.Contains(sanitizeText(linked.Text), req.ContentText) {
			chosen = cand
			break
		}
	}
	if chosen.route == "" {
		return finalizeDocs(result, CodeTextMismatch, fmt.Sprintf(
			"none of the %d links the index carries shows %q", len(candidates), req.ContentText))
	}

	result.FollowedLink = chosen.href
	result.FollowedRoute = chosen.route

	proof, _ := b.Run(ctx, RunRequest{
		Card:       req.Card,
		SiteDir:    req.SiteDir,
		EntryRoute: entry,
		Assertions: []RouteAssertion{
			{Route: entry, Selector: req.IndexSelector, ExpectedText: req.IndexText},
			{Route: chosen.route, Selector: req.ContentSelector, ExpectedText: req.ContentText},
		},
		DriverLock:         req.DriverLock,
		NavigationDeadline: req.NavigationDeadline,
	})
	result.Proof = &proof

	return finalizeDocs(result, proof.Code, proof.Detail)
}

func validateDocsRequest(req DocsVerifyRequest) error {
	if strings.TrimSpace(req.SiteDir) == "" {
		return errors.New("no published site directory to serve")
	}
	if err := validatePublishedURL(req.PublishedURL); err != nil {
		return err
	}
	if strings.TrimSpace(req.IndexText) == "" {
		return errors.New("no expected index text was declared")
	}
	if strings.TrimSpace(req.ContentText) == "" {
		return errors.New("no expected content was declared")
	}
	for _, selector := range []string{req.IndexSelector, req.ContentSelector} {
		if _, err := parseSelector(selector); err != nil {
			return err
		}
	}
	if req.NavigationDeadline <= 0 {
		return errors.New("the navigation deadline must be positive")
	}
	if req.NavigationDeadline > MaxNavigationDeadline {
		return fmt.Errorf("navigation deadline %s exceeds the %s ceiling", req.NavigationDeadline, MaxNavigationDeadline)
	}
	if req.DriverLock.Kind == "" || req.DriverLock.Digest == "" {
		return errors.New("the driver lock must pin a kind and a digest")
	}
	return nil
}

func validatePublishedURL(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return errors.New("no published URL was declared")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return fmt.Errorf("published URL %q does not parse: %w", trimmed, err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("published URL %q is not an http(s) location", trimmed)
	}
	return nil
}

func finalizeDocsStep(result DocsVerifyResultV1, err error) (DocsVerifyResultV1, error) {
	var step *stepError
	if errors.As(err, &step) {
		return finalizeDocs(result, step.code, step.detail)
	}

	return finalizeDocs(result, CodeDriverFault, err.Error())
}

// finalizeDocs is the only exit of VerifyDocs: it sets the verdict, validates
// it against the published contract and downgrades anything it cannot
// substantiate, exactly as finalize does for Run.
func finalizeDocs(result DocsVerifyResultV1, code, detail string) (DocsVerifyResultV1, error) {
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
