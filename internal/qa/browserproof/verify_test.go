package browserproof_test

// AUR-429: VerifyDocs is the end-to-end docs verification — open the home
// page, follow a link of the index, confirm the expected content. These tests
// pin the outcomes the card promises: proof only for a site that actually
// opens and navigates, refusals that name what broke, no proof from a forged
// observation, and a verdict contract that rejects what it cannot back.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/qa/browserproof"
)

const (
	verifyIndexText = "Generated API documentation"
	verifyContent   = "func NewGreeting"
	verifyURL       = "https://usuario.github.io/projeto"
)

func verifyRequest(site string) browserproof.DocsVerifyRequest {
	return browserproof.DocsVerifyRequest{
		Card:            "AUR-429",
		SiteDir:         site,
		PublishedURL:    verifyURL,
		IndexSelector:   "h2",
		IndexText:       verifyIndexText,
		ContentSelector: "body",
		ContentText:     verifyContent,
		DriverLock: browserproof.DriverLock{
			Kind:   browserproof.ScriptedDriverKind,
			Digest: browserproof.ScriptedDriverDigest,
		},
		NavigationDeadline: 10 * time.Second,
	}
}

func runVerify(t *testing.T, driver browserproof.Driver, site string) (browserproof.DocsVerifyResultV1, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	return browserproof.New(driver).VerifyDocs(ctx, verifyRequest(site))
}

// verifySite writes a published tree with an index and the given pages.
func verifySite(t *testing.T, indexItems string, pages map[string]string) string {
	t.Helper()

	root := t.TempDir()
	index := "<!DOCTYPE html>\n<html lang=\"pt\">\n<head><meta charset=\"utf-8\"><title>Docs</title></head>\n" +
		"<body>\n<h2>" + verifyIndexText + "</h2>\n<ul>\n" + indexItems + "\n</ul>\n</body>\n</html>\n"
	verifyWrite(t, filepath.Join(root, "index.html"), index)
	for rel, text := range pages {
		page := "<!DOCTYPE html>\n<html lang=\"pt\">\n<head><meta charset=\"utf-8\"><title>Pagina</title></head>\n" +
			"<body>\n<h1>Pagina</h1>\n<p>" + text + "</p>\n</body>\n</html>\n"
		verifyWrite(t, filepath.Join(root, filepath.FromSlash(rel)), page)
	}
	return root
}

func verifyWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func TestVerifyDocsProvesASiteThatOpensAndNavigates(t *testing.T) {
	site := verifySite(t, `<li><a href="/guia/">Guia</a></li>`, map[string]string{
		"guia/index.html": "O guia documenta func NewGreeting em detalhe.",
	})

	result, err := runVerify(t, browserproof.NewScriptedDriver(), site)
	if err != nil {
		t.Fatalf("navigable site was not proved: %v\n%+v", err, result)
	}
	if !result.Proved || result.Outcome != browserproof.OutcomeProved || result.Code != "" {
		t.Fatalf("verdict is not proof: %+v", result)
	}
	if result.EntryRoute != "/" || result.FollowedLink != "/guia/" || result.FollowedRoute != "/guia" {
		t.Fatalf("the verification did not record the link it followed: %+v", result)
	}
	if result.Proof == nil || !result.Proof.Proved {
		t.Fatalf("proved verdict without a backing browser proof: %+v", result)
	}
	if len(result.Proof.Routes) != 2 || result.Proof.Routes[0].Route != "/" || result.Proof.Routes[1].Route != "/guia" {
		t.Fatalf("embedded proof is not about the navigation: %+v", result.Proof.Routes)
	}
	if !strings.Contains(result.Proof.Routes[1].ObservedText, verifyContent) {
		t.Fatalf("followed page does not show the expected content: %+v", result.Proof.Routes[1])
	}
	if validateErr := result.Validate(); validateErr != nil {
		t.Fatalf("proved verdict fails its own contract: %v", validateErr)
	}

	raw, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		t.Fatalf("marshal verdict: %v", marshalErr)
	}
	parsed, parseErr := browserproof.ParseDocsVerifyResultV1(raw)
	if parseErr != nil || !parsed.Proved {
		t.Fatalf("verdict does not round-trip through its parser: %v", parseErr)
	}
}

func TestVerifyDocsFollowsPastLinksWithoutTheContent(t *testing.T) {
	// The first two index links do not carry the content; the third does. The
	// verification must keep following until the promise is met, and must
	// report the link that met it.
	site := verifySite(t,
		`<li><a href="/um/">Um</a></li><li><a href="/dois/">Dois</a></li><li><a href="/tres/">Tres</a></li>`,
		map[string]string{
			"um/index.html":   "Nada aqui.",
			"dois/index.html": "Nada aqui tambem.",
			"tres/index.html": "A pagina certa documenta func NewGreeting.",
		})

	result, err := runVerify(t, browserproof.NewScriptedDriver(), site)
	if err != nil || !result.Proved {
		t.Fatalf("site with the content behind the third link was not proved: %v\n%+v", err, result)
	}
	if result.FollowedRoute != "/tres" {
		t.Fatalf("expected /tres followed, got %+v", result)
	}
}

func TestVerifyDocsRefusalsNameWhatBroke(t *testing.T) {
	cases := []struct {
		name string
		site func(t *testing.T) string
		code string
	}{
		{
			name: "linked page without the expected content",
			site: func(t *testing.T) string {
				return verifySite(t, `<li><a href="/guia/">Guia</a></li>`, map[string]string{
					"guia/index.html": "Uma pagina sem o simbolo prometido.",
				})
			},
			code: browserproof.CodeTextMismatch,
		},
		{
			name: "index without any link to follow",
			site: func(t *testing.T) string {
				return verifySite(t, "", map[string]string{
					"guia/index.html": "O guia documenta func NewGreeting em detalhe.",
				})
			},
			code: browserproof.CodeUnreachableRoute,
		},
		{
			name: "home page absent",
			site: func(t *testing.T) string {
				site := verifySite(t, `<li><a href="/guia/">Guia</a></li>`, map[string]string{
					"guia/index.html": "O guia documenta func NewGreeting em detalhe.",
				})
				if err := os.Remove(filepath.Join(site, "index.html")); err != nil {
					t.Fatalf("remove index: %v", err)
				}
				return site
			},
			code: browserproof.CodeTargetAbsent,
		},
		{
			name: "home page without the index heading",
			site: func(t *testing.T) string {
				site := verifySite(t, `<li><a href="/guia/">Guia</a></li>`, map[string]string{
					"guia/index.html": "O guia documenta func NewGreeting em detalhe.",
				})
				verifyWrite(t, filepath.Join(site, "index.html"),
					"<!DOCTYPE html>\n<html><head><title>Docs</title></head><body>\n<h2>Outra coisa</h2>\n"+
						"<ul><li><a href=\"/guia/\">Guia</a></li></ul>\n</body></html>\n")
				return site
			},
			code: browserproof.CodeTextMismatch,
		},
	}

	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			result, err := runVerify(t, browserproof.NewScriptedDriver(), testCase.site(t))
			if err == nil || result.Proved {
				t.Fatalf("%s was accepted: %+v", testCase.name, result)
			}
			if result.Outcome != browserproof.OutcomeRefused {
				t.Fatalf("%s must be refused, got %s: %s", testCase.name, result.Outcome, result.Detail)
			}
			if result.Code != testCase.code {
				t.Fatalf("expected %s, got %s: %s", testCase.code, result.Code, result.Detail)
			}
			if validateErr := result.Validate(); validateErr != nil {
				t.Fatalf("refusal fails its own contract: %v", validateErr)
			}
		})
	}
}

func TestVerifyDocsForgedObservationsNeverBecomeProof(t *testing.T) {
	site := verifySite(t, `<li><a href="/guia/">Guia</a></li>`, map[string]string{
		"guia/index.html": "O guia documenta func NewGreeting em detalhe.",
	})

	for name, mode := range map[string]browserproof.ForgeMode{
		"claims success after navigating": browserproof.ForgeClaimSuccess,
		"claims success without fetching": browserproof.ForgeWithoutFetch,
	} {
		mode := mode
		t.Run(name, func(t *testing.T) {
			driver := browserproof.NewScriptedDriver()
			driver.Forge = mode
			driver.ForgedText = verifyContent

			result, err := runVerify(t, driver, site)
			if err == nil || result.Proved {
				t.Fatalf("a forging driver produced docs proof: %+v", result)
			}
			if result.Outcome != browserproof.OutcomeInconclusive {
				t.Fatalf("an uncorroborated observation is a harness diagnosis, got %s: %s",
					result.Outcome, result.Detail)
			}
		})
	}
}

func TestVerifyDocsDriverOutsideTheLockIsRefused(t *testing.T) {
	site := verifySite(t, `<li><a href="/guia/">Guia</a></li>`, map[string]string{
		"guia/index.html": "O guia documenta func NewGreeting em detalhe.",
	})

	request := verifyRequest(site)
	request.DriverLock = browserproof.DriverLock{Kind: browserproof.ExternalDriverKind, Digest: "sha256:0000"}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := browserproof.New(browserproof.NewScriptedDriver()).VerifyDocs(ctx, request)
	if err == nil || result.Proved {
		t.Fatalf("a driver outside the lock produced proof: %+v", result)
	}
	if result.Code != browserproof.CodeDriverMismatch || result.Outcome != browserproof.OutcomeInconclusive {
		t.Fatalf("expected %s, got %s (%s)", browserproof.CodeDriverMismatch, result.Code, result.Outcome)
	}
}

func TestVerifyDocsInvalidRequestsAreDiagnosedNotJudged(t *testing.T) {
	site := verifySite(t, `<li><a href="/guia/">Guia</a></li>`, map[string]string{
		"guia/index.html": "O guia documenta func NewGreeting em detalhe.",
	})

	break_ := func(mutate func(*browserproof.DocsVerifyRequest)) browserproof.DocsVerifyRequest {
		request := verifyRequest(site)
		mutate(&request)
		return request
	}

	cases := map[string]browserproof.DocsVerifyRequest{
		"no published URL":       break_(func(r *browserproof.DocsVerifyRequest) { r.PublishedURL = "" }),
		"published URL not http": break_(func(r *browserproof.DocsVerifyRequest) { r.PublishedURL = "ftp://x/y" }),
		"no expected content":    break_(func(r *browserproof.DocsVerifyRequest) { r.ContentText = " " }),
		"no index text":          break_(func(r *browserproof.DocsVerifyRequest) { r.IndexText = "" }),
		"unsupported selector":   break_(func(r *browserproof.DocsVerifyRequest) { r.ContentSelector = "div > p" }),
		"no deadline":            break_(func(r *browserproof.DocsVerifyRequest) { r.NavigationDeadline = 0 }),
		"no driver lock":         break_(func(r *browserproof.DocsVerifyRequest) { r.DriverLock = browserproof.DriverLock{} }),
	}

	for name, request := range cases {
		request := request
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			result, err := browserproof.New(browserproof.NewScriptedDriver()).VerifyDocs(ctx, request)
			if err == nil || result.Proved {
				t.Fatalf("invalid request produced a verdict: %+v", result)
			}
			if result.Code != browserproof.CodeRequestInvalid || result.Outcome != browserproof.OutcomeInconclusive {
				t.Fatalf("expected %s, got %s (%s): %s",
					browserproof.CodeRequestInvalid, result.Code, result.Outcome, result.Detail)
			}
		})
	}
}

func TestDocsVerifyContractRejectsWhatItCannotBack(t *testing.T) {
	proved := func(mutate func(*browserproof.DocsVerifyResultV1)) browserproof.DocsVerifyResultV1 {
		site := verifySite(t, `<li><a href="/guia/">Guia</a></li>`, map[string]string{
			"guia/index.html": "O guia documenta func NewGreeting em detalhe.",
		})
		result, err := runVerify(t, browserproof.NewScriptedDriver(), site)
		if err != nil {
			t.Fatalf("baseline proof failed: %v", err)
		}
		mutate(&result)
		return result
	}

	cases := map[string]browserproof.DocsVerifyResultV1{
		"proved without a proof":          proved(func(r *browserproof.DocsVerifyResultV1) { r.Proof = nil }),
		"proved without a followed route": proved(func(r *browserproof.DocsVerifyResultV1) { r.FollowedRoute = "" }),
		"proved that never left home":     proved(func(r *browserproof.DocsVerifyResultV1) { r.FollowedRoute = r.EntryRoute }),
		"proved over an unproved proof": proved(func(r *browserproof.DocsVerifyResultV1) {
			r.Proof.Proved = false
			r.Proof.Outcome = browserproof.OutcomeRefused
			r.Proof.Code = browserproof.CodeTextMismatch
		}),
		"proved about another route":   proved(func(r *browserproof.DocsVerifyResultV1) { r.FollowedRoute = "/outra" }),
		"proved contradicting outcome": proved(func(r *browserproof.DocsVerifyResultV1) { r.Outcome = browserproof.OutcomeRefused }),
		"refusal without a code": {
			Schema:  browserproof.SchemaDocsVerifyResultV1,
			Outcome: browserproof.OutcomeRefused,
		},
		"foreign schema": {Schema: "SomethingElseV9", Outcome: browserproof.OutcomeRefused, Code: "X"},
	}

	for name, verdict := range cases {
		verdict := verdict
		t.Run(name, func(t *testing.T) {
			if verdict.Validate() == nil {
				t.Fatalf("contract accepted a verdict it cannot back: %+v", verdict)
			}
		})
	}
}
