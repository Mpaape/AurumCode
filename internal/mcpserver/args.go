package mcpserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
)

var (
	refRe       = regexp.MustCompile(refPattern)
	pathRe      = regexp.MustCompile(pathPattern)
	findingIDRe = regexp.MustCompile(findingIDPattern)
)

// baseArgs are the arguments of aurum_review and aurum_gate.
type baseArgs struct {
	Base string `json:"base"`
}

// rulesArgs are the arguments of aurum_rules.
type rulesArgs struct {
	Paths []string `json:"paths"`
}

// explainArgs are the arguments of aurum_explain.
type explainArgs struct {
	FindingID string `json:"finding_id"`
}

// decodeStrict decodes raw into dst refusing any property the schema does
// not declare: a client cannot smuggle a policy path or a rule switch in.
func decodeStrict(raw json.RawMessage, dst any) *rpcError {
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = json.RawMessage("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return invalidParams(fmt.Sprintf("invalid arguments: %v", err))
	}
	if dec.More() {
		return invalidParams("invalid arguments: trailing data")
	}
	return nil
}

func parseBaseArgs(raw json.RawMessage) (baseArgs, *rpcError) {
	var a baseArgs
	if err := decodeStrict(raw, &a); err != nil {
		return a, err
	}
	if !refRe.MatchString(a.Base) || strings.Contains(a.Base, "..") {
		return a, invalidParams("invalid arguments: base must be a git ref matching " + refPattern + " without \"..\"")
	}
	return a, nil
}

func parseRulesArgs(raw json.RawMessage) (rulesArgs, *rpcError) {
	var a rulesArgs
	if err := decodeStrict(raw, &a); err != nil {
		return a, err
	}
	if len(a.Paths) == 0 || len(a.Paths) > maxPaths {
		return a, invalidParams(fmt.Sprintf("invalid arguments: paths must list 1 to %d entries", maxPaths))
	}
	for _, p := range a.Paths {
		clean := path.Clean(p)
		if !pathRe.MatchString(p) || clean == ".." || strings.HasPrefix(clean, "../") {
			return a, invalidParams("invalid arguments: each path must be repository relative and match " + pathPattern)
		}
	}
	return a, nil
}

func parseExplainArgs(raw json.RawMessage) (explainArgs, *rpcError) {
	var a explainArgs
	if err := decodeStrict(raw, &a); err != nil {
		return a, err
	}
	if !findingIDRe.MatchString(a.FindingID) {
		return a, invalidParams("invalid arguments: finding_id must match " + findingIDPattern)
	}
	return a, nil
}
