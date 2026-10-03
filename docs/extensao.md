# Estendendo o Aurum

Este guia mostra os quatro pontos por onde o AurumCode cresce sem mudar o
núcleo: **engine de scanner**, **ferramenta de deliberação**, **skill** e
**fonte de contexto** (`ContextProvider`). Para cada um: o contrato exato
(tipos e métodos que existem no código; um teste compara cada nome citado
aqui com os pacotes), o que o produto garante em volta dele e um exemplo
mínimo. O tutorial executável [Estendendo o Aurum na prática](tutorials/extensao.md)
roda a engine de exemplo, uma skill e uma ferramenta pedida pelo modelo.

A regra que vale para os quatro: **dado nunca é autoridade**. O que uma
extensão devolve é evidência (engine) ou contexto (ferramenta, skill, fonte
de contexto). Quem decide o gate é a configuração (`.aurumcode.yml` e a
política central) e o código; nenhum texto vindo de modelo, skill ou fonte
de contexto liga ou desliga regra, muda severidade ou afrouxa o limiar.

Os nomes abaixo vêm de `internal/scanner`, `internal/deliberation`,
`internal/review/tools`, `internal/context/skills` e `internal/config`. A
visão geral dos pacotes está em [Arquitetura](architecture.md).

## 1. Engine de scanner

Uma engine é evidência determinística: seus achados entram no gate com uma
origem tipada e o modelo pode comentá-los, nunca removê-los nem rebaixá-los.

### O contrato

- `scanner.Scanner` é a engine: `scanner.Scanner.Name` devolve o nome
  registrado e `scanner.Scanner.Run` recebe um `scanner.Request` e devolve um
  `scanner.Report` ou um erro.
- `scanner.Request` traz a árvore (`scanner.Request.Root`), quem mandou
  varrer (`scanner.Request.Trust`: `scanner.TrustRepository` ou
  `scanner.TrustPolicy`; sob política a engine ignora as supressões do
  autor), as opções da entrada (`scanner.Request.Options`), o intervalo de
  commits revisado (`scanner.Request.Range`, com `scanner.Range.Empty`) e o
  executor de binários (`scanner.Request.Command`).
- `scanner.Report` tem `scanner.Report.Findings`, `scanner.Report.Complete`
  e `scanner.Report.Version`. `Complete: false` é inconclusivo, nunca "zero
  achados"; a versão entra no digest do cache de veredito.
- `scanner.Finding` é um achado (`Path`, `Line`, `Side`, `RuleID`,
  `Severity`, `Message`); `scanner.Finding.ToIssue` é a única conversão em
  achado de revisão, com a origem da engine.
- `scanner.Engine` é a entrada do registro: `scanner.Engine.Scanner`,
  `scanner.Engine.Category` (o nome que `gate.sources` também aceita, como
  `sast` ou `secrets`), `scanner.Engine.Origin` (vazio: o nome) e
  `scanner.Engine.Validate`, que confere as `options` quando a configuração
  é lida. `scanner.Engine.TypedOrigin` é a origem que aparece na linha do
  gate, na auditoria e no SARIF.
- `scanner.Register` é chamado no `init` do pacote da engine. O registro é
  **fechado**: o binário só conhece as engines importadas por
  `internal/scanner/engines`. Um `engine:` que o registro não tem é erro de
  configuração (`unknown engine`), nunca uma engine carregada em tempo de
  execução.
- `scanner.Executor` roda a engine (`scanner.Executor.Scan`, com o limite
  `scanner.Timeout`) e devolve um `scanner.Outcome`. A **regra única de
  inconclusivo** é `scanner.FailureReason`: erro, binário ausente, saída
  inválida (`scanner.ErrInvalidOutput`) ou relatório incompleto viram
  `<categoria>_unavailable`, `_execution_error`, `_invalid_output` ou
  `_incomplete`, e `scanner.Outcome.Findings` fica vazio. Falha nunca é lida
  como "limpo".

### Ligar na configuração

A engine entra por `quality_gates.scanners`, uma entrada por engine:

```yaml
quality_gates:
  scanners:
    - engine: exemplo
      required: false   # opcional: vira a ferramenta scanner_exemplo
      fail_on: error
gate:
  fail_on: [error]
```

Uma entrada roda a menos que `enabled: false`. Sob política central,
`required: true` impede o repositório de remover ou afrouxar a entrada. Com
a deliberação ligada, uma entrada `required: false` não roda antes do
modelo: é oferecida a ele como ferramenta (seção 2).

### Exemplo mínimo

A engine de exemplo (`internal/scanner/engines/exemplo`) reporta cada linha
da árvore revisada que contém a marca `exemplo.Marker` (`EXEMPLO-ACHADO`),
sem binário externo. O pacote não se registra sozinho:

```go
// internal/scanner/engines/exemplo/exemplo.go (resumo)
func Engine() scanner.Engine {
	return scanner.Engine{Scanner: Scanner{}, Category: Name, Origin: Name, Validate: validateOptions}
}

func (Scanner) Run(ctx context.Context, req scanner.Request) (scanner.Report, error) {
	// percorre req.Root sem .git; cada linha com a marca vira um scanner.Finding
	return scanner.Report{Findings: findings, Complete: true, Version: Version}, nil
}
```

O registro fica num arquivo com tag de build, então **o binário padrão não
contém a engine de exemplo**:

```go
//go:build aurum_exemplo

package engines

func init() { scanner.Register(exemplo.Engine()) }
```

A imagem com a engine é construída com `docker build --build-arg
GO_TAGS=aurum_exemplo` (o `Dockerfile` passa `GO_TAGS` ao `go build -tags`).
Uma engine de verdade não precisa de tag: é um pacote em
`internal/scanner/<engine>` e uma linha de import em
`internal/scanner/engines/engines.go`. Nada em `internal/gate`,
`internal/config` ou `cmd` muda.

## 2. Ferramenta de deliberação

Na deliberação o modelo pode pedir ferramentas antes de responder. O código
nunca pede uma ferramenta sozinho; o modelo decide pelo que o prompt mostra.

### O contrato

- `deliberation.Tool` tem dois métodos: `deliberation.Tool.Spec` devolve um
  `llm.ToolSpec` (nome, descrição e o JSON Schema dos argumentos) e
  `deliberation.Tool.Run` recebe argumentos que **já passaram** pelo schema
  (`deliberation.ValidateArguments`) e devolve um `deliberation.Result`.
- `deliberation.Result` tem `deliberation.Result.Content` (vai ao modelo,
  redigido e limitado em tamanho), `deliberation.Result.Summary` (a linha do
  transcript) e `deliberation.Result.Digest` (entra na chave do cache, para
  um resultado diferente nunca reaproveitar um veredito).
- Uma ferramenta só lê: não escreve e não acessa a rede além do que o
  produto já faz.
- `deliberation.Limits` limita a conversa: `deliberation.Limits.MaxRounds`,
  `deliberation.Limits.MaxCostTokens` e `deliberation.Limits.PerToolTimeout`
  (`deliberation.max_rounds`, `max_cost_tokens` e `per_tool_timeout_seconds`
  na configuração). Estourar um limite é um `deliberation.LimitError`: a
  revisão é **inconclusiva** (`deliberation_limit:<limite>`), nunca um
  parecer.
- `deliberation.Transcript` é o registro na auditoria (campo `deliberation`):
  ferramentas oferecidas, pedidas e não pedidas, cada `deliberation.Call` com
  argumentos redigidos, duração e resultado resumido, e o limite estourado.
- O manifesto (`tools.Manifest`, a partir de cada `tools.Offer` com seu
  custo declarado em `tools.Offer.Cost`) vai ao prompt com o nome exato que
  o modelo deve usar.

### Exemplo mínimo

Toda engine `required: false` já vira a ferramenta `scanner_<engine>`
(`tools.ScannerTool`, nome dado por `tools.ScannerToolName`): o tutorial usa
`scanner_exemplo` e o achado conta no gate com origem `exemplo`, qualquer
que seja a resposta do modelo. Uma ferramenta que não é scanner é código
compilado em `internal/review/tools`, oferecido em `toolOffers`
(`cmd/aurumcode/review_deliberation.go`):

```go
type contarLinhas struct{ root string }

type contarArgs struct {
	Path string `json:"path" desc:"arquivo alterado"`
}

func (t contarLinhas) Spec() llm.ToolSpec {
	return llm.ToolSpec{Name: "contar_linhas", Description: "Conta as linhas de um arquivo alterado.", Parameters: llm.SchemaOf(contarArgs{})}
}

func (t contarLinhas) Run(ctx context.Context, raw json.RawMessage) (deliberation.Result, error) {
	var args contarArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return deliberation.Result{}, err
	}
	n := 0 // ... lê args.Path sob t.root, sem escrever nada
	return deliberation.Result{Content: fmt.Sprintf("%d linhas", n), Summary: fmt.Sprintf("%d linhas", n), Digest: args.Path}, nil
}
```

## 3. Skill

Skill é Markdown: orientação de revisão escrita pelo time, sem execução de
código. Ela chega ao prompt como contexto e, por seção, vira regra citável.

- Em arquivo listado: `review.context.skills: [.aurumcode/skills/<nome>.md]`;
  cada seção `## Título` vira a regra `<nome>#<titulo>`, com piso de
  severidade opcional (`severity: error`).
- Em diretório: `.aurumcode/skills/<nome>/SKILL.md` (`skills.DocName`), com
  metadados `name`, `version`, `languages` e `paths`. Sem seletor, a skill
  fica desligada; ela vale quando todos os critérios declarados casam com um
  arquivo alterado.
- **Catálogo em camadas**: a política central tem as suas em
  `<política>/.aurumcode/skills/`; no mesmo seletor a da política vence e o
  repositório recebe um aviso. Sob política, só as seções da política
  entram no gate. Um `SKILL.md` ilegível na política é erro de carga; no
  repositório, aviso.
- O texto da skill é dado: nada lê a skill para decidir regra, severidade,
  limiar, redação ou custo (`skills.Provider` é um `config.ContextProvider`).

Exemplo mínimo (do tutorial):

```markdown
# Skill de exemplo

## Funcoes pequenas
severity: warning
Uma funcao faz uma coisa so. MARCA-SKILL-EXEMPLO
```

Detalhes em [Prompts, skills e docs](configuration.md#prompts-skills-e-docs)
e no tutorial [Skills de convenção](tutorials/skills.md).

## 4. Fonte de contexto (`ContextProvider`)

`config.ContextProvider` é a costura comum de todo contexto injetado na
revisão: arquivos, skills, e no futuro MCP (AUR-469) e índices de
repositório.

- `config.ContextProvider.Name` identifica a fonte no prompt e nos avisos.
- `config.ContextProvider.Provide` recebe os caminhos alterados e devolve
  texto livre; `""` significa "nada a contribuir".
- Limites: cada chamada roda sob `config.ProviderTimeout` (10 s) e o texto é
  limitado a `config.MaxProviderContributionBytes` (64 KiB). Passar do
  tamanho é erro alto, nunca truncamento silencioso; uma fonte que falha ou
  estoura o tempo vira `config.ProviderWarning` e a revisão segue sem ela.
- `config.BuildContextBlockWithWarnings` monta o bloco, redigindo segredos em
  cada contribuição e de novo no bloco inteiro.
- O texto é **dado, nunca autoridade**: só entra no prompt como contexto.

```go
type notasDoTime struct{ texto string }

func (n notasDoTime) Name() string { return "notas do time" }

func (n notasDoTime) Provide(ctx context.Context, changedPaths []string) (string, error) {
	if len(changedPaths) == 0 {
		return "", nil
	}
	return n.texto, nil
}
```

**O lugar do MCP.** Um servidor MCP entra como fonte de contexto (AUR-469):
um `config.ContextProvider` cujo `Provide` consulta o servidor dentro de
`config.ProviderTimeout`. Ele informa o modelo; não vira ferramenta que
decide o gate nem engine de scanner.

## O que NÃO é ponto de extensão

- **Plugin dinâmico.** Não existe carga de código em tempo de execução: nem
  `.so`, nem binário apontado pela configuração, nem script de skill. Uma
  engine ou ferramenta nova é código compilado no binário, revisado como o
  resto.
- **Texto do modelo alterando o gate.** A severidade que o modelo escreve não
  é confiável; uma skill, uma fonte de contexto ou uma resposta de ferramenta
  não liga regra, não rebaixa achado e não afrouxa `fail_on`.
- **MCP como autoridade.** MCP é contexto (AUR-469), nunca fonte de achado do
  gate nem de configuração.
- **Engine fora do registro.** `engine:` com nome que o binário não contém é
  recusado ao ler a configuração; o tutorial mostra o binário padrão
  recusando `engine: exemplo`.
