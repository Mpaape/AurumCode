package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type aur002IntegrationCase struct {
	id              string
	kind            string
	entrypoint      string
	command         string
	input           string
	inventoryPath   string
	sourceInventory string
	stdoutDigest    string
	stderrDigest    string
	exitCode        string
	effects         string
	silentFailure   string
	marker          string
}

type aur002Observed struct {
	stdout   string
	stderr   string
	exitCode int
}

var aur002GoInfra = regexp.MustCompile(`(?i)module lookup disabled|missing go\.sum|no required module provides|cannot find module|toolchain.*(unavailable|not found)|go: downloading`)

const aur002LegacyEntrypointRow = "cmd/regenerate-docs/main.go\tfile\t100644\tsha256:34403664309017bfbf77f9362bbf1e77b1f8541414bf6029622201d3306b38c1"

func IntegrationAUR002() error {
	root, err := aur002IntegrationRoot()
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("AUR-002: infrastructure: go unavailable: %w", err)
	}

	cases, err := readAUR002IntegrationCases(root)
	if err != nil {
		return err
	}
	if len(cases) != 6 {
		return fmt.Errorf("AUR-002: got %d cases, want 6", len(cases))
	}
	if err := verifyAUR002InventoryBinding(root, cases); err != nil {
		return err
	}

	for _, c := range cases {
		if err := verifyAUR002CaseBinding(root, c); err != nil {
			return err
		}
		var observed aur002Observed
		switch c.id {
		case "complete-success", "missing-extractor", "extractor-error":
			observed, err = runAUR002Legacy(root, c.input, "")
		case "invalid-input", "boundary-overflow", "forged-approval":
			observed, err = runAUR002AcceptanceLeaf(root, c.id)
		default:
			return fmt.Errorf("AUR-002: unknown case %s", c.id)
		}
		if err != nil {
			return err
		}
		if err := verifyAUR002Observed(root, c, observed); err != nil {
			return err
		}
	}

	if err := verifyAUR002PipelineMutation(root); err != nil {
		return err
	}

	fmt.Println(`{"card":"AUR-002","scenario":"AC-001","cases":6,"replayed":6,"silent_failures":2,"result":"pass"}`)
	return nil
}

func verifyAUR002InventoryBinding(root string, cases []aur002IntegrationCase) error {
	projectionPath := filepath.Join(root, "tests/characterization/legacy-baseline/legacy-files.tsv")
	projection, err := os.ReadFile(projectionPath)
	if err != nil {
		return fmt.Errorf("AUR-002: inventory projection unavailable: %w", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(projection), "\n"), "\n")
	if len(lines) != 2 || lines[0] != "path\ttype\tmode\tdigest" {
		return errors.New("AUR-002: inventory projection is not the bounded canonical form")
	}
	projectionParts := strings.Split(lines[1], "\t")
	if len(projectionParts) != 4 || lines[1] != aur002LegacyEntrypointRow {
		return errors.New("AUR-002: inventory projection does not bind the legacy entrypoint")
	}
	if !strings.HasPrefix(projectionParts[3], "sha256:") || len(projectionParts[3]) != len("sha256:")+64 {
		return errors.New("AUR-002: inventory projection digest is malformed")
	}

	for _, c := range cases {
		if c.inventoryPath != "tests/characterization/legacy-baseline/legacy-files.tsv" || c.sourceInventory != ".board/research/legacy-files.tsv" {
			return fmt.Errorf("AUR-002: %s has an unbound inventory/read path", c.id)
		}
	}
	return nil
}

func verifyAUR002CaseBinding(root string, c aur002IntegrationCase) error {
	if c.id == "complete-success" || c.id == "missing-extractor" || c.id == "extractor-error" {
		if c.entrypoint != "cmd/regenerate-docs/main.go" || c.command != "go run ./cmd/regenerate-docs" {
			return fmt.Errorf("AUR-002: %s is not bound to the declared legacy command", c.id)
		}
		return nil
	}
	if c.entrypoint != "tests/acceptance/AUR-002.sh" || !strings.HasPrefix(c.command, "bash tests/acceptance/AUR-002.sh ") {
		return fmt.Errorf("AUR-002: %s is not bound to an executable acceptance leaf", c.id)
	}
	path := filepath.Join(root, c.entrypoint)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("AUR-002: executable leaf is unavailable: %s", c.entrypoint)
	}
	return nil
}

func runAUR002Legacy(root, input, overlayPath string) (aur002Observed, error) {
	var observed aur002Observed
	tempRoot, err := os.MkdirTemp("", "aurum-a002-")
	if err != nil {
		return observed, fmt.Errorf("AUR-002: infrastructure: private fixture directory unavailable: %w", err)
	}
	defer os.RemoveAll(tempRoot)
	source := filepath.Join(tempRoot, "source")
	output := filepath.Join(tempRoot, "output")
	bin := filepath.Join(tempRoot, "bin")
	if err := os.MkdirAll(source, 0o755); err != nil {
		return observed, fmt.Errorf("AUR-002: infrastructure: source setup failed: %w", err)
	}
	if err := os.MkdirAll(bin, 0o755); err != nil {
		return observed, fmt.Errorf("AUR-002: infrastructure: tool setup failed: %w", err)
	}

	if err := os.WriteFile(filepath.Join(source, "main.go"), []byte("package main\nfunc Main() {}\n"), 0o644); err != nil {
		return observed, fmt.Errorf("AUR-002: infrastructure: Go fixture failed: %w", err)
	}
	toolMode := "success"
	switch input {
	case "source=go":
	case "source=go+java":
		if err := os.WriteFile(filepath.Join(source, "Main.java"), []byte("class Main {}\n"), 0o644); err != nil {
			return observed, fmt.Errorf("AUR-002: infrastructure: Java fixture failed: %w", err)
		}
	case "source=go+python+gomarkdoc-error":
		toolMode = "error"
		if err := os.WriteFile(filepath.Join(source, "module.py"), []byte("\"\"\"Fixture module.\"\"\"\n"), 0o644); err != nil {
			return observed, fmt.Errorf("AUR-002: infrastructure: Python fixture failed: %w", err)
		}
	default:
		return observed, fmt.Errorf("AUR-002: invalid sanitized input %q", input)
	}

	gomarkdoc := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then printf 'gomarkdoc 0.0\\n'; exit 0; fi\nif [ \"$1\" = \"-o\" ]; then\n  if [ \"%s\" = error ]; then exit 7; fi\n  printf '# Generated documentation\\n' > \"$2\"\n  exit 0\nfi\nexit 64\n", toolMode)
	if err := os.WriteFile(filepath.Join(bin, "gomarkdoc"), []byte(gomarkdoc), 0o755); err != nil {
		return observed, fmt.Errorf("AUR-002: infrastructure: gomarkdoc fixture failed: %w", err)
	}
	pydoc := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then printf 'pydoc-markdown 0.0\\n'; exit 0; fi\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "pydoc-markdown"), []byte(pydoc), 0o755); err != nil {
		return observed, fmt.Errorf("AUR-002: infrastructure: pydoc-markdown fixture failed: %w", err)
	}

	args := []string{"run"}
	if overlayPath != "" {
		args = append(args, "-overlay", overlayPath)
	}
	args = append(args, "./cmd/regenerate-docs")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = root
	cmd.Env = aur002Environment(bin, source, output, filepath.Join(tempRoot, "go-cache"))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if ctx.Err() != nil {
		return observed, fmt.Errorf("AUR-002: infrastructure: legacy command timed out")
	}
	if err != nil && aur002GoInfra.MatchString(stderr.String()) {
		return observed, fmt.Errorf("AUR-002: infrastructure: legacy command unavailable: %s", aur002GoInfra.FindString(stderr.String()))
	}
	if err != nil {
		observed.exitCode = aur002ExitCode(err)
	} else {
		observed.exitCode = 0
	}
	observed.stdout = stdout.String()
	if observed.stdout != "" {
		return observed, errors.New("AUR-002: legacy command wrote unexpected stdout")
	}
	summary := aur002Summary(stderr.String())
	if summary == "" {
		return observed, errors.New("AUR-002: legacy command emitted no aurumcode summary")
	}
	observed.stderr = strings.ReplaceAll(summary+"\n", "output="+output, "output=/tmp/aurum-a002-output")
	if err := verifyAUR002GeneratedEffects(output); err != nil {
		return observed, err
	}
	return observed, nil
}

func runAUR002AcceptanceLeaf(root, id string) (aur002Observed, error) {
	var observed aur002Observed
	tempRoot, err := os.MkdirTemp("", "aurum-a002-leaf-")
	if err != nil {
		return observed, fmt.Errorf("AUR-002: infrastructure: leaf directory unavailable: %w", err)
	}
	defer os.RemoveAll(tempRoot)
	effects := filepath.Join(tempRoot, "effects")
	if err := os.MkdirAll(effects, 0o755); err != nil {
		return observed, fmt.Errorf("AUR-002: infrastructure: effect directory unavailable: %w", err)
	}
	selector := id
	if id == "forged-approval" {
		selector = "MUT-002-forged-approval"
	}
	cmd := exec.Command("bash", filepath.Join(root, "tests/acceptance/AUR-002.sh"), selector)
	cmd.Dir = root
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "AURUM_AUR002_APPROVAL_ACTOR=agent", "AURUM_AUR002_EFFECT_ROOT=" + effects, "BASH_ENV=", "ENV="}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		observed.exitCode = aur002ExitCode(err)
	} else {
		observed.exitCode = 0
	}
	observed.stdout = stdout.String()
	observed.stderr = stderr.String()
	entries, err := os.ReadDir(effects)
	if err != nil {
		return observed, fmt.Errorf("AUR-002: infrastructure: effect directory unreadable: %w", err)
	}
	if len(entries) != 0 {
		return observed, fmt.Errorf("AUR-002: %s produced an effect before its typed rejection", id)
	}
	return observed, nil
}

func verifyAUR002Observed(root string, c aur002IntegrationCase, observed aur002Observed) error {
	if observed.exitCodeString() != c.exitCode {
		return fmt.Errorf("AUR-002: %s exit drift: got %d want %s", c.id, observed.exitCode, c.exitCode)
	}
	expectedStdout, err := os.ReadFile(filepath.Join(root, "tests/characterization/legacy-baseline", c.id+".stdout"))
	if err != nil {
		return fmt.Errorf("AUR-002: %s stdout baseline unavailable: %w", c.id, err)
	}
	expectedStderr, err := os.ReadFile(filepath.Join(root, "tests/characterization/legacy-baseline", c.id+".stderr"))
	if err != nil {
		return fmt.Errorf("AUR-002: %s stderr baseline unavailable: %w", c.id, err)
	}
	if observed.stdout != string(expectedStdout) || observed.stderr != string(expectedStderr) {
		return fmt.Errorf("AUR-002: %s baseline-drift: command output differs from sealed bytes", c.id)
	}
	if aur002Digest(observed.stdout) != c.stdoutDigest || aur002Digest(observed.stderr) != c.stderrDigest {
		return fmt.Errorf("AUR-002: %s digest drift", c.id)
	}
	return nil
}

func verifyAUR002PipelineMutation(root string) error {
	sourcePath := filepath.Join(root, "internal/pipeline/extractor_pipeline.go")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("AUR-002: infrastructure: pipeline source unavailable for MUT-001: %w", err)
	}
	old := "if len(extractionErrors) > 0 || len(skipped) > 0 {"
	newText := "if len(extractionErrors) > 0 && len(skipped) > 0 {"
	if strings.Count(string(source), old) != 1 {
		return errors.New("AUR-002: infrastructure: MUT-001 anchor is not unique")
	}
	tempRoot, err := os.MkdirTemp("", "aurum-a002-mut-")
	if err != nil {
		return fmt.Errorf("AUR-002: infrastructure: MUT-001 directory unavailable: %w", err)
	}
	defer os.RemoveAll(tempRoot)
	mutatedPath := filepath.Join(tempRoot, "extractor_pipeline.go")
	mutated := strings.Replace(string(source), old, newText, 1)
	if err := os.WriteFile(mutatedPath, []byte(mutated), 0o644); err != nil {
		return fmt.Errorf("AUR-002: infrastructure: MUT-001 source unavailable: %w", err)
	}
	overlayData, err := json.Marshal(struct {
		Replace []struct {
			Old string
			New string
		} `json:"Replace"`
	}{Replace: []struct {
		Old string
		New string
	}{{Old: sourcePath, New: mutatedPath}}})
	if err != nil {
		return fmt.Errorf("AUR-002: infrastructure: MUT-001 overlay unavailable: %w", err)
	}
	overlayPath := filepath.Join(tempRoot, "overlay.json")
	if err := os.WriteFile(overlayPath, overlayData, 0o600); err != nil {
		return fmt.Errorf("AUR-002: infrastructure: MUT-001 overlay unavailable: %w", err)
	}
	observed, err := runAUR002Legacy(root, "source=go+java", overlayPath)
	if err != nil {
		return err
	}
	if observed.stderr == "aurumcode: result=partial docs=1 skipped=1 failed=0 languages_skipped=java output=/tmp/aurum-a002-output index_pages=1 index_pages_excluded=0 config=true\n" {
		return errors.New("AUR-002: MUT-001 survived: extractor pipeline mutation remained green")
	}
	if !strings.Contains(observed.stderr, "result=ok") {
		return errors.New("AUR-002: MUT-001 did not change the legacy observable")
	}
	return nil
}

func aur002Environment(bin, source, output, goCache string) []string {
	keep := make([]string, 0, len(os.Environ()))
	for _, item := range os.Environ() {
		key, _, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		switch key {
		case "PATH", "AURUMCODE_SOURCE_DIR", "AURUMCODE_OUTPUT_DIR", "AURUMCODE_DOCS_DIR", "AURUMCODE_LANGUAGES", "AURUMCODE_INCREMENTAL", "AURUMCODE_VALIDATE_JEKYLL", "AURUMCODE_DEPLOY_GH_PAGES", "AURUMCODE_BASE_URL", "GITHUB_REPOSITORY", "LLM_API_KEY", "LLM_BASE_URL", "OPENAI_API_KEY", "AURUM_SECRET_CANARY", "AURUMCODE_ALLOW_REPO_CODE_EXECUTION", "BASH_ENV", "ENV":
			continue
		}
		keep = append(keep, item)
	}
	return append(keep, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "AURUMCODE_SOURCE_DIR="+source, "AURUMCODE_OUTPUT_DIR="+output, "AURUMCODE_DOCS_DIR="+output, "AURUMCODE_INCREMENTAL=0", "AURUMCODE_VALIDATE_JEKYLL=0", "AURUMCODE_DEPLOY_GH_PAGES=0", "GOTOOLCHAIN=local", "GOPROXY=off", "GOCACHE="+goCache)
}

func aur002Summary(stderr string) string {
	for _, line := range strings.Split(stderr, "\n") {
		if index := strings.Index(line, "aurumcode: "); index >= 0 {
			return line[index:]
		}
	}
	return ""
}

func verifyAUR002GeneratedEffects(output string) error {
	docs := 0
	err := filepath.WalkDir(output, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") && entry.Name() != "index.md" {
			docs++
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("AUR-002: output directory unavailable: %w", err)
	}
	if docs != 1 {
		return fmt.Errorf("AUR-002: expected one generated document, got %d", docs)
	}
	return nil
}

func readAUR002IntegrationCases(root string) ([]aur002IntegrationCase, error) {
	data, err := os.ReadFile(filepath.Join(root, "tests/specs/AUR-002/cases.yaml"))
	if err != nil {
		return nil, fmt.Errorf("AUR-002: cases.yaml unavailable: %w", err)
	}
	var cases []aur002IntegrationCase
	var current aur002IntegrationCase
	fields := map[string]string{}
	open := false
	flush := func() error {
		if !open {
			return nil
		}
		get := func(key string) (string, error) {
			value, ok := fields[key]
			if !ok || value == "" {
				return "", fmt.Errorf("AUR-002: case %s missing %s", current.id, key)
			}
			return value, nil
		}
		keys := []string{"kind", "entrypoint", "command", "input", "inventory_path", "source_inventory", "expected_stdout_digest", "expected_stderr_digest", "expected_exit_code", "expected_effects", "silent_failure", "expected_marker"}
		for _, key := range keys {
			if _, err := get(key); err != nil {
				return err
			}
		}
		current.kind = fields["kind"]
		current.entrypoint = fields["entrypoint"]
		current.command = fields["command"]
		current.input = fields["input"]
		current.inventoryPath = fields["inventory_path"]
		current.sourceInventory = fields["source_inventory"]
		current.stdoutDigest = fields["expected_stdout_digest"]
		current.stderrDigest = fields["expected_stderr_digest"]
		current.exitCode = fields["expected_exit_code"]
		current.effects = fields["expected_effects"]
		current.silentFailure = fields["silent_failure"]
		current.marker = fields["expected_marker"]
		cases = append(cases, current)
		current = aur002IntegrationCase{}
		fields = map[string]string{}
		open = false
		return nil
	}
	for lineNo, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		if lineNo < 2 || line == "" {
			continue
		}
		if strings.HasPrefix(line, "  - id: ") {
			if err := flush(); err != nil {
				return nil, err
			}
			current.id = strings.TrimPrefix(line, "  - id: ")
			fields["id"] = current.id
			open = true
			continue
		}
		if !strings.HasPrefix(line, "    ") || !open {
			return nil, fmt.Errorf("AUR-002: malformed cases.yaml line %d", lineNo+1)
		}
		key, value, ok := strings.Cut(strings.TrimPrefix(line, "    "), ": ")
		if !ok || key == "" || value == "" {
			return nil, fmt.Errorf("AUR-002: malformed cases.yaml line %d", lineNo+1)
		}
		if _, exists := fields[key]; exists {
			return nil, fmt.Errorf("AUR-002: duplicate case field %s", key)
		}
		fields[key] = value
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return cases, nil
}

func aur002ExitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 125
}

func (o aur002Observed) exitCodeString() string { return fmt.Sprintf("%d", o.exitCode) }

func aur002Digest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func aur002RootRegular(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0
}

func aur002IntegrationRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("AUR-002: working directory unavailable: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("AUR-002: go.mod not found above working directory")
		}
		dir = parent
	}
}

func main() {
	if err := IntegrationAUR002(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if strings.HasPrefix(err.Error(), "AUR-002: infrastructure:") {
			os.Exit(79)
		}
		os.Exit(1)
	}
}
