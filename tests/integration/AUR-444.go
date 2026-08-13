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
//
// Criterion (a), against the REAL binary: AUR-444's read_paths now carries
// cmd/aurumcode's full compile closure, so this layer also builds
// cmd/aurumcode for real (`go build ./cmd/aurumcode`) and confronts the
// entrypoint's actual argv against that binary's own `review --help`
// output -- see aur444RequireRealBinaryAcceptsEntrypointFlags below. This is
// independent of, and does not replace, tests/unit/AUR-444.go's go/parser
// proof of the same criterion from source text.
package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
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

// IntegrationAUR444 is AUR-444's Integration-layer proof (selector name
// inherited verbatim from the card's own TDD proof section; see
// tests/unit/AUR-444.go's header for why it is kept as declared).
func IntegrationAUR444(t *testing.T) {
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

	// --- Criterion (a), proved against the REAL binary, not source text.
	// AUR-444's read_paths now carries cmd/aurumcode's full compile closure
	// (internal/analyzer, internal/prompt, internal/review, internal/llm,
	// internal/security/redaction, internal/git/githubclient, pkg/types),
	// so `go build ./cmd/aurumcode` succeeds here, inside the sealed
	// acceptance sandbox, and this confronts the entrypoint's actual argv
	// against the binary's own `--help` output -- the same criterion (a)
	// tests/unit/AUR-444.go proves from source with go/parser, now also
	// proved by running the artifact itself. Neither replaces the other:
	// go/parser needs no build and stays a fast, independent cross-check;
	// this needs the build but is unimpeachable -- if the entrypoint's
	// flags did not parse, `--help`'s own exit code would already say so
	// (Go's flag package still emits full usage on -h/--help, exit 2).
	aur444RequireRealBinaryAcceptsEntrypointFlags(t, root, entrypoint)
}

// aur444RealReviewFlags builds cmd/aurumcode from root and parses the real
// `review --help` usage output (Go's flag package format: "  -name\n
// \tdescription") into the set of flag names it actually registers.
func aur444RealReviewFlags(t *testing.T, root string) map[string]bool {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "aurumcode-aur444-real")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	var buildOut bytes.Buffer
	build.Stdout = &buildOut
	build.Stderr = &buildOut
	if err := build.Run(); err != nil {
		t.Fatalf("AUR-444/AC-001/infrastructure: go build ./cmd/aurumcode failed: %v\n%s", err, buildOut.String())
	}

	help := exec.Command(binPath, "review", "--help")
	var helpOut bytes.Buffer
	help.Stdout = &helpOut
	help.Stderr = &helpOut
	// --help makes Go's flag package return flag.ErrHelp, which
	// cmd/aurumcode's fs.Parse (flag.ContinueOnError) reports as exit 2
	// after printing full usage -- expected here, not an infrastructure
	// failure; only a build/exec failure (binary absent or unrunnable) is.
	_ = help.Run()

	flagLineRE := regexp.MustCompile(`^\s{2}-([A-Za-z][A-Za-z0-9-]*)\b`)
	flags := map[string]bool{}
	for _, line := range strings.Split(helpOut.String(), "\n") {
		if m := flagLineRE.FindStringSubmatch(line); m != nil {
			flags[m[1]] = true
		}
	}
	if len(flags) == 0 {
		t.Fatalf("AUR-444/AC-001/infrastructure: `%s review --help` printed zero recognizable flag lines:\n%s", binPath, helpOut.String())
	}
	return flags
}

// aur444EntrypointReviewFlagTokens extracts the long-flag tokens (without
// --) the entrypoint's review invocation sends, following backslash line
// continuations, skipping commented-out lines. Independent re-derivation
// from tests/unit/AUR-444.go's aur444EntrypointFlagUses (different
// package -- test packages in this office are proved standalone, see
// tests/acceptance/AUR-444.sh's per-selector staging), but the same method.
func aur444EntrypointReviewFlagTokens(entrypoint string) map[string]bool {
	invocationRE := regexp.MustCompile(`\$(?:\{AURUMCODE_CLI\}|AURUMCODE_CLI)"?\s+review\b`)
	flagTokenRE := regexp.MustCompile(`--([A-Za-z][A-Za-z0-9-]*)`)
	lines := strings.Split(entrypoint, "\n")
	tokens := map[string]bool{}
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if !invocationRE.MatchString(line) {
			continue
		}
		block := line
		for strings.HasSuffix(block, `\`) && i+1 < len(lines) {
			i++
			block = strings.TrimSuffix(block, `\`) + "\n" + lines[i]
		}
		for _, m := range flagTokenRE.FindAllStringSubmatch(block, -1) {
			tokens[m[1]] = true
		}
	}
	return tokens
}

func aur444RequireRealBinaryAcceptsEntrypointFlags(t *testing.T, root, entrypoint string) {
	t.Helper()
	realFlags := aur444RealReviewFlags(t, root)
	sent := aur444EntrypointReviewFlagTokens(entrypoint)
	if len(sent) == 0 {
		t.Fatalf("AUR-444/AC-001/behavior-missing: no review flag tokens found in the entrypoint to confront against the real binary")
	}
	if !sent["base"] {
		t.Fatalf("AUR-444/AC-001/behavior-missing: entrypoint never sends --base (real binary flags: %v)", aur444SortedSet(realFlags))
	}
	var unknown []string
	for flagName := range sent {
		if !realFlags[flagName] {
			unknown = append(unknown, flagName)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		t.Fatalf("AUR-444/AC-001/behavior-missing: the REAL binary (go build ./cmd/aurumcode; review --help) does not register flag(s) the entrypoint sends: --%s (registered: %s)",
			strings.Join(unknown, ", --"), strings.Join(aur444SortedSet(realFlags), ", "))
	}
}

func aur444SortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func aur444SortedNames(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
