package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/deliberation"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/scanner"
)

// ScannerToolPrefix starts the name of every scanner tool: scanner_<engine>.
// Tool names on the wire allow letters, digits, '_' and '-' only.
const ScannerToolPrefix = "scanner_"

// ScanFunc runs one engine the way the evidence phase runs it (same tree,
// same trust, same refusal of an unverified checkout).
type ScanFunc func(ctx context.Context) scanner.Outcome

// scannerArgs is the scanner tool's argument object: it takes none, the
// scan covers the whole reviewed tree.
type scannerArgs struct{}

// ScannerTool offers one registered engine to the model.
type ScannerTool struct {
	engine scanner.Engine
	scan   ScanFunc
}

// NewScannerTool offers engine. scan runs it and is responsible for
// joining the outcome, findings or inconclusive reason, to the session's
// scans, so the gate counts it whatever the model does with it.
func NewScannerTool(engine scanner.Engine, scan ScanFunc) *ScannerTool {
	return &ScannerTool{engine: engine, scan: scan}
}

// ScannerToolName is the tool name of engine.
func ScannerToolName(engine string) string { return ScannerToolPrefix + scanner.Normalize(engine) }

// Spec implements deliberation.Tool.
func (t *ScannerTool) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        ScannerToolName(t.engine.Name()),
		Description: fmt.Sprintf("Executa o scanner %s (categoria %s) sobre a árvore revisada e devolve seus achados; os achados contam no gate com origem %s.", t.engine.Name(), t.engine.Category, t.engine.TypedOrigin()),
		Parameters:  llm.SchemaOf(scannerArgs{}),
	}
}

// Run implements deliberation.Tool. An inconclusive scan is an error the
// model is told about; the recorded outcome makes the gate inconclusive.
func (t *ScannerTool) Run(ctx context.Context, _ json.RawMessage) (deliberation.Result, error) {
	out := t.scan(ctx)
	if out.Reason != "" {
		return deliberation.Result{}, fmt.Errorf("varredura inconclusiva (%s)", out.Reason)
	}
	return deliberation.Result{
		Content: renderFindings(t.engine, out.Findings),
		Summary: fmt.Sprintf("%d achado(s)", len(out.Findings)),
		Digest:  findingsDigest(out.Findings),
	}, nil
}

// renderFindings is the tool message of a clean scan: one line per finding.
func renderFindings(engine scanner.Engine, findings []scanner.Finding) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %d achado(s) (origem %s)\n", engine.Name(), len(findings), engine.TypedOrigin())
	for _, f := range findings {
		fmt.Fprintf(&b, "- %s %s:%d [%s] %s\n", f.RuleID, f.Path, f.Line, f.Severity, f.Message)
	}
	return b.String()
}

// findingsDigest identifies a scan's findings for the cache keys.
func findingsDigest(findings []scanner.Finding) string {
	raw, err := json.Marshal(findings)
	if err != nil {
		return "unavailable"
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
