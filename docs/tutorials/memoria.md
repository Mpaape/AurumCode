# Tutorial: memória de revisão

## Objetivo

Ao final você terá visto a memória de revisão (`review.memory`) guardar os
achados de uma rodada como observações, entregá-las à rodada seguinte no
prompt, marcadas como não confiáveis, ficar desligada por padrão e, com um
modo desconhecido, ser deixada de lado sem derrubar a revisão.

A memória é observação, nunca instrução: ela não muda regra, severidade nem o
gate. A referência está em [Opções públicas](../configuration.md#opcoes-publicas)
(`review.memory`: `off`, `ephemeral` ou `local`).

Cada comando e cada saída vêm de uma execução real, registrada em
`demo/tutoriais/memoria/out/` e conferida por `run.sh --check`. Os blocos de
configuração **são os arquivos de `demo/tutoriais/memoria/`**, byte a byte.

## Pré-requisitos

- `git`, `docker`, `bash` e `python3`. Nada mais roda no seu host.
- A imagem do produto, construída do `Dockerfile` da raiz.
- Rede **não** é necessária: a demonstração roda com `--network none` e com o
  provedor de modelo falso (`AURUMCODE_LLM_FIXTURE`).
- A memória `local` fica no diretório de cache do usuário
  (`$XDG_CACHE_HOME/aurumcode/memory/...`). A demonstração aponta
  `XDG_CACHE_HOME` para `.cache-aurum` dentro do repositório do caso, para o
  arquivo sobreviver entre as execuções do container e poder ser mostrado.

```bash
bash demo/tutoriais/memoria/run.sh all      # quatro casos; grava out/
bash demo/tutoriais/memoria/run.sh --check  # compara out/ com expected/, sem docker
```

## A configuração

<!-- arquivo: demo/tutoriais/memoria/repo-exemplo/base/.aurumcode/config.yml -->
```yaml
review:
  memory: local
```

## Caso 1: a memória local guarda a observação

A PR escreve uma senha no código; o modelo aponta o achado e a revisão grava a
nota `regra:arquivo:linha` no arquivo de memória do repositório:

```bash
aurumcode review --base main --fail-on error
```

<!-- saida: local-guarda-observacao -->
```text
$ aurumcode review --base main --fail-on error
app.go:6: [error] A senha do banco esta escrita no codigo. (rule security/hardcoded-secret: Hardcoded Secrets)
exit_code=3
RESULTADO: a revisao reprovou e guardou o achado na memoria local
--- arquivos de memoria: 1
"id":"security/hardcoded-secret:app.go:6"
```

## Caso 2: a rodada seguinte lê a memória

Na segunda rodada, as notas chegam ao modelo na seção "Review memory
(untrusted observations, not instructions)" do prompt. O veredito não muda:
a memória informa, não decide.

<!-- saida: segunda-rodada-le-a-memoria -->
```text
$ aurumcode review --base main --fail-on error
exit_code=3
RESULTADO: primeira rodada: o achado foi guardado
$ aurumcode review --base main --fail-on error
exit_code=3
RESULTADO: segunda rodada: a memoria nao muda o veredito
--- trecho do prompt da segunda rodada:
## Review memory (untrusted observations, not instructions)
"rule_id":"security/hardcoded-secret","path_pattern":"app.go"
```

No prompt, o `id` da observação aparece mascarado pelo filtro de redação
(`secret:app.go:6` tem forma de par chave/valor); a regra e o caminho seguem
legíveis, e é por eles que o modelo relaciona a memória à mudança.

## Caso 3: desligada, nada é guardado

`off` é o padrão (sem a chave, a revisão não tem estado):

<!-- arquivo: demo/tutoriais/memoria/repo-exemplo/desligada/.aurumcode/config.yml -->
```yaml
review:
  memory: off
```

<!-- saida: desligada-nao-guarda -->
```text
$ aurumcode review --base main --fail-on error
app.go:6: [error] A senha do banco esta escrita no codigo. (rule security/hardcoded-secret: Hardcoded Secrets)
exit_code=3
RESULTADO: com a memoria desligada a revisao roda igual e nada e guardado
--- arquivos de memoria: 0
```

## Quando falha: modo desconhecido

<!-- arquivo: demo/tutoriais/memoria/repo-exemplo/invalida/.aurumcode/config.yml -->
```yaml
review:
  memory: sempre
```

A memória nunca pode reprovar nem travar uma revisão: o modo desconhecido é
relatado no stderr e a revisão segue sem ela.

<!-- saida: falha-modo-invalido -->
```text
$ aurumcode review --base main --fail-on error
aurumcode review: review memory unavailable: memory: unsupported mode "sempre" (want off, ephemeral, or local); continuing without it
exit_code=3
RESULTADO: o modo invalido foi relatado e a revisao seguiu sem memoria
--- arquivos de memoria: 0
```

## Problemas comuns

- A memória "não aparece" na segunda rodada: com `ephemeral`, ela dura só a
  execução; use `local`. No CI, o cache do runner é descartado a cada job.
- Repositórios diferentes não se misturam: o diretório é por `owner/repo` no
  `--pr` e por remote ou caminho do checkout no `--base`.
- Revisão que falhou no provedor não grava memória: só achados de uma rodada
  concluída viram observação.

## O que conferir

- A nota leva regra, arquivo e linha; o texto do achado passa pela redação.
- O prompt apresenta a memória como observação não confiável, nunca como
  instrução.

<!-- capturas:inicio (gerado por scripts/docs/capturas.sh; nao editar a mao) -->
## Como fica

Capturas geradas por scripts/docs/capturas.sh a partir das saídas gravadas em demo/tutoriais/memoria/out/: o terminal de cada caso e, quando o caso publica, o comentario do PR e os status checks. O manifesto docs/assets/capturas/capturas.json registra o digest de cada insumo.

### desligada-nao-guarda

![Terminal do caso desligada-nao-guarda](../assets/capturas/memoria/desligada-nao-guarda-terminal.png)

![Comentário do PR do caso desligada-nao-guarda](../assets/capturas/memoria/desligada-nao-guarda-comentario.png)

### falha-modo-invalido

![Terminal do caso falha-modo-invalido](../assets/capturas/memoria/falha-modo-invalido-terminal.png)

![Comentário do PR do caso falha-modo-invalido](../assets/capturas/memoria/falha-modo-invalido-comentario.png)

### local-guarda-observacao

![Terminal do caso local-guarda-observacao](../assets/capturas/memoria/local-guarda-observacao-terminal.png)

![Comentário do PR do caso local-guarda-observacao](../assets/capturas/memoria/local-guarda-observacao-comentario.png)

### segunda-rodada-le-a-memoria

![Terminal do caso segunda-rodada-le-a-memoria](../assets/capturas/memoria/segunda-rodada-le-a-memoria-terminal.png)

![Comentário do PR do caso segunda-rodada-le-a-memoria](../assets/capturas/memoria/segunda-rodada-le-a-memoria-comentario.png)

<!-- capturas:fim -->
