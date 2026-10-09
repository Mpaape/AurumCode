# Do zero: um projeto, o AurumCode e o primeiro parecer

Duas formas de mostrar o AurumCode num projeto novo (um assistente de terminal
que pergunta a um serviço de IA), com a mesma história: uma mudança com dois
defeitos de natureza diferente, a PR bloqueada e depois aprovada.

Três analistas revisam cada mudança, cada um com o seu foco, e cada achado
sai marcado com quem o encontrou: **segurança** (embutido no AurumCode),
**padrões** do time de dev e **QA** (os dois em `aurum/profiles.yml`). Bloqueia o merge
a regra escrita com `severity: error` (por um time ou pela segurança) e o
achado de ferramenta; o resto orienta.

### Receita: um time cria o seu analista

1. **Foco** — em `.aurumcode/profiles.yml`, um item com `name` (como o analista
   aparece no parecer, `[perfil qa]`), `emphasis` (o foco em poucas palavras),
   `families` (`quality`, `security` ou `performance`: a família dos achados) e
   `instructions` (o que ele deve olhar). `version` é a versão do texto do analista.
2. **Regra** — em `.aurumcode/skills/<nome>/SKILL.md`, um `## ` por regra, com
   `severity: error` (bloqueia) ou `warning` (orienta). O analista cita a regra.
3. **Ativar** — `review.profiles: [seguranca, padroes, qa]` no `config.yml`.

Exemplo do QA: `aurum/profiles.yml` (analista `qa`) e `aurum/skills/time/SKILL.md`
(regra `QA-001 Comportamento novo tem teste`, `severity: warning`).

- a **chave fixa no código** é achada pela camada de ferramentas, que vê o valor real;
- **executar a resposta do modelo** (`exec(codigo)`) o catálogo de regras fixas do Aurum não
  reconhece: quem acha é o modelo, lendo o código à luz da regra IA-001 do time
  (`aurum/skills/ia/SKILL.md`, em Markdown).

## 1. Com um agente de IA configurando (Claude Code, Codex)

```sh
bash demo/do-zero/run.sh ia                    # mock: modelo = fixture, sem rede
bash demo/do-zero/run.sh ia --modo real --repo meu-assistente   # GitHub e modelo reais
```

Cria o projeto em `~/aurum-poc/<nome>` (só o boilerplate, commitado), registra o
AurumCode como servidor MCP `aurum` em `.mcp.json` (pela imagem do produto; fora
do git) e imprime o pedido para colar no agente, que segue
[Instalar com ajuda da IA](../../docs/tutorials/instalacao-ia.md): pergunta uma
coisa por vez e configura `.aurumcode/`, a skill e o workflow. Depois:

```sh
bash demo/do-zero/run.sh ia-defeito     # a mudança com os dois defeitos, na branch feature
# GitHub real: "abra a PR sem corrigir" -> parecer Bloqueado, merge travado;
#   depois "leia o parecer, confirme com o Aurum e corrija" -> o mesmo parecer vira Aprovado
# mock: "antes de abrir a PR, pergunte ao Aurum (aurum_gate, base main) e corrija"
bash demo/do-zero/run.sh ia-correcao    # se faltar tempo para o agente corrigir
bash demo/do-zero/run.sh limpar         # apaga o projeto criado por este script
```

No mock, peça a skill exatamente como o script sugere (`.aurumcode/skills/ia/SKILL.md`,
regra `## IA-001 Resposta do modelo nunca e executada`): o modelo falso cita essa regra.

## 2. Roteiro guiado, passo a passo

```sh
bash demo/do-zero/run.sh                     # mock: tudo local, sem rede, sem segredo
bash demo/do-zero/run.sh --auto              # sem pausas (gravação)
bash demo/do-zero/run.sh --modo real --repo meu-assistente
```

| Passo | O que acontece | mock | real |
|---|---|---|---|
| 1 Projeto | `boilerplate/` vira o projeto (chave lida do ambiente) | local | local |
| 2 Git e remoto | `git init`, primeiro commit, `push` | remoto bare em `.estado/` | `gh repo create … --push` |
| 3 AurumCode | `.aurumcode/config.yml` (pt-BR, gate), skill `ia` (IA-001, IA-002), workflow | commit local | commit e push |
| 4 Secrets | `gh secret set LLM_API_KEY / LLM_BASE_URL`, `gh variable set LLM_MODEL` | só mostra | executa com o seu ambiente |
| 5 Defeitos | `etapas/defeito.py.modelo`: chave fixa (linha 10) e `exec` da resposta (linha 36) | | |
| 6 Agente | `aurumcode mcp`: `aurum_gate` reprova, `aurum_explain` explica; o mesmo pela CLI | modelo = fixture | modelo do ambiente |
| 7 PR bloqueada | parecer "Bloqueado: 2 problemas", comentário na linha e correção aplicável | GitHub falso em 127.0.0.1 | PR real, espera o workflow |
| 8 Correção | `etapas/correcao.py`; o parecer é editado: aprovado | | |
| 9 Resumo | o que ficou e como continuar | | |

Modo real: `gh` autenticado e `LLM_API_KEY`, `LLM_BASE_URL` (e `LLM_MODEL`, se o
serviço exigir) exportados. O repositório é criado público (`--privado` para
privado) e nunca é apagado pelo script. `--ref` fixa a versão do AurumCode no
workflow (padrão: `v2.0.0` se a tag existir, senão `main`).

As skills e a config da PR vêm da branch base (a PR nunca traz a própria regra):
no mock, o GitHub falso serve o conteúdo do remoto local (`GITHUB_FALSO_ARVORE`).
Peças reutilizadas: `demo/tutoriais/_lib` (imagem do produto, GitHub falso,
`review --pr`) e o cliente MCP de teste de `demo/tutoriais/agente`.
