package api

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Parser parses OpenAPI specifications
type Parser struct{}

// NewParser creates a new OpenAPI parser
func NewParser() *Parser {
	return &Parser{}
}

// Parse parses an OpenAPI spec from a file
func (p *Parser) Parse(location *SpecLocation) (*OpenAPISpec, error) {
	// Read file
	content, err := os.ReadFile(location.Path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	var spec OpenAPISpec

	// Parse based on format
	if location.Format == "json" {
		if err := json.Unmarshal(content, &spec); err != nil {
			return nil, fmt.Errorf("parse JSON: %w", err)
		}
	} else {
		if err := yaml.Unmarshal(content, &spec); err != nil {
			return nil, fmt.Errorf("parse YAML: %w", err)
		}
	}

	// Validate
	if err := p.validate(&spec); err != nil {
		return nil, fmt.Errorf("validate spec: %w", err)
	}

	return &spec, nil
}

// validate checks if the spec is valid
func (p *Parser) validate(spec *OpenAPISpec) error {
	if spec.Info.Title == "" {
		return fmt.Errorf("missing required field: info.title")
	}

	if spec.Info.Version == "" {
		return fmt.Errorf("missing required field: info.version")
	}

	// Check OpenAPI version
	if spec.OpenAPI == "" {
		return fmt.Errorf("missing required field: openapi")
	}

	if !strings.HasPrefix(spec.OpenAPI, "3.") {
		return fmt.Errorf("unsupported OpenAPI version: %s (only 3.x supported)", spec.OpenAPI)
	}

	return nil
}

// GroupEndpoints groups endpoints by tag
func (p *Parser) GroupEndpoints(spec *OpenAPISpec) []EndpointGroup {
	tagMap := make(map[string][]Endpoint)

	// Process all paths
	for path, pathItem := range spec.Paths {
		operations := map[string]*Operation{
			"GET":     pathItem.Get,
			"POST":    pathItem.Post,
			"PUT":     pathItem.Put,
			"DELETE":  pathItem.Delete,
			"PATCH":   pathItem.Patch,
			"OPTIONS": pathItem.Options,
			"HEAD":    pathItem.Head,
		}

		for method, op := range operations {
			if op == nil {
				continue
			}

			endpoint := Endpoint{
				Method:      method,
				Path:        path,
				Summary:     op.Summary,
				Description: op.Description,
				Deprecated:  op.Deprecated,
			}

			// Assign to tags
			if len(op.Tags) > 0 {
				for _, tag := range op.Tags {
					tagMap[tag] = append(tagMap[tag], endpoint)
				}
			} else {
				// No tag, use "default"
				tagMap["default"] = append(tagMap["default"], endpoint)
			}
		}
	}

	// Convert to groups
	var groups []EndpointGroup
	for tag, endpoints := range tagMap {
		groups = append(groups, EndpointGroup{
			Tag:       tag,
			Endpoints: endpoints,
		})
	}

	return groups
}

// ExtractSummary extracts a summary of the API
func (p *Parser) ExtractSummary(spec *OpenAPISpec) map[string]interface{} {
	summary := make(map[string]interface{})

	summary["title"] = spec.Info.Title
	summary["version"] = spec.Info.Version
	summary["description"] = spec.Info.Description

	// Count endpoints
	endpointCount := 0
	for _, pathItem := range spec.Paths {
		if pathItem.Get != nil {
			endpointCount++
		}
		if pathItem.Post != nil {
			endpointCount++
		}
		if pathItem.Put != nil {
			endpointCount++
		}
		if pathItem.Delete != nil {
			endpointCount++
		}
		if pathItem.Patch != nil {
			endpointCount++
		}
		if pathItem.Options != nil {
			endpointCount++
		}
		if pathItem.Head != nil {
			endpointCount++
		}
	}

	summary["endpoint_count"] = endpointCount
	summary["path_count"] = len(spec.Paths)
	summary["tag_count"] = len(spec.Tags)

	// Servers
	if len(spec.Servers) > 0 {
		servers := make([]string, len(spec.Servers))
		for i, server := range spec.Servers {
			servers[i] = server.URL
		}
		summary["servers"] = servers
	}

	return summary
}
