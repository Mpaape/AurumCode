# Configuração

Sem arquivo: inglês, comentário na conversa e sem comentários nas linhas.
Para mudar, crie `.aurumcode/config.yml`:

```yaml
review:
  language: pt-BR
  publication: review
  inline_comments: true
```

`publication: review` usa a revisão formal do GitHub. `comments` publica na
conversa. `inline_comments` habilita comentários nas linhas; no review formal,
uma sugestão elegível pode aparecer como substituição aplicável pelo GitHub.
O autor decide se aplica. AurumCode não altera o código automaticamente.

O idioma é enviado ao modelo. Os títulos do parecer têm tradução específica
para português e inglês; os demais idiomas aceitos usam títulos em inglês.
Tags aceitas: `pt-BR`, `pt`, `en-US`, `en`, `es-ES`, `fr-FR`,
`de-DE`, `it-IT`, `ja-JP`.

## Prompts, skills e docs

Escreva orientações em `.aurumcode/prompt.md`; o arquivo é encontrado
automaticamente. Para expandir:

```yaml
review:
  language: pt-BR
  context:
    skills:
      - .aurumcode/skills/backend.md
    docs:
      - docs/architecture.md
```

Uma skill é um Markdown com orientações de revisão, sem execução de scripts.
Liste apenas arquivos existentes. `context.prompt` permite substituir o
caminho do prompt adicional, mantendo a política embutida do produto.

O PR usa configuração e contexto da branch base; o idioma pode vir da versão
do PR. Isso significa que um novo prompt só passa a orientar reviews depois
de integrado à base. O uso local lê os arquivos do checkout.

Exemplo de prompt:

```text
Revise correção e compatibilidade dos contratos públicos.
Valores monetários são armazenados em centavos inteiros.
Use os testes e contratos disponíveis para sustentar cada achado.
Sugira código apenas quando a substituição for local e segura.
Indique contexto ausente sem presumir um defeito.
```

As contribuições são contexto para o modelo. Não alteram permissões,
redação de segredos nem opções do programa. A implementação atual aceita até
64 KiB por contribuição e informa erro se esse tamanho for excedido.

## Modelo e credenciais

Credenciais ficam nos secrets `LLM_API_KEY` e `LLM_BASE_URL` do repositório
hospedeiro. O exemplo passa a variável `LLM_MODEL` para o workflow reutilizável.
Nenhuma credencial deve estar em Markdown, YAML versionado ou na página.

O modelo é escolhido pelo serviço quando não há identificador explícito.
Não há limite de saída imposto por padrão pelo AurumCode; o serviço continua
sujeito à janela de contexto, ao timeout e às restrições do modelo.

## Opções avançadas

- `ignore`: lista de globs de caminhos a excluir antes da análise.
- `rules`: overrides explícitos de regras reconhecidas, por identificador.
- `review.memory`: `off` (padrão, sem estado), `ephemeral` (em processo) ou
  `local` (persistido por repositório no diretório de cache). No review de PR,
  o arquivo fica em `$XDG_CACHE_HOME/aurumcode/memory/repo/OWNER/REPO/notes.json`
  (ou no cache padrão do sistema quando `XDG_CACHE_HOME` está ausente).
  Sem coordenadas do GitHub, a identidade usa um hash do remote origin ou do
  caminho absoluto do checkout. O antigo cache global não é importado nem apagado.
  Memória guarda observações de
  revisões anteriores para reduzir repetição; nunca altera regras, severidade
  ou veredito.
- Workflow reutilizável: `model`, `publication`, `inline_comments`, `security`.
  Ele publica um status de commit; só bloqueia merge se exigido pela branch.
- Action Docker direta: usa `Mpaape/AurumCode@main`, exige
  `GITHUB_TOKEN`, `LLM_API_KEY`, `LLM_BASE_URL` no ambiente e evento de PR.
  Acrescenta inputs `check` e `fail-on`; não coleta CI automaticamente.
- Localmente, `.aurumcode/instructions/*.md` pode usar front matter
  `applyTo` para escopo por caminho. O fluxo remoto usa os arquivos
  explicitamente listados em `review.context`.

Sem configuração, o review já inclui análise estática determinística, contexto
de codebase limitado, resumo e diagrama Mermaid; essas capacidades funcionam
com os padrões, sem nenhum arquivo. `aurumcode fix` converte as sugestões de
uma revisão em um diff unificado aplicável.

`inline_comments: true` na configuração é cumulativo com o input do workflow;
para desligá-lo, remova-o do arquivo ou defina false e não habilite o input.
