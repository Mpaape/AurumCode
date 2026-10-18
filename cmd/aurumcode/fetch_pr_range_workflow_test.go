package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const fetchPRRangeStep = "Fetch only the pull request history"

// fetchPRRangeStepOf returns the pull request checkout's fetch-depth and the
// fetch step's run script and env in a workflow job.
func fetchPRRangeStepOf(t *testing.T, workflow, job string) (string, string, map[string]string) {
	t.Helper()
	root := filepath.Join("..", "..")
	skip, err := manifestCheckSkipped(root, os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	if skip {
		t.Skip("AURUMCODE_MODULE_ONLY=1: the acceptance staged only the Go module")
	}
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", workflow))
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string            `yaml:"name"`
				With map[string]string `yaml:"with"`
				Env  map[string]string `yaml:"env"`
				Run  string            `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatal(err)
	}
	steps := wf.Jobs[job].Steps
	for i, s := range steps {
		if s.Name != "Checkout pull request" {
			continue
		}
		if i+1 >= len(steps) || steps[i+1].Name != fetchPRRangeStep {
			t.Fatalf("%s/%s: %q must follow the pull request checkout", workflow, job, fetchPRRangeStep)
		}
		return s.With["fetch-depth"], steps[i+1].Run, steps[i+1].Env
	}
	t.Fatalf("%s/%s: no pull request checkout", workflow, job)
	return "", "", nil
}

// The pull request checkout is shallow and the next step fetches only its
// history; a pinned tool without the script fetches the whole history, as
// before, with the token only in git's environment, never in its arguments.
func TestWorkflowsFetchOnlyThePullRequestHistory(t *testing.T) {
	for _, wj := range [][2]string{{"review.yml", "review"}, {"changelog.yml", "changelog"}} {
		depth, run, env := fetchPRRangeStepOf(t, wj[0], wj[1])
		if depth != "1" || env["GH_TOKEN"] != "${{ github.token }}" {
			t.Fatalf("%s: fetch-depth %q, env %v", wj[0], depth, env)
		}
		for _, withScript := range []bool{true, false} {
			ws := t.TempDir()
			if withScript {
				dir := filepath.Join(ws, ".aurumcode-tool", "scripts", "ci")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "fetch-pr-range.sh"), []byte(`echo "script $*" >> "$LOG"`+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			bin := t.TempDir()
			fake := "#!/bin/sh\necho \"git $* header=${GIT_CONFIG_VALUE_0:-}\" >> \"$LOG\"\n"
			if err := os.WriteFile(filepath.Join(bin, "git"), []byte(fake), 0o755); err != nil {
				t.Fatal(err)
			}
			script := filepath.Join(t.TempDir(), "step.sh")
			if err := os.WriteFile(script, []byte(run), 0o644); err != nil {
				t.Fatal(err)
			}
			logPath := filepath.Join(ws, "calls.log")
			cmd := exec.Command("sh", "-e", script)
			cmd.Dir = ws
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "LOG="+logPath,
				"GH_TOKEN=step-canary", "GITHUB_SERVER_URL=https://github.com", "BASE_SHA=b", "HEAD_SHA=h")
			out, err := cmd.CombinedOutput()
			calls, _ := os.ReadFile(logPath)
			if err != nil {
				t.Fatalf("%s (script %v): %v\n%s", wj[0], withScript, err, out)
			}
			want := "script .aurumcode-target b h"
			if !withScript {
				want = "git -C .aurumcode-target fetch -q --no-tags --unshallow origin b h header=AUTHORIZATION: basic "
				if !strings.Contains(string(out), "::notice") {
					t.Fatalf("%s: the fallback says nothing:\n%s", wj[0], out)
				}
			}
			if !strings.Contains(string(calls), want) || strings.Contains(string(calls)+string(out), "step-canary") {
				t.Fatalf("%s (script %v): calls:\n%s\noutput:\n%s", wj[0], withScript, calls, out)
			}
		}
	}
}
