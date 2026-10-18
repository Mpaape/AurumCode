---
name: teste-prova-a-mudanca
version: 1
paths: ["cmd/**", "internal/**", "pkg/**", "scripts/**"]
---
O teste de uma mudança prova a mudança. Cada `## ` abaixo é uma regra citável
(`teste-prova-a-mudanca#<slug do titulo>`).

## TEST-001 Comportamento novo tem teste que afirma o resultado certo
severity: warning
Cada comportamento que a mudança acrescenta ou corrige tem um teste que afirma
o resultado esperado, não só a ausência do resultado antigo nem só
`err != nil`. Um caminho novo de erro, de fallback ou de configuração conta
como comportamento.

Violação: o conserto troca o título "Aprovado" por "Bloqueado", e o teste só
verifica que "Aprovado" não aparece; um título "Inconclusivo" errado também
passaria.

Correção: o teste exige "Bloqueado" e a contagem certa.

## TEST-002 O teste falha se a mudança for desfeita
severity: warning
Um teste que continua passando com a correção revertida não prova nada da
mudança. Aponte o teste cujo cenário não exercita o caminho alterado, por
exemplo um teste que roda só com `changelog=false` quando o conserto está no
caminho `changelog=true`.

Correção: um caso que entra no caminho alterado e afirma o resultado dele.
