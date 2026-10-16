package gitleaks

import (
	"context"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/scanner"
)

// #178: a full range scans every commit reachable from Head, and only Head
// is checked as a commit.
func TestScanArgsOfAFullRangeScanTheWholeHistory(t *testing.T) {
	head := strings.Repeat("a", 40)
	args := strings.Join(scanArgs(scanner.Range{Head: head, Full: true}, "r", "c", "d", false), " ")
	if !strings.Contains(args, "--log-opts="+head+" ") || strings.Contains(args, "..") {
		t.Fatalf("full range gitleaks args: %s", args)
	}
	var checked []string
	run := func(_ context.Context, _ string, name string, args ...string) (string, string, error) {
		if name == "git" && len(args) > 0 && args[0] == "rev-parse" {
			return "false\n", "", nil
		}
		checked = append(checked, strings.Join(args, " "))
		return "", "", nil
	}
	if err := verifyRange(context.Background(), run, ".", scanner.Range{Head: head, Full: true}); err != nil {
		t.Fatalf("a full range with a commit head is valid: %v", err)
	}
	if len(checked) != 1 || !strings.Contains(checked[0], head) {
		t.Fatalf("only the head is checked: %v", checked)
	}
}
