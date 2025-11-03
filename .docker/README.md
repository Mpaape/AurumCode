# AurumCode Documentation Docker Container

This Docker container includes all tools necessary for multi-language documentation generation.

## Included Tools

### Programming Languages
- **Go 1.21** - godoc, gomarkdoc
- **Node.js 20** - TypeDoc, JSDoc, documentation
- **Python 3** - pydoc-markdown, Sphinx, MkDocs
- **.NET 8.0** - xmldocmd, dotnet-doc
- **Java 17** - Javadoc, Maven
- **Rust** - cargo doc, rustdoc
- **PowerShell 7** - platyPS

### Documentation Generators
- **Doxygen** - C/C++ documentation
- **Doxybook2** - Doxygen to Markdown converter
- **Jekyll** - Static site generator
- **Bundler** - Ruby dependency management

## Building the Container

```bash
docker build -f .docker/docs.Dockerfile -t aurumcode-docs:latest .
```

## Running the Container

### Interactive Shell
```bash
docker run -it --rm \
  -v $(pwd):/workspace \
  -w /workspace \
  aurumcode-docs:latest \
  /bin/bash
```

### Generate Documentation
```bash
docker run --rm \
  -v $(pwd):/workspace \
  -w /workspace \
  aurumcode-docs:latest \
  bash -c "
    # Run your documentation pipeline here
    ./scripts/generate-docs.sh
  "
```

### Build Jekyll Site
```bash
docker run --rm \
  -v $(pwd)/docs:/workspace/docs \
  -w /workspace/docs \
  aurumcode-docs:latest \
  bash -c "bundle install && bundle exec jekyll build"
```

## Container Size

The final container is approximately 4-5 GB due to the inclusion of all language toolchains. For production use, consider creating language-specific containers.

## Caching

The Dockerfile uses multi-stage builds for efficient caching. When updating tools, only affected layers will be rebuilt.

## GitHub Actions

This container is automatically built and used in the `.github/workflows/documentation.yml` workflow for CI/CD documentation generation.
