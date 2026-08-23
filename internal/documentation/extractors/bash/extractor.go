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

// bashKeywordPattern recognizes `function name` and `function name()`
// declarations (with or without a trailing "{" on the same line).
var bashKeywordPattern = regexp.MustCompile(`^\s*function\s+([A-Za-z_][A-Za-z0-9_]*)\s*(?:\(\s*\))?\s*\{?\s*$`)

// bashParenPattern recognizes the POSIX `name()` function declaration form.
var bashParenPattern = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*\(\s*\)\s*\{?\s*$`)

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

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(trimmed, "#!/"):
			// Shebang: not a doc comment, and it never had a pending block
			// built up before it (it is always the file's first line).
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
			flushStray()
			continue
		}

		if name, signature, ok := matchBashFunction(line); ok {
			page.Symbols = append(page.Symbols, bashFunctionSymbol{
				Name:      name,
				Signature: signature,
				Doc:       strings.Join(pendingDoc, "\n"),
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

	for _, sym := range page.Symbols {
		fmt.Fprintf(&b, "### function %s\n\n", sym.Name)
		if sym.Doc != "" {
			fmt.Fprintf(&b, "%s\n\n", sym.Doc)
		}
		fmt.Fprintf(&b, "```bash\n%s\n```\n\n", sym.Signature)
	}

	return b.String()
}
