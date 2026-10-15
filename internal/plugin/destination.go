package plugin

import (
	"errors"
	"strings"
)

// The destinations of a plugin's findings.
const (
	// DestinationSecurity is code scanning (the Security tab): visible only
	// to who has write access.
	DestinationSecurity = "security"
	// DestinationIssues is one issue per finding in the repository itself,
	// for a private repository without GitHub Code Security: only its
	// collaborators see it.
	DestinationIssues = "issues"
)

// ErrPublicIssues refuses issues on a public repository: an issue there is
// readable by anyone, so a finding would be published.
var ErrPublicIssues = errors.New("repositório público: achado nunca vira issue, só alerta na aba Security")

// Destination decides where the findings go from the repository's
// visibility (the API's "public", "private" or "internal") and whether code
// scanning is available. A public repository always uses the Security tab;
// a visibility that cannot be read is treated as public.
func Destination(visibility string, codeScanning bool) string {
	v := strings.ToLower(strings.TrimSpace(visibility))
	if (v == "private" || v == "internal") && !codeScanning {
		return DestinationIssues
	}
	return DestinationSecurity
}

// CheckDestination refuses a requested destination the visibility forbids.
func CheckDestination(visibility, destination string) error {
	v := strings.ToLower(strings.TrimSpace(visibility))
	if destination == DestinationIssues && v != "private" && v != "internal" {
		return ErrPublicIssues
	}
	if destination != DestinationIssues && destination != DestinationSecurity {
		return errors.New("destino desconhecido: " + destination)
	}
	return nil
}
