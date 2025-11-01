package api

// OpenAPISpec represents a simplified OpenAPI specification
type OpenAPISpec struct {
	OpenAPI string            `json:"openapi" yaml:"openapi"`
	Info    Info              `json:"info" yaml:"info"`
	Servers []Server          `json:"servers,omitempty" yaml:"servers,omitempty"`
	Paths   map[string]Path   `json:"paths" yaml:"paths"`
	Tags    []Tag             `json:"tags,omitempty" yaml:"tags,omitempty"`
}

// Info contains API metadata
type Info struct {
	Title       string  `json:"title" yaml:"title"`
	Description string  `json:"description,omitempty" yaml:"description,omitempty"`
	Version     string  `json:"version" yaml:"version"`
	Contact     Contact `json:"contact,omitempty" yaml:"contact,omitempty"`
}

// Contact information
type Contact struct {
	Name  string `json:"name,omitempty" yaml:"name,omitempty"`
	URL   string `json:"url,omitempty" yaml:"url,omitempty"`
	Email string `json:"email,omitempty" yaml:"email,omitempty"`
}

// Server information
type Server struct {
	URL         string `json:"url" yaml:"url"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// Path represents endpoints for a path
type Path struct {
	Get     *Operation `json:"get,omitempty" yaml:"get,omitempty"`
	Post    *Operation `json:"post,omitempty" yaml:"post,omitempty"`
	Put     *Operation `json:"put,omitempty" yaml:"put,omitempty"`
	Delete  *Operation `json:"delete,omitempty" yaml:"delete,omitempty"`
	Patch   *Operation `json:"patch,omitempty" yaml:"patch,omitempty"`
	Options *Operation `json:"options,omitempty" yaml:"options,omitempty"`
	Head    *Operation `json:"head,omitempty" yaml:"head,omitempty"`
}

// Operation represents an API operation
type Operation struct {
	Tags        []string  `json:"tags,omitempty" yaml:"tags,omitempty"`
	Summary     string    `json:"summary,omitempty" yaml:"summary,omitempty"`
	Description string    `json:"description,omitempty" yaml:"description,omitempty"`
	OperationID string    `json:"operationId,omitempty" yaml:"operationId,omitempty"`
	Parameters  []Parameter `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	Deprecated  bool      `json:"deprecated,omitempty" yaml:"deprecated,omitempty"`
}

// Parameter represents an API parameter
type Parameter struct {
	Name        string `json:"name" yaml:"name"`
	In          string `json:"in" yaml:"in"` // "query", "header", "path", "cookie"
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Required    bool   `json:"required,omitempty" yaml:"required,omitempty"`
	Schema      Schema `json:"schema,omitempty" yaml:"schema,omitempty"`
}

// Schema represents a JSON schema
type Schema struct {
	Type string `json:"type,omitempty" yaml:"type,omitempty"`
}

// Tag represents an API tag for grouping
type Tag struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// EndpointGroup groups endpoints by tag
type EndpointGroup struct {
	Tag       string
	Endpoints []Endpoint
}

// Endpoint represents a single API endpoint
type Endpoint struct {
	Method      string
	Path        string
	Summary     string
	Description string
	Deprecated  bool
}
