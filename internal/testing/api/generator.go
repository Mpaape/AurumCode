package api

import (
	"aurumcode/internal/documentation/api"
	"fmt"
	"path/filepath"
	"strings"
)

// TestGenerator generates API tests
type TestGenerator struct {
	detector *api.Detector
	parser   *api.Parser
}

// NewTestGenerator creates a new API test generator
func NewTestGenerator() *TestGenerator {
	return &TestGenerator{
		detector: api.NewDetector(),
		parser:   api.NewParser(),
	}
}

// GenerateFromOpenAPI generates API tests from OpenAPI spec
func (g *TestGenerator) GenerateFromOpenAPI(repoPath string) ([]APITest, error) {
	// Detect OpenAPI spec
	location, err := g.detector.Detect(repoPath)
	if err != nil {
		return nil, fmt.Errorf("no OpenAPI spec found: %w", err)
	}

	// Parse spec
	spec, err := g.parser.Parse(location)
	if err != nil {
		return nil, fmt.Errorf("parse spec: %w", err)
	}

	// Generate tests from endpoints
	var tests []APITest

	for path, pathItem := range spec.Paths {
		// Generate test for each method
		if pathItem.Get != nil {
			tests = append(tests, g.generateEndpointTest("GET", path, pathItem.Get, spec))
		}
		if pathItem.Post != nil {
			tests = append(tests, g.generateEndpointTest("POST", path, pathItem.Post, spec))
		}
		if pathItem.Put != nil {
			tests = append(tests, g.generateEndpointTest("PUT", path, pathItem.Put, spec))
		}
		if pathItem.Delete != nil {
			tests = append(tests, g.generateEndpointTest("DELETE", path, pathItem.Delete, spec))
		}
		if pathItem.Patch != nil {
			tests = append(tests, g.generateEndpointTest("PATCH", path, pathItem.Patch, spec))
		}
	}

	return tests, nil
}

// generateEndpointTest generates a test for an endpoint
func (g *TestGenerator) generateEndpointTest(method, path string, op *api.Operation, spec *api.OpenAPISpec) APITest {
	testName := g.generateTestName(method, path, op)

	return APITest{
		Name:        testName,
		Method:      method,
		Path:        path,
		Description: op.Summary,
		Tags:        op.Tags,
	}
}

// generateTestName generates a test name from endpoint
func (g *TestGenerator) generateTestName(method, path string, op *api.Operation) string {
	if op.OperationID != "" {
		return "Test_" + op.OperationID
	}

	// Generate from method and path
	name := "Test_" + method
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if part != "" && !strings.HasPrefix(part, "{") {
			name += "_" + strings.Title(part)
		}
	}

	return name
}

// GenerateGoTests generates Go API tests
func (g *TestGenerator) GenerateGoTests(tests []APITest, baseURL string) string {
	var sb strings.Builder

	sb.WriteString("package api_test\n\n")
	sb.WriteString("import (\n")
	sb.WriteString("\t\"net/http\"\n")
	sb.WriteString("\t\"net/http/httptest\"\n")
	sb.WriteString("\t\"testing\"\n")
	sb.WriteString(")\n\n")

	for _, test := range tests {
		sb.WriteString(g.generateGoTest(test))
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// generateGoTest generates a single Go test
func (g *TestGenerator) generateGoTest(test APITest) string {
	return fmt.Sprintf(`func %s(t *testing.T) {
	req := httptest.NewRequest("%s", "%s", nil)
	w := httptest.NewRecorder()

	// TODO: Call handler
	// handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %%d", w.Code)
	}
}`, test.Name, test.Method, test.Path)
}

// GeneratePythonTests generates Python API tests
func (g *TestGenerator) GeneratePythonTests(tests []APITest, baseURL string) string {
	var sb strings.Builder

	sb.WriteString("import pytest\n")
	sb.WriteString("import requests\n\n")
	sb.WriteString("BASE_URL = \"" + baseURL + "\"\n\n")

	for _, test := range tests {
		sb.WriteString(g.generatePythonTest(test))
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// generatePythonTest generates a single Python test
func (g *TestGenerator) generatePythonTest(test APITest) string {
	method := strings.ToLower(test.Method)
	funcName := strings.ToLower(test.Name)

	return fmt.Sprintf(`def %s():
    """Test %s %s"""
    response = requests.%s(f"{BASE_URL}%s")
    assert response.status_code == 200`, funcName, test.Method, test.Path, method, test.Path)
}

// GenerateJSTests generates JavaScript/TypeScript API tests
func (g *TestGenerator) GenerateJSTests(tests []APITest, baseURL string) string {
	var sb strings.Builder

	sb.WriteString("import { describe, it, expect } from 'vitest';\n\n")
	sb.WriteString("const BASE_URL = '" + baseURL + "';\n\n")

	for _, test := range tests {
		sb.WriteString(g.generateJSTest(test))
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// generateJSTest generates a single JavaScript test
func (g *TestGenerator) generateJSTest(test APITest) string {
	return fmt.Sprintf(`describe('%s', () => {
  it('should return 200', async () => {
    const response = await fetch(BASE_URL + '%s', {
      method: '%s'
    });
    expect(response.status).toBe(200);
  });
});`, test.Name, test.Path, test.Method)
}

// WriteTestFile writes tests to a file
func (g *TestGenerator) WriteTestFile(outputPath string, content string) error {
	dir := filepath.Dir(outputPath)
	return writeFile(dir, filepath.Base(outputPath), content)
}
