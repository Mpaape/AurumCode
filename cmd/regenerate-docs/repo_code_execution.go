package main

import (
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	csharpExtractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/csharp"
	rustExtractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/rust"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

// Two of the language extractors cannot produce documentation without running
// the consumer's build system, and both build systems execute code that lives in
// the repository being documented:
//
//   - cargo doc compiles the crate. Compiling a crate EXECUTES its build.rs (and
//     the build.rs of every dependency) and EXECUTES proc-macro crates while
//     expanding macros. Neither --no-deps nor --offline changes that: they limit
//     what is documented and what is downloaded, not what is executed. cargo has
//     no flag that disables build scripts. cargo metadata avoids execution but
//     produces manifest data, not documentation.
//   - dotnet build runs MSBuild, which EXECUTES the <Exec> tasks, the custom
//     <UsingTask> assemblies, and the .targets/.props imported by the .csproj -
//     including files restored from NuGet. The XML documentation file only
//     exists after a compile, so there is no MSBuild-free path to it, and
//     xmldocmd afterwards loads the produced assembly by reflection, which runs
//     its module initializers.
//
// So there is no "safe mode" to fall back to, and the decision is fail-closed:
// this binary does not register them unless the consumer names them. Leaving
// them on by default means installing this action on a public repository lets
// any pull request run arbitrary code on the maintainer's runner - the workflow
// checks the contributor's branch out first, and the runner image already ships
// cargo.
//
// A consumer who owns every branch the workflow runs on can opt in per language;
// the opt-in is deliberately a list of languages rather than a boolean, so
// enabling Rust never silently enables C# as well.
const envAllowRepoCodeExecution = "AURUMCODE_ALLOW_REPO_CODE_EXECUTION"

// repoCodeExecutingExtractor is an extractor whose external tool executes code
// from the tree it documents.
type repoCodeExecutingExtractor struct {
	// token is what the consumer writes in envAllowRepoCodeExecution.
	token string
	// language is the extractor's language.
	language extractors.Language
	// executes names the repository-controlled code the tool runs, so the
	// warning tells the consumer what they are accepting. It is taken from the
	// extractor's own package, next to the line that launches the tool, so the
	// warning cannot drift away from the behaviour it describes.
	executes string
	// build constructs the extractor. It is only called once the opt-in for this
	// entry has been parsed and accepted.
	build func(site.CommandRunner) extractors.Extractor
}

// repoCodeExecutingExtractors is the complete list of extractors held back from
// the default registration. Adding an entry here is how a code-executing
// extractor stays out of the default path; adding one to registerLanguageExtractors
// instead is the bug this list exists to prevent.
var repoCodeExecutingExtractors = []repoCodeExecutingExtractor{
	{
		token:    "rust",
		language: extractors.LanguageRust,
		executes: rustExtractor.ExecutesRepositoryCode,
		build: func(runner site.CommandRunner) extractors.Extractor {
			return rustExtractor.NewRustExtractor(runner)
		},
	},
	{
		token:    "csharp",
		language: extractors.LanguageCSharp,
		executes: csharpExtractor.ExecutesRepositoryCode,
		build: func(runner site.CommandRunner) extractors.Extractor {
			return csharpExtractor.NewCSharpExtractor(runner)
		},
	},
}

// safeTokenPattern bounds what an unrecognised opt-in value may be echoed as.
// The variable is consumer-controlled and a misconfigured workflow could point
// it at anything, so a value that does not look like a language name is
// summarised instead of printed.
var safeTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_.+#-]{1,32}$`)

func describeToken(token string) string {
	if safeTokenPattern.MatchString(token) {
		return fmt.Sprintf("%q", token)
	}
	return fmt.Sprintf("<%d unprintable character(s)>", len(token))
}

// parseRepoCodeExecutionOptIn turns the raw environment value into the set of
// enabled tokens.
//
// It fails closed in both directions: an empty or unset value enables nothing,
// and an unrecognised value is an error rather than a silent no-op. Silently
// ignoring a typo would leave a consumer who believes they enabled Rust with a
// run that skips it - and, worse, the mirror-image mistake would let a value
// like "all" or "true" be read as "enable everything" by a later change.
func parseRepoCodeExecutionOptIn(raw string) (map[string]bool, error) {
	known := make(map[string]bool, len(repoCodeExecutingExtractors))
	for _, entry := range repoCodeExecutingExtractors {
		known[entry.token] = true
	}

	enabled := make(map[string]bool)
	for _, part := range strings.Split(raw, ",") {
		token := strings.ToLower(strings.TrimSpace(part))
		if token == "" {
			continue
		}

		if !known[token] {
			return nil, fmt.Errorf(
				"%s: unsupported value %s; accepted values are %s (comma-separated), and each one allows the named toolchain to execute code from the documented repository",
				envAllowRepoCodeExecution, describeToken(token), strings.Join(knownTokens(), ", "))
		}

		enabled[token] = true
	}

	return enabled, nil
}

func knownTokens() []string {
	tokens := make([]string, 0, len(repoCodeExecutingExtractors))
	for _, entry := range repoCodeExecutingExtractors {
		tokens = append(tokens, entry.token)
	}
	sort.Strings(tokens)

	return tokens
}

// registerRepoCodeExecutingExtractors registers only the extractors the consumer
// explicitly opted in to, and logs what each opt-in permits.
//
// The whole value is parsed before anything is registered, so a value naming one
// valid and one invalid language registers nothing at all instead of half of it.
func registerRepoCodeExecutingExtractors(
	register func(extractors.Extractor) error,
	runner site.CommandRunner,
	raw string,
) error {
	enabled, err := parseRepoCodeExecutionOptIn(raw)
	if err != nil {
		return err
	}

	if len(enabled) == 0 {
		log.Printf("ℹ️  %s extractors are disabled: their toolchains execute code from the documented repository. "+
			"Set %s to a comma-separated list (%s) to enable them, and only where every branch this runs on is trusted.",
			strings.Join(knownTokens(), "/"), envAllowRepoCodeExecution, strings.Join(knownTokens(), ","))
		return nil
	}

	for _, entry := range repoCodeExecutingExtractors {
		if !enabled[entry.token] {
			continue
		}

		log.Printf("⚠️  %s documentation enabled via %s: %s. Do not enable this on a repository that accepts untrusted pull requests.",
			entry.language, envAllowRepoCodeExecution, entry.executes)

		if err := register(entry.build(runner)); err != nil {
			return err
		}
	}

	return nil
}
