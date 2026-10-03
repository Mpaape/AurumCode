package config

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// yamlKeys lists the yaml keys of t's exported fields. An exported field
// without a yaml key is reported as its Go name, so it is never skipped.
func yamlKeys(t reflect.Type) []string {
	var keys []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		key := strings.Split(f.Tag.Get("yaml"), ",")[0]
		if key == "" || key == "-" {
			key = f.Name
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// assertClassified fails when a field of t has no classification, or a
// classification names a field t no longer has.
func assertClassified(t *testing.T, typ reflect.Type, classified []string) {
	t.Helper()
	want := yamlKeys(typ)
	if len(want) == 0 {
		t.Fatalf("%s has no fields; the reflection no longer sees it", typ.Name())
	}
	seen := map[string]int{}
	for _, k := range classified {
		seen[k]++
	}
	for _, k := range want {
		if seen[k] == 0 {
			t.Errorf("%s.%s has no central-policy classification: declare who decides it in governance.go", typ.Name(), k)
		}
	}
	fields := map[string]bool{}
	for _, k := range want {
		fields[k] = true
	}
	for k, n := range seen {
		if !fields[k] {
			t.Errorf("governance.go classifies %s.%s, which does not exist", typ.Name(), k)
		}
		if n > 1 {
			t.Errorf("%s.%s is classified %d times", typ.Name(), k, n)
		}
	}
}

// TestEveryConfigFieldHasCentralPolicyAuthority is the reflexive guard: a
// new field of Config, ReviewConfig or QualityGatesConfig fails here until
// governance.go says whether the policy or the repository decides it.
func TestEveryConfigFieldHasCentralPolicyAuthority(t *testing.T) {
	var sections []string
	for _, s := range governedSections {
		if s.apply == nil {
			t.Errorf("section %s has no rule", s.key)
		}
		sections = append(sections, s.key)
	}
	assertClassified(t, reflect.TypeOf(Config{}), sections)

	review := append([]string{}, reviewRepositoryFields...)
	for _, f := range reviewPolicyFields {
		review = append(review, f.key)
	}
	assertClassified(t, reflect.TypeOf(ReviewConfig{}), review)

	assertClassified(t, reflect.TypeOf(QualityGatesConfig{}), qualityGateSections)
}
