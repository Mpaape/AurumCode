package main

// AUR-513 behavior proof: the per-file review cache (AUR-441,
// cmd/aurumcode/review_cache.go) must invalidate whenever ANYTHING that can
// change the model's answer changes -- the effective prompt (built-in +
// repo + central policy), every configured skill/doc's content, the
// selected profile(s), the review language, the model, the dynamic
// rule/skill-section catalog and the relevant codebase context (AC-001,
// AC-002) -- while a per-file hit must never drop the cross-file evidence a
// changed sibling file still needs (AC-003), and a corrupted or unreadable
// cache must degrade to a fresh review, never to a silent approval or to
// omitted coverage (AC-004). Each test here fails if the behavior it names
// is removed. Mirrors aur519_e2e_test.go's / aur476_test.go's own
// httptest-free, AURUMCODE_LLM_FIXTURE + AURUMCODE_CACHE_DIR pattern.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

// TestAUR513AC002SkillContentChangeForcesFreshReview covers AC-002: editing
// the content of a configured repository skill file, with everything else
// held constant, must invalidate the cache for a byte-identical diff -- and
// the cache's own key (the on-disk entry filenames under AURUMCODE_CACHE_DIR)
// never carries the skill's text in legible form.
func TestAUR513AC002SkillContentChangeForcesFreshReview(t *testing.T) {
	dir := cleanFixture(t, "review:\n  context:\n    docs:\n      - docs/notes.md\n")
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0700); err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(dir, "docs", "notes.md")
	const marker = "zz-secret-shaped-marker-9f3c"
	if err := os.WriteFile(skillPath, []byte("## Style\n\nPrefer small functions. token:"+marker+"-v1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur513WriteFixture(t, aur513CleanResponse))

	var out1, err1 strings.Builder
	if code := runReview([]string{"--base", "HEAD~1"}, &out1, &err1, redaction.NewFilter()); code != 0 {
		t.Fatalf("round1 exit=%d, want 0; stdout=%s stderr=%s", code, out1.String(), err1.String())
	}

	if err := os.WriteFile(skillPath, []byte("## Style\n\nPrefer small functions. token:"+marker+"-v2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out2, err2 strings.Builder
	if code := runReview([]string{"--base", "HEAD~1"}, &out2, &err2, redaction.NewFilter()); code != 0 {
		t.Fatalf("round2 exit=%d, want 0; stdout=%s stderr=%s", code, out2.String(), err2.String())
	}
	if strings.Contains(err2.String(), "reused") {
		t.Fatalf("AC-002: editing the configured skill file must invalidate the cache:\n%s", err2.String())
	}

	var out3, err3 strings.Builder
	if code := runReview([]string{"--base", "HEAD~1"}, &out3, &err3, redaction.NewFilter()); code != 0 {
		t.Fatalf("round3 exit=%d, want 0; stdout=%s stderr=%s", code, out3.String(), err3.String())
	}
	if !strings.Contains(err3.String(), "reused") {
		t.Fatalf("AC-002: an unchanged skill file (round3 repeats round2) must still reuse the cache:\n%s", err3.String())
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
			t.Fatalf("AC-002: the cache key (entry filename) must never carry the skill's own text in legible form: %s", e.Name())
		}
		data, rerr := os.ReadFile(filepath.Join(cacheDir, e.Name()))
		if rerr != nil {
			t.Fatal(rerr)
		}
		if strings.Contains(string(data), marker) {
			t.Fatalf("AC-002: a persisted cache entry must never carry the skill's own text: %s", e.Name())
		}
	}
}

// TestAUR513AC003CrossFileEvidenceSurvivesPartialHit covers AC-003: lib.go
// defines HelperZZZ and is reviewed once (round 1, alongside app.go which
// calls it) so its own content becomes a cache hit on round 2, where only
// app.go changes again. The model must still receive evidence of lib.go's
// symbol in round 2's prompt (via the codebase-context pack, computed from
// the FULL diff before per-file partitioning -- see partitionByCache's own
// doc) even though lib.go itself is never resent as a diff. Without that,
// a model asked to judge app.go's new call site would have no idea
// HelperZZZ exists at all.
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
	if !strings.Contains(string(captured), "HelperZZZ") {
		t.Fatalf("AC-003: lib.go's own cache hit must not drop cross-file evidence -- round2's prompt must still carry HelperZZZ's symbol via the codebase-context pack:\n%s", string(captured))
	}
}

// TestAUR513AC004CorruptCacheEntryDegradesToFreshReview covers AC-004: a
// cache entry that cannot be parsed (truncated/garbled JSON on disk) must
// degrade to a fresh review for that file -- never a silent approval, and
// never omitted coverage. cache.Cache.Get already reports a parse error as
// (nil, false, err), which partitionByCache (review_cache.go) treats
// exactly like a genuine miss; this test fails if that degrade path is ever
// removed.
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

	secretResp := `{"summary":"ok","verdict":"approve","issues":[{"file":"app.go","line":3,"severity":"error","rule_id":"security#no-hardcoded-secrets","message":"Hardcoded secret","evidence":"x","impact":"y","verification":"z"}]}`
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur513WriteFixture(t, secretResp))

	var out2, err2 strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out2, &err2, redaction.NewFilter())
	if code != 0 {
		t.Fatalf("round2 exit=%d, want 0 (no --fail-on); stdout=%s stderr=%s", code, out2.String(), err2.String())
	}
	if strings.Contains(err2.String(), "reused") {
		t.Fatalf("AC-004: a corrupted cache entry must never be served as a hit:\n%s", err2.String())
	}
	combined := out2.String() + err2.String()
	if !strings.Contains(combined, "security#no-hardcoded-secrets") {
		t.Fatalf("AC-004: a corrupted entry must degrade to a fresh review, surfacing the real finding, never a silent approval or omitted coverage:\n%s", combined)
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
