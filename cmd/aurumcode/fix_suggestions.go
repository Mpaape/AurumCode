// Reading the suggestions `aurumcode fix` applies: the flag set and the
// review JSON file the suggestions come from.
package main

import (
	"encoding/json"
	"flag"
	"io"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// newFixFlagSet declares the flags of `fix`; the help reads the same set.
func newFixFlagSet() (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet("fix", flag.ContinueOnError)
	file := fs.String("file", "", "JSON file with review suggestions or a review response (default: stdin)")
	return fs, file
}

// parseFixSuggestions accepts either a bare JSON array of suggestions or a
// full review response object whose "suggestions" field holds them.
func parseFixSuggestions(data []byte) ([]types.ReviewSuggestion, error) {
	var suggestions []types.ReviewSuggestion
	if err := json.Unmarshal(data, &suggestions); err == nil {
		return suggestions, nil
	}
	var result struct {
		Suggestions []types.ReviewSuggestion `json:"suggestions"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result.Suggestions, nil
}

// readFixSuggestions reads the suggestions JSON from --file, or from stdin
// when --file is empty.
func readFixSuggestions(file string) ([]byte, error) {
	if strings.TrimSpace(file) != "" {
		return os.ReadFile(file)
	}
	return io.ReadAll(os.Stdin)
}
