// Package sample is a bounded, non-sensitive fixture corpus for the
// AUR-363 Go adapter lock. It exists only to be digested, never parsed.
package sample

// Greet returns a static greeting.
func Greet(name string) string {
	return "hello, " + name
}
