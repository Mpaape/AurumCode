package dependencies

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// LicenseReader returns the license file text of a package version, ""
// when it has none. It is the fallback when the registry gives no SPDX
// expression; without one, such a license is unknown.
type LicenseReader interface {
	LicenseText(ctx context.Context, c Change) (string, error)
}

// vetLicenses judges the registered license of every new or updated package
// against denied (only when the policy lists licenses). The registry's
// expressions are judged one by one, all of them must allow the package.
// When the registry has no usable expression the model may classify the
// package's license text; without a confident classification the license is
// unknown and the check inconclusive. A denied license is recorded whatever
// happens to the others.
func vetLicenses(ctx context.Context, model Completer, reg Registry, text LicenseReader, denied []string, report *Report) {
	for _, c := range report.Changes {
		if !c.NewOrUpdated() {
			continue
		}
		lic := judgeRegistered(ctx, reg, c, denied)
		if lic.Verdict == LicenseUnknown && text != nil && c.Head != "" {
			lic = classifyText(ctx, model, text, c, denied, lic)
		}
		report.Licenses = append(report.Licenses, lic)
		if lic.Verdict == LicenseUnknown {
			report.fail(ReasonLicenseUnknown, fmt.Sprintf("licenca de %s %s (%s) sem classificacao confiavel: %s", c.Name, c.Head, c.Manifest, lic.Reason))
		}
	}
}

// judgeRegistered asks the registry for the package version's licenses.
func judgeRegistered(ctx context.Context, reg Registry, c Change, denied []string) License {
	lic := License{Change: c, Verdict: LicenseUnknown}
	if c.Head == "" {
		lic.Reason = "so faixa declarada, sem versao para consultar"
		return lic
	}
	expressions, err := reg.Licenses(ctx, c)
	switch {
	case errors.Is(err, ErrNoSystem):
		lic.Reason = "pacote sem sistema no registro"
		return lic
	case err != nil:
		lic.Reason = err.Error()
		return lic
	case len(expressions) == 0:
		lic.Reason = "registro sem licenca"
		return lic
	}
	lic.Expression = strings.Join(expressions, "; ")
	lic.Verdict = combine(expressions, denied)
	if lic.Verdict == LicenseUnknown {
		lic.Reason = "expressao sem identificador SPDX avaliavel"
	}
	return lic
}

// combine requires every registered expression to allow the package.
func combine(expressions, denied []string) LicenseVerdict {
	out := LicenseAllowed
	for _, e := range expressions {
		switch EvaluateLicense(e, denied) {
		case LicenseDenied:
			return LicenseDenied
		case LicenseUnknown:
			out = LicenseUnknown
		}
	}
	return out
}

// classifyText asks the model to name the SPDX expression of the license
// text; only a confident, parseable answer is judged.
func classifyText(ctx context.Context, model Completer, text LicenseReader, c Change, denied []string, prior License) License {
	body, err := text.LicenseText(ctx, c)
	if err != nil || strings.TrimSpace(body) == "" {
		return prior
	}
	var answer struct {
		SPDX      string `json:"spdx"`
		Confident bool   `json:"confident"`
	}
	if err := ask(ctx, model, "license.md", body, &answer); err != nil {
		prior.Reason = err.Error()
		return prior
	}
	if !answer.Confident || strings.TrimSpace(answer.SPDX) == "" {
		prior.Reason = "o modelo nao classificou o texto da licenca com confianca"
		return prior
	}
	verdict := EvaluateLicense(answer.SPDX, denied)
	if verdict == LicenseUnknown {
		prior.Reason = "classificacao do modelo sem identificador SPDX avaliavel"
		return prior
	}
	return License{Change: c, Expression: answer.SPDX, Verdict: verdict, ByModel: true}
}
