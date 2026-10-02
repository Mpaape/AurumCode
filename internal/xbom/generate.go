package xbom

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/sbom"
)

// SpecVersion is the CycloneDX version this package writes.
const SpecVersion = "1.6"

// Options drives Generate.
type Options struct {
	Type    string
	Root    string
	Catalog *Catalog
	// Provider is nil when no LLM is configured: the BOM then carries only
	// the deterministic evidence and metadata records llm=absent.
	Provider llm.Provider
	Prompt   string
	// Redact, when set, is applied to every repository line sent to the model.
	Redact func(string) string
	Now    time.Time
}

// Result is a generated BOM plus its accounting.
type Result struct {
	JSON       []byte
	Components int
	Dropped    VerifyResult
	LLM        LLMOutcome
}

// Generate extracts, optionally classifies with the LLM, verifies evidence
// and serializes the BOM. It does not write files.
func Generate(o Options) (*Result, error) {
	cands, err := Extract(o.Root, o.Catalog)
	if err != nil {
		return nil, err
	}
	res := &Result{}
	var comps []*Component
	if o.Provider != nil {
		kept, extra, out := enrich(o.Provider, o.Prompt, cands, o.Redact, o.Catalog)
		res.LLM = out
		comps = append(kept, extra...)
	} else {
		res.LLM.Status = "absent"
		for _, c := range cands {
			comps = append(comps, c.Component)
		}
	}
	comps = merge(comps)
	comps, res.Dropped = verifyEvidence(o.Root, comps)
	res.Components = len(comps)

	now := o.Now
	if now.IsZero() {
		now = time.Now()
	}
	data, err := marshalBOM(o, comps, res, len(cands), now)
	if err != nil {
		return nil, err
	}
	res.JSON = data
	return res, nil
}

func propList(m map[string]string) []map[string]string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]map[string]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, map[string]string{"name": k, "value": m[k]})
	}
	return out
}

func newSerial() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("urn:uuid:%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func marshalBOM(o Options, comps []*Component, res *Result, ncand int, now time.Time) ([]byte, error) {
	sort.SliceStable(comps, func(i, j int) bool {
		a, b := comps[i], comps[j]
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		if a.Version != b.Version {
			return a.Version < b.Version
		}
		return a.Key() < b.Key()
	})
	var jc []map[string]any
	for _, c := range comps {
		sort.SliceStable(c.Occurrences, func(i, j int) bool {
			a, b := c.Occurrences[i], c.Occurrences[j]
			if a.Location != b.Location {
				return a.Location < b.Location
			}
			return a.Line < b.Line
		})
		var occ []map[string]any
		seen := map[string]bool{}
		for _, oc := range c.Occurrences {
			k := oc.Location + ":" + strconv.Itoa(oc.Line)
			if seen[k] {
				continue
			}
			seen[k] = true
			occ = append(occ, map[string]any{"location": oc.Location, "line": oc.Line, "additionalContext": oc.Token})
		}
		m := map[string]any{
			"bom-ref":  "xbom-" + c.Key()[:12],
			"type":     c.Type,
			"name":     c.Name,
			"evidence": map[string]any{"occurrences": occ},
		}
		if c.Version != "" {
			m["version"] = c.Version
		}
		if c.Purl != "" {
			m["purl"] = c.Purl
		}
		if c.Description != "" {
			m["description"] = c.Description
		}
		if len(c.Crypto) > 0 {
			m["cryptoProperties"] = c.Crypto
		}
		if len(c.Properties) > 0 {
			m["properties"] = propList(c.Properties)
		}
		jc = append(jc, m)
	}
	if jc == nil {
		jc = []map[string]any{}
	}
	meta := map[string]string{
		"aurumcode:xbom:type":                     o.Type,
		"aurumcode:xbom:catalog":                  o.Catalog.Source,
		"aurumcode:xbom:llm":                      res.LLM.Status,
		"aurumcode:xbom:candidates":               strconv.Itoa(ncand),
		"aurumcode:xbom:dropped_without_evidence": strconv.Itoa(res.Dropped.DroppedComponents),
		"aurumcode:xbom:dropped_occurrences":      strconv.Itoa(res.Dropped.DroppedOccurrences),
	}
	if o.Provider != nil {
		meta["aurumcode:xbom:llm_excluded"] = strconv.Itoa(res.LLM.Excluded)
		meta["aurumcode:xbom:llm_rejected"] = strconv.Itoa(res.LLM.Rejected)
	}
	bom := map[string]any{
		"bomFormat":    "CycloneDX",
		"specVersion":  SpecVersion,
		"serialNumber": newSerial(),
		"version":      1,
		"metadata": map[string]any{
			"timestamp":  now.UTC().Format(time.RFC3339),
			"tools":      map[string]any{"components": []map[string]string{{"type": "application", "name": "aurumcode"}}},
			"component":  map[string]string{"type": "application", "name": filepath.Base(absOr(o.Root))},
			"properties": propList(meta),
		},
		"components": jc,
	}
	return json.MarshalIndent(bom, "", "  ")
}

func absOr(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}

// ValidateBOM checks a serialized BOM: the AUR-549 validation (bomFormat and
// specVersion >= minSpec, via internal/sbom) plus this package's own
// structural contract: every component cites at least one occurrence with a
// relative location and a positive line, and every cryptographic-asset has a
// valid cryptoProperties.assetType.
func ValidateBOM(path, minSpec string) error {
	if err := sbom.Validate(path, minSpec); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc struct {
		Components []struct {
			Name     string         `json:"name"`
			Type     string         `json:"type"`
			Crypto   map[string]any `json:"cryptoProperties"`
			Evidence struct {
				Occurrences []struct {
					Location string `json:"location"`
					Line     int    `json:"line"`
				} `json:"occurrences"`
			} `json:"evidence"`
		} `json:"components"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	for _, c := range doc.Components {
		if len(c.Evidence.Occurrences) == 0 {
			return fmt.Errorf("xbom: component %q has no evidence.occurrences", c.Name)
		}
		for _, oc := range c.Evidence.Occurrences {
			if oc.Location == "" || filepath.IsAbs(oc.Location) || strings.HasPrefix(oc.Location, "..") || oc.Line < 1 {
				return fmt.Errorf("xbom: component %q has an invalid occurrence (%q:%d)", c.Name, oc.Location, oc.Line)
			}
		}
		if c.Type == "cryptographic-asset" {
			if at, _ := c.Crypto["assetType"].(string); !cryptoAssetTypes[at] {
				return fmt.Errorf("xbom: crypto component %q lacks a valid cryptoProperties.assetType", c.Name)
			}
		}
	}
	return nil
}
