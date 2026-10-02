// Package benchmark holds the versioned review-quality corpus and the
// deterministic harness that turns a set of findings into precision, recall,
// noise, duplication, location, latency and cost measurements.
package benchmark

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

const (
	CorpusSchema  = "aurum.benchmark-corpus"
	CorpusVersion = 1
)

type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
)

type Defect struct {
	ID        string   `json:"id"`
	File      string   `json:"file"`
	Line      int      `json:"line"`
	EndLine   int      `json:"end_line,omitempty"`
	Severity  Severity `json:"severity"`
	Category  string   `json:"category"`
	CrossFile bool     `json:"cross_file,omitempty"`
	Marker    string   `json:"marker,omitempty"`
}

type Case struct {
	ID          string   `json:"id"`
	Language    string   `json:"language"`
	Split       string   `json:"split"`
	Base        string   `json:"base"`
	Head        string   `json:"head"`
	Prompt      string   `json:"prompt"`
	Skills      []string `json:"skills,omitempty"`
	HasNegative bool     `json:"has_negative,omitempty"`
	Defects     []Defect `json:"defects"`
}

type Corpus struct {
	Schema   string `json:"schema"`
	Version  int    `json:"version"`
	FrozenAt string `json:"frozen_at"`
	Cases    []Case `json:"cases"`
}

func LoadCorpus(path string) (*Corpus, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read corpus %s: %w", path, err)
	}
	var corpus Corpus
	if err := json.Unmarshal(raw, &corpus); err != nil {
		return nil, fmt.Errorf("parse corpus %s: %w", path, err)
	}
	if err := corpus.Validate(); err != nil {
		return nil, fmt.Errorf("validate corpus %s: %w", path, err)
	}
	return &corpus, nil
}

// Validate enforces the frozen ground-truth contract: a dev/holdout split, at
// least two languages, a cross-file defect, at least one defect and at least
// one defect-free negative. A corpus that loses its negative or its holdout is
// no longer a valid benchmark and is refused before any scoring happens.
func (c *Corpus) Validate() error {
	if c.Schema != CorpusSchema {
		return fmt.Errorf("unexpected schema %q", c.Schema)
	}
	if c.Version != CorpusVersion {
		return fmt.Errorf("unexpected version %d", c.Version)
	}
	if strings.TrimSpace(c.FrozenAt) == "" {
		return fmt.Errorf("frozen_at must be set before execution")
	}
	if len(c.Cases) == 0 {
		return fmt.Errorf("corpus has no cases")
	}
	seen := make(map[string]bool)
	splits := map[string]int{"dev": 0, "holdout": 0}
	languages := make(map[string]bool)
	crossFile := false
	negatives := 0
	defectCount := 0
	for i := range c.Cases {
		cs := &c.Cases[i]
		if cs.ID == "" {
			return fmt.Errorf("case %d has no id", i)
		}
		if seen[cs.ID] {
			return fmt.Errorf("duplicate case id %q", cs.ID)
		}
		seen[cs.ID] = true
		switch cs.Split {
		case "dev", "holdout":
			splits[cs.Split]++
		default:
			return fmt.Errorf("case %q has invalid split %q", cs.ID, cs.Split)
		}
		if cs.Language == "" {
			return fmt.Errorf("case %q has no language", cs.ID)
		}
		languages[cs.Language] = true
		if cs.Base == "" || cs.Head == "" {
			return fmt.Errorf("case %q must pin base and head", cs.ID)
		}
		if cs.Prompt == "" {
			return fmt.Errorf("case %q must pin a prompt", cs.ID)
		}
		if cs.HasNegative && len(cs.Defects) != 0 {
			return fmt.Errorf("negative case %q must have no defects", cs.ID)
		}
		if cs.HasNegative {
			negatives++
		}
		for j := range cs.Defects {
			d := &cs.Defects[j]
			if d.ID == "" || d.File == "" || d.Category == "" || d.Line <= 0 {
				return fmt.Errorf("case %q has an incomplete defect", cs.ID)
			}
			switch d.Severity {
			case SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow:
			default:
				return fmt.Errorf("case %q defect %q has invalid severity %q", cs.ID, d.ID, d.Severity)
			}
			if d.CrossFile {
				crossFile = true
			}
			defectCount++
		}
	}
	if splits["dev"] == 0 || splits["holdout"] == 0 {
		return fmt.Errorf("corpus must freeze both dev and holdout cases")
	}
	if len(languages) < 2 {
		return fmt.Errorf("corpus must span at least two languages")
	}
	if !crossFile {
		return fmt.Errorf("corpus must contain a cross-file defect")
	}
	if negatives == 0 {
		return fmt.Errorf("corpus must contain at least one defect-free negative")
	}
	if defectCount == 0 {
		return fmt.Errorf("corpus must contain at least one defect")
	}
	return nil
}

func (c *Corpus) CaseByID(id string) (*Case, bool) {
	for i := range c.Cases {
		if c.Cases[i].ID == id {
			return &c.Cases[i], true
		}
	}
	return nil, false
}

// GroundTruthDigest is a stable hash of every defect. It changes when ground
// truth changes, so a run record can be bound to the exact corpus it scored.
func (c *Corpus) GroundTruthDigest() string {
	lines := make([]string, 0)
	for _, cs := range c.Cases {
		for _, d := range cs.Defects {
			lines = append(lines, fmt.Sprintf("%s|%s|%s|%d|%s|%s", cs.ID, d.ID, d.File, d.Line, d.Severity, d.Category))
		}
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return "sha256:" + hex.EncodeToString(sum[:])
}
