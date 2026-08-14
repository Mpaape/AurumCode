// Package taskspec parses and validates the versioned atomic card contract.
// It treats card data as untrusted input and never authorizes a board change.
package taskspec

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	SchemaName    = "aurum.task-spec"
	SchemaVersion = 1

	CodeMalformed      = "malformed"
	CodeUnknownField   = "unknown_field"
	CodeDuplicateField = "duplicate_field"
	CodeMissingField   = "missing_field"
	CodeInvalidField   = "invalid_field"
	CodeUnsafePath     = "unsafe_path"
)

const (
	maxTitle         = 256
	maxText          = 4096
	maxPath          = 512
	maxListItems     = 128
	maxDependencies  = 64
	maxReferences    = 64
	maxTrustBoundary = 16
)

var (
	cardIDPattern      = regexp.MustCompile(`^AUR-[0-9]{3}$`)
	officePattern      = regexp.MustCompile(`^O[0-9]{2}-[a-z0-9]+(?:-[a-z0-9]+)*$`)
	requirementPattern = regexp.MustCompile(`^PR-[A-Z0-9]+-[0-9]{3}$`)
	controlPattern     = regexp.MustCompile(`^CR-[A-Z0-9]+-[0-9]{3}$`)
	mutationPattern    = regexp.MustCompile(`^MUT-[0-9]{3}$`)
	digestPattern      = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	gitSHA256          = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// FieldError identifies the exact field that made a TaskSpec unusable.
// Detail is bounded, structural information and never includes input values.
type FieldError struct {
	Field  string
	Code   string
	Detail string
}

func (e *FieldError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("taskspec: %s: %s", e.Field, e.Code)
	}
	return fmt.Sprintf("taskspec: %s: %s: %s", e.Field, e.Code, e.Detail)
}

// Mutation is the one reversible mutation bound to a TaskSpec scenario.
type Mutation struct {
	ID       string `yaml:"id" json:"id"`
	Boundary string `yaml:"boundary" json:"boundary"`
	Change   string `yaml:"change" json:"change"`
	Expected string `yaml:"expected" json:"expected"`
}

// TaskSpec is the strict, normalized representation of an atomic card.
type TaskSpec struct {
	Schema          string   `yaml:"schema" json:"schema"`
	Version         int      `yaml:"version" json:"version"`
	ID              string   `yaml:"id" json:"id"`
	Title           string   `yaml:"title" json:"title"`
	Status          string   `yaml:"status" json:"status"`
	Validation      string   `yaml:"validation" json:"validation"`
	Office          string   `yaml:"office" json:"office"`
	DependsOn       []string `yaml:"depends_on" json:"depends_on"`
	Requirements    []string `yaml:"requirements" json:"requirements"`
	Controls        []string `yaml:"controls" json:"controls"`
	Paths           []string `yaml:"paths" json:"paths"`
	ReadPaths       []string `yaml:"read_paths,omitempty" json:"read_paths,omitempty"`
	ForbiddenPaths  []string `yaml:"forbidden_paths" json:"forbidden_paths"`
	BaseSHA         string   `yaml:"base_sha" json:"base_sha"`
	SpecDigest      string   `yaml:"spec_digest" json:"spec_digest"`
	Risk            string   `yaml:"risk" json:"risk"`
	DataClass       string   `yaml:"data_class" json:"data_class"`
	TrustBoundaries []string `yaml:"trust_boundaries" json:"trust_boundaries"`
	Outcome         string   `yaml:"outcome" json:"outcome"`
	Mutation        Mutation `yaml:"mutation" json:"mutation"`
}

var rootFields = map[string]bool{
	"schema": true, "version": true, "id": true, "title": true,
	"status": true, "validation": true, "office": true, "depends_on": true,
	"requirements": true, "controls": true, "paths": true, "read_paths": true,
	"forbidden_paths": true, "base_sha": true, "spec_digest": true,
	"risk": true, "data_class": true, "trust_boundaries": true,
	"outcome": true, "mutation": true,
}

var mutationFields = map[string]bool{
	"id": true, "boundary": true, "change": true, "expected": true,
}

var requiredFields = []string{
	"schema", "version", "id", "title", "status", "validation", "office",
	"depends_on", "requirements", "controls", "paths", "forbidden_paths",
	"base_sha", "spec_digest", "risk", "data_class", "trust_boundaries",
	"outcome", "mutation",
}

var statusValues = map[string]bool{
	"backlog": true, "ready": true, "doing": true, "review": true,
	"done": true, "blocked-on-owner": true, "cancelled": true,
}

var validationValues = map[string]bool{"none": true, "tested": true, "skeptical": true}
var riskValues = map[string]bool{"low": true, "medium": true, "high": true, "critical": true}
var dataClassValues = map[string]bool{"public": true, "internal": true, "confidential": true, "restricted": true}

var boundaryValues = map[string]bool{
	"repository": true, "authorization-source": true, "network-source": true,
	"network-endpoint": true, "container-engine": true, "supply-chain": true,
	"scm": true, "documentation-tool": true, "parser-runtime": true,
	"parser-worker": true, "model-provider": true, "review-isolation": true,
	"filesystem": true, "secret-channel": true, "labelled-corpus": true,
	"policy-input": true, "candidate-identity": true, "state-store": true,
	"git-metadata": true, "process-runtime": true, "skill-runtime": true,
	"skill-policy": true, "mcp": true, "mcp-client": true, "database": true,
	"security-scanner": true,
}

// Load decodes one YAML document and validates every field before returning it.
// JSON is accepted as the JSON-compatible subset of YAML.
func Load(data []byte) (TaskSpec, error) {
	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&document); err != nil {
		return TaskSpec{}, fieldError("$", CodeMalformed, "document cannot be decoded")
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err != io.EOF {
		return TaskSpec{}, fieldError("$", CodeMalformed, "multiple documents are not accepted")
	}
	if len(document.Content) != 1 {
		return TaskSpec{}, fieldError("$", CodeMalformed, "document must contain one value")
	}
	root := document.Content[0]
	if err := validateNodeSafety(root); err != nil {
		return TaskSpec{}, err
	}
	if root.Kind != yaml.MappingNode {
		return TaskSpec{}, fieldError("$", CodeMalformed, "root must be a mapping")
	}
	fields, err := mappingFields(root, rootFields, "$")
	if err != nil {
		return TaskSpec{}, err
	}
	for _, field := range requiredFields {
		if _, ok := fields[field]; !ok {
			return TaskSpec{}, fieldError(field, CodeMissingField, "required field is absent")
		}
	}
	if err := validateRootShapes(fields); err != nil {
		return TaskSpec{}, err
	}

	var spec TaskSpec
	if err := root.Decode(&spec); err != nil {
		return TaskSpec{}, fieldError("$", CodeMalformed, "field value has the wrong type")
	}
	if err := spec.Validate(); err != nil {
		return TaskSpec{}, err
	}
	return spec, nil
}

// Parse is an alias for Load for callers that already use parser terminology.
func Parse(data []byte) (TaskSpec, error) { return Load(data) }

// LoadFile reads and validates one TaskSpec. It does not write or resolve any
// path from the document.
func LoadFile(path string) (TaskSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TaskSpec{}, fieldError("$", CodeMalformed, "document cannot be read")
	}
	return Load(data)
}

// Validate rechecks a normalized TaskSpec. It is safe to call before every
// operation that would use the spec.
func (s TaskSpec) Validate() error {
	if s.Schema != SchemaName {
		return fieldError("schema", CodeInvalidField, "unsupported schema")
	}
	if s.Version != SchemaVersion {
		return fieldError("version", CodeInvalidField, "unsupported schema version")
	}
	if !cardIDPattern.MatchString(s.ID) {
		return fieldError("id", CodeInvalidField, "must match AUR-NNN")
	}
	if !boundedText(s.Title, maxTitle) {
		return fieldError("title", CodeInvalidField, "must be a bounded non-empty string")
	}
	if !statusValues[s.Status] {
		return fieldError("status", CodeInvalidField, "value is not in the closed status enum")
	}
	if !validationValues[s.Validation] {
		return fieldError("validation", CodeInvalidField, "value is not in the closed validation enum")
	}
	if !officePattern.MatchString(s.Office) {
		return fieldError("office", CodeInvalidField, "must identify an office")
	}
	if len(s.DependsOn) > maxDependencies {
		return fieldError("depends_on", CodeInvalidField, "too many dependencies")
	}
	if err := validateIDs("depends_on", s.DependsOn, cardIDPattern); err != nil {
		return err
	}
	for _, dependency := range s.DependsOn {
		if dependency == s.ID {
			return fieldError("depends_on", CodeInvalidField, "a card cannot depend on itself")
		}
	}
	if err := validateIDs("requirements", s.Requirements, requirementPattern); err != nil {
		return err
	}
	if len(s.Requirements) == 0 {
		return fieldError("requirements", CodeInvalidField, "at least one requirement is required")
	}
	if err := validateIDs("controls", s.Controls, controlPattern); err != nil {
		return err
	}
	if len(s.Controls) == 0 {
		return fieldError("controls", CodeInvalidField, "at least one control is required")
	}
	if err := validatePathList("paths", s.Paths, 1); err != nil {
		return err
	}
	if err := validatePathList("read_paths", s.ReadPaths, 0); err != nil {
		return err
	}
	if err := validatePathList("forbidden_paths", s.ForbiddenPaths, 1); err != nil {
		return err
	}
	if err := validateDisjointPaths(s.Paths, "paths", s.ReadPaths, "read_paths"); err != nil {
		return err
	}
	if err := validateDisjointPaths(s.Paths, "paths", s.ForbiddenPaths, "forbidden_paths"); err != nil {
		return err
	}
	if err := validateDisjointPaths(s.ReadPaths, "read_paths", s.ForbiddenPaths, "forbidden_paths"); err != nil {
		return err
	}
	if s.BaseSHA != "lock-at-execution" && !gitSHA256.MatchString(s.BaseSHA) {
		return fieldError("base_sha", CodeInvalidField, "must be lock-at-execution or a 40-hex SHA")
	}
	if s.SpecDigest != "lock-at-execution" && !digestPattern.MatchString(s.SpecDigest) {
		return fieldError("spec_digest", CodeInvalidField, "must be lock-at-execution or a sha256 digest")
	}
	if !riskValues[s.Risk] {
		return fieldError("risk", CodeInvalidField, "value is not in the closed risk enum")
	}
	if !dataClassValues[s.DataClass] {
		return fieldError("data_class", CodeInvalidField, "value is not in the closed data class enum")
	}
	if len(s.TrustBoundaries) == 0 || len(s.TrustBoundaries) > maxTrustBoundary {
		return fieldError("trust_boundaries", CodeInvalidField, "must contain one to sixteen boundaries")
	}
	if err := validateEnumList("trust_boundaries", s.TrustBoundaries, boundaryValues); err != nil {
		return err
	}
	if !contains(s.TrustBoundaries, "repository") {
		return fieldError("trust_boundaries", CodeInvalidField, "repository is required")
	}
	if !boundedText(s.Outcome, maxText) || strings.ContainsAny(s.Outcome, "\r\n") {
		return fieldError("outcome", CodeInvalidField, "must be exactly one bounded line")
	}
	if err := validateMutation(s.Mutation, s.TrustBoundaries); err != nil {
		return err
	}
	return nil
}

// Digest returns the stable artifact digest for a validated TaskSpec.
func Digest(spec TaskSpec) (string, error) {
	if err := spec.Validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(spec)
	if err != nil {
		return "", fieldError("$", CodeMalformed, "normalized artifact cannot be encoded")
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func validateRootShapes(fields map[string]*yaml.Node) error {
	stringFields := []string{"schema", "id", "title", "status", "validation", "office", "base_sha", "spec_digest", "risk", "data_class", "outcome"}
	for _, field := range stringFields {
		if err := requireScalarString(fields[field], field); err != nil {
			return err
		}
	}
	if fields["version"].Kind != yaml.ScalarNode || fields["version"].Tag != "!!int" {
		return fieldError("version", CodeInvalidField, "must be an integer")
	}
	for _, field := range []string{"depends_on", "requirements", "controls", "paths", "read_paths", "forbidden_paths", "trust_boundaries"} {
		if field == "read_paths" && fields[field] == nil {
			continue
		}
		if err := requireStringSequence(fields[field], field); err != nil {
			return err
		}
	}
	mutation := fields["mutation"]
	if mutation.Kind != yaml.MappingNode {
		return fieldError("mutation", CodeInvalidField, "must be a mapping")
	}
	mutationMap, err := mappingFields(mutation, mutationFields, "mutation")
	if err != nil {
		return err
	}
	for _, field := range []string{"id", "boundary", "change", "expected"} {
		if _, ok := mutationMap[field]; !ok {
			return fieldError("mutation."+field, CodeMissingField, "required field is absent")
		}
		if err := requireScalarString(mutationMap[field], "mutation."+field); err != nil {
			return err
		}
	}
	return nil
}

func mappingFields(node *yaml.Node, allowed map[string]bool, field string) (map[string]*yaml.Node, error) {
	if node.Kind != yaml.MappingNode || len(node.Content)%2 != 0 {
		return nil, fieldError(field, CodeMalformed, "mapping is malformed")
	}
	fields := make(map[string]*yaml.Node, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || key.Value == "" {
			return nil, fieldError(field, CodeMalformed, "mapping key must be a non-empty string")
		}
		if !allowed[key.Value] {
			return nil, fieldError(unknownFieldToken(key.Value), CodeUnknownField, "field is not in the schema")
		}
		if _, exists := fields[key.Value]; exists {
			return nil, fieldError(key.Value, CodeDuplicateField, "field occurs more than once")
		}
		fields[key.Value] = value
	}
	return fields, nil
}

func validateNodeSafety(node *yaml.Node) error {
	if node.Kind == yaml.AliasNode {
		return fieldError("$", CodeMalformed, "aliases are not accepted")
	}
	if node.Anchor != "" {
		return fieldError("$", CodeMalformed, "anchors are not accepted")
	}
	switch node.Kind {
	case yaml.MappingNode:
		if node.Tag != "!!map" {
			return fieldError("$", CodeMalformed, "custom YAML tags are not accepted")
		}
	case yaml.SequenceNode:
		if node.Tag != "!!seq" {
			return fieldError("$", CodeMalformed, "custom YAML tags are not accepted")
		}
	case yaml.ScalarNode:
		if node.Tag != "!!str" && node.Tag != "!!int" && node.Tag != "!!null" && node.Tag != "!!bool" && node.Tag != "!!float" {
			return fieldError("$", CodeMalformed, "custom YAML tags are not accepted")
		}
	}
	for _, child := range node.Content {
		if err := validateNodeSafety(child); err != nil {
			return err
		}
	}
	return nil
}

func requireScalarString(node *yaml.Node, field string) error {
	if node == nil || node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return fieldError(field, CodeInvalidField, "must be a string")
	}
	return nil
}

func requireStringSequence(node *yaml.Node, field string) error {
	if node == nil || node.Kind != yaml.SequenceNode {
		return fieldError(field, CodeInvalidField, "must be a sequence")
	}
	if len(node.Content) > maxListItems {
		return fieldError(field, CodeInvalidField, "sequence exceeds the bounded item count")
	}
	for index, item := range node.Content {
		if err := requireScalarString(item, fmt.Sprintf("%s[%d]", field, index)); err != nil {
			return err
		}
	}
	return nil
}

func validateIDs(field string, values []string, pattern *regexp.Regexp) error {
	seen := make(map[string]bool, len(values))
	limit := maxListItems
	if field == "depends_on" {
		limit = maxDependencies
	} else if field == "requirements" || field == "controls" {
		limit = maxReferences
	}
	if len(values) > limit {
		return fieldError(field, CodeInvalidField, "sequence exceeds the bounded item count")
	}
	for index, value := range values {
		if !pattern.MatchString(value) {
			return fieldError(fmt.Sprintf("%s[%d]", field, index), CodeInvalidField, "identifier has an invalid format")
		}
		if seen[value] {
			return fieldError(fmt.Sprintf("%s[%d]", field, index), CodeInvalidField, "duplicate identifier")
		}
		seen[value] = true
	}
	return nil
}

func validateEnumList(field string, values []string, allowed map[string]bool) error {
	seen := make(map[string]bool, len(values))
	for index, value := range values {
		if !allowed[value] {
			return fieldError(fmt.Sprintf("%s[%d]", field, index), CodeInvalidField, "value is not in the closed enum")
		}
		if seen[value] {
			return fieldError(fmt.Sprintf("%s[%d]", field, index), CodeInvalidField, "duplicate value")
		}
		seen[value] = true
	}
	return nil
}

func validatePathList(field string, paths []string, minimum int) error {
	if len(paths) < minimum || len(paths) > maxListItems {
		return fieldError(field, CodeUnsafePath, "path list is outside its bounds")
	}
	seen := make(map[string]bool, len(paths))
	for index, path := range paths {
		if !safePath(path) {
			return fieldError(fmt.Sprintf("%s[%d]", field, index), CodeUnsafePath, "path must be relative and normalized")
		}
		if seen[path] {
			return fieldError(fmt.Sprintf("%s[%d]", field, index), CodeUnsafePath, "duplicate path")
		}
		seen[path] = true
	}
	return nil
}

func validateDisjointPaths(left []string, leftField string, right []string, rightField string) error {
	for leftIndex, a := range left {
		for rightIndex, b := range right {
			if pathPrefix(a, b) || pathPrefix(b, a) {
				return fieldError(fmt.Sprintf("%s[%d]", rightField, rightIndex), CodeUnsafePath, fmt.Sprintf("overlaps %s[%d]", leftField, leftIndex))
			}
		}
	}
	return nil
}

func validateMutation(m Mutation, trustBoundaries []string) error {
	if !mutationPattern.MatchString(m.ID) {
		return fieldError("mutation.id", CodeInvalidField, "must match MUT-NNN")
	}
	if !boundaryValues[m.Boundary] || !contains(trustBoundaries, m.Boundary) {
		return fieldError("mutation.boundary", CodeInvalidField, "boundary is closed and must be declared")
	}
	if !boundedText(m.Change, maxText) {
		return fieldError("mutation.change", CodeInvalidField, "must be a bounded non-empty string")
	}
	if !boundedText(m.Expected, maxText) {
		return fieldError("mutation.expected", CodeInvalidField, "must be a bounded non-empty string")
	}
	return nil
}

func safePath(path string) bool {
	if path == "" || utf8.RuneCountInString(path) > maxPath || strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.Contains(path, "\\") || strings.Contains(path, "//") {
		return false
	}
	if len(path) >= 2 && ((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) && path[1] == ':' {
		return false
	}
	if strings.TrimSpace(path) != path || strings.HasSuffix(path, "/") {
		return false
	}
	for _, r := range path {
		if r == '/' {
			continue
		}
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	for _, component := range strings.Split(path, "/") {
		if component == "" || component == "." || component == ".." {
			return false
		}
	}
	return true
}

func pathPrefix(parent, child string) bool {
	return parent == child || strings.HasPrefix(child, parent+"/")
}

func boundedText(value string, maximum int) bool {
	return len(value) > 0 && len(value) <= maximum && strings.TrimSpace(value) != "" && !strings.ContainsRune(value, '\x00')
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func fieldError(field, code, detail string) *FieldError {
	return &FieldError{Field: sanitizeField(field), Code: code, Detail: detail}
}

func unknownFieldToken(field string) string {
	sum := sha256.Sum256([]byte(field))
	return "unknown:" + hex.EncodeToString(sum[:8])
}

func sanitizeField(field string) string {
	if field == "$" || strings.HasPrefix(field, "unknown:") {
		return field
	}
	for _, r := range field {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || strings.ContainsRune("._[]-", r) {
			continue
		}
		return unknownFieldToken(field)
	}
	return field
}
