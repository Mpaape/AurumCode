# Benchmark de review: protocolo, métricas e corpus

Este documento descreve o corpus versionado e o harness determinístico em
`tests/benchmark`. O objetivo e comparar AurumCode, OCR e produtos hospedados
sob o mesmo protocolo e demonstrar utilidade ao time sem contar comentário como
qualidade.

## Corpus congelado

O corpus vive em `tests/benchmark/testdata/corpus.json` e segue o schema
`aurum.benchmark-corpus` versão 1. Antes de qualquer execução:

- `frozen_at` registra quando o ground truth foi congelado;
- cada caso pina `base` e `head` (SHAs de 40 hex) e o prompt versão;
- a separação `dev`/`holdout` fica congelada antes da execução;
- o corpus cobre múltiplas linguagens, ao menos um defeito cross-file e ao
  menos um negativo sem defeito (PR limpo).

`Corpus.Validate` recusa um corpus sem dev ou sem holdout, sem duas linguagens,
sem defeito cross-file, sem negativo ou sem defeito. `Corpus.GroundTruthDigest`
resume o ground truth; cada rodada registra esse digest, de modo que a
pontuação fica amarrada ao corpus exato.

## Harness

`Harness.Run` executa quatro rodadas por caso, na ordem declarada:

| rodada | significado |
|---|---|
| `original` | PR como submetido, com os defeitos |
| `unchanged` | reexecução sem mudança, mede repetição |
| `partial` | PR parcialmente corrigido |
| `fixed` | PR corrigido |

Cada `RunRecord` registra modelo, provider, config, prompt, skills, `base`,
`head`, digest do corpus, latência e custo. Um pipeline indisponível vira
`not-measured`: sem acesso nunca e nota zero.

`Evaluate` casa achado e defeito por arquivo e linha com tolerância de duas
linhas. O primeiro achado que casa e o verdadeiro positivo; um achado posterior
no mesmo defeito e duplicata; um achado que não casa nada e ruído. Duplicata e
ruído contam contra a precisão, então repetir ou inventar achado nunca melhora a
nota. São reportados precisão, recall, ruído, duplicatas, localização exata,
latência, custo, tamanho de amostra e intervalo de Wilson de 95%.

## Fixtures e o piloto semântico

Fixtures determinísticas validam o harness e são marcadas como `synthetic`;
`RealScores` remove essas rodadas. O piloto semântico local usa Qwen e registra o
modelo realmente servido, mas uma fixture nunca pode ser apresentada como score
de modelo real.

## Comparações controladas

- `ComparePipelines` separa um pipeline local de um produto hospedado e exige o
  mesmo modelo e corpus; ausência de acesso vira `not-measured`.
- `CompareConfigs` liga/desliga contexto e varia prompts e skills versionados
  com modelo e corpus constantes, registrando ganho e perda por cenário para
  que um agregado não esconda regressão.
- `Anonymize` rotula divergências com ids opacos e devolve o mapa oculto, para
  adjudicação cega.

## Sugestões aplicáveis e decisão humana

Uma sugestão só é aplicável quando o patch aplica no head, preserva os testes
pertinentes e remove o defeito (`EvaluateSuggestion`). `TriageEntry` guarda
tempo ativo de triagem, tempo de decisão e número de comentários em campos
separados; só uma decisão explícita (`accepted`) conta como aceitação, nunca a
quantidade de comentários.

## Como executar

```bash
./.board/bin/oci-run --profile go-unit-offline-v1 --card AUR-511
```

O aceite roda `go test ./tests/benchmark` no container selado, sem rede e sem
toolchain no host. O mesmo comando pode ser selecionado por cenário
(`AC-001`..`AC-006`, `MUT-001`).

## Limitações

O harness mede o que o pipeline entrega; ele não substitui review independente.
Resultados de concorrentes sem acesso ficam `not-measured`. Amostras pequenas
vem com o intervalo de Wilson, e qualquer conclusão deve respeitar esse
intervalo e o tamanho da amostra. Fontes e protocolo do produto estão em
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

`tests/benchmark/testdata/multilang/`:

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

1. Crie `tests/benchmark/testdata/multilang/cases/<id>/` com `case.json` e o arquivo
   (pequeno, sintético, sem dado de organização; o id é o nome do diretório).
2. Regenere manifest e relatório com o harness, nunca à mão:
   `go test ./tests/benchmark -run TestAUR523 -update-aur523` (no container
   compartilhado).
3. Commite o caso, o `manifest.json` e o relatório gerado. Nunca edite o
   relatório ou o manifest manualmente: o teste falha se eles não forem
   exatamente o que o corpus e a política produzem.
