package reviewprofile

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// builtinSource names the embedded built-in profile file in errors.
const builtinSource = "builtin.yml"

// builtinYAML holds the code-owned built-ins as data. Each declares a version
// and only emphasis, families and instructions: a built-in that carried a
// forbidden clause would fail mustBuiltins at init, so the binary cannot start
// with a boundary-violating preset.
//
//go:embed builtin.yml
var builtinYAML []byte

// builtinSpecs are the decoded built-ins, in file order.
var builtinSpecs = mustDecodeBuiltins()

var builtins = mustBuiltins()

// decodeProfiles is the one decoder of a profile file, built-in or team: the
// raw scan refuses a forbidden clause anywhere (merge keys and aliases
// included) before the typed decode, and an unknown key fails closed.
func decodeProfiles(data []byte, source string) ([]Spec, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("reviewprofile: parse-error (%s): %w", source, err)
	}
	// The raw scan descends sequences, so every profile mapping inside the
	// `profiles:` list is checked.
	if len(doc.Content) > 0 {
		if err := scanRefusedClauses(doc.Content[0], ""); err != nil {
			return nil, err
		}
	}
	var parsed teamDoc
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("reviewprofile: parse-error (%s): %w", source, err)
	}
	return parsed.Profiles, nil
}

func mustDecodeBuiltins() []Spec {
	specs, err := decodeProfiles(builtinYAML, builtinSource)
	if err != nil {
		panic(fmt.Sprintf("reviewprofile: %s is invalid: %v", builtinSource, err))
	}
	return specs
}

func mustBuiltins() map[string]*Profile {
	m := make(map[string]*Profile, len(builtinSpecs))
	for _, spec := range builtinSpecs {
		if err := refuseForbidden(spec); err != nil {
			panic(fmt.Sprintf("reviewprofile: built-in %q violates the boundary: %v", spec.Name, err))
		}
		p, err := compileSpec(spec)
		if err != nil {
			panic(fmt.Sprintf("reviewprofile: built-in %q is invalid: %v", spec.Name, err))
		}
		m[strings.ToLower(p.Name)] = p
	}
	return m
}
