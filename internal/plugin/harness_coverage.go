package plugin

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// The harness reports lost coverage in three places, and findings.json is
// not enough: a real run exited 0 with `degraded: false` and no finding
// while its own SARIF said coverageDegraded (deep-dive units that failed
// or timed out) and errors.jsonl listed the failed analysis units. A
// vvah-findings ingest therefore also reads the harness's SARIF (required)
// and its errors.jsonl (when present).

// analysisStage matches the harness's analysis stages, s2 to s6, whose
// failure means part of the tree was not analysed.
var analysisStage = regexp.MustCompile(`^(?i)(s|step|stage)[_-]?[2-6]([^0-9]|$)`)

// newestMatching is the most recently modified file of dir matching
// pattern (a filepath.Match glob); "" when none.
func newestMatching(dir, pattern string) string {
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil || len(matches) == 0 {
		return ""
	}
	sort.Slice(matches, func(i, j int) bool {
		a, errA := os.Stat(matches[i])
		b, errB := os.Stat(matches[j])
		if errA != nil || errB != nil || a.ModTime().Equal(b.ModTime()) {
			return matches[i] > matches[j]
		}
		return a.ModTime().After(b.ModTime())
	})
	return matches[0]
}

// harnessSARIFCoverage reads the harness's own SARIF: absent, malformed,
// executionSuccessful false, hollow or coverageDegraded is inconclusive.
// Its results are not ingested (findings.json carries the verdicts).
func harnessSARIFCoverage(path string) error {
	if path == "" {
		return inconclusive(ReasonOutputMissing)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return inconclusive(ReasonOutputMissing)
	}
	_, err = parseSARIF(data)
	return err
}

// harnessErrors reads errors.jsonl: any record of an analysis stage (s2 to
// s6), or a record that cannot be read as one, is inconclusive. A missing
// file means no error was recorded.
func harnessErrors(path string) error {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return inconclusive(ReasonOutputInvalid)
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), len(data)+1)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec map[string]any
		if json.Unmarshal([]byte(line), &rec) != nil {
			return inconclusive(ReasonOutputInvalid)
		}
		stage := ""
		for _, key := range []string{"stage", "step"} {
			if s, ok := rec[key].(string); ok {
				stage = strings.TrimSpace(s)
				break
			}
		}
		// A record without a readable stage cannot be proved harmless.
		if stage == "" || analysisStage.MatchString(stage) {
			return inconclusive(ReasonAnalysisErrors)
		}
	}
	if sc.Err() != nil {
		return inconclusive(ReasonOutputInvalid)
	}
	return nil
}
