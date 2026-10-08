# AurumCode

Code review de pull requests com o modelo e o contexto do seu time.

Projeto gratuito e de código aberto, sob licença MIT: não há assinatura do
AurumCode. O único custo possível é o do provedor de modelo que você escolher;
a análise determinística roda offline, sem nenhuma credencial.

**[Abra o guia interativo →](https://mpaape.github.io/AurumCode/)**

## O que o Aurum oferece

- **[Revisão](docs/tutorials/revisao.md)**: a IA revisa cada pull request seguindo as regras do seu time e diz o quê, onde e por quê.
- **[Correção](docs/getting-started.md#corrigir-sugestoes-com-o-fix)**: `aurumcode fix` aplica no código a correção que a revisão sugeriu.
- **[Inventário de dependências](docs/tutorials/sbom-dependency-track.md)**: o SBOM é a lista de todas as bibliotecas e versões do projeto, como o rótulo de ingredientes de um alimento.
- **[Assinatura](docs/tutorials/assinatura.md)**: `aurumcode sign` sela o inventário e a imagem e prova que nada foi alterado depois.
- **[Inventário ampliado](docs/tutorials/xbom.md)**: o xBOM registra também como o software foi construído e que criptografia ele usa.
- **[Dependências vulneráveis](docs/configuration.md#dependencias-do-pr-dependencies)**: aponta biblioteca com falha conhecida, pacote malicioso ou licença proibida, em cada pull request e em varredura agendada.
- **[Consulta do agente de IA](docs/tutorials/agente.md)**: seu agente de código (Claude Code, Codex e outros) consulta o Aurum pelo protocolo MCP antes de abrir a pull request.
- **[Changelog](docs/tutorials/changelog.md)**: o Aurum não faz nada (`off`), sugere a entrada do changelog (`suggest`) ou exige a entrada (`required`).
- **[Melhoria contínua](docs/tutorials/realimentacao.md)**: propõe ajustes de regras a partir do uso real, e uma pessoa aprova cada ajuste.

Funciona em qualquer linguagem de programação ([exemplo](docs/tutorials/qualquer-linguagem.md)).

## Comece aqui

1. No repositório que receberá os reviews, configure os secrets de Actions
   `LLM_API_KEY` e `LLM_BASE_URL` do seu provedor de modelo. Se o serviço
   exigir o identificador do modelo, adicione a variável de Actions
   `LLM_MODEL`. Para um provedor que não é compatível com OpenAI (Azure
   OpenAI, Anthropic, Gemini, Bedrock e outros), escolha o perfil no input
   `provider` do workflow; veja [Provedores de LLM](docs/provedores.md).
2. Copie o [workflow pronto](docs/site/workflow.yml) para
   `.github/workflows/aurumcode.yml` no seu repositório. Ele usa a branch
   publicada `main`.
3. Integre o arquivo na branch base e abra um PR com código.

Sem provedor configurado, o review local roda apenas a análise determinística
gratuita; veja [Uso local](docs/getting-started.md). A tag histórica `v1` não é
atualizada por esse fluxo: instalações novas usam `main`. Para fixar uma versão,
use um SHA revisado.

## Personalize somente o necessário

Arquivo opcional `.aurumcode/config.yml`:

```yaml
review:
  language: pt-BR
  publication: review
  inline_comments: true
```

Sem configuração: inglês, comentário na conversa e sem comentários nas linhas.
Para orientar a análise, escreva `.aurumcode/prompt.md`. Skills e documentos
podem ser acrescentados em `review.context`; consulte a
[referência de configuração](docs/configuration.md).

## Capacidades

Cada capacidade aponta para um comando ou tutorial reproduzível e declara uma
limitação real. O guia interativo reúne exemplos e saídas observadas.

- **Review local e de PR** — `aurumcode review --base HEAD~1` no checkout, ou o
  workflow publicado no PR. Tutorial: [Primeiro review](docs/getting-started.md).
  Limitação: o resultado semântico depende do modelo configurado.
- **Análise determinística gratuita (sem credencial)** — catálogo embutido de
  segurança e qualidade sobre as linhas do diff. Tutorial:
  [Uso local](docs/getting-started.md). Limitação: são heurísticas; não provam
  que o defeito existe em execução.
- **Contexto de codebase** — quem mais a mudança afeta, por heurística, sem
  índice do repositório. Configuração: [Prompts, skills e docs](docs/configuration.md).
  Limitação: aumenta os tokens enviados ao modelo.
- **Resumo e diagrama** — TL;DR e fluxo alterado em Mermaid, determinísticos.
  Limitação: o diagrama representa referências inferidas, não o fluxo em runtime.
- **`aurumcode fix`** — converte sugestões da revisão em um diff unificado
  aplicável. Exemplo: [Corrigir sugestões com o fix](docs/getting-started.md).
  Limitação: nada é escrito no repositório automaticamente; você inspeciona e aplica.
- **Memória por repositório (opt-in)** — `review.memory: local|ephemeral`,
  padrão desligado. Detalhes: [configuração](docs/configuration.md). Limitação:
  em runner efêmero, exige persistir o cache entre execuções; nunca altera
  regras, severidade ou veredito.
- **Custo sob controle** — `--limite` estima antes de chamar o modelo e recusa
  sem gastar nada. Limitação: a estimativa depende do preço configurado do modelo.
- **Modelo agnóstico** — qualquer endpoint compatível com OpenAI, inclusive
  local, ou um perfil de provedor por `LLM_PROVIDER` (Azure OpenAI, Anthropic,
  Gemini, Bedrock, LiteLLM, OpenRouter, OpenCode Zen, Ollama). Detalhes:
  [Provedores de LLM](docs/provedores.md). Limitação: sem provedor, não há
  revisão de qualidade por IA.
- **Prompts, skills e docs do time** — `.aurumcode/prompt.md` e
  `review.context` (inclusive fontes MCP em `review.context.mcp`) em
  [configuração](docs/configuration.md). Limitação: são contexto para o
  modelo; não alteram permissões, redação de segredos nem opções do programa.
- **Gate de política e cadeia de suprimentos** — política central, gate por
  severidade com exceções e auditoria SARIF, SAST com Semgrep, segredos com
  gitleaks, SBOM, Dependency-Track, Cosign e xBOM. Guia:
  [Guia corporativo](docs/gate-corporativo.md). Limitação: cada scanner
  precisa estar disponível; um scanner que falha deixa o gate inconclusivo,
  nunca aprovado.
- **Verificação de dependências** — dependência nova ou alterada no PR
  conferida contra o OSV, pacote malicioso ou typosquat, licença proibida e
  `fail_on` próprio, com exceção por CVE; `aurumcode dependencies` faz a
  varredura agendada do repositório inteiro. Referência:
  [Dependências do PR](docs/configuration.md#dependencias-do-pr-dependencies).
  Limitação: depende da base pública de advisories; sem resposta dela, o
  resultado é inconclusivo.
- **Changelog obrigatório** — `aurumcode changelog` reprova a PR sem entrada
  em `## Unreleased`. Guia: [Changelog obrigatório](docs/changelog.md).
  Limitação: confere a presença e a forma da entrada, não a qualidade do texto.
- **Realimentação da política** — `aurumcode realimentacao` transforma sinais
  de uso numa PR de propostas para a política. Tutorial:
  [Realimentação](docs/tutorials/realimentacao.md). Limitação: só propõe; uma
  pessoa revisa e integra a PR.
- **No seu agente de código** — `aurumcode mcp`, servidor MCP local com o mesmo
  gate do CI. Guia: [Aurum no seu agente](docs/agentes.md). Limitação: revisa
  commits, não edições ainda não commitadas.

Limitações atuais: o modelo só lê e busca arquivos do repositório por
ferramentas com limites, quando `deliberation` está ligado; não há busca web
nem execução automática de testes em sandbox. Entre rodadas do mesmo PR, um achado já comentado não é
comentado de novo (identidade por regra, caminho e código da linha); um achado
que o modelo descreve de outro jeito em outra regra é outro achado. PRs de forks não recebem os
secrets do repositório por padrão. No CI, o histórico de discussão do PR exige
permissão de leitura e o parecer é publicado com o `github.token`. O veredito
semântico depende do modelo; veja [qualidade e limitações](docs/review-quality.md).

## Executar e desenvolver

[Uso local em Docker](docs/getting-started.md) ·
[QA em container](docs/qa.md) · [Contrato de qualidade](docs/review-quality.md)

O CLI e o motor de revisão são Go. A integração usa Bash, Docker e workflows
YAML; o site usa HTML, CSS e JavaScript. Não é necessário instalar Go no host.

O site do AurumCode é publicado de `docs/site` por GitHub Pages. O antigo
gerador de documentação de outros repositórios foi removido. Especificações
históricas em `docs/specs` documentam o board, não capacidades atuais.
