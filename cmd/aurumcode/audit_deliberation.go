package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Mpaape/AurumCode/internal/deliberation"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// auditDeliberationKey is the audit record's field for the deliberation
// transcript. It is written only when the model was offered tools, so
// every other audit keeps its bytes.
const auditDeliberationKey = "deliberation"

// appendAuditDeliberation adds the transcript to the audit record already
// written at path, as its last field, and passes the whole text through the
// redaction filter again, like every other audit field.
func appendAuditDeliberation(path string, t *deliberation.Transcript, filter *redaction.Filter) error {
	if t == nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	body := bytes.TrimRight(data, "\n")
	if !bytes.HasSuffix(body, []byte("}")) {
		return fmt.Errorf("audit record %s does not end with an object", path)
	}
	transcript, err := json.MarshalIndent(t, "  ", "  ")
	if err != nil {
		return err
	}
	var out bytes.Buffer
	out.Write(bytes.TrimRight(body[:len(body)-1], "\n "))
	fmt.Fprintf(&out, ",\n  %q: %s\n}", auditDeliberationKey, transcript)
	redacted := out.String()
	if filter != nil {
		redacted = filter.Redact(redacted)
	}
	return os.WriteFile(path, []byte(redacted+"\n"), 0o600)
}
