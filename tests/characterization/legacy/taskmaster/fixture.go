//go:build aur318_characterization

// Package taskmasterlegacy characterizes the frozen .taskmaster backlog for
// AUR-318. The bounded snapshot under snapshot/ is the audit copy of the
// legacy tree: its done states and embedded instructions are historical data
// only. This package models the board queue's selection boundary so the
// acceptance can prove that no .taskmaster item enters the queue and that a
// legacy done state can never authorize a board transition.
package taskmasterlegacy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	// OriginBoard marks a candidate produced by the board's own card lanes.
	OriginBoard = "board"
	// OriginTaskmaster marks a candidate derived from the legacy .taskmaster tree.
	OriginTaskmaster = "taskmaster"

	// MaxSnapshotFiles bounds the audit copy: the legacy tree is 21 files.
	MaxSnapshotFiles = 64
	// MaxSnapshotBytes bounds the audit copy to 1 MiB.
	MaxSnapshotBytes = 1 << 20
	// maxTasksFileBytes bounds the tasks.json parse input.
	maxTasksFileBytes = 512 * 1024

	// DispositionAuditOnly labels a legacy record preserved as audit input.
	DispositionAuditOnly = "audit-only"
	// DispositionNotReady labels a board candidate that is not selectable yet.
	DispositionNotReady = "not-ready"

	tasksRelPath        = "tasks/tasks.json"
	instructionsRelPath = "CLAUDE.md"
)

// ErrLegacyTransitionRefused is the stable refusal for any board transition
// derived from .taskmaster state.
var ErrLegacyTransitionRefused = errors.New("aur318: .taskmaster state is audit input only and cannot authorize a board transition")

// ErrSnapshotUnbounded reports an audit copy that exceeds its declared bounds.
var ErrSnapshotUnbounded = errors.New("aur318: legacy snapshot exceeds its declared bounds")

// ErrSnapshotInvalid reports an unreadable or structurally invalid audit copy.
var ErrSnapshotInvalid = errors.New("aur318: legacy snapshot is not a readable taskmaster tree")

// LegacyTask is one flattened task or subtask from the audit copy. Titles,
// details and instructions are deliberately not retained: audit minimization
// keeps free prose out of every rendered surface.
type LegacyTask struct {
	ID     string
	Status string
}

// LegacySnapshot is the bounded audit view of the frozen .taskmaster tree.
type LegacySnapshot struct {
	FileCount        int
	TotalBytes       int64
	InstructionBytes int64
	Tasks            []LegacyTask
}

// QueueCandidate is one item offered to the board queue feeder.
type QueueCandidate struct {
	Origin string
	ID     string
	Status string
}

// QueueItem is one item actually admitted into the board queue.
type QueueItem struct {
	Origin string
	ID     string
}

// AuditRecord is one candidate preserved outside the queue.
type AuditRecord struct {
	Origin      string
	ID          string
	Status      string
	Disposition string
}

// FeedResult is the deterministic outcome of one queue feed.
type FeedResult struct {
	Queue []QueueItem
	Audit []AuditRecord
}

// LoadLegacySnapshot reads the bounded audit copy, enforcing the declared
// file-count and byte bounds fail-closed before any parse.
func LoadLegacySnapshot(dir string) (*LegacySnapshot, error) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%w: %s", ErrSnapshotInvalid, dir)
	}
	snapshot := &LegacySnapshot{}
	walkErr := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("%w: %v", ErrSnapshotInvalid, err)
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("%w: irregular entry %s", ErrSnapshotInvalid, entry.Name())
		}
		fileInfo, infoErr := entry.Info()
		if infoErr != nil {
			return fmt.Errorf("%w: %v", ErrSnapshotInvalid, infoErr)
		}
		snapshot.FileCount++
		snapshot.TotalBytes += fileInfo.Size()
		if snapshot.FileCount > MaxSnapshotFiles || snapshot.TotalBytes > MaxSnapshotBytes {
			return ErrSnapshotUnbounded
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	if snapshot.FileCount == 0 {
		return nil, fmt.Errorf("%w: empty tree", ErrSnapshotInvalid)
	}
	instructionInfo, err := os.Stat(filepath.Join(dir, instructionsRelPath))
	if err != nil || instructionInfo.IsDir() || instructionInfo.Size() == 0 {
		return nil, fmt.Errorf("%w: missing embedded instructions %s", ErrSnapshotInvalid, instructionsRelPath)
	}
	snapshot.InstructionBytes = instructionInfo.Size()
	tasks, err := parseTasksFile(filepath.Join(dir, tasksRelPath))
	if err != nil {
		return nil, err
	}
	snapshot.Tasks = tasks
	return snapshot, nil
}

func parseTasksFile(path string) ([]LegacyTask, error) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return nil, fmt.Errorf("%w: missing %s", ErrSnapshotInvalid, tasksRelPath)
	}
	if info.Size() > maxTasksFileBytes {
		return nil, fmt.Errorf("%w: %s exceeds %d bytes", ErrSnapshotUnbounded, tasksRelPath, maxTasksFileBytes)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSnapshotInvalid, err)
	}
	type rawSubtask struct {
		ID     json.RawMessage `json:"id"`
		Status string          `json:"status"`
	}
	type rawTask struct {
		ID       json.RawMessage `json:"id"`
		Status   string          `json:"status"`
		Subtasks []rawSubtask    `json:"subtasks"`
	}
	type rawTag struct {
		Tasks []rawTask `json:"tasks"`
	}
	var document map[string]rawTag
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSnapshotInvalid, err)
	}
	tagNames := make([]string, 0, len(document))
	for name := range document {
		tagNames = append(tagNames, name)
	}
	sort.Strings(tagNames)
	var tasks []LegacyTask
	for _, name := range tagNames {
		for _, task := range document[name].Tasks {
			taskID := formatTaskID(task.ID)
			tasks = append(tasks, LegacyTask{ID: taskID, Status: task.Status})
			for _, subtask := range task.Subtasks {
				tasks = append(tasks, LegacyTask{
					ID:     taskID + "." + formatTaskID(subtask.ID),
					Status: subtask.Status,
				})
			}
		}
	}
	if len(tasks) == 0 {
		return nil, fmt.Errorf("%w: no tasks in %s", ErrSnapshotInvalid, tasksRelPath)
	}
	return tasks, nil
}

func formatTaskID(raw json.RawMessage) string {
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return sanitizeToken(asString)
	}
	var asNumber json.Number
	if err := json.Unmarshal(raw, &asNumber); err == nil {
		return sanitizeToken(asNumber.String())
	}
	return "invalid"
}

// sanitizeToken keeps identifiers bounded and free of arbitrary prose: only
// [A-Za-z0-9._-] survives and the result is truncated to 64 bytes.
func sanitizeToken(token string) string {
	var builder strings.Builder
	for _, char := range token {
		if builder.Len() >= 64 {
			break
		}
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z',
			char >= '0' && char <= '9', char == '.', char == '_', char == '-':
			builder.WriteRune(char)
		}
	}
	if builder.Len() == 0 {
		return "redacted"
	}
	return builder.String()
}

// DoneTasks returns the tasks recorded as done in the audit copy.
func (snapshot *LegacySnapshot) DoneTasks() []LegacyTask {
	var done []LegacyTask
	for _, task := range snapshot.Tasks {
		if task.Status == "done" {
			done = append(done, task)
		}
	}
	return done
}

// LegacyCandidates converts every audited task into a queue candidate carrying
// the taskmaster origin.
func LegacyCandidates(snapshot *LegacySnapshot) []QueueCandidate {
	candidates := make([]QueueCandidate, 0, len(snapshot.Tasks))
	for _, task := range snapshot.Tasks {
		candidates = append(candidates, QueueCandidate{
			Origin: OriginTaskmaster,
			ID:     "taskmaster:" + task.ID,
			Status: task.Status,
		})
	}
	return candidates
}

// FeedBoardQueue selects the board queue from the offered candidates. The
// frozen .taskmaster tree is never a selection source: every taskmaster-origin
// candidate is preserved as an audit-only record, regardless of its status.
func FeedBoardQueue(candidates []QueueCandidate) FeedResult {
	var result FeedResult
	for _, candidate := range candidates {
		if candidate.Origin == OriginTaskmaster {
			result.Audit = append(result.Audit, AuditRecord{
				Origin:      candidate.Origin,
				ID:          sanitizeToken(candidate.ID),
				Status:      sanitizeToken(candidate.Status),
				Disposition: DispositionAuditOnly,
			})
			continue
		}
		if candidate.Status == "ready" || candidate.Status == "done" {
			result.Queue = append(result.Queue, QueueItem{
				Origin: candidate.Origin,
				ID:     sanitizeToken(candidate.ID),
			})
			continue
		}
		result.Audit = append(result.Audit, AuditRecord{
			Origin:      candidate.Origin,
			ID:          sanitizeToken(candidate.ID),
			Status:      sanitizeToken(candidate.Status),
			Disposition: DispositionNotReady,
		})
	}
	sort.Slice(result.Queue, func(left, right int) bool {
		if result.Queue[left].Origin != result.Queue[right].Origin {
			return result.Queue[left].Origin < result.Queue[right].Origin
		}
		return result.Queue[left].ID < result.Queue[right].ID
	})
	sort.Slice(result.Audit, func(left, right int) bool {
		if result.Audit[left].Origin != result.Audit[right].Origin {
			return result.Audit[left].Origin < result.Audit[right].Origin
		}
		return result.Audit[left].ID < result.Audit[right].ID
	})
	return result
}

// PromoteDoneToBoard applies one promotion request against the board. A
// candidate derived from .taskmaster is refused unconditionally: its done
// state is historical audit input and can never authorize a board transition.
func PromoteDoneToBoard(candidate QueueCandidate) error {
	if candidate.Origin == OriginTaskmaster {
		return ErrLegacyTransitionRefused
	}
	if candidate.Status != "done" {
		return fmt.Errorf("aur318: only a done candidate can request promotion, got %q", sanitizeToken(candidate.Status))
	}
	return nil
}

// PromotionExitCode maps a promotion outcome onto the process exit convention.
func PromotionExitCode(err error) int {
	if err == nil {
		return 0
	}
	return 1
}

// CompareTrees reports whether two directory trees contain the same relative
// regular files with identical bytes.
func CompareTrees(leftDir string, rightDir string) (bool, error) {
	leftFiles, err := listTreeFiles(leftDir)
	if err != nil {
		return false, err
	}
	rightFiles, err := listTreeFiles(rightDir)
	if err != nil {
		return false, err
	}
	if len(leftFiles) != len(rightFiles) {
		return false, nil
	}
	for index := range leftFiles {
		if leftFiles[index] != rightFiles[index] {
			return false, nil
		}
		leftBytes, err := os.ReadFile(filepath.Join(leftDir, leftFiles[index]))
		if err != nil {
			return false, err
		}
		rightBytes, err := os.ReadFile(filepath.Join(rightDir, rightFiles[index]))
		if err != nil {
			return false, err
		}
		if string(leftBytes) != string(rightBytes) {
			return false, nil
		}
	}
	return true, nil
}

func listTreeFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		files = append(files, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// AuditSummary renders a bounded, prose-free summary of one feed. It contains
// only counts, so instructions, titles or hostile identifiers can never leak
// into logs or observations.
func AuditSummary(result FeedResult) string {
	taskmasterQueued := 0
	for _, item := range result.Queue {
		if item.Origin == OriginTaskmaster {
			taskmasterQueued++
		}
	}
	taskmasterAudited := 0
	for _, record := range result.Audit {
		if record.Origin == OriginTaskmaster {
			taskmasterAudited++
		}
	}
	return fmt.Sprintf("queue=%d taskmaster_in_queue=%d audit=%d taskmaster_audit_records=%d",
		len(result.Queue), taskmasterQueued, len(result.Audit), taskmasterAudited)
}
