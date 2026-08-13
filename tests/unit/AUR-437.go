package unit

// Unit selector for card AUR-437: the restored GitHub client
// (internal/git/githubclient) with its four measured restoration defects
// fixed, plus the write-permission gate that refuses to publish with a
// read-only token.
//
// Every subtest runs against a local net/http/httptest server; nothing here
// touches the network. Tokens are synthetic strings assembled at runtime and
// carry no real credential shape. Each of the four defect subtests fails
// against the client as restored from commit c12d7ab and passes only with
// the declared fix (see docs/specs/AUR-437.md for the before/after map).
//
// The harness bridges this file with a generated _test.go shim (see
// tests/acceptance/AUR-437.sh), the same pattern the other cards in this
// wave use, so the file itself is a plain package file.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/git/githubclient"
)

// aur437Root resolves the repository root. The acceptance harness sets
// AURUMCODE_ROOT to the staged materialization root; running the bridge
// directly from a full checkout climbs two directories from tests/unit.
func aur437Root(t *testing.T) string {
	t.Helper()
	if r := os.Getenv("AURUMCODE_ROOT"); r != "" {
		return r
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolvendo a raiz do repositorio: %v", err)
	}
	return root
}

func aur437Fixture(t *testing.T, name string) []byte {
	t.Helper()
	p := filepath.Join(aur437Root(t), "tests", "fixtures", "scm", "github", name)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("fixture obrigatoria ausente %s: %v", p, err)
	}
	return b
}

// Synthetic tokens, assembled at runtime only. They are opaque strings the
// fake server maps to a permission fixture; no credential-shaped byte
// sequence ever exists in a tracked file.
func aur437WriteToken() string  { return "synthetic-" + "write-" + "token" }
func aur437ReadToken() string   { return "synthetic-" + "read-only-" + "token" }
func aur437AuthHeader(tok string) string { return "token " + tok }

// permissionAwareHandler serves GET /repos/dono/projeto from the permission
// fixture selected by the Authorization header, and delegates everything
// else to next.
func aur437PermissionRoute(t *testing.T, next http.HandlerFunc) http.HandlerFunc {
	t.Helper()
	readWrite := aur437Fixture(t, "repo-read-write.json")
	readOnly := aur437Fixture(t, "repo-read-only.json")
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/repos/dono/projeto" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if r.Header.Get("Authorization") == aur437AuthHeader(aur437WriteToken()) {
				_, _ = w.Write(readWrite)
			} else {
				_, _ = w.Write(readOnly)
			}
			return
		}
		next(w, r)
	}
}

// TestAUR437 is the unit selector declared by the card (TDD proof: Unit).
func TestAUR437(t *testing.T) {
	t.Run("Defeito1RetryReenviaOCorpoCompleto", func(t *testing.T) {
		// Restored defect: doRequest retries POSTs by cloning the request,
		// and http.Request.Clone shares the already-consumed body reader, so
		// the retried attempt carries an empty body. The fix replays via
		// GetBody on every attempt after the first. The 429 answer closes
		// the connection on purpose: net/http's own rewindBody rescue only
		// fires on a REUSED connection, so a rate limiter that closes the
		// connection is exactly the case where the shared reader surfaces
		// as "http: ContentLength=N with Body length 0". First attempt is
		// rate-limited with Retry-After: 0 (no sleep); the second attempt
		// must deliver the full JSON body.
		var mu sync.Mutex
		postAttempts := 0
		retriedBody := ""
		handler := aur437PermissionRoute(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && r.URL.Path == "/repos/dono/projeto/issues/42/comments" {
				body, _ := io.ReadAll(r.Body)
				mu.Lock()
				postAttempts++
				attempt := postAttempts
				if attempt > 1 {
					retriedBody = string(body)
				}
				mu.Unlock()
				if attempt == 1 {
					w.Header().Set("Retry-After", "0")
					w.Header().Set("Connection", "close")
					w.WriteHeader(http.StatusTooManyRequests)
					return
				}
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id": 437100}`))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		})
		server := httptest.NewServer(handler)
		defer server.Close()

		client := githubclient.NewClientWithBaseURL(aur437WriteToken(), server.URL)
		err := client.PostIssueComment(context.Background(), "dono", "projeto", 42, "corpo de teste AUR-437")
		if err != nil {
			t.Fatalf("PostIssueComment apos rate limit deveria ter sucesso, obteve: %v", err)
		}
		mu.Lock()
		attempts, got := postAttempts, retriedBody
		mu.Unlock()
		if attempts != 2 {
			t.Fatalf("esperava 2 tentativas de POST, obteve %d", attempts)
		}
		if !strings.Contains(got, "corpo de teste AUR-437") {
			t.Fatalf("a retentativa reenviou o corpo consumido: corpo recebido na 2a tentativa = %q", got)
		}
	})

	t.Run("Defeito2RateLimit403ERetentado", func(t *testing.T) {
		// Restored defect: only 429 is treated as rate limit. GitHub's
		// primary rate limiter answers 403 with X-RateLimit-Remaining: 0;
		// the restored client returns that as a hard error instead of
		// retrying.
		var mu sync.Mutex
		attempts := 0
		diffBody := aur437Fixture(t, "pr-42.diff")
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			attempts++
			n := attempts
			mu.Unlock()
			if n == 1 {
				w.Header().Set("Retry-After", "0")
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
				return
			}
			w.Header().Set("ETag", `"aur437-etag"`)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(diffBody)
		}))
		defer server.Close()

		client := githubclient.NewClientWithBaseURL(aur437WriteToken(), server.URL)
		diff, err := client.GetPullRequestDiff(context.Background(), "dono", "projeto", 42)
		if err != nil {
			t.Fatalf("403 de rate limit deveria ser retentado, obteve erro: %v", err)
		}
		mu.Lock()
		n := attempts
		mu.Unlock()
		if n != 2 {
			t.Fatalf("esperava 2 tentativas (403 rate-limit + sucesso), obteve %d", n)
		}
		if len(diff.Files) != 2 {
			t.Fatalf("esperava 2 arquivos no diff, obteve %d", len(diff.Files))
		}
	})

	t.Run("Defeito2Forbidden403SimplesNaoRetenta", func(t *testing.T) {
		// Guard: a plain 403 (real permission denial, no rate-limit
		// signal) must NOT be retried — retrying it would mask the
		// permission refusal this card exists to surface.
		var mu sync.Mutex
		attempts := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			attempts++
			mu.Unlock()
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"Forbidden"}`))
		}))
		defer server.Close()

		client := githubclient.NewClientWithBaseURL(aur437WriteToken(), server.URL)
		_, err := client.GetPullRequestDiff(context.Background(), "dono", "projeto", 42)
		if err == nil {
			t.Fatal("403 simples deveria ser erro, obteve nil")
		}
		if !strings.Contains(err.Error(), "403") {
			t.Fatalf("esperava 403 na mensagem de erro, obteve: %v", err)
		}
		mu.Lock()
		n := attempts
		mu.Unlock()
		if n != 1 {
			t.Fatalf("403 simples nao deve ser retentado: esperava 1 tentativa, obteve %d", n)
		}
	})

	t.Run("Defeito3BackoffRespeitaOContexto", func(t *testing.T) {
		// Restored defect: the retry backoff sleeps with time.Sleep, which
		// ignores context cancellation; a canceled caller still waits the
		// full backoff (InitialBackoff = 1s). With the fix the wait aborts
		// as soon as the context is done, so the call returns in ~60ms.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		client := githubclient.NewClientWithBaseURL(aur437WriteToken(), server.URL)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
		defer cancel()

		start := time.Now()
		_, err := client.GetPullRequestDiff(ctx, "dono", "projeto", 42)
		elapsed := time.Since(start)
		if err == nil {
			t.Fatal("esperava erro de contexto, obteve nil")
		}
		if !strings.Contains(err.Error(), "context") {
			t.Fatalf("esperava erro de contexto, obteve: %v", err)
		}
		// Before the fix the call only observes cancellation after the full
		// 1s time.Sleep; after the fix it returns right at the 60ms
		// deadline. 700ms splits the two outcomes with wide margin on both
		// sides.
		if elapsed >= 700*time.Millisecond {
			t.Fatalf("backoff ignorou o contexto cancelado: retornou apos %v (limite 700ms)", elapsed)
		}
	})

	t.Run("Defeito4CaminhoComDiretorioTerminadoEmB", func(t *testing.T) {
		// Restored defect: extractFilePath scans for the first literal
		// "b/" in the header, so "diff --git a/cmdb/settings.go
		// b/cmdb/settings.go" matches inside "cmdb/" and yields
		// "settings.go" instead of "cmdb/settings.go".
		diffBody := aur437Fixture(t, "pr-42.diff")
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("ETag", `"aur437-etag"`)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(diffBody)
		}))
		defer server.Close()

		client := githubclient.NewClientWithBaseURL(aur437WriteToken(), server.URL)
		diff, err := client.GetPullRequestDiff(context.Background(), "dono", "projeto", 42)
		if err != nil {
			t.Fatalf("GetPullRequestDiff falhou: %v", err)
		}
		if len(diff.Files) != 2 {
			t.Fatalf("esperava 2 arquivos no diff, obteve %d", len(diff.Files))
		}
		if got := diff.Files[0].Path; got != "cmdb/settings.go" {
			t.Fatalf("caminho errado para diretorio terminado em 'b': esperava %q, obteve %q", "cmdb/settings.go", got)
		}
		if got := diff.Files[1].Path; got != "docs/notas.md" {
			t.Fatalf("caminho errado: esperava %q, obteve %q", "docs/notas.md", got)
		}
	})

	t.Run("RecusaPublicarComTokenSomenteLeitura", func(t *testing.T) {
		// MUT-001 vector: with a token whose repository permission fixture
		// says push=false, every publishing method must refuse before any
		// POST reaches the server.
		var mu sync.Mutex
		posted := false
		handler := aur437PermissionRoute(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				mu.Lock()
				posted = true
				mu.Unlock()
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id": 437100}`))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		})
		server := httptest.NewServer(handler)
		defer server.Close()

		client := githubclient.NewClientWithBaseURL(aur437ReadToken(), server.URL)
		ctx := context.Background()

		if err := client.PostIssueComment(ctx, "dono", "projeto", 42, "resumo do review"); err == nil {
			t.Fatal("PostIssueComment publicou sem permissao de escrita")
		} else if !strings.Contains(strings.ToLower(err.Error()), "permission") {
			t.Fatalf("recusa deveria citar a permissao de escrita, obteve: %v", err)
		}

		comment := githubclient.ReviewComment{Body: "problema", CommitID: "abc123", Path: "cmdb/settings.go", Line: 3}
		if err := client.PostReviewComment(ctx, "dono", "projeto", 42, comment, "chave-437"); err == nil {
			t.Fatal("PostReviewComment publicou sem permissao de escrita")
		}

		status := githubclient.CommitStatus{State: "success", Context: "aurumcode/review"}
		if err := client.SetStatus(ctx, "dono", "projeto", "abc123", status); err == nil {
			t.Fatal("SetStatus publicou sem permissao de escrita")
		}

		mu.Lock()
		leaked := posted
		mu.Unlock()
		if leaked {
			t.Fatal("um POST chegou ao servidor apesar do token somente-leitura")
		}
	})

	t.Run("PublicaComTokenDeEscrita", func(t *testing.T) {
		// The same flow with a read-write token must publish, and the
		// inline review comment must carry the corrected path and line.
		var mu sync.Mutex
		reviewBody := ""
		handler := aur437PermissionRoute(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && r.URL.Path == "/repos/dono/projeto/pulls/42/comments" {
				body, _ := io.ReadAll(r.Body)
				mu.Lock()
				reviewBody = string(body)
				mu.Unlock()
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id": 437100}`))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		})
		server := httptest.NewServer(handler)
		defer server.Close()

		client := githubclient.NewClientWithBaseURL(aur437WriteToken(), server.URL)
		comment := githubclient.ReviewComment{
			Body:     "Problema: revisar cmdb/settings.go",
			CommitID: "abc123",
			Path:     "cmdb/settings.go",
			Line:     3,
		}
		if err := client.PostReviewComment(context.Background(), "dono", "projeto", 42, comment, "chave-437"); err != nil {
			t.Fatalf("PostReviewComment com token de escrita falhou: %v", err)
		}
		mu.Lock()
		got := reviewBody
		mu.Unlock()
		if !strings.Contains(got, `"path":"cmdb/settings.go"`) || !strings.Contains(got, `"line":3`) {
			t.Fatalf("comentario inline sem path/linha esperados: %s", got)
		}
	})
}
