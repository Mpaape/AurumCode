package main

// A policy bigger than any ceiling an earlier version carried must never
// abort a review (issue #179): the rule list and the text of the skills
// enter the prompt whole, so the model can cite every section. Every
// assertion reads the prompt the model actually received.

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)

// largeSkill renders a SKILL.md of sections sections, each with a long title
// and a body of about bodyBytes bytes, and returns the rule id of each one
// (<skill>#<slug of the title>) with the last section's title.
func largeSkill(name string, sections, bodyBytes int) (content string, ids []string, lastTitle string) {
	var b strings.Builder
	b.WriteString("---\nname: " + name + "\nversion: 1\nlanguages: [typescript]\n---\n")
	sentence := "A value of this kind never leaves the process boundary unreviewed. "
	body := strings.Repeat(sentence, bodyBytes/len(sentence)+1)[:bodyBytes]
	for i := 1; i <= sections; i++ {
		lastTitle = fmt.Sprintf("REG-%03d A value of this kind never goes back to the agent without review", i)
		fmt.Fprintf(&b, "\n## %s\n\n%s\n", lastTitle, body)
		slug := strings.Trim(nonAlphanumeric.ReplaceAllString(strings.ToLower(lastTitle), "-"), "-")
		ids = append(ids, name+"#"+slug)
	}
	return b.String(), ids, lastTitle
}

// TestLargePolicyNeverAbortsTheReview: three skills of 40 sections each, long
// titles and 520-byte bodies (about 120 rule ids and 70 KB of skill text) are
// a normal corporate policy. The review exits 0 and the prompt carries every
// rule id and the whole text of every skill.
func TestLargePolicyNeverAbortsTheReview(t *testing.T) {
	root := aur565Repo(t, "app.ts")
	var wantIDs, lastTitles []string
	for _, name := range []string{"seguranca", "entrega", "time"} {
		content, ids, last := largeSkill(name, 40, 520)
		aur565WriteSkill(t, root, name, content)
		wantIDs = append(wantIDs, ids...)
		lastTitles = append(lastTitles, last)
	}
	prompt, _, _ := aur565Review(t)
	for _, id := range wantIDs {
		if !strings.Contains(prompt, "`"+id+"`") {
			t.Fatalf("rule id %q is missing from the rule list the model received: it could not cite that section", id)
		}
	}
	for _, title := range lastTitles {
		if !strings.Contains(prompt, "## "+title) {
			t.Fatalf("the skill text was cut: the section %q never reached the model", title)
		}
	}
}
