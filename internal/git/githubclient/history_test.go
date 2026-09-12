package githubclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetPullRequestHistorySourcesAndAttribution(t *testing.T) {
	seen := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path]++
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "token fixture-token" || r.URL.Query().Get("per_page") != "100" || r.URL.Query().Get("page") != "1" {
			t.Errorf("incorrect history request: %s %s", r.Method, r.URL)
		}
		switch r.URL.Path {
		case "/repos/owner/repo/pulls/42/reviews":
			fmt.Fprint(w, `[
			 {"id":9,"user":{"login":"reviewer"},"body":"approve","state":"APPROVED","commit_id":"old-sha","submitted_at":"2026-01-02T00:00:00Z","created_at":"2020-01-01T00:00:00Z"},
			 {"id":8,"user":null,"body":"","state":"PENDING","submitted_at":null}
			]`)
		case "/repos/owner/repo/pulls/42/comments":
			fmt.Fprint(w, `[
			 {"id":6,"user":{"login":"replier"},"body":"reply","state":"IGNORED","commit_id":"new-sha","original_commit_id":"old-sha","path":"old.go","line":null,"original_line":12,"side":"LEFT","in_reply_to_id":5,"created_at":"2026-01-02T00:00:00Z","updated_at":"2026-01-03T00:00:00Z"},
			 {"id":5,"user":{"login":"author"},"body":"parent","line":20,"original_line":20,"side":"RIGHT","created_at":"2026-01-02T00:00:00Z"}
			]`)
		case "/repos/owner/repo/issues/42/comments":
			fmt.Fprint(w, `[
			 {"id":9,"user":null,"body":"deleted account, retained comment","state":"IGNORED","created_at":"2026-01-02T00:00:00Z"},
			 {"id":7,"user":{"login":"starter"},"body":"first","created_at":"2026-01-01T02:00:00+02:00"}
			]`)
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	got, err := NewClientWithBaseURL("fixture-token", server.URL).GetPullRequestHistory(context.Background(), "owner", "repo", 42)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, entry := range got {
		order = append(order, fmt.Sprintf("%s/%d", entry.Kind, entry.ID))
	}
	want := []string{"review/8", "comment/7", "comment/9", "inline/5", "inline/6", "review/9"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v; want %v", order, want)
	}
	if len(seen) != 3 {
		t.Fatalf("sources = %v", seen)
	}
	reply := got[4]
	if reply.Author != "replier" || reply.Body != "reply" || reply.CommitID != "new-sha" || reply.OriginalCommitID != "old-sha" || reply.Path != "old.go" || reply.Line != nil || reply.OriginalLine == nil || *reply.OriginalLine != 12 || reply.Side != "LEFT" || reply.InReplyToID == nil || *reply.InReplyToID != 5 || reply.UpdatedAt == nil || reply.UpdatedAt.Format(time.RFC3339) != "2026-01-03T00:00:00Z" {
		t.Fatalf("lost reply/old-line attribution: %+v", reply)
	}
	if got[3].Line == nil || *got[3].Line != 20 || got[3].Side != "RIGHT" || got[3].InReplyToID != nil {
		t.Fatalf("lost current-line attribution: %+v", got[3])
	}
	if got[0].CreatedAt != nil || got[0].Author != "" || got[2].Author != "" || got[5].State != "APPROVED" || got[5].Author != "reviewer" || got[5].CommitID != "old-sha" {
		t.Fatalf("lost review/deleted-user attribution: %+v", got)
	}
	encoded, err := json.Marshal(reply)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"kind", "id", "body", "author", "commit_id", "original_commit_id", "path", "line", "original_line", "side", "in_reply_to_id", "created_at", "updated_at"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("missing JSON field %s", key)
		}
	}
	if _, ok := fields["state"]; ok {
		t.Error("non-review state must be omitted")
	}
}

func TestGetPullRequestHistoryPaginationIgnoresHostileURLs(t *testing.T) {
	var leaked atomic.Int32
	hostile := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked.Add(1)
		fmt.Fprint(w, `[]`)
	}))
	defer hostile.Close()
	for _, target := range []string{hostile.URL + "/steal?page=900", "/other/private/path?page=900", "https://fixture-token@attacker.invalid/steal?page=900"} {
		t.Run(target, func(t *testing.T) {
			seen := make(map[string][]string)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/o/r/pulls/1/reviews", "/repos/o/r/pulls/1/comments", "/repos/o/r/issues/1/comments":
				default:
					leaked.Add(1)
					w.WriteHeader(404)
					return
				}
				page := r.URL.Query().Get("page")
				seen[r.URL.Path] = append(seen[r.URL.Path], page)
				if r.Header.Get("Authorization") != "token fixture-token" || r.URL.Query().Get("per_page") != "100" {
					t.Error("missing auth or incorrect page size")
				}
				if page == "1" {
					w.Header().Add("Link", `<ignored>; rel="last"`)
					w.Header().Add("Link", "<"+target+">; rel=\"next\"")
				} else if page != "2" {
					t.Errorf("followed untrusted page: %s", page)
				}
				fmt.Fprintf(w, `[{"id":%s,"body":"page %s"}]`, page, page)
			}))
			defer server.Close()
			got, err := NewClientWithBaseURL("fixture-token", server.URL).GetPullRequestHistory(context.Background(), "o", "r", 1)
			if err != nil || len(got) != 6 {
				t.Fatalf("history length = %d, error = %v", len(got), err)
			}
			if len(seen) != 3 {
				t.Fatalf("sources = %v", seen)
			}
			for path, pages := range seen {
				if !reflect.DeepEqual(pages, []string{"1", "2"}) {
					t.Errorf("%s pages = %v", path, pages)
				}
			}
		})
	}
	if leaked.Load() != 0 {
		t.Fatalf("%d requests escaped their endpoint", leaked.Load())
	}
}

func TestGetPullRequestHistoryRejectsRedirects(t *testing.T) {
	var escaped atomic.Int32
	hostile := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { escaped.Add(1) }))
	defer hostile.Close()
	for _, target := range []string{hostile.URL + "/steal", "/steal"} {
		t.Run(target, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/steal" {
					escaped.Add(1)
				}
				http.Redirect(w, r, target, http.StatusFound)
			}))
			defer server.Close()
			got, err := NewClientWithBaseURL("fixture-token", server.URL).GetPullRequestHistory(context.Background(), "o", "r", 1)
			if err == nil || !strings.Contains(err.Error(), "302") || got != nil {
				t.Fatalf("history = %v, error = %v", got, err)
			}
		})
	}
	if escaped.Load() != 0 {
		t.Fatal("redirect escaped original endpoint")
	}
}

func TestGetPullRequestHistoryFailureIsNotPartialSuccess(t *testing.T) {
	for _, source := range []string{"/pulls/1/reviews", "/pulls/1/comments", "/issues/1/comments"} {
		for _, tc := range []struct {
			name   string
			status int
			body   string
		}{
			{"malformed", 200, `[{"body":"fixture-token"`},
			{"wrong-shape", 200, `{}`},
			{"null", 200, `null`},
			{"null-entry", 200, `[null]`},
			{"missing-id", 200, `[{}]`},
			{"trailing-json", 200, `[] {"secret":"fixture-token"}`},
			{"invalid-time", 200, `[{"id":1,"created_at":"fixture-token"}]`},
			{"forbidden", 403, "fixture-token"},
			{"missing", 404, "fixture-token"},
			{"rate-limit", 429, "fixture-token"},
			{"server", 500, "fixture-token"},
		} {
			t.Run(source+"/"+tc.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.HasSuffix(r.URL.Path, source) && r.URL.Query().Get("page") == "2" {
						w.WriteHeader(tc.status)
						fmt.Fprint(w, tc.body)
						return
					}
					if strings.HasSuffix(r.URL.Path, source) {
						w.Header().Set("Link", `<ignored>; rel="next"`)
					}
					fmt.Fprint(w, `[{"id":1}]`)
				}))
				defer server.Close()
				got, err := NewClientWithBaseURL("fixture-token", server.URL).GetPullRequestHistory(context.Background(), "o", "r", 1)
				if err == nil || got != nil {
					t.Fatalf("history = %v, error = %v", got, err)
				}
				if strings.Contains(err.Error(), "fixture-token") || !strings.Contains(err.Error(), "page 2") {
					t.Fatalf("unsafe or unattributed error: %v", err)
				}
				if tc.status != 200 && !strings.Contains(err.Error(), strconv.Itoa(tc.status)) {
					t.Errorf("missing HTTP status: %v", err)
				}
			})
		}
	}
}

func TestGetPullRequestHistoryCancellation(t *testing.T) {
	for _, phase := range []string{"before-request", "during-headers", "during-body", "between-pages"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				switch phase {
				case "during-body":
					fmt.Fprint(w, `[{"id":1`)
					w.(http.Flusher).Flush()
					cancel()
					<-r.Context().Done()
				case "between-pages":
					w.Header().Set("Link", `<ignored>; rel="next"`)
					fmt.Fprint(w, `[{"id":1}]`)
					cancel()
				default:
					cancel()
					<-r.Context().Done()
				}
			}))
			defer server.Close()
			if phase == "before-request" {
				cancel()
			}
			got, err := NewClientWithBaseURL("fixture-token", server.URL).GetPullRequestHistory(ctx, "o", "r", 1)
			if !errors.Is(err, context.Canceled) || got != nil {
				t.Fatalf("history = %v, error = %v", got, err)
			}
			if calls.Load() > 1 || (phase == "before-request" && calls.Load() != 0) {
				t.Fatalf("unexpected requests after cancellation: %d", calls.Load())
			}
		})
	}
}

func TestGetPullRequestHistoryPreservesFullBodyAndEmptyHistory(t *testing.T) {
	body := strings.Repeat("raw untrusted text\n", 100000)
	for _, empty := range []bool{false, true} {
		t.Run(strconv.FormatBool(empty), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if empty || !strings.HasSuffix(r.URL.Path, "/reviews") {
					fmt.Fprint(w, `[]`)
					return
				}
				json.NewEncoder(w).Encode([]map[string]interface{}{{"id": 1, "body": body}})
			}))
			defer server.Close()
			got, err := NewClientWithBaseURL("", server.URL).GetPullRequestHistory(context.Background(), "o", "r", 1)
			if err != nil || got == nil {
				t.Fatalf("history = %v, error = %v", got, err)
			}
			if empty {
				if len(got) != 0 {
					t.Fatal("expected empty history")
				}
			} else if len(got) != 1 || got[0].Body != body {
				t.Fatal("body was altered or truncated")
			}
		})
	}
}

func TestHistoryHasNext(t *testing.T) {
	for _, tc := range []struct {
		link string
		want bool
	}{
		{`<https://example.invalid/next>; rel="last"`, false},
		{`<ignored>; rel="prev", <ignored>; rel="next"`, true},
		{`<ignored>; type="application/json"; rel="prev next"`, true},
		{`<ignored>; rel=next`, true},
		{"", false},
	} {
		if got := historyHasNext([]string{tc.link}); got != tc.want {
			t.Errorf("historyHasNext(%q) = %v; want %v", tc.link, got, tc.want)
		}
	}
}
