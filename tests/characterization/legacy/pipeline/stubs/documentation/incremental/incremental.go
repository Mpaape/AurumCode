package incremental

import (
	"context"
	"path/filepath"

	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

var events []string

func ResetTrace() { events = nil }

func Trace() []string { return append([]string(nil), events...) }

func record(event string) { events = append(events, event) }

type Manager struct {
	sourceDir string
}

func NewManager(_ site.CommandRunner, sourceDir string) *Manager {
	record("new")
	return &Manager{sourceDir: sourceDir}
}

func (m *Manager) LoadCache() error {
	record("load")
	return nil
}

func (m *Manager) GetChangedFiles(context.Context) ([]string, error) {
	record("get")
	return []string{filepath.Join(m.sourceDir, "sample.go")}, nil
}

func (*Manager) RegisterDocumentation(_ string, _ ...string) { record("register_documentation") }

func (*Manager) RegisterLanguage(_ string, _ ...string) { record("register_language") }

func (*Manager) UpdateCommit(context.Context) error {
	record("update")
	return nil
}

func (*Manager) SaveCache() error {
	record("save")
	return nil
}
