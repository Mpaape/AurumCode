package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/apply"
	"github.com/Mpaape/AurumCode/internal/apply/applycheck"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// aur566Repo makes a git repository holding app.go (20 numbered lines, line
// 10 being the password) and returns its directory, with cwd moved into it.
func aur566Repo(t *testing.T) (dir, gitPath string) {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH")
	}
	dir = t.TempDir()
	writeWorkingTreeFile(t, dir, "app.go", 10, "\tdbPassword := \"hunter2\"")
	more := "// tail 1\n// tail 2\n// tail 3\n// tail 4\n"
	f, err := os.OpenFile(filepath.Join(dir, "app.go"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(more); err != nil {
		t.Fatal(err)
	}
	f.Close()
	gitIn(t, gitPath, dir, nil, "init", "-q")
	t.Cleanup(chdir(t, dir))
	return dir, gitPath
}

func gitIn(t *testing.T, gitPath, dir string, stdin []byte, args ...string) string {
	t.Helper()
	cmd := exec.Command(gitPath, args...)
	cmd.Dir = dir
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func aur566Fix(t *testing.T) string {
	t.Helper()
	data, err := json.Marshal([]types.ReviewSuggestion{{File: "app.go", Line: 10, CurrentCode: "\tdbPassword := \"hunter2\"", ProposedCode: "\tdbPassword := os.Getenv(\"DB_PASSWORD\")"}})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runFix([]string{"--file", writeFixInput(t, data)}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	return stdout.String()
}

// TestAUR566FixAppliesWithPlainGitApply is AC-001: the output of fix passes
// `git apply --check` and `git apply` with NO flag, and the file then holds
// the correction. With zero-context hunks (the pre-AUR-566 output) this fails.
func TestAUR566FixAppliesWithPlainGitApply(t *testing.T) {
	dir, gitPath := aur566Repo(t)
	patch := aur566Fix(t)
	gitIn(t, gitPath, dir, []byte(patch), "apply", "--check")
	gitIn(t, gitPath, dir, []byte(patch), "apply")
	got, err := os.ReadFile(filepath.Join(dir, "app.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "\tdbPassword := os.Getenv(\"DB_PASSWORD\")\n") || strings.Contains(string(got), "hunter2") {
		t.Fatalf("AUR-566/AC-001: the correction is not in the file:\n%s", got)
	}
}

// TestAUR566PatchShapeAndOtherTools is AC-002: three context lines, correct
// headers, and the patch is also accepted by patch -p1 when it is installed.
func TestAUR566PatchShapeAndOtherTools(t *testing.T) {
	dir, _ := aur566Repo(t)
	patch := aur566Fix(t)
	for _, want := range []string{"--- a/app.go\n+++ b/app.go\n@@ -7,7 +7,7 @@\n", " // filler line 9\n-\tdbPassword", "+\tdbPassword := os.Getenv", " // tail 1\n // tail 2\n // tail 3\n"} {
		if !strings.Contains(patch, want) {
			t.Fatalf("AUR-566/AC-002: patch missing %q:\n%s", want, patch)
		}
	}
	if strings.HasSuffix(patch, "\n\n") {
		t.Fatalf("AUR-566/AC-002: patch ends with a blank line:\n%q", patch)
	}
	patchPath, err := exec.LookPath("patch")
	if err != nil {
		t.Log("patch(1) not installed here; the shell acceptance covers it where present")
		return
	}
	cmd := exec.Command(patchPath, "-p1", "--dry-run")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(patch)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("AUR-566/AC-002: patch -p1 --dry-run: %v\n%s", err, out)
	}
}

// TestAUR566NewFileAndRemovalApply is AC-002: the new-file and removal
// patches apply for real with plain git apply.
func TestAUR566NewFileAndRemovalApply(t *testing.T) {
	dir, gitPath := aur566Repo(t)
	create := apply.CreateFilePatch("pkg/new.go", "package pkg\n\nvar A = 1\n")
	gitIn(t, gitPath, dir, []byte(create), "apply", "--check")
	gitIn(t, gitPath, dir, []byte(create), "apply")
	if b, err := os.ReadFile(filepath.Join(dir, "pkg/new.go")); err != nil || string(b) != "package pkg\n\nvar A = 1\n" {
		t.Fatalf("new file not created: %v %q", err, b)
	}
	data, err := os.ReadFile(filepath.Join(dir, "app.go"))
	if err != nil {
		t.Fatal(err)
	}
	del := apply.DeleteFilePatch("app.go", string(data))
	gitIn(t, gitPath, dir, []byte(del), "apply")
	if _, err := os.Stat(filepath.Join(dir, "app.go")); !os.IsNotExist(err) {
		t.Fatalf("app.go was not removed: %v", err)
	}
}

// TestAUR566FixOutputAppliesStrictlyWithoutGit is AC-001 where no git binary
// exists: the same runFix output is applied with plain-git strictness.
func TestAUR566FixOutputAppliesStrictlyWithoutGit(t *testing.T) {
	dir := t.TempDir()
	writeWorkingTreeFile(t, dir, "app.go", 10, "\tdbPassword := \"hunter2\"")
	defer chdir(t, dir)()
	before, err := os.ReadFile(filepath.Join(dir, "app.go"))
	if err != nil {
		t.Fatal(err)
	}
	patch := aur566Fix(t)
	got, err := applycheck.Apply(map[string]string{"app.go": string(before)}, patch)
	if err != nil {
		t.Fatalf("AUR-566/AC-001: the printed patch does not apply:\n%s\n%v", patch, err)
	}
	if !strings.Contains(got["app.go"], "\tdbPassword := os.Getenv(\"DB_PASSWORD\")\n") || strings.Contains(got["app.go"], "hunter2") {
		t.Fatalf("AUR-566/AC-001: the correction is not in the file:\n%s", got["app.go"])
	}
}
