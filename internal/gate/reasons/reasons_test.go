package reasons_test

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/artifacts"
	"github.com/Mpaape/AurumCode/internal/dependencies"
	"github.com/Mpaape/AurumCode/internal/dtrack"
	"github.com/Mpaape/AurumCode/internal/gate"
	"github.com/Mpaape/AurumCode/internal/gate/reasons"
)

// aur607Codes is every inconclusive reason code the gate can record: the
// model and coverage reasons, each scanner category's three failures, the
// analysis data, Dependency-Track, dependency check and artifact reasons.
func aur607Codes() []string {
	codes := []string{
		string(gate.ReasonProviderFailure), string(gate.ReasonQualitySkipped), string(gate.ReasonModelParseFailure),
		string(gate.ReasonDegradedParse), string(gate.ReasonPartialCoverage), string(gate.ReasonDeliberationLimit),
		gate.ReasonAuditWriteFailed, gate.ReasonSARIFWriteFailed, "sbom_generation_failure",
		gate.ReasonDTrackSecretMissing, gate.ReasonDTrackSBOMUnavailable, gate.ReasonDTrackInvalidHost,
		dtrack.ReasonUnreachable, dtrack.ReasonHTTPError, dtrack.ReasonTimeout, dtrack.ReasonMetricsIncomplete, dtrack.ReasonMetricsUnsettled,
		artifacts.ReasonUnavailable, artifacts.ReasonStale, artifacts.ReasonDigestMismatch, artifacts.ReasonInvalid,
		dependencies.ReasonNoModel, dependencies.ReasonModelFailed, dependencies.ReasonSourceFailed, dependencies.ReasonSourceStale,
		dependencies.ReasonScannerMissing, dependencies.ReasonUnresolved, dependencies.ReasonLicenseUnknown, dependencies.ReasonMetadataFailed,
		dependencies.ReasonCheckoutBlocked, dependencies.ReasonManifestsOmitted, dependencies.ReasonNotRun, dependencies.ReasonExtractionGap,
		dependencies.ReasonScannerFailed, dependencies.ReasonUnvetted,
	}
	for _, category := range []string{"sast", "secrets", "lint"} {
		for _, failure := range []string{"_unavailable", "_execution_error", "_invalid_output"} {
			codes = append(codes, category+failure)
		}
	}
	return append(codes, "deliberation_limit:max_rounds", "contributor_error:scanners")
}

// AC-002 (MUT-002): every reason code is a Portuguese sentence followed by
// the code in brackets, and the code itself in English.
func TestAUR607InconclusiveReasonIsASentenceWithItsCode(t *testing.T) {
	for _, code := range aur607Codes() {
		if got := reasons.Text("en-US", code); got != code {
			t.Errorf("en %s = %q, want the code itself", code, got)
		}
		pt := reasons.Text("pt-BR", code)
		if !strings.HasSuffix(pt, " ["+code+"]") || strings.HasPrefix(pt, "motivo sem descrição") {
			t.Errorf("pt-BR %s = %q, want a sentence ending in [%s]", code, pt, code)
		}
	}
	for code, want := range map[string]string{
		"quality_skipped":     "revisão por modelo não executada (sem provedor configurado) [quality_skipped]",
		"provider_failure":    "o provedor de modelo não respondeu [provider_failure]",
		"partial_coverage":    "parte do diff ficou fora da revisão [partial_coverage]",
		"sast_unavailable":    "análise estática habilitada, mas o Semgrep não está instalado [sast_unavailable]",
		"secrets_unavailable": "varredura de segredos habilitada, mas o Gitleaks não está instalado [secrets_unavailable]",
		"some_new_code":       "motivo sem descrição no catálogo [some_new_code]",
	} {
		if got := reasons.Text("pt-BR", code); got != want {
			t.Errorf("pt-BR %s = %q, want %q", code, got, want)
		}
	}
	if got := reasons.Text("en-US", "some_new_code"); got != "some_new_code" {
		t.Errorf("en unknown code = %q", got)
	}
}

// AC-002: the gate's inconclusive line keeps its English bytes and reads as
// sentences in Portuguese.
func TestAUR607InconclusiveLineBothLanguages(t *testing.T) {
	for _, tc := range []struct{ language, list, want string }{
		{"en-US", "partial_coverage", "review inconclusive (partial_coverage)"},
		{"", "sast_execution_error,partial_coverage", "review inconclusive (sast_execution_error,partial_coverage)"},
		{"pt-BR", "partial_coverage", "revisão inconclusiva — parte do diff ficou fora da revisão [partial_coverage]"},
		{"pt-BR", "sast_execution_error,partial_coverage", "revisão inconclusiva — o Semgrep falhou ao rodar [sast_execution_error]; parte do diff ficou fora da revisão [partial_coverage]"},
	} {
		if got := reasons.Line(tc.language, tc.list); got != tc.want {
			t.Errorf("Line(%q, %q) = %q, want %q", tc.language, tc.list, got, tc.want)
		}
	}
	if got := reasons.List("en-US", "a,b"); got != "a, b" {
		t.Errorf("en List = %q", got)
	}
}
