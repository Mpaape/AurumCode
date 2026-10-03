# Tutoriais

Cada tutorial é executável: os blocos de configuração são os arquivos de
`demo/tutoriais/<tutorial>/`, cada caso roda como uma fase (`run.sh <caso>`),
a saída da última execução real fica em `out/` e `run.sh --check` a compara
com `expected/`. Para criar um tutorial novo, veja
`demo/tutoriais/README.md`.

| Tutorial | Casos de uso |
|---|---|
| [Revisão de código](revisao.md) | 1. primeira revisão local (`review --base`); 2. sem provedor (análise determinística); 3. com provedor (achados que citam a regra); 4. `fix` aplicando uma sugestão; 5. revisão de PR pelo workflow reutilizável; falha: arquivo não revisado nunca conta como aprovado |
| [Skills de convenção](skills.md) | 1. skill de convenção do repositório; 2. escopo por caminho (`applyTo`); 3. seleção por linguagem e apelidos (o que existe hoje); 4. skill do repositório contra skill da política; 5. seção de skill como regra citável do gate; falha: skill referenciada que não existe |
| [Política central](politica-central.md) | 1. repositório de política e `--politica`; 2. `policy_repository` no workflow; 3. precedência por seção; 4. `analysis_data`; 5. o repositório tenta afrouxar o `fail_on` e não consegue; falha: política inválida |
| [Qualquer linguagem](qualquer-linguagem.md) | 1. repositório poliglota e contexto estrutural por gramática; 2. arquivo sem gramática, declarado; 3. binário e gerado retêm a aprovação (sem e com gate); 4. achado de política em Terraform bloqueia; 5. apelidos de linguagem (o que existe hoje); falha: extensão desconhecida ainda é revisada |
| [Benchmark de recall](benchmark.md) | 1. rodar o corpus com o provedor falso; 2. ler recall, precisão e intervalo; 3. "aprovado com defeito"; 4. adicionar um caso por PR; falha: caso sem manifest é recusado |
| [Operação](operacao.md) | 1. ambiente de desenvolvimento em container; 2. aceite selado e exit codes; 3. profiles e locks; 4. dependência Go e repin; 5. scanners por digest; 6. entrega e evidência; falha: `done` sem evidência |
