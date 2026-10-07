---
name: erros
version: 1
paths: ["**/*.go"]
---
Convencao de erros do time. Cada `## ` abaixo e uma regra citavel.

## ERR-001 Erro nunca ignorado
severity: error
Todo erro retornado e tratado ou devolvido com contexto; nunca descartado.
