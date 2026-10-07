package changelog

import (
	_ "embed"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// The required-changelog check (AUR-509). Verify decides, from the two sides
// of the changelog file in a pull request, whether the change adds a useful
// and concise entry. It is pure: no git, no model, no network. The entry text
// is untrusted data; it is only compared, counted and quoted, never executed
// or interpreted as an instruction.

//go:embed require_defaults.yml
var requireDefaultsYAML []byte

// Requirement is the effective rule set of the check.
type Requirement struct {
	File            string   `yaml:"file"`
	Section         string   `yaml:"section"`
	MaxEntryLines   int      `yaml:"max_entry_lines"`
	MaxLineLength   int      `yaml:"max_line_length"`
	MaxReleaseLines int      `yaml:"max_release_lines"`
	MinWords        int      `yaml:"min_words"`
	AgentLogMarkers []string `yaml:"agent_log_markers"`
}

// DefaultRequirement decodes the embedded defaults (require_defaults.yml).
func DefaultRequirement() (Requirement, error) {
	var r Requirement
	if err := yaml.Unmarshal(requireDefaultsYAML, &r); err != nil {
		return Requirement{}, fmt.Errorf("changelog: embedded defaults: %w", err)
	}
	if err := r.Validate(); err != nil {
		return Requirement{}, fmt.Errorf("changelog: embedded defaults: %w", err)
	}
	return r, nil
}

// Validate refuses a rule set that could never fail an entry.
func (r Requirement) Validate() error {
	switch {
	case strings.TrimSpace(r.File) == "":
		return fmt.Errorf("file must not be empty")
	case strings.TrimSpace(r.Section) == "":
		return fmt.Errorf("section must not be empty")
	case r.MaxEntryLines <= 0, r.MaxLineLength <= 0, r.MaxReleaseLines <= 0, r.MinWords <= 0:
		return fmt.Errorf("limits must be positive")
	}
	return nil
}

// FileState is what the diff says about the changelog file.
type FileState int

const (
	// FileUntouched: the pull request does not change the file (or the file
	// does not exist on either side).
	FileUntouched FileState = iota
	// FileChanged: both sides are known; Old is empty for a new file.
	FileChanged
	// FileUnreadable: the file changed but its content could not be read
	// (binary, too large, deleted side unknown). Never a pass.
	FileUnreadable
)

// Change is the changelog file as the pull request leaves it.
type Change struct {
	State FileState
	Old   string
	New   string
}

// Reasons are stable identifiers printed by the check and asserted by tests.
const (
	ReasonOK            = "entrada_valida"
	ReasonMissing       = "entrada_ausente"
	ReasonWhitespace    = "apenas_espacos"
	ReasonNoInformation = "sem_informacao_nova"
	ReasonTooLong       = "entrada_longa"
	ReasonAgentLog      = "log_de_agente"
	ReasonIndeterminate = "indeterminado"
)

// Verdict is the decision of the check. OK is true only for ReasonOK.
type Verdict struct {
	OK     bool
	Reason string
	Detail string
}

func fail(reason, detail string) Verdict { return Verdict{Reason: reason, Detail: detail} }

// Verify applies r to c. Anything the check cannot establish is a failure.
func (r Requirement) Verify(c Change) Verdict {
	if err := r.Validate(); err != nil {
		return fail(ReasonIndeterminate, "regras inválidas: "+err.Error())
	}
	switch c.State {
	case FileUnreadable:
		return fail(ReasonIndeterminate, r.File+" mudou, mas o conteúdo não pode ser lido")
	case FileChanged:
	default:
		return fail(ReasonMissing, "a PR não altera "+r.File)
	}
	if normalizeLine(c.Old) == normalizeLine(c.New) {
		return fail(ReasonWhitespace, "a alteração de "+r.File+" muda apenas espaços ou quebras de linha")
	}
	added := addedLines(c.Old, c.New)
	return r.judge(added)
}

// judge decides on the lines the pull request added.
func (r Requirement) judge(added []addedLine) Verdict {
	entry, release := 0, 0
	for _, l := range added {
		if marker := r.agentLogMarker(l.text); marker != "" {
			return fail(ReasonAgentLog, fmt.Sprintf("linha %d parece log de agente ou de ferramenta (%q)", l.number, marker))
		}
		if len([]rune(strings.TrimSpace(l.text))) > r.MaxLineLength {
			return fail(ReasonTooLong, fmt.Sprintf("linha %d passa de %d caracteres", l.number, r.MaxLineLength))
		}
		if !r.informative(l.text) {
			continue
		}
		switch {
		case sectionMatches(l.section, r.Section):
			entry++
		case isReleaseHeading(l.section):
			release++
		}
	}
	if entry > r.MaxEntryLines {
		return fail(ReasonTooLong, fmt.Sprintf("a entrada em %s tem %d linhas; o limite é %d", r.Section, entry, r.MaxEntryLines))
	}
	if release > r.MaxReleaseLines {
		return fail(ReasonTooLong, fmt.Sprintf("as notas da release têm %d linhas; o limite é %d", release, r.MaxReleaseLines))
	}
	if entry+release == 0 {
		return fail(ReasonNoInformation, "nenhuma linha nova com informação de mudança em "+r.Section+" nem numa seção de versão")
	}
	return Verdict{OK: true, Reason: ReasonOK, Detail: fmt.Sprintf("%d linha(s) nova(s) em %s, %d em notas de versão", entry, r.Section, release)}
}

// informative reports whether an added line carries change information: not
// blank, not a heading, at least MinWords words once the list marker is gone.
func (r Requirement) informative(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" || strings.HasPrefix(t, "#") {
		return false
	}
	t = strings.TrimSpace(strings.TrimLeft(t, "-*+>"))
	return len(strings.Fields(t)) >= r.MinWords
}

func (r Requirement) agentLogMarker(text string) string {
	lower := strings.ToLower(text)
	for _, m := range r.AgentLogMarkers {
		if m = strings.TrimSpace(m); m != "" && strings.Contains(lower, strings.ToLower(m)) {
			return m
		}
	}
	return ""
}
