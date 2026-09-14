# Qualidade e limitações

O modelo recebe o diff, as linguagens detectadas e o contexto fornecido. Deve
avaliar correção, compatibilidade, legibilidade, arquitetura, performance e
segurança, reportando problemas introduzidos pela mudança.

O filtro atual exige uma linha adicionada (`RIGHT`, numeração nova) ou removida
(`LEFT`, numeração antiga), uma regra do catálogo
e campos não vazios de evidência, impacto e verificação. Isso valida localização
e estrutura, **não demonstra automaticamente que o problema existe**.
Hipóteses plausíveis ainda dependem da qualidade do modelo e do prompt.

Sugestões são opcionais e devem ser pequenas, locais e justificadas.
Pontos fortes devem descrever benefícios do código/testes, sem elogios
genéricos ao workflow ou à existência do AurumCode.

## Heurísticas versus defeitos provados

Um achado do catálogo determinístico ou do modelo é uma heurística: aponta um
padrão provável, não uma prova de execução de que o defeito existe. `go vet`
é a exceção que carrega prova de compilação/análise estática real, e por
isso é identificado separadamente no catálogo (`RuleGoVet`). A comparação
funcional com ferramentas dedicadas (linters, SAST, SCA) registra apenas o
que cada uma cobre; o AurumCode não afirma superioridade nem paridade medida
frente a essas fontes primárias.

## Contexto e continuidade

Prompts, skills, documentação e contexto de CI são fornecidos em cada chamada.
A implementação atual não navega autonomamente por todo o repositório, não
executa ferramentas pelo modelo e não busca documentação na web.

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
diff continua sendo o PR completo. O Aurum não resolve threads, não atualiza
comentários anteriores e não garante deduplicação. A decisão semântica permanece
com o modelo; repetir uma revisão pode produzir novos resultados. A leitura das
três fontes não é um snapshot transacional do GitHub. O uso local `--base` e seu
cache legado não recebem esse histórico remoto.

O histórico aumenta o contexto enviado ao endpoint LLM já configurado: inclui
também discussões de pessoas e outros bots. Considere a política de dados do
time para esse endpoint; a remoção de segredos reconhecidos não é anonimização.

## Cobertura e falhas

Não há teto arbitrário de saída enviado por padrão. Isso não significa leitura
ilimitada: o leitor local pode omitir arquivos binários ou muito grandes,
prosa não compete com código no prompt e contribuições de contexto têm limite
de tamanho. Confira os avisos e a cobertura; ausência de achados não é garantia
de revisão completa nem de ausência de defeitos.

No workflow reutilizável, o modelo recebe estados e links dos checks de CI.
Diagnosticar a causa exige evidência adicional, como logs. Falhas de autenticação,
provider ou publicação podem impedir que qualquer comentário seja enviado.
