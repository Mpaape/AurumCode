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
// reading is shared. A reading belongs to one review: files changed on disk
// after their first reading are not read again.

// loaded is one file as read and parsed.
type loaded struct {
	data      []byte
	truncated bool
	structure grammar.Structure
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

// load reads and parses root/rel, once per Reading resolver. The content of
// a file with neither structure nor imports is not kept: nothing matches in
// it, and a caller that still wants its text reads it again.
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
		if !entry.file.structure.HasStructure && len(entry.file.structure.Imports) == 0 {
			entry.file.data = nil
		}
	})
	return entry.file, entry.err
}

// text is the content of root/rel, from the reading when it kept it.
func (r *Resolver) text(root, rel string) ([]byte, error) {
	file, err := r.load(root, rel)
	if err != nil || file.data != nil {
		return file.data, err
	}
	data, _, err := r.readFile(root, rel)
	return data, err
}

func (r *Resolver) readAndParse(root, rel string) (loaded, error) {
	data, truncated, err := r.readFile(root, rel)
	if err != nil {
		return loaded{}, err
	}
	return loaded{data: data, truncated: truncated, structure: r.structureOf(rel, data)}, nil
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
