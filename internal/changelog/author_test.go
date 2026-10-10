package changelog

import "testing"

// AUR-610 AC-005: only the event's account type or the reserved [bot]
// login suffix marks a bot; a login that merely mentions bot is a person.
func TestAUR610AC005AuthorIsBot(t *testing.T) {
	cases := []struct {
		name   string
		author Author
		want   bool
	}{
		{"type Bot", Author{Type: "Bot"}, true},
		{"dependabot login", Author{Login: "dependabot[bot]"}, true},
		{"renovate login typed User", Author{Login: "renovate[bot]", Type: "User"}, true},
		{"person", Author{Login: "paape"}, false},
		{"person typed User", Author{Login: "paape", Type: "User"}, false},
		{"bot-lover", Author{Login: "bot-lover"}, false},
		{"suffix in the middle", Author{Login: "x[bot]y"}, false},
		{"absent", Author{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.author.IsBot(); got != c.want {
				t.Fatalf("%+v.IsBot() = %v, want %v", c.author, got, c.want)
			}
		})
	}
}

// AUR-610: the login echoed in the log line cannot carry a line break or a
// workflow command, and an absent login is named.
func TestAUR610AuthorLabelIsOneBoundedToken(t *testing.T) {
	if got := (Author{Login: "dependabot[bot]"}).Label(); got != "dependabot[bot]" {
		t.Fatalf("Label = %q", got)
	}
	if got := (Author{Login: "x\n::error::forjado"}).Label(); got != "x::error::forjado" {
		t.Fatalf("control characters survived: %q", got)
	}
	if got := (Author{}).Label(); got != "login ausente" {
		t.Fatalf("absent login: %q", got)
	}
	long := (Author{Login: "a123456789b123456789c123456789d123456789e123456789f123456789g123456789"}).Label()
	if len([]rune(long)) != maxAuthorLabel {
		t.Fatalf("label not bounded: %d runes", len([]rune(long)))
	}
}
