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

## Corpus multilinguagem contra o gate real (AUR-523)

Além do harness de fixtures acima, `tests/benchmark` mede o gate REAL: o
harness compila `./cmd/aurumcode` num diretório temporário e executa
`aurumcode review --base HEAD~1 --politica <política> --auditoria --sarif`
sobre cada caso do corpus. O veredito (decisão do gate e código de saída) e os
achados vêm da saída do binário (registro de auditoria e SARIF), nunca da
fixture que fez as vezes do modelo.

**O modelo é um provider fake determinístico.** Cada caso tem uma fixture
derivada do seu rótulo (`AURUMCODE_LLM_FIXTURE`): defeito com
`simulated_model: hit` cita o defeito, `miss` fica calado; limpo com `silent`
fica calado, `false_alarm` acusa um defeito que não existe. Os `miss` e
`false_alarm` existem para exercitar "aprovado com defeito presente" e falso
positivo; não são notas de modelo. A rodada com modelo real é decisão e gasto
do dono e não faz parte deste relatório nem do CI.

### Corpus

`tests/benchmark/multilang/`:

- `cases/<id>/case.json` (linguagem, arquivo, rótulo `defect`/`clean`, regra e
  linha do defeito) mais o arquivo-fonte do caso. O harness enumera o
  diretório: não há lista de linguagens ou de casos no código Go.
- `policy/.aurumcode/` é a política central usada (config com `gate` e a skill
  de segurança, uma seção `##` por regra).
- `manifest.json` guarda o sha256 de cada caso e o do conjunto. O harness
  recalcula e RECUSA qualquer divergência (caso alterado, acrescentado ou
  removido).

### Relatório

`tests/benchmark/out/multilang-report.json` e `.md`, por linguagem e no total:
recall, precisão, intervalo de Wilson de 95% de ambos, falsos positivos e a
contagem (com os casos) de "aprovado com defeito presente" (defeito rotulado e
gate `pass` com saída 0). Achados da mesma classe de regra e linha vindos da
skill de política e da análise embutida contam como um achado só. O cabeçalho
grava `corpus_sha256` e `policy_sha256`; o relatório não tem data, caminho nem
duração, então o mesmo corpus e a mesma política dão o mesmo relatório byte a
byte. O teste compara com o arquivo versionado.

### Como rodar

```bash
bash tests/acceptance/AUR-523.sh all          # ou AC-001..AC-004, MUT-001
./.board/bin/oci-run --profile go-unit-offline-v1 --card AUR-523
```

### Como adicionar um caso por PR

1. Crie `tests/benchmark/multilang/cases/<id>/` com `case.json` e o arquivo
   (pequeno, sintético, sem dado de organização; o id é o nome do diretório).
2. Regenere manifest e relatório com o harness, nunca à mão:
   `go test ./tests/benchmark -run TestAUR523 -update-aur523` (no container
   compartilhado).
3. Commite o caso, o `manifest.json` e o relatório gerado. Nunca edite o
   relatório ou o manifest manualmente: o teste falha se eles não forem
   exatamente o que o corpus e a política produzem.
