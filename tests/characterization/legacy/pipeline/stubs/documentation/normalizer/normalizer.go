package normalizer

import (
	"io/fs"
	"path/filepath"
)

var events []string

func ResetTrace() { events = nil }

func Trace() []string { return append([]string(nil), events...) }

type Normalizer struct{}

func NewNormalizer(_ string) *Normalizer { return &Normalizer{} }

func (*Normalizer) NormalizeDir(root string) (int, []error) {
	events = append(events, "normalize")
	count := 0
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && filepath.Ext(path) == ".md" {
			count++
		}
		return nil
	})
	return count, nil
}
