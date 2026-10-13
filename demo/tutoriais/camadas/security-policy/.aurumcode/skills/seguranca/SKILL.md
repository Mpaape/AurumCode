---
name: seguranca
version: 1
languages: [python]
---
Regras do time de seguranca, iguais para todos os servicos. Cada `## ` abaixo
e uma regra citavel pelo AurumCode; `severity: error` reprova o merge.

## SEG-001 Verificacao de TLS nunca e desligada
severity: error
Toda chamada HTTPS valida o certificado do servidor: `verify=False`,
`ssl.CERT_NONE` ou `check_hostname = False` nunca entram no codigo, nem
"so em homologacao".
