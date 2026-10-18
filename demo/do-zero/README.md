# Do zero, em camadas: três repositórios, o AurumCode e o primeiro parecer

Um projeto novo (um assistente de terminal que pergunta a um serviço de IA)
ganha o AurumCode sem copiar regra nenhuma para dentro de si. Quem decide o
que bloqueia e quem escreve as convenções são outros times, em repositórios
próprios; o serviço só aponta para eles. A história é a mesma nos dois modos:
uma mudança com dois defeitos de natureza diferente, a PR bloqueada, uma
tentativa de afrouxar que não muda nada, e a correção aprovada.

| Repositório | Quem mantém | O que fica nele | Como entra no serviço |
|---|---|---|---|
| `security-policy` (`camadas/security-policy/`) | time de segurança | o gate obrigatório (`fail_on: [error]`, inconclusivo bloqueia), o idioma do parecer, o `ignore` de arquivos gerados, a regra IA-001 (`skills/ia`) e, quando houver, as exceções aprovadas, com dono e validade | `policy_repository` no workflow; `--politica` na linha de comando |
| `devops-policy` (`camadas/devops-policy/`) | time de engenharia | as convenções PAD-001 e QA-001 (`skills/time`) e os analistas `padroes` e `qa` (`profiles.yml`); o `config.yml` fica vazio: a camada só contribui skills e analistas | `standards_repository` no workflow; `--padroes` na linha de comando |
| `assistente` (o serviço, `boilerplate/` + `camadas/servico/`) | o time do serviço | só o que é dele: idioma, comentários na linha e quais analistas rodam (`review.profiles`) | é o repositório revisado |

Por que o serviço não consegue afrouxar a segurança: `gate`, `ignore`,
`exceptions` e `rules` são sempre da política central. Se o serviço os
escrever no próprio `config.yml`, o AurumCode os descarta com um aviso que diz
o que foi descartado e por quê (`a política central decide sozinha`), e o
bloqueio é o mesmo. Na PR nem isso seria lido: a revisão usa a configuração da
`main` do serviço, nunca a da branch. A camada de padrões também não decide:
um `config.yml` dela com qualquer seção é recusado antes de o modelo ser
chamado. Para destravar, quem muda é o time de segurança, por uma PR no
`security-policy`. O passo 10 do roteiro mostra isso acontecendo.

Quando a regra vem da camada de padrões, o achado traz a camada
(`[padrões de engenharia]`), e os detalhes do parecer listam as regras citadas
com os arquivos e linhas. O roteiro ainda imprime, lendo essas regras e os
repositórios clonados, de que repositório veio cada uma:

- a **chave fixa no código** (linha 10) é achada pela verificação
  determinística do AurumCode, cobrada pelo gate do `security-policy`;
- **executar a resposta do modelo** (`exec(codigo)`, linha 36) nenhuma regra
  fixa reconhece: quem acha é o modelo, lendo o código à luz da regra IA-001 do
  `security-policy`, e é o que reprova (`severity: error`);
- a opção lida **filtrando `sys.argv` na mão** (PAD-001) e o **comportamento
  novo sem teste** (QA-001) vêm dos analistas do `devops-policy`: orientam
  (`severity: warning`) e seguem abertos depois da correção, sem bloquear.

O analista embutido `seguranca` é uma passada do modelo com foco em segurança,
não a varredura determinística de segredos: ligar ou desligar o analista não a
muda.

## 1. Com um agente de IA configurando (Claude Code, Codex)

```sh
bash demo/do-zero/run.sh ia                    # mock: modelo = fixture, sem rede
bash demo/do-zero/run.sh ia --modo real --repo meu-assistente   # GitHub e modelo reais
```

Cria os dois repositórios centrais, cria o projeto em `~/aurum-poc/<nome>`
(só o boilerplate, commitado), registra o AurumCode como servidor MCP `aurum`
em `.mcp.json` (pela imagem do produto, já recebendo as duas camadas por
`AURUMCODE_POLICY` e `AURUMCODE_STANDARDS`; fora do git) e imprime o pedido
para colar no agente, que segue
[Instalar com ajuda da IA](../../docs/tutorials/instalacao-ia.md): pergunta uma
coisa por vez e aponta o serviço para os dois repositórios (`policy_repository`
e `standards_repository`, cada um fixado em SHA). Com a configuração na
`main`, os próximos passos impressos incluem a varredura inicial: o comando
`aurumcode review --completo --seguranca` na mesma imagem e com as duas
camadas, para revisar uma vez o código que já existia. Depois:

```sh
bash demo/do-zero/run.sh ia-defeito     # a mudança com os dois defeitos, na branch feature
# GitHub real: "abra a PR sem corrigir" -> parecer Bloqueado, merge travado;
#   depois "leia o parecer, confirme com o Aurum e corrija" -> o mesmo parecer vira Aprovado
# mock: "antes de abrir a PR, pergunte ao Aurum (aurum_gate, base main) e corrija"
bash demo/do-zero/run.sh ia-correcao    # se faltar tempo para o agente corrigir
bash demo/do-zero/run.sh limpar         # apaga .estado/ (os três repositórios) e o projeto criado por este script
```

## 2. Roteiro guiado, passo a passo

```sh
bash demo/do-zero/run.sh                     # mock: tudo local, sem rede, sem segredo
bash demo/do-zero/run.sh --auto              # sem pausas; grava out/roteiro-mock.log (evidência)
bash demo/do-zero/run.sh --modo real --repo meu-assistente --mostrar   # só lista o que o modo real faria
bash demo/do-zero/run.sh --modo real --repo meu-assistente             # na sua conta
```

| Passo | O que acontece | mock | real |
|---|---|---|---|
| 1 Projeto | `boilerplate/` vira o projeto (chave lida do ambiente) | local | local |
| 2 Git e remoto | `git init`, primeiro commit, `push` | remoto bare em `.estado/remotos/` | `gh repo create … --push` |
| 3 Segurança | `camadas/security-policy/` vira o repositório `security-policy`: gate, idioma, `ignore`, IA-001 | remoto bare | `gh repo create` |
| 4 Engenharia | `camadas/devops-policy/` vira o repositório `devops-policy`: PAD-001, QA-001, analistas `padroes` e `qa` | remoto bare | `gh repo create` |
| 5 O serviço aponta | `.aurumcode/config.yml` só com o que é do serviço; workflow com `policy_repository`/`standards_repository` fixados na SHA da `main` de cada um | commit e push | commit e push |
| 6 Secrets e tokens | `gh secret set LLM_API_KEY / LLM_BASE_URL`, `gh variable set LLM_MODEL`; com `--privado`, os dois tokens de leitura | só mostra | executa com o seu ambiente |
| 7 Defeito | `etapas/defeito.py.modelo`: chave fixa (linha 10), `--executar` lido na mão (32), sem teste (34), `exec` da resposta (36) | | |
| 8 Agente | `aurumcode mcp` com as duas camadas: `aurum_gate` reprova, `aurum_explain` explica; o mesmo pela CLI (`--politica`, `--padroes`) | modelo = fixture | modelo do ambiente |
| 9 PR bloqueada | o workflow faz o checkout dos dois centrais e publica: "Bloqueado: 2 problemas", comentário na linha, correção aplicável, cada achado com analista e camada | GitHub falso em 127.0.0.1 | PR real, espera o workflow |
| 10 Afrouxar | `camadas/servico/afrouxa.yml` no `config.yml` do serviço: gate e `ignore` descartados com aviso, bloqueio igual | | |
| 11 Correção | `etapas/correcao.py`; o parecer é atualizado: aprovado, com as observações da engenharia abertas | | |
| 12 Resumo | o que ficou e como continuar | | |

### Modo real

Pré-requisitos: `gh` autenticado e `LLM_API_KEY`, `LLM_BASE_URL` (e
`LLM_MODEL`, se o serviço exigir) exportados. `--mostrar` imprime, sem rede e
sem `gh`, os nomes dos três repositórios e cada comando que o roteiro
executaria: `NOME-security-policy`, `NOME-devops-policy` e `NOME`, derivados
de `--repo` (`dono/NOME` escolhe a conta ou organização), para não colidir com
nada seu. Os repositórios nascem públicos; `--privado` cria os três privados e
exige, no ambiente, `AURUMCODE_POLICY_TOKEN` e `AURUMCODE_STANDARDS_TOKEN`
(um token fine-grained com `Contents: read` só nos dois repositórios centrais;
pode ser o mesmo valor), que o roteiro cadastra como secrets do serviço. Com
repositórios públicos o checkout das duas camadas usa o `github.token` do
próprio workflow e nenhum token extra é preciso. `--ref` fixa a versão do
AurumCode no workflow (padrão: `v2.0.0` se a tag existir, senão `main`). O
script nunca apaga um repositório seu.

### Evidência e conferência

`run.sh --modo mock --auto` grava a saída em `out/roteiro-mock.log` (com os
valores voláteis, como SHAs e caminhos, normalizados) e a imagem do produto em
`out/.imagem`; `run.sh --check` confere, sem docker, que cada linha de
`expected/roteiro-mock.txt` está nesse registro e que a imagem é desta árvore.
`tests/acceptance/AUR-614.sh` usa essa evidência, o modo real montado
(`--mostrar`) e `limpar`.

As skills e a config da PR vêm da branch base (a PR nunca traz a própria regra):
no mock, o GitHub falso serve o conteúdo da `main` de cada remoto local
(`GITHUB_FALSO_ARVORE`), e as duas camadas são montadas fora da árvore do
serviço, como o workflow faz. Peças reutilizadas: `demo/tutoriais/_lib`
(imagem do produto, GitHub falso, `review --pr`) e o cliente MCP de teste de
`demo/tutoriais/agente`. O tutorial executável com os mesmos três papéis em
repositórios de exemplo menores está em
[camadas](../../docs/tutorials/camadas.md).
