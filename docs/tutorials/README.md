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
| [O gate de política](gate.md) | 1. `fail_on` por severidade; 2. inconclusivo por provedor ausente; 3. inconclusivo por cobertura parcial; 4. inconclusivo por SAST falhando; 5. inconclusivo por `analysis_data`; 6. `gate.sources` e a origem de cada achado; 7. status `aurumcode/policy-gate` e exit codes; 8. o repositório tenta afrouxar; falha: `gate.sources` inválido |
| [Exceções aprovadas](excecoes.md) | 1. exceção válida com dono e validade; 2. exceção vencida; 3. a exceção cobre só o achado exato; 4. exceção do repositório ignorada sob política; falha: exceção sem dono |
| [Auditoria e SARIF](auditoria-sarif.md) | 1. auditoria campo a campo; 2. SARIF; 3. revisão inconclusiva; 4. canário de redação; 5. upload para o Code Scanning pelo chamador (conferência); falha: auditoria não gravada |
| [Reaproveitamento](reaproveitamento.md) | 1. mesmo SHA, política e modelo; 2. `--base` e `--pr` (não demonstrado); 3. modelo mudou; 4. política mudou; 5. cache degradado; falha: sem `AURUMCODE_CACHE_DIR` |
