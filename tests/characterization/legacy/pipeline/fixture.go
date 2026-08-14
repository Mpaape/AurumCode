//go:build aur309_characterization

package pipelinecharacterization

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/incremental"
	"github.com/Mpaape/AurumCode/internal/documentation/normalizer"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
	"github.com/Mpaape/AurumCode/internal/documentation/welcome"
	"github.com/Mpaape/AurumCode/internal/llm"
	legacy "github.com/Mpaape/AurumCode/internal/pipeline"
)

type Observation struct {
	Entrypoint     string
	Classification string
	Effect         string
}

type aur309Extractor struct {
	validateCalls int
	extractCalls  int
}

func (*aur309Extractor) Language() extractors.Language { return extractors.LanguageGo }

func (extractor *aur309Extractor) Validate(context.Context) error {
	extractor.validateCalls++
	return nil
}

func (extractor *aur309Extractor) Extract(_ context.Context, request *extractors.ExtractRequest) (*extractors.ExtractResult, error) {
	extractor.extractCalls++
	if err := os.MkdirAll(request.OutputDir, 0o755); err != nil {
		return nil, err
	}
	doc := filepath.Join(request.OutputDir, "sample.md")
	if err := os.WriteFile(doc, []byte("# Sample\n"), 0o600); err != nil {
		return nil, err
	}
	return &extractors.ExtractResult{
		Stats: extractors.ExtractionStats{FilesProcessed: 1, DocsGenerated: 1},
		Files: []string{doc},
	}, nil
}

type aur309Runner struct {
	calls []string
}

func (runner *aur309Runner) Run(_ context.Context, command string, arguments []string, _ string, _ map[string]string) (string, error) {
	runner.calls = append(runner.calls, strings.TrimSpace(command+" "+strings.Join(arguments, " ")))
	return "", nil
}

func aur309Write(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func aur309Contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func aur309Reachable(name, effect string, observed bool) Observation {
	classification := "stub/unreachable"
	if observed {
		classification = "reachable"
	}
	return Observation{Entrypoint: name, Classification: classification, Effect: effect}
}

func Characterize(t *testing.T) []Observation {
	t.Helper()

	incremental.ResetTrace()
	normalizer.ResetTrace()
	site.ResetTrace()
	welcome.ResetTrace()

	root := t.TempDir()
	source := filepath.Join(root, "source")
	docs := filepath.Join(root, "docs")
	output := filepath.Join(docs, "generated")
	aur309Write(t, filepath.Join(source, "sample.go"), "package sample\n")
	aur309Write(t, filepath.Join(source, "README.md"), "# Fixture\n")

	runner := &aur309Runner{}
	orchestrator := &llm.Orchestrator{}
	config := &legacy.ExtractorPipelineConfig{
		SourceDir:       source,
		OutputDir:       output,
		DocsDir:         docs,
		GenerateWelcome: true,
		ValidateJekyll:  true,
		DeployGHPages:   true,
	}
	pipeline := legacy.NewExtractorPipeline(config, runner, orchestrator)
	constructed := pipeline != nil
	extractor := &aur309Extractor{}
	if err := pipeline.RegisterExtractor(extractor); err != nil {
		t.Fatalf("register extractor: %v", err)
	}

	var logged bytes.Buffer
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&logged)
	log.SetFlags(0)
	err := pipeline.Run(context.Background())
	log.SetOutput(previousWriter)
	log.SetFlags(previousFlags)
	if err != nil {
		t.Fatalf("full pipeline run: %v", err)
	}

	_, docErr := os.Stat(filepath.Join(output, "go", "sample.md"))
	_, indexErr := os.Stat(filepath.Join(docs, "index.md"))
	_, configErr := os.Stat(filepath.Join(docs, "_config.yml"))
	registeredAndInvoked := extractor.validateCalls == 1 && extractor.extractCalls == 1
	fullWalkObserved := registeredAndInvoked
	extractionObserved := registeredAndInvoked && docErr == nil && aur309Contains(normalizer.Trace(), "normalize")
	welcomeObserved := aur309Contains(welcome.Trace(), "generate")
	scaffoldObserved := aur309Contains(site.Trace(), "generate") && indexErr == nil && configErr == nil
	jekyllObserved := len(runner.calls) == 2 && runner.calls[0] == "bundle --version" && runner.calls[1] == "bundle exec jekyll build"
	deployBranchObserved := strings.Contains(logged.String(), "gh-pages deployment not yet implemented")
	deployCallObserved := false
	for _, call := range runner.calls {
		if strings.Contains(call, "gh-pages") || strings.Contains(call, "deploy") || strings.HasPrefix(call, "git ") {
			deployCallObserved = true
		}
	}

	incremental.ResetTrace()
	incrementalRoot := filepath.Join(root, "incremental")
	incrementalSource := filepath.Join(incrementalRoot, "source")
	incrementalDocs := filepath.Join(incrementalRoot, "docs")
	aur309Write(t, filepath.Join(incrementalSource, "sample.go"), "package sample\n")
	incrementalExtractor := &aur309Extractor{}
	incrementalPipeline := legacy.NewExtractorPipeline(&legacy.ExtractorPipelineConfig{
		SourceDir:   incrementalSource,
		OutputDir:   filepath.Join(incrementalDocs, "generated"),
		DocsDir:     incrementalDocs,
		Incremental: true,
	}, &aur309Runner{}, nil)
	if err := incrementalPipeline.RegisterExtractor(incrementalExtractor); err != nil {
		t.Fatalf("register incremental extractor: %v", err)
	}
	if err := incrementalPipeline.Run(context.Background()); err != nil {
		t.Fatalf("incremental pipeline run: %v", err)
	}
	incrementalEvents := incremental.Trace()
	incrementalObserved := true
	for _, expected := range []string{"load", "get", "register_documentation", "register_language", "update", "save"} {
		incrementalObserved = incrementalObserved && aur309Contains(incrementalEvents, expected)
	}

	skipRoot := filepath.Join(root, "skip")
	skipSource := filepath.Join(skipRoot, "source")
	aur309Write(t, filepath.Join(skipSource, "orphan.java"), "class Orphan {}\n")
	skipPipeline := legacy.NewExtractorPipeline(&legacy.ExtractorPipelineConfig{
		SourceDir: skipSource,
		OutputDir: filepath.Join(skipRoot, "docs", "generated"),
		DocsDir:   filepath.Join(skipRoot, "docs"),
	}, &aur309Runner{}, nil)
	skipErr := skipPipeline.Run(context.Background())
	var extractionError *legacy.ExtractionError
	extractionErrorObserved := errors.As(skipErr, &extractionError) && strings.Contains(skipErr.Error(), "produced no documentation")
	languageSkipObserved := extractionError != nil && len(extractionError.Skipped) == 1 && strings.Contains(extractionError.Skipped[0].String(), "no extractor registered")

	sentinel := errors.New("aur309 nested cause")
	unwrapped := &legacy.ExtractionError{Errors: []error{sentinel}}
	unwrapObserved := errors.Is(unwrapped, sentinel)

	observations := []Observation{
		aur309Reachable("ExtractionError.Error", "typed_error_rendered", extractionErrorObserved),
		aur309Reachable("ExtractionError.Unwrap", "nested_cause_observed", unwrapObserved),
		aur309Reachable("ExtractorPipeline.Run", "document_written", err == nil && docErr == nil),
		aur309Reachable("LanguageSkip.String", "bounded_skip_rendered", languageSkipObserved),
		aur309Reachable("NewExtractorPipeline", "pipeline_constructed", constructed),
		aur309Reachable("RegisterExtractor", "registered_extractor_invoked", registeredAndInvoked),
		{Entrypoint: "deployToGHPages", Classification: "reachable", Effect: "deploy_call_recorded"},
		aur309Reachable("determineFilesToProcess", "full_walk_and_incremental_query", fullWalkObserved && aur309Contains(incrementalEvents, "get")),
		aur309Reachable("extractDocumentation", "extractor_validate_and_extract", extractionObserved),
		aur309Reachable("generateWelcomePage", "welcome_generator_called", welcomeObserved),
		aur309Reachable("incrementalCache", "load_get_register_update_save", incrementalObserved),
		aur309Reachable("validateJekyllSite", "bundle_probe_and_build", jekyllObserved),
		aur309Reachable("writeSiteScaffold", "index_and_config_written", scaffoldObserved),
	}
	if deployBranchObserved && !deployCallObserved {
		observations[6].Classification = "stub/unreachable"
		observations[6].Effect = "placeholder_log_without_deploy_call"
	} else if !deployBranchObserved {
		observations[6].Classification = "stub/unreachable"
		observations[6].Effect = "branch_not_observed"
	}

	sort.Slice(observations, func(i, j int) bool {
		return observations[i].Entrypoint < observations[j].Entrypoint
	})
	return observations
}

func AssertFullyObserved(t *testing.T, observations []Observation) {
	t.Helper()
	if len(observations) != 13 {
		t.Fatalf("expected 13 entrypoints, got %d", len(observations))
	}
	seen := make(map[string]bool, len(observations))
	for _, observation := range observations {
		if observation.Entrypoint == "" || observation.Effect == "" {
			t.Fatalf("empty observation: %#v", observation)
		}
		if seen[observation.Entrypoint] {
			t.Fatalf("duplicate entrypoint %q", observation.Entrypoint)
		}
		seen[observation.Entrypoint] = true
		switch observation.Classification {
		case "reachable":
			if observation.Entrypoint == "deployToGHPages" {
				t.Fatal("deployment placeholder was incorrectly classified as functional")
			}
		case "stub/unreachable":
			if observation.Entrypoint != "deployToGHPages" || observation.Effect != "placeholder_log_without_deploy_call" {
				t.Fatalf("unexpected unobserved entrypoint: %#v", observation)
			}
		default:
			t.Fatalf("invalid classification %q", observation.Classification)
		}
	}
}

func PrintObservations(observations []Observation) {
	for _, observation := range observations {
		fmt.Printf("AUR309_OBSERVATION\t%s\t%s\t%s\n", observation.Entrypoint, observation.Classification, observation.Effect)
	}
}
