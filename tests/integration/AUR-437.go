package integration

// Integration selector for card AUR-437: the full review-publication flow of
// the restored GitHub client against a single httptest server built from the
// card's fixtures (tests/fixtures/scm/github): read the PR diff (ETag cache
// included), list the changed files across pages, publish the findings as
// comments with a write token, and refuse to publish with a read-only token.
// Everything runs offline; tokens are synthetic runtime-only strings.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Mpaape/AurumCode/internal/git/githubclient"
)

func aur437IntegrationRoot(t *testing.T) string {
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

func aur437IntegrationFixture(t *testing.T, name string) []byte {
	t.Helper()
	p := filepath.Join(aur437IntegrationRoot(t), "tests", "fixtures", "scm", "github", name)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("fixture obrigatoria ausente %s: %v", p, err)
	}
	return b
}

// aur437Server is the deterministic fake GitHub built from the card's
// fixtures. It records what was published and never requires the network.
type aur437Server struct {
	t *testing.T

	mu             sync.Mutex
	diffRequests   int
	reviewComments []string
	issueComments  []string
	statuses       []string
}

func (s *aur437Server) handler() http.HandlerFunc {
	diffBody := aur437IntegrationFixture(s.t, "pr-42.diff")
	page1 := aur437IntegrationFixture(s.t, "pr-42-files-page1.json")
	page2 := aur437IntegrationFixture(s.t, "pr-42-files-page2.json")
	readWrite := aur437IntegrationFixture(s.t, "repo-read-write.json")
	readOnly := aur437IntegrationFixture(s.t, "repo-read-only.json")
	created := aur437IntegrationFixture(s.t, "comment-created.json")

	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/dono/projeto":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if strings.Contains(r.Header.Get("Authorization"), "escrita") {
				_, _ = w.Write(readWrite)
			} else {
				_, _ = w.Write(readOnly)
			}

		case r.Method == http.MethodGet && r.URL.Path == "/repos/dono/projeto/pulls/42":
			s.mu.Lock()
			s.diffRequests++
			n := s.diffRequests
			s.mu.Unlock()
			if r.Header.Get("If-None-Match") == `"aur437-etag"` && n > 1 {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", `"aur437-etag"`)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(diffBody)

		case r.Method == http.MethodGet && r.URL.Path == "/repos/dono/projeto/pulls/42/files":
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("page") == "2" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(page2)
				return
			}
			w.Header().Set("Link", `<http://`+r.Host+`/repos/dono/projeto/pulls/42/files?page=2>; rel="next"`)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(page1)

		case r.Method == http.MethodPost && r.URL.Path == "/repos/dono/projeto/pulls/42/comments":
			body, _ := io.ReadAll(r.Body)
			s.mu.Lock()
			s.reviewComments = append(s.reviewComments, string(body))
			s.mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write(created)

		case r.Method == http.MethodPost && r.URL.Path == "/repos/dono/projeto/issues/42/comments":
			body, _ := io.ReadAll(r.Body)
			s.mu.Lock()
			s.issueComments = append(s.issueComments, string(body))
			s.mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write(created)

		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/repos/dono/projeto/statuses/"):
			s.mu.Lock()
			s.statuses = append(s.statuses, r.URL.Path)
			s.mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"state":"success"}`))

		default:
			s.t.Errorf("requisicao inesperada: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

// IntegrationAUR437 is the integration selector declared by the card.
func IntegrationAUR437(t *testing.T) {
	fake := &aur437Server{t: t}
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	ctx := context.Background()

	// Synthetic runtime-only tokens: the fake maps "escrita" to the
	// read-write permission fixture, anything else to read-only.
	writeClient := githubclient.NewClientWithBaseURL("token-sintetico-escrita", server.URL)
	readClient := githubclient.NewClientWithBaseURL("token-sintetico-leitura", server.URL)

	// 1. Read the PR: diff with the corrected paths, files across pages.
	diff, err := writeClient.GetPullRequestDiff(ctx, "dono", "projeto", 42)
	if err != nil {
		t.Fatalf("GetPullRequestDiff falhou: %v", err)
	}
	if len(diff.Files) != 2 || diff.Files[0].Path != "cmdb/settings.go" || diff.Files[1].Path != "docs/notas.md" {
		t.Fatalf("diff com caminhos errados: %+v", diff.Files)
	}

	files, err := writeClient.ListChangedFiles(ctx, "dono", "projeto", 42)
	if err != nil {
		t.Fatalf("ListChangedFiles falhou: %v", err)
	}
	if len(files) != 2 || files[0] != "cmdb/settings.go" || files[1] != "docs/notas.md" {
		t.Fatalf("paginacao errada: %v", files)
	}

	// Repeating the read serves the cached diff via ETag/304 and must be
	// byte-for-byte identical (determinism the card promises).
	diff2, err := writeClient.GetPullRequestDiff(ctx, "dono", "projeto", 42)
	if err != nil {
		t.Fatalf("segunda leitura do diff falhou: %v", err)
	}
	if len(diff2.Files) != len(diff.Files) {
		t.Fatalf("diff em cache difere do original")
	}

	// 2. Publish the findings with the write token: one inline comment per
	// changed file plus the summary comment.
	ok, err := writeClient.HasWritePermission(ctx, "dono", "projeto")
	if err != nil || !ok {
		t.Fatalf("HasWritePermission(escrita) = %v, %v; esperava true", ok, err)
	}

	for _, f := range diff.Files {
		comment := githubclient.ReviewComment{
			Body:     "Problema encontrado em " + f.Path,
			CommitID: "abc123def456",
			Path:     f.Path,
			Line:     3,
		}
		if err := writeClient.PostReviewComment(ctx, "dono", "projeto", 42, comment, "aur437-"+f.Path); err != nil {
			t.Fatalf("PostReviewComment(%s) falhou: %v", f.Path, err)
		}
	}
	if err := writeClient.PostIssueComment(ctx, "dono", "projeto", 42, "Review AurumCode: 2 problemas publicados."); err != nil {
		t.Fatalf("PostIssueComment falhou: %v", err)
	}
	if err := writeClient.SetStatus(ctx, "dono", "projeto", "abc123def456", githubclient.CommitStatus{State: "success", Context: "aurumcode/review"}); err != nil {
		t.Fatalf("SetStatus falhou: %v", err)
	}

	fake.mu.Lock()
	reviews := len(fake.reviewComments)
	issues := len(fake.issueComments)
	statuses := len(fake.statuses)
	firstReview := ""
	if reviews > 0 {
		firstReview = fake.reviewComments[0]
	}
	fake.mu.Unlock()

	if reviews != 2 || issues != 1 || statuses != 1 {
		t.Fatalf("publicacao incompleta: reviews=%d issues=%d statuses=%d", reviews, issues, statuses)
	}
	if !strings.Contains(firstReview, `"path":"cmdb/settings.go"`) || !strings.Contains(firstReview, `"line":3`) {
		t.Fatalf("comentario inline sem path/linha corretos: %s", firstReview)
	}

	// 3. The read-only token reads the PR but is refused on publish, and no
	// additional POST reaches the server.
	if _, err := readClient.GetPullRequestDiff(ctx, "dono", "projeto", 42); err != nil {
		t.Fatalf("leitura com token somente-leitura falhou: %v", err)
	}
	if ok, err := readClient.HasWritePermission(ctx, "dono", "projeto"); err != nil || ok {
		t.Fatalf("HasWritePermission(leitura) = %v, %v; esperava false", ok, err)
	}
	err = readClient.PostIssueComment(ctx, "dono", "projeto", 42, "nao deveria publicar")
	if !errors.Is(err, githubclient.ErrNoWritePermission) {
		t.Fatalf("esperava ErrNoWritePermission, obteve: %v", err)
	}

	fake.mu.Lock()
	leaked := len(fake.issueComments) != issues || len(fake.reviewComments) != reviews
	fake.mu.Unlock()
	if leaked {
		t.Fatal("um POST chegou ao servidor apesar do token somente-leitura")
	}
}
