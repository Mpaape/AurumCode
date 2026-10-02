package xbom

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	maxFileBytes = 1 << 20
	maxLineBytes = 4000
)

// Occurrence is one cited place: repo-relative slash path, 1-based line and
// the evidence token that must appear on that line.
type Occurrence struct {
	Location string
	Line     int
	Token    string
}

// Component is an xBOM component before serialization.
type Component struct {
	Type        string
	Name        string
	Version     string
	Purl        string
	Description string
	Properties  map[string]string
	Crypto      map[string]any
	Occurrences []Occurrence
}

// Key identifies a component: occurrences of the same rendered component merge.
func (c *Component) Key() string {
	cr, _ := json.Marshal(c.Crypto)
	h := sha256.Sum256([]byte(strings.Join([]string{c.Type, c.Name, c.Version, c.Purl, string(cr)}, "\x00")))
	return hex.EncodeToString(h[:])
}

// Candidate is a deterministic extraction result handed to the LLM.
type Candidate struct {
	ID        string
	Entry     string
	Component *Component
	Line      string
}

func (c *Catalog) matchingEntries(rel string) []*Entry {
	var out []*Entry
	for i := range c.Entries {
		e := &c.Entries[i]
		for _, g := range e.globs {
			if g.MatchString(rel) {
				out = append(out, e)
				break
			}
		}
	}
	return out
}

// Extract walks root and applies the catalog. It never follows symlinks and
// skips binary or oversized files. Results are unverified candidates.
func Extract(root string, c *Catalog) ([]*Candidate, error) {
	var cands []*Candidate
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if c.excluded(rel, true) {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || c.excluded(rel, false) {
			return nil
		}
		entries := c.matchingEntries(rel)
		if len(entries) == 0 {
			return nil
		}
		lines, ok := readLines(p)
		if !ok {
			return nil
		}
		for i, line := range lines {
			if len(line) > maxLineBytes {
				continue
			}
			for _, e := range entries {
				for _, m := range e.re.FindAllStringSubmatch(line, -1) {
					g := map[string]string{}
					for gi, n := range e.re.SubexpNames() {
						if n != "" {
							g[n] = m[gi]
						}
					}
					token := g[e.Token]
					if token == "" {
						continue
					}
					comp := renderComponent(e, g)
					if comp == nil {
						continue
					}
					comp.Occurrences = []Occurrence{{Location: rel, Line: i + 1, Token: token}}
					cands = append(cands, &Candidate{Entry: e.ID, Component: comp, Line: strings.TrimSpace(line)})
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("xbom: walking %s: %w", root, err)
	}
	sort.SliceStable(cands, func(i, j int) bool {
		a, b := cands[i].Component.Occurrences[0], cands[j].Component.Occurrences[0]
		if a.Location != b.Location {
			return a.Location < b.Location
		}
		return a.Line < b.Line
	})
	for i, cd := range cands {
		cd.ID = fmt.Sprintf("c%d", i+1)
	}
	return cands, nil
}

func renderComponent(e *Entry, g map[string]string) *Component {
	t := e.Component
	c := &Component{
		Type:        t.Type,
		Name:        collapseName(renderString(t.Name, g)),
		Version:     renderString(t.Version, g),
		Purl:        strings.TrimSuffix(renderString(t.Purl, g), "@"),
		Description: renderString(t.Description, g),
		Properties:  map[string]string{"aurumcode:xbom:entry": e.ID},
	}
	if c.Name == "" {
		return nil
	}
	for k, v := range t.Properties {
		if s := renderString(v, g); s != "" {
			c.Properties[k] = s
		}
	}
	if len(t.Crypto) > 0 {
		if r, ok := renderValue(map[string]any(t.Crypto), g).(map[string]any); ok {
			c.Crypto = r
		}
	}
	return c
}

func readLines(p string) ([]string, bool) {
	fi, err := os.Lstat(p)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxFileBytes {
		return nil, false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, false
	}
	head := data
	if len(head) > 8000 {
		head = head[:8000]
	}
	if bytes.IndexByte(head, 0) >= 0 {
		return nil, false
	}
	var lines []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), maxFileBytes+1)
	for sc.Scan() {
		lines = append(lines, strings.TrimSuffix(sc.Text(), "\r"))
	}
	return lines, true
}

// merge folds components with the same key, joining their occurrences.
func merge(comps []*Component) []*Component {
	byKey := map[string]*Component{}
	var order []string
	for _, c := range comps {
		k := c.Key()
		if ex, ok := byKey[k]; ok {
			ex.Occurrences = append(ex.Occurrences, c.Occurrences...)
			for pk, pv := range c.Properties {
				if _, has := ex.Properties[pk]; !has {
					ex.Properties[pk] = pv
				}
			}
			if ex.Description == "" {
				ex.Description = c.Description
			}
			continue
		}
		byKey[k] = c
		order = append(order, k)
	}
	out := make([]*Component, 0, len(order))
	for _, k := range order {
		out = append(out, byKey[k])
	}
	return out
}
