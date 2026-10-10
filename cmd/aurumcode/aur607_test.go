package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/gate"
	"github.com/Mpaape/AurumCode/internal/gate/facts"
	"github.com/Mpaape/AurumCode/internal/gate/reasons"
	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/internal/scanner"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// aur607NoProviderEN is the no-provider teaching text of earlier releases,
// copied byte for byte: English must keep it.
const aur607NoProviderEN = `no LLM provider configured: set AURUMCODE_LLM_FIXTURE=<path> to a JSON file shaped like {"issues":[{"file":"<path>","line":<n>,"severity":"error|warning|info","rule_id":"<id from the embedded rule catalog, e.g. security/hardcoded-secret>","message":"<text>"}]} for offline use -- a finding whose rule_id is missing or unknown is discarded, never shown, so rule_id is not optional -- if you have the AurumCode source checked out, tests/fixtures/review/known-problem-response.json is a worked example -- or set LLM_API_KEY and LLM_BASE_URL for a live provider`

// aur607EnglishPhrases are the fixed English sentences a pt-BR review used
// to print on the terminal.
var aur607EnglishPhrases = []string{
	"no LLM provider configured",
	"quality review skipped",
	"running deterministic analysis only",
	"LLM quality review did not run",
	"covers deterministic analysis only",
	"gate verdict reuse unavailable",
	"review inconclusive",
	"No issues found.",
	"the quality review did not run",
	"finding(s) at severity",
}

// aur607Review runs `review --base HEAD~1` over a fixture repository whose
// configuration declares a gate and, when language is set, review.language.
// The verdict cache directory is unset, so the reuse notice prints.
func aur607Review(t *testing.T, language string, extra ...string) (stdout, stderr string, code int) {
	t.Helper()
	cfg := "gate:\n  fail_on: [error]\n"
	if language != "" {
		cfg += "review:\n  language: " + language + "\n"
	}
	cleanFixture(t, cfg)
	t.Setenv("AURUMCODE_CACHE_DIR", "")
	var out, errOut strings.Builder
	code = runReview(append([]string{"--base", "HEAD~1"}, extra...), &out, &errOut, redaction.NewFilter())
	return out.String(), errOut.String(), code
}

// AC-001 (MUT-001): without a provider, a pt-BR review prints no fixed
// English sentence on stdout or stderr; English prints exactly what it
// printed before; the exit code is the same in both.
func TestAUR607BaseReviewSpeaksTheReviewLanguage(t *testing.T) {
	enOut, enErr, enCode := aur607Review(t, "")
	ptOut, ptErr, ptCode := aur607Review(t, "pt-BR")
	if enCode != ptCode {
		t.Fatalf("the language changed the exit code: en=%d pt-BR=%d", enCode, ptCode)
	}
	for _, want := range []string{
		"aurumcode review: no LLM provider configured: quality review skipped; running deterministic analysis only\n",
		"aurumcode review: " + aur607NoProviderEN + "\n",
		"aurumcode review: gate verdict reuse unavailable (AURUMCODE_CACHE_DIR not set): this run's verdict cannot be shared with another run, and could not reuse one either\n",
		"aurumcode review: policy gate: review inconclusive (quality_skipped)\n",
	} {
		if !strings.Contains(enErr, want) {
			t.Errorf("en stderr lacks the earlier line %q:\n%s", want, enErr)
		}
	}
	if !strings.HasPrefix(enOut, "LLM quality review did not run. The following report covers deterministic analysis only.\n") {
		t.Errorf("en stdout lacks the earlier first line:\n%s", enOut)
	}
	for _, phrase := range aur607EnglishPhrases {
		if strings.Contains(ptOut, phrase) || strings.Contains(ptErr, phrase) {
			t.Errorf("pt-BR output still says %q:\nstdout:\n%s\nstderr:\n%s", phrase, ptOut, ptErr)
		}
	}
	for _, want := range []string{
		"aurumcode review: nenhum provedor de modelo configurado: revisão por modelo não executada; rodando só a análise determinística\n",
		"aurumcode review: Para revisar com um modelo, defina LLM_API_KEY e LLM_BASE_URL.\n",
		"aurumcode review: reaproveitamento do veredito do gate indisponível (AURUMCODE_CACHE_DIR não definido)",
		"aurumcode review: policy gate: revisão inconclusiva — revisão por modelo não executada (sem provedor configurado) [quality_skipped]\n",
	} {
		if !strings.Contains(ptErr, want) {
			t.Errorf("pt-BR stderr lacks %q:\n%s", want, ptErr)
		}
	}
	if !strings.HasPrefix(ptOut, "A revisão por modelo não rodou. O relatório abaixo cobre só a análise determinística.\n") {
		t.Errorf("pt-BR stdout lacks its first line:\n%s", ptOut)
	}
}

// AC-001/AC-002: the closing line of a report without findings names each
// reason as a sentence with its code in Portuguese, the code in English.
func TestAUR607ClosingLineNamesTheReasons(t *testing.T) {
	for _, tc := range []struct{ language, want string }{
		{"en-US", "Sem achados nas fontes concluídas; inconclusivo: sast_unavailable, partial_coverage\n"},
		{"pt-BR", "Sem achados nas fontes concluídas; inconclusivo: análise estática habilitada, mas o Semgrep não está instalado [sast_unavailable]; parte do diff ficou fora da revisão [partial_coverage]\n"},
	} {
		var out bytes.Buffer
		printFindings(&out, &types.ReviewResult{}, "sast_unavailable,partial_coverage,sast_unavailable", tc.language)
		if out.String() != tc.want {
			t.Errorf("%s: got %q, want %q", tc.language, out.String(), tc.want)
		}
	}
}

// AC-001: --exigir-qualidade's line follows the review language.
func TestAUR607RequiredQualityLineFollowsTheLanguage(t *testing.T) {
	_, enErr, _ := aur607Review(t, "", "--exigir-qualidade")
	if !strings.Contains(enErr, "aurumcode review: --exigir-qualidade: the quality review did not run, so this run is not a clean review\n") {
		t.Errorf("en lost the earlier --exigir-qualidade line:\n%s", enErr)
	}
	_, ptErr, _ := aur607Review(t, "pt-BR", "--exigir-qualidade")
	if !strings.Contains(ptErr, "aurumcode review: --exigir-qualidade: a revisão por modelo não rodou, então esta execução não é uma revisão completa\n") {
		t.Errorf("pt-BR lacks the --exigir-qualidade line:\n%s", ptErr)
	}
}

// AC-001: the no-provider text is the earlier error in English and short
// lines in Portuguese.
func TestAUR607NoProviderTextIsShortLinesInPortuguese(t *testing.T) {
	if errNoProviderConfigured.Error() != aur607NoProviderEN || noProviderText("en-US") != aur607NoProviderEN {
		t.Fatalf("English no-provider text changed:\n%s", noProviderText("en-US"))
	}
	lines := strings.Split(noProviderText("pt-BR"), "\n")
	if len(lines) < 5 {
		t.Fatalf("pt-BR no-provider text is not broken into lines: %q", lines)
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "{") && utf8.RuneCountInString(line) > 100 {
			t.Errorf("pt-BR line longer than 100 characters: %q", line)
		}
	}
	var out bytes.Buffer
	printLines(&out, "p: ", "a\nb")
	if out.String() != "p: a\np: b\n" {
		t.Errorf("printLines = %q", out.String())
	}
}

// AC-001: the --fail-on closing line counts in the review language.
func TestAUR607FailOnLineCountsInTheLanguage(t *testing.T) {
	for _, tc := range []struct {
		language string
		n        int
		want     string
	}{
		{"en-US", 1, "1 finding(s) at severity error or above (--fail-on error)"},
		{"en-US", 3, "3 finding(s) at severity error or above (--fail-on error)"},
		{"pt-BR", 1, "1 achado com severidade error ou maior (--fail-on error)"},
		{"pt-BR", 3, "3 achados com severidade error ou maior (--fail-on error)"},
	} {
		if got := failOnLine(tc.language, tc.n, "error"); got != tc.want {
			t.Errorf("failOnLine(%s, %d) = %q, want %q", tc.language, tc.n, got, tc.want)
		}
	}
}

// AC-001: the remaining terminal texts keep their English bytes and have a
// Portuguese text of their own.
func TestAUR607TerminalTextsBothLanguages(t *testing.T) {
	for _, tc := range []struct{ key, en, pt string }{
		{"terminal.batches", "the diff did not fit one prompt; reviewed in 2 batches by directory (files per batch: 3, 4)", "o diff não coube em um só prompt; revisado em 2 lotes por diretório (arquivos por lote: 3, 4)"},
		{"terminal.model_unparsed", "could not understand the model's response (truncated)", "não foi possível entender a resposta do modelo (truncated)"},
		{"terminal.model_degraded", "degrading to deterministic analysis; the model review is inconclusive", "voltando à análise determinística; a revisão por modelo ficou inconclusiva"},
		{"terminal.no_issues", "No issues found.", "Nenhum problema encontrado."},
	} {
		args := map[string][]any{"terminal.batches": {2, "3, 4"}, "terminal.model_unparsed": {"truncated"}}[tc.key]
		if got := i18n.Format("en-US", tc.key, args...); got != tc.en {
			t.Errorf("en %s = %q, want %q", tc.key, got, tc.en)
		}
		if got := i18n.Format("pt-BR", tc.key, args...); got != tc.pt {
			t.Errorf("pt-BR %s = %q, want %q", tc.key, got, tc.pt)
		}
	}
	var clean bytes.Buffer
	printFindings(&clean, &types.ReviewResult{}, "", "pt-BR")
	if clean.String() != "Nenhum problema encontrado.\n" {
		t.Errorf("pt-BR clean review printed %q", clean.String())
	}
}

// AC-001: a skipped binary or generated file is announced in the review
// language; English keeps the analyzer's message.
func TestAUR607SkippedFileNoticeFollowsTheLanguage(t *testing.T) {
	notices := []analyzer.DiffNotice{
		{Path: "logo.png", Reason: analyzer.NoticeReasonBinary, Message: "binary file, skipped: logo.png"},
		{Path: "gerado.go", Reason: "generated", Message: "generated file, skipped: gerado.go"},
		{Path: "big.txt", Reason: "too large", Message: "diff too large: big.txt (9 lines > limit 8)"},
	}
	var en, pt bytes.Buffer
	printNotices(&en, redaction.NewFilter(), notices, "en-US")
	printNotices(&pt, redaction.NewFilter(), notices, "pt-BR")
	if want := "binary file, skipped: logo.png\ngenerated file, skipped: gerado.go\ndiff too large: big.txt (9 lines > limit 8)\n"; en.String() != want {
		t.Errorf("en notices = %q, want %q", en.String(), want)
	}
	if want := "arquivo binário, não revisado: logo.png\narquivo gerado, não revisado: gerado.go\ndiff too large: big.txt (9 lines > limit 8)\n"; pt.String() != want {
		t.Errorf("pt-BR notices = %q, want %q", pt.String(), want)
	}
}

// AC-001: the publication lines of the pull request name the decision and
// count the line comments without "(s)" in Portuguese.
func TestAUR607PublicationLinesFollowTheLanguage(t *testing.T) {
	for _, tc := range []struct {
		language, event string
		comments        int
		want            string
	}{
		{"en-US", "APPROVE", 0, `review formal "APPROVE" publicado no pull request #7 (0 comentário(s) na linha).`},
		{"pt-BR", "APPROVE", 0, "parecer publicado na PR #7 como aprovação (sem comentários na linha)."},
		{"pt-BR", "REQUEST_CHANGES", 2, "parecer publicado na PR #7 como pedido de alteração (2 comentários na linha)."},
		{"pt-BR", "COMMENT", 1, "parecer publicado na PR #7 como comentário (1 comentário na linha)."},
	} {
		if got := formalReviewLine(tc.language, tc.event, 7, tc.comments); got != tc.want {
			t.Errorf("formalReviewLine(%s, %s, %d) = %q, want %q", tc.language, tc.event, tc.comments, got, tc.want)
		}
	}
	if got, want := parecerLine("en-US", "publicado", 7, 1), "parecer publicado no pull request #7 (1 comentário(s) na linha)."; got != want {
		t.Errorf("en parecerLine = %q, want %q", got, want)
	}
	if got, want := parecerLine("pt-BR", "atualizado", 7, 0), "parecer atualizado na PR #7 (sem comentários na linha)."; got != want {
		t.Errorf("pt-BR parecerLine = %q, want %q", got, want)
	}
}

// aur607Breach is a gate decision failed by one finding.
func aur607Breach(title string, inconclusive bool) gateDecision {
	return gateDecision{
		Active: true, Fail: true, Breach: true, Inconclusive: inconclusive, Reason: map[bool]string{true: "partial_coverage"}[inconclusive],
		Lines:            []string{"seguranca#sem-segredos - " + title + " (severidade error, limiar error, origem skills)"},
		BlockingFindings: []facts.AuditFinding{{RuleID: "seguranca#sem-segredos", Path: "config.py", Line: 1, Severity: "error"}},
		FirstBreach:      &gate.Breach{Title: title, Severity: "error", Path: "config.py", Line: 1},
	}
}

// AC-002/AC-003: in Portuguese the status names the finding that failed the
// gate, never repeats the pull request number, gives each inconclusive
// reason as a sentence with its code, and cuts at a word.
func TestAUR607PolicyGateStatusInPortuguese(t *testing.T) {
	for _, tc := range []struct {
		name  string
		g     gateDecision
		state string
		want  string
	}{
		{"breach", aur607Breach("Sem segredos no codigo", false), "failure", "falha: 1 achado reprova o gate — Sem segredos no codigo (error) em config.py:1; detalhes no parecer"},
		{"breach and inconclusive", aur607Breach("Sem segredos no codigo", true), "failure", "falha: 1 achado reprova o gate — Sem segredos no codigo (error) em config.py:1; revisão também inconclusiva; detalhes no parecer"},
		{"block", gateDecision{Active: true, Fail: true, Inconclusive: true, Reason: "partial_coverage", Lines: []string{"revisão inconclusiva — parte do diff ficou fora da revisão [partial_coverage]"}}, "failure", "inconclusivo: a revisão não cobriu tudo e o gate reprova por configuração — parte do diff ficou fora da revisão [partial_coverage]"},
		{"warn", gateDecision{Active: true, Inconclusive: true, Reason: "provider_failure"}, "success", "inconclusivo: a revisão não cobriu tudo e o gate só avisa por configuração — o provedor de modelo não respondeu [provider_failure]"},
		{"long reasons keep their codes", gateDecision{Active: true, Inconclusive: true, Reason: "quality_skipped,partial_coverage"}, "success", "inconclusivo: a revisão não cobriu tudo e o gate só avisa por configuração — [quality_skipped]; [partial_coverage]"},
		{"approved", gateDecision{Active: true}, "success", "aprovado: gate de política aprovado"},
	} {
		state, got := gateStatusDescription("pt-BR", tc.g, 7)
		if state != tc.state || got != tc.want {
			t.Errorf("%s: got (%s, %q), want (%s, %q)", tc.name, state, got, tc.state, tc.want)
		}
		if strings.Contains(got, "pull request #") || strings.Contains(got, "#7") {
			t.Errorf("%s: description repeats the pull request number: %q", tc.name, got)
		}
	}
	two := aur607Breach("Sem segredos", false)
	two.BlockingFindings = append(two.BlockingFindings, two.BlockingFindings[0])
	if _, got := gateStatusDescription("pt-BR", two, 7); !strings.HasPrefix(got, "falha: 2 achados reprovam o gate — ") {
		t.Errorf("two findings: %q", got)
	}
}

// AC-003: a long title gives way to the cap at a word boundary, and the
// place and the pointer to the review survive it.
func TestAUR607PolicyGateStatusCutsAtAWord(t *testing.T) {
	title := strings.Repeat("palavra longa ", 20) + "fim"
	_, got := gateStatusDescription("pt-BR", aur607Breach(title, true), 7)
	if utf8.RuneCountInString(got) > statusDescriptionLimit {
		t.Fatalf("description has %d runes > %d: %q", utf8.RuneCountInString(got), statusDescriptionLimit, got)
	}
	if !strings.HasSuffix(got, " (error) em config.py:1; revisão também inconclusiva; detalhes no parecer") {
		t.Fatalf("the place or the pointer was cut: %q", got)
	}
	kept := strings.TrimPrefix(got[:strings.Index(got, "…")], "falha: 1 achado reprova o gate — ")
	if !strings.HasPrefix(title, kept) || title[len(kept)] != ' ' {
		t.Fatalf("title cut inside a word: kept %q of %q", kept, title)
	}
	for _, tc := range []struct {
		in    string
		limit int
		want  string
	}{
		{"aaa bbb ccc", 8, "aaa bbb…"},
		{"aaa bbbbbbb", 8, "aaa…"},
		{"aaaaaaaaaa", 5, "aaaa…"},
		{"curto", 10, "curto"},
	} {
		if got := capAtWord(tc.in, tc.limit); got != tc.want {
			t.Errorf("capAtWord(%q, %d) = %q, want %q", tc.in, tc.limit, got, tc.want)
		}
	}
}

// AC-002/AC-003: English keeps the description of earlier releases byte for
// byte, mid-word cut included.
func TestAUR607PolicyGateStatusEnglishUnchanged(t *testing.T) {
	for _, tc := range []struct {
		g    gateDecision
		want string
	}{
		{gateDecision{Active: true}, "aprovado: gate de política aprovado no pull request #7"},
		{gateDecision{Active: true, Fail: true, Inconclusive: true, Reason: "partial_coverage", Lines: []string{"review inconclusive (partial_coverage)"}}, "inconclusivo: revisão inconclusiva (bloqueio) no pull request #7: review inconclusive (partial_coverage)"},
		{gateDecision{Active: true, Inconclusive: true, Lines: []string{"review inconclusive (quality_skipped)"}}, "inconclusivo: revisão inconclusiva (alerta) no pull request #7: review inconclusive (quality_skipped)"},
		{aur607Breach("Sem segredos no codigo", false), "falha: achado(s) reprovam o gate no pull request #7: seguranca#sem-segredos - Sem segredos no codigo (severidade error, limiar error, orige…"},
	} {
		for _, language := range []string{"", "en-US"} {
			if _, got := gateStatusDescription(language, tc.g, 7); got != tc.want {
				t.Errorf("%q: got %q, want %q", language, got, tc.want)
			}
		}
	}
}

// AC-004: a Gitleaks finding leads with the catalog's label in Portuguese
// and keeps the tool's own text as the detail; English is unchanged.
func TestAUR607GitleaksFindingLeadsWithTheCatalogLabel(t *testing.T) {
	finding := scanner.Finding{
		Path: "config.py", Line: 1, RuleID: "gitleaks:github-pat", Severity: "error",
		Message: "Uncovered a GitHub Personal Access Token, potentially leading to unauthorized repository access and sensitive content exposure. in commit 37eb089803ee (gitleaks v8.30.1)",
	}
	en, enTools := (&reviewState{reviewLanguage: "en-US"}).scannerIssues([]scanner.Finding{finding}, "gitleaks", "secrets", "gitleaks")
	if want := finding.Message + " (rule gitleaks:github-pat)"; en[0].Message != want {
		t.Errorf("en message = %q, want %q", en[0].Message, want)
	}
	pt, ptTools := (&reviewState{reviewLanguage: "pt-BR"}).scannerIssues([]scanner.Finding{finding}, "gitleaks", "secrets", "gitleaks")
	want := "Segredo ou credencial escrito no código (regra `gitleaks:github-pat`). Texto original do Gitleaks: Uncovered a GitHub Personal Access Token, potentially leading to unauthorized repository access and sensitive content exposure (commit 37eb089803ee, gitleaks v8.30.1)"
	if pt[0].Message != want {
		t.Errorf("pt-BR message = %q, want %q", pt[0].Message, want)
	}
	if enTools != nil {
		t.Errorf("en kept tool messages for an unchanged message: %v", enTools)
	}
	if got := ptTools[findingOriginKey("gitleaks:github-pat", "config.py", 1)]; got != finding.Message+" (rule gitleaks:github-pat)" {
		t.Errorf("pt-BR lost the engine's own message for the machine artifacts: %q", got)
	}
	if strings.Contains(pt[0].Message, ". in commit") || pt[0].RuleID != finding.RuleID || pt[0].File != "config.py" || pt[0].Line != 1 || pt[0].Severity != "error" {
		t.Errorf("pt-BR finding changed more than its text: %+v", pt[0])
	}
	sast, _ := (&reviewState{reviewLanguage: "pt-BR"}).scannerIssues([]scanner.Finding{{Path: "a.go", Line: 1, RuleID: "semgrep:x", Message: "m"}}, "sast", "sast", "semgrep")
	if sast[0].Message != "m (rule semgrep:x)" {
		t.Errorf("a non-secret finding changed: %q", sast[0].Message)
	}
}

// AC-001: --pr without a provider teaches in the review language too, and
// English prints the earlier error byte for byte.
func TestAUR607PullRequestWithoutProviderSpeaksTheLanguage(t *testing.T) {
	cleanFixture(t, "")
	for _, tc := range []struct{ language, want string }{
		{"en-US", "aurumcode review: " + aur607NoProviderEN + "\n"},
		{"pt-BR", "aurumcode review: nenhum provedor de modelo configurado.\naurumcode review: Para revisar com um modelo, defina LLM_API_KEY e LLM_BASE_URL.\n"},
	} {
		var stderr bytes.Buffer
		p := &prReview{}
		p.stderr, p.reviewLanguage = &stderr, tc.language
		if code, done := p.selectProvider(); code != 1 || !done {
			t.Fatalf("%s: selectProvider = %d, %v", tc.language, code, done)
		}
		if !strings.HasPrefix(stderr.String(), tc.want) {
			t.Errorf("%s stderr = %q, want prefix %q", tc.language, stderr.String(), tc.want)
		}
	}
}

// AC-001: the aurumcode/review status and the cache line count in the
// review language; English keeps the earlier descriptions byte for byte.
func TestAUR607ReviewCheckAndCacheLinesFollowTheLanguage(t *testing.T) {
	for _, tc := range []struct {
		language                        string
		grave                           int
		qualityIncomplete, providerFail bool
		want                            string
	}{
		{"en-US", 0, false, false, "nenhum achado grave no pull request #7"},
		{"en-US", 2, false, false, "2 achado(s) grave(s) no pull request #7"},
		{"en-US", 0, false, true, "revisão não executada no pull request #7: falha do provedor (provider_failure)"},
		{"en-US", 0, true, false, "revisão por modelo inconclusiva no pull request #7"},
		{"pt-BR", 0, false, false, "nenhum achado grave"},
		{"pt-BR", 1, false, false, "1 achado grave"},
		{"pt-BR", 2, false, false, "2 achados graves"},
		{"pt-BR", 0, false, true, "revisão não executada: o provedor de modelo não respondeu [provider_failure]"},
	} {
		if got := reviewCheckDescription(tc.language, 7, tc.grave, tc.qualityIncomplete, tc.providerFail); got != tc.want {
			t.Errorf("%s grave=%d: got %q, want %q", tc.language, tc.grave, got, tc.want)
		}
	}
	for _, tc := range []struct {
		language string
		n        int
		want     string
	}{
		{"en-US", 1, "reused 1 file(s) from cache (not resent to the model)"},
		{"en-US", 3, "reused 3 file(s) from cache (not resent to the model)"},
		{"pt-BR", 1, "1 arquivo reaproveitado do cache (não reenviado ao modelo)"},
		{"pt-BR", 3, "3 arquivos reaproveitados do cache (não reenviados ao modelo)"},
	} {
		if got := cacheReusedLine(tc.language, tc.n); got != tc.want {
			t.Errorf("cacheReusedLine(%s, %d) = %q, want %q", tc.language, tc.n, got, tc.want)
		}
	}
}

// AUR-538 AC-007 in both languages: a breach without a finding to name
// (dependencies) stays ahead of the inconclusive line, which is recognized
// by the catalog's template in Portuguese too; English is unchanged.
func TestAUR607BreachWithoutFindingKeepsTheBreachAhead(t *testing.T) {
	dep := "DEPENDÊNCIAS reprova evil-pkg 1.0.0 (npm, package.json), severidade critical, introduzida pelo PR, pacote malicioso [dependencies]"
	for _, tc := range []struct{ language, want string }{
		{"en-US", "falha: achado(s) reprovam o gate numa revisão também inconclusiva no pull request #7: " + dep + "; review inconclusive (partial_coverage,quality_skipped)"},
		{"pt-BR", "falha: o gate reprovou — DEPENDÊNCIAS reprova evil-pkg 1.0.0 (npm, package.json), severidade critical, introduzida pelo PR, pacote…"},
	} {
		g := gateDecision{
			Active: true, Breach: true, Fail: true, Inconclusive: true, Reason: "partial_coverage,quality_skipped",
			Lines:            []string{reasons.Line(tc.language, "partial_coverage,quality_skipped"), dep},
			BlockingFindings: []facts.AuditFinding{{RuleID: "dependencies/evil-pkg", Path: "package.json", Severity: "critical"}},
		}
		_, got := gateStatusDescription(tc.language, g, 7)
		want := tc.want
		if tc.language == "en-US" {
			want = capStatusDescription(gateStatusWordFailure, strings.TrimPrefix(tc.want, "falha: "), statusDescriptionLimit)
		}
		if got != want {
			t.Errorf("%s: got %q, want %q", tc.language, got, want)
		}
		if !strings.Contains(got, "evil-pkg") || utf8.RuneCountInString(got) > statusDescriptionLimit {
			t.Errorf("%s: the dependency that failed the gate was cut: %q", tc.language, got)
		}
	}
	if !reasons.IsLine("pt-BR", reasons.Line("pt-BR", "partial_coverage")) || !reasons.IsLine("en-US", "review inconclusive (x)") || reasons.IsLine("pt-BR", "DEPENDÊNCIAS reprova x") {
		t.Error("IsLine does not recognize the inconclusive line by the catalog template")
	}
}

// aur607Artifacts runs the review of aur607Review writing the audit record
// and the SARIF document, and returns the audit's gate reason and every
// SARIF message.
func aur607Artifacts(t *testing.T, language string) (reason string, messages []string) {
	t.Helper()
	dir := t.TempDir()
	audit, sarif := filepath.Join(dir, "audit.json"), filepath.Join(dir, "review.sarif")
	aur607Review(t, language, "--auditoria", audit, "--sarif", sarif)
	var rec struct {
		Gate struct{ Decision, Reason string } `json:"gate"`
	}
	aur607ReadJSON(t, audit, &rec)
	var doc struct {
		Runs []struct {
			Results     []struct{ Message struct{ Text string } } `json:"results"`
			Invocations []struct {
				Notifications []struct{ Message struct{ Text string } } `json:"toolExecutionNotifications"`
			} `json:"invocations"`
		} `json:"runs"`
	}
	aur607ReadJSON(t, sarif, &doc)
	for _, run := range doc.Runs {
		for _, r := range run.Results {
			messages = append(messages, r.Message.Text)
		}
		for _, inv := range run.Invocations {
			for _, n := range inv.Notifications {
				messages = append(messages, n.Message.Text)
			}
		}
	}
	return rec.Gate.Decision + "|" + rec.Gate.Reason, messages
}

func aur607ReadJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("%s: %v\n%s", path, err, data)
	}
}

// The audit record and the SARIF document are machine data: the same in
// pt-BR and en, with the reason codes written as before.
func TestAUR607AuditAndSARIFIgnoreTheLanguage(t *testing.T) {
	enReason, enMessages := aur607Artifacts(t, "")
	ptReason, ptMessages := aur607Artifacts(t, "pt-BR")
	if enReason != ptReason || strings.Join(enMessages, "\n") != strings.Join(ptMessages, "\n") {
		t.Fatalf("artifacts differ by language:\nen %q %q\npt %q %q", enReason, enMessages, ptReason, ptMessages)
	}
	if !strings.Contains(enReason, "review inconclusive (quality_skipped)") {
		t.Fatalf("audit reason lost the earlier line: %q", enReason)
	}
}

// A secret finding's SARIF message is the engine's own text in every
// language; the shown message is the parecer's.
func TestAUR607SARIFKeepsTheEngineMessage(t *testing.T) {
	finding := scanner.Finding{Path: "config.py", Line: 1, RuleID: "gitleaks:github-pat", Severity: "error",
		Message: "Uncovered a GitHub Personal Access Token. in commit 37eb089803ee (gitleaks v8.30.1)"}
	issues, tools := (&reviewState{reviewLanguage: "pt-BR"}).scannerIssues([]scanner.Finding{finding}, "gitleaks", "secrets", "gitleaks")
	path := filepath.Join(t.TempDir(), "review.sarif")
	in := complianceArtifactInputs{sarifPath: path, issues: issues, toolMessages: tools}
	if err := writeSARIFFile(in, redaction.NewFilter()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), finding.Message+" (rule gitleaks:github-pat)") || strings.Contains(string(data), "Segredo ou credencial") {
		t.Fatalf("SARIF does not keep the engine's message:\n%s", data)
	}
}

// TestAUR607TriageNoticeShowsTheReasonSentence: the triage notice AUR-608
// added shows the inconclusive reason as the catalog sentence in pt-BR (code
// in brackets, no nested parentheses) and keeps the bare code in English.
func TestAUR607TriageNoticeShowsTheReasonSentence(t *testing.T) {
	pt := i18n.Format("pt-BR", "notice.triage_not_run", reasons.Text("pt-BR", "quality_skipped"))
	if strings.Contains(pt, "(quality_skipped)") || !strings.Contains(pt, "não ocorreu — revisão por modelo não executada") || !strings.Contains(pt, "[quality_skipped]") {
		t.Fatalf("pt-BR triage notice must carry the reason sentence: %q", pt)
	}
	en := i18n.Format("en", "notice.triage_not_run", reasons.Text("en", "quality_skipped"))
	if en != "gate.triage: the model's triage did not run (quality_skipped); the deterministic evidence counted in full and the block was kept" {
		t.Fatalf("English triage notice changed: %q", en)
	}
}
