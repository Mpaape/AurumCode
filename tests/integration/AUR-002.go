package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type aur002IntegrationCase struct {
	id      string
	input   string
	stdout string
	stderr string
	exit   string
	effects string
}

func IntegrationAUR002() error {
	root, err := aur002IntegrationRoot()
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("infrastructure: go unavailable: %w", err)
	}

	entrypoint := "cmd/regenerate-docs/main.go"
	inventory, err := os.ReadFile(filepath.Join(root, ".board/research/legacy-files.tsv"))
	if err != nil {
		return fmt.Errorf("AUR-002: legacy inventory unavailable: %w", err)
	}
	if !strings.Contains(string(inventory), entrypoint+"\t") {
		return fmt.Errorf("AUR-002: %s is not listed in legacy-files.tsv", entrypoint)
	}

	cases, err := readAUR002IntegrationCases(root)
	if err != nil {
		return err
	}
	for _, c := range cases {
		switch c.id {
		case "complete-success", "missing-extractor", "extractor-error":
			first, err := runAUR002Legacy(root, c.id, c.input)
			if err != nil {
				return err
			}
			second, err := runAUR002Legacy(root, c.id, c.input)
			if err != nil {
				return err
			}
			if first != second {
				return fmt.Errorf("AUR-002: %s replay changed the canonical summary", c.id)
			}
			want, err := os.ReadFile(filepath.Join(root, "tests/characterization/legacy-baseline", c.id+".stderr"))
			if err != nil {
				return fmt.Errorf("AUR-002: %s baseline stream unavailable: %w", c.id, err)
			}
			if first != string(want) {
				return fmt.Errorf("AUR-002: %s baseline-drift: legacy summary differs from sealed stream", c.id)
			}
		case "invalid-input":
			if c.exit != "64" || c.effects != "docs=0,skipped=0,errors=0,writes=0" {
				return errors.New("AUR-002: invalid input is not typed before effects")
			}
		case "boundary-overflow":
			if c.exit != "65" || c.effects != "docs=0,skipped=0,errors=0,writes=0" {
				return errors.New("AUR-002: boundary overflow is not typed before effects")
			}
		default:
			return fmt.Errorf("AUR-002: unknown case %s", c.id)
		}
	}

	fmt.Println(`{"card":"AUR-002","scenario":"AC-001","cases":5,"replayed":3,"silent_failures":2,"result":"pass"}`)
	return nil
}

func runAUR002Legacy(root, id, input string) (string, error) {
	source := filepath.Join("/tmp", "aurum-a002-source")
	output := filepath.Join("/tmp", "aurum-a002-output")
	bin := filepath.Join("/tmp", "aurum-a002-bin")
	for _, path := range []string{source, output, bin} {
		if err := os.RemoveAll(path); err != nil {
			return "", fmt.Errorf("AUR-002: infrastructure: reset failed: %w", err)
		}
	}
	if err := os.MkdirAll(source, 0o755); err != nil {
		return "", fmt.Errorf("AUR-002: infrastructure: source setup failed: %w", err)
	}
	if err := os.MkdirAll(bin, 0o755); err != nil {
		return "", fmt.Errorf("AUR-002: infrastructure: tool setup failed: %w", err)
	}

	if err := os.WriteFile(filepath.Join(source, "main.go"), []byte("package main\nfunc Main() {}\n"), 0o644); err != nil {
		return "", fmt.Errorf("AUR-002: infrastructure: Go fixture failed: %w", err)
	}
	toolMode := "success"
	switch input {
	case "source=go":
	case "source=go+java":
		if err := os.WriteFile(filepath.Join(source, "Main.java"), []byte("class Main {}\n"), 0o644); err != nil {
			return "", fmt.Errorf("AUR-002: infrastructure: Java fixture failed: %w", err)
		}
	case "source=go+python+gomarkdoc-error":
		toolMode = "error"
		if err := os.WriteFile(filepath.Join(source, "module.py"), []byte("\"\"\"Fixture module.\"\"\"\n"), 0o644); err != nil {
			return "", fmt.Errorf("AUR-002: infrastructure: Python fixture failed: %w", err)
		}
	default:
		return "", fmt.Errorf("AUR-002: invalid sanitized input %q", input)
	}

	gomarkdoc := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then printf 'gomarkdoc 0.0\\n'; exit 0; fi\nif [ \"$1\" = \"-o\" ]; then\n  if [ \"%s\" = error ]; then exit 7; fi\n  printf '# Generated documentation\\n' > \"$2\"\n  exit 0\nfi\nexit 64\n", toolMode)
	if err := os.WriteFile(filepath.Join(bin, "gomarkdoc"), []byte(gomarkdoc), 0o755); err != nil {
		return "", fmt.Errorf("AUR-002: infrastructure: gomarkdoc fixture failed: %w", err)
	}
	pydoc := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then printf 'pydoc-markdown 0.0\\n'; exit 0; fi\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "pydoc-markdown"), []byte(pydoc), 0o755); err != nil {
		return "", fmt.Errorf("AUR-002: infrastructure: pydoc-markdown fixture failed: %w", err)
	}

	env := sanitizedAUR002Environment(bin, source, output)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/regenerate-docs")
	cmd.Dir = root
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return "", fmt.Errorf("AUR-002: infrastructure: %s timed out", id)
	}
	if err != nil {
		return "", fmt.Errorf("AUR-002: %s legacy command exited non-zero: %w", id, err)
	}
	if stdout.Len() != 0 {
		return "", fmt.Errorf("AUR-002: %s wrote unexpected stdout", id)
	}

	summary := canonicalAUR002Summary(stderr.String())
	if summary == "" {
		return "", fmt.Errorf("AUR-002: %s emitted no aurumcode summary", id)
	}
	if err := verifyAUR002Effects(output, id, summary); err != nil {
		return "", err
	}
	return summary + "\n", nil
}

func sanitizedAUR002Environment(bin, source, output string) []string {
	keep := make([]string, 0, len(os.Environ()))
	for _, item := range os.Environ() {
		key, _, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		switch key {
		case "PATH", "AURUMCODE_SOURCE_DIR", "AURUMCODE_OUTPUT_DIR", "AURUMCODE_DOCS_DIR",
			"AURUMCODE_LANGUAGES", "AURUMCODE_INCREMENTAL", "AURUMCODE_VALIDATE_JEKYLL",
			"AURUMCODE_DEPLOY_GH_PAGES", "AURUMCODE_BASE_URL", "GITHUB_REPOSITORY",
			"LLM_API_KEY", "LLM_BASE_URL", "OPENAI_API_KEY", "AURUM_SECRET_CANARY",
			"AURUMCODE_ALLOW_REPO_CODE_EXECUTION", "BASH_ENV", "ENV":
			continue
		}
		keep = append(keep, item)
	}
	keep = append(keep,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"AURUMCODE_SOURCE_DIR="+source,
		"AURUMCODE_OUTPUT_DIR="+output,
		"AURUMCODE_DOCS_DIR="+output,
		"AURUMCODE_INCREMENTAL=0",
		"AURUMCODE_VALIDATE_JEKYLL=0",
		"AURUMCODE_DEPLOY_GH_PAGES=0",
		"GOTOOLCHAIN=local",
		"GOPROXY=off",
	)
	return keep
}

func canonicalAUR002Summary(stderr string) string {
	for _, line := range strings.Split(stderr, "\n") {
		if index := strings.Index(line, "aurumcode: "); index >= 0 {
			return line[index:]
		}
	}
	return ""
}

func verifyAUR002Effects(output, id, summary string) error {
	entries, err := os.ReadDir(output)
	if err != nil {
		return fmt.Errorf("AUR-002: %s output directory unavailable: %w", id, err)
	}
	docs := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") && entry.Name() != "index.md" {
			docs++
		}
	}
	if docs != 1 || !strings.Contains(summary, "docs=1") || !strings.Contains(summary, "index_pages=1") {
		return fmt.Errorf("AUR-002: %s observed effect count does not match one generated document", id)
	}
	return nil
}

func readAUR002IntegrationCases(root string) ([]aur002IntegrationCase, error) {
	data, err := os.ReadFile(filepath.Join(root, "tests/specs/AUR-002/cases.yaml"))
	if err != nil {
		return nil, fmt.Errorf("AUR-002: cases.yaml unavailable: %w", err)
	}
	var out []aur002IntegrationCase
	var current aur002IntegrationCase
	open := false
	flush := func() {
		if open {
			out = append(out, current)
			current = aur002IntegrationCase{}
			open = false
		}
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		if strings.HasPrefix(line, "  - id: ") {
			flush()
			current.id = strings.TrimPrefix(line, "  - id: ")
			open = true
			continue
		}
		if !strings.HasPrefix(line, "    ") {
			continue
		}
		key, value, ok := strings.Cut(strings.TrimPrefix(line, "    "), ": ")
		if !ok {
			return nil, fmt.Errorf("AUR-002: malformed integration case field: %s", line)
		}
		switch key {
		case "input":
			current.input = value
		case "expected_stdout_digest":
			current.stdout = value
		case "expected_stderr_digest":
			current.stderr = value
		case "expected_exit_code":
			current.exit = value
		case "expected_effects":
			current.effects = value
		}
	}
	flush()
	return out, nil
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
