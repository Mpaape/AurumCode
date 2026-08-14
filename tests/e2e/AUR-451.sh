#!/usr/bin/env bash
# E2E check for AUR-451: build (or reuse) the real aurumcode binary and use
# `review --pr 42 --repo dono/projeto --publicar --na-linha --seguranca
# --fail-on high` as a user would, against a loopback fake GitHub built
# from this card's read_paths fixtures (tests/fixtures/scm/github) plus a
# diff body this script supplies itself, since the shared pr-42.diff
# fixture carries nothing a security-category rule of the embedded catalog
# would ever match.
#
# Proves AC-001 end to end: --seguranca now runs on the --pr path (the
# planted secret is published as an inline pull request comment citing
# security/hardcoded-secret), --fail-on now gates that path too (exit 3
# when the finding is grave), --limite still caps spend before any comment
# is published, and --modelo still fails loudly when it cannot be served --
# reusing the exact functions cmd/aurumcode/main.go's --base path already
# calls (see cmd/aurumcode/pr.go's package doc). The pre-AUR-451 contract
# (no new flags) is untouched: it still publishes nothing for this diff,
# because nothing but --seguranca can ever see the planted secret.
set -euo pipefail
export LC_ALL=C

ulimit -v 8388608 2>/dev/null || true
export GOMEMLIMIT=2GiB

readonly card=AUR-451
selector="${1:-E2EAUR451}"
[[ "$selector" == "E2EAUR451" ]] || { printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64; }

fail() { printf '%s/AC-001/%s\n' "$card" "$1" >&2; exit 1; }
infra() { printf '%s/AC-001/infrastructure/%s\n' "$card" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

command -v go >/dev/null 2>&1 || infra missing_go

fixtures_dir="$repo_root/tests/fixtures/scm/github"
test -s "$fixtures_dir/repo-read-write.json" || infra missing_fixture_readwrite
test -s "$fixtures_dir/repo-read-only.json" || infra missing_fixture_readonly
test -s "$fixtures_dir/comment-created.json" || infra missing_fixture_created

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e2e-a451.XXXXXX")" || infra mktemp
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

# The diff this script's own proof reviews: one file, one added line at new
# line 3, planting the exact synthetic secret value
# (API_KEY=AURUM-FAKE-KEY-9000-2222) tests/unit/AUR-442.go's TestAUR442
# already proves matches security/hardcoded-secret. Same hunk geometry as
# the shared tests/fixtures/scm/github/pr-42.diff (a read_path this card
# cannot edit) -- only the added line's content differs.
vuln_diff="$run_dir/pr-42-vuln.diff"
# printf, not a heredoc: a unified diff's context line for a genuinely
# blank source line is " " (one leading space), never a fully empty line
# (git always emits the marker column) -- a heredoc's blank line loses
# that leading space silently, which shifts every following added-line
# number off by one (internal/git/githubclient's parseDiff and this card's
# own addedLineNumbers, cmd/aurumcode/pr.go, both drop a truly empty
# content line instead of treating it as a context line). printf's %s
# makes the single space explicit and unambiguous.
printf '%s\n' \
  'diff --git a/cmdb/settings.go b/cmdb/settings.go' \
  'index 1111111..2222222 100644' \
  '--- a/cmdb/settings.go' \
  '+++ b/cmdb/settings.go' \
  '@@ -1,3 +1,4 @@' \
  ' package cmdb' \
  ' ' \
  '+API_KEY=AURUM-FAKE-KEY-9000-2222' \
  ' const RetryLimit = 3' \
  >"$vuln_diff"

# fakegithub: a loopback-only fake GitHub API serving vuln_diff above (not
# the shared pr-42.diff) plus AUR-438's own read-write/read-only permission
# fixtures and comment-created fixture. Every request is appended to a log
# file this script inspects.
fakegithub_src="$run_dir/fakegithub.go"
cat >"$fakegithub_src" <<'EOF'
package main

import (
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

func main() {
	fixturesDir := os.Args[1]
	diffPath := os.Args[2]
	scenario := os.Args[3]
	logPath := os.Args[4]

	diffBody := mustRead(diffPath)
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

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			fmt.Fprintf(logFile, "POST %s %s\n", r.URL.Path, body)
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

FAKE_URL=""
start_fake() {
  local scenario="$1" log="$2" url_file="$3"
  "$fakegithub_bin" "$fixtures_dir" "$vuln_diff" "$scenario" "$log" >"$url_file" 2>>"$run_dir/fakegithub.stderr" &
  pids+=("$!")
  local waited=0
  while [[ ! -s "$url_file" ]]; do
    sleep 0.05
    waited=$((waited + 1))
    ((waited < 100)) || infra fakegithub_did_not_start
  done
  FAKE_URL="$(head -n1 "$url_file")"
}

# A deterministic offline model response with zero quality findings, so
# every finding a scenario observes below comes from --seguranca, never
# from the (irrelevant, here) quality review.
fixture_empty="$run_dir/response-empty.json"
cat >"$fixture_empty" <<'EOF'
{
  "issues": [],
  "summary": "Nenhum achado de qualidade para AUR-451 (e2e)."
}
EOF

run_review() {
  local base_url="$1" fixture="$2" sha="$3"; shift 3
  set +e
  if [[ -n "$sha" ]]; then
    AURUMCODE_LLM_FIXTURE="$fixture" AURUMCODE_GITHUB_API_URL="$base_url" \
      GITHUB_TOKEN="token-sintetico-write" GITHUB_SHA="$sha" \
      "$bin" review --pr 42 --repo dono/projeto --publicar --na-linha "$@" \
      >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  else
    env -u GITHUB_SHA \
      AURUMCODE_LLM_FIXTURE="$fixture" AURUMCODE_GITHUB_API_URL="$base_url" \
      GITHUB_TOKEN="token-sintetico-write" \
      "$bin" review --pr 42 --repo dono/projeto --publicar --na-linha "$@" \
      >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  fi
  rc=$?
  set -e
}

readonly sha1="e2e-451-sha-1"

## Scenario 1 (AC-001): --seguranca plus --fail-on high. The planted
## secret must be published as an inline comment citing its rule, the
## coverage note must appear on stderr, and the gate must close (exit 3).
log1="$run_dir/ac001.log"
start_fake write "$log1" "$run_dir/ac001.url"
run_review "$FAKE_URL" "$fixture_empty" "$sha1" --seguranca --fail-on high
[[ "$rc" -eq 3 ]] || fail "ac001_wrong_exit:$rc"
grep -Fq 'cmdb/settings.go:3:' "$run_dir/out.stdout" || fail ac001_missing_finding_line
grep -Fq '[error]' "$run_dir/out.stdout" || fail ac001_missing_severity
grep -Fq 'rule security/hardcoded-secret' "$run_dir/out.stdout" || fail ac001_missing_rule_citation
grep -Fq -- '-- publicado na linha' "$run_dir/out.stdout" || fail ac001_missing_inline_marker
grep -Fq 'security pass applied' "$run_dir/out.stderr" || fail ac001_missing_coverage_note
grep -Fq 'finding(s) at severity error or above (--fail-on error)' "$run_dir/out.stderr" || fail ac001_missing_gate_note
post_count="$(grep -c "^POST /repos/dono/projeto/pulls/42/comments " "$log1" || true)"
[[ "$post_count" -eq 1 ]] || fail "ac001_wrong_post_count:$post_count"
ac001_first_stdout="$(cat "$run_dir/out.stdout")"

## Determinism: rerunning the exact same input against a fresh server
## reproduces the exact same stdout and exit code.
log1b="$run_dir/ac001b.log"
start_fake write "$log1b" "$run_dir/ac001b.url"
run_review "$FAKE_URL" "$fixture_empty" "$sha1" --seguranca --fail-on high
[[ "$rc" -eq 3 ]] || fail "ac001_rerun_wrong_exit:$rc"
[[ "$(cat "$run_dir/out.stdout")" == "$ac001_first_stdout" ]] || fail non_deterministic

## Scenario 2: --seguranca alone (no --fail-on) still publishes the
## finding, but the gate does not close -- exit 0, exactly the --base
## path's own "the gate is opt-in" contract (docs/specs/AUR-431.md).
log2="$run_dir/seguranca_only.log"
start_fake write "$log2" "$run_dir/seguranca_only.url"
run_review "$FAKE_URL" "$fixture_empty" "$sha1" --seguranca
[[ "$rc" -eq 0 ]] || fail "seguranca_only_wrong_exit:$rc"
grep -Fq 'rule security/hardcoded-secret' "$run_dir/out.stdout" || fail seguranca_only_missing_finding
[[ "$(grep -c "^POST " "$log2")" -eq 1 ]] || fail seguranca_only_wrong_post_count

## Scenario 3: the pre-AUR-451 contract, no new flags at all, against this
## same vulnerable diff. Nothing but --seguranca can ever see the planted
## secret, so this must still report "No issues found." and post nothing
## -- proving the fix is additive, not a behavior change on the existing
## surface.
log3="$run_dir/plain.log"
start_fake write "$log3" "$run_dir/plain.url"
run_review "$FAKE_URL" "$fixture_empty" "$sha1"
[[ "$rc" -eq 0 ]] || fail "plain_wrong_exit:$rc"
grep -Fq 'No issues found.' "$run_dir/out.stdout" || fail plain_missing_no_issues
if grep -q '^POST ' "$log3"; then
  fail plain_post_leaked
fi

## Scenario 4: --limite far below the diff's cost refuses before any
## comment is published -- the AUR-433 gate now caps the --pr path too.
log4="$run_dir/limite.log"
start_fake write "$log4" "$run_dir/limite.url"
run_review "$FAKE_URL" "$fixture_empty" "$sha1" --seguranca --limite 0.0001
[[ "$rc" -eq 1 ]] || fail "limite_wrong_exit:$rc"
grep -Fq 'refusing to call the model' "$run_dir/out.stderr" || fail limite_missing_refusal
if grep -q '^POST ' "$log4"; then
  fail limite_post_leaked
fi

## Scenario 5: --modelo names a model nothing can serve. The command must
## still fail loudly (reportModelUnavailable, AUR-436's contract), never
## an empty review with exit 0, and post nothing.
log5="$run_dir/modelo.log"
start_fake write "$log5" "$run_dir/modelo.url"
set +e
AURUMCODE_GITHUB_API_URL="$FAKE_URL" GITHUB_TOKEN="token-sintetico-write" GITHUB_SHA="$sha1" \
  "$bin" review --pr 42 --repo dono/projeto --publicar --na-linha --modelo local \
  >"$run_dir/modelo.stdout" 2>"$run_dir/modelo.stderr"
rc=$?
set -e
[[ "$rc" -eq 1 ]] || fail "modelo_wrong_exit:$rc"
grep -Fq 'model "local" is unavailable' "$run_dir/modelo.stderr" || fail modelo_missing_message
if grep -q '^POST ' "$log5"; then
  fail modelo_post_leaked
fi

## Scenario 6: a read-only token is still refused before anything is
## fetched or published -- AUR-437's/AUR-438's fail-closed guarantee is
## unaffected by this card's four new flags.
log6="$run_dir/readonly.log"
start_fake readonly "$log6" "$run_dir/readonly.url"
run_review "$FAKE_URL" "$fixture_empty" "$sha1" --seguranca --fail-on high
[[ "$rc" -eq 1 ]] || fail "readonly_wrong_exit:$rc"
grep -Fq 'refusing to publish' "$run_dir/out.stderr" || fail readonly_not_refused
if grep -q '^POST ' "$log6"; then
  fail readonly_post_leaked
fi

printf '%s/AC-001/E2EAUR451/ok\n' "$card"
