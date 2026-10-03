package config

import (
	"bytes"
	"errors"
	"io"

	"gopkg.in/yaml.v3"
)

// decodeStrict decodes a configuration document refusing every key the
// schema does not know, in every section. A misspelled key ("fial_on"
// instead of "fail_on") would otherwise be dropped silently and leave the
// section it belonged to undeclared -- a gate that approves because it was
// never configured. The repository's config.yml and the central policy go
// through this same decoder, so both name the unknown key and its line. An
// empty (or comment-only) document is the valid zero configuration.
func decodeStrict(data []byte) (*Config, error) {
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return &cfg, nil
}
