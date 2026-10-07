package tools

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/Mpaape/AurumCode/internal/analyzer"
)

// gitSymlinkMode is the git tree mode of a symbolic link.
const gitSymlinkMode = "120000"

// RevisionOptions describe the reviewed revision the repository tools read.
type RevisionOptions struct {
	// Root is the checkout whose HEAD is the reviewed revision, already
	// proven by the caller (--pr verifies it is the pull request's head).
	Root string
	// Tracked is the reviewed commit's tree (analyzer.Repo.TrackedFiles):
	// the only paths a tool may name, each with the blob id the bytes read
	// from Root must hash to.
	Tracked map[string]analyzer.TrackedEntry
	// Ignored is the policy's ignore globs; nil ignores nothing.
	Ignored func(string) bool
	// Secret is the secret-file catalog; nil refuses every path (fail closed).
	Secret func(string) bool
	// MaxFileBytes bounds one file read; 0 means grammarMaxFileBytes.
	MaxFileBytes int64
	// Budget bounds the bytes returned to the model over the review.
	Budget *ByteBudget
	// MaxCacheBytes bounds the file bytes kept in memory over the review;
	// 0 means defaultMaxCacheBytes. Crossing it exhausts Budget with
	// max_cache_bytes: the review is partial.
	MaxCacheBytes int
}

// defaultMaxCacheBytes is the default in-memory ceiling of read files.
const defaultMaxCacheBytes = 64 << 20

// grammarMaxFileBytes is the default per-file read bound.
const grammarMaxFileBytes = 1 << 20

// Revision reads files of the reviewed revision and nothing else: a path
// outside its tree, a symbolic link (in the tree or on disk), a path whose
// directory escapes the root, an ignored or secret file, and bytes that do
// not hash to the revision's own blob (a working tree that differs from
// the reviewed commit) are all refused.
type Revision struct {
	opts     RevisionOptions
	realRoot string
	mu       sync.Mutex
	cache    map[string][]byte
	cached   int
}

// NewRevision opens the reviewed revision under opts.Root.
func NewRevision(opts RevisionOptions) (*Revision, error) {
	if opts.Root == "" || opts.Tracked == nil {
		return nil, fmt.Errorf("revisão revisada indisponível")
	}
	real, err := filepath.EvalSymlinks(opts.Root)
	if err != nil {
		return nil, fmt.Errorf("raiz da revisão ilegível: %w", err)
	}
	if opts.MaxFileBytes <= 0 {
		opts.MaxFileBytes = grammarMaxFileBytes
	}
	if opts.Budget == nil {
		opts.Budget = NewByteBudget(0)
	}
	if opts.MaxCacheBytes <= 0 {
		opts.MaxCacheBytes = defaultMaxCacheBytes
	}
	return &Revision{opts: opts, realRoot: real, cache: map[string][]byte{}}, nil
}

// Budget is the byte budget shared by the tools reading this revision.
func (r *Revision) Budget() *ByteBudget { return r.opts.Budget }

// Paths is every path of the revision the tools may read, sorted.
func (r *Revision) Paths() []string {
	out := make([]string, 0, len(r.opts.Tracked))
	for p := range r.opts.Tracked {
		if r.admit(p) == nil {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// admit refuses a path by name alone: not a clean relative path of the
// revision's tree, a symbolic link of the tree, ignored or secret.
func (r *Revision) admit(rel string) error {
	if rel == "" || strings.HasPrefix(rel, "/") || strings.Contains(rel, "\\") || path.Clean(rel) != rel || rel == ".." || strings.HasPrefix(rel, "../") {
		return fmt.Errorf("%q não é um caminho relativo dentro do repositório", rel)
	}
	entry, ok := r.opts.Tracked[rel]
	if !ok {
		return fmt.Errorf("%q não existe na revisão revisada", rel)
	}
	if entry.Mode == gitSymlinkMode {
		return fmt.Errorf("%q é um link simbólico na revisão revisada; não é seguido", rel)
	}
	if r.opts.Ignored != nil && r.opts.Ignored(rel) {
		return fmt.Errorf("%q é ignorado pela política", rel)
	}
	if r.opts.Secret == nil || r.opts.Secret(rel) {
		return fmt.Errorf("%q é um arquivo de segredo; não é lido", rel)
	}
	return nil
}

// Read returns the bytes of rel at the reviewed revision.
func (r *Revision) Read(rel string) ([]byte, error) {
	if err := r.admit(rel); err != nil {
		return nil, err
	}
	r.mu.Lock()
	cached, ok := r.cache[rel]
	r.mu.Unlock()
	if ok {
		return cached, nil
	}
	full := filepath.Join(r.opts.Root, filepath.FromSlash(rel))
	info, err := os.Lstat(full)
	if err != nil {
		return nil, fmt.Errorf("%q não pôde ser lido da revisão revisada", rel)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%q é um link simbólico no checkout; não é seguido", rel)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%q não é um arquivo regular", rel)
	}
	if err := r.contained(full); err != nil {
		return nil, err
	}
	if info.Size() > r.opts.MaxFileBytes {
		return nil, fmt.Errorf("%q excede %d bytes", rel, r.opts.MaxFileBytes)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, fmt.Errorf("%q não pôde ser lido da revisão revisada", rel)
	}
	if blobID(data) != r.opts.Tracked[rel].SHA {
		return nil, fmt.Errorf("%q no checkout difere da revisão revisada; não é lido", rel)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cached+len(data) > r.opts.MaxCacheBytes {
		r.opts.Budget.Exhaust(LimitMaxCacheBytes, fmt.Sprintf("%d de %d bytes de arquivos já em memória", r.cached, r.opts.MaxCacheBytes))
		return nil, fmt.Errorf("teto de %s atingido; a revisão fica parcial", LimitMaxCacheBytes)
	}
	r.cache[rel] = data
	r.cached += len(data)
	return data, nil
}

// contained refuses a file whose directory resolves outside the root (a
// directory symlink that escapes).
func (r *Revision) contained(full string) error {
	dir, err := filepath.EvalSymlinks(filepath.Dir(full))
	if err != nil {
		return fmt.Errorf("diretório ilegível")
	}
	rel, err := filepath.Rel(r.realRoot, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("o caminho escapa do repositório")
	}
	return nil
}

// blobID is git's object id of a blob with content data.
func blobID(data []byte) string {
	h := sha1.New()
	h.Write([]byte("blob " + strconv.Itoa(len(data)) + "\x00"))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}
