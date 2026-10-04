package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Source is where a set of skill directories is read from: a directory on
// disk, or a repository ref read through an API. It only lists and reads; it
// never interprets the documents.
type Source interface {
	// Dirs lists the skill directory names, in any order. A source that does
	// not exist lists none and returns a nil error.
	Dirs() ([]string, error)
	// Read returns the SKILL.md of one skill directory; found is false when
	// the directory has none.
	Read(dir string) (data []byte, found bool, err error)
	// Label is the path shown for a skill directory in warnings and in the
	// prompt provenance.
	Label(dir string) string
}

// DirSource reads skills from a directory on disk. Prefix is the path shown
// for the directory in labels (a repository-relative path keeps warnings
// independent of where the tree is checked out).
type DirSource struct {
	Dir    string
	Prefix string
}

// Dirs lists the immediate subdirectories. A missing directory is the
// zero-config case.
func (d DirSource) Dirs() ([]string, error) {
	entries, err := os.ReadDir(d.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("skills: reading %s: %w", d.Dir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// Read returns dir/SKILL.md.
func (d DirSource) Read(dir string) ([]byte, bool, error) {
	data, err := os.ReadFile(filepath.Join(d.Dir, dir, DocName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return data, true, nil
}

// Label is Prefix/dir.
func (d DirSource) Label(dir string) string {
	return filepath.ToSlash(filepath.Join(d.Prefix, dir))
}

// LoadSource reads every skill directory of src, in sorted order. A source
// with no skills is an empty Set; an unreadable document or an
// opened-but-unterminated front matter block is a loud error.
func LoadSource(src Source) (*Set, error) {
	names, err := src.Dirs()
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	set := &Set{}
	for _, name := range names {
		label := src.Label(name)
		data, found, err := src.Read(name)
		if err != nil {
			return nil, fmt.Errorf("skills: reading %s/%s: %w", label, DocName, err)
		}
		if !found {
			continue
		}
		sk, err := parseSkill(name, label+"/"+DocName, string(data))
		if err != nil {
			return nil, err
		}
		set.Skills = append(set.Skills, sk)
	}
	return set, nil
}

// LoadSourceSkippingMalformed reads every skill of src like LoadSource, but a
// SKILL.md that cannot be parsed (malformed front matter) only drops that one
// skill: it comes back as a warning naming the file, and the other skills
// still load. A source that cannot be listed or read is still an error. The
// repository's skills use this; the central policy's use LoadSource, so a
// broken policy skill fails closed.
func LoadSourceSkippingMalformed(src Source) (*Set, []string, error) {
	names, err := src.Dirs()
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(names)
	set := &Set{}
	var warnings []string
	for _, name := range names {
		label := src.Label(name)
		data, found, err := src.Read(name)
		if err != nil {
			return nil, nil, fmt.Errorf("skills: reading %s/%s: %w", label, DocName, err)
		}
		if !found {
			continue
		}
		sk, err := parseSkill(name, label+"/"+DocName, string(data))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("repository skill %s/%s unavailable, the other skills still apply: %v", label, DocName, err))
			continue
		}
		set.Skills = append(set.Skills, sk)
	}
	return set, warnings, nil
}
