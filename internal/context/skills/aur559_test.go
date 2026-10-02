package skills

import (
	"context"
	"strings"
	"testing"
)

// AUR-559 AC-004: an alias resolves through the data catalog; an unknown one
// is declared in the selection result and in the provider text, never silently
// ignored.
func TestAUR559AliasResolvesAndUnknownIsDeclared(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "a", "---\nname: a\nlanguages: [golang]\n---\nA body.\n")
	writeSkill(t, root, "b", "---\nname: b\nlanguages: [klingon]\n---\nB body.\n")
	set, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	sel := set.SelectWith([]string{"svc.go"}, DefaultLanguages())
	if len(sel.Skills) != 1 || sel.Skills[0].Name != "a" {
		t.Fatalf("alias golang must select a: %+v", sel.Skills)
	}
	if len(sel.Warnings) != 1 || !strings.Contains(sel.Warnings[0], "klingon") {
		t.Fatalf("unknown language must be declared: %v", sel.Warnings)
	}
	text, err := NewProvider(root).Provide(context.Background(), []string{"svc.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "klingon") || !strings.Contains(text, "A body.") {
		t.Fatalf("provider text must carry the warning and the skill: %q", text)
	}
}
