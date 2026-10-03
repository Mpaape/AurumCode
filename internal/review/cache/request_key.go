package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// RequestKeyInput is everything that can change a model's answer to one
// review request, reduced to digests. The final prompt text already carries
// every rendered slot; the evidence and tool-result digests are folded in
// separately because they also cover what the prompt does NOT show -- an
// evidence item omitted by its slot budget, a tool result the engine
// consumed -- and a cached answer must never be reused across a different
// set of either.
type RequestKeyInput struct {
	PromptDigest      string // digest of the exact system and user text sent
	EvidenceDigest    string // DigestOf(the evidence items offered)
	ToolResultsDigest string // DigestOf(the tool results the engine consumed)
}

// requestKeyVersion names the layout of RequestKey's hashed input, so a
// change to that layout can never collide with keys written under the old
// one.
const requestKeyVersion = "review-request-key/1"

// RequestKey returns the content-addressed cache key of one review
// request. It replaces a positional concatenation of selected inputs: any
// input that reaches the model reaches the key through one of the three
// digests.
func RequestKey(in RequestKeyInput) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%s", requestKeyVersion, in.PromptDigest, in.EvidenceDigest, in.ToolResultsDigest)))
	return hex.EncodeToString(sum[:])
}

// DigestOf returns the sha256 of v's canonical JSON encoding (struct
// fields in declaration order, map keys sorted). It is how a caller turns
// evidence items or tool results into RequestKeyInput digests.
func DigestOf(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("digesting cache key input: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
