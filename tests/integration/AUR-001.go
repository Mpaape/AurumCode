// This harness is a standalone program invoked by its acceptance script as
// `go run tests/integration/AUR-001.go`, while every sibling in this directory belongs to the
// shared test package. Go allows one package per directory, so without this
// constraint the two clauses collide and `go build ./...` -- the whole CI
// build and the race suite -- fails before compiling a single line. The
// constraint excludes the file from package resolution; naming it explicitly
// on the command line still runs it.
//go:build ignore

package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	legacyInventory = ".board/research/legacy-files.tsv"
	legacyLedger    = ".board/research/legacy-disposition.md"
	legacyClaims    = "tests/specs/AUR-001/claims.yaml"
	maxInventoryRows = 10000
	maxInventoryBytes = 64 * 1024 * 1024
)

type inventoryRow struct {
	path   string
	typeID string
	mode   string
	digest string
}

type claimRecord struct {
	id         string
	status     string
	disposition string
	entrypoint string
	test       string
	reason     string
}

var sha256DigestRE = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// IntegrationAUR001 independently validates the card-owned metadata. It is a
// standalone integration selector because the card deliberately declares a
// .go program rather than reviving the former OCI/identity test ceremony.
func IntegrationAUR001(root string) error {
	rows, byPath, err := readInventory(root)
	if err != nil {
		return err
	}
	tracked, err := trackedPaths(root)
	if err != nil {
		return err
	}
	if len(rows) != len(tracked) {
		return fmt.Errorf("AUR-001: inventory has %d rows but tracked snapshot has %d paths", len(rows), len(tracked))
	}
	for path := range tracked {
		if _, ok := byPath[path]; !ok {
			return fmt.Errorf("AUR-001: inventory omits tracked path %s", path)
		}
	}
	for path := range byPath {
		if _, ok := tracked[path]; !ok {
			return fmt.Errorf("AUR-001: inventory contains untracked path %s", path)
		}
	}

	if err := verifyFiles(root, rows); err != nil {
		return err
	}
	rules, err := readDispositionRules(root)
	if err != nil {
		return err
	}
	if err := verifyDispositions(rows, rules); err != nil {
		return err
	}
	claims, err := readClaims(root)
	if err != nil {
		return err
	}
	if err := verifyClaims(root, byPath, claims); err != nil {
		return err
	}

	return nil
}

func main() {
	root, err := findRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := IntegrationAUR001(root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("{\"card\":\"AUR-001\",\"scenario\":\"AC-001\",\"result\":\"pass\"}\n")
}

func findRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("AUR-001: working directory unavailable: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("AUR-001: go.mod not found above working directory")
		}
		dir = parent
	}
}

func readInventory(root string) ([]inventoryRow, map[string]inventoryRow, error) {
	path := filepath.Join(root, legacyInventory)
	data, err := readRegularBounded(root, path, maxInventoryBytes)
	if err != nil {
		return nil, nil, err
	}
	if len(data) > maxInventoryBytes {
		return nil, nil, fmt.Errorf("AUR-001: inventory exceeds 64 MiB: %d bytes", len(data))
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) < 2 || lines[0] != "path\ttype\tmode\tdigest" {
		return nil, nil, errors.New("AUR-001: inventory header must be path/type/mode/digest")
	}
	rows := make([]inventoryRow, 0, len(lines)-2)
	byPath := make(map[string]inventoryRow)
	previous := ""
	for lineNo, line := range lines[1:] {
		if line == "" {
			if lineNo == len(lines)-2 {
				continue
			}
			return nil, nil, fmt.Errorf("AUR-001: empty inventory row at line %d", lineNo+2)
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 4 {
			return nil, nil, fmt.Errorf("AUR-001: inventory row %d has %d fields", lineNo+2, len(fields))
		}
		row := inventoryRow{path: fields[0], typeID: fields[1], mode: fields[2], digest: fields[3]}
		if err := validatePath(row.path); err != nil {
			return nil, nil, fmt.Errorf("AUR-001: inventory row %d: %w", lineNo+2, err)
		}
		if previous != "" && row.path <= previous {
			return nil, nil, fmt.Errorf("AUR-001: inventory order is not lexical at line %d", lineNo+2)
		}
		previous = row.path
		if _, exists := byPath[row.path]; exists {
			return nil, nil, fmt.Errorf("AUR-001: duplicate inventory path %s", row.path)
		}
		if row.typeID != "file" && row.typeID != "symlink" {
			return nil, nil, fmt.Errorf("AUR-001: unsupported inventory type %s", row.typeID)
		}
		if (row.typeID == "file" && row.mode != "100644" && row.mode != "100755") ||
			(row.typeID == "symlink" && row.mode != "120000") {
			return nil, nil, fmt.Errorf("AUR-001: inventory mode %s does not match type %s", row.mode, row.typeID)
		}
		if !sha256DigestRE.MatchString(row.digest) {
			return nil, nil, fmt.Errorf("AUR-001: invalid digest for %s", row.path)
		}
		if len(rows) == maxInventoryRows {
			return nil, nil, fmt.Errorf("AUR-001: inventory exceeds %d tracked paths", maxInventoryRows)
		}
		rows = append(rows, row)
		byPath[row.path] = row
	}
	if len(rows) == 0 {
		return nil, nil, errors.New("AUR-001: inventory declares no tracked file")
	}
	return rows, byPath, nil
}

func validatePath(path string) error {
	if path == "" || filepath.IsAbs(path) || strings.Contains(path, "\\") ||
		strings.Contains(path, "\x00") || strings.Contains(path, "\n") ||
		strings.Contains(path, "\r") {
		return fmt.Errorf("unsafe path %q", path)
	}
	for _, part := range strings.Split(path, "/") {
		if part == ".." {
			return fmt.Errorf("traversal path %q", path)
		}
	}
	return nil
}

func trackedPaths(root string) (map[string]bool, error) {
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = root
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("AUR-001: git ls-files failed: %w", err)
	}
	paths := make(map[string]bool)
	for _, raw := range strings.Split(string(data), "\x00") {
		if raw == "" || forbiddenPath(raw) {
			continue
		}
		paths[raw] = true
	}
	return paths, nil
}

func forbiddenPath(path string) bool {
	for _, prefix := range []string{".git", ".env", "credentials", "secrets", ".board/cards"} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func readRegular(root, path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("AUR-001: artifact unavailable: %s: %w", filepath.Rel(root, path), err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("AUR-001: artifact is not a regular file: %s", filepath.Rel(root, path))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("AUR-001: artifact unreadable: %s: %w", filepath.Rel(root, path), err)
	}
	return data, nil
}

func readRegularBounded(root, path string, maxBytes int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("AUR-001: artifact unavailable: %s: %w", filepath.Rel(root, path), err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("AUR-001: artifact is not a regular file: %s", filepath.Rel(root, path))
	}
	if info.Size() > maxBytes {
		return nil, fmt.Errorf("AUR-001: inventory exceeds 64 MiB: %d bytes", info.Size())
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("AUR-001: artifact unreadable: %s: %w", filepath.Rel(root, path), err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("AUR-001: artifact unreadable: %s: %w", filepath.Rel(root, path), err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("AUR-001: inventory exceeds 64 MiB: more than %d bytes", maxBytes)
	}
	return data, nil
}

func verifyFiles(root string, rows []inventoryRow) error {
	for _, row := range rows {
		path := filepath.Join(root, filepath.FromSlash(row.path))
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("AUR-001: inventory path is absent: %s", row.path)
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return fmt.Errorf("AUR-001: cannot resolve inventory path %s: %w", row.path, err)
		}
		rootResolved, err := filepath.EvalSymlinks(root)
		if err != nil {
			return fmt.Errorf("AUR-001: cannot resolve repository root: %w", err)
		}
		if !withinRoot(rootResolved, resolved) {
			return fmt.Errorf("AUR-001: inventory path resolves outside repository: %s", row.path)
		}
		if row.typeID == "symlink" {
			if info.Mode()&os.ModeSymlink == 0 {
				return fmt.Errorf("AUR-001: %s is not the declared symlink", row.path)
			}
			target, err := os.Readlink(path)
			if err != nil {
				return fmt.Errorf("AUR-001: symlink unreadable: %s", row.path)
			}
			actual := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(target)))
			if actual != row.digest {
				return fmt.Errorf("AUR-001: symlink digest mismatch: %s", row.path)
			}
			continue
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("AUR-001: %s is not the declared regular file", row.path)
		}
		mode := "100644"
		if info.Mode()&0o111 != 0 {
			mode = "100755"
		}
		if row.mode != mode {
			return fmt.Errorf("AUR-001: mode mismatch for %s: inventory %s, actual %s", row.path, row.mode, mode)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("AUR-001: file unreadable: %s", row.path)
		}
		data = canonicalDigestBytes(row.path, data)
		actual := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
		if actual != row.digest {
			return fmt.Errorf("AUR-001: digest mismatch: %s", row.path)
		}
	}
	return nil
}

func withinRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func canonicalDigestBytes(path string, data []byte) []byte {
	if path == legacyInventory {
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			fields := strings.Split(line, "\t")
			if len(fields) == 4 && fields[0] == legacyInventory {
				fields[3] = "sha256:" + strings.Repeat("0", 64)
				lines[i] = strings.Join(fields, "\t")
			}
		}
		return []byte(strings.Join(lines, "\n"))
	}
	return data
}

type dispositionRule struct {
	pattern string
	verb    string
}

func readDispositionRules(root string) ([]dispositionRule, error) {
	data, err := readRegular(root, filepath.Join(root, legacyLedger))
	if err != nil {
		return nil, err
	}
	rules := []dispositionRule{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- disposition: ") {
			continue
		}
		body := strings.TrimPrefix(line, "- disposition: ")
		parts := strings.Split(body, " -> ")
		if len(parts) != 2 || parts[0] == "" {
			return nil, fmt.Errorf("AUR-001: malformed disposition rule: %s", line)
		}
		if !map[string]bool{"keep": true, "migrate": true, "replace": true, "quarantine": true, "characterize": true, "delete": true, "absent": true}[parts[1]] {
			return nil, fmt.Errorf("AUR-001: unknown disposition verb: %s", parts[1])
		}
		rules = append(rules, dispositionRule{pattern: parts[0], verb: parts[1]})
	}
	if len(rules) == 0 {
		return nil, errors.New("AUR-001: disposition ledger declares no machine-readable rules")
	}
	return rules, nil
}

func verifyDispositions(rows []inventoryRow, rules []dispositionRule) error {
	hits := make([]int, len(rules))
	for _, row := range rows {
		matches := []int{}
		for i, rule := range rules {
			ok, err := globMatch(rule.pattern, row.path)
			if err != nil {
				return err
			}
			if ok {
				matches = append(matches, i)
			}
		}
		if len(matches) == 0 {
			return fmt.Errorf("AUR-001: no disposition rule covers %s", row.path)
		}
		if len(matches) > 1 {
			return fmt.Errorf("AUR-001: %s matches %d disposition rules", row.path, len(matches))
		}
		rule := rules[matches[0]]
		hits[matches[0]]++
		if rule.verb == "absent" {
			return fmt.Errorf("AUR-001: absent disposition covers tracked path %s", row.path)
		}
	}
	for i, rule := range rules {
		if hits[i] == 0 && rule.verb != "absent" {
			return fmt.Errorf("AUR-001: disposition rule covers no tracked path: %s", rule.pattern)
		}
	}
	return nil
}

func globMatch(pattern, value string) (bool, error) {
	var b strings.Builder
	b.WriteByte('^')
	for _, r := range pattern {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteByte('.')
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteByte('$')
	re, err := regexp.Compile(b.String())
	if err != nil {
		return false, fmt.Errorf("AUR-001: invalid disposition pattern %s: %w", pattern, err)
	}
	return re.MatchString(value), nil
}

func readClaims(root string) ([]claimRecord, error) {
	data, err := readRegular(root, filepath.Join(root, legacyClaims))
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 || lines[0] != "claims:" {
		return nil, errors.New("AUR-001: claims manifest must start with claims:")
	}
	claims := []claimRecord{}
	var current *claimRecord
	seen := map[string]bool{}
	flush := func() error {
		if current == nil {
			return nil
		}
		if current.id == "" || seen[current.id] {
			return fmt.Errorf("AUR-001: duplicate or empty claim id: %s", current.id)
		}
		seen[current.id] = true
		if current.status != "implemented" && current.status != "absent" && current.status != "disposition" {
			return fmt.Errorf("AUR-001: unsupported status for claim %s", current.id)
		}
		if current.status == "implemented" && (current.entrypoint == "" || current.test == "") {
			return fmt.Errorf("AUR-001: claim %s is implemented without entrypoint and test", current.id)
		}
		if current.status == "disposition" {
			if !validDisposition(current.disposition) || strings.TrimSpace(current.reason) == "" || current.test != "" {
				return fmt.Errorf("AUR-001: disposition claim %s needs a valid disposition, reason, and no test proof path", current.id)
			}
		}
		if current.status == "absent" && (strings.TrimSpace(current.reason) == "" || current.entrypoint != "" || current.test != "") {
			return fmt.Errorf("AUR-001: absent claim %s has no reason or names proof paths", current.id)
		}
		claims = append(claims, *current)
		current = nil
		return nil
	}
	for i, raw := range lines[1:] {
		lineNo := i + 2
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "- claim: ") {
			if err := flush(); err != nil {
				return nil, err
			}
			id := strings.TrimPrefix(line, "- claim: ")
			if id == "" || strings.ContainsAny(id, " \t") {
				return nil, fmt.Errorf("AUR-001: invalid claim id at line %d", lineNo)
			}
			current = &claimRecord{id: id}
			continue
		}
		if current == nil || !strings.Contains(line, ": ") {
			return nil, fmt.Errorf("AUR-001: malformed claim line %d", lineNo)
		}
		parts := strings.SplitN(line, ": ", 2)
		switch parts[0] {
		case "status":
			current.status = parts[1]
		case "disposition":
			current.disposition = parts[1]
		case "entrypoint":
			current.entrypoint = parts[1]
		case "test":
			current.test = parts[1]
		case "reason":
			current.reason = parts[1]
		default:
			return nil, fmt.Errorf("AUR-001: unknown claim field %s", parts[0])
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	if len(claims) == 0 {
		return nil, errors.New("AUR-001: no public claim is declared")
	}
	return claims, nil
}

func verifyClaims(root string, inventory map[string]inventoryRow, claims []claimRecord) error {
	for _, claim := range claims {
		if claim.status == "absent" {
			continue
		}
		if claim.status == "disposition" {
			if claim.entrypoint == "" {
				continue
			}
			if _, ok := inventory[claim.entrypoint]; !ok {
				return fmt.Errorf("AUR-001: disposition %s entrypoint is not tracked: %s", claim.id, claim.entrypoint)
			}
			info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(claim.entrypoint)))
			if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("AUR-001: disposition %s entrypoint is not a regular file: %s", claim.id, claim.entrypoint)
			}
			continue
		}
		for label, path := range map[string]string{"entrypoint": claim.entrypoint, "test": claim.test} {
			if _, ok := inventory[path]; !ok {
				return fmt.Errorf("AUR-001: claim %s %s is not tracked: %s", claim.id, label, path)
			}
			if label == "test" && !isExecutableTestPath(path, inventory[path]) {
				return fmt.Errorf("AUR-001: claim %s test is not executable: %s", claim.id, path)
			}
			info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(path)))
			if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("AUR-001: claim %s %s is not a regular file: %s", claim.id, label, path)
			}
		}
	}
	return nil
}

func validDisposition(value string) bool {
	switch value {
	case "keep", "migrate", "replace", "quarantine", "characterize", "delete", "absent":
		return true
	default:
		return false
	}
}

func isExecutableTestPath(path string, row inventoryRow) bool {
	if strings.HasSuffix(path, "_test.go") {
		return row.typeID == "file"
	}
	return strings.HasPrefix(path, "tests/acceptance/") && strings.HasSuffix(path, ".sh") && row.typeID == "file" && row.mode == "100755"
}
