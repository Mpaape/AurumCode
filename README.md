# AurumCode

Code review de pull requests com o modelo e o contexto do seu time.

**[Abra o guia interativo →](https://mpaape.github.io/AurumCode/)**

Configure duas credenciais no repositório que receberá os reviews:
`LLM_API_KEY` e `LLM_BASE_URL`. Se o serviço exigir o identificador do modelo,
adicione a variável de Actions `LLM_MODEL`.

Copie o [workflow pronto](docs/site/workflow.yml) para
`.github/workflows/aurumcode.yml` no seu repositório. Ele usa a branch publicada
`main`. Integre o arquivo na branch base e abra um PR com código.

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

O núcleo em Go lê o diff, detecta linguagens, monta o prompt, chama um serviço
compatível com OpenAI e publica o parecer. O review cobre correção, contratos,
manutenção, performance e segurança, com sugestões opcionais de implementação.
Um filtro exige linha adicionada ou removida, regra reconhecida e campos de evidência,
impacto e verificação; não verifica automaticamente a veracidade do raciocínio.

No PR, o contexto inclui o diff, os arquivos configurados, o CI fornecido e as
reviews, comentários e respostas já publicados no GitHub. Ainda sem configuração,
o review também ganha: análise estática determinística (catálogo embutido de
segurança/qualidade), contexto de codebase limitado por heurística (quem mais o
mudança afeta), um resumo TL;DR e um diagrama Mermaid do fluxo alterado, e uma
lista proposta de testes. `aurumcode fix` converte as sugestões em um diff
unificado aplicável. A memória entre revisões é opt-in (`review.memory:
local|ephemeral`), padrão desligado.

Ainda não há navegação autônoma, busca web, execução automática de testes em
sandbox ou deduplicação garantida. O resultado semântico depende do modelo.

## Executar e desenvolver

[Uso local em Docker](docs/getting-started.md#uso-local) ·
[QA em container](docs/qa.md) · [Contrato de qualidade](docs/review-quality.md)

O CLI e o motor de revisão são Go. A integração usa Bash, Docker e workflows
YAML; o site usa HTML, CSS e JavaScript. Não é necessário instalar Go no host.

O site do AurumCode é publicado de `docs/site` por GitHub Pages. O antigo
gerador de documentação de outros repositórios foi removido. Especificações
históricas em `docs/specs` documentam o board, não capacidades atuais.
