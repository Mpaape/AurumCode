package githubclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// AUR-610: the pull request metadata carries the author's login and account
// type as GitHub reports them; a payload without user leaves both empty (a
// person, for the changelog check).
func TestAUR610MetadataCarriesTheAuthor(t *testing.T) {
	cases := []struct {
		name, payload, login, typ string
	}{
		{"bot", `{"title":"t","body":"b","head":{"sha":"h1"},"base":{"sha":"b1"},"user":{"login":"dependabot[bot]","type":"Bot"}}`, "dependabot[bot]", "Bot"},
		{"person", `{"title":"t","head":{"sha":"h1"},"base":{"sha":"b1"},"user":{"login":"paape","type":"User"}}`, "paape", "User"},
		{"absent", `{"title":"t","head":{"sha":"h1"},"base":{"sha":"b1"}}`, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/repos/owner/repo/pulls/9" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, c.payload)
			}))
			defer server.Close()
			meta, err := NewClientWithBaseURL("token", server.URL).GetPullRequestMetadata(context.Background(), "owner", "repo", 9)
			if err != nil {
				t.Fatal(err)
			}
			if meta.AuthorLogin != c.login || meta.AuthorType != c.typ || meta.HeadSHA != "h1" || meta.BaseSHA != "b1" || meta.Title != "t" {
				t.Fatalf("metadata = %+v, want author %q/%q", meta, c.login, c.typ)
			}
		})
	}
}
