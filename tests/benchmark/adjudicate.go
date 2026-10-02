package benchmark

import (
	"fmt"
	"sort"
	"strings"
)

// DivergenceItem is a disagreement between two pipelines about one finding.
// Origin names the pipeline; it is stripped before a human sees the item.
type DivergenceItem struct {
	ID      string  `json:"id"`
	CaseID  string  `json:"case_id"`
	Finding Finding `json:"finding"`
	Origin  string  `json:"origin"`
}

type Adjudication struct {
	AnonID  string `json:"anon_id"`
	Verdict string `json:"verdict"`
	Judge   string `json:"judge"`
}

// Anonymize replaces origin-identifying ids with opaque labels and returns the
// hidden mapping. The adjudicator sees only the anonymized item, so a verdict
// cannot be influenced by which product produced the finding.
func Anonymize(items []DivergenceItem) ([]DivergenceItem, map[string]string) {
	order := make([]int, len(items))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		left := items[order[a]]
		right := items[order[b]]
		if left.CaseID != right.CaseID {
			return left.CaseID < right.CaseID
		}
		return left.ID < right.ID
	})
	anon := make([]DivergenceItem, 0, len(items))
	mapping := make(map[string]string, len(items))
	for rank, idx := range order {
		label := fmt.Sprintf("DIV-%03d", rank+1)
		item := items[idx]
		mapping[label] = item.Origin
		item.ID = label
		item.Origin = ""
		anon = append(anon, item)
	}
	return anon, mapping
}

func Verdicts(adjudications []Adjudication) map[string]string {
	verdicts := make(map[string]string, len(adjudications))
	for _, a := range adjudications {
		verdicts[a.AnonID] = a.Verdict
	}
	return verdicts
}

func LeaksOrigin(anon []DivergenceItem, origins []string) bool {
	for _, item := range anon {
		haystack := strings.ToLower(item.ID + " " + item.CaseID)
		for _, origin := range origins {
			if origin == "" {
				continue
			}
			if strings.Contains(haystack, strings.ToLower(origin)) {
				return true
			}
		}
	}
	return false
}
