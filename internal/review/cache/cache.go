// Package cache implements AUR-441: do not pay twice for the same file.
//
// The review engine sends the entire reviewed diff to the model in one
// prompt, one Complete call per invocation (internal/review.Reviewer.
// GenerateReview, internal/prompt.PromptBuilder.BuildPrompt) -- there is no
// per-file call for a cache to intercept. So this package does not wrap the
// model call itself; it gives the caller (cmd/aurumcode) what it needs to
// decide, PER FILE and BEFORE the call, whether that file's content was
// already reviewed under the same model and the same prompt contract:
//
//   - Key computes a content-addressed key for one diff file.
//   - Cache.Get / Cache.Put persist and retrieve that file's findings from a
//     previous run.
//
// cmd/aurumcode filters the diff it sends to GenerateReview down to the
// cache misses, merges the cache hits' findings back into the printed
// result, and skips the model call entirely when nothing missed. See
// cmd/aurumcode/review_cache.go.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// PromptVersion pins the cache key to the shape of the prompt and rule
// catalog this engine currently sends to a model. internal/prompt and
// internal/review/rules.go (both read_paths for this card, owned and
// versioned by other cards) carry no version of their own to key off, so
// this constant is the deliberate, manually-bumped invalidation lever: the
// day either changes in a way that could change what a file's findings look
// like, bumping PromptVersion makes every entry cached under the old value
// simply miss -- exactly like a changed file would.
const PromptVersion = "v1"

// EnvDir is the environment variable that pins the cache directory
// (AURUMCODE_CACHE_DIR). See ResolveDir for the default when it is unset.
const EnvDir = "AURUMCODE_CACHE_DIR"

// defaultSubdirPrefix names the directory ResolveDir creates under the OS
// temp directory when AURUMCODE_CACHE_DIR is not set; os.Getpid() makes it
// unique per process.
const defaultSubdirPrefix = "aurumcode-review-cache-"

// ResolveDir returns the effective cache directory: AURUMCODE_CACHE_DIR
// when set, taken as-is (even a relative path), so a caller can pin an
// exact, SHARED location deliberately.
//
// Without it, the default is unique to THIS process invocation --
// os.TempDir()/aurumcode-review-cache-<pid> -- never a location two
// separate `aurumcode` invocations could land on by coincidence. This is a
// deliberate reversal from a first design that scoped the default to the
// reviewed repository's own directory (so that a bare, unconfigured second
// run of the same command would reuse it): that design collided with
// AUR-433's already-published, already-`done` --limite contract, whose own
// acceptance program (tests/acceptance/AUR-433.sh) runs the built binary
// several times in a row against the SAME repository -- once unmetered,
// twice at an identical --limite ceiling to prove the two invocations are
// byte-for-byte deterministic, once refused -- and asserts every one of
// those calls independently reaches the model and reports its own real,
// freshly-computed cost. A repo-scoped default cache would have served the
// second --limite invocation from the first's cache entry: no call, no
// "actual cost" line matching what the first invocation printed, a
// determinism assertion that can never pass again by construction, because
// the two invocations would no longer be doing the same amount of work.
// That is not a scoping bug to patch around; a shared default and
// AUR-433's per-invocation cost accounting cannot both be satisfied for
// the exact case of "the same command run twice against the same
// repository" -- which is also this card's own Outcome. Being unique per
// process resolves the conflict rather than papering over it: reuse across
// separate invocations becomes something the caller always asks for
// explicitly via AURUMCODE_CACHE_DIR (as this card's own acceptance,
// integration and e2e programs do for every case that proves the Outcome),
// never something that happens silently underneath an unrelated card's
// published contract.
func ResolveDir() string {
	if dir := os.Getenv(EnvDir); dir != "" {
		return dir
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("%s%d", defaultSubdirPrefix, os.Getpid()))
}

// Cache is a directory holding one file per cache key.
type Cache struct {
	dir string
}

// Open prepares dir to hold cache entries, creating it (mode 0700) if
// necessary. A directory that cannot be created or used is reported once,
// here, so the caller can degrade to "no cache this run" up front instead
// of discovering it entry by entry.
func Open(dir string) (*Cache, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("review cache: %s: %w", dir, err)
	}
	return &Cache{dir: dir}, nil
}

// Entry is what Put persists and Get returns for one file.
type Entry struct {
	// Path is the diff-relative file path the entry was computed for,
	// carried for the on-disk cache's own readability only; lookups are
	// always by Key, never by Path.
	Path string `json:"path"`
	// Issues are exactly the review findings GenerateReview returned for
	// this file on the run that wrote this entry: already redacted
	// (internal/review.redactReviewResult runs, inside GenerateReview,
	// before this package's caller ever sees a result) and already
	// rule-cited (internal/review.enforceRuleCitations). Put neither
	// receives nor persists anything upstream of that boundary -- there is
	// nothing left for this package to redact, and re-redacting here would
	// risk rewriting the trusted rule-citation suffix the same way
	// cmd/aurumcode's own printFindings/printSecurityFindings already
	// document for the same reason.
	Issues []types.ReviewIssue `json:"issues"`
}

// Key computes the cache key for one diff file, reviewed by model under
// promptVersion: a content digest of the file's own diff -- path, language
// tag and every hunk, the only bytes this engine bases a finding on -- so
// that any change to what the model would see invalidates the entry, plus
// the model identity and the prompt version, so a cache built under one
// model or one prompt shape is never served to another.
//
// The digest is computed over the RAW, unredacted diff content, not the
// redacted form the model actually receives (internal/review.redactDiff).
// That is deliberate: redacting first would replace a secret value with the
// same fixed marker regardless of what the secret actually was, so two
// genuinely different file versions that differ only inside a redacted span
// -- a rotated API key, for instance -- would collapse to the SAME key and
// this cache would wrongly serve a stale finding for changed content. A
// digest of raw content has the opposite property that matters here: it is
// one-way, so it never persists anything recoverable, while remaining
// accurate about whether the file actually changed. What DOES get persisted
// (Entry.Issues) is already the redacted, AUR-432-safe output; see Entry's
// doc.
func Key(file types.DiffFile, model, promptVersion string) string {
	// json.Marshal on this exported, fixed-shape struct is deterministic:
	// field order follows the struct declaration in pkg/types, not map
	// iteration, so the same file always marshals to the same bytes.
	body, err := json.Marshal(file)
	if err != nil {
		// types.DiffFile has no unmarshalable field (no chan, no func); this
		// is unreachable in practice. Fail toward "this file alone never
		// matches a cache entry" rather than panicking the whole review: an
		// error-shaped body still combines deterministically with the
		// model/version below, it just cannot coincide with any real
		// digest, so this one file degrades to no caching instead of
		// crashing it.
		body = []byte("marshal-error:" + file.Path)
	}
	h := sha256.New()
	h.Write([]byte(model))
	h.Write([]byte{0})
	h.Write([]byte(promptVersion))
	h.Write([]byte{0})
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

func (c *Cache) entryPath(key string) string {
	return filepath.Join(c.dir, key+".json")
}

// Get returns the cached entry for key and whether one existed. A read or
// parse error is reported as (nil, false, err); callers of this package
// treat that exactly like a genuine miss for the review itself -- a
// corrupt or racing cache file never fails the command -- but can still
// tell the two apart if useful.
func (c *Cache) Get(key string) (*Entry, bool, error) {
	data, err := os.ReadFile(c.entryPath(key))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, false, err
	}
	return &e, true, nil
}

// Put persists entry under key (mode 0600), replacing any previous entry
// for that key outright.
func (c *Cache) Put(key string, entry Entry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return os.WriteFile(c.entryPath(key), data, 0o600)
}
