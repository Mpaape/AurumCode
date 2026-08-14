//go:build aur318_characterization

package integration

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	taskmasterlegacy "github.com/Mpaape/AurumCode/tests/characterization/legacy/taskmaster"
)

func resolveAUR318Dir(t *testing.T, envName string, relative string) string {
	t.Helper()
	if fromEnv := os.Getenv(envName); fromEnv != "" {
		return fromEnv
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("cwd unavailable: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return filepath.Join(dir, filepath.FromSlash(relative))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("repository root not found for %s", relative)
		}
		dir = parent
	}
}

func emitAUR318(observation string, value string, effect string) {
	fmt.Printf("AUR318_OBSERVATION\t%s\t%s\t%s\n", observation, value, effect)
}

// IntegrationAUR318 feeds the board queue with the bounded audit copy of the
// frozen .taskmaster tree plus board-native candidates, attempts the forbidden
// legacy promotion, and emits one observation line per characterized fact. It
// records outcomes; the acceptance baseline decides pass or fail.
func IntegrationAUR318(t *testing.T) {
	fixtureDir := resolveAUR318Dir(t, "AUR318_FIXTURE_DIR", "tests/characterization/legacy/taskmaster")
	taskmasterDir := resolveAUR318Dir(t, "AUR318_TASKMASTER_DIR", ".taskmaster")
	snapshotDir := filepath.Join(fixtureDir, "snapshot")

	snapshot, err := taskmasterlegacy.LoadLegacySnapshot(snapshotDir)
	if err != nil {
		t.Fatalf("bounded audit copy did not load: %v", err)
	}

	identical, err := taskmasterlegacy.CompareTrees(taskmasterDir, snapshotDir)
	if err != nil {
		t.Fatalf("byte comparison against the live .taskmaster tree failed: %v", err)
	}
	bytesLabel := "identical"
	if !identical {
		bytesLabel = "divergent"
	}

	boardCandidates := []taskmasterlegacy.QueueCandidate{
		{Origin: taskmasterlegacy.OriginBoard, ID: "AUR-901", Status: "ready"},
		{Origin: taskmasterlegacy.OriginBoard, ID: "AUR-902", Status: "ready"},
	}
	feed := taskmasterlegacy.FeedBoardQueue(append(boardCandidates, taskmasterlegacy.LegacyCandidates(snapshot)...))

	taskmasterQueued := 0
	boardAdmitted := 0
	for _, item := range feed.Queue {
		switch item.Origin {
		case taskmasterlegacy.OriginTaskmaster:
			taskmasterQueued++
		case taskmasterlegacy.OriginBoard:
			boardAdmitted++
		}
	}
	taskmasterAudited := 0
	for _, record := range feed.Audit {
		if record.Origin == taskmasterlegacy.OriginTaskmaster {
			taskmasterAudited++
		}
	}

	doneTasks := snapshot.DoneTasks()
	if len(doneTasks) == 0 {
		t.Fatal("audit copy carries no done task to attempt the forbidden promotion with")
	}
	legacyPromotionErr := taskmasterlegacy.PromoteDoneToBoard(taskmasterlegacy.QueueCandidate{
		Origin: taskmasterlegacy.OriginTaskmaster,
		ID:     "taskmaster:" + doneTasks[0].ID,
		Status: doneTasks[0].Status,
	})
	legacyEffect := "allowed"
	if errors.Is(legacyPromotionErr, taskmasterlegacy.ErrLegacyTransitionRefused) {
		legacyEffect = "refused"
	}
	boardPromotionErr := taskmasterlegacy.PromoteDoneToBoard(taskmasterlegacy.QueueCandidate{
		Origin: taskmasterlegacy.OriginBoard,
		ID:     "AUR-903",
		Status: "done",
	})

	queueEffect := "excluded"
	if taskmasterQueued != 0 {
		queueEffect = "admitted"
	}

	emitAUR318("audit_taskmaster_records", strconv.Itoa(taskmasterAudited), "audit-only")
	emitAUR318("board_ready_admitted", strconv.Itoa(boardAdmitted), "selected")
	emitAUR318("promote_board_done", "exit:"+strconv.Itoa(taskmasterlegacy.PromotionExitCode(boardPromotionErr)), "board-transition")
	emitAUR318("promote_taskmaster_done", "exit:"+strconv.Itoa(taskmasterlegacy.PromotionExitCode(legacyPromotionErr)), legacyEffect)
	emitAUR318("queue_taskmaster_items", strconv.Itoa(taskmasterQueued), queueEffect)
	emitAUR318("snapshot_instruction_bytes", strconv.FormatInt(snapshot.InstructionBytes, 10), "audit-only")
	emitAUR318("taskmaster_bytes", bytesLabel, "audit-input")
	emitAUR318("taskmaster_done_tasks", strconv.Itoa(len(doneTasks)), "audit-only")
}
