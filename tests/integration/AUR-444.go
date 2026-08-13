// Package integration holds AUR-444's Integration-layer proof: the
// cross-artifact contract between scripts/action-entrypoint.sh, Dockerfile
// and action.yml -- the three files the card's Outcome spans -- agree with
// each other, and none of the three falsehoods the card measured survives
// in the file that carried it.
//
// Criterion (b): the Dockerfile actually compiles and copies cmd/aurumcode,
// AND the binary path it copies to is the exact path
// scripts/action-entrypoint.sh resolves AURUMCODE_CLI to by default. Checked
// jointly, not independently: independently, the image could ship
// /app/aurumcode while the script still invoked /app/cli and both checks
// would pass, which is the same class of defect this card exists to close
// only moved one file over.
//
// Criterion (c): none of the three false claims the card's "Achado medido"
// names survives, checked as absence of the ORIGINAL exact wording (so a
// revert is caught) AND presence of a corrected replacement (so a fix that
// only deletes the comment, without saying anything true, is also caught).
// gomarkdoc is checked in both files it appeared in (Dockerfile and
// action.yml), not only the one the card cites a line number for.
//
// Anchors are long, specific literal substrings on purpose -- the AUR-440
// lesson (a bare 'v1' matches inside "bootstrap-readonly-v1") applies here
// exactly as much as it did there.
package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func aur444IntegrationRoot(t *testing.T) string {
	t.Helper()
	if root := os.Getenv("AURUMCODE_ROOT"); root != "" {
		return root
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("AUR-444/AC-001/infrastructure: getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("AUR-444/AC-001/infrastructure: no go.mod above the working directory and AURUMCODE_ROOT is unset")
		}
		dir = parent
	}
}

func aur444ReadFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("AUR-444/AC-001/infrastructure: reading %s: %v", path, err)
	}
	return string(raw)
}

// aur444DockerfileCopyRE matches a `COPY --from=builder /build/<src> /app/<dst>`
// instruction and captures <dst>, the basename the final image carries it
// under.
var aur444DockerfileCopyRE = regexp.MustCompile(`(?m)^COPY --from=builder /build/[A-Za-z0-9_.-]+ /app/([A-Za-z0-9_.-]+)$`)

// aur444EntrypointCLIDefaultRE matches the entrypoint's own default
// resolution of AURUMCODE_CLI and captures the binary basename it falls
// back to when the caller does not override it.
var aur444EntrypointCLIDefaultRE = regexp.MustCompile(`AURUMCODE_CLI="\$\{AURUMCODE_CLI:-\$\{AURUMCODE_BIN_DIR\}/([A-Za-z0-9_.-]+)\}"`)

// IntegrationAUR428 is AUR-444's Integration-layer proof (selector name
// inherited verbatim from the card's own TDD proof section; see
// tests/unit/AUR-444.go's header for why it is kept as declared).
func IntegrationAUR428(t *testing.T) {
	root := aur444IntegrationRoot(t)
	dockerfile := aur444ReadFile(t, filepath.Join(root, "Dockerfile"))
	actionYML := aur444ReadFile(t, filepath.Join(root, "action.yml"))
	entrypoint := aur444ReadFile(t, filepath.Join(root, "scripts", "action-entrypoint.sh"))

	// --- Criterion (b): the image actually contains the binary the script
	// invokes, at the exact path the script resolves by default. ---
	if !strings.Contains(dockerfile, "-o aurumcode ./cmd/aurumcode") {
		t.Fatalf("AUR-444/AC-001/behavior-missing: Dockerfile has no `go build -o aurumcode ./cmd/aurumcode` instruction")
	}
	copies := aur444DockerfileCopyRE.FindAllStringSubmatch(dockerfile, -1)
	if len(copies) == 0 {
		t.Fatalf("AUR-444/AC-001/behavior-missing: Dockerfile has no `COPY --from=builder /build/<bin> /app/<bin>` instruction at all")
	}
	copiedNames := map[string]bool{}
	for _, m := range copies {
		copiedNames[m[1]] = true
	}
	if !copiedNames["aurumcode"] {
		t.Fatalf("AUR-444/AC-001/behavior-missing: Dockerfile never copies a binary named 'aurumcode' into /app (copies: %v)", copiedNames)
	}

	cliMatch := aur444EntrypointCLIDefaultRE.FindStringSubmatch(entrypoint)
	if cliMatch == nil {
		t.Fatalf("AUR-444/AC-001/behavior-missing: scripts/action-entrypoint.sh does not resolve AURUMCODE_CLI to a ${AURUMCODE_BIN_DIR}-relative default at all")
	}
	cliDefaultName := cliMatch[1]
	if !copiedNames[cliDefaultName] {
		t.Fatalf("AUR-444/AC-001/behavior-missing: entrypoint's AURUMCODE_CLI default resolves to binary %q, but the Dockerfile only copies %v into /app -- the script would invoke a binary the image never ships",
			cliDefaultName, aur444SortedNames(copiedNames))
	}

	// --- Criterion (c), defect 2's false claim: "there is no cmd/cli in the
	// source tree" (originally in both scripts/action-entrypoint.sh and, in
	// a paraphrased form, Dockerfile). ---
	const falseNoCLIClaim = "there is no cmd/cli in the source tree"
	if strings.Contains(entrypoint, falseNoCLIClaim) {
		t.Fatalf("AUR-444/AC-001/behavior-missing: scripts/action-entrypoint.sh still claims %q", falseNoCLIClaim)
	}
	const falseOnlyPackageClaim = "only main package in the source tree"
	if strings.Contains(dockerfile, falseOnlyPackageClaim) {
		t.Fatalf("AUR-444/AC-001/behavior-missing: Dockerfile still claims %q", falseOnlyPackageClaim)
	}
	const falseExactlyOneClaim = "ships exactly one binary"
	if strings.Contains(dockerfile, falseExactlyOneClaim) {
		t.Fatalf("AUR-444/AC-001/behavior-missing: Dockerfile still claims %q", falseExactlyOneClaim)
	}
	if !strings.Contains(entrypoint, "cmd/aurumcode") {
		t.Fatalf("AUR-444/AC-001/behavior-missing: scripts/action-entrypoint.sh never mentions cmd/aurumcode as the source of the binary it invokes")
	}

	// Same false "only one binary" shape, in a second, easy-to-miss spot: an
	// exclusivity claim about the OTHER binary (regenerate-docs) or the
	// OTHER mode (documentation) that a fix could correct in one place and
	// leave standing in another -- exactly the internal-contradiction risk
	// this card exists to close.
	const falseOneGeneratorClaim = "ONE binary the image"
	if strings.Contains(entrypoint, falseOneGeneratorClaim) {
		t.Fatalf("AUR-444/AC-001/behavior-missing: scripts/action-entrypoint.sh still claims %q, contradicted by the two-binary image this card ships", falseOneGeneratorClaim)
	}
	const falseOnlyModeClaim = "is the only mode this image can actually serve"
	if strings.Contains(dockerfile, falseOnlyModeClaim) {
		t.Fatalf("AUR-444/AC-001/behavior-missing: Dockerfile still claims %q, false now that review is servable too", falseOnlyModeClaim)
	}

	// --- Criterion (c), defect 3's false claim: gomarkdoc required for Go,
	// in BOTH files it appeared in, not only the one the card cites a line
	// number for. ---
	const falseGomarkdocFatal = "gomarkdoc failure is fatal"
	if strings.Contains(actionYML, falseGomarkdocFatal) {
		t.Fatalf("AUR-444/AC-001/behavior-missing: action.yml still claims %q", falseGomarkdocFatal)
	}
	const activeGomarkdocInstall = "go install \"github.com/princjef/gomarkdoc"
	if strings.Contains(actionYML, activeGomarkdocInstall) {
		t.Fatalf("AUR-444/AC-001/behavior-missing: action.yml still unconditionally installs gomarkdoc")
	}
	if strings.Contains(dockerfile, "go install github.com/princjef/gomarkdoc") {
		t.Fatalf("AUR-444/AC-001/behavior-missing: Dockerfile still installs gomarkdoc")
	}
	if !strings.Contains(actionYML, "AUR-424") {
		t.Fatalf("AUR-444/AC-001/behavior-missing: action.yml's corrected prose never cites AUR-424 (the change that removed Go's gomarkdoc dependency)")
	}
	if !strings.Contains(dockerfile, "AUR-424") {
		t.Fatalf("AUR-444/AC-001/behavior-missing: Dockerfile's corrected prose never cites AUR-424")
	}

	// Specific, not generalized (the card's own warning): the other seven
	// languages still need an external tool, and the corrected prose must
	// still say so by name for at least two of them, not erase the whole
	// requirement.
	for _, tool := range []string{"pydoc-markdown", "doxygen"} {
		if !strings.Contains(actionYML, tool) {
			t.Fatalf("AUR-444/AC-001/behavior-missing: action.yml's corrected prose stopped naming external tool %q that a non-Go extractor still needs", tool)
		}
	}
}

func aur444SortedNames(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
