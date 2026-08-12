package goproject

import "fmt"

// Greeting represents a salutation composed of a recipient name and a
// two-letter language tag.
type Greeting struct {
	// Name is who the greeting addresses.
	Name string
	// Lang is a BCP-47-ish language tag, e.g. "en" or "pt".
	Lang string
}

// NewGreeting builds a Greeting for name in the given language.
func NewGreeting(name, lang string) *Greeting {
	return &Greeting{Name: name, Lang: lang}
}

// String renders the greeting as a human-readable sentence.
func (g *Greeting) String() string {
	switch g.Lang {
	case "pt":
		return fmt.Sprintf("Ola, %s!", g.Name)
	default:
		return fmt.Sprintf("Hello, %s!", g.Name)
	}
}
