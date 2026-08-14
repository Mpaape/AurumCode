//go:build aur318_characterization

package unit

import (
	"errors"
	"strings"
	"testing"

	taskmasterlegacy "github.com/Mpaape/AurumCode/tests/characterization/legacy/taskmaster"
)

// TestAUR318 proves the freeze boundary at the unit level: a .taskmaster done
// state can never authorize a board transition, the queue feeder never admits
// a taskmaster-origin candidate, and no candidate prose (including a hostile
// canary-bearing identifier) reaches the bounded audit summary.
func TestAUR318(t *testing.T) {
	canary := "aur318-unit-canary"
	t.Setenv("AURUM_SECRET_CANARY", canary)

	legacyDone := taskmasterlegacy.QueueCandidate{
		Origin: taskmasterlegacy.OriginTaskmaster,
		ID:     "taskmaster:12",
		Status: "done",
	}
	err := taskmasterlegacy.PromoteDoneToBoard(legacyDone)
	if !errors.Is(err, taskmasterlegacy.ErrLegacyTransitionRefused) {
		t.Fatalf("promotion of a taskmaster done task was not refused with the typed error: %v", err)
	}
	if exit := taskmasterlegacy.PromotionExitCode(err); exit != 1 {
		t.Fatalf("refused legacy promotion must map to exit 1, got %d", exit)
	}

	boardDone := taskmasterlegacy.QueueCandidate{
		Origin: taskmasterlegacy.OriginBoard,
		ID:     "AUR-903",
		Status: "done",
	}
	if err := taskmasterlegacy.PromoteDoneToBoard(boardDone); err != nil {
		t.Fatalf("board-native done promotion must not be refused by the freeze: %v", err)
	}
	if exit := taskmasterlegacy.PromotionExitCode(nil); exit != 0 {
		t.Fatalf("board-native promotion must map to exit 0, got %d", exit)
	}

	hostile := taskmasterlegacy.QueueCandidate{
		Origin: taskmasterlegacy.OriginTaskmaster,
		ID:     "task-" + canary,
		Status: "done",
	}
	feed := taskmasterlegacy.FeedBoardQueue([]taskmasterlegacy.QueueCandidate{
		hostile,
		{Origin: taskmasterlegacy.OriginBoard, ID: "AUR-901", Status: "ready"},
	})
	for _, item := range feed.Queue {
		if item.Origin == taskmasterlegacy.OriginTaskmaster {
			t.Fatalf("queue admitted a taskmaster-origin item: %q", item.ID)
		}
	}
	if len(feed.Queue) != 1 || feed.Queue[0].ID != "AUR-901" {
		t.Fatalf("board ready candidate was not selected exactly once: %+v", feed.Queue)
	}
	audited := false
	for _, record := range feed.Audit {
		if record.Origin != taskmasterlegacy.OriginTaskmaster {
			continue
		}
		audited = true
		if record.Disposition != taskmasterlegacy.DispositionAuditOnly {
			t.Fatalf("taskmaster record must be audit-only, got %q", record.Disposition)
		}
	}
	if !audited {
		t.Fatal("taskmaster candidate disappeared instead of being preserved as audit input")
	}

	summary := taskmasterlegacy.AuditSummary(feed)
	if strings.Contains(summary, canary) {
		t.Fatalf("audit summary leaked candidate prose: %q", summary)
	}
	if !strings.Contains(summary, "taskmaster_in_queue=0") {
		t.Fatalf("audit summary does not record the empty taskmaster queue: %q", summary)
	}
}
