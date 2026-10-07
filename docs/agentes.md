# Aurum no seu agente de código

Quem programa com um agente de IA (Claude Code, Codex, Cursor) commita muito
e rápido. Sem isto, o parecer do Aurum só chega na PR, depois do push. Com
`aurumcode mcp`, o agente pergunta ao **mesmo gate** antes de commitar e
corrige o que vier, sem esperar o CI.

- **Mesmo gate.** Cada pergunta do agente é uma sessão `review --base`, a
  mesma da CLI e do CI: mesma política central, mesmas skills, mesma redação
  de segredos, mesmo fail-closed. O servidor não tem caminho de gate próprio.
- **Só leitura.** Nenhuma ferramenta escreve arquivo, abre PR ou muda
  política. O agente escolhe só a ref de comparação (`base`) e os caminhos
  que quer consultar; nenhum argumento desliga regra ou aponta outra política.
- **Nunca "passa" por omissão.** Sem provedor de modelo, com o provedor
  falhando, com um scanner que não concluiu ou estourando o tempo, a resposta
  é `inconclusive`, nunca `pass`.

## As ferramentas

| Ferramenta | Argumentos | O que responde |
|---|---|---|
| `aurum_gate` | `base` (ref) | `decision` (`pass`, `fail`, `inconclusive`), `reason`, `exit_code`, os achados que bloqueiam e o próximo passo |
| `aurum_review` | `base` (ref) | o mesmo, mais todos os achados (arquivo, linha, severidade, regra citada, origem, evidência, sugestão) e o relatório da CLI |
| `aurum_rules` | `paths` (lista) | as skills (da política e do repositório) e as regras citáveis que valem para esses caminhos |
| `aurum_explain` | `finding_id` | o detalhe de um achado da última resposta e como corrigir |

`base` é a branch em que a mudança vai entrar (`main`, `origin/main`) ou um
commit (`HEAD~1`). A revisão cobre os commits entre `base` e `HEAD`: o agente
commita na branch de trabalho, pergunta, corrige e só então faz push. Nada
entre `base` e `HEAD` (o agente perguntou antes de commitar) é `inconclusive`
com motivo `empty_change`, nunca `pass`. Para
barrar antes do commit existir, use o [hook de pre-commit](#hook-de-pre-commit-sem-agente).

## Instalar

O servidor é o próprio binário `aurumcode` (subcomando `mcp`), sem
dependência nova. Ele lê a configuração do diretório em que o agente o inicia
(a raiz do repositório) e o provedor de modelo do ambiente, como a CLI:
`LLM_API_KEY` e `LLM_BASE_URL` (endpoint compatível com OpenAI, local ou
remoto) e, opcionalmente, `LLM_MODEL`. A política central, quando houver, vem
de `AURUMCODE_POLICY`, definida por quem configura o agente, nunca pelo agente.

Limites do operador, no comando que inicia o servidor:

```bash
aurumcode mcp --tempo 10m --limite 0.50
```

`--tempo` limita cada chamada (estourou: `inconclusive`, motivo
`time_limit`); `--limite` é o teto de custo em USD de cada revisão, o mesmo de
`review --limite`. Os limites da configuração (`deliberation`, scanners) valem
como na CLI.

Nunca grave a chave do provedor em arquivo versionado: deixe-a no ambiente do
shell que abre o agente e referencie a variável.

### Claude Code

```bash
claude mcp add aurum -- aurumcode mcp
```

Para versionar a configuração do time, use o escopo de projeto
(`claude mcp add --scope project ...`), que grava um `.mcp.json` na raiz:

```json
{
  "mcpServers": {
    "aurum": {
      "command": "aurumcode",
      "args": ["mcp"],
      "env": {
        "LLM_API_KEY": "${LLM_API_KEY}",
        "LLM_BASE_URL": "${LLM_BASE_URL}"
      }
    }
  }
}
```

### Codex

No `~/.codex/config.toml`:

```toml
[mcp_servers.aurum]
command = "aurumcode"
args = ["mcp"]
```

O provedor (`LLM_API_KEY`, `LLM_BASE_URL`) vem do ambiente em que o Codex
inicia o servidor; repasse as variáveis como a documentação do Codex indica,
sem escrever a chave no arquivo.

### Cursor

Em `.cursor/mcp.json` (projeto) ou `~/.cursor/mcp.json` (usuário):

```json
{
  "mcpServers": {
    "aurum": {
      "command": "aurumcode",
      "args": ["mcp"],
      "env": {
        "LLM_API_KEY": "${env:LLM_API_KEY}",
        "LLM_BASE_URL": "${env:LLM_BASE_URL}"
      }
    }
  }
}
```

### Pela imagem de contêiner

Sem o binário instalado, o comando do servidor pode ser a imagem do produto,
com o repositório montado e stdin aberto (`-i`):

```bash
docker run -i --rm -v "$PWD:/work" -w /work -e LLM_API_KEY -e LLM_BASE_URL \
  --entrypoint /app/aurumcode <imagem-do-aurumcode> mcp
```

## A skill do agente

`.agents/skills/aurum-review/SKILL.md` está no formato padrão de Agent
Skills (front matter `name` e `description`). Ela ensina o agente a chamar
`aurum_gate` antes de cada commit ou push, corrigir cada achado que bloqueia
(com `aurum_explain` quando precisar), tratar `inconclusive` como "não
passou" e **nunca** editar `.aurumcode/`, a política, skills ou exceções para
passar. Copie o diretório para onde o seu agente lê skills (no Claude Code,
`.claude/skills/aurum-review/`).

## Hook de pre-commit, sem agente

`.agents/skills/aurum-review/hooks/pre-commit` faz a mesma pergunta antes do
commit existir: fotografa o que está no índice (o que vai ser commitado),
revisa numa worktree temporária com `review --base` contra a base de merge e
barra o commit se o gate não passar. Não edita seus arquivos e remove a
worktree ao terminar.

```bash
cp .agents/skills/aurum-review/hooks/pre-commit .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit
```

`AURUM_BASE` escolhe a branch de destino (padrão `main`) e `AURUMCODE` o
comando (padrão `aurumcode` do `PATH`).

## Na prática

O tutorial [Aurum no seu agente](tutorials/agente.md) executa tudo isto com
um cliente MCP de teste: o agente consulta o gate, explica o achado, corrige
e passa; o resultado é igual ao de `review --base`; a violação de uma skill
do repositório reprova; sem provedor o gate é inconclusivo; e o hook barra
o commit.

## Limites conhecidos

- A revisão vê commits (`base..HEAD`), não edições ainda não commitadas;
  para o índice, use o hook.
- Um servidor por repositório: ele revisa o diretório em que foi iniciado.
- Não é servidor remoto: só stdio, local. Ferramentas que escrevem, abrem PR
  ou mudam política estão fora de escopo.
