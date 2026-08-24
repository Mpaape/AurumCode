package prompt

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/pkg/types"
)

//go:embed templates/*.md
var templateFS embed.FS

// PromptBuilder builds prompts for LLM code review
type PromptBuilder struct {
	languageDetector *analyzer.LanguageDetector
	templates        map[string]*template.Template
	estimator        TokenEstimator
	// ruleCatalog is the closed list of rule_id values rendered into the
	// review prompt, so the model chooses from the catalog instead of
	// inventing an id the AUR-434 gate discards. See rulecatalog.go for
	// why it is mirrored here rather than imported from internal/review.
	ruleCatalog []string
}

// NewPromptBuilder creates a new prompt builder
func NewPromptBuilder() *PromptBuilder {
	pb := &PromptBuilder{
		languageDetector: analyzer.NewLanguageDetector(),
		templates:        make(map[string]*template.Template),
		estimator:        NewHeuristicEstimator(), // Default estimator
		ruleCatalog:      DefaultRuleCatalog,
	}
	pb.loadTemplates()
	return pb
}

// NewPromptBuilderWithEstimator creates a prompt builder with custom estimator
func NewPromptBuilderWithEstimator(estimator TokenEstimator) *PromptBuilder {
	pb := &PromptBuilder{
		languageDetector: analyzer.NewLanguageDetector(),
		templates:        make(map[string]*template.Template),
		estimator:        estimator,
		ruleCatalog:      DefaultRuleCatalog,
	}
	pb.loadTemplates()
	return pb
}

// loadTemplates loads prompt templates from embedded files
func (b *PromptBuilder) loadTemplates() {
	templateNames := []string{"review.md", "documentation.md", "test.md", "summary.md"}

	for _, name := range templateNames {
		content, err := templateFS.ReadFile("templates/" + name)
		if err != nil {
			// Fallback to inline templates if embedded files not found
			continue
		}

		tmpl, err := template.New(name).Parse(string(content))
		if err != nil {
			continue
		}

		b.templates[name] = tmpl
	}
}

// BuildReviewPrompt builds a prompt for code review
func (b *PromptBuilder) BuildReviewPrompt(diff *types.Diff, metrics *analyzer.DiffMetrics) string {
	// Try to use template first
	if tmpl, ok := b.templates["review.md"]; ok {
		// The catalog is rendered unconditionally here. This entry point
		// carries no token budget of its own (it takes no MaxTokens), and
		// the built-in catalog is complete compile-time data, so there is
		// no partial-list case to signal; AC-002's budget check lives in
		// BuildPrompt and ValidateRuleCatalog. Leaving the key out would
		// render the literal "<no value>" where the rule list belongs.
		data := map[string]interface{}{
			"Metrics":        b.formatMetrics(metrics),
			"Languages":      b.formatLanguages(metrics),
			"DiffContent":    b.formatDiffContent(diff),
			"CIContext":      "No CI failure context was supplied.",
			"RuleCatalog":    RenderRuleCatalog(b.ruleCatalog),
			"ReviewLanguage": "en-US",
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err == nil {
			return buf.String()
		}
	}

	// Fallback to original implementation if template fails
	return b.buildReviewPromptFallback(diff, metrics)
}

// formatMetrics formats metrics for template
func (b *PromptBuilder) formatMetrics(metrics *analyzer.DiffMetrics) string {
	return fmt.Sprintf("- Total files: %d\n- Lines added: %d\n- Lines deleted: %d\n- Test files: %d\n- Config files: %d",
		metrics.TotalFiles, metrics.LinesAdded, metrics.LinesDeleted, metrics.TestFiles, metrics.ConfigFiles)
}

// formatLanguages formats language breakdown for template
func (b *PromptBuilder) formatLanguages(metrics *analyzer.DiffMetrics) string {
	if len(metrics.LanguageBreakdown) == 0 {
		return ""
	}

	var sb strings.Builder
	for lang, count := range metrics.LanguageBreakdown {
		sb.WriteString(fmt.Sprintf("- %s: %d files\n", lang, count))
	}
	return sb.String()
}

// formatDiffContent formats diff content for template
func (b *PromptBuilder) formatDiffContent(diff *types.Diff) string {
	var sb strings.Builder
	for _, file := range diff.Files {
		sb.WriteString(fmt.Sprintf("### File: %s\n", file.Path))
		language := b.languageDetector.DetectLanguage(file.Path)
		sb.WriteString(fmt.Sprintf("Language: %s\n\n", language))

		for _, hunk := range file.Hunks {
			sb.WriteString("```diff\n")
			for _, line := range hunk.Lines {
				sb.WriteString(line + "\n")
			}
			sb.WriteString("```\n\n")
		}
	}
	return sb.String()
}

// buildReviewPromptFallback provides fallback when template is not available
func (b *PromptBuilder) buildReviewPromptFallback(diff *types.Diff, metrics *analyzer.DiffMetrics) string {
	var sb strings.Builder
	sb.WriteString("You are an expert code reviewer. Analyze the following code changes and provide a thorough review.\n\n")
	sb.WriteString(fmt.Sprintf("## Change Summary\n%s\n\n", b.formatMetrics(metrics)))

	if langs := b.formatLanguages(metrics); langs != "" {
		sb.WriteString("## Languages:\n")
		sb.WriteString(langs)
		sb.WriteString("\n")
	}

	sb.WriteString("## Code Changes\n\n")
	sb.WriteString(b.formatDiffContent(diff))

	sb.WriteString("\n## Review Instructions\n")
	sb.WriteString("Please provide a comprehensive code review covering:\n\n")
	sb.WriteString("1. **Code Quality**: Check for code smells, anti-patterns, and best practices\n")
	sb.WriteString("2. **Security**: Identify potential security vulnerabilities\n")
	sb.WriteString("3. **Performance**: Spot performance issues or inefficiencies\n")
	sb.WriteString("4. **Maintainability**: Assess code readability and maintainability\n")
	sb.WriteString("5. **Testing**: Check if changes are adequately tested\n")
	sb.WriteString("6. **Documentation**: Verify if code is properly documented\n\n")
	sb.WriteString("For each issue found, provide:\n")
	sb.WriteString("- Severity (error/warning/info)\n")
	sb.WriteString("- File path and line number\n")
	sb.WriteString("- Clear description of the issue\n")
	sb.WriteString("- Suggested fix or improvement\n\n")
	sb.WriteString("Format your response as JSON with the following structure:\n")
	sb.WriteString("```json\n{\n  \"issues\": [{\n")
	sb.WriteString("      \"file\": \"path/to/file\",\n      \"line\": 42,\n")
	sb.WriteString("      \"severity\": \"error\",\n      \"rule_id\": \"security/sql-injection\",\n")
	sb.WriteString("      \"message\": \"Description of the issue\",\n")
	sb.WriteString("      \"suggestion\": \"How to fix it\"\n    }],\n")
	sb.WriteString("  \"iso_scores\": {\n")
	sb.WriteString("    \"functionality\": 8, \"reliability\": 7, \"usability\": 9,\n")
	sb.WriteString("    \"efficiency\": 8, \"maintainability\": 7, \"portability\": 9,\n")
	sb.WriteString("    \"security\": 6, \"compatibility\": 8\n  },\n")
	sb.WriteString("  \"summary\": \"Overall assessment of the changes\"\n}\n```\n")

	return sb.String()
}

// BuildDocumentationPrompt builds a prompt for generating documentation
func (b *PromptBuilder) BuildDocumentationPrompt(diff *types.Diff, language string) string {
	// Try to use template first
	if tmpl, ok := b.templates["documentation.md"]; ok {
		data := map[string]interface{}{
			"Language":    language,
			"DiffContent": b.formatDiffContent(diff),
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err == nil {
			return buf.String()
		}
	}

	// Fallback to original implementation
	return b.buildDocumentationPromptFallback(diff, language)
}

// buildDocumentationPromptFallback provides fallback when template is not available
func (b *PromptBuilder) buildDocumentationPromptFallback(diff *types.Diff, language string) string {
	var sb strings.Builder
	sb.WriteString("You are a technical documentation expert. Generate documentation for the following code changes.\n\n")
	sb.WriteString(fmt.Sprintf("## Language: %s\n\n", language))
	sb.WriteString("## Code Changes\n\n")
	sb.WriteString(b.formatDiffContent(diff))
	sb.WriteString("## Documentation Requirements\n\n")
	sb.WriteString("Generate:\n")
	sb.WriteString("1. **API Documentation**: Document any new or modified APIs\n")
	sb.WriteString("2. **Usage Examples**: Provide clear usage examples\n")
	sb.WriteString("3. **Configuration**: Document any new configuration options\n")
	sb.WriteString("4. **Breaking Changes**: Highlight any breaking changes\n")
	sb.WriteString("5. **Migration Guide**: If applicable, provide migration steps\n\n")
	sb.WriteString("Format the documentation in Markdown.\n")
	return sb.String()
}

// BuildTestPrompt builds a prompt for generating tests
func (b *PromptBuilder) BuildTestPrompt(diff *types.Diff, language string) string {
	// Try to use template first
	if tmpl, ok := b.templates["test.md"]; ok {
		// Filter out test files
		filteredDiff := b.filterTestFiles(diff)

		data := map[string]interface{}{
			"Language":    language,
			"DiffContent": b.formatDiffContent(filteredDiff),
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err == nil {
			return buf.String()
		}
	}

	// Fallback to original implementation
	return b.buildTestPromptFallback(diff, language)
}

// filterTestFiles filters out test files from diff.
//
// The restored c12d7ab version of this function also copied
// diff.Repository/BaseSHA/HeadSHA onto the filtered copy. Those fields do
// not exist on pkg/types.Diff today (see AUR-430's restoration audit) --
// types.Diff only ever carries Files -- so there is nothing left to carry
// over besides the filtered file list.
func (b *PromptBuilder) filterTestFiles(diff *types.Diff) *types.Diff {
	filtered := &types.Diff{
		Files: []types.DiffFile{},
	}

	for _, file := range diff.Files {
		if !b.languageDetector.IsTestFile(file.Path) {
			filtered.Files = append(filtered.Files, file)
		}
	}

	return filtered
}

// buildTestPromptFallback provides fallback when template is not available
func (b *PromptBuilder) buildTestPromptFallback(diff *types.Diff, language string) string {
	var sb strings.Builder
	sb.WriteString("You are an expert test engineer. Generate comprehensive tests for the following code changes.\n\n")
	sb.WriteString(fmt.Sprintf("## Language: %s\n\n", language))
	sb.WriteString("## Code to Test\n\n")

	filteredDiff := b.filterTestFiles(diff)
	sb.WriteString(b.formatDiffContent(filteredDiff))

	sb.WriteString("## Test Requirements\n\n")
	sb.WriteString("Generate tests that cover:\n")
	sb.WriteString("1. **Happy Path**: Test normal, expected behavior\n")
	sb.WriteString("2. **Edge Cases**: Test boundary conditions and edge cases\n")
	sb.WriteString("3. **Error Handling**: Test error conditions and exceptions\n")
	sb.WriteString("4. **Integration**: Test interactions with other components\n\n")
	sb.WriteString(fmt.Sprintf("Use the testing framework appropriate for %s.\n", language))
	sb.WriteString("Include setup, test cases, and assertions.\n")
	return sb.String()
}

// BuildSummaryPrompt builds a prompt for generating a summary
func (b *PromptBuilder) BuildSummaryPrompt(diff *types.Diff, metrics *analyzer.DiffMetrics) string {
	// Try to use template first
	if tmpl, ok := b.templates["summary.md"]; ok {
		data := map[string]interface{}{
			"Metrics":   b.formatSummaryMetrics(metrics),
			"FilesList": b.formatFilesList(diff),
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err == nil {
			return buf.String()
		}
	}

	// Fallback to original implementation
	return b.buildSummaryPromptFallback(diff, metrics)
}

// formatSummaryMetrics formats metrics for summary
func (b *PromptBuilder) formatSummaryMetrics(metrics *analyzer.DiffMetrics) string {
	return fmt.Sprintf("- Files changed: %d\n- Lines: +%d -%d\n- Languages: %v",
		metrics.TotalFiles, metrics.LinesAdded, metrics.LinesDeleted, b.getLanguageList(metrics))
}

// formatFilesList formats files list for template
func (b *PromptBuilder) formatFilesList(diff *types.Diff) string {
	var sb strings.Builder
	for _, file := range diff.Files {
		sb.WriteString(fmt.Sprintf("- %s\n", file.Path))
	}
	return sb.String()
}

// buildSummaryPromptFallback provides fallback when template is not available
func (b *PromptBuilder) buildSummaryPromptFallback(diff *types.Diff, metrics *analyzer.DiffMetrics) string {
	var sb strings.Builder
	sb.WriteString("Summarize the following code changes in 2-3 sentences.\n\n")
	sb.WriteString("## Metrics\n")
	sb.WriteString(b.formatSummaryMetrics(metrics))
	sb.WriteString("\n\n## Files Changed\n")
	sb.WriteString(b.formatFilesList(diff))
	sb.WriteString("\nProvide a concise summary highlighting the main purpose and impact of these changes.\n")
	return sb.String()
}

// getLanguageList returns a list of languages from metrics
func (b *PromptBuilder) getLanguageList(metrics *analyzer.DiffMetrics) []string {
	var languages []string
	for lang := range metrics.LanguageBreakdown {
		languages = append(languages, lang)
	}
	return languages
}

// TruncatePrompt truncates a prompt to fit within token limits
func (b *PromptBuilder) TruncatePrompt(prompt string, maxTokens int) string {
	// Rough estimation: 1 token ≈ 4 characters
	maxChars := maxTokens * 4

	if len(prompt) <= maxChars {
		return prompt
	}

	// Truncate and add indication
	return prompt[:maxChars-100] + "\n\n... (truncated due to length) ...\n"
}

// BuildPrompt builds a complete prompt with token budgeting
func (b *PromptBuilder) BuildPrompt(diff *types.Diff, metrics *analyzer.DiffMetrics, opts BuildOptions) (PromptParts, error) {
	reviewLanguage := strings.TrimSpace(opts.Language)
	if reviewLanguage == "" {
		reviewLanguage = "en-US"
	}

	// Create token budget
	budget := NewTokenBudget(b.estimator, opts.MaxTokens, opts.ReserveReply)

	// Build context segments
	segments := budget.BuildContextSegments(diff, b.languageDetector)

	// AUR-467: prose (documentation-classified) segments never compete
	// for the code token budget and never reach the code rule-catalog
	// content -- see filetype.go for why, and coverage.go for how their
	// exclusion is declared rather than silent. codeSegments preserves
	// every other behavior (priority, ordering, truncation) unchanged.
	codeSegments := make([]ContextSegment, 0, len(segments))
	for _, s := range segments {
		if !s.IsProse {
			codeSegments = append(codeSegments, s)
		}
	}
	codePaths, prosePaths := splitFilesByProse(diff, b.languageDetector)
	totals := hunkTotals(diff)

	// Estimate base prompt tokens (system message + instructions,
	// including the rule catalog for a review)
	basePrompt, err := b.buildBasePrompt(opts.SchemaKind, metrics, opts.CIContext, reviewLanguage)
	if err != nil {
		return PromptParts{}, err
	}
	baseTokens := b.estimator.Estimate(basePrompt)

	// AUR-467 blocker 1 (post-ff64e18 adversarial review): the coverage
	// declaration appended below is CONTENT of the assembled prompt, so
	// its worst-case size must be reserved out of the content budget
	// BEFORE TrimToFit runs -- otherwise the declaration itself can push
	// the assembled prompt over MaxTokens, and worse as the budget gets
	// tighter, since a tighter budget omits more files and so grows the
	// declaration that reports the omissions. See coverage.go's
	// maxCoverageDeclarationTokens for why this reservation is a proven
	// upper bound, not a guess.
	coverageReserve := maxCoverageDeclarationTokens(codePaths, prosePaths, totals, b.estimator)

	// AC-002: the instructions, the coverage declaration's worst case, and
	// the reply reservation must leave room for at least some code. When
	// they do not, this fails loudly instead of letting TrimToFit drop
	// every diff segment and shipping a review prompt with no diff in it
	// -- a request the model can only answer by inventing findings.
	if opts.MaxTokens > 0 && baseTokens+coverageReserve+opts.ReserveReply >= opts.MaxTokens {
		return PromptParts{}, fmt.Errorf(
			"prompt instructions need %d tokens, the coverage declaration reserves %d, and the reply reserves %d, which does not fit the %d-token budget: refusing to assemble a %s prompt with no room for the code changes",
			baseTokens, coverageReserve, opts.ReserveReply, opts.MaxTokens, opts.SchemaKind)
	}

	// Trim segments to fit budget -- code segments only, budgeted against
	// baseTokens PLUS the reserved coverage declaration size; prose never
	// ate into this budget in the first place, which is also the fix for
	// the measured ordering starvation (see filetype.go).
	trimmedSegments := budget.TrimToFit(codeSegments, baseTokens+coverageReserve)

	// Build user content from trimmed segments, plus AC-003's coverage
	// declaration (coverage.go): every code file classified complete,
	// partial (named, with its hunk fraction -- AUR-467 blocker 2: a file
	// missing even one hunk to the budget is never silently "reviewed"),
	// or omitted, and which documentation files were excluded from the
	// code rule catalog.
	userContent := b.buildUserContent(trimmedSegments, metrics, opts.CIContext)
	covered := coveredHunkCounts(trimmedSegments)
	coverages := classifyCodeCoverage(codePaths, totals, covered)
	userContent += "\n" + renderCoverageDeclaration(coverages, prosePaths)

	var completeCount, partialCount, omittedCount int
	for _, c := range coverages {
		switch c.state() {
		case "complete":
			completeCount++
		case "partial":
			partialCount++
		default:
			omittedCount++
		}
	}

	parts := PromptParts{
		System: basePrompt,
		User:   userContent,
		Meta: map[string]string{
			"schema_kind":    opts.SchemaKind,
			"role":           opts.Role,
			"segments_total": fmt.Sprintf("%d", len(segments)),
			"segments_used":  fmt.Sprintf("%d", len(trimmedSegments)),
			// AUR-467 blocker 1: estimated from the ACTUAL final System+User
			// text this function returns, not a partial sum of its pieces --
			// so it can never undercount what the declaration itself added.
			"estimated_tokens":     fmt.Sprintf("%d", b.estimator.Estimate(basePrompt+userContent)),
			"code_files_total":     fmt.Sprintf("%d", len(codePaths)),
			"code_files_complete":  fmt.Sprintf("%d", completeCount),
			"code_files_partial":   fmt.Sprintf("%d", partialCount),
			"code_files_omitted":   fmt.Sprintf("%d", omittedCount),
			"prose_files_excluded": fmt.Sprintf("%d", len(prosePaths)),
		},
	}

	return parts, nil
}

// buildBasePrompt creates the system prompt based on schema kind. For
// "review" it uses the full instructional prompt embedded from
// .aurumcode/prompts/review.md (see templates/review.md and
// docs/specs/AUR-430.md for why it is a build-time copy rather than a
// runtime read of that read-only path), with its own Metrics/Languages
// sections filled in; DiffContent is deliberately left as a pointer rather
// than the unbudgeted full diff, because BuildPrompt's caller receives the
// actual, token-budgeted code changes separately in PromptParts.User (see
// buildUserContent) -- rendering the whole diff into both halves would
// double the token cost for nothing. The other schema kinds keep the
// original engine's short inline instructions, unchanged.
func (b *PromptBuilder) buildBasePrompt(schemaKind string, metrics *analyzer.DiffMetrics, ciContext, reviewLanguage string) (string, error) {
	switch schemaKind {
	case "review":
		if tmpl, ok := b.templates["review.md"]; ok {
			// The rule catalog is assembled BEFORE the template runs and
			// its error is returned, never swallowed: a review prompt
			// that reached the model without the closed list is this
			// card's defect, and one that reached it with a truncated
			// list would be the same defect wearing a catalog id the gate
			// still discards.
			catalog, err := b.ruleCatalogSection()
			if err != nil {
				return "", err
			}
			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, map[string]interface{}{
				"Metrics":        b.formatMetrics(metrics),
				"Languages":      b.formatLanguages(metrics),
				"DiffContent":    "(see the Code Changes section that follows this prompt)",
				"CIContext":      reviewCIContext(ciContext),
				"RuleCatalog":    catalog,
				"ReviewLanguage": reviewLanguage,
			}); err != nil {
				return "", fmt.Errorf("rendering the review prompt template: %w", err)
			}
			return buf.String(), nil
		}
		// No silent one-line fallback for a review. The old fallback
		// returned "You are an expert code reviewer..." with a nil error:
		// no response schema, no rule_id instruction and no catalog, which
		// is a stronger form of the very defect this card fixes -- the
		// model would have to invent both the shape and the ids. A missing
		// or unparseable template (see loadTemplates) is an assembly
		// failure, announced.
		return "", fmt.Errorf("the review prompt template is unavailable: refusing to send a review request without the response schema and the rule catalog")
	case "test":
		return "You are an expert test engineer. Generate comprehensive tests for the following code changes.", nil
	case "docs":
		return "You are a technical documentation expert. Generate documentation for the following code changes.", nil
	case "summary":
		return "Summarize the following code changes in 2-3 sentences.", nil
	default:
		return "You are a helpful code analysis assistant.", nil
	}
}

// buildUserContent assembles user content from segments
func (b *PromptBuilder) buildUserContent(segments []ContextSegment, metrics *analyzer.DiffMetrics, ciContext string) string {
	var result strings.Builder

	// Add metrics summary
	result.WriteString("## Change Summary\n")
	result.WriteString(fmt.Sprintf("- Total files: %d\n", metrics.TotalFiles))
	result.WriteString(fmt.Sprintf("- Lines added: %d\n", metrics.LinesAdded))
	result.WriteString(fmt.Sprintf("- Lines deleted: %d\n\n", metrics.LinesDeleted))
	result.WriteString("## Existing CI Context\n")
	result.WriteString(reviewCIContext(ciContext))
	result.WriteString("\n\n")

	// Add code changes
	result.WriteString("## Code Changes\n\n")
	for _, segment := range segments {
		result.WriteString(segment.Content)
		result.WriteString("\n")
	}

	return result.String()
}

func reviewCIContext(value string) string {
	if strings.TrimSpace(value) == "" {
		return "No CI failure context was supplied. Do not invent CI failures or claim that checks passed."
	}
	return value
}
