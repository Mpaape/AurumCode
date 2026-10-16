package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// AUR-623: `aurumcode plugin issues` is wired to the API root of
// AURUMCODE_GITHUB_API_URL; a usage error or an inconclusive scan makes no
// request at all.
func TestAUR623PluginIssuesVerb(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(500)
	}))
	defer srv.Close()
	t.Setenv("AURUMCODE_GITHUB_API_URL", srv.URL)
	t.Setenv("GITHUB_TOKEN", "token-falso")
	missing := filepath.Join(t.TempDir(), "plugin-vvah.sarif")
	cases := []struct {
		args []string
		rc   int
		want string
	}{
		{[]string{"issues", "--name", "vvah", "--sarif", missing}, 2, "OWNER/REPO"},
		{[]string{"issues", "--name", "vvah", "--sarif", missing, "--repo", "dono/repo"}, 1, "nenhuma issue foi tocada"},
	}
	for _, c := range cases {
		var out, errb bytes.Buffer
		if rc := runPlugin(c.args, &out, &errb, redaction.NewFilter()); rc != c.rc || !strings.Contains(errb.String(), c.want) {
			t.Fatalf("%v: rc=%d stderr=%q", c.args, rc, errb.String())
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("%d requests before the SARIF was conclusive", calls.Load())
	}
}
