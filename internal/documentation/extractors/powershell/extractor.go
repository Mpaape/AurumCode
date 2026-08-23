package powershell

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

// PowerShellExtractor extracts documentation from PowerShell scripts
type PowerShellExtractor struct {
	runner site.CommandRunner
}

// NewPowerShellExtractor creates a new PowerShell documentation extractor
func NewPowerShellExtractor(runner site.CommandRunner) *PowerShellExtractor {
	return &PowerShellExtractor{
		runner: runner,
	}
}

// Extract generates documentation from PowerShell scripts
func (p *PowerShellExtractor) Extract(ctx context.Context, req *extractors.ExtractRequest) (*extractors.ExtractResult, error) {
	if req.Language != extractors.LanguagePowerShell {
		return nil, fmt.Errorf("invalid language: expected %s, got %s", extractors.LanguagePowerShell, req.Language)
	}

	if _, err := os.Stat(req.SourceDir); err != nil {
		return nil, fmt.Errorf("invalid source directory: %w", err)
	}

	if err := os.MkdirAll(req.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Find PowerShell scripts
	scripts, err := p.findPowerShellScripts(req.SourceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to find PowerShell scripts: %w", err)
	}

	if len(scripts) == 0 {
		return &extractors.ExtractResult{
			Language: extractors.LanguagePowerShell,
			Files:    []string{},
			Stats:    extractors.ExtractionStats{},
		}, nil
	}

	// Extract documentation from each script
	result := &extractors.ExtractResult{
		Language: extractors.LanguagePowerShell,
		Files:    []string{},
		Stats:    extractors.ExtractionStats{},
	}

	for _, script := range scripts {
		outputPath := filepath.Join(req.OutputDir, filepath.Base(script)+".md")
		if err := p.extractScriptDocs(script, outputPath); err != nil {
			result.Errors = append(result.Errors, err)
			continue
		}
		result.Files = append(result.Files, outputPath)
		result.Stats.FilesProcessed++
		result.Stats.DocsGenerated++
	}

	return result, nil
}

// Validate checks if PowerShell is available.
//
// Only a genuine lookup failure means pwsh is unavailable; a pwsh that is
// installed and exits non-zero is an extraction error, not a skip.
func (p *PowerShellExtractor) Validate(ctx context.Context) error {
	_, err := p.runner.Run(ctx, "pwsh", []string{"-Version"}, ".", nil)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return extractors.NewToolUnavailableError("pwsh", "please install from https://aka.ms/powershell", err)
		}
		return fmt.Errorf("pwsh is installed but failed: %w", err)
	}
	return nil
}

// Language returns the language this extractor handles
func (p *PowerShellExtractor) Language() extractors.Language {
	return extractors.LanguagePowerShell
}

func (p *PowerShellExtractor) findPowerShellScripts(rootDir string) ([]string, error) {
	scripts := []string{}
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".ps1" || ext == ".psm1" {
			scripts = append(scripts, path)
		}
		return nil
	})
	return scripts, err
}

func (p *PowerShellExtractor) extractScriptDocs(scriptPath, outputPath string) error {
	page, err := scanPowerShellFile(scriptPath)
	if err != nil {
		return err
	}

	markdown := renderPowerShellMarkdown(filepath.Base(scriptPath), page)
	if err := os.WriteFile(outputPath, []byte(markdown), 0644); err != nil {
		return err
	}

	// Confirm on disk before the caller counts this script as documented.
	return extractors.ConfirmOutputFile("powershell comment extraction", outputPath)
}

// powerShellFunctionSymbol is one function (cmdlet-style, script-defined)
// this parser recognized, documented or not: an undocumented function is
// still real, callable API, so it is still reported -- just with no prose
// (see powerShellFunctionPattern).
type powerShellFunctionSymbol struct {
	// Name is the function's own identifier (its Verb-Noun name, or
	// whatever the script itself uses), the page heading for this symbol
	// and, via Markdown's own heading-to-anchor rule, its unique anchor.
	Name string
	// Signature is the function's own declaration line, trimmed and with a
	// trailing "{" removed.
	Signature string
	// Doc is the comment-based help or line-comment block that immediately
	// preceded this function, already stripped of its comment markers.
	// Empty when the function carries no comment: AC-003 requires the
	// symbol still be reported, with its signature and no synthesized prose.
	Doc string
}

// powerShellPage is everything scanPowerShellFile found in one script.
type powerShellPage struct {
	// Notes carries every comment block that precedes something other than
	// a recognized function. Such a comment documents the SCRIPT, not any
	// one symbol, so it is never attached to a function and never given its
	// own "## Documentation"-style heading per block -- that repetition is
	// exactly AUR-464's defect. Instead every stray block is collected once
	// under a single, real, non-repeating heading.
	Notes string
	// Symbols are the functions this parser recognized, in source order.
	Symbols []powerShellFunctionSymbol
}

// powerShellFunctionPattern recognizes a `function Name` declaration (with
// or without a parameter list / trailing "{" on the same line). PowerShell
// names commonly use "Verb-Noun" hyphenation, hence the hyphen in the class.
var powerShellFunctionPattern = regexp.MustCompile(`^\s*function\s+([A-Za-z_][A-Za-z0-9_.-]*)\b`)

// cleanPowerShellSignature trims a declaration line down to a one-line
// signature: no trailing "{", no leading/trailing whitespace.
func cleanPowerShellSignature(line string) string {
	s := strings.TrimSpace(line)
	s = strings.TrimSuffix(s, "{")
	s = strings.TrimRight(s, " \t")
	return s
}

// scanPowerShellFile reads one script and separates its comments into
// function documentation (attached to the function each comment block --
// line comments or a `<# ... #>` block -- immediately precedes) and
// script-level notes (every comment block that precedes anything else,
// collected once).
func scanPowerShellFile(path string) (powerShellPage, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return powerShellPage{}, err
	}

	var page powerShellPage
	var pendingDoc []string
	var strayNotes []string
	inBlockComment := false

	flushStray := func() {
		if len(pendingDoc) > 0 {
			strayNotes = append(strayNotes, strings.Join(pendingDoc, "\n"))
			pendingDoc = nil
		}
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if inBlockComment {
			if strings.HasSuffix(trimmed, "#>") {
				body := strings.TrimSpace(strings.TrimSuffix(trimmed, "#>"))
				if body != "" {
					pendingDoc = append(pendingDoc, body)
				}
				inBlockComment = false
			} else {
				pendingDoc = append(pendingDoc, trimmed)
			}
			continue
		}

		switch {
		case strings.HasPrefix(trimmed, "<#"):
			body := strings.TrimSpace(strings.TrimPrefix(trimmed, "<#"))
			if strings.HasSuffix(body, "#>") {
				body = strings.TrimSpace(strings.TrimSuffix(body, "#>"))
				if body != "" {
					pendingDoc = append(pendingDoc, body)
				}
			} else {
				if body != "" {
					pendingDoc = append(pendingDoc, body)
				}
				inBlockComment = true
			}
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

		if m := powerShellFunctionPattern.FindStringSubmatch(line); m != nil {
			doc := strings.Join(pendingDoc, "\n")
			if len(pendingDoc) > 0 && len(page.Symbols) == 0 && len(strayNotes) == 0 {
				// This comment block is the very FIRST content the scan has
				// resolved in the whole file. Structurally that is
				// indistinguishable from a file-level header (a license
				// notice, a copyright banner, comment-based help meant for
				// the whole script) glued directly to the first function
				// with no blank line separating them, from that same first
				// function simply carrying its own doc comment: both shapes
				// are "comment, then immediately the first symbol," with
				// nothing else in the file yet to tell them apart. Per the
				// card, a comment attached to the wrong symbol documents a
				// lie, which is strictly worse than a symbol carrying no
				// prose, so this ambiguous leading block is always treated
				// as a script-level note, never attached here. Every block
				// that follows is unambiguous: it necessarily comes after
				// something (a prior symbol or a prior note) already
				// resolved the file's own header.
				strayNotes = append(strayNotes, doc)
				doc = ""
			}
			page.Symbols = append(page.Symbols, powerShellFunctionSymbol{
				Name:      m[1],
				Signature: cleanPowerShellSignature(line),
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

// renderPowerShellMarkdown turns one script's scanned page into Markdown: at
// most one "## Script Notes" section for comments no function owns, then one
// "### function <name>" section per recognized function, each carrying its
// own real signature and its own real doc comment (or no prose, if the
// function carries none).
func renderPowerShellMarkdown(scriptName string, page powerShellPage) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# %s\n\n", scriptName)

	if page.Notes != "" {
		fmt.Fprintf(&b, "## Script Notes\n\n%s\n\n", page.Notes)
	}

	baseAnchorCount := map[string]int{}
	for _, sym := range page.Symbols {
		heading := "function " + sym.Name
		// AC-002 requires distinct ANCHORS, not just distinct heading text:
		// the site's renderer slugs a heading to lowercase before turning
		// it into an anchor, so e.g. "function Get-Thing" and
		// "function get-thing" carry different text but collide on the
		// same anchor -- which would send a link for one to the other's
		// page. baseAnchorCount is keyed by each heading's OWN
		// (undisambiguated) anchor, so every symbol that would collide on
		// it -- not just the second one -- gets counted and, past the
		// first, a distinguishing suffix that changes ITS anchor too.
		// Checking the raw heading text alone would have missed this.
		base := headingAnchor(heading)
		baseAnchorCount[base]++
		if n := baseAnchorCount[base]; n > 1 {
			heading = fmt.Sprintf("%s (%d)", heading, n)
		}
		fmt.Fprintf(&b, "### %s\n\n", heading)
		if sym.Doc != "" {
			fmt.Fprintf(&b, "%s\n\n", sym.Doc)
		}
		fmt.Fprintf(&b, "```powershell\n%s\n```\n\n", sym.Signature)
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
