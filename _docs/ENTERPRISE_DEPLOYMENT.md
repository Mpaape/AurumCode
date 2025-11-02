---
layout: default
title: ENTERPRISE DEPLOYMENT
parent: Documentation
nav_order: 7
---

# AurumCode Enterprise Deployment Guide

## 🏢 Deploying AurumCode at Scale (100+ Repositories)

This guide shows you how to deploy AurumCode across your entire enterprise GitHub organization with minimal per-repo configuration.

---

## Option 1: GitHub App Deployment (RECOMMENDED) ⭐

**Best for:** GitHub Enterprise Cloud/Server with centralized management

### Architecture

```
GitHub Organization (100+ repos)
    ↓
GitHub App (AurumCode)
    ↓
Central AurumCode Server (self-hosted or cloud)
    ↓
Reviews all repos automatically
```

### Benefits
- ✅ **Install once, works everywhere** - No per-repo webhook setup
- ✅ **Centralized configuration** - Organization-level defaults
- ✅ **Per-repo overrides** - Repos can customize via `.aurumcode/config.yml`
- ✅ **Automatic discovery** - New repos get reviewed automatically
- ✅ **Fine-grained permissions** - GitHub App security model
- ✅ **Professional** - Appears in GitHub Marketplace

### Step-by-Step Setup

#### 1. Create GitHub App

```bash
# Navigate to your organization settings
https://github.com/organizations/YOUR_ORG/settings/apps/new

# Fill in these fields:
GitHub App name: AurumCode
Homepage URL: https://aurumcode.yourcompany.com
Webhook URL: https://aurumcode.yourcompany.com/webhook
Webhook secret: <generate strong secret>

# Permissions (Repository):
- Pull requests: Read & write
- Contents: Read & write
- Issues: Read & write
- Commit statuses: Read & write
- Metadata: Read-only

# Subscribe to events:
- Pull request
- Push
- Pull request review

# Where can this GitHub App be installed?
→ Only on this account
```

#### 2. Deploy Central AurumCode Server

**Option A: Docker (Recommended)**

```bash
# Clone AurumCode
git clone https://github.com/Mpaape/AurumCode.git
cd AurumCode

# Create production config
cat > .env <<EOF
# GitHub App credentials
GITHUB_APP_ID=123456
GITHUB_APP_PRIVATE_KEY_PATH=/secrets/github-app-private-key.pem
GITHUB_WEBHOOK_SECRET=your_webhook_secret_here

# LLM Provider (choose one)
OPENAI_API_KEY=sk-...
# OR
ANTHROPIC_API_KEY=sk-ant-...
# OR
TOTVS_DTA_API_KEY=sk-...
TOTVS_DTA_BASE_URL=https://proxy.dta.totvs.ai

# Server config
PORT=8080
DEBUG_LOGS=false
EOF

# Build and run
docker-compose up -d

# Or use Docker directly
docker build -t aurumcode:latest .
docker run -d \
  --name aurumcode \
  -p 8080:8080 \
  --env-file .env \
  -v /path/to/github-app-key.pem:/secrets/github-app-private-key.pem:ro \
  aurumcode:latest
```

**Option B: Kubernetes (Enterprise)**

```yaml
# k8s/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: aurumcode
  namespace: devtools
spec:
  replicas: 3  # Scale as needed
  selector:
    matchLabels:
      app: aurumcode
  template:
    metadata:
      labels:
        app: aurumcode
    spec:
      containers:
      - name: aurumcode
        image: ghcr.io/yourorg/aurumcode:latest
        ports:
        - containerPort: 8080
        env:
        - name: GITHUB_APP_ID
          valueFrom:
            secretKeyRef:
              name: aurumcode-secrets
              key: github-app-id
        - name: GITHUB_WEBHOOK_SECRET
          valueFrom:
            secretKeyRef:
              name: aurumcode-secrets
              key: webhook-secret
        - name: OPENAI_API_KEY
          valueFrom:
            secretKeyRef:
              name: aurumcode-secrets
              key: openai-api-key
        volumeMounts:
        - name: github-app-key
          mountPath: /secrets
          readOnly: true
      volumes:
      - name: github-app-key
        secret:
          secretName: github-app-private-key
---
apiVersion: v1
kind: Service
metadata:
  name: aurumcode
  namespace: devtools
spec:
  type: LoadBalancer
  ports:
  - port: 443
    targetPort: 8080
  selector:
    app: aurumcode
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: aurumcode
  namespace: devtools
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  tls:
  - hosts:
    - aurumcode.yourcompany.com
    secretName: aurumcode-tls
  rules:
  - host: aurumcode.yourcompany.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: aurumcode
            port:
              number: 8080
```

#### 3. Install GitHub App in Organization

```bash
# Go to your GitHub App settings
https://github.com/organizations/YOUR_ORG/settings/installations

# Click "Install App"
# Select "All repositories" or specific repos
# Approve installation
```

#### 4. Configure Organization-Level Defaults

Create centralized config repository:

```bash
# Create special repo: .github
# This repo provides defaults for all repos in your org

mkdir -p .github/.aurumcode
cat > .github/.aurumcode/config.yml <<EOF
version: "2.0"

# Organization-wide defaults
llm:
  provider: "openai"
  model: "gpt-4"
  temperature: 0.3
  max_tokens: 4000

features:
  code_review: true
  code_review_on_push: false
  documentation: true
  qa_testing: true

github:
  post_comments: true
  set_status: true

outputs:
  comment_on_pr: true
  update_docs: true
  generate_tests: true
  deploy_site: false

# Coverage thresholds
qa:
  coverage_threshold: 80

# Custom prompts (organization-wide)
prompts:
  code_review: ".aurumcode/prompts/review.md"

# Custom rules (organization-wide)
rules:
  - ".aurumcode/rules/security.yml"
  - ".aurumcode/rules/code-standards.yml"
EOF
```

#### 5. Per-Repo Configuration (Optional)

Individual repos can override defaults:

```yaml
# my-service/.aurumcode/config.yml
version: "2.0"

# Override only what's different
llm:
  model: "gpt-4-turbo"  # This repo uses faster model

qa:
  coverage_threshold: 90  # Higher standards for this repo

features:
  qa_testing: false  # Disable QA for this legacy repo
```

#### 6. Test the Setup

```bash
# Create test PR in any repo
git checkout -b test-aurumcode
echo "test" >> README.md
git add README.md
git commit -m "test: Testing AurumCode integration"
git push origin test-aurumcode

# Create PR on GitHub
# AurumCode should automatically review it!
```

---

## Option 2: Organization Workflow Templates

**Best for:** Simpler GitHub Actions-based approach

### Architecture

```
GitHub Organization
    ↓
Organization workflow templates
    ↓
Each repo uses template (one-time setup)
    ↓
GitHub Actions runs AurumCode
```

### Setup

#### 1. Create Organization Template Repository

```bash
# Create special repo: .github
mkdir -p .github/workflow-templates

cat > .github/workflow-templates/aurumcode.yml <<EOF
name: AurumCode - Automated Code Quality

on:
  pull_request:
    types: [opened, synchronize, reopened]
  push:
    branches: [main, master]

permissions:
  contents: write
  pull-requests: write
  issues: write

jobs:
  aurumcode:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Run AurumCode
        uses: Mpaape/AurumCode@main
        with:
          mode: 'all'
          llm_provider: 'openai'
          llm_api_key: \${{ secrets.ORG_OPENAI_API_KEY }}
          github_token: \${{ secrets.GITHUB_TOKEN }}
          coverage_threshold: '80'
EOF

cat > .github/workflow-templates/aurumcode.properties.json <<EOF
{
  "name": "AurumCode Quality Automation",
  "description": "Automated code review, documentation, and testing",
  "iconName": "code-review",
  "categories": ["Code Quality", "Automation"],
  "filePatterns": ["*.yml", "*.yaml"]
}
EOF
```

#### 2. Configure Organization Secrets

```bash
# Go to: https://github.com/organizations/YOUR_ORG/settings/secrets/actions

# Add organization secret:
Name: ORG_OPENAI_API_KEY
Value: sk-...

# Make available to all repos or specific repos
```

#### 3. Enable in Each Repo (One Command)

```bash
# In each repo, developers run:
gh workflow init

# Select "AurumCode Quality Automation" template
# Commit and push

# Or use GitHub CLI to automate:
for repo in $(gh repo list YOUR_ORG --limit 1000 --json name -q '.[].name'); do
  gh workflow enable aurumcode.yml --repo YOUR_ORG/$repo
done
```

---

## Option 3: Self-Hosted Server + Organization Webhooks

**Best for:** GitHub Enterprise Server (on-premise)

### Setup

#### 1. Deploy Central Server

```bash
# Same as GitHub App deployment (Docker or K8s)
docker-compose up -d
```

#### 2. Create Organization Webhook

```bash
# Go to: https://github.com/organizations/YOUR_ORG/settings/hooks

Payload URL: https://aurumcode.yourcompany.com/webhook
Content type: application/json
Secret: <your webhook secret>

Events:
☑ Pull requests
☑ Pushes
☑ Pull request reviews

Active: ☑
```

#### 3. Configure Default Settings

Same as GitHub App approach - use `.github` repo for defaults.

---

## Option 4: API-Based Integration

**Best for:** Existing CI/CD pipelines (Jenkins, GitLab, etc.)

### Architecture

```
Your CI/CD (Jenkins, GitLab CI, etc.)
    ↓
HTTP POST to AurumCode API
    ↓
AurumCode processes
    ↓
Returns results as JSON
```

### API Endpoints

Create REST API wrapper:

```go
// cmd/api/main.go
package main

import (
    "encoding/json"
    "net/http"
    // ... AurumCode imports
)

type ReviewRequest struct {
    RepoURL    string `json:"repo_url"`
    PRNumber   int    `json:"pr_number"`
    CommitSHA  string `json:"commit_sha"`
    APIKey     string `json:"api_key"`
}

func handleReviewRequest(w http.ResponseWriter, r *http.Request) {
    var req ReviewRequest
    json.NewDecoder(r.Body).Decode(&req)

    // Validate API key
    if !isValidAPIKey(req.APIKey) {
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
        return
    }

    // Run AurumCode review
    result := runReview(req)

    json.NewEncoder(w).Encode(result)
}

func main() {
    http.HandleFunc("/api/v1/review", handleReviewRequest)
    http.ListenAndServe(":8080", nil)
}
```

### Usage in Jenkins

```groovy
// Jenkinsfile
pipeline {
    agent any

    stages {
        stage('Code Review') {
            steps {
                script {
                    def response = httpRequest(
                        url: 'https://aurumcode.yourcompany.com/api/v1/review',
                        httpMode: 'POST',
                        contentType: 'APPLICATION_JSON',
                        requestBody: """
                        {
                            "repo_url": "${env.GIT_URL}",
                            "pr_number": ${env.CHANGE_ID},
                            "commit_sha": "${env.GIT_COMMIT}",
                            "api_key": "${env.AURUMCODE_API_KEY}"
                        }
                        """
                    )

                    def result = readJSON text: response.content

                    if (result.issues.size() > 0) {
                        error "Code review found ${result.issues.size()} issues"
                    }
                }
            }
        }
    }
}
```

---

## 📊 Comparison: Which Option to Choose?

| Feature | GitHub App | Workflow Templates | Self-Hosted Webhooks | API Integration |
|---------|-----------|-------------------|---------------------|----------------|
| **Setup Complexity** | Medium | Easy | Medium | Hard |
| **Per-Repo Config** | None (auto) | One-time | None (auto) | Per-pipeline |
| **Centralized Mgmt** | ✅ Excellent | ⚠️ Limited | ✅ Excellent | ⚠️ Limited |
| **GitHub Enterprise** | ✅ Best | ✅ Good | ✅ Best | ✅ Good |
| **Non-GitHub** | ❌ No | ❌ No | ❌ No | ✅ Yes |
| **Scaling (100+ repos)** | ✅ Excellent | ⚠️ OK | ✅ Excellent | ⚠️ OK |
| **GitHub Marketplace** | ✅ Yes | ✅ Yes | ❌ No | ❌ No |
| **Cost** | Server only | GitHub Actions minutes | Server only | Variable |

### Recommendations by Use Case

1. **GitHub Enterprise Cloud (100+ repos)**: Use **GitHub App** ⭐
2. **GitHub.com open source**: Use **Workflow Templates**
3. **GitHub Enterprise Server (on-prem)**: Use **Self-Hosted + Org Webhooks**
4. **Multi-platform (GitHub + GitLab)**: Use **API Integration**

---

## 🚀 Quick Start for 100+ Repos

**Fastest path to deploy across 100 repositories:**

### 1. GitHub App (30 minutes)

```bash
# 1. Create GitHub App (5 min)
- Go to: https://github.com/organizations/YOUR_ORG/settings/apps/new
- Configure permissions and webhooks

# 2. Deploy server (10 min)
git clone https://github.com/Mpaape/AurumCode.git
cd AurumCode
docker-compose up -d

# 3. Install in organization (2 min)
- Install GitHub App
- Select "All repositories"

# 4. Configure org defaults (5 min)
- Create .github repo with config

# 5. Test (5 min)
- Create test PR
- Verify review appears

# 6. Announce to teams (5 min)
- Send email with documentation
```

**Done! All 100+ repos now have automated code review** ✅

### 2. Alternative: Workflow Templates (45 minutes)

```bash
# 1. Create workflow template (10 min)
- Create .github repo
- Add workflow template

# 2. Configure org secrets (5 min)
- Add API keys as org secrets

# 3. Bulk enable (30 min)
# Use GitHub CLI to enable in all repos
for repo in $(gh repo list YOUR_ORG --limit 1000 --json name -q '.[].name'); do
  echo "Enabling AurumCode in $repo"

  # Clone repo
  gh repo clone YOUR_ORG/$repo temp-$repo
  cd temp-$repo

  # Create workflow
  mkdir -p .github/workflows
  cp ~/.github/workflow-templates/aurumcode.yml .github/workflows/

  # Commit and push
  git add .github/workflows/aurumcode.yml
  git commit -m "ci: Add AurumCode automation"
  git push

  cd ..
  rm -rf temp-$repo
done
```

---

## 🔧 Advanced Configuration

### Multi-Tenant Configuration

Support different configs for different teams:

```yaml
# .github/.aurumcode/config.yml (organization defaults)
version: "2.0"

# Global defaults
llm:
  provider: "openai"
  model: "gpt-4"

# Team overrides (based on repo name patterns)
team_configs:
  backend-*:
    qa:
      coverage_threshold: 90

  frontend-*:
    qa:
      coverage_threshold: 80

  legacy-*:
    features:
      qa_testing: false
```

### Cost Management

```yaml
# Centralized cost control
cost_management:
  monthly_budget_usd: 5000
  per_repo_limit_usd: 50

  # Prioritize critical repos
  priority_repos:
    - "core-api"
    - "payment-service"

  # Use cheaper model for less critical repos
  low_priority_model: "gpt-3.5-turbo"
```

### Monitoring & Dashboards

```bash
# Expose Prometheus metrics
docker run -d \
  -p 9090:9090 \
  -v ./prometheus.yml:/etc/prometheus/prometheus.yml \
  prom/prometheus

# Grafana dashboard
docker run -d \
  -p 3000:3000 \
  grafana/grafana
```

---

## 📞 Support & Troubleshooting

### Common Issues

**Issue: "GitHub App not receiving webhooks"**
```bash
# Check webhook deliveries
https://github.com/organizations/YOUR_ORG/settings/apps/YOUR_APP

# Check server logs
docker logs aurumcode

# Test webhook manually
curl -X POST https://aurumcode.yourcompany.com/webhook \
  -H "Content-Type: application/json" \
  -d '{"test": true}'
```

**Issue: "API rate limits"**
```bash
# GitHub Apps get much higher rate limits (5000/hour)
# Check your rate limit:
curl -H "Authorization: Bearer $GITHUB_TOKEN" \
  https://api.github.com/rate_limit
```

**Issue: "Different configs per team"**
```bash
# Use CODEOWNERS to enforce config per team
# .github/CODEOWNERS
.aurumcode/config.yml @platform-team
```

---

## 🎓 Training Materials

### For Developers

- [Quick Start Guide](USAGE_GUIDE.md)
- [Configuration Reference](CONFIG_REFERENCE.md)
- [Best Practices](BEST_PRACTICES.md)

### For Admins

- [Installation Guide](INSTALLATION.md)
- [Monitoring Guide](MONITORING.md)
- [Troubleshooting](TROUBLESHOOTING.md)

---

## 📊 Success Metrics

Track these metrics to measure AurumCode effectiveness:

```yaml
metrics:
  # Quality metrics
  - Issues found per PR (target: > 5)
  - Code quality score (target: > 7.5/10)
  - Documentation coverage (target: > 80%)

  # Adoption metrics
  - Repos using AurumCode (target: 100%)
  - PRs reviewed (target: > 90%)
  - Issues fixed (target: > 70%)

  # Efficiency metrics
  - Review time (target: < 30 seconds)
  - False positive rate (target: < 10%)
  - Developer satisfaction (target: > 4/5)
```

---

**Next Steps:**
1. Choose deployment option (GitHub App recommended)
2. Deploy central server
3. Configure organization defaults
4. Test with 5-10 repos
5. Roll out to remaining repos
6. Monitor and iterate

For questions: [GitHub Issues](https://github.com/Mpaape/AurumCode/issues)
