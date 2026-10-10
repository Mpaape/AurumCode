// AUR-611: the version stamped at build time (main.version, the image's
// AURUMCODE_VERSION) is named in the parecer's details, the audit record and
// the SARIF; a dev build publishes exactly what it published before.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/review/blocking"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// stampVersion sets main.version as -ldflags "-X main.version=..." would,
// restoring it when the test ends. No test in this package runs in
// parallel with one that reads it.
func stampVersion(t *testing.T, value string) {
	t.Helper()
	saved := version
	t.Cleanup(func() { version = saved })
	version = value
}

// aur611Result has one observation and one limitation, so the parecer has
// a details block of its own.
func aur611Result() *types.ReviewResult {
	return &types.ReviewResult{
		Issues:      []types.ReviewIssue{{File: "app.go", Line: 7, Severity: "info", RuleID: "quality/poor-naming", Message: "ReadAll could say what it reads"}},
		Limitations: []string{"diff grande"},
	}
}

// The parecer of aur611Result and of an empty result as the formatter wrote
// them before AUR-611 (captured from the unchanged code).
const (
	aur611ParecerBefore = "<!-- aurumcode-review -->\n## AurumCode revisão de código\n\n> [!NOTE]\n> **Aprovado com 1 observação que não bloqueia o merge.**\n\nsem gate declarado\n\n### Observações (não bloqueiam)\n\n- `app.go:7` — ReadAll could say what it reads (`quality/poor-naming`)\n\n<details>\n<summary>Detalhes da revisão</summary>\n\n#### Limitações da revisão\n\n- diff grande\n\n</details>\n"
	aur611EmptyBefore   = "<!-- aurumcode-review -->\n## AurumCode revisão de código\n\n> [!TIP]\n> **Aprovado: nenhum problema encontrado na mudança revisada.**\n\nsem gate declarado\n"
)

// detailsOf is the content of the parecer's collapsed block, or "".
func detailsOf(body string) string {
	start := strings.Index(body, "<details>")
	end := strings.Index(body, "</details>")
	if start < 0 || end < start {
		return ""
	}
	return body[start:end]
}

// AC-002: a stamped build names its version inside the details, in the
// review's language, after the sections that were there; with nothing else
// to detail, the block holds the version line alone.
func TestAUR611DetailsCarryToolVersionWhenNotDev(t *testing.T) {
	stampVersion(t, "v9.9.9")
	body := formatReviewDocument(aur611Result(), nil, "pt-BR", blocking.Ungated())
	details := detailsOf(body)
	if !strings.Contains(details, "- diff grande\n\nRevisado com AurumCode v9.9.9") {
		t.Fatalf("details lack the version line after the limitations:\n%s", body)
	}
	if strings.Count(body, "AurumCode v9.9.9") != 1 {
		t.Fatalf("the version is named once, in the details:\n%s", body)
	}
	if want := strings.Replace(aur611ParecerBefore, "- diff grande\n\n</details>", "- diff grande\n\nRevisado com AurumCode v9.9.9\n\n</details>", 1); body != want {
		t.Fatalf("the parecer changed beyond the version line:\n%q\nwant:\n%q", body, want)
	}
	english := formatReviewDocument(&types.ReviewResult{}, nil, "en", blocking.Ungated())
	if details := detailsOf(english); !strings.Contains(details, "\n\nReviewed with AurumCode v9.9.9") {
		t.Fatalf("an otherwise empty details block lacks the version line:\n%s", english)
	}
	stampVersion(t, "0123456789ab")
	if body := formatReviewDocument(aur611Result(), nil, "pt-BR", blocking.Ungated()); !strings.Contains(detailsOf(body), "Revisado com AurumCode 0123456789ab") {
		t.Fatalf("a short SHA version is not named:\n%s", body)
	}
}

// AC-002 (MUT-001): a dev build's parecer is byte-identical to the one
// before AUR-611; no "AurumCode dev" line, and no details block where there
// was none.
func TestAUR611DetailsOmitToolVersionWhenDev(t *testing.T) {
	for _, dev := range []string{"dev", ""} {
		stampVersion(t, dev)
		if body := formatReviewDocument(aur611Result(), nil, "pt-BR", blocking.Ungated()); body != aur611ParecerBefore {
			t.Fatalf("dev build %q changed the parecer:\n%q\nwant:\n%q", dev, body, aur611ParecerBefore)
		}
		if body := formatReviewDocument(&types.ReviewResult{}, nil, "pt-BR", blocking.Ungated()); body != aur611EmptyBefore {
			t.Fatalf("dev build %q changed the empty parecer:\n%q\nwant:\n%q", dev, body, aur611EmptyBefore)
		}
		if got := detailsSections(&types.ReviewResult{}, reviewCopyFor("en")); got != "" {
			t.Fatalf("dev build %q has details for an empty result: %q", dev, got)
		}
	}
}

// AC-001: the SARIF names the stamped version as the tool's version, and a
// dev build keeps "dev".
func TestAUR611SARIFCarriesStampedVersion(t *testing.T) {
	for stamped, want := range map[string]string{"v9.9.9": "v9.9.9", "dev": "dev", "": "dev"} {
		stampVersion(t, stamped)
		path := filepath.Join(t.TempDir(), "review.sarif")
		if err := writeSARIFFile(complianceArtifactInputs{sarifPath: path}, redaction.NewFilter()); err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Runs []struct {
				Tool struct {
					Driver struct {
						Version string `json:"version"`
					} `json:"driver"`
				} `json:"tool"`
			} `json:"runs"`
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}
		if len(doc.Runs) != 1 || doc.Runs[0].Tool.Driver.Version != want {
			t.Fatalf("stamped %q: SARIF tool version = %+v, want %q", stamped, doc.Runs, want)
		}
	}
}

// AC-001: --version prints the stamped version, and dev for a build that
// stamped none.
func TestAUR611VersionFlagPrintsStampedVersion(t *testing.T) {
	for stamped, want := range map[string]string{"v9.9.9": "aurumcode v9.9.9\n", "": "aurumcode dev\n"} {
		stampVersion(t, stamped)
		var out strings.Builder
		printVersion(&out)
		if out.String() != want {
			t.Fatalf("stamped %q: --version printed %q, want %q", stamped, out.String(), want)
		}
	}
}

// AC-003: the audit file the review writes carries tool_version for a
// stamped build and no such key for a dev build.
func TestAUR611AuditFileCarriesToolVersionOnlyWhenNotDev(t *testing.T) {
	for stamped, want := range map[string]string{"v9.9.9": "v9.9.9", "dev": ""} {
		stampVersion(t, stamped)
		path := filepath.Join(t.TempDir(), "audit.json")
		if err := writeAuditFile(complianceArtifactInputs{auditoriaPath: path, repo: "o/r"}, redaction.NewFilter()); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		value, present := got["tool_version"]
		if want == "" && present {
			t.Fatalf("dev build wrote tool_version: %s", data)
		}
		if want != "" && value != want {
			t.Fatalf("stamped %q: tool_version = %v, want %q", stamped, value, want)
		}
	}
}
