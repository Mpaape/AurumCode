# Qualidade e limitações

O modelo recebe o diff, as linguagens detectadas e o contexto fornecido. Deve
avaliar correção, compatibilidade, legibilidade, arquitetura, performance e
segurança, reportando problemas introduzidos pela mudança.
O review pede saída estruturada ao provedor quando o perfil dele a oferece
(`json_schema` ou `json_object`; veja [Provedores de LLM](provedores.md)); o
parser continua validando a estrutura e os achados antes da publicação, inclusive
quando o provedor não tem saída estruturada. Isso evita depender apenas da
instrução textual para formar JSON. Respostas já válidas são analisadas como um objeto inteiro,
mesmo quando contêm blocos de código em campos de texto.

O filtro atual exige uma linha adicionada (`RIGHT`, numeração nova) ou removida
(`LEFT`, numeração antiga), uma regra do catálogo
e campos não vazios de evidência, impacto e verificação. Isso valida localização
e estrutura, **não demonstra automaticamente que o problema existe**.
Hipóteses plausíveis ainda dependem da qualidade do modelo e do prompt.

Se a resposta do modelo não passa na validação, o parecer do PR diz
**Inconclusivo**, não **Aprovado**. A análise determinística ainda é publicada.
O workflow reutilizável exige a revisão por modelo: nesse caso, o status
`aurumcode/review` e o job falham em vez de exibir um falso verde. Um uso local
de `review --pr` sem `--exigir-qualidade` mantém o modo determinístico opcional.

Sugestões são opcionais e devem ser pequenas, locais e justificadas.
Pontos fortes devem descrever benefícios do código/testes, sem elogios
genéricos ao workflow ou à existência do AurumCode.

## O parecer

O `--pr` publica **um parecer por pull request**, pensado para quem decide o
merge ler em dez segundos:

1. **A decisão**, num bloco de aviso do GitHub: *Aprovado*, *Aprovado com N
   observações*, *Bloqueado: N problemas precisam de correção* ou
   *Inconclusivo: esta revisão não aprova a mudança*. Quem decide é o gate
   declarado (ou, sem gate, a severidade), nunca o rótulo que o modelo se dá.
2. **Uma linha de fatos**: gate passou ou reprovou, quantos arquivos foram
   revisados e em quantas partes.
3. **Corrigir antes do merge**: cada problema bloqueante com onde, o quê, a
   regra citada e os campos que ajudam a corrigir (impacto, evidência,
   correção sugerida, verificação).
4. **Observações (não bloqueiam)**: uma linha por achado abaixo do limiar,
   inclusive os provados fora das linhas alteradas.
5. **Resumo** do modelo.
6. **Detalhes da revisão**, recolhidos: pontos fortes (até três), sugestões,
   status do CI, testes (o plano do modelo e a contagem dos testes afetados),
   limitações, cobertura, verificação, rodadas anteriores e consolidação.

A rodada seguinte **edita o mesmo parecer** em vez de publicar outro; só um
achado **bloqueante** numa linha alterada ganha comentário na linha (com
`inline_comments`), e o comentário de um achado que a rodada seguinte não
reencontra recebe a nota *Resolvido*. O relatório do terminal (`--base`)
abre com a mesma decisão e a mesma linha de fatos.

## Consolidação e preferências de apresentação

No `--pr`, o mesmo problema no mesmo trecho (mesmo arquivo, linha, lado e
regra) relatado por passes diferentes (modelo, análise determinística,
passagem de segurança, scanner, lotes) é publicado uma vez: fica a ocorrência
de origem determinística quando há uma, com a maior severidade do grupo, a
evidência de cada passe e as fontes no texto (`[fontes: analysis, model]`).
Regra diferente na mesma linha é outro problema, com uma exceção: um achado do
**modelo** no mesmo trecho em que um scanner já apontou um problema da mesma
categoria (o prefixo da regra do modelo, como `security`, aparece na regra do
scanner) é o scanner repetido pelo modelo, e é publicado uma vez, sob a regra
do scanner, com o texto do modelo como evidência. Não há corte por contagem nem
teto de achados. A consolidação muda só o que é publicado: gate, evento da
revisão e status continuam lendo os achados da execução.

`review.presentation.collapse` (ver [Configuração](configuration.md)) é a
preferência explícita de concisão: achados não bloqueantes das severidades
listadas saem numa linha de "Consolidação e preferências", nos detalhes do
parecer, que nomeia cada um (severidade, regra, arquivo e linha). Achado
bloqueante nunca é agrupado; valor desconhecido é avisado e nada é agrupado.

## Heurísticas versus defeitos provados

Um achado do catálogo determinístico ou do modelo é uma heurística: aponta um
padrão provável, não uma prova de execução de que o defeito existe. Isso
inclui `go vet`: a fonte primária do próprio `go vet`
(https://pkg.go.dev/cmd/vet) declara explicitamente que a ferramenta usa
heurísticas e que nem todo problema relatado é um defeito genuíno; o
AurumCode não trata `RuleGoVet` como prova de defeito, apenas como uma
categoria própria no catálogo.

## Comparação funcional com fontes primárias

O Aurum hoje revisa o diff completo do PR e o histórico de discussão como
contexto; entre rodadas, não repete o comentário de um achado que uma rodada
anterior já comentou (ver "Rodadas do mesmo PR" abaixo). Isso é diferente do
que outras ferramentas documentam: o CodeRabbit distingue explicitamente uma
revisão incremental (`@coderabbitai review`) de uma revisão completa
(`@coderabbitai full review`), conforme
https://docs.coderabbit.ai/reference/review-commands; o GitHub documenta que
o Copilot fornece feedback de revisão tanto para o PR quanto para alterações
ainda não commitadas, conforme
https://docs.github.com/en/copilot/concepts/agents/code-review. Este registro
é apenas uma comparação funcional do que cada ferramenta documenta fazer; o
AurumCode não afirma superioridade nem paridade medida frente a essas fontes
primárias, e nenhuma métrica de acurácia é inferida ou inventada aqui.

## Contexto e continuidade

Prompts, skills, documentação e contexto de CI são fornecidos em cada chamada;
fontes MCP declaradas em `review.context.mcp` entram como dado não confiável
com a origem `mcp:<name>/<tool>`. Com `deliberation.enabled: true`, o modelo
pode pedir ferramentas dentro de limites: ler e buscar arquivos do repositório
(`read_file`, `search_text`, `find_symbol`), o contexto de um arquivo alterado
e scanners opcionais (veja
[Deliberação](configuration.md#deliberacao-o-modelo-pede-ferramentas-dentro-de-limites)).
Fora disso, o Aurum não navega livremente pelo repositório e não busca
documentação na web.

Em `review --pr`, o Aurum consulta as reviews, comentários inline, respostas e
comentários gerais do próprio PR, com paginação. Preserva autor, IDs, commits,
datas e a localização original quando o GitHub a fornece. Esse histórico entra
no prompt como observações não confiáveis, após remoção de segredos reconhecidos.
Não cria banco, arquivo de memória, secret ou configuração adicional.

O prompt orienta a conferir correções e contestações no código atual, evitar
cobranças repetidas e explicar nova evidência antes de reabrir uma questão.
Uma aprovação ou um comentário dizendo "corrigido" não altera os filtros nem
autoriza a publicação. Exceções de um PR não viram regras globais.

Se qualquer fonte do histórico falhar, a revisão continua sobre o diff atual
com aviso no diagnóstico e no parecer publicado. A leitura compartilha o prazo
de fontes de contexto (`ProviderTimeout`); não aplica um teto próprio de linhas,
itens ou tokens. Um orçamento explícito de prompt inclui o histórico completo e
recusa quando ele não cabe, em vez de truncar a resposta do autor silenciosamente.

Isso ainda **não é revisão incremental nem um gerenciador de pendências**: o
diff continua sendo o PR completo. O Aurum não resolve threads nem edita
comentários anteriores. A decisão semântica permanece com o modelo; repetir uma
revisão pode produzir novos resultados. A leitura das três fontes não é um
snapshot transacional do GitHub. O uso local `--base` e seu cache legado não
recebem esse histórico remoto.

### Rodadas do mesmo PR

O parecer de uma rodada nova edita o da rodada anterior (o comentário do
publicador que abre com o marcador do parecer); sem histórico legível ou sem
login conhecido, publica um novo. Cada comentário de achado publicado na linha leva um marcador oculto
(`<!-- aurumcode:finding <impressão> <regra> -->`) com a impressão do achado:
a mesma de `render.FindingFingerprint`, sobre regra, caminho e o código da
linha revisada, sem o número da linha. A rodada seguinte lê os marcadores do
histórico do PR, só de comentários do próprio publicador (o login em
`AURUMCODE_PUBLISHER_LOGIN`, que o workflow reutilizável define como
`github-actions[bot]`, o autor do `github.token`); respostas dentro de uma
thread e marcadores de qualquer outro autor não contam, e sem esse login
nenhum marcador é lido e todo achado é comentado de novo. Então:

- não comenta de novo um achado já comentado, nem quando o mesmo código só
  mudou de linha; outro defeito na mesma linha (outra regra) é comentado;
- conta no parecer quantos achados não foram comentados de novo: eles
  continuam no parecer, no gate e nos status, de modo que o veredito de duas
  rodadas iguais é o mesmo;
- lista, em "Rodadas anteriores" (nos detalhes do parecer), os achados de
  rodadas anteriores que esta execução não reencontrou (corrigidos ou não
  reencontrados), e edita cada um desses comentários com a nota *Resolvido*
  e um marcador que o tira das rodadas seguintes. Um achado
  agrupado por `review.presentation.collapse` continua reportado e nunca entra
  nessa lista; uma execução inconclusiva (falha do modelo ou gate
  inconclusivo) não lista nada como resolvido. O evento da
  revisão e os status seguem só os achados desta execução, então um bug
  corrigido deixa de bloquear. Uma revisão formal `REQUEST_CHANGES` de uma
  rodada anterior continua no GitHub até uma aprovação posterior; o Aurum não
  a dispensa.

O marcador nunca é autoridade sobre um achado: decide apenas se um comentário
a mais é publicado. Uma resposta de pessoa (por exemplo, "falso positivo")
entra no prompt como contexto, mas não desliga regra; um marcador forjado pode,
no máximo, evitar a repetição de um comentário, nunca tirar o achado do
parecer, do gate ou dos status; só o publicador consegue escrevê-lo. Sem histórico legível, todo achado é
comentado. Mudança de prompt, configuração ou contexto muda as chaves de cache
e a revisão é refeita; um defeito novo é comentado e os equivalentes já
comentados, não.

O histórico aumenta o contexto enviado ao endpoint LLM já configurado: inclui
também discussões de pessoas e outros bots. Considere a política de dados do
time para esse endpoint; a remoção de segredos reconhecidos não é anonimização.

## Achado do modelo verificado contra o código

Um achado plausível e falso do modelo é o pior defeito de uma revisão: ele
bloqueia o merge com uma afirmação que o código refuta (por exemplo, "método
chamado sem guarda de nil" quando o método começa com `if c == nil { return
nil }`). Por isso todo achado do modelo passa por uma verificação adversarial
com o código real da revisão revisada, os que bloqueariam o gate primeiro
(o teto de chamadas protege o gate antes das observações). Só uma refutação
apoiada numa citação literal desse código tira o achado do parecer: um
bloqueante deixa o gate, uma observação é descartada, e os dois ficam nomeados,
com o motivo e a citação, nos detalhes do parecer, no stderr e na auditoria.
Qualquer outra resposta, ou falha da verificação, mantém o achado: a
verificação só pode tirar um achado com prova, nunca por silêncio. Sem a
revisão revisada (um `--base` fora da raiz de um checkout), nada é verificado,
tudo continua contando e a auditoria registra o motivo; o aviso no stderr só
aparece quando `review.verification` foi declarada. Achados de scanner não
passam por ela. Configuração em
[`review.verification`](configuration.md#verificacao-adversarial-dos-achados-do-modelo-reviewverification);
o caso `achado-refutado` do tutorial de revisão mostra as duas pontas.

## Cobertura e falhas

Não há teto arbitrário de saída enviado por padrão. Isso não significa leitura
ilimitada: o leitor local pode omitir arquivos binários ou muito grandes,
prosa não compete com código no prompt e contribuições de contexto têm limite
de tamanho. Confira os avisos e a cobertura; ausência de achados não é garantia
de revisão completa nem de ausência de defeitos.

No workflow reutilizável, o modelo recebe estados e links dos checks de CI
concluídos. O parecer publica estado e link como observados apenas quando vêm
desse contexto; o `status` que o próprio modelo escreve nunca vira afirmação de
CI aprovado ou reprovado e, sem check correspondente, o item sai como "estado
não verificado". Diagnosticar a causa exige evidência adicional, como logs: a
revisão não baixa logs, então uma falha sem trecho de log no contexto sai com
causa desconhecida, a explicação do modelo como hipótese não verificada,
nenhuma correção e a orientação de abrir o log do check. Quando o contexto traz
um trecho sanitizado (`excerpt`) e a evidência do modelo o cita, a correção é
publicada separando o que foi observado no log do que o modelo inferiu (ver
"Status do CI no parecer" em [Configuração](configuration.md)). Falhas de
autenticação, provider ou publicação podem impedir que qualquer comentário seja
enviado.

PRs vindos de forks não recebem os secrets do repositório por padrão, então a
revisão por modelo não roda neles sem uma configuração explícita; o baseline
determinístico continua disponível quando o workflow permitir. No CI, o parecer
é publicado com o `github.token`, limitado ao repositório, e o histórico de
discussão exige permissão de leitura.
