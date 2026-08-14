// AUR-429 unit selector: the smallest observable seam of "a pagina publicada
// abre e navega" — browserproof.VerifyDocs driven over hand-built published
// trees through the offline scripted driver.
//
// What this lane proves, per AC-001:
//   - a site whose home page opens, whose index links to a page, and whose
//     linked page shows the expected content is PROVED, and the verdict
//     carries the followed link, the followed route and a full corroborated
//     browser proof;
//   - a linked page WITHOUT the expected content is REFUSED with
//     BROWSERPROOF_TEXT_MISMATCH — the exact behavior MUT-001 deletes; if it
//     ever comes back proved this lane prints AUR-429/AC-001/MUT-001 and
//     fails;
//   - an index that links to nothing is REFUSED as unreachable, a missing
//     home page is REFUSED as absent, and a driver that forges observations
//     can never produce proof;
//   - the verdict survives its own contract: Validate accepts the real run
//     and rejects a doctored "proved" verdict, and the JSON round-trips
//     through ParseDocsVerifyResultV1.
//
// This is a plain (non-_test) source in package unit; the acceptance stages
// it into a private module next to a generated bridge _test file, mirroring
// AUR-424/AUR-425/AUR-428.
package unit

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
	aur429IndexText   = "Generated API documentation"
	aur429ContentText = "func NewGreeting"
	aur429URL         = "https://usuario.github.io/projeto"
)

// TestAUR429 is AC-001 at the exported-API seam.
func TestAUR429(t *testing.T) {
	t.Run("published site that opens and navigates is proved", func(t *testing.T) {
		site := aur429Site(t, aur429IndexHTML(`<li><a href="/guia/">Guia</a></li>`), map[string]string{
			"guia/index.html": aur429PageHTML("Guia", "O guia documenta func NewGreeting em detalhe."),
		})

		result, err := aur429Verify(t, browserproof.NewScriptedDriver(), site)
		if err != nil {
			t.Fatalf("AUR-429/AC-001/behavior-missing: navigable site was not proved: %v\n%+v", err, result)
		}
		if !result.Proved || result.Outcome != browserproof.OutcomeProved || result.Code != "" {
			t.Fatalf("AUR-429/AC-001/behavior-missing: verdict is not proof: %+v", result)
		}
		if result.FollowedLink != "/guia/" || result.FollowedRoute != "/guia" {
			t.Fatalf("AUR-429/AC-001/behavior-missing: the verification did not follow the index link: %+v", result)
		}
		if result.Proof == nil || !result.Proof.Proved || len(result.Proof.Routes) != 2 {
			t.Fatalf("AUR-429/AC-001/behavior-missing: proved verdict without a full browser proof: %+v", result)
		}
		if got := result.Proof.Routes[1]; got.Route != "/guia" ||
			!strings.Contains(got.ObservedText, aur429ContentText) {
			t.Fatalf("AUR-429/AC-001/behavior-missing: followed page does not show the expected content: %+v", got)
		}
		if validateErr := result.Validate(); validateErr != nil {
			t.Fatalf("AUR-429/AC-001/behavior-missing: proved verdict fails its own contract: %v", validateErr)
		}

		// AC-001's repeat clause: the same input yields the same verdict.
		again, err := aur429Verify(t, browserproof.NewScriptedDriver(), site)
		if err != nil || !again.Proved {
			t.Fatalf("AUR-429/AC-001/nondeterministic: second run over the same site: %v\n%+v", err, again)
		}
		if again.FollowedRoute != result.FollowedRoute ||
			again.Proof.ArtifactDigest != result.Proof.ArtifactDigest {
			t.Fatalf("AUR-429/AC-001/nondeterministic:\nfirst  %+v\nsecond %+v", result, again)
		}

		// The verdict round-trips through the published parser.
		raw, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			t.Fatalf("AUR-429/AC-001/infrastructure/marshal: %v", marshalErr)
		}
		parsed, parseErr := browserproof.ParseDocsVerifyResultV1(raw)
		if parseErr != nil || !parsed.Proved {
			t.Fatalf("AUR-429/AC-001/behavior-missing: verdict does not round-trip: %v", parseErr)
		}
	})

	t.Run("linked page without the expected content is refused", func(t *testing.T) {
		site := aur429Site(t, aur429IndexHTML(`<li><a href="/guia/">Guia</a></li>`), map[string]string{
			"guia/index.html": aur429PageHTML("Guia", "Uma pagina publicada sem o simbolo prometido."),
		})

		result, err := aur429Verify(t, browserproof.NewScriptedDriver(), site)
		if err == nil || result.Proved {
			t.Fatalf("AUR-429/AC-001/MUT-001\nAUR-429/AC-001/behavior-missing: "+
				"a page without the expected content was accepted: %+v", result)
		}
		if result.Outcome != browserproof.OutcomeRefused || result.Code != browserproof.CodeTextMismatch {
			// This probe is MUT-001's: a mutant that stops demanding the
			// content shows up here either as acceptance or as a garbled
			// verdict, and both register the marker.
			t.Fatalf("AUR-429/AC-001/MUT-001\nAUR-429/AC-001/wrong-refusal: got %s (%s): %s",
				result.Code, result.Outcome, result.Detail)
		}
	})

	t.Run("index that links to nothing is refused as unreachable", func(t *testing.T) {
		site := aur429Site(t, aur429IndexHTML(""), map[string]string{
			"guia/index.html": aur429PageHTML("Guia", "O guia documenta func NewGreeting em detalhe."),
		})

		result, err := aur429Verify(t, browserproof.NewScriptedDriver(), site)
		if err == nil || result.Proved {
			t.Fatalf("AUR-429/AC-001/MUT-001\nAUR-429/AC-001/behavior-missing: "+
				"an index with no link to follow was accepted: %+v", result)
		}
		if result.Outcome != browserproof.OutcomeRefused || result.Code != browserproof.CodeUnreachableRoute {
			t.Fatalf("AUR-429/AC-001/wrong-refusal: got %s (%s): %s", result.Code, result.Outcome, result.Detail)
		}
	})

	t.Run("missing home page is refused as absent", func(t *testing.T) {
		site := aur429Site(t, "", map[string]string{
			"guia/index.html": aur429PageHTML("Guia", "O guia documenta func NewGreeting em detalhe."),
		})
		if err := os.Remove(filepath.Join(site, "index.html")); err != nil {
			t.Fatalf("AUR-429/AC-001/infrastructure/fixture: %v", err)
		}

		result, err := aur429Verify(t, browserproof.NewScriptedDriver(), site)
		if err == nil || result.Proved {
			t.Fatalf("AUR-429/AC-001/behavior-missing: a site without a home page was accepted: %+v", result)
		}
		if result.Outcome != browserproof.OutcomeRefused || result.Code != browserproof.CodeTargetAbsent {
			t.Fatalf("AUR-429/AC-001/wrong-refusal: got %s (%s): %s", result.Code, result.Outcome, result.Detail)
		}
	})

	t.Run("forging driver can never produce proof", func(t *testing.T) {
		site := aur429Site(t, aur429IndexHTML(`<li><a href="/guia/">Guia</a></li>`), map[string]string{
			"guia/index.html": aur429PageHTML("Guia", "O guia documenta func NewGreeting em detalhe."),
		})

		for name, mode := range map[string]browserproof.ForgeMode{
			"claims success after navigating": browserproof.ForgeClaimSuccess,
			"claims success without fetching": browserproof.ForgeWithoutFetch,
		} {
			driver := browserproof.NewScriptedDriver()
			driver.Forge = mode
			driver.ForgedText = aur429ContentText

			result, err := aur429Verify(t, driver, site)
			if err == nil || result.Proved {
				t.Fatalf("AUR-429/AC-001/forgery: driver that %s produced proof: %+v", name, result)
			}
			if result.Outcome == browserproof.OutcomeProved {
				t.Fatalf("AUR-429/AC-001/forgery: driver that %s reached a proved outcome", name)
			}
		}
	})

	t.Run("contract rejects a doctored proved verdict", func(t *testing.T) {
		doctored := browserproof.DocsVerifyResultV1{
			Schema:          browserproof.SchemaDocsVerifyResultV1,
			PublishedURL:    aur429URL,
			EntryRoute:      "/",
			ExpectedContent: aur429ContentText,
			Outcome:         browserproof.OutcomeProved,
			Proved:          true,
		}
		if doctored.Validate() == nil {
			t.Fatal("AUR-429/AC-001/MUT-001\nAUR-429/AC-001/behavior-missing: " +
				"a proved verdict without a followed route and a browser proof passed Validate")
		}
	})
}

// ---------------------------------------------------------------------------
// helpers (deliberately self-contained: the acceptance stages and compiles
// package unit on its own)
// ---------------------------------------------------------------------------

func aur429Verify(
	t *testing.T,
	driver browserproof.Driver,
	site string,
) (browserproof.DocsVerifyResultV1, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	return browserproof.New(driver).VerifyDocs(ctx, browserproof.DocsVerifyRequest{
		Card:               "AUR-429",
		SiteDir:            site,
		PublishedURL:       aur429URL,
		IndexSelector:      "h2",
		IndexText:          aur429IndexText,
		ContentSelector:    "body",
		ContentText:        aur429ContentText,
		DriverLock:         browserproof.DriverLock{Kind: browserproof.ScriptedDriverKind, Digest: browserproof.ScriptedDriverDigest},
		NavigationDeadline: 10 * time.Second,
	})
}

// aur429Site writes a published tree: an index.html (when indexHTML is not
// empty a placeholder index is still written so a page-only fixture exists)
// plus the given pages.
func aur429Site(t *testing.T, indexHTML string, pages map[string]string) string {
	t.Helper()

	root := t.TempDir()
	index := indexHTML
	if index == "" {
		index = aur429IndexHTML(`<li><a href="/guia/">Guia</a></li>`)
	}
	aur429Write(t, filepath.Join(root, "index.html"), index)
	for rel, content := range pages {
		aur429Write(t, filepath.Join(root, filepath.FromSlash(rel)), content)
	}
	return root
}

func aur429Write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("AUR-429/AC-001/infrastructure/fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("AUR-429/AC-001/infrastructure/fixture: %v", err)
	}
}

func aur429IndexHTML(items string) string {
	return "<!DOCTYPE html>\n<html lang=\"pt\">\n<head><meta charset=\"utf-8\"><title>Docs</title></head>\n" +
		"<body>\n<h2>" + aur429IndexText + "</h2>\n<ul>\n" + items + "\n</ul>\n</body>\n</html>\n"
}

func aur429PageHTML(title, text string) string {
	return "<!DOCTYPE html>\n<html lang=\"pt\">\n<head><meta charset=\"utf-8\"><title>" + title + "</title></head>\n" +
		"<body>\n<h1>" + title + "</h1>\n<p>" + text + "</p>\n</body>\n</html>\n"
}
