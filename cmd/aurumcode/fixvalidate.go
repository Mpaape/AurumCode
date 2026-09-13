package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/apply"
)

// validateFixPatch confirms that patch actually applies to the real working
// tree rooted at dir before runFix ever reports success (AUR-489, AC-003).
// Before this card, `aurumcode fix` never read the file a suggestion named:
// a suggestion fabricated with a current_code that did not exist anywhere
// in the file still produced a patch and exit 0.
//
// When git is on PATH, `git apply --check` against dir IS the check: the
// same mechanism a human or CI would use to actually apply the patch, so
// this asks the real question ("would this patch apply?") instead of
// reimplementing a diff engine. --unidiff-zero is required because
// apply.BuildPlan's hunks carry no real file context beyond whatever the
// suggestion's current_code and proposed_code happen to share (its LCS): a
// hunk that replaces a line outright has zero context lines, which plain
// `git apply` refuses to trust.
//
// Without git, the fallback -- validateAgainstFiles -- compares each
// hunk's removed lines against the real file's content at the claimed line
// numbers: the "context comparison" AC-003 asks for as the non-git path.
func validateFixPatch(dir, patch string, plan *apply.Plan) error {
	if strings.TrimSpace(patch) == "" || plan == nil || len(plan.Files) == 0 {
		return nil
	}
	if gitPath, err := exec.LookPath("git"); err == nil {
		return validatePatchWithGit(gitPath, dir, patch)
	}
	return validateAgainstFiles(dir, plan)
}

// validatePatchWithGit runs `git apply --check --unidiff-zero` against dir,
// feeding patch on stdin. git's own error output already names the
// offending file and line (e.g. "error: patch failed: f.go:2"), so it is
// wrapped, not replaced.
func validatePatchWithGit(gitPath, dir, patch string) error {
	cmd := exec.Command(gitPath, "apply", "--check", "--unidiff-zero")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(patch)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("patch does not apply to the working tree: %s", msg)
	}
	return nil
}

// validateAgainstFiles is the no-git fallback: for every file the plan
// touches, it reads the real file at dir/path and confirms each removed
// line's text matches the real file at that exact line number. A pure
// insertion (no removals) is instead checked for a plausible insertion
// point within the file's real line range.
func validateAgainstFiles(dir string, plan *apply.Plan) error {
	for _, fe := range plan.Files {
		full := filepath.Join(dir, fe.Path)
		data, err := os.ReadFile(full)
		if err != nil {
			return fmt.Errorf("%s: not found in the working tree (%w)", fe.Path, err)
		}
		lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")

		if len(fe.Removals) == 0 {
			if len(fe.Additions) == 0 {
				continue
			}
			at := fe.Additions[0].Number
			if at < 1 || at > len(lines)+1 {
				return fmt.Errorf("%s:%d: insertion point is outside the working tree (file has %d lines)", fe.Path, at, len(lines))
			}
			continue
		}

		for _, rem := range fe.Removals {
			idx := rem.Number - 1
			if idx < 0 || idx >= len(lines) {
				return fmt.Errorf("%s:%d: line does not exist in the working tree (file has %d lines)", fe.Path, rem.Number, len(lines))
			}
			if lines[idx] != rem.Text {
				return fmt.Errorf("%s:%d: working tree does not match the suggestion's current_code", fe.Path, rem.Number)
			}
		}
	}
	return nil
}
