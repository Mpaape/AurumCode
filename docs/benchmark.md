# Benchmark de review: protocolo, metricas e corpus

Este documento descreve o corpus versionado e o harness deterministico em
`tests/benchmark`. O objetivo e comparar AurumCode, OCR e produtos hospedados
sob o mesmo protocolo e demonstrar utilidade ao time sem contar comentario como
qualidade.

## Corpus congelado

O corpus vive em `tests/benchmark/testdata/corpus.json` e segue o schema
`aurum.benchmark-corpus` versao 1. Antes de qualquer execucao:

- `frozen_at` registra quando o ground truth foi congelado;
- cada caso pina `base` e `head` (SHAs de 40 hex) e o prompt versao;
- a separacao `dev`/`holdout` fica congelada antes da execucao;
- o corpus cobre multiplas linguagens, ao menos um defeito cross-file e ao
  menos um negativo sem defeito (PR limpo).

`Corpus.Validate` recusa um corpus sem dev ou sem holdout, sem duas linguagens,
sem defeito cross-file, sem negativo ou sem defeito. `Corpus.GroundTruthDigest`
resume o ground truth; cada rodada registra esse digest, de modo que a
pontuacao fica amarrada ao corpus exato.

## Harness

`Harness.Run` executa quatro rodadas por caso, na ordem declarada:

| rodada | significado |
|---|---|
| `original` | PR como submetido, com os defeitos |
| `unchanged` | reexecucao sem mudanca, mede repeticao |
| `partial` | PR parcialmente corrigido |
| `fixed` | PR corrigido |

Cada `RunRecord` registra modelo, provider, config, prompt, skills, `base`,
`head`, digest do corpus, latencia e custo. Um pipeline indisponivel vira
`not-measured`: sem acesso nunca e nota zero.

`Evaluate` casa achado e defeito por arquivo e linha com tolerancia de duas
linhas. O primeiro achado que casa e o verdadeiro positivo; um achado posterior
no mesmo defeito e duplicata; um achado que nao casa nada e ruido. Duplicata e
ruido contam contra a precisao, entao repetir ou inventar achado nunca melhora a
nota. Sao reportados precisao, recall, ruido, duplicatas, localizacao exata,
latencia, custo, tamanho de amostra e intervalo de Wilson de 95%.

## Fixtures e o piloto semantico

Fixtures deterministicas validam o harness e sao marcadas como `synthetic`;
`RealScores` remove essas rodadas. O piloto semantico local usa Qwen e registra o
modelo realmente servido, mas uma fixture nunca pode ser apresentada como score
de modelo real.

## Comparacoes controladas

- `ComparePipelines` separa um pipeline local de um produto hospedado e exige o
  mesmo modelo e corpus; ausencia de acesso vira `not-measured`.
- `CompareConfigs` liga/desliga contexto e varia prompts e skills versionados
  com modelo e corpus constantes, registrando ganho e perda por cenario para
  que um agregado nao esconda regressao.
- `Anonymize` rotula divergencias com ids opacos e devolve o mapa oculto, para
  adjudicacao cega.

## Sugestoes aplicaveis e decisao humana

Uma sugestao so e aplicavel quando o patch aplica no head, preserva os testes
pertinentes e remove o defeito (`EvaluateSuggestion`). `TriageEntry` guarda
tempo ativo de triagem, tempo de decisao e numero de comentarios em campos
separados; so uma decisao explicita (`accepted`) conta como aceitacao, nunca a
quantidade de comentarios.

## Como executar

```bash
./.board/bin/oci-run --profile go-unit-offline-v1 --card AUR-511
```

O aceite roda `go test ./tests/benchmark` no container selado, sem rede e sem
toolchain no host. O mesmo comando pode ser selecionado por cenario
(`AC-001`..`AC-006`, `MUT-001`).

## Limitacoes

O harness mede o que o pipeline entrega; ele nao substitui review independente.
Resultados de concorrentes sem acesso ficam `not-measured`. Amostras pequenas
vem com o intervalo de Wilson, e qualquer conclusao deve respeitar esse
intervalo e o tamanho da amostra. Fontes e protocolo do produto estao em
`.board/PRODUCT_PLAN.md`.
