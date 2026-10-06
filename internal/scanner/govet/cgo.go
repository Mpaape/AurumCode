package govet

import (
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// cgoImport is the pseudo-package a cgo file imports.
const cgoImport = "C"

// ErrCgoUnvetted: a package the reviewed range touched has a file that
// imports "C". The child go runs with CGO_ENABLED=0 (the pull request
// controls the #cgo directives), so go vet leaves every such file out of
// the package it vets without a word; reporting the package as vetted would
// read a file nobody looked at as clean. The scan is inconclusive and the
// error names each file.
var ErrCgoUnvetted = errors.New("govet: package has a cgo file go vet does not vet (CGO_ENABLED=0)")

// unvettedCgoFiles returns, sorted and relative to root, every file that
// imports "C" in a directory holding a Go file the range added lines to.
func unvettedCgoFiles(root string, added lineSet) ([]string, error) {
	dirs := map[string]bool{}
	for rel := range added {
		if strings.HasSuffix(rel, ".go") && filepath.IsLocal(filepath.FromSlash(rel)) {
			dirs[path.Dir(rel)] = true
		}
	}
	var found []string
	for dir := range dirs {
		files, err := cgoFilesIn(root, dir)
		if err != nil {
			return nil, err
		}
		found = append(found, files...)
	}
	sort.Strings(found)
	return found, nil
}

// cgoFilesIn lists the .go files of root/dir whose imports include "C". A
// directory the head removed has nothing to vet; a file that does not parse
// is left to go vet, which fails the scan on it.
func cgoFilesIn(root, dir string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(dir)))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("govet: reading %s: %w", dir, err)
	}
	var out []string
	fset := token.NewFileSet()
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		rel := path.Join(dir, entry.Name())
		file, err := parser.ParseFile(fset, filepath.Join(root, filepath.FromSlash(rel)), nil, parser.ImportsOnly)
		if err != nil {
			continue
		}
		for _, spec := range file.Imports {
			if value, err := strconv.Unquote(spec.Path.Value); err == nil && value == cgoImport {
				out = append(out, rel)
				break
			}
		}
	}
	return out, nil
}

// refuseUnvettedCgo turns cgo files in a touched package into ErrCgoUnvetted.
func refuseUnvettedCgo(root string, added lineSet) error {
	files, err := unvettedCgoFiles(root, added)
	if err != nil {
		return err
	}
	if len(files) > 0 {
		return fmt.Errorf("%w: %s", ErrCgoUnvetted, strings.Join(files, ", "))
	}
	return nil
}
