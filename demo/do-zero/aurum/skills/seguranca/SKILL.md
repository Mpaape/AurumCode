---
name: seguranca
version: 1
paths: ["**/*.py"]
---
Regras de segurança do time. Cada `## ` abaixo é uma regra citável pelo
AurumCode; `severity: error` reprova o merge.

## SEG-001 Segredo nunca no codigo
severity: error
Chave de API, senha ou token nunca aparece como literal no código nem no
histórico: vem do ambiente (`os.environ`) ou do cofre de secrets do CI.
