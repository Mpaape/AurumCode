// Review memory: the optional store of earlier findings a review reads before
// the model call and updates after it.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Mpaape/AurumCode/internal/memory"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// notesFromIssues turns this round's findings into review-memory notes,
// merged with the existing ones and deduplicated by id. It is bounded so a
// pathological diff cannot grow the memory file without limit.
func notesFromIssues(existing []memory.Note, issues []types.ReviewIssue) []memory.Note {
	notes := append([]memory.Note(nil), existing...)
	seen := make(map[string]bool, len(notes))
	for _, n := range notes {
		seen[n.ID] = true
	}
	for _, issue := range issues {
		id := fmt.Sprintf("%s:%s:%d", issue.RuleID, issue.File, issue.Line)
		if seen[id] {
			continue
		}
		seen[id] = true
		notes = append(notes, memory.Note{
			ID:          id,
			RuleID:      issue.RuleID,
			PathPattern: issue.File,
			Action:      "note",
			Body:        issue.Message,
			At:          time.Now(),
		})
		if len(notes) >= 500 {
			break
		}
	}
	return notes
}

// openReviewMemory is the memory pass's open-and-load half, shared by both
// paths. mode is review.memory ("off", "ephemeral", "local"); owner/repo are
// empty for the local --base path, where the directory falls back to the
// checkout's origin remote or path hash (memorydir.go, AUR-489). The returned
// store reads and writes that directory; the returned notes are its current
// contents (as untrusted observations), and text is their JSON encoding for
// ReviewContext.MemoryNotes. On any error the store degrades to the off store
// and the reason is reported on stderr -- memory is observation, never
// instruction, so it can never fail a review.
func openReviewMemory(mode, owner, repo string, stderr io.Writer, filter *redaction.Filter) (memory.Store, []memory.Note, string) {
	store, err := newRepoMemory(mode, owner, repo)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode review: review memory unavailable: %s; continuing without it\n", filter.Redact(err.Error()))
		store, _ = memory.New("off", "")
	}
	notes, loadErr := store.Load()
	if loadErr != nil {
		fmt.Fprintf(stderr, "aurumcode review: loading review memory: %s; continuing without prior observations\n", filter.Redact(loadErr.Error()))
		notes = nil
	}
	text := ""
	if len(notes) > 0 {
		if data, merr := json.Marshal(notes); merr == nil {
			text = string(data)
		}
	}
	return store, notes, text
}

// persistReviewMemory is the memory pass's save half, shared by both paths. It
// writes this round's findings as notes for the next run, merged with the
// existing notes and deduplicated (notesFromIssues). Memory is observation,
// never instruction; a save failure is reported and never affects the verdict.
// Disabled ("" or "off") memory is a no-op, matching the PR path's published
// behavior of never saving when memory is off.
func persistReviewMemory(store memory.Store, mode string, existing []memory.Note, issues []types.ReviewIssue, stderr io.Writer, filter *redaction.Filter) {
	if strings.TrimSpace(mode) == "" || strings.TrimSpace(mode) == "off" {
		return
	}
	if err := store.Save(notesFromIssues(existing, issues)); err != nil {
		fmt.Fprintf(stderr, "aurumcode review: saving review memory: %v\n", filter.Redact(err.Error()))
	}
}
