package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Generator generates Markdown documentation from OpenAPI specs
type Generator struct {
	includeDeprecated bool
	includeServers    bool
}

// NewGenerator creates a new Markdown generator
func NewGenerator() *Generator {
	return &Generator{
		includeDeprecated: true,
		includeServers:    true,
	}
}

// WithDeprecated controls whether deprecated endpoints are included
func (g *Generator) WithDeprecated(include bool) *Generator {
	g.includeDeprecated = include
	return g
}

// WithServers controls whether server information is included
func (g *Generator) WithServers(include bool) *Generator {
	g.includeServers = include
	return g
}

// Generate generates Markdown documentation
func (g *Generator) Generate(spec *OpenAPISpec) string {
	var sb strings.Builder

	// Title and version
	sb.WriteString(fmt.Sprintf("# %s\n\n", spec.Info.Title))
	sb.WriteString(fmt.Sprintf("**Version:** %s\n\n", spec.Info.Version))

	// Description
	if spec.Info.Description != "" {
		sb.WriteString(fmt.Sprintf("%s\n\n", spec.Info.Description))
	}

	// Contact
	if spec.Info.Contact.Name != "" || spec.Info.Contact.Email != "" || spec.Info.Contact.URL != "" {
		sb.WriteString("## Contact\n\n")
		if spec.Info.Contact.Name != "" {
			sb.WriteString(fmt.Sprintf("**Name:** %s\n\n", spec.Info.Contact.Name))
		}
		if spec.Info.Contact.Email != "" {
			sb.WriteString(fmt.Sprintf("**Email:** %s\n\n", spec.Info.Contact.Email))
		}
		if spec.Info.Contact.URL != "" {
			sb.WriteString(fmt.Sprintf("**URL:** %s\n\n", spec.Info.Contact.URL))
		}
	}

	// Servers
	if g.includeServers && len(spec.Servers) > 0 {
		sb.WriteString("## Servers\n\n")
		for _, server := range spec.Servers {
			sb.WriteString(fmt.Sprintf("- **%s**", server.URL))
			if server.Description != "" {
				sb.WriteString(fmt.Sprintf(" - %s", server.Description))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Tags
	if len(spec.Tags) > 0 {
		sb.WriteString("## Tags\n\n")
		for _, tag := range spec.Tags {
			sb.WriteString(fmt.Sprintf("- **%s**", tag.Name))
			if tag.Description != "" {
				sb.WriteString(fmt.Sprintf(": %s", tag.Description))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Endpoints grouped by tag
	parser := NewParser()
	groups := parser.GroupEndpoints(spec)

	sb.WriteString("## Endpoints\n\n")

	for _, group := range groups {
		// Group header
		sb.WriteString(fmt.Sprintf("### %s\n\n", strings.Title(group.Tag)))

		// Endpoints in this group
		for _, endpoint := range group.Endpoints {
			// Skip deprecated if configured
			if endpoint.Deprecated && !g.includeDeprecated {
				continue
			}

			g.writeEndpoint(&sb, endpoint)
		}

		sb.WriteString("\n")
	}

	return sb.String()
}

// writeEndpoint writes a single endpoint to the builder
func (g *Generator) writeEndpoint(sb *strings.Builder, endpoint Endpoint) {
	// Method and path
	sb.WriteString(fmt.Sprintf("#### `%s %s`\n\n", endpoint.Method, endpoint.Path))

	// Deprecated badge
	if endpoint.Deprecated {
		sb.WriteString("**⚠️ DEPRECATED**\n\n")
	}

	// Summary
	if endpoint.Summary != "" {
		sb.WriteString(fmt.Sprintf("%s\n\n", endpoint.Summary))
	}

	// Description
	if endpoint.Description != "" && endpoint.Description != endpoint.Summary {
		sb.WriteString(fmt.Sprintf("%s\n\n", endpoint.Description))
	}
}

// GenerateToFile generates and writes documentation to a file
func (g *Generator) GenerateToFile(spec *OpenAPISpec, outputPath string) error {
	content := g.Generate(spec)

	// Ensure directory exists
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// GenerateAPI is a convenience function that detects, parses, and generates API docs
func GenerateAPI(repoPath string) error {
	// Detect OpenAPI spec
	detector := NewDetector()
	location, err := detector.Detect(repoPath)
	if err != nil {
		// No spec found, that's okay - just return
		return nil
	}

	// Parse spec
	parser := NewParser()
	spec, err := parser.Parse(location)
	if err != nil {
		return fmt.Errorf("parse spec: %w", err)
	}

	// Generate documentation
	generator := NewGenerator()
	outputPath := filepath.Join(repoPath, "docs", "API.md")

	if err := generator.GenerateToFile(spec, outputPath); err != nil {
		return fmt.Errorf("generate docs: %w", err)
	}

	return nil
}
