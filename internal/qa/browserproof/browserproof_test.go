package browserproof_test

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
	symbolRoute    = "/symbols/extractorpipeline.html"
	symbolSelector = "#symbol-name"
	symbolText     = "ExtractorPipeline"
)

func fixtureDir(t *testing.T, variant string) string {
	t.Helper()

	dir := filepath.Join("..", "..", "..", "tests", "fixtures", "AUR-423", variant)
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("AUR-423 fixture variant %q unavailable: %v", variant, err)
	}

	return dir
}

func scriptedRequest(siteDir string) browserproof.RunRequest {
	return browserproof.RunRequest{
		Card:       "AUR-423",
		SiteDir:    siteDir,
		EntryRoute: "/index.html",
		Assertions: []browserproof.RouteAssertion{{
			Route:        symbolRoute,
			Selector:     symbolSelector,
			ExpectedText: symbolText,
		}},
		DriverLock: browserproof.DriverLock{
			Kind:   browserproof.ScriptedDriverKind,
			Digest: browserproof.ScriptedDriverDigest,
		},
		NavigationDeadline: 10 * time.Second,
	}
}

// TestAUR423AC001Variants is AC-001: the three built-site variants must reach
// three different verdicts, and only the nominal one may be proof of function.
func TestAUR423AC001Variants(t *testing.T) {
	cases := []struct {
		variant string
		outcome browserproof.Outcome
		code    string
		proved  bool
	}{
		{"nominal", browserproof.OutcomeProved, "", true},
		{"missing-symbol-page", browserproof.OutcomeRefused, browserproof.CodeTargetAbsent, false},
		{"unlinked-symbol-page", browserproof.OutcomeRefused, browserproof.CodeUnreachableRoute, false},
	}

	seen := map[string]string{}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.variant, func(t *testing.T) {
			proof := browserproof.New(browserproof.NewScriptedDriver())

			result, err := proof.Run(context.Background(), scriptedRequest(fixtureDir(t, tc.variant)))
			if tc.proved && err != nil {
				t.Fatalf("AUR-423/AC-001/%s: want proved run, got error: %v", tc.variant, err)
			}
			if !tc.proved && err == nil {
				t.Fatalf("AUR-423/AC-001/%s: refusal must surface as a non-nil error", tc.variant)
			}

			if result.Proved != tc.proved {
				t.Errorf("AUR-423/AC-001/%s: proved = %v, want %v", tc.variant, result.Proved, tc.proved)
			}
			if result.Outcome != tc.outcome {
				t.Errorf("AUR-423/AC-001/%s: outcome = %q, want %q", tc.variant, result.Outcome, tc.outcome)
			}
			if result.Code != tc.code {
				t.Errorf("AUR-423/AC-001/%s: code = %q, want %q", tc.variant, result.Code, tc.code)
			}

			if len(result.Routes) != 1 {
				t.Fatalf("AUR-423/AC-001/%s: want 1 route observation, got %d", tc.variant, len(result.Routes))
			}
			observed := result.Routes[0]
			if observed.Route != symbolRoute || observed.Selector != symbolSelector {
				t.Errorf("AUR-423/AC-001/%s: observation lost route/selector: %+v", tc.variant, observed)
			}

			if tc.proved {
				if observed.Assertion != browserproof.AssertionPass {
					t.Errorf("AUR-423/AC-001/%s: assertion = %q, want pass", tc.variant, observed.Assertion)
				}
				if !strings.Contains(observed.ObservedText, symbolText) {
					t.Errorf("AUR-423/AC-001/%s: extracted text %q lacks %q", tc.variant, observed.ObservedText, symbolText)
				}
				if result.ArtifactDigest == "" || result.DriverDigest == "" {
					t.Errorf("AUR-423/AC-001/%s: result must record artifact and driver digests: %+v", tc.variant, result)
				}
			} else if observed.Assertion == browserproof.AssertionPass {
				t.Errorf("AUR-423/AC-001/%s: refused run must not record a passing assertion", tc.variant)
			}

			key := string(result.Outcome) + "/" + result.Code
			if other, clash := seen[key]; clash {
				t.Errorf("AUR-423/AC-001: variants %q and %q share verdict %q", other, tc.variant, key)
			}
			seen[key] = tc.variant
		})
	}
}

// TestAUR423EmptyRenderIsNeverProved covers the fourth case of AC-001: a route
// that answers 200 with an empty rendered body is refused, not proved.
func TestAUR423EmptyRenderIsNeverProved(t *testing.T) {
	proof := browserproof.New(browserproof.NewScriptedDriver())

	result, err := proof.Run(context.Background(), scriptedRequest(fixtureDir(t, "empty-render")))
	if err == nil {
		t.Fatal("AUR-423/AC-001: empty render must surface as a non-nil error")
	}
	if result.Proved {
		t.Fatalf("AUR-423/AC-001: empty render reported as proof: %+v", result)
	}
	if result.Code != browserproof.CodeEmptyRender {
		t.Fatalf("AUR-423/AC-001: code = %q, want %q", result.Code, browserproof.CodeEmptyRender)
	}
	if result.Routes[0].Status != 200 {
		t.Fatalf("AUR-423/AC-001: empty render must record the 200 it observed, got %d", result.Routes[0].Status)
	}
}

// TestAUR423ReplayIsDeterministic pins the replay guarantee of AC-001: the same
// request over the same artifact keeps outcome, route order and artifact digest.
func TestAUR423ReplayIsDeterministic(t *testing.T) {
	dir := fixtureDir(t, "nominal")
	req := scriptedRequest(dir)
	req.Assertions = append(req.Assertions, browserproof.RouteAssertion{
		Route:        "/symbols/index.html",
		Selector:     "#page-title",
		ExpectedText: "Symbols",
	})

	first, err := browserproof.New(browserproof.NewScriptedDriver()).Run(context.Background(), req)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	second, err := browserproof.New(browserproof.NewScriptedDriver()).Run(context.Background(), req)
	if err != nil {
		t.Fatalf("replay run: %v", err)
	}

	if first.ArtifactDigest != second.ArtifactDigest {
		t.Errorf("artifact digest drifted: %q vs %q", first.ArtifactDigest, second.ArtifactDigest)
	}
	if len(first.Routes) != len(req.Assertions) {
		t.Fatalf("want %d route observations, got %d", len(req.Assertions), len(first.Routes))
	}
	for i := range first.Routes {
		if first.Routes[i].Route != req.Assertions[i].Route {
			t.Errorf("route order drifted at %d: %q", i, first.Routes[i].Route)
		}
		if first.Routes[i].Route != second.Routes[i].Route {
			t.Errorf("replay reordered routes at %d", i)
		}
	}
}

// TestAUR423EvidenceCarriesNoRawHTML keeps screenshots and raw markup out of the
// serialized verdict while requiring the observation fields the card demands.
func TestAUR423EvidenceCarriesNoRawHTML(t *testing.T) {
	result, err := browserproof.New(browserproof.NewScriptedDriver()).
		Run(context.Background(), scriptedRequest(fixtureDir(t, "nominal")))
	if err != nil {
		t.Fatalf("nominal run: %v", err)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal verdict: %v", err)
	}
	text := string(encoded)
	t.Logf("verdict: %s", text)

	for _, forbidden := range []string{"<html", "<h1", "<body", "<nav", "screenshot", "\\u003c"} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Errorf("verdict leaked %q: %s", forbidden, text)
		}
	}
	for _, required := range []string{"artifact_digest", "driver_digest", "selector", "observed_text", "assertion"} {
		if !strings.Contains(text, required) {
			t.Errorf("verdict is missing %q: %s", required, text)
		}
	}
}

// TestAUR423RequestLimits pins the ceilings AC-001 declares: at most 8 routes and
// a navigation deadline no longer than 30s, both refused as harness diagnosis.
func TestAUR423RequestLimits(t *testing.T) {
	base := scriptedRequest(fixtureDir(t, "nominal"))

	tooManyRoutes := base
	tooManyRoutes.Assertions = nil
	for i := 0; i < browserproof.MaxRoutes+1; i++ {
		tooManyRoutes.Assertions = append(tooManyRoutes.Assertions, browserproof.RouteAssertion{
			Route: symbolRoute, Selector: symbolSelector, ExpectedText: symbolText,
		})
	}

	tooLongDeadline := base
	tooLongDeadline.NavigationDeadline = browserproof.MaxNavigationDeadline + time.Second

	for name, req := range map[string]browserproof.RunRequest{
		"routes":   tooManyRoutes,
		"deadline": tooLongDeadline,
	} {
		result, err := browserproof.New(browserproof.NewScriptedDriver()).Run(context.Background(), req)
		if err == nil || result.Proved {
			t.Errorf("%s: over-limit request must be refused, got %+v", name, result)
		}
		if result.Outcome != browserproof.OutcomeInconclusive || result.Code != browserproof.CodeRequestInvalid {
			t.Errorf("%s: outcome/code = %q/%q, want inconclusive/%s", name, result.Outcome, result.Code, browserproof.CodeRequestInvalid)
		}
	}
}
