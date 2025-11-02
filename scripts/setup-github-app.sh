#!/bin/bash
# Enterprise GitHub App Setup Script
# This script helps you set up AurumCode as a GitHub App for your organization

set -e

echo "🏢 AurumCode - Enterprise GitHub App Setup"
echo ""
echo "This script will guide you through setting up AurumCode for 100+ repos."
echo ""

# Check if running in correct directory
if [ ! -f "go.mod" ]; then
    echo "❌ Error: Please run this script from the AurumCode root directory"
    exit 1
fi

# Function to prompt for input
prompt() {
    local var_name=$1
    local prompt_text=$2
    local default_value=$3

    if [ -n "$default_value" ]; then
        read -p "$prompt_text [$default_value]: " value
        value=${value:-$default_value}
    else
        read -p "$prompt_text: " value
    fi

    eval "$var_name='$value'"
}

# Function to prompt for secret
prompt_secret() {
    local var_name=$1
    local prompt_text=$2

    read -sp "$prompt_text: " value
    echo ""
    eval "$var_name='$value'"
}

echo "Step 1: GitHub App Information"
echo "================================"
echo ""
echo "First, create a GitHub App:"
echo "  1. Go to: https://github.com/organizations/YOUR_ORG/settings/apps/new"
echo "  2. Fill in the basic information"
echo "  3. Note the App ID and generate a private key"
echo ""

prompt GITHUB_APP_ID "Enter your GitHub App ID"
prompt GITHUB_APP_PRIVATE_KEY_PATH "Path to private key PEM file" "./github-app-private-key.pem"
prompt_secret GITHUB_WEBHOOK_SECRET "Enter webhook secret (generate with: openssl rand -hex 32)"

echo ""
echo "Step 2: LLM Provider Configuration"
echo "===================================="
echo ""
echo "Choose your LLM provider:"
echo "  1. OpenAI"
echo "  2. Anthropic"
echo "  3. TOTVS DTA"
echo "  4. Other (Custom)"

prompt LLM_CHOICE "Enter choice [1-4]" "1"

case $LLM_CHOICE in
    1)
        LLM_PROVIDER="openai"
        prompt_secret OPENAI_API_KEY "Enter OpenAI API key"
        LLM_API_KEY=$OPENAI_API_KEY
        ;;
    2)
        LLM_PROVIDER="anthropic"
        prompt_secret ANTHROPIC_API_KEY "Enter Anthropic API key"
        LLM_API_KEY=$ANTHROPIC_API_KEY
        ;;
    3)
        LLM_PROVIDER="totvs"
        prompt_secret TOTVS_DTA_API_KEY "Enter TOTVS DTA API key"
        prompt TOTVS_DTA_BASE_URL "Enter TOTVS DTA base URL" "https://proxy.dta.totvs.ai"
        LLM_API_KEY=$TOTVS_DTA_API_KEY
        ;;
    *)
        prompt LLM_PROVIDER "Enter LLM provider name"
        prompt_secret LLM_API_KEY "Enter API key"
        ;;
esac

echo ""
echo "Step 3: Deployment Configuration"
echo "================================="
echo ""
echo "How will you deploy AurumCode?"
echo "  1. Docker Compose (Recommended)"
echo "  2. Kubernetes"
echo "  3. Manual (systemd/pm2)"

prompt DEPLOY_METHOD "Enter choice [1-3]" "1"

prompt PORT "Server port" "8080"
prompt DEBUG_LOGS "Enable debug logs? (true/false)" "false"

echo ""
echo "Step 4: Generating Configuration Files"
echo "======================================"

# Create .env file
cat > .env <<EOF
# GitHub App Configuration
GITHUB_APP_ID=$GITHUB_APP_ID
GITHUB_APP_PRIVATE_KEY_PATH=$GITHUB_APP_PRIVATE_KEY_PATH
GITHUB_WEBHOOK_SECRET=$GITHUB_WEBHOOK_SECRET

# LLM Provider Configuration
LLM_PROVIDER=$LLM_PROVIDER
EOF

case $LLM_CHOICE in
    1)
        echo "OPENAI_API_KEY=$LLM_API_KEY" >> .env
        ;;
    2)
        echo "ANTHROPIC_API_KEY=$LLM_API_KEY" >> .env
        ;;
    3)
        echo "TOTVS_DTA_API_KEY=$LLM_API_KEY" >> .env
        echo "TOTVS_DTA_BASE_URL=$TOTVS_DTA_BASE_URL" >> .env
        ;;
    *)
        echo "${LLM_PROVIDER}_API_KEY=$LLM_API_KEY" >> .env
        ;;
esac

cat >> .env <<EOF

# Server Configuration
PORT=$PORT
DEBUG_LOGS=$DEBUG_LOGS
EOF

echo "✅ Created .env file"

# Create organization config
mkdir -p .aurumcode
cat > .aurumcode/org-config.yml <<EOF
version: "2.0"

# Organization-wide defaults
llm:
  provider: "$LLM_PROVIDER"
  model: "gpt-4"  # Adjust as needed
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

qa:
  coverage_threshold: 80
EOF

echo "✅ Created organization config"

# Create deployment files based on method
case $DEPLOY_METHOD in
    1)
        # Docker Compose
        cat > docker-compose.prod.yml <<EOF
version: '3.8'

services:
  aurumcode:
    build:
      context: .
      dockerfile: Dockerfile
    ports:
      - "\${PORT}:8080"
    env_file:
      - .env
    volumes:
      - ./\${GITHUB_APP_PRIVATE_KEY_PATH}:/secrets/github-app-key.pem:ro
      - ./.aurumcode:/app/.aurumcode:ro
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
EOF
        echo "✅ Created docker-compose.prod.yml"
        ;;

    2)
        # Kubernetes
        mkdir -p k8s
        cat > k8s/deployment.yaml <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: aurumcode
---
apiVersion: v1
kind: Secret
metadata:
  name: aurumcode-secrets
  namespace: aurumcode
type: Opaque
stringData:
  github-app-id: "$GITHUB_APP_ID"
  webhook-secret: "$GITHUB_WEBHOOK_SECRET"
  llm-api-key: "$LLM_API_KEY"
---
apiVersion: v1
kind: Secret
metadata:
  name: github-app-key
  namespace: aurumcode
type: Opaque
data:
  private-key.pem: $(base64 < "$GITHUB_APP_PRIVATE_KEY_PATH")
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: aurumcode
  namespace: aurumcode
spec:
  replicas: 3
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
        image: aurumcode:latest
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
        - name: ${LLM_PROVIDER}_API_KEY
          valueFrom:
            secretKeyRef:
              name: aurumcode-secrets
              key: llm-api-key
        - name: GITHUB_APP_PRIVATE_KEY_PATH
          value: /secrets/private-key.pem
        - name: PORT
          value: "$PORT"
        volumeMounts:
        - name: github-app-key
          mountPath: /secrets
          readOnly: true
      volumes:
      - name: github-app-key
        secret:
          secretName: github-app-key
---
apiVersion: v1
kind: Service
metadata:
  name: aurumcode
  namespace: aurumcode
spec:
  type: LoadBalancer
  ports:
  - port: 443
    targetPort: 8080
  selector:
    app: aurumcode
EOF
        echo "✅ Created k8s/deployment.yaml"
        ;;

    3)
        # Systemd service
        cat > aurumcode.service <<EOF
[Unit]
Description=AurumCode - Automated Code Quality Platform
After=network.target

[Service]
Type=simple
User=$(whoami)
WorkingDirectory=$(pwd)
EnvironmentFile=$(pwd)/.env
ExecStart=$(pwd)/bin/aurumcode-server
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF
        echo "✅ Created aurumcode.service (copy to /etc/systemd/system/)"
        ;;
esac

echo ""
echo "Step 5: Next Steps"
echo "=================="
echo ""
echo "Configuration complete! Here's what to do next:"
echo ""

case $DEPLOY_METHOD in
    1)
        echo "1. Build and start the server:"
        echo "   docker-compose -f docker-compose.prod.yml up -d"
        echo ""
        echo "2. Check logs:"
        echo "   docker-compose -f docker-compose.prod.yml logs -f"
        ;;
    2)
        echo "1. Build and push Docker image:"
        echo "   docker build -t yourregistry/aurumcode:latest ."
        echo "   docker push yourregistry/aurumcode:latest"
        echo ""
        echo "2. Deploy to Kubernetes:"
        echo "   kubectl apply -f k8s/deployment.yaml"
        echo ""
        echo "3. Check status:"
        echo "   kubectl get pods -n aurumcode"
        ;;
    3)
        echo "1. Build the server:"
        echo "   make build"
        echo ""
        echo "2. Install systemd service:"
        echo "   sudo cp aurumcode.service /etc/systemd/system/"
        echo "   sudo systemctl daemon-reload"
        echo "   sudo systemctl enable aurumcode"
        echo "   sudo systemctl start aurumcode"
        echo ""
        echo "3. Check status:"
        echo "   sudo systemctl status aurumcode"
        ;;
esac

echo ""
echo "3. Install GitHub App in your organization:"
echo "   Go to: https://github.com/organizations/YOUR_ORG/settings/installations"
echo "   Install your GitHub App"
echo ""
echo "4. Configure organization defaults:"
echo "   Create a .github repository with:"
echo "   - .aurumcode/config.yml (organization defaults)"
echo "   - .aurumcode/prompts/ (custom prompts)"
echo "   - .aurumcode/rules/ (custom rules)"
echo ""
echo "5. Test with a PR in any repository!"
echo ""
echo "📚 Documentation:"
echo "   - Enterprise Guide: docs/ENTERPRISE_DEPLOYMENT.md"
echo "   - Usage Guide: docs/USAGE_GUIDE.md"
echo ""
echo "🎉 Setup complete! AurumCode is ready to review 100+ repos!"
