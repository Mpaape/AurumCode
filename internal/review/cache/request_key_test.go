package cache

import "testing"

// A request key changes when any one of its three digests changes, and
// only then.
func TestRequestKeyCoversEveryDigest(t *testing.T) {
	base := RequestKeyInput{PromptDigest: "p", EvidenceDigest: "e", ToolResultsDigest: "t"}
	k := RequestKey(base)
	if k != RequestKey(base) {
		t.Fatal("equal inputs produced different keys")
	}
	for name, in := range map[string]RequestKeyInput{
		"prompt":   {PromptDigest: "p2", EvidenceDigest: "e", ToolResultsDigest: "t"},
		"evidence": {PromptDigest: "p", EvidenceDigest: "e2", ToolResultsDigest: "t"},
		"tools":    {PromptDigest: "p", EvidenceDigest: "e", ToolResultsDigest: "t2"},
	} {
		if RequestKey(in) == k {
			t.Errorf("a different %s digest shared the key", name)
		}
	}
}
