package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runActionEntrypoint runs scripts/action-entrypoint.sh in publication
// status mode against a fake CLI and jq, with extra environment, and returns
// the exit code and the combined output.
func runActionEntrypoint(t *testing.T, dir string, fakeCLI string, extraEnv ...string) (int, string) {
	t.Helper()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	writeExec(t, filepath.Join(bin, "jq"), `#!/usr/bin/env bash
case "$*" in
  *".number"*) echo 7 ;;
  *".pull_request.head.sha"*) echo headsha ;;
  *".pull_request.base.sha"*) echo basesha ;;
  *) echo "" ;;
esac
`)
	writeExec(t, filepath.Join(bin, "aurumcode"), fakeCLI)
	workspace := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	event := filepath.Join(dir, "event.json")
	if err := os.WriteFile(event, []byte(`{"number":7}`), 0o600); err != nil {
		t.Fatal(err)
	}
	script, _ := filepath.Abs(filepath.Join("..", "..", "scripts", "action-entrypoint.sh"))
	cmd := exec.Command("bash", script, "action", "status", "false", "true", "true", "error", "default", "false")
	env := []string{}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GITHUB_STEP_SUMMARY=") && !strings.HasPrefix(kv, "AURUMCODE_PARECER_FILE=") {
			env = append(env, kv)
		}
	}
	cmd.Env = append(env,
		"PATH="+bin+":"+os.Getenv("PATH"),
		"GITHUB_EVENT_NAME=pull_request",
		"GITHUB_EVENT_PATH="+event,
		"GITHUB_WORKSPACE="+workspace,
		"GITHUB_REPOSITORY=team/project",
		"RUNNER_TEMP="+dir,
		"AURUMCODE_CLI="+filepath.Join(bin, "aurumcode"),
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return 0, string(out)
	case errors.As(err, &exitErr):
		return exitErr.ExitCode(), string(out)
	default:
		t.Fatalf("entrypoint did not run: %v\n%s", err, out)
		return -1, ""
	}
}

// The fake CLI writes the parecer where publication status puts it and
// closes the gate (exit 3), as a blocked status-mode review does.
const parecerCLI = `#!/usr/bin/env bash
if [[ -n "${AURUMCODE_PARECER_FILE:-}" ]]; then
  printf '## AurumCode: Bloqueado\n\n- error: segredo na linha 10\n' > "$AURUMCODE_PARECER_FILE"
  echo "parecer-file=$AURUMCODE_PARECER_FILE"
else
  echo "parecer-file=<unset>"
fi
exit 3
`

// TestActionStatusParecerInJobSummary (#201): with the Action used directly,
// publication status appends the parecer to $GITHUB_STEP_SUMMARY, also when
// the gate closes (exit 3, passed through), and removes its temp file.
func TestActionStatusParecerInJobSummary(t *testing.T) {
	dir := t.TempDir()
	summary := filepath.Join(dir, "step_summary")
	if err := os.WriteFile(summary, []byte("anterior\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out := runActionEntrypoint(t, dir, parecerCLI, "GITHUB_STEP_SUMMARY="+summary)
	if code != 3 {
		t.Fatalf("exit = %d, want 3 (the CLI's own)\n%s", code, out)
	}
	got, err := os.ReadFile(summary)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), "anterior\n") || !strings.Contains(string(got), "## AurumCode: Bloqueado") ||
		!strings.Contains(string(got), "segredo na linha 10") {
		t.Fatalf("job summary lacks the parecer (or lost what was there):\n%s", got)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "aurumcode-parecer.*"))
	if len(matches) != 0 {
		t.Fatalf("temp parecer file left behind: %v", matches)
	}
}

// TestActionParecerSummaryWithoutRunnerTemp: inside a `using: docker` action
// RUNNER_TEMP names the host's directory, absent in the container; the
// entrypoint still runs the review and fills the summary from TMPDIR.
func TestActionParecerSummaryWithoutRunnerTemp(t *testing.T) {
	dir := t.TempDir()
	tmp := filepath.Join(dir, "tmp")
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		t.Fatal(err)
	}
	summary := filepath.Join(dir, "step_summary")
	code, out := runActionEntrypoint(t, dir, parecerCLI,
		"GITHUB_STEP_SUMMARY="+summary, "RUNNER_TEMP=/nao/existe/_temp", "TMPDIR="+tmp)
	if code != 3 || !strings.Contains(out, "parecer-file="+tmp+"/aurumcode-parecer.") {
		t.Fatalf("exit = %d, want 3 with the parecer file under TMPDIR:\n%s", code, out)
	}
	if got, _ := os.ReadFile(summary); !strings.Contains(string(got), "## AurumCode: Bloqueado") {
		t.Fatalf("job summary lacks the parecer:\n%s", got)
	}
}

// TestActionParecerFileOnlyWithSummary: without $GITHUB_STEP_SUMMARY the
// entrypoint sets no parecer file (the CLI prints it to stdout), and a
// caller's own AURUMCODE_PARECER_FILE is kept.
func TestActionParecerFileOnlyWithSummary(t *testing.T) {
	dir := t.TempDir()
	if _, out := runActionEntrypoint(t, dir, parecerCLI); !strings.Contains(out, "parecer-file=<unset>") {
		t.Fatalf("without GITHUB_STEP_SUMMARY the CLI got a parecer file:\n%s", out)
	}
	dir = t.TempDir()
	own := filepath.Join(dir, "meu-parecer.md")
	summary := filepath.Join(dir, "step_summary")
	_, out := runActionEntrypoint(t, dir, parecerCLI, "GITHUB_STEP_SUMMARY="+summary, "AURUMCODE_PARECER_FILE="+own)
	if !strings.Contains(out, "parecer-file="+own) {
		t.Fatalf("the caller's AURUMCODE_PARECER_FILE was replaced:\n%s", out)
	}
	if _, err := os.Stat(own); err != nil {
		t.Fatalf("the caller's parecer file was removed: %v", err)
	}
}
