package bash

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

// BashExtractor extracts documentation from Bash scripts
type BashExtractor struct {
	runner site.CommandRunner
}

// NewBashExtractor creates a new Bash documentation extractor
func NewBashExtractor(runner site.CommandRunner) *BashExtractor {
	return &BashExtractor{
		runner: runner,
	}
}

// Extract generates documentation from Bash scripts
func (b *BashExtractor) Extract(ctx context.Context, req *extractors.ExtractRequest) (*extractors.ExtractResult, error) {
	if req.Language != extractors.LanguageBash {
		return nil, fmt.Errorf("invalid language: expected %s, got %s", extractors.LanguageBash, req.Language)
	}

	if _, err := os.Stat(req.SourceDir); err != nil {
		return nil, fmt.Errorf("invalid source directory: %w", err)
	}

	if err := os.MkdirAll(req.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Find Bash scripts
	scripts, err := b.findBashScripts(req.SourceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to find Bash scripts: %w", err)
	}

	if len(scripts) == 0 {
		return &extractors.ExtractResult{
			Language: extractors.LanguageBash,
			Files:    []string{},
			Stats:    extractors.ExtractionStats{},
		}, nil
	}

	// Extract documentation from each script
	result := &extractors.ExtractResult{
		Language: extractors.LanguageBash,
		Files:    []string{},
		Stats:    extractors.ExtractionStats{},
	}

	for _, script := range scripts {
		outputPath := filepath.Join(req.OutputDir, filepath.Base(script)+".md")
		if err := b.extractScriptDocs(script, outputPath); err != nil {
			result.Errors = append(result.Errors, err)
			continue
		}
		result.Files = append(result.Files, outputPath)
		result.Stats.FilesProcessed++
		result.Stats.DocsGenerated++
	}

	return result, nil
}

// Validate checks if Bash is available.
//
// Only a genuine lookup failure means bash is unavailable; a bash that is
// installed and exits non-zero is an extraction error, not a skip.
func (b *BashExtractor) Validate(ctx context.Context) error {
	_, err := b.runner.Run(ctx, "bash", []string{"--version"}, ".", nil)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return extractors.NewToolUnavailableError("bash", "", err)
		}
		return fmt.Errorf("bash is installed but failed: %w", err)
	}
	return nil
}

// Language returns the language this extractor handles
func (b *BashExtractor) Language() extractors.Language {
	return extractors.LanguageBash
}

func (b *BashExtractor) findBashScripts(rootDir string) ([]string, error) {
	scripts := []string{}
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext == ".sh" || ext == ".bash" {
			scripts = append(scripts, path)
		}
		return nil
	})
	return scripts, err
}

func (b *BashExtractor) extractScriptDocs(scriptPath, outputPath string) error {
	page, err := scanBashFile(scriptPath)
	if err != nil {
		return err
	}

	markdown := renderBashMarkdown(filepath.Base(scriptPath), page)
	if err := os.WriteFile(outputPath, []byte(markdown), 0644); err != nil {
		return err
	}

	// Confirm on disk before the caller counts this script as documented.
	return extractors.ConfirmOutputFile("bash comment extraction", outputPath)
}

// bashFunctionSymbol is one function this parser recognized, documented or
// not: an undocumented function is still real, callable API, so it is still
// reported -- just with no prose (see bashSymbolPattern/bashKeywordPattern).
type bashFunctionSymbol struct {
	// Name is the function's own identifier, used both for this symbol's
	// page heading and (via Markdown's own heading-to-anchor rule) for the
	// anchor that makes it independently linkable.
	Name string
	// Signature is the function's own declaration line, trimmed and with a
	// trailing "{" removed.
	Signature string
	// Doc is the comment block that immediately preceded this function,
	// already stripped of its leading "#" markers. Empty when the function
	// carries no comment: AC-003 requires the symbol still be reported, with
	// its signature and no synthesized prose.
	Doc string
}

// bashPage is everything scanBashFile found in one script.
type bashPage struct {
	// Notes carries every comment block that precedes something other than
	// a recognized function (a plain statement, a variable assignment, the
	// end of file). Such a comment documents the SCRIPT, not any one symbol,
	// so it is never attached to a function and never given its own
	// "## Documentation"-style heading per block -- that repetition is
	// exactly AUR-464's defect. Instead every stray block is collected once
	// under a single, real, non-repeating heading.
	Notes string
	// Symbols are the functions this parser recognized, in source order.
	Symbols []bashFunctionSymbol
}

// bashBodyTail is shared by bashKeywordPattern and bashParenPattern: what is
// allowed to follow the declaration's own name (and, for the paren form,
// its "()") on the SAME physical line. Three shapes, and only these three:
//
//  1. nothing at all -- the opening "{" itself is on a later line ("corpo
//     abre em linha propria", the form this extractor already recognized);
//  2. a bare "{" with nothing after it but whitespace -- same case, "{"
//     just happens to share the declaration's own line;
//  3. "{", a one-line body, and a closing "}" that ends the line (trailing
//     whitespace allowed) -- AUR-474's target: `name() { body; }` with the
//     ENTIRE declaration on one physical line.
//
// This is deliberately not brace-counting: per the card, the target is the
// one-line FORM, not a Bash parser. A line whose body itself contains an
// unmatched "{"/"}" pair (nested braces, a brace inside a string) is out of
// scope; ".*" greedily consumes up to the LAST "}" on the line, which is
// enough for the common one-liner shape without attempting real balancing.
const bashBodyTail = `(?:\{\s*.*\}\s*|\{\s*)?$`

// bashKeywordPattern recognizes `function name` and `function name()`
// declarations, whether the body opens on its own line or -- see
// bashBodyTail -- the whole declaration is one physical line. The name
// class includes "-": Bash function names are not restricted the way shell
// variable names are, and a hyphenated name (e.g. "my-func") is valid and
// common; PowerShell's own function-name pattern already allows it.
var bashKeywordPattern = regexp.MustCompile(`^\s*function\s+([A-Za-z_][A-Za-z0-9_-]*)\s*(?:\(\s*\))?\s*` + bashBodyTail)

// bashParenPattern recognizes the POSIX `name()` function declaration form,
// whether the body opens on its own line or -- see bashBodyTail -- the
// whole declaration is one physical line. See bashKeywordPattern for why
// "-" is part of the name class.
//
// The name is anchored directly to "(" (only whitespace allowed between):
// this is what keeps `my-array=()`, `x=$(cmd)`, and similar non-function
// lines from matching, one-line body or not -- the char immediately after
// the name-class run is never "=" for those, so the match never starts.
var bashParenPattern = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_-]*)\s*\(\s*\)\s*` + bashBodyTail)

// matchBashFunction reports the function name and cleaned signature if line
// is a recognized function declaration.
func matchBashFunction(line string) (name string, signature string, ok bool) {
	if m := bashKeywordPattern.FindStringSubmatch(line); m != nil {
		return m[1], cleanBashSignature(line), true
	}
	if m := bashParenPattern.FindStringSubmatch(line); m != nil {
		return m[1], cleanBashSignature(line), true
	}
	return "", "", false
}

// cleanBashSignature trims a declaration line down to a one-line signature:
// no trailing "{", no leading/trailing whitespace.
func cleanBashSignature(line string) string {
	s := strings.TrimSpace(line)
	s = strings.TrimSuffix(s, "{")
	s = strings.TrimRight(s, " \t")
	return s
}

// scanBashFile reads one script and separates its comments into function
// documentation (attached to the function each comment block immediately
// precedes) and script-level notes (every comment block that precedes
// anything else, collected once).
func scanBashFile(path string) (bashPage, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return bashPage{}, err
	}

	var page bashPage
	var pendingDoc []string
	var strayNotes []string

	flushStray := func() {
		if len(pendingDoc) > 0 {
			strayNotes = append(strayNotes, strings.Join(pendingDoc, "\n"))
			pendingDoc = nil
		}
	}

	// sawCode becomes true the first time this scan resolves a REAL,
	// executable line of the script -- a function declaration, or any other
	// statement. It never resets. This is the structural signal that
	// distinguishes a file-level header from a function's own doc comment:
	// a file header, by definition, appears before any code has run; a
	// function's own doc comment appears after whatever code (if any)
	// preceded it in the script, even if that is only a "set -euo
	// pipefail" or another function's own body. The shebang and blank
	// lines are never code for this purpose -- see the two cases below.
	var sawCode bool

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(trimmed, "#!/"):
			// Shebang: not a doc comment and not code -- it never had a
			// pending block built up before it (always the file's first
			// line), and it must not itself count as "code has run yet",
			// or a header glued straight to the first function (no blank
			// line, nothing else in the file) would stop being ambiguous
			// for the wrong reason.
			continue
		case strings.HasPrefix(trimmed, "#"):
			pendingDoc = append(pendingDoc, strings.TrimSpace(strings.TrimPrefix(trimmed, "#")))
			continue
		case trimmed == "":
			// A genuinely blank line (as opposed to a bare "#" paragraph
			// separator inside a comment block, which the case above still
			// treats as a comment line) ends whatever comment was pending.
			// Without this, a file-level overview separated from the next
			// function only by whitespace would be misattributed as that
			// function's own doc instead of becoming a script-level note.
			// A blank line is not code either: it must not flip sawCode.
			flushStray()
			continue
		}

		// Every line reaching this point is a real, executable line of the
		// script (a function declaration or any other statement). Capture
		// whether code had ALREADY run before this specific line -- the
		// question that decides whether pendingDoc, if this line turns out
		// to be a function, is that function's own doc or an ambiguous
		// leading header -- before marking sawCode true for what follows.
		hadCodeBefore := sawCode
		sawCode = true

		if name, signature, ok := matchBashFunction(line); ok {
			doc := strings.Join(pendingDoc, "\n")
			// The two prior signals are COMPLEMENTARY, not substitutes for
			// one another: this comment is an ambiguous file-level header
			// only when ALL THREE hold -- it precedes the very first
			// symbol (len(page.Symbols)==0), no earlier comment block has
			// already been resolved as a note (len(strayNotes)==0, which
			// alone proves this script has no lingering ambiguity: an
			// overview already separated into its own Notes block cannot
			// also BE this function's doc), and no code has run yet
			// (!hadCodeBefore). Dropping any one of the three re-creates a
			// defect this card already fixed once: strayNotes alone missed
			// "set -euo pipefail before the doc" (round 2); hadCodeBefore
			// alone missed "overview, blank line, then the doc" (round 3,
			// this fix) -- a resolved stray note already proves the file
			// header was already found and filed, so nothing after it can
			// still be file-level-ambiguous, even if no code ever ran.
			if len(pendingDoc) > 0 && len(page.Symbols) == 0 && len(strayNotes) == 0 && !hadCodeBefore {
				// pendingDoc precedes the first executable line AND the
				// first symbol AND the first resolved note this scan has
				// seen anywhere in the file: nothing but the shebang
				// (never code) and possibly more comment lines within THIS
				// SAME block came before it. That is structurally a
				// file-level header (a license notice, a copyright banner)
				// glued directly to the first function with no blank line
				// separating them -- a common Bash style -- indistinguishable
				// by position from that same first function simply carrying
				// its own doc comment. Per the card, a comment attached to
				// the wrong symbol documents a lie, which is strictly worse
				// than a symbol carrying no prose, so this ambiguous leading
				// block is always treated as a script-level note instead.
				// The measured cost (declared in docs/specs/AUR-464.md): a
				// script whose very FIRST executable line is a documented
				// function, with NOTHING above it at all -- no code, no
				// earlier note -- loses that function's real doc into
				// Notes too. The two shapes are genuinely indistinguishable
				// by position and syntax alone. Any one of the three
				// conditions failing (code already ran, a symbol already
				// exists, or an earlier note already resolved the file's
				// own header) removes the ambiguity and the doc attaches
				// normally.
				strayNotes = append(strayNotes, doc)
				doc = ""
			}
			page.Symbols = append(page.Symbols, bashFunctionSymbol{
				Name:      name,
				Signature: signature,
				Doc:       doc,
			})
			pendingDoc = nil
			continue
		}

		// Any other code line: a pending comment block documented this
		// statement, not a function -- it becomes a script-level note, never
		// a symbol section, and never leaks onto a later function by
		// accident.
		flushStray()
	}
	flushStray()

	page.Notes = strings.Join(strayNotes, "\n\n")
	return page, nil
}

// renderBashMarkdown turns one script's scanned page into Markdown: at most
// one "## Script Notes" section for comments no function owns, then one
// "### function <name>" section per recognized function, each carrying its
// own real signature and its own real doc comment (or no prose, if the
// function carries none).
func renderBashMarkdown(scriptName string, page bashPage) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# %s\n\n", scriptName)

	if page.Notes != "" {
		fmt.Fprintf(&b, "## Script Notes\n\n%s\n\n", page.Notes)
	}

	// usedAnchors holds every FINAL anchor already written to this page --
	// after disambiguation, not the pre-suffix base -- because a suffixed
	// anchor can itself collide with either a later symbol's own plain name
	// or with another symbol's own suffixed anchor (e.g. the trio "foo",
	// "Foo", "foo-2": "Foo" disambiguates to anchor "function-foo-2", which
	// then collides with plain "foo-2"'s own anchor "function-foo-2"
	// outright). Checking only a per-base occurrence COUNT, instead of the
	// actual set of anchors already on the page, would miss exactly that
	// case. So for each symbol: try its own anchor; if taken, try
	// increasing "(N)" suffixes -- checked against usedAnchors, not
	// recomputed from a count -- until one is free, then reserve it.
	usedAnchors := map[string]bool{}
	for _, sym := range page.Symbols {
		base := "function " + sym.Name
		heading := base
		anchor := headingAnchor(heading)
		for suffix := 2; usedAnchors[anchor]; suffix++ {
			heading = fmt.Sprintf("%s (%d)", base, suffix)
			anchor = headingAnchor(heading)
		}
		usedAnchors[anchor] = true
		fmt.Fprintf(&b, "### %s\n\n", heading)
		if sym.Doc != "" {
			fmt.Fprintf(&b, "%s\n\n", sym.Doc)
		}
		fmt.Fprintf(&b, "```bash\n%s\n```\n\n", sym.Signature)
	}

	return b.String()
}

// headingAnchor mirrors the Markdown-heading-to-anchor slug rule the site's
// Jekyll/kramdown renderer applies (lowercase, spaces to hyphens, strip
// anything that is not a letter, digit, hyphen or underscore). It exists
// here only to detect, before a heading is written, whether it would
// collide with one already written on the same page -- never to alter a
// heading that does not collide.
var headingAnchorNonWord = regexp.MustCompile(`[^a-z0-9_-]+`)

func headingAnchor(text string) string {
	s := strings.ToLower(strings.TrimSpace(text))
	s = strings.ReplaceAll(s, " ", "-")
	return headingAnchorNonWord.ReplaceAllString(s, "")
}
