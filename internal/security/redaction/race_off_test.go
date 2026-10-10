//go:build !race

package redaction

// raceDetector reports whether the tests run under -race; see race_on_test.go.
const raceDetector = false
