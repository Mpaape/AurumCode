# 🚀 AurumCode - Guia de Configuração Final

Este guia te ajudará a configurar o repositório para produção e regenerar toda a documentação.

## ✅ Já Completado

- ✅ 35 arquivos de documentação desatualizada removidos
- ✅ Pasta `_docs/` antiga limpa
- ✅ Hugo removido (migrado para Jekyll)
- ✅ Script de regeneração criado (`cmd/regenerate-docs/main.go`)
- ✅ GitHub Action reusável criada (`action.yml`)
- ✅ Workflow de documentação corrigido (sem Docker)

## 📋 Passos Restantes

### 1️⃣ Tornar o Repositório Público

1. Acesse: https://github.com/Mpaape/AurumCode/settings
2. Role até **"Danger Zone"** no final da página
3. Clique em **"Change visibility"**
4. Selecione **"Make public"**
5. Digite o nome do repositório para confirmar: `Mpaape/AurumCode`
6. Clique em **"I understand, change repository visibility"**

### 2️⃣ Adicionar Secrets da API (Para Features de IA)

1. Acesse: https://github.com/Mpaape/AurumCode/settings/secrets/actions
2. Clique em **"New repository secret"**
3. Adicione os seguintes secrets:

**Secret 1:**
- Name: `TOTVS_DTA_API_KEY`
- Value: `sk-123123213`
- Clique em **"Add secret"**

**Secret 2:**
- Name: `TOTVS_DTA_BASE_URL`
- Value: `https://proxy.com`
- Clique em **"Add secret"**

### 3️⃣ Configurar GitHub Pages

1. Acesse: https://github.com/Mpaape/AurumCode/settings/pages
2. Em **"Source"**, selecione:
   - **Branch:** `gh-pages`
   - **Folder:** `/ (root)`
3. Clique em **"Save"**

**Nota:** O branch `gh-pages` será criado automaticamente pelo workflow quando você rodar pela primeira vez.

### 4️⃣ Executar o Workflow de Documentação

1. Acesse: https://github.com/Mpaape/AurumCode/actions/workflows/documentation.yml
2. Clique em **"Run workflow"** (botão azul no canto direito)
3. Selecione **branch: main**
4. Clique em **"Run workflow"** (verde)
5. Aguarde a execução (5-10 minutos)

O workflow irá:
- ✅ Configurar Go 1.21
- ✅ Executar `go run cmd/regenerate-docs/main.go`
- ✅ Gerar documentação para todas as linguagens detectadas
- ✅ Configurar Ruby e Jekyll
- ✅ Compilar o site Jekyll
- ✅ Fazer deploy para `gh-pages`

### 5️⃣ Verificar o Site

Após o workflow completar:

1. Acesse: **https://mpaape.github.io/AurumCode/**
2. Verifique se a documentação foi gerada corretamente
3. Navegue pelas seções:
   - Home (Welcome page)
   - Stack
   - Architecture
   - Tutorials
   - API Reference

## 🔧 Testando Localmente (Opcional)

Se quiser testar localmente antes:

```bash
# 1. Configurar variáveis de ambiente (opcional - para IA)
export TOTVS_DTA_API_KEY=sk-XPoBopNFOW3yfGbz9dhavg
export TOTVS_DTA_BASE_URL=https://proxy.dta.totvs.ai

# 2. Regenerar documentação
go run cmd/regenerate-docs/main.go

# 3. Build Jekyll
cd docs
bundle install
bundle exec jekyll serve

# 4. Abrir no navegador
# http://localhost:4000
```

## 📊 Documentação Gerada

O script gerará documentação para:

| Linguagem | Ferramenta | Pasta de Saída |
|-----------|-----------|----------------|
| Go | gomarkdoc | `docs/go/` |
| JavaScript/TypeScript | TypeDoc | `docs/javascript/` |
| Python | pydoc-markdown | `docs/python/` |
| C# | xmldocmd | `docs/csharp/` |
| C/C++ | Doxygen + doxybook2 | `docs/cpp/` |
| Rust | rustdoc | `docs/rust/` |
| Bash | shdoc | `docs/bash/` |
| PowerShell | platyPS | `docs/powershell/` |

## 🎯 Usando AurumCode em Outros Repositórios

Outros projetos podem usar AurumCode adicionando ao workflow:

```yaml
- uses: Mpaape/AurumCode@main
  with:
    source-dir: '.'
    output-dir: '.aurumcode'
```

Ver `ACTION_USAGE.md` para mais detalhes.

## ❓ Troubleshooting

### Workflow falhou no step "Extract documentation"

**Problema:** Go não encontrou módulos ou dependências

**Solução:**
1. Verifique se `go.mod` e `go.sum` estão commitados
2. Execute localmente: `go mod tidy`
3. Commit e push

### Jekyll build falhou

**Problema:** Dependências Ruby não encontradas

**Solução:**
1. Verifique `docs/Gemfile` e `docs/_config.yml`
2. Execute localmente:
   ```bash
   cd docs
   bundle install
   bundle exec jekyll build
   ```

### Documentação não aparece no site

**Problema:** Branch `gh-pages` não foi criado

**Solução:**
1. Rode o workflow novamente
2. Verifique se o branch `gh-pages` existe
3. Configure GitHub Pages para usar branch `gh-pages`

### API de IA não funciona

**Problema:** Secrets não configurados corretamente

**Solução:**
1. Verifique se os secrets estão na aba Actions (não Codespaces ou Dependabot)
2. Confirme que os nomes estão exatos: `TOTVS_DTA_API_KEY` e `TOTVS_DTA_BASE_URL`
3. Rode o workflow novamente

## 📝 Checklist Final

- [ ] Repositório público
- [ ] Secrets adicionados (`TOTVS_DTA_API_KEY`, `TOTVS_DTA_BASE_URL`)
- [ ] GitHub Pages configurado (branch `gh-pages`)
- [ ] Workflow executado com sucesso
- [ ] Site acessível em https://mpaape.github.io/AurumCode/
- [ ] Documentação gerada para todas as linguagens

## 🎉 Pronto!

Após completar todos os passos, o AurumCode estará:
- ✅ Público e acessível
- ✅ Com documentação atualizada
- ✅ Pronto para ser usado por outros repositórios
- ✅ Com CI/CD automatizado

---

**Dúvidas?** Abra uma issue em: https://github.com/Mpaape/AurumCode/issues
