package context

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/grammar"
)

// Limits bounds every phase of resolution so a large or hostile repo can
// never exhaust memory or time. The zero value is not valid; use NewResolver
// for sensible defaults or NewResolverWithLimits to customize.
type Limits struct {
	MaxFiles      int // max repo files enumerated for the reference scan
	MaxBytes      int // max bytes read from any single file
	MaxDependents int // max dependent files / reference edges recorded
}

const (
	// DefaultMaxFiles bounds how many files are enumerated for the reference
	// scan. Changed files are always read, regardless of this limit.
	DefaultMaxFiles = 2000
	// DefaultMaxBytes bounds how many bytes are read from any single file.
	// Files larger than this are scanned as a truncated prefix.
	DefaultMaxBytes = 256 * 1024
	// DefaultMaxDependents bounds how many dependent references are recorded.
	DefaultMaxDependents = 500

	// maxDroppedNotes bounds the length of Pack.Dropped so a pathological repo
	// cannot make the report itself unbounded.
	maxDroppedNotes = 200
)

// Reference is a single cross-file dependency edge: File references Symbol at
// 1-based Line. Symbol is either an import path that resolves to a changed
// file or a symbol name defined in a changed file.
type Reference struct {
	File   string `json:"file"`
	Symbol string `json:"symbol"`
	Line   int    `json:"line"`
}

// Pack is the bounded, deterministic context summary of how a change set can
// affect the rest of the repository. All slices are sorted and de-duplicated.
type Pack struct {
	// Symbols lists the symbol names defined in the changed files.
	Symbols []string `json:"symbols"`
	// References lists the import and symbol-reference edges discovered.
	References []Reference `json:"references"`
	// Dependents lists the files (outside the change set) that import or
	// reference the changed files.
	Dependents []string `json:"dependents"`
	// Unstructured lists the changed files whose structure the grammar runtime
	// could not produce, each with its reason ("path: reason"). The review
	// continues on the text of those files; this field is the declaration that
	// symbol and import context for them is absent.
	Unstructured []string `json:"unstructured,omitempty"`
	// Snippets are excerpts around the references, with file and line
	// (AUR-470): the impact of the change on other files, as context only.
	Snippets []Snippet `json:"snippets,omitempty"`
	// Dropped records bounded omissions: missing files, skipped symlinks,
	// truncated files, and limit overflows. Omissions are never fatal.
	Dropped []string `json:"dropped"`
}

// Resolver resolves codebase context for a change set.
type Resolver struct {
	limits  Limits
	grammar grammar.Provider
	exclude func(rel string) bool
}

// NewResolver returns a Resolver with the default limits.
func NewResolver() *Resolver {
	return NewResolverWithLimits(Limits{
		MaxFiles:      DefaultMaxFiles,
		MaxBytes:      DefaultMaxBytes,
		MaxDependents: DefaultMaxDependents,
	})
}

// NewResolverWithLimits returns a Resolver with explicit limits. Any
// non-positive field falls back to its default.
func NewResolverWithLimits(limits Limits) *Resolver {
	if limits.MaxFiles <= 0 {
		limits.MaxFiles = DefaultMaxFiles
	}
	if limits.MaxBytes <= 0 {
		limits.MaxBytes = DefaultMaxBytes
	}
	if limits.MaxDependents <= 0 {
		limits.MaxDependents = DefaultMaxDependents
	}
	return &Resolver{limits: limits, grammar: grammar.Default()}
}

// WithGrammar returns the resolver using an injected grammar provider.
func (r *Resolver) WithGrammar(p grammar.Provider) *Resolver {
	r.grammar = p
	return r
}

// Resolve walks root and reports the bounded dependency context for the given
// changed file paths. Changed paths may be absolute or relative to root and
// use either slash or backslash separators. A missing root returns an error;
// every per-file condition is instead recorded in Pack.Dropped. Output is
// deterministic for a fixed input and tree.
func (r *Resolver) Resolve(root string, changed []string) (*Pack, error) {
	return r.resolve(root, changed, false, nil)
}

// ResolveWithFiles behaves exactly like Resolve, except the repo-wide
// reference scan never discovers its own candidate set by walking root: it
// scans exactly files, nothing else -- not even anything under ".git",
// regardless of what enumerate's own walk would otherwise do with it. A
// caller that has already verified, by its own means (AUR-536's
// clean-checkout proof), exactly which files are safe to read can pass
// that set here and make it the ONLY one this resolver ever opens for the
// scan, by construction -- not merely a set its own, independent walk
// happens to agree with today. changed is handled exactly as in Resolve
// (a changed path need not also appear in files to have its own content
// read for symbol extraction); files governs only the repo-wide scan's
// candidate set, matching repoFiles' own role in Resolve.
//
// A nil or empty files restricts the scan to NOTHING -- it is never
// treated as "no restriction given" and never falls back to Resolve's own
// walk. The two calls are distinguished by a separate flag internally,
// precisely so an empty allow-list can never be silently reinterpreted as
// "walk everything."
func (r *Resolver) ResolveWithFiles(root string, changed, files []string) (*Pack, error) {
	return r.resolve(root, changed, true, files)
}

func (r *Resolver) resolve(root string, changed []string, restrict bool, allowedFiles []string) (*Pack, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("context: stat root %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("context: root %q is not a directory", root)
	}

	pack := &Pack{}
	changedPaths := r.keepIncluded(r.normalizeChanged(root, changed, pack), pack)
	changedSet := make(map[string]bool, len(changedPaths))
	for _, p := range changedPaths {
		changedSet[p] = true
	}

	var repoFiles []string
	if restrict {
		repoFiles = r.normalizeAllowed(root, allowedFiles, pack)
	} else {
		repoFiles = r.enumerate(root, pack)
	}
	repoFiles = r.keepIncluded(repoFiles, pack)

	symbolFiles := make(map[string][]string) // symbol name -> defining changed files
	dirKeys := make(map[string]struct{})     // directory path of changed files
	stemKeys := make(map[string]struct{})    // base name (no ext) of changed files
	dirBaseKeys := make(map[string]struct{}) // base directory name of changed files

	for _, rel := range changedPaths {
		data, truncated, err := r.readFile(root, rel)
		if err != nil {
			pack.Dropped = appendDropped(pack.Dropped, "unreadable "+rel+": "+err.Error())
			continue
		}
		if truncated {
			pack.Dropped = appendDropped(pack.Dropped, "truncated "+rel+" at "+itoa(r.limits.MaxBytes)+" bytes")
		}
		st := r.structureOf(rel, data)
		if !st.HasStructure {
			// No grammar (or binary content): the model reads the text and the
			// pack says the structural context was not produced.
			pack.Unstructured = append(pack.Unstructured, rel+": "+st.Reason)
		}
		for _, sym := range st.Symbols {
			symbolFiles[sym] = append(symbolFiles[sym], rel)
		}
		dir, stem, dirBase := importKeys(rel)
		if dir != "" {
			dirKeys[dir] = struct{}{}
			dirBaseKeys[dirBase] = struct{}{}
		}
		stemKeys[stem] = struct{}{}
	}

	r.scan(root, repoFiles, changedSet, symbolFiles, dirKeys, stemKeys, dirBaseKeys, pack)

	for sym := range symbolFiles {
		pack.Symbols = append(pack.Symbols, sym)
	}

	r.finalize(pack)
	r.attachSnippets(root, pack)
	sort.Strings(pack.Dropped)
	pack.Dropped = dedupeStrings(pack.Dropped)
	return pack, nil
}

// normalizeChanged converts changed paths into sorted, de-duplicated relative
// slash paths that are known regular files. Anything else is recorded in
// Dropped and omitted from the returned set.
func (r *Resolver) normalizeChanged(root string, changed []string, pack *Pack) []string {
	set := make(map[string]struct{})
	for _, raw := range changed {
		p := filepath.ToSlash(strings.TrimSpace(raw))
		if p == "" {
			continue
		}
		if filepath.IsAbs(raw) {
			rel, err := filepath.Rel(root, filepath.FromSlash(p))
			if err != nil {
				pack.Dropped = appendDropped(pack.Dropped, "unresolvable path "+p)
				continue
			}
			p = filepath.ToSlash(rel)
		}
		p = strings.TrimPrefix(p, "./")
		if p == "" || p == "." || strings.HasPrefix(p, "../") {
			pack.Dropped = appendDropped(pack.Dropped, "path outside root "+p)
			continue
		}
		if _, ok := set[p]; ok {
			continue
		}
		if !regularFile(filepath.Join(root, filepath.FromSlash(p))) {
			pack.Dropped = appendDropped(pack.Dropped, "missing or non-regular changed file "+p)
			continue
		}
		set[p] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// normalizeAllowed converts an explicit, already-verified file set
// (ResolveWithFiles) into sorted, de-duplicated relative slash paths that
// are known regular files -- the same shape enumerate's own walk returns,
// so everything downstream (scan, Dependents) treats the two identically.
// Anything that doesn't check out -- an absolute or ".."-escaping path, or
// one no longer a regular file -- is recorded in Dropped and omitted,
// exactly like normalizeChanged; it is never fatal. Bounded by MaxFiles,
// same as enumerate.
func (r *Resolver) normalizeAllowed(root string, allowed []string, pack *Pack) []string {
	set := make(map[string]struct{})
	for _, raw := range allowed {
		p := filepath.ToSlash(strings.TrimSpace(raw))
		if p == "" {
			continue
		}
		p = strings.TrimPrefix(p, "./")
		if p == "" || p == "." || strings.HasPrefix(p, "../") || filepath.IsAbs(p) {
			pack.Dropped = appendDropped(pack.Dropped, "path outside root "+p)
			continue
		}
		if _, ok := set[p]; ok {
			continue
		}
		if !regularFile(filepath.Join(root, filepath.FromSlash(p))) {
			pack.Dropped = appendDropped(pack.Dropped, "missing or non-regular allowed file "+p)
			continue
		}
		set[p] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	if len(out) > r.limits.MaxFiles {
		pack.Dropped = appendDropped(pack.Dropped, "file enumeration truncated at "+itoa(r.limits.MaxFiles)+" files")
		out = out[:r.limits.MaxFiles]
	}
	return out
}

// enumerate returns the sorted relative slash paths of every regular file
// under root, bounded by MaxFiles. ".git" (this checkout's own git
// directory, whatever shape it takes) is never descended into at any
// depth, so nothing stored there -- loose objects, config, or anything
// else sitting in it -- is ever read as if it were repository content.
// Symlinks are skipped and any truncation is recorded in Dropped. Every
// Dropped note here names a root-relative path, never the absolute
// filesystem path WalkDir itself reports, so a published review can never
// carry this host's own directory layout.
func (r *Resolver) enumerate(root string, pack *Pack) []string {
	var files []string
	truncated := false
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = filepath.Base(path)
		}
		rel = filepath.ToSlash(rel)
		if err != nil {
			pack.Dropped = appendDropped(pack.Dropped, "walk error "+rel+": "+err.Error())
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() == ".git" {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil // a ".git" FILE: worktree/submodule indirection, never repository content.
		}
		if d.Type()&fs.ModeSymlink != 0 {
			pack.Dropped = appendDropped(pack.Dropped, "skipped symlink "+rel)
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if len(files) >= r.limits.MaxFiles {
			truncated = true
			return nil
		}
		files = append(files, rel)
		return nil
	})
	sort.Strings(files)
	if truncated {
		pack.Dropped = appendDropped(pack.Dropped, "file enumeration truncated at "+itoa(r.limits.MaxFiles)+" files")
	}
	return files
}

// readFile reads up to MaxBytes of root/rel, skipping symlinks. It reports
// whether the read was truncated.
func (r *Resolver) readFile(root, rel string) ([]byte, bool, error) {
	full := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Lstat(full)
	if err != nil {
		return nil, false, err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return nil, false, fmt.Errorf("symlink")
	}
	if !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("not a regular file")
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, int64(r.limits.MaxBytes)))
	if err != nil {
		return nil, false, err
	}
	truncated := len(data) == r.limits.MaxBytes && info.Size() > int64(r.limits.MaxBytes)
	return data, truncated, nil
}

// scan walks every non-changed file and records import and symbol-reference
// edges that point at the changed set.
func (r *Resolver) scan(root string, repoFiles []string, changedSet map[string]bool, symbolFiles map[string][]string, dirKeys, stemKeys, dirBaseKeys map[string]struct{}, pack *Pack) {
	symRegex, skipped := symbolAlternation(symbolFiles)
	if skipped != "" {
		pack.Dropped = appendDropped(pack.Dropped, skipped)
	}

	refs := make([]Reference, 0)
	truncated := false
	for _, rel := range repoFiles {
		if changedSet[rel] {
			continue
		}
		if len(refs) >= r.limits.MaxDependents {
			truncated = true
			break
		}
		data, wasTruncated, err := r.readFile(root, rel)
		if err != nil {
			pack.Dropped = appendDropped(pack.Dropped, "unreadable "+rel+": "+err.Error())
			continue
		}
		if wasTruncated {
			pack.Dropped = appendDropped(pack.Dropped, "truncated "+rel+" at "+itoa(r.limits.MaxBytes)+" bytes")
		}
		for _, imp := range r.structureOf(rel, data).Imports {
			if matchesImport(imp, dirKeys, stemKeys, dirBaseKeys) {
				offset := indexOf(data, []byte(imp))
				refs = append(refs, Reference{File: rel, Symbol: imp, Line: lineOf(data, offset)})
			}
		}
		if symRegex != nil {
			for _, m := range symRegex.FindAllIndex(data, -1) {
				if len(refs) >= r.limits.MaxDependents {
					truncated = true
					break
				}
				refs = append(refs, Reference{File: rel, Symbol: string(data[m[0]:m[1]]), Line: lineOf(data, m[0])})
			}
		}
	}
	if truncated {
		pack.Dropped = appendDropped(pack.Dropped, "dependents truncated at "+itoa(r.limits.MaxDependents))
	}
	pack.References = refs
}

// finalize sorts and de-duplicates the pack and derives Dependents from the
// reference edges.
func (r *Resolver) finalize(pack *Pack) {
	sort.Strings(pack.Symbols)
	pack.Symbols = dedupeStrings(pack.Symbols)
	sort.Strings(pack.Unstructured)

	sort.Slice(pack.References, func(i, j int) bool {
		a, b := pack.References[i], pack.References[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Symbol != b.Symbol {
			return a.Symbol < b.Symbol
		}
		return a.Line < b.Line
	})
	pack.References = dedupeReferences(pack.References)

	seen := make(map[string]struct{})
	dependents := make([]string, 0, len(pack.References))
	for _, ref := range pack.References {
		if _, ok := seen[ref.File]; ok {
			continue
		}
		seen[ref.File] = struct{}{}
		dependents = append(dependents, ref.File)
	}
	pack.Dependents = dependents

	sort.Strings(pack.Dropped)
	pack.Dropped = dedupeStrings(pack.Dropped)
}

func regularFile(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

func appendDropped(dropped []string, note string) []string {
	if len(dropped) >= maxDroppedNotes {
		return dropped
	}
	return append(dropped, note)
}

func dedupeStrings(in []string) []string {
	if len(in) < 2 {
		return in
	}
	out := in[:1]
	for _, s := range in[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}

func dedupeReferences(in []Reference) []Reference {
	if len(in) < 2 {
		return in
	}
	out := in[:1]
	for _, r := range in[1:] {
		last := out[len(out)-1]
		if last.File == r.File && last.Symbol == r.Symbol && last.Line == r.Line {
			continue
		}
		out = append(out, r)
	}
	return out
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
