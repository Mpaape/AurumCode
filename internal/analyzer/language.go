package analyzer

import (
	_ "embed"
	"path"
	"strings"
	"sync"
	"unicode"

	"gopkg.in/yaml.v3"

	"github.com/Mpaape/AurumCode/internal/grammar"
)

//go:embed language_catalog.yml
var languageCatalogYAML []byte

type languageCatalog struct {
	Categories  map[string][]string `yaml:"categories"`
	ConfigFiles []string            `yaml:"config_files"`
}

var (
	catalogOnce   sync.Once
	catalogByLang map[string]string
	catalogConfig map[string]bool
)

func loadCatalog() {
	catalogOnce.Do(func() {
		catalogByLang = map[string]string{}
		catalogConfig = map[string]bool{}
		var c languageCatalog
		if err := yaml.Unmarshal(languageCatalogYAML, &c); err != nil {
			return // an unreadable catalog degrades every category to "other"
		}
		for cat, names := range c.Categories {
			for _, n := range names {
				catalogByLang[n] = cat
			}
		}
		for _, f := range c.ConfigFiles {
			catalogConfig[strings.ToLower(f)] = true
		}
	})
}

// LanguageDetector names the language of a file. It holds no table of its own:
// the answer comes from the grammar runtime (internal/grammar).
type LanguageDetector struct{}

// NewLanguageDetector creates a new language detector
func NewLanguageDetector() *LanguageDetector { return &LanguageDetector{} }

// DetectLanguage returns the runtime's grammar name for a file path, or
// "unknown" when the runtime has no grammar for it. Only the path is known
// here; content-based detection (shebangs) lives in grammar.Detect.
func (d *LanguageDetector) DetectLanguage(filePath string) string {
	if name := grammar.Detect(filePath, nil); name != "" {
		return name
	}
	return grammar.NoStructure
}

// IsTestFile checks if a file is a test file from language-neutral naming
// conventions: a "test"/"tests"/"spec" word in the file name or a test
// directory in its path.
func (d *LanguageDetector) IsTestFile(filePath string) bool {
	p := strings.ToLower(strings.ReplaceAll(filePath, "\\", "/"))
	for _, dir := range strings.Split(path.Dir(p), "/") {
		switch dir {
		case "test", "tests", "__tests__", "spec", "specs":
			return true
		}
	}
	base := path.Base(filePath)
	stem := strings.TrimSuffix(base, path.Ext(base))
	for _, w := range nameWords(stem) {
		switch w {
		case "test", "tests", "spec", "specs":
			return true
		}
	}
	return false
}

// nameWords splits a file stem on separators and lower/upper camel boundaries.
func nameWords(stem string) []string {
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	runes := []rune(stem)
	for i, r := range runes {
		switch {
		case r == '.' || r == '_' || r == '-' || r == ' ':
			flush()
		case unicode.IsUpper(r) && i > 0 && unicode.IsLower(runes[i-1]):
			flush()
			cur = append(cur, r)
		default:
			cur = append(cur, r)
		}
	}
	flush()
	return words
}

// IsConfigFile checks if a file is configuration: a grammar the catalog files
// under "config", or a well-known configuration file name from the catalog.
func (d *LanguageDetector) IsConfigFile(filePath string) bool {
	loadCatalog()
	if catalogConfig[strings.ToLower(path.Base(filePath))] {
		return true
	}
	return d.GetLanguageCategory(d.DetectLanguage(filePath)) == "config"
}

// GetLanguageCategory returns the catalog category of a grammar name, or
// "other" when the catalog has no entry for it.
func (d *LanguageDetector) GetLanguageCategory(language string) string {
	loadCatalog()
	if cat, ok := catalogByLang[language]; ok {
		return cat
	}
	return "other"
}
