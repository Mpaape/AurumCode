#!/usr/bin/env bash
# E2E check for AUR-437: build a small harness that uses the restored
# internal/git/githubclient library exactly as AUR-438's CLI will — read the
# pull request's changes from a fake GitHub built from the card's fixtures,
# publish the findings as comments with a write token, and refuse to publish
# with a read-only token. Everything is offline (loopback only); tokens are
# synthetic runtime-only strings. See docs/specs/AUR-437.md.
set -euo pipefail
export LC_ALL=C

readonly card=AUR-437
selector="${1:-E2EAUR437}"
[[ "$selector" == "E2EAUR437" ]] || { printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64; }

fail() { printf '%s/AC-001/%s\n' "$card" "$1" >&2; exit 1; }
infra() { printf '%s/AC-001/infrastructure/%s\n' "$card" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

command -v go >/dev/null 2>&1 || infra missing_go

# The library and fixtures are this card's own deliverables: their absence is
# behavioral RED, not an environment problem.
test -f "$repo_root/internal/git/githubclient/client.go" || fail missing_client
test -s "$repo_root/tests/fixtures/scm/github/pr-42.diff" || fail missing_fixture_diff
test -s "$repo_root/tests/fixtures/scm/github/repo-read-only.json" || fail missing_fixture_readonly

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e2e-a437.XXXXXX")" || infra mktemp
cleanup() {
  chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true
  rm -rf -- "$run_dir" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM HUP

mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
# Reuse an already-warm build cache when the acceptance harness provides one;
# standalone invocation builds its own isolated copy.
: "${GOCACHE:=$run_dir/gocache}"
: "${GOTMPDIR:=$run_dir/gotmp}"
export GOCACHE GOTMPDIR
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS=-mod=mod

# Stage a private module copy so the harness main package can live inside the
# module without writing anything into the repository tree.
mod="$run_dir/mod"
mkdir -p "$mod/internal/git" "$mod/tests/fixtures/scm"
cp "$repo_root/go.mod" "$mod/go.mod"
[[ -f "$repo_root/go.sum" ]] && cp "$repo_root/go.sum" "$mod/go.sum"
cp -R "$repo_root/internal/git/githubclient" "$mod/internal/git/githubclient"
cp -R "$repo_root/tests/fixtures/scm/github" "$mod/tests/fixtures/scm/github"
chmod -R u+w -- "$mod"

mkdir -p "$mod/aur437e2e"
cat > "$mod/aur437e2e/main.go" <<'EOF'
// Harness for tests/e2e/AUR-437.sh: drives the restored githubclient library
// against a loopback fake GitHub built from the card's fixtures, printing a
// deterministic transcript the e2e script asserts on.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Mpaape/AurumCode/internal/git/githubclient"
)

func mustFixture(dir, name string) []byte {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		fmt.Fprintf(os.Stderr, "fixture ausente: %v\n", err)
		os.Exit(79)
	}
	return b
}

func main() {
	scenario := flag.String("scenario", "write", "write | readonly")
	fixtures := flag.String("fixtures", "tests/fixtures/scm/github", "fixture directory")
	flag.Parse()

	diffBody := mustFixture(*fixtures, "pr-42.diff")
	page1 := mustFixture(*fixtures, "pr-42-files-page1.json")
	page2 := mustFixture(*fixtures, "pr-42-files-page2.json")
	created := mustFixture(*fixtures, "comment-created.json")
	var repoJSON []byte
	if *scenario == "write" {
		repoJSON = mustFixture(*fixtures, "repo-read-write.json")
	} else {
		repoJSON = mustFixture(*fixtures, "repo-read-only.json")
	}

	var mu sync.Mutex
	postReceived := false

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			mu.Lock()
			postReceived = true
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write(created)
			return
		}
		switch r.URL.Path {
		case "/repos/dono/projeto":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(repoJSON)
		case "/repos/dono/projeto/pulls/42":
			w.Header().Set("ETag", `"aur437-etag"`)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(diffBody)
		case "/repos/dono/projeto/pulls/42/files":
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("page") == "2" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(page2)
				return
			}
			w.Header().Set("Link", "<http://"+r.Host+"/repos/dono/projeto/pulls/42/files?page=2>; rel=\"next\"")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(page1)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "loopback indisponivel: %v\n", err)
		os.Exit(79)
	}
	server := &http.Server{Handler: handler}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()

	baseURL := "http://" + listener.Addr().String()

	// Synthetic runtime-only token; no credential shape ever touches a file.
	token := "token-sintetico-" + *scenario
	client := githubclient.NewClientWithBaseURL(token, baseURL)
	ctx := context.Background()

	diff, err := client.GetPullRequestDiff(ctx, "dono", "projeto", 42)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetPullRequestDiff: %v\n", err)
		os.Exit(1)
	}
	for _, f := range diff.Files {
		fmt.Printf("ARQUIVO_DO_DIFF path=%s hunks=%d\n", f.Path, len(f.Hunks))
	}

	files, err := client.ListChangedFiles(ctx, "dono", "projeto", 42)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ListChangedFiles: %v\n", err)
		os.Exit(1)
	}
	for _, f := range files {
		fmt.Printf("ARQUIVO_ALTERADO %s\n", f)
	}

	// Publish one inline finding plus the summary comment, as the review
	// pipeline will.
	first := diff.Files[0]
	comment := githubclient.ReviewComment{
		Body:     "Problema: revisar " + first.Path,
		CommitID: "abc123def456",
		Path:     first.Path,
		Line:     3,
	}
	err = client.PostReviewComment(ctx, "dono", "projeto", 42, comment, "aur437-e2e")
	if err == nil {
		err = client.PostIssueComment(ctx, "dono", "projeto", 42, "Review AurumCode: 1 problema publicado.")
	}

	switch {
	case err == nil:
		fmt.Printf("COMENTARIO_PUBLICADO path=%s line=3\n", first.Path)
	case strings.Contains(err.Error(), "write permission"):
		fmt.Println("RECUSADO_SEM_PERMISSAO_DE_ESCRITA")
	default:
		fmt.Fprintf(os.Stderr, "erro inesperado ao publicar: %v\n", err)
		os.Exit(1)
	}

	mu.Lock()
	fmt.Printf("POST_RECEBIDO=%v\n", postReceived)
	mu.Unlock()
}
EOF

bin="$run_dir/aur437-harness"
build_log="$run_dir/build.log"
if ! (cd "$mod" && go build -o "$bin" ./aur437e2e) >"$build_log" 2>&1; then
  cat "$build_log" >&2
  fail build_failed
fi

fixtures_dir="$mod/tests/fixtures/scm/github"

run_write() { "$bin" --scenario write --fixtures "$fixtures_dir"; }
run_readonly() { "$bin" --scenario readonly --fixtures "$fixtures_dir"; }

out1="$(run_write)" || fail write_run_failed
grep -Fq 'ARQUIVO_DO_DIFF path=cmdb/settings.go' <<<"$out1" || fail missing_diff_path
grep -Fq 'ARQUIVO_ALTERADO docs/notas.md' <<<"$out1" || fail missing_paginated_file
grep -Fq 'COMENTARIO_PUBLICADO path=cmdb/settings.go line=3' <<<"$out1" || fail missing_published_comment
grep -Fq 'POST_RECEBIDO=true' <<<"$out1" || fail publish_never_reached_server

out2="$(run_write)" || fail write_rerun_failed
[[ "$out1" == "$out2" ]] || fail non_deterministic

out3="$(run_readonly)" || fail readonly_run_failed
grep -Fq 'RECUSADO_SEM_PERMISSAO_DE_ESCRITA' <<<"$out3" || fail readonly_not_refused
grep -Fq 'POST_RECEBIDO=false' <<<"$out3" || fail readonly_post_leaked

printf '%s/AC-001/E2EAUR437/ok\n' "$card"
