# AurumCode

Code review de pull requests com o modelo e o contexto do seu time.

Projeto gratuito e de código aberto, sob licença MIT: não há assinatura do
AurumCode. O único custo possível é o do provedor de modelo que você escolher;
a análise determinística roda offline, sem nenhuma credencial.

**[Abra o guia interativo →](https://mpaape.github.io/AurumCode/)**

## Comece aqui

1. No repositório que receberá os reviews, configure os secrets de Actions
   `LLM_API_KEY` e `LLM_BASE_URL` do seu serviço compatível com OpenAI. Se o
   serviço exigir o identificador do modelo, adicione a variável de Actions
   `LLM_MODEL`.
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
- **Modelo agnóstico** — qualquer endpoint compatível com OpenAI, inclusive local.
  Limitação: sem provedor, não há revisão de qualidade por IA.
- **Prompts, skills e docs do time** — `.aurumcode/prompt.md` e
  `review.context` em [configuração](docs/configuration.md). Limitação: são
  contexto para o modelo; não alteram permissões, redação de segredos nem
  opções do programa.

Limitações atuais: não há navegação autônoma, busca web nem execução automática
de testes em sandbox. Entre rodadas do mesmo PR, um achado já comentado não é
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
