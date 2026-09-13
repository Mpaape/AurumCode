// Package memory implements the optional review memory described by ADR-0001.
// It exposes three explicit modes: "off" (a stateless no-op, the default when
// the mode is empty), "ephemeral" (notes kept in process memory only), and
// "local" (notes persisted as a single JSON file under a per-user cache
// directory).
//
// Notes are observations only: loading or saving them never grants approval,
// publication authority, model access, or any other capability. The package is
// standard-library only, and every store is safe for concurrent use.
package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Mode names accepted by New.
const (
	ModeOff       = "off"
	ModeEphemeral = "ephemeral"
	ModeLocal     = "local"
)

const localFileName = "notes.json"

// Note is a single remembered observation tied to a review rule and a path
// pattern. Action is one of "suppress", "require", or "note".
type Note struct {
	ID          string    `json:"id"`
	RuleID      string    `json:"rule_id"`
	PathPattern string    `json:"path_pattern"`
	Action      string    `json:"action"`
	Author      string    `json:"author"`
	At          time.Time `json:"at"`
	Body        string    `json:"body"`
}

// Store is the minimal read/write contract shared by all memory modes.
type Store interface {
	// Load returns every remembered note, deterministically ordered.
	Load() ([]Note, error)
	// Save atomically replaces the remembered notes with the given slice.
	Save([]Note) error
}

// New returns a Store for the given mode. The empty mode and ModeOff return a
// no-op store that never persists anything. ModeEphemeral keeps notes in
// process memory. ModeLocal persists notes as a single JSON file under dir;
// when dir is empty it defaults to os.UserCacheDir()/aurumcode.
func New(mode, dir string) (Store, error) {
	switch strings.TrimSpace(mode) {
	case "", ModeOff:
		return noopStore{}, nil
	case ModeEphemeral:
		return &ephemeralStore{}, nil
	case ModeLocal:
		if dir == "" {
			base, err := os.UserCacheDir()
			if err != nil {
				return nil, fmt.Errorf("memory: resolve cache dir: %w", err)
			}
			dir = filepath.Join(base, "aurumcode")
		}
		return &localStore{path: filepath.Join(dir, localFileName)}, nil
	default:
		return nil, fmt.Errorf("memory: unsupported mode %q (want off, ephemeral, or local)", mode)
	}
}

// noopStore is the default off mode: a stateless store that never writes and
// always loads an empty result.
type noopStore struct{}

func (noopStore) Load() ([]Note, error) { return nil, nil }
func (noopStore) Save([]Note) error     { return nil }

// ephemeralStore keeps notes in process memory for the lifetime of the run.
type ephemeralStore struct {
	mu    sync.Mutex
	notes []Note
}

func (s *ephemeralStore) Load() ([]Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sortedCopy(s.notes), nil
}

func (s *ephemeralStore) Save(notes []Note) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notes = append([]Note(nil), notes...)
	return nil
}

// localStore persists notes as a single JSON file, written atomically via a
// temporary file and rename with restrictive permissions.
type localStore struct {
	mu   sync.Mutex
	path string
}

func (s *localStore) Load() ([]Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("memory: load %s: %w", s.path, err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	var notes []Note
	if err := json.Unmarshal(data, &notes); err != nil {
		return nil, fmt.Errorf("memory: decode %s: %w", s.path, err)
	}
	return sortedCopy(notes), nil
}

func (s *localStore) Save(notes []Note) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("memory: create dir for %s: %w", s.path, err)
	}
	data, err := json.Marshal(notes)
	if err != nil {
		return fmt.Errorf("memory: encode notes: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".notes-*.tmp")
	if err != nil {
		return fmt.Errorf("memory: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("memory: write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("memory: sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("memory: close temp file: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("memory: chmod temp file: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("memory: rename temp file: %w", err)
	}
	tmpName = "" // committed; nothing left to clean up
	return nil
}

// sortedCopy returns a copy of notes ordered by At, then ID, so every Load is
// deterministic regardless of insertion or on-disk order.
func sortedCopy(notes []Note) []Note {
	out := append([]Note(nil), notes...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].At.Equal(out[j].At) {
			return out[i].ID < out[j].ID
		}
		return out[i].At.Before(out[j].At)
	})
	return out
}
