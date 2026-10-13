package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/i18n"
	"gopkg.in/yaml.v3"
)

// validateLayerConfig enforces layer.Sections on the config.yml at path.
// AnySection accepts anything (the file is decoded by its own loader).
// NoSection accepts a missing, empty or comment-only file and refuses
// every other shape, naming what was found: a document that is not a
// mapping, or a mapping with any section. Every document of a
// multi-document file is checked, so a second document cannot carry a
// section past the check.
func validateLayerConfig(layer Layer, path, language string) error {
	if layer.Sections == AnySection {
		return nil
	}
	language = layer.ErrorLang(language)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.New(i18n.Format(language, "layer.load_failed", layer.Prefix(language), err.Error()))
	}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	var found []string
	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return errors.New(i18n.Format(language, "layer.config_invalid", layer.Prefix(language), path, err.Error()))
		}
		keys, kind := documentKeys(&doc)
		if kind != "" {
			return errors.New(i18n.Format(language, "layer.config_not_mapping", layer.Prefix(language), path, i18n.Text(language, "layer.kind."+kind)))
		}
		for _, key := range keys {
			found = append(found, fmt.Sprintf("%q", key))
		}
	}
	if len(found) > 0 {
		return errors.New(i18n.Format(language, "layer.config_forbidden", layer.Prefix(language), path, strings.Join(found, ", ")))
	}
	return nil
}

// documentKeys returns the top-level keys of one decoded YAML document. An
// empty or null document has none. For a document that is not a mapping,
// kind names what it is instead ("sequence", "scalar", "alias": the
// layer.kind.* catalog keys), so the error can say what was found. A merge
// key ("<<") is a key like any other.
func documentKeys(doc *yaml.Node) (keys []string, kind string) {
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return nil, ""
	}
	body := doc.Content[0]
	switch body.Kind {
	case yaml.MappingNode:
	case yaml.ScalarNode:
		if body.Tag == "!!null" {
			return nil, ""
		}
		return nil, "scalar"
	case yaml.SequenceNode:
		return nil, "sequence"
	default:
		return nil, "alias"
	}
	for i := 0; i+1 < len(body.Content); i += 2 {
		keys = append(keys, body.Content[i].Value)
	}
	return keys, ""
}
