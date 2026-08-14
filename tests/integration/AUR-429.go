// AUR-429 integration selector: the whole chain the card promises, across the
// real package boundaries — cmd/regenerate-docs builds the documentation for
// tests/fixtures/docs/goproject, sitepublish renders the markdown as the HTML
// tree a static host serves, and browserproof.VerifyDocs opens the home page,
// follows a link of the index and confirms the expected content, offline,
// through the scripted driver.
//
// The negative scenarios break the GENERATED MARKDOWN, never the verifier:
//   - stripping the documented symbol from the symbol page must flip the
//     verdict to a refusal (BROWSERPROOF_TEXT_MISMATCH) — MUT-001's shape; a
//     run that stays proved prints AUR-429/AC-001/MUT-001 and fails;
//   - stripping the index links must flip it to BROWSERPROOF_UNREACHABLE_ROUTE.
//
// Both breaks also prove the publisher invents nothing: a publisher that
// synthesised an index link or page content would keep the verdict green.
//
// This is a plain (non-_test) source in package integration; the acceptance
// stages it into a private module next to a generated bridge _test file,
// mirroring AUR-424/AUR-425/AUR-428.
package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/qa/browserproof"
	"github.com/Mpaape/AurumCode/internal/qa/browserproof/sitepublish"
)

const (
	aur429iIndexText   = "Generated API documentation"
	aur429iContentText = "func NewGreeting"
	aur429iURL         = "https://usuario.github.io/projeto"
)

// IntegrationAUR429 is AC-001 across the generator/publisher/verifier chain.
func IntegrationAUR429(t *testing.T) {
	docsDir := aur429iGenerate(t)

	t.Run("generated site opens, navigates and shows the symbol", func(t *testing.T) {
		result, err := aur429iVerify(t, aur429iPublish(t, docsDir))
		if err != nil {
			t.Fatalf("AUR-429/AC-001/behavior-missing: generated site was not proved navigable: %v\n%+v", err, result)
		}
		if !result.Proved || result.FollowedRoute != "/go/root" {
			t.Fatalf("AUR-429/AC-001/behavior-missing: expected /go/root followed from the index, got %+v", result)
		}
		if result.Proof == nil || len(result.Proof.Routes) != 2 ||
			!strings.Contains(result.Proof.Routes[1].ObservedText, aur429iContentText) {
			t.Fatalf("AUR-429/AC-001/behavior-missing: proof does not show the documented symbol: %+v", result.Proof)
		}
		if validateErr := result.Validate(); validateErr != nil {
			t.Fatalf("AUR-429/AC-001/behavior-missing: verdict fails its own contract: %v", validateErr)
		}
	})

	t.Run("symbol stripped from the generated page is refused", func(t *testing.T) {
		broken := aur429iCopyDocs(t, docsDir)
		aur429iRewrite(t, filepath.Join(broken, "go", "root.md"), func(content string) string {
			if !strings.Contains(content, "NewGreeting") {
				t.Fatalf("AUR-429/AC-001/infrastructure/fixture: generated page no longer documents NewGreeting")
			}
			return strings.ReplaceAll(content, "NewGreeting", "Suprimido")
		})

		result, err := aur429iVerify(t, aur429iPublish(t, broken))
		if err == nil || result.Proved {
			t.Fatalf("AUR-429/AC-001/MUT-001\nAUR-429/AC-001/behavior-missing: "+
				"a published page without the expected content was accepted: %+v", result)
		}
		if result.Outcome != browserproof.OutcomeRefused || result.Code != browserproof.CodeTextMismatch {
			// This probe is MUT-001's: a mutant that stops demanding the
			// content shows up here either as acceptance or as a garbled
			// verdict, and both register the marker.
			t.Fatalf("AUR-429/AC-001/MUT-001\nAUR-429/AC-001/wrong-refusal: got %s (%s): %s",
				result.Code, result.Outcome, result.Detail)
		}
	})

	t.Run("index without its links is refused as unreachable", func(t *testing.T) {
		broken := aur429iCopyDocs(t, docsDir)
		aur429iRewrite(t, filepath.Join(broken, "index.md"), func(content string) string {
			var kept []string
			removed := 0
			for _, line := range strings.Split(content, "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "- [") {
					removed++
					continue
				}
				kept = append(kept, line)
			}
			if removed == 0 {
				t.Fatalf("AUR-429/AC-001/infrastructure/fixture: generated index carries no page link to remove:\n%s", content)
			}
			return strings.Join(kept, "\n")
		})

		result, err := aur429iVerify(t, aur429iPublish(t, broken))
		if err == nil || result.Proved {
			t.Fatalf("AUR-429/AC-001/MUT-001\nAUR-429/AC-001/behavior-missing: "+
				"an index that links to nothing was accepted: %+v", result)
		}
		if result.Outcome != browserproof.OutcomeRefused || result.Code != browserproof.CodeUnreachableRoute {
			t.Fatalf("AUR-429/AC-001/wrong-refusal: got %s (%s): %s", result.Code, result.Outcome, result.Detail)
		}
	})
}

// ---------------------------------------------------------------------------
// chain steps
// ---------------------------------------------------------------------------

// aur429iGenerate runs the real generator over the repository fixture and
// returns the documentation directory it wrote. A generator that cannot run is
// a broken dependency of this card (AUR-425 owns its behavior), so failures
// here are infrastructure, never a verdict about this card.
func aur429iGenerate(t *testing.T) string {
	t.Helper()

	root := aur429iRepoRoot(t)
	src := filepath.Join(root, "tests", "fixtures", "docs", "goproject")
	if _, err := os.Stat(filepath.Join(src, "greeting.go")); err != nil {
		t.Fatalf("AUR-429/AC-001/infrastructure/closure: fixture not materialized: %v", err)
	}

	binary := aur429iBuildGenerator(t, root)
	out := filepath.Join(t.TempDir(), "docs")

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary)
	cmd.Dir = t.TempDir()
	// Minimal explicit environment: an inherited LLM_API_KEY would route the
	// run through a provider, and this chain must stay offline-deterministic.
	cmd.Env = []string{
		"PATH=" + t.TempDir(),
		"HOME=" + t.TempDir(),
		"AURUMCODE_SOURCE_DIR=" + src,
		"AURUMCODE_OUTPUT_DIR=" + out,
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("AUR-429/AC-001/infrastructure/generator: %v\n%s", err, output)
	}
	for _, page := range []string{"index.md", filepath.Join("go", "root.md")} {
		if _, err := os.Stat(filepath.Join(out, page)); err != nil {
			t.Fatalf("AUR-429/AC-001/infrastructure/generator: expected page missing: %v", err)
		}
	}
	return out
}

func aur429iPublish(t *testing.T, docsDir string) string {
	t.Helper()

	published := filepath.Join(t.TempDir(), "published")
	if _, err := sitepublish.PublishDocs(docsDir, published); err != nil {
		t.Fatalf("AUR-429/AC-001/behavior-missing: publish stand-in failed: %v", err)
	}
	return published
}

func aur429iVerify(t *testing.T, site string) (browserproof.DocsVerifyResultV1, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	return browserproof.New(browserproof.NewScriptedDriver()).VerifyDocs(ctx, browserproof.DocsVerifyRequest{
		Card:          "AUR-429",
		SiteDir:       site,
		PublishedURL:  aur429iURL,
		IndexSelector: "h2",
		IndexText:     aur429iIndexText,
		// h4 is the element the publisher emits for the generated "#### func
		// NewGreeting" heading; asserting it keeps the expected content inside
		// the bounded evidence window on a long page.
		ContentSelector:    "h4",
		ContentText:        aur429iContentText,
		DriverLock:         browserproof.DriverLock{Kind: browserproof.ScriptedDriverKind, Digest: browserproof.ScriptedDriverDigest},
		NavigationDeadline: 10 * time.Second,
	})
}

// ---------------------------------------------------------------------------
// helpers (deliberately self-contained: package integration is staged and
// compiled on its own by the acceptance)
// ---------------------------------------------------------------------------

func aur429iRepoRoot(t *testing.T) string {
	t.Helper()

	if root := os.Getenv("AURUMCODE_ROOT"); root != "" {
		return root
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("AUR-429/AC-001/infrastructure: cannot locate the test source file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func aur429iBuildGenerator(t *testing.T, root string) string {
	t.Helper()

	if _, err := os.Stat(filepath.Join(root, "cmd", "regenerate-docs", "main.go")); err != nil {
		t.Fatalf("AUR-429/AC-001/infrastructure/closure: cmd/regenerate-docs not materialized under %s: %v", root, err)
	}

	binary := filepath.Join(t.TempDir(), "regenerate-docs")
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/regenerate-docs")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("AUR-429/AC-001/infrastructure/build: %v\n%s", err, output)
	}
	return binary
}

func aur429iCopyDocs(t *testing.T, docsDir string) string {
	t.Helper()

	copy := filepath.Join(t.TempDir(), "docs")
	err := filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(docsDir, path)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(copy, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		return os.WriteFile(target, content, 0o644)
	})
	if err != nil {
		t.Fatalf("AUR-429/AC-001/infrastructure/fixture: copy generated docs: %v", err)
	}
	return copy
}

func aur429iRewrite(t *testing.T, path string, transform func(string) string) {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("AUR-429/AC-001/infrastructure/fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte(transform(string(content))), 0o644); err != nil {
		t.Fatalf("AUR-429/AC-001/infrastructure/fixture: %v", err)
	}
}
