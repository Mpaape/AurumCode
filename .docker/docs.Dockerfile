# Multi-stage Dockerfile for AurumCode Documentation Generation
# Contains all tools needed for multi-language documentation extraction

FROM ubuntu:22.04 AS base

# Prevent interactive prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# Install base dependencies
RUN apt-get update && apt-get install -y \
    curl \
    wget \
    git \
    build-essential \
    ca-certificates \
    gnupg \
    lsb-release \
    software-properties-common \
    && rm -rf /var/lib/apt/lists/*

# ============================================================================
# Go Installation (for godoc, go doc)
# ============================================================================
FROM base AS golang
RUN wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz \
    && tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz \
    && rm go1.21.5.linux-amd64.tar.gz

ENV PATH="/usr/local/go/bin:$PATH"
ENV GOPATH="/go"
ENV PATH="$GOPATH/bin:$PATH"

# Install Go documentation tools
RUN go install golang.org/x/tools/cmd/godoc@latest \
    && go install github.com/princjef/gomarkdoc/cmd/gomarkdoc@latest

# ============================================================================
# Node.js Installation (for TypeScript/JavaScript docs)
# ============================================================================
FROM golang AS nodejs
RUN curl -fsSL https://deb.nodesource.com/setup_20.x | bash - \
    && apt-get install -y nodejs \
    && rm -rf /var/lib/apt/lists/*

# Install Node.js documentation tools
RUN npm install -g \
    typedoc \
    jsdoc \
    documentation

# ============================================================================
# Python Installation (for pydoc, sphinx)
# ============================================================================
FROM nodejs AS python
RUN apt-get update && apt-get install -y \
    python3 \
    python3-pip \
    python3-dev \
    && rm -rf /var/lib/apt/lists/*

# Install Python documentation tools
RUN pip3 install --no-cache-dir \
    pydoc-markdown \
    sphinx \
    sphinx-rtd-theme \
    mkdocs \
    mkdocs-material

# ============================================================================
# .NET Installation (for C# documentation)
# ============================================================================
FROM python AS dotnet
RUN wget https://packages.microsoft.com/config/ubuntu/22.04/packages-microsoft-prod.deb -O packages-microsoft-prod.deb \
    && dpkg -i packages-microsoft-prod.deb \
    && rm packages-microsoft-prod.deb \
    && apt-get update \
    && apt-get install -y dotnet-sdk-8.0 \
    && rm -rf /var/lib/apt/lists/*

# Install C# documentation tools
RUN dotnet tool install --global xmldocmd \
    && dotnet tool install --global dotnet-doc

ENV PATH="$PATH:/root/.dotnet/tools"

# ============================================================================
# Java Installation (for Javadoc)
# ============================================================================
FROM dotnet AS java
RUN apt-get update && apt-get install -y \
    openjdk-17-jdk \
    maven \
    && rm -rf /var/lib/apt/lists/*

ENV JAVA_HOME=/usr/lib/jvm/java-17-openjdk-amd64
ENV PATH="$JAVA_HOME/bin:$PATH"

# ============================================================================
# C/C++ Tools (for Doxygen)
# ============================================================================
FROM java AS cpp
RUN apt-get update && apt-get install -y \
    doxygen \
    graphviz \
    && rm -rf /var/lib/apt/lists/*

# Install doxybook2 for markdown conversion
RUN wget https://github.com/matusnovak/doxybook2/releases/download/v1.4.0/doxybook2-linux-amd64-v1.4.0.zip \
    && unzip doxybook2-linux-amd64-v1.4.0.zip -d /usr/local/bin \
    && chmod +x /usr/local/bin/doxybook2 \
    && rm doxybook2-linux-amd64-v1.4.0.zip

# ============================================================================
# Rust Installation (for cargo doc)
# ============================================================================
FROM cpp AS rust
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
ENV PATH="/root/.cargo/bin:$PATH"

# ============================================================================
# PowerShell Installation (for platyPS)
# ============================================================================
FROM rust AS powershell
RUN wget -q https://packages.microsoft.com/config/ubuntu/22.04/packages-microsoft-prod.deb \
    && dpkg -i packages-microsoft-prod.deb \
    && apt-get update \
    && apt-get install -y powershell \
    && rm packages-microsoft-prod.deb \
    && rm -rf /var/lib/apt/lists/*

# Install PowerShell documentation module
RUN pwsh -Command "Install-Module -Name platyPS -Force -Scope CurrentUser"

# ============================================================================
# Ruby & Jekyll (for site building)
# ============================================================================
FROM powershell AS jekyll
RUN apt-get update && apt-get install -y \
    ruby-full \
    rubygems \
    && rm -rf /var/lib/apt/lists/*

RUN gem install bundler jekyll jekyll-seo-tag jekyll-sitemap

# ============================================================================
# Final Stage - Combine all tools
# ============================================================================
FROM jekyll AS final

# Set working directory
WORKDIR /workspace

# Create documentation output directory
RUN mkdir -p /workspace/docs

# Copy Go binary and tools
COPY --from=golang /usr/local/go /usr/local/go
COPY --from=golang /go /go
ENV PATH="/usr/local/go/bin:/go/bin:$PATH"
ENV GOPATH="/go"

# Ensure all tools are in PATH
ENV PATH="$PATH:/root/.dotnet/tools:/root/.cargo/bin"

# Verify installations (this helps debugging)
RUN echo "=== Tool Versions ===" \
    && go version || echo "Go not found" \
    && node --version || echo "Node not found" \
    && python3 --version || echo "Python not found" \
    && dotnet --version || echo ".NET not found" \
    && java -version || echo "Java not found" \
    && doxygen --version || echo "Doxygen not found" \
    && cargo --version || echo "Rust not found" \
    && pwsh -Version || echo "PowerShell not found" \
    && bundle --version || echo "Bundler not found" \
    && echo "===================="

# Default command
CMD ["/bin/bash"]
