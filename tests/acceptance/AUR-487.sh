#!/usr/bin/env bash
# Offline static-site contracts, scoped to actual HTML elements. No Go/runtime
# build. Browser layout/clipboard tests remain in tests/docs/site.test.cjs.
set -Eeuo pipefail
export LC_ALL=C
cd "$(dirname "$0")/../.."
selector=${1:-AC-001..AC-006}
case "$selector" in AC-00[1-6]|AC-001..AC-006) ;; *) exit 64 ;; esac
fail() { printf 'FAIL AUR-487 %s\n' "$*" >&2; exit 1; }
for tool in awk sed grep cmp; do command -v "$tool" >/dev/null || exit 79; done
for file in index.html style.css app.js workflow.yml mark.svg; do
  test -s "docs/site/$file" || fail "missing $file"
done
# Scope every assertion to its owning section; unrelated text cannot satisfy it.
section() { awk -v id="$1" 'index($0,"<section id=\"" id "\""){on=1} on{print} on && /<\/section>/{exit}' docs/site/index.html; }
text_only() { sed 's/<[^>]*>/ /g'; }
check() {
  local ac=$1 html token source
  case "$ac" in
  AC-001)
    html=$(section hero | text_only | tr '\n' ' ')
    html=${html:0:600}
    [[ ${html,,} == *'code review'* && ${html,,} == *'github actions'* && ${html,,} == *'open source'* ]] || fail 'hero message'
    section hero | grep -q 'href="#instalar"' || fail 'hero CTA'
    ;;
  AC-002)
    [[ $(sed -n 's/^<section id="\([^"]*\)".*/\1/p' docs/site/index.html | tr '\n' ' ') == 'hero como-funciona capacidades exemplo limites tutoriais instalar ' ]] || fail 'section order'
    section tutoriais | grep -q 'class="tut-flow"' || fail 'tutorial flow cards'
    [[ $(section tutoriais | grep -c 'class="flag ') -ge 8 ]] || fail 'tutorial flags'
    section como-funciona | awk '/<li>/{n++} /class="step-num"/{ if (index($0,">" n "</span>")==0) exit 1; nums++ } END{if(n!=3 || nums!=3) exit 1}' || fail 'three numbered steps'
    section capacidades | awk '
      /<article>/{n++; svg=title=para=0; body=""}
      /<svg /{svg++} /<h3>[^<]+<\/h3>/{title++}
      /<p>/{para++; body=$0; gsub(/<[^>]*>/,"",body)}
      /<\/article>/{if(svg!=1 || title!=1 || para!=1 || length(body)>180) bad=1}
      END{if(n<6 || bad) exit 1}' || fail 'capability icon/title/concise copy'
    html=$(section exemplo)
    [[ $html == *'<blockquote '* && $html == *'Potential SQL injection vulnerability detected</blockquote>'* && $html == *'pull/1#issuecomment-5289628080'* && $html == *'26fbc62897a6b137b6dd2f37bce427a0f6d2d0f7'* ]] || fail 'real cited review excerpt'
    html=$(section limites)
    for token in sql-injection command-injection hardcoded-secret xss; do [[ $html == *"security/$token"* ]] || fail "missing matcher $token"; done
    ! section limites | grep -q '<table>' || fail 'limites must stay objective (no coverage table)'
    sed -n '/<footer>/,/<\/footer>/p' docs/site/index.html | awk '/<h4>/{n++} END{if(n<3) exit 1}' || fail 'footer columns'
    ;;
  AC-003)
    for token in hero capacidades; do section "$token" | grep -q 'href="#instalar"' || fail "CTA $token"; done
    sed -n '/<footer>/,/<\/footer>/p' docs/site/index.html | grep -q 'href="#instalar"' || fail 'footer CTA'
    ;;
  AC-004)
    # Extract actual registered flags, env reads and YAML tags, excluding tests
    # and comments so a mere mention in source cannot validate an invented API.
    source=$(find cmd/aurumcode internal/config -name '*.go' ! -name '*_test.go' -exec sed '/^[[:space:]]*\/\//d' {} +)
    while IFS= read -r token; do
      [[ $source =~ fs\.(String|Bool|Int|Float64)\(\"${token#--}\" ]] || fail "unknown flag $token"
    done < <(grep -oE -- '--[a-z][a-z-]+' docs/site/index.html | sort -u)
    while IFS= read -r token; do
      [[ $source == *"os.Getenv(\"$token\")"* ]] || fail "unknown environment $token"
    done < <(grep -oE '\b(LLM|AURUMCODE)_[A-Z_]+\b' docs/site/index.html | sort -u)
    for token in review memory language publication inline_comments context prompt skills docs; do
      [[ $source == *"yaml:\"$token\""* ]] || fail "unknown YAML key $token"
    done
    ! grep -iE '(sem|não implementad[oa]).*(memória persistente|histórico do PR|resumo|mermaid|análise determinística)' docs/site/index.html || fail 'false missing capability'
    ! grep -iE 'catálogo completo|resto do catálogo' docs/site/index.html || fail 'overclaims full deterministic catalog coverage'
    section instalar | grep -q 'GitHub Actions pode ter custo' || fail 'provider/Actions cost disclosure'
    ;;
  AC-005)
    awk 'BEGIN{RS="[;{}]"} /font-size[[:space:]]*:/ {s=$0; sub(/.*font-size[[:space:]]*:[[:space:]]*/,"",s); if(s ~ /^[0-9.]+px/ && s+0<11) bad=1} END{exit bad}' docs/site/style.css || fail 'font below 11px'
    grep -q '^:root' docs/site/style.css || fail 'color tokens'
    grep -q 'prefers-color-scheme: dark' docs/site/style.css || fail 'dark palette'
    grep -q 'prefers-reduced-motion: reduce' docs/site/style.css || fail 'reduced motion'
    ! grep -q '@main' docs/site/workflow.yml docs/site/index.html || fail 'unpinned snippet'
    cmp -s docs/site/workflow.yml .github/workflows/examples/code-review.yml || fail 'workflow drift (requires AUR-493 @v2 integration)'
    ;;
  AC-006)
    while IFS= read -r token; do
      token=${token#*\"#}; token=${token%\"}
      [[ $(grep -o "id=\"$token\"" docs/site/index.html | wc -l) -eq 1 ]] || fail "anchor $token"
    done < <(grep -oE 'href="#[^"]+"' docs/site/index.html)
    while IFS= read -r token; do
      token=${token#*\"}; token=${token%\"}
      case "$token" in \#*|https:*|http:*) continue ;; esac
      [[ -f "docs/site/$token" && $token != *'..'* ]] || fail "asset $token"
    done < <(grep -oE '(src|href)="[^"]+"' docs/site/index.html)
    ;;
  esac
  printf 'PASS AUR-487 %s\n' "$ac"
}
# Exact static snippet comparison also proves the page works without JS/fetch.
if [[ $selector == AC-002 || $selector == AC-005 || $selector == AC-001..AC-006 ]]; then
  inline=$(sed -n '/<code id="workflow-code">/,/<\/code>/p' docs/site/index.html | sed '1s/.*<code id="workflow-code">//; $s/<\/code>.*//')
  [[ "$inline" == "$(<docs/site/workflow.yml)" ]] || fail 'inline/download workflow mismatch'
fi
if [[ $selector == AC-001..AC-006 ]]; then
  for ac in AC-001 AC-002 AC-003 AC-004 AC-005 AC-006; do check "$ac"; done
else check "$selector"; fi
