package main

// AUR-513 behavior proof: the per-file review cache (AUR-441,
// cmd/aurumcode/review_cache.go) must invalidate whenever ANYTHING that can
// change the model's answer changes -- the real answering model/endpoint
// even once context-wrapped, the effective prompt (built-in + repo +
// central policy), every configured skill/doc's content, the selected
// profile(s), the review language, the dynamic rule/skill-section catalog
// and the relevant codebase context (AC-001, AC-002) -- while a per-file
// hit must never drop the cross-file evidence a changed sibling file still
// needs (AC-003), and a corrupted or unreadable cache must degrade to a
// fresh review, never to a silent approval or to omitted coverage (AC-004).
// Each test here fails if the behavior it names is removed. Mirrors
// aur519_e2e_test.go's / aur476_test.go's own httptest-free,
// AURUMCODE_LLM_FIXTURE + AURUMCODE_CACHE_DIR pattern, plus a real
// httptest.Server where a call count is the only trustworthy proof
// (TestAUR513ModelIdentitySurvivesContextWrapping).

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	codebasectx "github.com/Mpaape/AurumCode/internal/context"
	"github.com/Mpaape/AurumCode/internal/llm/provider/litellm"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

const aur513CleanResponse = `{"summary":"ok","verdict":"approve","issues":[]}`

func aur513WriteFixture(t *testing.T, body string) string {
	t.Helper()
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return fixture
}

// TestAUR513AC001ProfileSelectionChangeForcesFreshReview covers AC-001: a
// diff byte-identical across two rounds, reviewed under a different profile
// selection, must call the provider again (never reuse the no-profile
// round's cache entry) -- but a THIRD round repeating the exact same
// profile selection may validly reuse the second round's result. Before
// AUR-513, partitionByCache's key never varied with the profile selection
// at all (it is computed once, before runProfilePasses ever sees which
// profile(s) were chosen), so round 2 would have wrongly been served round
// 1's no-profile answer.
func TestAUR513AC001ProfileSelectionChangeForcesFreshReview(t *testing.T) {
	cleanFixture(t, "")
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur513WriteFixture(t, aur513CleanResponse))

	var out1, err1 strings.Builder
	if code := runReview([]string{"--base", "HEAD~1"}, &out1, &err1, redaction.NewFilter()); code != 0 {
		t.Fatalf("round1 exit=%d, want 0; stdout=%s stderr=%s", code, out1.String(), err1.String())
	}

	var out2, err2 strings.Builder
	if code := runReview([]string{"--base", "HEAD~1", "--perfis", "seguranca"}, &out2, &err2, redaction.NewFilter()); code != 0 {
		t.Fatalf("round2 exit=%d, want 0; stdout=%s stderr=%s", code, out2.String(), err2.String())
	}
	if strings.Contains(err2.String(), "reused") {
		t.Fatalf("AC-001: a different profile selection must never reuse the no-profile round's cache entry:\n%s", err2.String())
	}

	var out3, err3 strings.Builder
	if code := runReview([]string{"--base", "HEAD~1", "--perfis", "seguranca"}, &out3, &err3, redaction.NewFilter()); code != 0 {
		t.Fatalf("round3 exit=%d, want 0; stdout=%s stderr=%s", code, out3.String(), err3.String())
	}
	if !strings.Contains(err3.String(), "reused") {
		t.Fatalf("AC-001: repeating the exact same profile selection on the same diff must reuse the cache:\n%s", err3.String())
	}
}

// TestAUR513AC002DocContentChangeForcesFreshReview covers AC-002: editing
// the content of a configured repository doc file, with everything else
// held constant, must invalidate the cache for a byte-identical diff -- and
// the cache's own key (the on-disk entry filenames under AURUMCODE_CACHE_DIR)
// never carries the doc's text in legible form. The marker text here is
// deliberately plain prose, not secret-shaped: contextBlockCacheDigest
// hashes the REDACTED outbound block (see its own doc for why), and a
// secret-shaped span would redact to the same fixed placeholder regardless
// of its value, which would prove nothing about whether the CONFIGURED
// TEXT actually reaches the digest.
func TestAUR513AC002DocContentChangeForcesFreshReview(t *testing.T) {
	dir := cleanFixture(t, "review:\n  context:\n    docs:\n      - docs/notes.md\n")
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0700); err != nil {
		t.Fatal(err)
	}
	docPath := filepath.Join(dir, "docs", "notes.md")
	const marker = "zz-visible-marker-9f3c"
	if err := os.WriteFile(docPath, []byte("## Style\n\nPrefer small functions. guideline-"+marker+"-alpha\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur513WriteFixture(t, aur513CleanResponse))

	var out1, err1 strings.Builder
	if code := runReview([]string{"--base", "HEAD~1"}, &out1, &err1, redaction.NewFilter()); code != 0 {
		t.Fatalf("round1 exit=%d, want 0; stdout=%s stderr=%s", code, out1.String(), err1.String())
	}

	if err := os.WriteFile(docPath, []byte("## Style\n\nPrefer small functions. guideline-"+marker+"-beta\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out2, err2 strings.Builder
	if code := runReview([]string{"--base", "HEAD~1"}, &out2, &err2, redaction.NewFilter()); code != 0 {
		t.Fatalf("round2 exit=%d, want 0; stdout=%s stderr=%s", code, out2.String(), err2.String())
	}
	if strings.Contains(err2.String(), "reused") {
		t.Fatalf("AC-002: editing the configured doc file must invalidate the cache:\n%s", err2.String())
	}

	var out3, err3 strings.Builder
	if code := runReview([]string{"--base", "HEAD~1"}, &out3, &err3, redaction.NewFilter()); code != 0 {
		t.Fatalf("round3 exit=%d, want 0; stdout=%s stderr=%s", code, out3.String(), err3.String())
	}
	if !strings.Contains(err3.String(), "reused") {
		t.Fatalf("AC-002: an unchanged doc file (round3 repeats round2) must still reuse the cache:\n%s", err3.String())
	}

	cacheDir := os.Getenv("AURUMCODE_CACHE_DIR")
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatalf("reading cache dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one cache entry on disk")
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), marker) {
			t.Fatalf("AC-002: the cache key (entry filename) must never carry the doc's own text in legible form: %s", e.Name())
		}
		data, rerr := os.ReadFile(filepath.Join(cacheDir, e.Name()))
		if rerr != nil {
			t.Fatal(rerr)
		}
		if strings.Contains(string(data), marker) {
			t.Fatalf("AC-002: a persisted cache entry must never carry the doc's own text: %s", e.Name())
		}
	}
}

// aur513CodebaseContextSymbols extracts the JSON codebasectx.Pack embedded
// in a captured prompt's own "## Codebase context" section (see
// internal/prompt/builder.go's fixedOverhead) and returns its Symbols. The
// JSON is a single json.Marshal line with no embedded newline, so the
// substring up to the next "\n" after the header is exactly that blob --
// never the diff section, which also happens to contain any symbol name
// the diff itself calls.
func aur513CodebaseContextSymbols(t *testing.T, prompt string) []string {
	t.Helper()
	const header = "## Codebase context (untrusted, bounded, heuristic)\n"
	idx := strings.Index(prompt, header)
	if idx == -1 {
		t.Fatalf("captured prompt carries no codebase-context section at all:\n%s", prompt)
	}
	rest := prompt[idx+len(header):]
	end := strings.IndexByte(rest, '\n')
	if end == -1 {
		end = len(rest)
	}
	blob := rest[:end]
	var pack codebasectx.Pack
	if err := json.Unmarshal([]byte(blob), &pack); err != nil {
		t.Fatalf("codebase-context section is not the expected JSON pack: %v\nblob=%s", err, blob)
	}
	return pack.Symbols
}

// TestAUR513AC003CrossFileEvidenceSurvivesPartialHit covers AC-003: lib.go
// defines HelperZZZ and is reviewed once (round 1, alongside app.go which
// calls it) so its own content becomes a cache hit on round 2, where only
// app.go changes again. The model must still receive evidence of lib.go's
// symbol in round 2's prompt -- specifically inside the codebase-context
// pack's own Symbols field, not merely somewhere in the prompt text, since
// app.go's OWN diff also happens to mention "HelperZZZ()" at its call site
// and would make a bare substring check pass vacuously even with an empty
// codebase-context pack. Isolating the pack's own Symbols is what actually
// proves lib.go's content, not just its name, survived the partial hit.
func TestAUR513AC003CrossFileEvidenceSurvivesPartialHit(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, data []byte) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	tree := func(appBlob, libBlob string) string {
		t.Helper()
		body := append(treeEntry(t, "100644", "app.go", appBlob), treeEntry(t, "100644", "lib.go", libBlob)...)
		return gitObject(t, dir, "tree", body)
	}
	commit := func(treeID, parent string) string {
		t.Helper()
		body := "tree " + treeID + "\n"
		if parent != "" {
			body += "parent " + parent + "\n"
		}
		body += "author Fixture <fixture@example.invalid> 1 +0000\ncommitter Fixture <fixture@example.invalid> 1 +0000\n\nfixture\n"
		return gitObject(t, dir, "commit", []byte(body))
	}

	appV0 := gitObject(t, dir, "blob", []byte("package demo\n"))
	libV0 := gitObject(t, dir, "blob", []byte("package demo\n"))
	root := commit(tree(appV0, libV0), "")

	libV1Src := "package demo\n\nfunc HelperZZZ() int {\n return 1\n}\n"
	libV1 := gitObject(t, dir, "blob", []byte(libV1Src))
	appV1Src := "package demo\n\nfunc Use() int {\n return HelperZZZ()\n}\n"
	appV1 := gitObject(t, dir, "blob", []byte(appV1Src))
	c1 := commit(tree(appV1, libV1), root)

	appV2Src := "package demo\n\nfunc Use() int {\n return HelperZZZ() + 1\n}\n"
	appV2 := gitObject(t, dir, "blob", []byte(appV2Src))
	c2 := commit(tree(appV2, libV1), c1) // lib.go unchanged since c1

	write(".git/HEAD", []byte("ref: refs/heads/main\n"))
	write(".git/config", []byte("[core]\nrepositoryformatversion = 0\nbare = false\n"))
	write(".git/refs/heads/main", []byte(c1+"\n"))
	write("app.go", []byte(appV1Src))
	write("lib.go", []byte(libV1Src))

	for _, key := range []string{"LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL", "AURUMCODE_LLM_FIXTURE", "AURUMCODE_LLM_INPUT_USD_PER_1K", "AURUMCODE_LLM_OUTPUT_USD_PER_1K"} {
		t.Setenv(key, "")
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("AURUMCODE_CACHE_DIR", t.TempDir())
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur513WriteFixture(t, aur513CleanResponse))
	restore := chdir(t, dir)
	t.Cleanup(restore)

	// Round 1: both app.go and lib.go are misses against root; both get
	// cached.
	var out1, err1 strings.Builder
	if code := runReview([]string{"--base", root}, &out1, &err1, redaction.NewFilter()); code != 0 {
		t.Fatalf("round1 exit=%d, want 0; stdout=%s stderr=%s", code, out1.String(), err1.String())
	}

	// Advance HEAD to c2: app.go changes again, lib.go's diff against root
	// is byte-identical to round 1's, so it must hit.
	write(".git/refs/heads/main", []byte(c2+"\n"))
	write("app.go", []byte(appV2Src))

	capture := filepath.Join(t.TempDir(), "prompt.txt")
	_ = os.Remove(capture)
	t.Setenv("AURUMCODE_PROMPT_CAPTURE", capture)

	var out2, err2 strings.Builder
	if code := runReview([]string{"--base", root}, &out2, &err2, redaction.NewFilter()); code != 0 {
		t.Fatalf("round2 exit=%d, want 0; stdout=%s stderr=%s", code, out2.String(), err2.String())
	}
	if !strings.Contains(err2.String(), "reused 1 file(s) from cache") {
		t.Fatalf("expected lib.go to be served from cache (unchanged since round1):\n%s", err2.String())
	}

	captured, err := os.ReadFile(capture)
	if err != nil {
		t.Fatalf("expected the model to be called again for app.go (a genuine miss), and its prompt captured: %v", err)
	}
	symbols := aur513CodebaseContextSymbols(t, string(captured))
	found := false
	for _, s := range symbols {
		if s == "HelperZZZ" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("AC-003: lib.go's own cache hit must not drop cross-file evidence -- round2's codebase-context Symbols must still carry HelperZZZ:\nsymbols=%v\nprompt=%s", symbols, string(captured))
	}
}

// TestAUR513AC004CorruptCacheEntryDegradesToFreshReview covers AC-004: a
// cache entry that cannot be parsed (truncated/garbled JSON on disk) must
// degrade to a fresh review for that file -- never a silent approval, and
// never omitted coverage. cache.Cache.Get already reports a parse error as
// (nil, false, err), which partitionByCache (review_cache.go) treats
// exactly like a genuine miss; this test fails if that degrade path is ever
// removed.
//
// The SAME fixture is used in both rounds on purpose: modelCacheKey folds
// in the fixture file's own content digest, so changing the fixture (an
// earlier version of this test did) would force a fresh key on its own,
// proving nothing about the corrupted-entry degrade path specifically. With
// an identical fixture, the ONLY thing that can explain round 2 not being
// served from cache is partitionByCache correctly treating the corrupted
// entry as a miss -- confirmed here two ways: no "reused" note, and the
// prompt capture file is actually (re)written, naming app.go.
func TestAUR513AC004CorruptCacheEntryDegradesToFreshReview(t *testing.T) {
	cleanFixture(t, "")
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur513WriteFixture(t, aur513CleanResponse))

	var out1, err1 strings.Builder
	if code := runReview([]string{"--base", "HEAD~1"}, &out1, &err1, redaction.NewFilter()); code != 0 {
		t.Fatalf("round1 exit=%d, want 0; stdout=%s stderr=%s", code, out1.String(), err1.String())
	}

	cacheDir := os.Getenv("AURUMCODE_CACHE_DIR")
	entries, err := os.ReadDir(cacheDir)
	if err != nil || len(entries) == 0 {
		t.Fatalf("expected a cache entry from round1: err=%v entries=%v", err, entries)
	}
	corrupted := filepath.Join(cacheDir, entries[0].Name())
	if err := os.WriteFile(corrupted, []byte("{not valid json, truncated"), 0600); err != nil {
		t.Fatal(err)
	}

	capture := filepath.Join(t.TempDir(), "prompt.txt")
	_ = os.Remove(capture)
	t.Setenv("AURUMCODE_PROMPT_CAPTURE", capture)

	var out2, err2 strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out2, &err2, redaction.NewFilter())
	if code != 0 {
		t.Fatalf("round2 exit=%d, want 0; stdout=%s stderr=%s", code, out2.String(), err2.String())
	}
	if strings.Contains(err2.String(), "reused") {
		t.Fatalf("AC-004: a corrupted cache entry must never be served as a hit:\n%s", err2.String())
	}
	captured, err := os.ReadFile(capture)
	if err != nil {
		t.Fatalf("AC-004: a corrupted entry must degrade to a fresh review -- expected the model to be called again and its prompt captured: %v", err)
	}
	if !strings.Contains(string(captured), "app.go") {
		t.Fatalf("AC-004: the fresh call must still be reviewing app.go, never an omitted file:\n%s", string(captured))
	}
}

// TestAUR513AC004UnreadableCacheDirDegradesToFreshReview covers AC-004's
// other half: an AURUMCODE_CACHE_DIR that cache.Open cannot use at all (here,
// a path that already exists as a plain FILE, so os.MkdirAll fails) must
// degrade the entire review to "no cache this run" -- every file sent to
// the model every time, never a crash and never a skipped file.
func TestAUR513AC004UnreadableCacheDirDegradesToFreshReview(t *testing.T) {
	cleanFixture(t, "")
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_CACHE_DIR", blocked)

	secretResp := `{"summary":"ok","verdict":"approve","issues":[{"file":"app.go","line":3,"severity":"error","rule_id":"security#no-hardcoded-secrets","message":"Hardcoded secret","evidence":"x","impact":"y","verification":"z"}]}`
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur513WriteFixture(t, secretResp))

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != 0 {
		t.Fatalf("exit=%d, want 0 (no --fail-on); stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	combined := out.String() + errOut.String()
	if !strings.Contains(combined, "security#no-hardcoded-secrets") {
		t.Fatalf("AC-004: an unusable cache directory must degrade to a full fresh review, never omitted coverage:\n%s", combined)
	}
	if strings.Contains(errOut.String(), "reused") {
		t.Fatalf("AC-004: an unusable cache directory must never report a reuse:\n%s", errOut.String())
	}
}

// TestAUR513ModelIdentitySurvivesContextWrapping covers point 3 of the
// AUR-513 review: config.contextInjectingProvider (internal/config/wrap.go)
// embeds llm.Provider as an INTERFACE field, so wrapping a provider that
// implements llm.ModelResolver (internal/llm/provider/litellm.Provider)
// with ANY configured context (a repo prompt, a skill, a doc) silently
// drops that extra method from promotion -- modelCacheKey(provider) called
// AFTER wrapping degrades to litellm.Provider.Name()'s fixed "litellm"
// literal, so two different models behind the same context configuration
// would otherwise collide on one cache entry.
//
// First half (unit): two litellm providers with different models AND
// different base URLs, each wrapped by the identical non-empty context,
// must still produce different review cache keys -- proven by capturing
// each one's own modelCacheKey BEFORE wrapping (baseModelIdentity, exactly
// as runReview now does) and feeding it into reviewContextCacheKey.
//
// Second half (end-to-end, the only trustworthy way to prove "the provider
// was actually called again"): a real httptest.Server standing in for the
// LiteLLM-compatible endpoint, with a request counter. A repository with a
// configured doc (so the provider WILL be context-wrapped) is reviewed
// twice against a persisted cache, switching only LLM_MODEL between
// rounds; round 2 must reach the server a second time.
func TestAUR513ModelIdentitySurvivesContextWrapping(t *testing.T) {
	ctxProviders := []config.ContextProvider{
		config.NewTextContextProvider(config.ContextFile{Kind: "prompt", Path: "x.md"}, "Repository-wide review instructions."),
	}

	p1 := litellm.NewProvider("key", "https://endpoint-a.example", "model-a")
	p2 := litellm.NewProvider("key", "https://endpoint-b.example", "model-b")
	wrapped1, err := config.WrapProvider(context.Background(), p1, ctxProviders, nil, redaction.NewFilter())
	if err != nil {
		t.Fatal(err)
	}
	wrapped2, err := config.WrapProvider(context.Background(), p2, ctxProviders, nil, redaction.NewFilter())
	if err != nil {
		t.Fatal(err)
	}

	if modelCacheKey(wrapped1) != modelCacheKey(wrapped2) {
		t.Fatalf("test assumption broken: expected the WRAPPED providers' own modelCacheKey to collide (demonstrating the promotion-loss bug this test exists to catch); got %q vs %q", modelCacheKey(wrapped1), modelCacheKey(wrapped2))
	}

	base1 := modelCacheKey(p1) // captured BEFORE wrapping, exactly as runReview does
	base2 := modelCacheKey(p2)
	if base1 == base2 {
		t.Fatal("test assumption broken: expected the two UNWRAPPED providers' own identity to differ")
	}
	key1 := reviewContextCacheKey(wrapped1, base1, "pt-BR", "ctx", "notes", "", "", "")
	key2 := reviewContextCacheKey(wrapped2, base2, "pt-BR", "ctx", "notes", "", "", "")
	if key1 == key2 {
		t.Fatal("AC: two different models/endpoints, wrapped by the identical context, must never share a review cache key")
	}

	// End-to-end: a persisted cache across two rounds, switching only
	// LLM_MODEL, against a real server that counts requests.
	dir := cleanFixture(t, "review:\n  context:\n    docs:\n      - docs/notes.md\n")
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "notes.md"), []byte("Follow the house style guide.\n"), 0600); err != nil {
		t.Fatal(err)
	}

	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"summary\":\"ok\",\"verdict\":\"approve\",\"issues\":[]}"}}],"usage":{}}`))
	}))
	defer server.Close()

	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_BASE_URL", server.URL)
	t.Setenv("LLM_MODEL", "model-a")

	var out1, err1 strings.Builder
	if code := runReview([]string{"--base", "HEAD~1"}, &out1, &err1, redaction.NewFilter()); code != 0 {
		t.Fatalf("round1 exit=%d; stdout=%s stderr=%s", code, out1.String(), err1.String())
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("expected exactly 1 request to the model after round1, got %d", got)
	}

	t.Setenv("LLM_MODEL", "model-b")
	var out2, err2 strings.Builder
	if code := runReview([]string{"--base", "HEAD~1"}, &out2, &err2, redaction.NewFilter()); code != 0 {
		t.Fatalf("round2 exit=%d; stdout=%s stderr=%s", code, out2.String(), err2.String())
	}
	if strings.Contains(err2.String(), "reused") {
		t.Fatalf("AC: switching LLM_MODEL must never reuse the previous model's cache entry:\n%s", err2.String())
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("expected a SECOND real request to the model after switching LLM_MODEL (never a cache hit across different models), total requests=%d", got)
	}
}
