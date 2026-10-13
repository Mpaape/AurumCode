---
name: padroes
version: 1
paths: ["**/*.py"]
---
Convencoes do time de engenharia, iguais para todos os servicos. Cada `## `
abaixo e uma regra citavel; `severity: warning` orienta sem bloquear o merge.

## PAD-001 Argumentos de linha de comando com argparse
severity: warning
Opcao de linha de comando e lida com `argparse` (ajuda, validacao e erro
claros), nunca filtrando `sys.argv` na mao.
