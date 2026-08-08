package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type aur002Case struct {
	id       string
	kind     string
	entry    string
	command  string
	input    string
	stdout   string
	stderr   string
	exitCode string
	effects  string
	silent   string
}

var aur002Digest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

var aur002IDs = []string{
	"complete-success",
	"missing-extractor",
	"extractor-error",
	"invalid-input",
	"boundary-overflow",
}

func TestAUR002() error {
	root, err := aur002Root()
	if err != nil {
		return err
	}
	cases, err := readAUR002Cases(root)
	if err != nil {
		return err
	}
	if len(cases) != len(aur002IDs) {
		return fmt.Errorf("AUR-002: got %d cases, want %d", len(cases), len(aur002IDs))
	}

	manifestPath := filepath.Join(root, "tests/characterization/legacy-baseline/manifest.tsv")
	manifest, err := readAUR002Manifest(manifestPath)
	if err != nil {
		return err
	}
	if len(manifest) != len(aur002IDs) {
		return fmt.Errorf("AUR-002: manifest has %d rows, want %d", len(manifest), len(aur002IDs))
	}

	inventory, err := os.ReadFile(filepath.Join(root, ".board/research/legacy-files.tsv"))
	if err != nil {
		return fmt.Errorf("AUR-002: legacy inventory unavailable: %w", err)
	}
	if !strings.Contains(string(inventory), "cmd/regenerate-docs/main.go\t") {
		return errors.New("AUR-002: executable entrypoint is absent from the AUR-001 inventory")
	}

	silent := 0
	for i, c := range cases {
		wantID := aur002IDs[i]
		if c.id != wantID {
			return fmt.Errorf("AUR-002: case %d is %q, want %q", i+1, c.id, wantID)
		}
		if c.entry != "cmd/regenerate-docs/main.go" || !aur002Digest.MatchString(c.stdout) || !aur002Digest.MatchString(c.stderr) {
			return fmt.Errorf("AUR-002: %s has an invalid entrypoint or digest", c.id)
		}
		if c.silent == "true" {
			silent++
		}
		row, ok := manifest[c.id]
		if !ok {
			return fmt.Errorf("AUR-002: manifest omits %s", c.id)
		}
		if row.stdoutPath != c.id+".stdout" || row.stderrPath != c.id+".stderr" {
			return fmt.Errorf("AUR-002: %s manifest stream paths drift", c.id)
		}
		if row.exitCode != c.exitCode || row.effects != c.effects {
			return fmt.Errorf("AUR-002: %s manifest outcome drifts from cases.yaml", c.id)
		}
		if row.marker == "silent-failure" && c.silent != "true" {
			return fmt.Errorf("AUR-002: %s has an unbound silent-failure marker", c.id)
		}

		stdoutPath := filepath.Join(root, "tests/characterization/legacy-baseline", row.stdoutPath)
		stderrPath := filepath.Join(root, "tests/characterization/legacy-baseline", row.stderrPath)
		stdoutDigest, err := aur002FileDigest(stdoutPath)
		if err != nil {
			return err
		}
		stderrDigest, err := aur002FileDigest(stderrPath)
		if err != nil {
			return err
		}
		if stdoutDigest != c.stdout || stderrDigest != c.stderr {
			return fmt.Errorf("AUR-002: %s replay digest mismatch", c.id)
		}
	}
	if silent != 2 {
		return fmt.Errorf("AUR-002: got %d silent failures, want 2", silent)
	}

	return nil
}

type aur002ManifestRow struct {
	stdoutPath string
	stderrPath string
	exitCode   string
	effects    string
	marker     string
}

func readAUR002Cases(root string) ([]aur002Case, error) {
	file, err := os.Open(filepath.Join(root, "tests/specs/AUR-002/cases.yaml"))
	if err != nil {
		return nil, fmt.Errorf("AUR-002: cases.yaml unavailable: %w", err)
	}
	defer file.Close()

	var cases []aur002Case
	fields := map[string]string{}
	open := false
	flush := func() error {
		if !open {
			return nil
		}
		get := func(key string) (string, error) {
			value, ok := fields[key]
			if !ok || value == "" {
				return "", fmt.Errorf("AUR-002: case is missing %s", key)
			}
			return value, nil
		}
		id, err := get("id")
		if err != nil {
			return err
		}
		values := []string{"kind", "entrypoint", "command", "input", "expected_stdout_digest", "expected_stderr_digest", "expected_exit_code", "expected_effects", "silent_failure"}
		for _, key := range values {
			if _, err := get(key); err != nil {
				return err
			}
		}
		cases = append(cases, aur002Case{
			id:       id,
			kind:     fields["kind"],
			entry:    fields["entrypoint"],
			command:  fields["command"],
			input:    fields["input"],
			stdout:   fields["expected_stdout_digest"],
			stderr:   fields["expected_stderr_digest"],
			exitCode: fields["expected_exit_code"],
			effects:  fields["expected_effects"],
			silent:   fields["silent_failure"],
		})
		fields = map[string]string{}
		open = false
		return nil
	}

	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if lineNo == 1 {
			if line != "version: 1" {
				return nil, fmt.Errorf("AUR-002: invalid cases.yaml version")
			}
			continue
		}
		if lineNo == 2 {
			if line != "cases:" {
				return nil, errors.New("AUR-002: cases.yaml is missing cases:")
			}
			continue
		}
		if strings.HasPrefix(line, "  - id: ") {
			if err := flush(); err != nil {
				return nil, err
			}
			fields["id"] = strings.TrimPrefix(line, "  - id: ")
			open = true
			continue
		}
		if strings.HasPrefix(line, "    ") {
			if !open {
				return nil, fmt.Errorf("AUR-002: field outside case at line %d", lineNo)
			}
			body := strings.TrimPrefix(line, "    ")
			key, value, ok := strings.Cut(body, ": ")
			if !ok || key == "" || value == "" {
				return nil, fmt.Errorf("AUR-002: malformed field at line %d", lineNo)
			}
			if _, exists := fields[key]; exists {
				return nil, fmt.Errorf("AUR-002: duplicate field %s", key)
			}
			fields[key] = value
			continue
		}
		return nil, fmt.Errorf("AUR-002: malformed cases.yaml line %d", lineNo)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("AUR-002: cases.yaml read failed: %w", err)
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return cases, nil
}

func readAUR002Manifest(path string) (map[string]aur002ManifestRow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("AUR-002: manifest unavailable: %w", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) < 2 || lines[0] != "id\tstdout_path\tstderr_path\texit_code\teffects\tmarker" {
		return nil, errors.New("AUR-002: invalid manifest header")
	}
	rows := make(map[string]aur002ManifestRow)
	for lineNo, line := range lines[1:] {
		parts := strings.Split(line, "\t")
		if len(parts) != 6 || parts[0] == "" {
			return nil, fmt.Errorf("AUR-002: invalid manifest row %d", lineNo+2)
		}
		if _, exists := rows[parts[0]]; exists {
			return nil, fmt.Errorf("AUR-002: duplicate manifest row %s", parts[0])
		}
		rows[parts[0]] = aur002ManifestRow{
			stdoutPath: parts[1],
			stderrPath: parts[2],
			exitCode:   parts[3],
			effects:    parts[4],
			marker:     parts[5],
		}
	}
	return rows, nil
}

func aur002FileDigest(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("AUR-002: replay unavailable: %s: %w", path, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("AUR-002: replay is not a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("AUR-002: replay unreadable: %s: %w", path, err)
	}
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func aur002Root() (string, error) {
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
	if err := TestAUR002(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(`{"card":"AUR-002","layer":"unit","result":"pass"}`)
}
