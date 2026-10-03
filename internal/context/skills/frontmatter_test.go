package skills

import (
	"reflect"
	"strings"
	"testing"
)

// TestFrontMatterAcceptsTheShapesSkillsUse: inline lists, block lists and a
// comma-separated scalar give the same selector; malformed YAML is an error,
// never a skill read with half its selector.
func TestFrontMatterAcceptsTheShapesSkillsUse(t *testing.T) {
	for name, content := range map[string]string{
		"inline": "---\nname: s\nversion: 2\nlanguages: [go, \"ts\"]\npaths: ['internal/**']\n---\nbody\n",
		"block":  "---\nname: s\nversion: 2\nlanguages:\n  - go\n  - ts\npaths:\n  - internal/**\n---\nbody\n",
		"scalar": "---\nname: s\nversion: 2\nlanguages: go, ts\npaths: internal/**\nextra: ignored\n---\nbody\n",
	} {
		sk, err := parseSkill("dir", "dir/SKILL.md", content)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want := Selector{Languages: []string{"go", "ts"}, Paths: []string{"internal/**"}}
		if sk.Name != "s" || sk.Version != "2" || !reflect.DeepEqual(sk.Selector, want) || strings.TrimSpace(sk.Instructions) != "body" {
			t.Errorf("%s: got %+v", name, sk)
		}
	}
	if _, err := parseSkill("dir", "dir/SKILL.md", "---\nname: [unclosed\n---\nbody\n"); err == nil {
		t.Error("malformed front matter must be an error")
	}
	sk, err := parseSkill("dir", "dir/SKILL.md", "---\nversion: 1\n---\nbody\n")
	if err != nil || sk.Name != "dir" {
		t.Errorf("a skill without a name is named by its directory, got %+v, %v", sk, err)
	}
}
