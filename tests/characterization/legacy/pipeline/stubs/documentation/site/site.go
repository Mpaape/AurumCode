package site

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var events []string

func ResetTrace() { events = nil }

func Trace() []string { return append([]string(nil), events...) }

func record(event string) { events = append(events, event) }

func Redact(value string) string {
	if canary := os.Getenv("AURUM_SECRET_CANARY"); canary != "" {
		value = strings.ReplaceAll(value, canary, "[REDACTED]")
	}
	if len(value) > 1024 {
		return value[:1024] + "...[truncated]"
	}
	return value
}

type CommandRunner interface {
	Run(context.Context, string, []string, string, map[string]string) (string, error)
}

type ScaffoldConfig struct {
	DocsDir     string
	OutputDir   string
	Title       string
	Description string
	BaseURL     string
}

type ScaffoldResult struct {
	IndexPath     string
	Pages         []string
	ConfigPath    string
	ConfigCreated bool
}

type Scaffold struct {
	config ScaffoldConfig
}

func NewScaffold(config ScaffoldConfig) *Scaffold { return &Scaffold{config: config} }

func (s *Scaffold) Generate() (*ScaffoldResult, error) {
	record("generate")
	if err := os.MkdirAll(s.config.DocsDir, 0o755); err != nil {
		return nil, err
	}
	pages := make([]string, 0)
	_ = filepath.Walk(s.config.OutputDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() && filepath.Ext(path) == ".md" {
			pages = append(pages, path)
		}
		return nil
	})
	sort.Strings(pages)
	indexPath := filepath.Join(s.config.DocsDir, "index.md")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		if err := os.WriteFile(indexPath, []byte("# "+s.config.Title+"\n"), 0o600); err != nil {
			return nil, err
		}
	}
	configPath := filepath.Join(s.config.DocsDir, "_config.yml")
	created := false
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := os.WriteFile(configPath, []byte("baseurl: \""+s.config.BaseURL+"\"\n"), 0o600); err != nil {
			return nil, err
		}
		created = true
	}
	return &ScaffoldResult{IndexPath: indexPath, Pages: pages, ConfigPath: configPath, ConfigCreated: created}, nil
}
