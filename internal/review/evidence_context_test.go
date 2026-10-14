package review

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

type mapSource map[string]string

func (m mapSource) FileAt(ref, path string) ([]byte, error) {
	return []byte(m[ref+":"+path]), nil
}

// A private key committed in the reviewed range reaches the model masked
// whole: the redaction filter only recognizes the base64 body when it sees
// the block, so the snippet is redacted as a whole before it is cut, never
// line by line (which leaked the body).
func TestEvidenceSnippetMasksAMultiLinePrivateKey(t *testing.T) {
	body := []string{"MIIEowIBAAKCAQEAx1y2z3", "QWERTYUIOPASDFGHJKLZXCVBNM0123456789abcd", "zxcvbnmasdfghjklqwertyuiop9876543210ZYXW"}
	key := "-----BEGIN RSA " + "PRIVATE KEY-----\n" + strings.Join(body, "\n") + "\n-----END RSA " + "PRIVATE KEY-----"
	file := "linha um\n" + key + "\nlinha depois\n"
	filter := redaction.NewFilter()
	ctx := EvidenceContext{Source: mapSource{"abc123:deploy/id_rsa.pem": file}, Redact: filter.Redact}
	item := ctx.Describe(prompt.EvidenceItem{ID: "E1", File: "deploy/id_rsa.pem", Line: 2}, "abc123", true)
	for _, b := range body {
		if strings.Contains(item.Snippet, b) {
			t.Fatalf("the key body leaked into the triage snippet:\n%s", item.Snippet)
		}
	}
	if !strings.Contains(item.Snippet, "linha depois") || !strings.Contains(item.Snippet, ">    2|") {
		t.Fatalf("the snippet lost its numbering or the lines around the key:\n%s", item.Snippet)
	}
}

// A removed line is numbered in the old file: the wider context never shows
// the current file at that number.
func TestEvidenceSnippetOfARemovedLineIsNotTheCurrentFile(t *testing.T) {
	ctx := EvidenceContext{Source: mapSource{"HEAD:app.go": "nada a ver\n"}}
	item := ctx.Describe(prompt.EvidenceItem{ID: "E1", File: "app.go", Line: 1, Side: "LEFT", Snippet: "-old"}, "", true)
	if item.Snippet != "-old" {
		t.Fatalf("a removed line got the current file: %q", item.Snippet)
	}
}
