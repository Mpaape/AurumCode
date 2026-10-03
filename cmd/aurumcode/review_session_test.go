package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/review/session"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// phaseStepNames are the steps a source maps the session's phases onto.
// A slice literal holding two of them is a second phase list.
var phaseStepNames = map[string]bool{
	"resolve": true, "resolveInputs": true, "validate": true, "analyze": true,
	"runModelPass": true, "collectEvidence": true, "decideGate": true,
	"runGate": true, "publish": true,
}

// exitCodeNames are the run's exit codes; a source method returning one of
// them is a second exit ladder.
var exitCodeNames = map[string]bool{
	"exitFindings": true, "exitQualityNotReviewed": true, "exitArtifactNotWritten": true,
}

// sourceReceivers are the review session's sources and their shared state.
var sourceReceivers = map[string]bool{"baseReview": true, "prReview": true, "reviewState": true}

// parseProductionFiles parses every non-test file of this package.
func parseProductionFiles(t *testing.T) []*ast.File {
	t.Helper()
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	return files
}

// receiverName is the type name of fn's receiver, "" for a function.
func receiverName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	typ := fn.Recv.List[0].Type
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	if id, ok := typ.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// TestAUR576OnePhaseListAndOneExitDecision is AC-001: the phase order lives
// only in internal/review/session (no slice literal of this package lists
// two phase steps, and no session.Phase list is declared here); no method of
// a review source returns an exit code of its own; gate.ExitPolicy is
// called from exactly one function.
func TestAUR576OnePhaseListAndOneExitDecision(t *testing.T) {
	var phaseLists, ladders, policyCallers []string
	for _, f := range parseProductionFiles(t) {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			where := receiverName(fn) + "." + fn.Name.Name
			callsPolicy := false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.CompositeLit:
					if countPhaseSteps(node) >= 2 || isPhaseSlice(node) {
						phaseLists = append(phaseLists, where)
					}
				case *ast.ReturnStmt:
					if sourceReceivers[receiverName(fn)] && returnsExitCode(node) {
						ladders = append(ladders, where)
					}
				case *ast.SelectorExpr:
					if pkg, ok := node.X.(*ast.Ident); ok && pkg.Name == "gate" && node.Sel.Name == "ExitPolicy" {
						callsPolicy = true
					}
				}
				return true
			})
			if callsPolicy {
				policyCallers = append(policyCallers, where)
			}
		}
	}
	if len(phaseLists) > 0 {
		t.Errorf("phase lists declared in cmd/aurumcode (the order belongs to session.Order): %v", phaseLists)
	}
	if len(ladders) > 0 {
		t.Errorf("review sources returning their own exit codes (use gate.ExitPolicy): %v", ladders)
	}
	if len(policyCallers) != 1 {
		t.Errorf("gate.ExitPolicy must be called from exactly one function, got %v", policyCallers)
	}
}

// countPhaseSteps counts the elements of lit that name a phase step.
func countPhaseSteps(lit *ast.CompositeLit) int {
	n := 0
	for _, elt := range lit.Elts {
		if sel, ok := elt.(*ast.SelectorExpr); ok && phaseStepNames[sel.Sel.Name] {
			n++
		}
	}
	return n
}

// isPhaseSlice reports a []session.Phase literal.
func isPhaseSlice(lit *ast.CompositeLit) bool {
	arr, ok := lit.Type.(*ast.ArrayType)
	if !ok {
		return false
	}
	sel, ok := arr.Elt.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Phase"
}

// returnsExitCode reports a return statement that yields an exit code
// constant.
func returnsExitCode(ret *ast.ReturnStmt) bool {
	for _, r := range ret.Results {
		if id, ok := r.(*ast.Ident); ok && exitCodeNames[id.Name] {
			return true
		}
	}
	return false
}

// evidenceState is a reviewState for source holding the same model
// findings and security findings.
func evidenceState(source session.Source) *reviewState {
	s := newReviewState(source, reviewIO{filter: redaction.NewFilter()})
	s.cfg = &config.Config{}
	s.result = &types.ReviewResult{Issues: []types.ReviewIssue{
		{File: "app.go", Line: 3, Severity: "warning", Message: "model finding", RuleID: "quality/naming"},
	}}
	s.securityFindings = []types.ReviewIssue{
		{File: "app.go", Line: 7, Severity: "error", Message: "hardcoded secret", RuleID: "security/hardcoded-secret"},
	}
	return &s
}

// findingKeys is a sorted identity of issues.
func findingKeys(issues []types.ReviewIssue) []string {
	keys := make([]string, 0, len(issues))
	for _, i := range issues {
		keys = append(keys, i.File+":"+i.RuleID+":"+i.Message)
	}
	sort.Strings(keys)
	return keys
}

// TestAUR576SnapshotHoldsTheSameFindingsOnBothSources is AC-003: the raw
// verdict-reuse snapshot holds the same findings, the security pass's
// included, whether the source keeps them apart (--base) or publishes them
// as review comments (--pr); and the gate reads that same set too.
func TestAUR576SnapshotHoldsTheSameFindingsOnBothSources(t *testing.T) {
	local, pr := evidenceState(session.LocalDiff), evidenceState(session.PullRequest)
	for _, s := range []*reviewState{local, pr} {
		s.joinSecurityFindings()
		s.snapshotAndApplyRules()
		s.run.Review, s.run.Extra = s.result, s.securityApart()
	}
	want := []string{"app.go:quality/naming:model finding", "app.go:security/hardcoded-secret:hardcoded secret"}
	if got := findingKeys(local.rawIssues); !reflect.DeepEqual(got, want) {
		t.Errorf("--base snapshot = %v, want %v", got, want)
	}
	if got := findingKeys(pr.rawIssues); !reflect.DeepEqual(got, want) {
		t.Errorf("--pr snapshot = %v, want %v", got, want)
	}
	if a, b := findingKeys(local.run.IssuesForGate()), findingKeys(pr.run.IssuesForGate()); !reflect.DeepEqual(a, b) || !reflect.DeepEqual(a, want) {
		t.Errorf("gate findings differ: --base=%v --pr=%v, want %v", a, b, want)
	}
}
