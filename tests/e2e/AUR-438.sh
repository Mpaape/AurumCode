#!/usr/bin/env bash
# E2E check for AUR-438: build (or reuse) the real aurumcode binary and use
# `review --pr 42 --repo dono/projeto --publicar --na-linha` as a user
# would, against a loopback fake GitHub built from this card's fixtures
# (tests/fixtures/scm/github) plus a deterministic offline model response
# this script writes itself. Proves AC-001: a finding on a line the diff
# added is published inline at its exact file and line; a finding outside
# the changed lines is not dropped, it becomes a general pull request
# comment; a read-only token is refused before any comment is posted; and
# repeating the run reproduces the exact same output. See
# docs/specs/AUR-438.md.
set -euo pipefail
export LC_ALL=C

readonly card=AUR-438
selector="${1:-E2EAUR438}"
[[ "$selector" == "E2EAUR438" ]] || { printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64; }

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

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e2e-a438.XXXXXX")" || infra mktemp
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

# fakegithub: a loopback-only fake GitHub API, built from this card's own
# fixtures. It answers GET /repos/{owner}/{repo} with the write or
# read-only permissions fixture (selectable per run), GET
# /repos/{owner}/{repo}/pulls/42 with the fixed pr-42.diff, and any POST
# with 201 + comment-created.json while appending "METHOD path body" to a
# log file this script inspects afterward. It imports only the standard
# library, so it builds as a single file with no module context.
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
)

func mustRead(path string) []byte {
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fixture ausente:", err)
		os.Exit(1)
	}
	return b
}

// bytesContainsCommitID reports whether a POST body carries a non-empty
// "commit_id" field, the same check the real GitHub API makes before
// accepting an inline review comment.
func bytesContainsCommitID(body []byte) bool {
	var v struct {
		CommitID string `json:"commit_id"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return false
	}
	return v.CommitID != ""
}

func main() {
	fixturesDir := os.Args[1]
	scenario := os.Args[2]
	logPath := os.Args[3]
	// failMode, when "first-post-400", fails the first POST this process
	// receives with a plain 400 (never retried by the client -- see the
	// commit_id check below for why a 5xx would be the wrong choice) so a
	// test can prove one failed publish does not swallow the others.
	// Default "none" changes nothing.
	failMode := "none"
	if len(os.Args) > 4 {
		failMode = os.Args[4]
	}

	diffBody := mustRead(fixturesDir + "/pr-42.diff")
	created := mustRead(fixturesDir + "/comment-created.json")
	var repoJSON []byte
	if scenario == "write" {
		repoJSON = mustRead(fixturesDir + "/repo-read-write.json")
	} else {
		repoJSON = mustRead(fixturesDir + "/repo-read-only.json")
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		fmt.Fprintln(os.Stderr, "abrindo log:", err)
		os.Exit(1)
	}

	postSeen := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			fmt.Fprintf(logFile, "POST %s %s\n", r.URL.Path, body)
			postSeen++

			// Real GitHub rejects an inline review comment with no
			// commit_id: 422, not success. A fake that always answers 201
			// regardless of commit_id cannot catch a missing SHA gate --
			// see the no-GITHUB_SHA scenario below.
			if r.URL.Path == "/repos/dono/projeto/pulls/42/comments" &&
				!bytesContainsCommitID(body) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"message":"Validation Failed","errors":[{"field":"commit_id","code":"missing_field"}]}`))
				return
			}

			if failMode == "first-post-400" && postSeen == 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"message":"synthetic failure injected for AUR-438's own e2e test"}`))
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
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(diffBody)
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

# start_fake starts the fake server for the given scenario in the
# background and sets FAKE_URL to its base URL once the listener is up.
# It must run as a plain background command, never inside a $(...)
# command substitution: a substitution runs in its own subshell, and a
# background job started there would leave $! -- and so the PID this
# script needs for cleanup -- invisible to the calling shell. A server
# that exits before printing a URL (e.g. loopback truly unavailable) is an
# infrastructure gap, not a RED.
FAKE_URL=""
start_fake() {
  local scenario="$1" log="$2" url_file="$3" failmode="${4:-none}"
  "$fakegithub_bin" "$fixtures_dir" "$scenario" "$log" "$failmode" >"$url_file" 2>>"$run_dir/fakegithub.stderr" &
  pids+=("$!")
  local waited=0
  while [[ ! -s "$url_file" ]]; do
    sleep 0.05
    waited=$((waited + 1))
    ((waited < 100)) || infra fakegithub_did_not_start
  done
  FAKE_URL="$(head -n1 "$url_file")"
}

# Deterministic offline model response this script fully controls: one
# finding on cmdb/settings.go's added line 3 (inline-eligible), one on
# docs/notas.md line 99 -- a line number nowhere near that file's single
# +1-line hunk (@@ -1,1 +1,2 @@), so it must become a general comment
# instead of being dropped. Both cite an embedded quality rule so they
# survive the AUR-434 rule-citation gate.
fixture="$run_dir/response.json"
cat >"$fixture" <<'EOF'
{
  "issues": [
    {
      "file": "cmdb/settings.go",
      "line": 3,
      "severity": "warning",
      "rule_id": "quality/long-function",
      "message": "Achado sintetico na linha que o diff adicionou."
    },
    {
      "file": "docs/notas.md",
      "line": 99,
      "severity": "info",
      "rule_id": "quality/long-function",
      "message": "Achado sintetico fora das linhas alteradas."
    }
  ],
  "summary": "Resposta sintetica e deterministica para AUR-438."
}
EOF

# run_pr runs the binary as a user would with the PR flags, against the
# given fake GitHub base URL and a synthetic, runtime-only token. Raw exit
# code lands in rc; stdout/stderr in $run_dir/out.{stdout,stderr}.
run_pr() {
  local base_url="$1" token="$2"
  set +e
  AURUMCODE_LLM_FIXTURE="$fixture" AURUMCODE_GITHUB_API_URL="$base_url" \
    GITHUB_TOKEN="$token" GITHUB_SHA="e2e-synthetic-sha" \
    "$bin" review --pr 42 --repo dono/projeto --publicar --na-linha \
    >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  rc=$?
  set -e
}

# run_pr_no_sha is run_pr with GITHUB_SHA left genuinely unset (not merely
# empty) in the child's environment, for the no-commit-SHA refusal proof.
run_pr_no_sha() {
  local base_url="$1" token="$2"
  set +e
  env -u GITHUB_SHA \
    AURUMCODE_LLM_FIXTURE="$fixture" AURUMCODE_GITHUB_API_URL="$base_url" \
    GITHUB_TOKEN="$token" \
    "$bin" review --pr 42 --repo dono/projeto --publicar --na-linha \
    >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  rc=$?
  set -e
}

## Scenario 1: a write-permission token. Both kinds of finding are
## published; none is dropped.
log1="$run_dir/write.log"
start_fake write "$log1" "$run_dir/write.url"
url1="$FAKE_URL"
run_pr "$url1" "token-sintetico-write"
[[ "$rc" -eq 0 ]] || fail "write_run_failed:exit:$rc"
grep -Fq 'cmdb/settings.go:3: [warning] Achado sintetico na linha que o diff adicionou.' "$run_dir/out.stdout" \
  || fail missing_inline_finding
grep -Fq -- '-- publicado na linha' "$run_dir/out.stdout" || fail missing_inline_marker
grep -Fq 'docs/notas.md:99: [info] Achado sintetico fora das linhas alteradas.' "$run_dir/out.stdout" \
  || fail missing_general_finding
grep -Fq -- '-- publicado como comentario geral' "$run_dir/out.stdout" || fail missing_general_marker
grep -Fq '2 comentario(s) publicado(s) no pull request #42 (1 na linha, 1 geral).' "$run_dir/out.stdout" \
  || fail missing_summary

# Server-side proof: exactly one inline review comment (path+line anchored)
# and exactly one general issue comment; the out-of-range finding was
# never silently dropped (MUT-001's target).
inline_posts="$(grep -c '^POST /repos/dono/projeto/pulls/42/comments ' "$log1" || true)"
[[ "$inline_posts" -eq 1 ]] || fail "wrong_inline_post_count:$inline_posts"
grep -Fq '"path":"cmdb/settings.go"' "$log1" || fail inline_post_wrong_path
grep -Fq '"line":3' "$log1" || fail inline_post_wrong_line
general_posts="$(grep -c '^POST /repos/dono/projeto/issues/42/comments ' "$log1" || true)"
[[ "$general_posts" -eq 1 ]] || fail missing_general_comment
grep -Fq 'docs/notas.md:99' "$log1" || fail missing_general_comment

first_stdout="$(cat "$run_dir/out.stdout")"

## Determinism: the same input run again produces the exact same stdout
## and the exact same publish transcript (a second, independent server).
log2="$run_dir/write2.log"
start_fake write "$log2" "$run_dir/write2.url"
url2="$FAKE_URL"
run_pr "$url2" "token-sintetico-write"
[[ "$rc" -eq 0 ]] || fail "write_rerun_failed:exit:$rc"
[[ "$(cat "$run_dir/out.stdout")" == "$first_stdout" ]] || fail non_deterministic
[[ "$(grep -c '^POST ' "$log2")" -eq 2 ]] || fail non_deterministic_publish_count

## Scenario 2: a read-only token. Both findings were computable, but
## nothing may be posted: the command refuses up front, and the fake
## server never receives a single POST.
log3="$run_dir/readonly.log"
start_fake readonly "$log3" "$run_dir/readonly.url"
url3="$FAKE_URL"
run_pr "$url3" "token-sintetico-readonly"
[[ "$rc" -eq 1 ]] || fail "readonly_wrong_exit:$rc"
grep -Fq 'refusing to publish' "$run_dir/out.stderr" || fail readonly_not_refused
grep -Fq 'write permission' "$run_dir/out.stderr" || fail readonly_not_refused
[[ -s "$log3" ]] || fail readonly_log_empty
if grep -q '^POST ' "$log3"; then
  fail readonly_post_leaked
fi

## Scenario 3: a write-permission token but no GITHUB_SHA. The fixture
## still has an inline-eligible finding (cmdb/settings.go:3), and the fake
## server now answers 422 to an inline comment with no commit_id -- exactly
## like the real GitHub API -- instead of always agreeing. The command
## must refuse before attempting ANY post, not rely on the fake's 422 to
## catch it after the fact: zero POST lines in the log proves the refusal
## is this command's own pre-publish gate.
log4="$run_dir/nosha.log"
start_fake write "$log4" "$run_dir/nosha.url"
url4="$FAKE_URL"
run_pr_no_sha "$url4" "token-sintetico-write"
[[ "$rc" -eq 1 ]] || fail "nosha_wrong_exit:$rc"
grep -Fq 'commit SHA' "$run_dir/out.stderr" || fail nosha_not_refused
if grep -q '^POST ' "$log4"; then
  fail nosha_post_leaked
fi

## Scenario 4: a write-permission token and GITHUB_SHA set, but the fake
## server fails the first POST it receives (a plain 400, not the
## commit_id rule above, and not a 5xx the client would silently retry).
## The second finding must still be published: one failed publish does not
## abort the rest.
log5="$run_dir/onefail.log"
start_fake write "$log5" "$run_dir/onefail.url" first-post-400
url5="$FAKE_URL"
run_pr "$url5" "token-sintetico-write"
[[ "$rc" -eq 1 ]] || fail "onefail_wrong_exit:$rc"
grep -Fq 'docs/notas.md:99: [info] Achado sintetico fora das linhas alteradas.' "$run_dir/out.stdout" \
  || fail onefail_swallowed_other_finding
grep -Fq -- '-- publicado como comentario geral' "$run_dir/out.stdout" || fail onefail_swallowed_other_finding
grep -Fq '1 comentario(s) falharam ao publicar' "$run_dir/out.stderr" || fail onefail_missing_failure_summary
[[ "$(grep -c '^POST ' "$log5")" -eq 2 ]] || fail onefail_second_post_not_attempted

printf '%s/AC-001/E2EAUR438/ok\n' "$card"
