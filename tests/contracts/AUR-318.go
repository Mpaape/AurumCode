//go:build aur318_characterization

package contracts

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	taskmasterlegacy "github.com/Mpaape/AurumCode/tests/characterization/legacy/taskmaster"
)

const contractTasksJSON = `{"master":{"tasks":[` +
	`{"id":1,"status":"done","subtasks":[{"id":1,"status":"done"},{"id":2,"status":"done"}]},` +
	`{"id":"2","status":"done","subtasks":[]}` +
	`],"metadata":{"taskCount":2}}}`

func writeContractSnapshot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "tasks"), 0o750); err != nil {
		t.Fatalf("snapshot scaffold failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tasks", "tasks.json"), []byte(contractTasksJSON), 0o600); err != nil {
		t.Fatalf("snapshot tasks.json write failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("legacy instructions, audit input only\n"), 0o600); err != nil {
		t.Fatalf("snapshot instructions write failed: %v", err)
	}
	return dir
}

// ContractAUR318 pins the public contract of the freeze fixture: bounded
// fail-closed snapshot loading, taskmaster/queue disjointness, complete audit
// preservation, deterministic ordering, a stable typed refusal, and a tree
// comparison that actually detects divergent bytes.
func ContractAUR318(t *testing.T) {
	dir := writeContractSnapshot(t)
	snapshot, err := taskmasterlegacy.LoadLegacySnapshot(dir)
	if err != nil {
		t.Fatalf("bounded snapshot did not load: %v", err)
	}
	if len(snapshot.Tasks) != 4 {
		t.Fatalf("expected 2 tasks + 2 subtasks flattened, got %d", len(snapshot.Tasks))
	}
	if len(snapshot.DoneTasks()) != 4 {
		t.Fatalf("expected every flattened task to be done, got %d", len(snapshot.DoneTasks()))
	}
	if snapshot.InstructionBytes == 0 {
		t.Fatal("embedded instructions were not measured as audit input")
	}
	if snapshot.Tasks[1].ID != "1.1" {
		t.Fatalf("subtask flattening lost its parent prefix: %q", snapshot.Tasks[1].ID)
	}

	if _, err := taskmasterlegacy.LoadLegacySnapshot(t.TempDir()); !errors.Is(err, taskmasterlegacy.ErrSnapshotInvalid) {
		t.Fatalf("empty snapshot must fail closed as invalid, got %v", err)
	}

	oversized := writeContractSnapshot(t)
	big := strings.Repeat("x", taskmasterlegacy.MaxSnapshotBytes)
	if err := os.WriteFile(filepath.Join(oversized, "huge.txt"), []byte(big), 0o600); err != nil {
		t.Fatalf("oversized fixture write failed: %v", err)
	}
	if _, err := taskmasterlegacy.LoadLegacySnapshot(oversized); !errors.Is(err, taskmasterlegacy.ErrSnapshotUnbounded) {
		t.Fatalf("oversized snapshot must fail closed as unbounded, got %v", err)
	}

	crowded := writeContractSnapshot(t)
	for index := 0; index <= taskmasterlegacy.MaxSnapshotFiles; index++ {
		name := filepath.Join(crowded, fmt.Sprintf("extra-%03d.txt", index))
		if err := os.WriteFile(name, []byte("x"), 0o600); err != nil {
			t.Fatalf("crowded fixture write failed: %v", err)
		}
	}
	if _, err := taskmasterlegacy.LoadLegacySnapshot(crowded); !errors.Is(err, taskmasterlegacy.ErrSnapshotUnbounded) {
		t.Fatalf("snapshot above the file bound must fail closed as unbounded, got %v", err)
	}

	candidates := append(taskmasterlegacy.LegacyCandidates(snapshot),
		taskmasterlegacy.QueueCandidate{Origin: taskmasterlegacy.OriginBoard, ID: "AUR-901", Status: "ready"},
		taskmasterlegacy.QueueCandidate{Origin: taskmasterlegacy.OriginBoard, ID: "AUR-902", Status: "ready"},
		taskmasterlegacy.QueueCandidate{Origin: taskmasterlegacy.OriginBoard, ID: "AUR-904", Status: "backlog"},
	)
	first := taskmasterlegacy.FeedBoardQueue(candidates)
	second := taskmasterlegacy.FeedBoardQueue(candidates)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("queue feeding is not deterministic for identical input")
	}
	for _, item := range first.Queue {
		if item.Origin == taskmasterlegacy.OriginTaskmaster {
			t.Fatalf("queue and taskmaster origins are not disjoint: %q", item.ID)
		}
	}
	if len(first.Queue) != 2 {
		t.Fatalf("expected exactly the 2 ready board candidates in the queue, got %d", len(first.Queue))
	}
	auditedLegacy := 0
	for _, record := range first.Audit {
		if record.Origin == taskmasterlegacy.OriginTaskmaster {
			auditedLegacy++
			if record.Disposition != taskmasterlegacy.DispositionAuditOnly {
				t.Fatalf("legacy record carries disposition %q instead of audit-only", record.Disposition)
			}
		}
	}
	if auditedLegacy != len(snapshot.Tasks) {
		t.Fatalf("audit lost legacy records: %d of %d preserved", auditedLegacy, len(snapshot.Tasks))
	}

	for _, status := range []string{"done", "ready", "pending"} {
		err := taskmasterlegacy.PromoteDoneToBoard(taskmasterlegacy.QueueCandidate{
			Origin: taskmasterlegacy.OriginTaskmaster,
			ID:     "taskmaster:7",
			Status: status,
		})
		if err == nil {
			t.Fatalf("taskmaster candidate with status %q was promoted", status)
		}
		if status == "done" && !errors.Is(err, taskmasterlegacy.ErrLegacyTransitionRefused) {
			t.Fatalf("legacy done refusal lost its typed error: %v", err)
		}
	}

	identical, err := taskmasterlegacy.CompareTrees(dir, dir)
	if err != nil || !identical {
		t.Fatalf("identical trees must compare equal: %v %v", identical, err)
	}
	diverged := writeContractSnapshot(t)
	if err := os.WriteFile(filepath.Join(diverged, "CLAUDE.md"), []byte("tampered instructions\n"), 0o600); err != nil {
		t.Fatalf("divergence fixture write failed: %v", err)
	}
	same, err := taskmasterlegacy.CompareTrees(dir, diverged)
	if err != nil {
		t.Fatalf("tree comparison errored on divergent bytes: %v", err)
	}
	if same {
		t.Fatal("tree comparison reported divergent bytes as identical")
	}
}
