package context

import (
	"path/filepath"
	"sync"

	"github.com/Mpaape/AurumCode/internal/grammar"
)

// One reading of the repository per review (#198). A review split into
// batches resolves the context of each batch's files; every resolution
// scans the same repository files. A resolver made with Reading reads and
// parses each file once, whatever the number of resolutions, and walks a
// root once: each batch's resolution matches its own changed files against
// what was read. The answers are the same as a fresh resolver's; only the
// parsing is shared. The reading keeps what resolution needs between
// batches -- each file's structure, imports and the line of each import --
// never its text: a monorepo's every file held for the whole run would be
// the runner's memory. The text is read again, without parsing, when a
// resolution scans a file for the changed symbols or a snippet is cut from
// it. A reading belongs to one review: a file changed on disk after its
// first reading keeps the structure of that first reading.

// loaded is one file as read and parsed. data is the text of this reading
// only: a Reading resolver keeps the rest and drops it.
type loaded struct {
	data        []byte
	truncated   bool
	structure   grammar.Structure
	importLines []int // the line of each structure.Imports entry, 0 when not found
}

// reading is what a Reading resolver has read so far, by absolute path.
type reading struct {
	mu    sync.Mutex
	files map[string]*readOnce
	walks map[string]*walkOnce
}

type readOnce struct {
	once sync.Once
	file loaded
	err  error
}

type walkOnce struct {
	once    sync.Once
	files   []string
	dropped []string
}

// Reading returns a resolver like NewResolver whose resolutions share one
// reading of the repository.
func Reading() *Resolver {
	r := NewResolver()
	r.reading = &reading{files: map[string]*readOnce{}, walks: map[string]*walkOnce{}}
	return r
}

// load reads and parses root/rel, once per Reading resolver, which keeps
// everything but the text (see loaded); a caller that wants the text gets
// it from contentOf or text.
func (r *Resolver) load(root, rel string) (loaded, error) {
	if r.reading == nil {
		return r.readAndParse(root, rel)
	}
	key := filepath.Join(root, filepath.FromSlash(rel))
	r.reading.mu.Lock()
	entry, ok := r.reading.files[key]
	if !ok {
		entry = &readOnce{}
		r.reading.files[key] = entry
	}
	r.reading.mu.Unlock()
	entry.once.Do(func() {
		entry.file, entry.err = r.readAndParse(root, rel)
		entry.file.data = nil
	})
	return entry.file, entry.err
}

// contentOf is file's text: the one it was parsed from, or root/rel read
// again when the reading dropped it.
func (r *Resolver) contentOf(root, rel string, file loaded) ([]byte, error) {
	if file.data != nil {
		return file.data, nil
	}
	data, _, err := r.readFile(root, rel)
	return data, err
}

// text is the content of root/rel.
func (r *Resolver) text(root, rel string) ([]byte, error) {
	file, err := r.load(root, rel)
	if err != nil {
		return nil, err
	}
	return r.contentOf(root, rel, file)
}

func (r *Resolver) readAndParse(root, rel string) (loaded, error) {
	data, truncated, err := r.readFile(root, rel)
	if err != nil {
		return loaded{}, err
	}
	st := r.structureOf(rel, data)
	lines := make([]int, len(st.Imports))
	for i, imp := range st.Imports {
		lines[i] = lineOf(data, indexOf(data, []byte(imp)))
	}
	return loaded{data: data, truncated: truncated, structure: st, importLines: lines}, nil
}

// enumerateOnce is enumerate, walked once per root by a Reading resolver;
// the walk's own notes go into every pack.
func (r *Resolver) enumerateOnce(root string, pack *Pack) []string {
	if r.reading == nil {
		return r.enumerate(root, pack)
	}
	r.reading.mu.Lock()
	walk, ok := r.reading.walks[root]
	if !ok {
		walk = &walkOnce{}
		r.reading.walks[root] = walk
	}
	r.reading.mu.Unlock()
	walk.once.Do(func() {
		notes := &Pack{}
		walk.files = r.enumerate(root, notes)
		walk.dropped = notes.Dropped
	})
	for _, note := range walk.dropped {
		pack.Dropped = appendDropped(pack.Dropped, note)
	}
	return append([]string(nil), walk.files...)
}
