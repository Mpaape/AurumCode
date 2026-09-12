# Qualidade e limitações

O modelo recebe o diff, as linguagens detectadas e o contexto fornecido. Deve
avaliar correção, compatibilidade, legibilidade, arquitetura, performance e
segurança, reportando problemas introduzidos pela mudança.

O filtro atual exige uma linha adicionada no lado direito, uma regra do catálogo
e campos não vazios de evidência, impacto e verificação. Isso valida localização
e estrutura, **não demonstra automaticamente que o problema existe**.
Hipóteses plausíveis ainda dependem da qualidade do modelo e do prompt.

Sugestões são opcionais e devem ser pequenas, locais e justificadas.
Pontos fortes devem descrever benefícios do código/testes, sem elogios
genéricos ao workflow ou à existência do AurumCode.

## Contexto e continuidade

Prompts, skills, documentação e contexto de CI são fornecidos em cada chamada.
A implementação atual não navega autonomamente por todo o repositório, não
executa ferramentas pelo modelo e não busca documentação na web.

Não há memória persistente dos achados ou decisões entre rodadas. Repetir uma
revisão pode produzir novos resultados. O cache local legado é por processo e
não deve ser confundido com continuidade de discussão.

## Cobertura e falhas

Não há teto arbitrário de saída enviado por padrão. Isso não significa leitura
ilimitada: o leitor local pode omitir arquivos binários ou muito grandes,
prosa não compete com código no prompt e contribuições de contexto têm limite
de tamanho. Confira os avisos e a cobertura; ausência de achados não é garantia
de revisão completa nem de ausência de defeitos.

No workflow reutilizável, o modelo recebe estados e links dos checks de CI.
Diagnosticar a causa exige evidência adicional, como logs. Falhas de autenticação,
provider ou publicação podem impedir que qualquer comentário seja enviado.
