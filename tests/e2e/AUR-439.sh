#!/usr/bin/env bash
# E2E check for AUR-439: build (or reuse) the real aurumcode binary and use
# `review --pr 42 --repo dono/projeto --publicar --check` as a user would,
# against a loopback fake GitHub built from AUR-438's fixtures
# (tests/fixtures/scm/github, a read_path here) plus deterministic offline
# model responses this script writes itself. Proves AC-001: the review
# result becomes one commit status on the pull request's head commit --
# "failure" when at least one finding is grave (error severity), "success"
# otherwise -- so a branch protection rule requiring this check blocks the
# merge until the grave finding is fixed (MUT-001: reporting success while
# a grave finding is present must be caught). --check needs no --na-linha
# of its own (AC-001's declared command has none), --check inherits the
# same fail-closed commit-SHA and write-permission gates AUR-438 already
# proved instead of bypassing them, and the pre-existing
# `--pr ... --publicar --na-linha` (no --check) contract is untouched: it
# never posts to the statuses endpoint at all. See docs/specs/AUR-439.md.
set -euo pipefail
export LC_ALL=C

readonly card=AUR-439
selector="${1:-E2EAUR439}"
[[ "$selector" == "E2EAUR439" ]] || { printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64; }

fail() { printf '%s/AC-001/%s\n' "$card" "$1" >&2; exit 1; }
infra() { printf '%s/AC-001/infrastructure/%s\n' "$card" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

command -v go >/dev/null 2>&1 || infra missing_go

fixtures_dir="$repo_root/tests/fixtures/scm/github"
test -s "$fixtures_dir/pr-42.diff" || infra missing_fixture_diff
test -s "$fixtures_dir/repo-read-write.json" || infra missing_fixture_readwrite
test -s "$fixtures_dir/repo-read-only.json" || infra missing_fixture_readonly
test -s "$fixtures_dir/comment-created.json" || infra missing_fixture_created

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e2e-a439.XXXXXX")" || infra mktemp
pids=()
cleanup() {
  local pid
  for pid in "${pids[@]:-}"; do
    [[ -n "$pid" ]] && kill "$pid" >/dev/null 2>&1 || true
  done
  chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true
  rm -rf -- "$run_dir" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM HUP

mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
: "${GOCACHE:=$run_dir/gocache}"
: "${GOTMPDIR:=$run_dir/gotmp}"
export GOCACHE GOTMPDIR
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local

if [[ -n "${AURUMCODE_BIN:-}" ]]; then
  bin="$AURUMCODE_BIN"
  test -x "$bin" || infra missing_prebuilt_binary
else
  bin="$run_dir/aurumcode"
  build_log="$run_dir/build.log"
  if ! (cd "$repo_root" && GOFLAGS=-mod=mod go build -o "$bin" ./cmd/aurumcode) >"$build_log" 2>&1; then
    cat "$build_log" >&2
    fail build_failed
  fi
fi

# fakegithub: a loopback-only fake GitHub API, built from AUR-438's
# fixtures plus AUR-439's own statuses endpoint. It answers GET
# /repos/{owner}/{repo} with the write or read-only permissions fixture,
# GET /repos/{owner}/{repo}/pulls/42 with the fixed pr-42.diff, any POST to
# the comment endpoints with 201 + comment-created.json, and POST to
# /repos/dono/projeto/statuses/<sha> with 201 -- but only when the body
# carries one of GitHub's real state values; a missing/invalid state gets
# 422, exactly like the real API would, so this is not a mock that always
# agrees. Every request is appended to a log file this script inspects.
fakegithub_src="$run_dir/fakegithub.go"
cat >"$fakegithub_src" <<'EOF'
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
)

func mustRead(path string) []byte {
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fixture ausente:", err)
		os.Exit(1)
	}
	return b
}

func bytesContainsCommitID(body []byte) bool {
	var v struct {
		CommitID string `json:"commit_id"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return false
	}
	return v.CommitID != ""
}

// bytesState reads the "state" field a statuses POST body carries.
func bytesState(body []byte) string {
	var v struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return ""
	}
	return v.State
}

func main() {
	fixturesDir := os.Args[1]
	scenario := os.Args[2]
	logPath := os.Args[3]

	diffBody := mustRead(fixturesDir + "/pr-42.diff")
	created := mustRead(fixturesDir + "/comment-created.json")
	var repoJSON []byte
	if scenario == "write" || scenario == "nohead" {
		repoJSON = mustRead(fixturesDir + "/repo-read-write.json")
	} else {
		repoJSON = mustRead(fixturesDir + "/repo-read-only.json")
	}
	// AUR-504: --check anchors the status on the pull request head read from
	// the metadata endpoint, never on GITHUB_SHA. "nohead" is the fail-closed
	// scenario: the API returns a PR payload with no head SHA, which must
	// still refuse to publish rather than fall back to a different commit.
	headSHA := "e2e-check-sha-1"
	if scenario == "nohead" {
		headSHA = ""
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		fmt.Fprintln(os.Stderr, "abrindo log:", err)
		os.Exit(1)
	}

	validStates := map[string]bool{"pending": true, "success": true, "error": true, "failure": true}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			fmt.Fprintf(logFile, "POST %s %s\n", r.URL.Path, body)

			if strings.HasPrefix(r.URL.Path, "/repos/dono/projeto/statuses/") {
				if !validStates[bytesState(body)] {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnprocessableEntity)
					_, _ = w.Write([]byte(`{"message":"Validation Failed","errors":[{"field":"state","code":"invalid"}]}`))
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id":439001,"state":"` + bytesState(body) + `"}`))
				return
			}

			// Real GitHub rejects an inline review comment with no
			// commit_id: 422, not success.
			if r.URL.Path == "/repos/dono/projeto/pulls/42/comments" &&
				!bytesContainsCommitID(body) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"message":"Validation Failed","errors":[{"field":"commit_id","code":"missing_field"}]}`))
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write(created)
			return
		}
		fmt.Fprintf(logFile, "GET %s\n", r.URL.Path)
		switch r.URL.Path {
		case "/repos/dono/projeto":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(repoJSON)
		case "/repos/dono/projeto/pulls/42":
			// AUR-504: the --check path requests the pull request metadata
			// with Accept: application/vnd.github+json; only the diff
			// request carries the .diff accept type. Serving the diff for
			// both made the metadata decode fail and --check fail closed.
			if strings.Contains(r.Header.Get("Accept"), "diff") {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(diffBody)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, `{"number":42,"title":"t","body":"b","head":{"sha":%q}}`, headSHA)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, "loopback indisponivel:", err)
		os.Exit(79)
	}
	fmt.Println("http://" + listener.Addr().String())
	server := &http.Server{Handler: handler}
	_ = server.Serve(listener)
}
EOF

fakegithub_bin="$run_dir/fakegithub"
fakegithub_build_log="$run_dir/fakegithub-build.log"
if ! go build -o "$fakegithub_bin" "$fakegithub_src" >"$fakegithub_build_log" 2>&1; then
  cat "$fakegithub_build_log" >&2
  fail fakegithub_build_failed
fi

FAKE_URL=""
start_fake() {
  local scenario="$1" log="$2" url_file="$3"
  "$fakegithub_bin" "$fixtures_dir" "$scenario" "$log" >"$url_file" 2>>"$run_dir/fakegithub.stderr" &
  pids+=("$!")
  local waited=0
  while [[ ! -s "$url_file" ]]; do
    sleep 0.05
    waited=$((waited + 1))
    ((waited < 100)) || infra fakegithub_did_not_start
  done
  FAKE_URL="$(head -n1 "$url_file")"
}

# Three deterministic offline model responses this script fully controls:
# a grave (error-severity) finding on cmdb/settings.go's added line 3, plus
# an info finding out of range (docs/notas.md:99, never dropped, see
# AUR-438); a clean response with only a warning finding on the same added
# line (nothing grave); and a response with zero findings at all.
# The scope+evidence gate (internal/review/scope.go) keeps a finding only
# when it anchors on an added/removed line AND carries evidence, impact and
# verification. cmdb/settings.go line 3 is the added line in pr-42.diff.
fixture_grave="$run_dir/response-grave.json"
cat >"$fixture_grave" <<'EOF'
{
  "issues": [
    {
      "file": "cmdb/settings.go",
      "line": 3,
      "severity": "error",
      "rule_id": "quality/long-function",
      "message": "Achado grave sintetico na linha que o diff adicionou.",
      "impact": "Um limite de retentativas inadequado degrada espelhos lentos.",
      "evidence": "A linha adicionada eleva o limite de retentativas sem justificativa.",
      "verification": "Reduzir o limite e rodar a suite de retentativas."
    }
  ],
  "summary": "Resposta sintetica grave para AUR-439."
}
EOF

fixture_clean="$run_dir/response-clean.json"
cat >"$fixture_clean" <<'EOF'
{
  "issues": [
    {
      "file": "cmdb/settings.go",
      "line": 3,
      "severity": "warning",
      "rule_id": "quality/long-function",
      "message": "Achado nao grave sintetico.",
      "impact": "Um limite de retentativas discutivel, sem impacto grave.",
      "evidence": "A linha adicionada altera o limite de retentativas.",
      "verification": "Rodar a suite de retentativas com o limite proposto."
    }
  ],
  "summary": "Resposta sintetica sem achado grave para AUR-439."
}
EOF

fixture_general_only="$run_dir/response-general-only.json"
cat >"$fixture_general_only" <<'EOF'
{
  "issues": [
    {
      "file": "docs/notas.md",
      "line": 99,
      "severity": "info",
      "rule_id": "quality/long-function",
      "message": "Achado geral sintetico, sem elegibilidade inline."
    }
  ],
  "summary": "Resposta sintetica so-geral para AUR-439."
}
EOF

fixture_empty="$run_dir/response-empty.json"
cat >"$fixture_empty" <<'EOF'
{
  "issues": [],
  "summary": "Nenhum achado para AUR-439."
}
EOF

run_check() {
  local base_url="$1" token="$2" fixture="$3" sha="$4"
  set +e
  if [[ -n "$sha" ]]; then
    AURUMCODE_LLM_FIXTURE="$fixture" AURUMCODE_GITHUB_API_URL="$base_url" \
      GITHUB_TOKEN="$token" GITHUB_SHA="$sha" \
      "$bin" review --pr 42 --repo dono/projeto --publicar --check \
      >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  else
    env -u GITHUB_SHA \
      AURUMCODE_LLM_FIXTURE="$fixture" AURUMCODE_GITHUB_API_URL="$base_url" \
      GITHUB_TOKEN="$token" \
      "$bin" review --pr 42 --repo dono/projeto --publicar --check \
      >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  fi
  rc=$?
  set -e
}

readonly sha1="e2e-check-sha-1"

## Scenario 1: a grave finding. The check must fail, blocking the merge --
## and the exit code itself must not read as success (MUT-001's target).
log1="$run_dir/grave.log"
start_fake write "$log1" "$run_dir/grave.url"
run_check "$FAKE_URL" "token-sintetico-write" "$fixture_grave" "$sha1"
[[ "$rc" -eq 3 ]] || fail "grave_wrong_exit:$rc"
grep -Fq "check \"aurumcode/review\" publicado no commit $sha1: failure" "$run_dir/out.stdout" \
  || fail grave_missing_check_line
status_posts="$(grep -c "^POST /repos/dono/projeto/statuses/$sha1 " "$log1" || true)"
[[ "$status_posts" -eq 1 ]] || fail "grave_wrong_status_post_count:$status_posts"
grep -F "POST /repos/dono/projeto/statuses/$sha1 " "$log1" | grep -Fq '"state":"failure"' \
  || fail wrong_state_reported_success
# The finding is still published exactly like AUR-438 -- --check does not
# suppress it. With no inline_comments config in this offline run it goes
# out as a general comment; either publication shape satisfies the check
# that --check did not swallow it.
grep -Eq -- '-- publicado na linha|-- publicado como comentario geral' "$run_dir/out.stdout" \
  || fail grave_missing_comment
grave_first_stdout="$(cat "$run_dir/out.stdout")"

## Determinism: rerunning the exact same input against a fresh server
## reproduces the exact same stdout and the exact same status.
log1b="$run_dir/grave2.log"
start_fake write "$log1b" "$run_dir/grave2.url"
run_check "$FAKE_URL" "token-sintetico-write" "$fixture_grave" "$sha1"
[[ "$rc" -eq 3 ]] || fail "grave_rerun_wrong_exit:$rc"
[[ "$(cat "$run_dir/out.stdout")" == "$grave_first_stdout" ]] || fail non_deterministic
[[ "$(grep -c "^POST /repos/dono/projeto/statuses/$sha1 " "$log1b" || true)" -eq 1 ]] || fail non_deterministic_status_count

## Scenario 2: no grave finding (only a warning). The check must succeed.
log2="$run_dir/clean.log"
start_fake write "$log2" "$run_dir/clean.url"
run_check "$FAKE_URL" "token-sintetico-write" "$fixture_clean" "$sha1"
[[ "$rc" -eq 0 ]] || fail "clean_wrong_exit:$rc"
grep -Fq "check \"aurumcode/review\" publicado no commit $sha1: success" "$run_dir/out.stdout" \
  || fail clean_missing_check_line
grep -F "POST /repos/dono/projeto/statuses/$sha1 " "$log2" | grep -Fq '"state":"success"' \
  || fail clean_wrong_state

## Scenario 3: zero findings at all. The --pr path's zero-finding contract
## is the published "success" status (the "No issues found." string is the
## --base path's stdout contract, not this one), so an earlier failing check
## on the same commit must still be clearable once the finding is fixed.
log3="$run_dir/empty.log"
start_fake write "$log3" "$run_dir/empty.url"
run_check "$FAKE_URL" "token-sintetico-write" "$fixture_empty" "$sha1"
[[ "$rc" -eq 0 ]] || fail "empty_wrong_exit:$rc"
grep -Fq '0 comentario(s) publicado(s)' "$run_dir/out.stdout" || fail empty_missing_no_issues_line
grep -Fq "check \"aurumcode/review\" publicado no commit $sha1: success" "$run_dir/out.stdout" \
  || fail empty_missing_check_line
# Two POSTs are expected: the summary review comment (always posted in the
# comments mode) and the single commit status. A second status POST would be
# the regression this scenario guards against.
[[ "$(grep -c '^POST ' "$log3")" -eq 2 ]] || fail empty_unexpected_extra_post
[[ "$(grep -c "^POST /repos/dono/projeto/statuses/$sha1 " "$log3")" -eq 1 ]] || fail empty_unexpected_extra_status

## Scenario 4: --check with only a general (non-inline-eligible) finding
## and no GITHUB_SHA at all, where the API reports a pull request with no
## head SHA. Before AUR-504 the anchor came from GITHUB_SHA and an unset
## variable tripped the fail-closed gate directly; AUR-504 moves the anchor
## to the API head (so an unset GITHUB_SHA is fine) and moves the fail-closed
## to a head that cannot be determined. The command must still refuse to
## publish on a different commit rather than guess.
log4="$run_dir/nosha.log"
start_fake nohead "$log4" "$run_dir/nosha.url"
run_check "$FAKE_URL" "token-sintetico-write" "$fixture_general_only" ""
[[ "$rc" -eq 1 ]] || fail "nosha_wrong_exit:$rc"
grep -Fq 'head SHA' "$run_dir/out.stderr" || fail nosha_not_refused
if grep -q '^POST ' "$log4"; then
  fail nosha_post_leaked
fi

## Scenario 5: --check with a read-only token. The pre-existing
## write-permission refusal (AUR-437/AUR-438) must still apply -- --check
## inherits it, it does not get a separate, weaker path around it.
log5="$run_dir/readonly.log"
start_fake readonly "$log5" "$run_dir/readonly.url"
run_check "$FAKE_URL" "token-sintetico-readonly" "$fixture_grave" "$sha1"
[[ "$rc" -eq 1 ]] || fail "readonly_wrong_exit:$rc"
grep -Fq 'refusing to publish' "$run_dir/out.stderr" || fail readonly_not_refused
grep -Fq 'write permission' "$run_dir/out.stderr" || fail readonly_not_refused
if grep -q '^POST ' "$log5"; then
  fail readonly_post_leaked
fi

## Scenario 6: the pre-existing contract, `--na-linha` instead of
## `--check`, is untouched -- exit 0 regardless of the grave finding, and
## the statuses endpoint is never contacted at all.
log6="$run_dir/nocheck.log"
start_fake write "$log6" "$run_dir/nocheck.url"
set +e
AURUMCODE_LLM_FIXTURE="$fixture_grave" AURUMCODE_GITHUB_API_URL="$FAKE_URL" \
  GITHUB_TOKEN="token-sintetico-write" GITHUB_SHA="$sha1" \
  "$bin" review --pr 42 --repo dono/projeto --publicar --na-linha \
  >"$run_dir/nocheck.stdout" 2>"$run_dir/nocheck.stderr"
rc=$?
set -e
[[ "$rc" -eq 0 ]] || fail "nocheck_wrong_exit:$rc"
if grep -q 'statuses/' "$log6"; then
  fail nocheck_status_leaked
fi
if grep -Fq 'check "aurumcode/review"' "$run_dir/nocheck.stdout"; then
  fail nocheck_check_line_leaked
fi

printf '%s/AC-001/E2EAUR439/ok\n' "$card"
