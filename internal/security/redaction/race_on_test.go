//go:build race

package redaction

// raceDetector reports whether the tests run under -race, which slows the
// filter by an order of magnitude: the linear-time test widens its absolute
// budget there and keeps the growth-ratio check that tells linear from
// quadratic.
const raceDetector = true
