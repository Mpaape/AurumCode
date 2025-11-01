package mock

import (
	"fmt"
	"strings"
)

// MockGenerator generates mocks for interfaces
type MockGenerator struct{}

// NewMockGenerator creates a new mock generator
func NewMockGenerator() *MockGenerator {
	return &MockGenerator{}
}

// GenerateGoMock generates a Go mock
func (m *MockGenerator) GenerateGoMock(interfaceName string, methods []Method) string {
	var sb strings.Builder

	mockName := "Mock" + interfaceName

	// Struct definition
	sb.WriteString(fmt.Sprintf("type %s struct {\n", mockName))
	for _, method := range methods {
		sb.WriteString(fmt.Sprintf("\t%sFunc func%s\n", method.Name, method.Signature))
	}
	sb.WriteString("}\n\n")

	// Method implementations
	for _, method := range methods {
		sb.WriteString(m.generateGoMethod(mockName, method))
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// generateGoMethod generates a single Go method
func (m *MockGenerator) generateGoMethod(mockName string, method Method) string {
	return fmt.Sprintf(`func (m *%s) %s%s {
	if m.%sFunc != nil {
		return m.%sFunc%s
	}
	%s
}`, mockName, method.Name, method.Signature, method.Name, method.Name, method.CallSignature, method.DefaultReturn)
}

// GeneratePythonMock generates a Python mock
func (m *MockGenerator) GeneratePythonMock(className string, methods []Method) string {
	var sb strings.Builder

	mockName := "Mock" + className

	sb.WriteString(fmt.Sprintf("class %s:\n", mockName))
	sb.WriteString("    \"\"\"Mock for " + className + "\"\"\"\n\n")

	sb.WriteString("    def __init__(self):\n")
	for _, method := range methods {
		sb.WriteString(fmt.Sprintf("        self.%s_called = False\n", method.Name))
	}
	sb.WriteString("\n")

	for _, method := range methods {
		sb.WriteString(m.generatePythonMethod(method))
		sb.WriteString("\n")
	}

	return sb.String()
}

// generatePythonMethod generates a single Python method
func (m *MockGenerator) generatePythonMethod(method Method) string {
	return fmt.Sprintf(`    def %s(self, *args, **kwargs):
        self.%s_called = True
        return None
`, method.Name, method.Name)
}

// GenerateJSMock generates a JavaScript/TypeScript mock
func (m *MockGenerator) GenerateJSMock(className string, methods []Method) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("export const create%sMock = () => ({\n", className))

	for i, method := range methods {
		sb.WriteString(fmt.Sprintf("  %s: jest.fn()", method.Name))
		if i < len(methods)-1 {
			sb.WriteString(",")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("});\n")

	return sb.String()
}

// ExtractInterface extracts interface information from code (simplified)
func (m *MockGenerator) ExtractInterface(code string, interfaceName string) *Interface {
	// Simplified extraction - in real implementation would use AST
	return &Interface{
		Name: interfaceName,
		Methods: []Method{
			{
				Name:          "Method1",
				Signature:     "(arg string) error",
				CallSignature: "(arg)",
				DefaultReturn: "return nil",
			},
		},
	}
}
