# Do zero: um projeto, o AurumCode e o primeiro parecer

POC interativa no terminal: um projeto novo (um assistente de terminal que
pergunta a um serviço de IA), o AurumCode configurado passo a passo e uma PR
reprovada e depois aprovada pelas mesmas regras. Cada passo mostra o comando,
espera o ⏎ e imprime o que aconteceu.

```sh
bash demo/do-zero/run.sh                     # mock: tudo local, sem rede, sem segredo
bash demo/do-zero/run.sh --auto              # sem pausas (gravação)
bash demo/do-zero/run.sh --modo real --repo meu-assistente   # na sua conta do GitHub
bash demo/do-zero/run.sh limpar
```

| Passo | O que acontece | mock | real |
|---|---|---|---|
| 1 Projeto | `boilerplate/` vira o projeto (chave lida do ambiente) | local | local |
| 2 Git e remoto | `git init`, primeiro commit, `push` | remoto bare em `.estado/` | `gh repo create … --push` |
| 3 AurumCode | `.aurumcode/config.yml` (pt-BR, gate), skill `seguranca` (SEG-001), workflow `@v2.0.0` | commit local | commit e push |
| 4 Secrets | `gh secret set LLM_API_KEY / LLM_BASE_URL`, `gh variable set LLM_MODEL` | só mostra | executa com o seu ambiente |
| 5 Defeito | branch `feature` troca a leitura do ambiente por uma chave fixa | | |
| 6 Agente | `aurumcode mcp`: `aurum_gate` reprova, `aurum_explain` explica; o mesmo pela CLI | modelo = fixture | modelo do ambiente |
| 7 PR bloqueada | `review --pr` publica o parecer e a correção na linha | GitHub falso em 127.0.0.1 | PR real, espera o workflow |
| 8 Correção | a chave volta ao ambiente; o parecer é editado: aprovado | | |
| 9 Resumo | o que ficou e como continuar | | |

No modo real: `gh` autenticado, `LLM_API_KEY` e `LLM_BASE_URL` (e `LLM_MODEL`
se o serviço exigir) exportados. O repositório é criado público (`--privado`
para privado) e nunca é apagado pelo script. `--ref` fixa a versão do AurumCode
no workflow (padrão: `v2.0.0` se a tag existir, senão `main`).

Peças reutilizadas: `demo/tutoriais/_lib` (imagem do produto, GitHub falso,
`review --pr`) e o cliente MCP de teste de `demo/tutoriais/agente`.
