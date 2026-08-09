// Package evidence defines the content-addressed evidence-bundle manifest.
// Manifest data is an observation only: parsing or verifying it never grants
// approval, publication authority, or any other capability.
package evidence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	SchemaName    = "aurum.evidence-bundle"
	SchemaVersion = 1

	MaxManifestBytes = 4 * 1024 * 1024
	MaxArtifacts     = 64
	MaxArtifactBytes = 4 * 1024 * 1024
	MaxPathBytes     = 512
)

const (
	manifestDigestDomain = "aurum.evidence-bundle.manifest.v1"
	pathPattern          = `^(?!/)(?![A-Za-z]:)(?!.*\\)(?!.*//)(?!.*(?:^|/)\.\.?(?:/|$))(?!.*\s)(?!.*[\u0000-\u001f\u007f-\u009f])[^/]+(?:/[^/]+)*$`
)

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

var candidateFields = []string{
	"repository_identity",
	"base_tree_digest",
	"head_tree_digest",
	"change_digest",
	"task_spec_digest",
	"configuration_digest",
	"policy_digest",
	"prompt_and_rubric_digest",
	"skill_set_digest",
	"provider_model_backend_identity_digest",
	"toolchain_and_tool_set_digest",
	"dependency_lock_digest",
	"container_image_set_digest",
	"test_manifest_digest",
	"role_context_manifest_digest",
}

var artifactKinds = []string{
	"task-spec",
	"container-image",
	"acceptance-result",
	"unit-result",
	"integration-result",
	"review-result",
	"skeptic-result",
}

var mediaTypes = []string{
	"application/json",
	"application/octet-stream",
	"text/plain",
}

const (
	CodeMalformed          = "malformed"
	CodeUnknownField       = "unknown_field"
	CodeDuplicateField     = "duplicate_field"
	CodeMissingField       = "missing_field"
	CodeInvalidField       = "invalid_field"
	CodeUnsafePath         = "unsafe_path"
	CodeLimitExceeded      = "limit_exceeded"
	CodeNonCanonical       = "non_canonical"
	CodeDigestMismatch     = "digest_mismatch"
	CodeCandidateMismatch  = "candidate_mismatch"
	CodeArtifactMissing    = "artifact_missing"
	CodeArtifactUnexpected = "artifact_unexpected"
	CodeAuthorityDenied    = "authority_denied"
	CodeSchemaContract     = "schema_contract"
)

// Error is a bounded, typed rejection. It intentionally never includes an
// untrusted value from the manifest or an evidence output.
type Error struct {
	Field string
	Code  string
}

func (e *Error) Error() string {
	return fmt.Sprintf("evidence manifest: %s: %s", e.Field, e.Code)
}

// CandidateIdentityV1 is the complete canonical tuple. Every member is a
// digest; consumers may not define a shorter local identity.
type CandidateIdentityV1 struct {
	RepositoryIdentity                 string `json:"repository_identity"`
	BaseTreeDigest                     string `json:"base_tree_digest"`
	HeadTreeDigest                     string `json:"head_tree_digest"`
	ChangeDigest                       string `json:"change_digest"`
	TaskSpecDigest                     string `json:"task_spec_digest"`
	ConfigurationDigest                string `json:"configuration_digest"`
	PolicyDigest                       string `json:"policy_digest"`
	PromptAndRubricDigest              string `json:"prompt_and_rubric_digest"`
	SkillSetDigest                     string `json:"skill_set_digest"`
	ProviderModelBackendIdentityDigest string `json:"provider_model_backend_identity_digest"`
	ToolchainAndToolSetDigest          string `json:"toolchain_and_tool_set_digest"`
	DependencyLockDigest               string `json:"dependency_lock_digest"`
	ContainerImageSetDigest            string `json:"container_image_set_digest"`
	TestManifestDigest                 string `json:"test_manifest_digest"`
	RoleContextManifestDigest          string `json:"role_context_manifest_digest"`
}

// ArtifactInput carries output bytes only while sealing or verifying. The
// bytes are never persisted in a Manifest by this package.
type ArtifactInput struct {
	Path      string
	Kind      string
	MediaType string
	Data      []byte
}

// Artifact is a digest record for one sanitized output. Authority is always
// "none"; an evidence record cannot authorize its own acceptance.
type Artifact struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	MediaType string `json:"media_type"`
	Authority string `json:"authority"`
	Bytes     int64  `json:"bytes"`
	SHA256    string `json:"sha256"`
}

// Manifest is self-addressed through ManifestDigest. That digest is computed
// over the canonical form with manifest_digest omitted.
type Manifest struct {
	Schema            string              `json:"schema"`
	Version           int                 `json:"version"`
	CandidateIdentity CandidateIdentityV1 `json:"candidate_identity"`
	Artifacts         []Artifact          `json:"artifacts"`
	ManifestDigest    string              `json:"manifest_digest"`
}

func reject(field, code string) *Error {
	return &Error{Field: field, Code: code}
}

// Seal constructs a deterministic manifest from an externally established
// candidate identity and sanitized output bytes.
func Seal(candidate CandidateIdentityV1, inputs []ArtifactInput) (Manifest, error) {
	if err := validateCandidate(candidate); err != nil {
		return Manifest{}, err
	}
	artifacts, err := recordsFor(inputs)
	if err != nil {
		return Manifest{}, err
	}
	manifest := Manifest{
		Schema:            SchemaName,
		Version:           SchemaVersion,
		CandidateIdentity: candidate,
		Artifacts:         artifacts,
	}
	digest, err := calculateManifestDigest(manifest)
	if err != nil {
		return Manifest{}, err
	}
	manifest.ManifestDigest = digest
	if err := validateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// Marshal returns the one accepted canonical JSON representation.
func Marshal(manifest Manifest) ([]byte, error) {
	if err := validateManifest(manifest); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return nil, reject("document", CodeMalformed)
	}
	if len(encoded) > MaxManifestBytes {
		return nil, reject("document", CodeLimitExceeded)
	}
	return encoded, nil
}

// Parse rejects malformed, duplicate, unknown, non-canonical, or unsealed
// input before returning a manifest.
func Parse(data []byte) (Manifest, error) {
	if len(data) == 0 {
		return Manifest{}, reject("document", CodeMalformed)
	}
	if len(data) > MaxManifestBytes {
		return Manifest{}, reject("document", CodeLimitExceeded)
	}
	if !utf8.Valid(data) {
		return Manifest{}, reject("document", CodeMalformed)
	}
	if err := rejectDuplicateNames(data); err != nil {
		return Manifest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		if strings.Contains(err.Error(), "unknown field") {
			return Manifest{}, reject("document", CodeUnknownField)
		}
		return Manifest{}, reject("document", CodeMalformed)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Manifest{}, reject("document", CodeMalformed)
	}
	if err := validateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	canonical, err := json.Marshal(manifest)
	if err != nil {
		return Manifest{}, reject("document", CodeMalformed)
	}
	if !bytes.Equal(data, canonical) {
		return Manifest{}, reject("document", CodeNonCanonical)
	}
	return manifest, nil
}

// Verify recomputes candidate, manifest, and output bindings without I/O.
func Verify(manifest Manifest, expected CandidateIdentityV1, outputs []ArtifactInput) error {
	if err := validateManifest(manifest); err != nil {
		return err
	}
	if err := validateCandidate(expected); err != nil {
		return err
	}
	if manifest.CandidateIdentity != expected {
		return reject("candidate_identity", CodeCandidateMismatch)
	}
	recomputed, err := recordsFor(outputs)
	if err != nil {
		return err
	}
	manifestByPath := make(map[string]int, len(manifest.Artifacts))
	for index, artifact := range manifest.Artifacts {
		manifestByPath[artifact.Path] = index
	}
	for _, output := range recomputed {
		if _, exists := manifestByPath[output.Path]; !exists {
			return reject("artifacts", CodeArtifactUnexpected)
		}
	}
	if len(recomputed) != len(manifest.Artifacts) {
		return reject("artifacts", CodeArtifactMissing)
	}
	for index := range manifest.Artifacts {
		sealed := manifest.Artifacts[index]
		observed := recomputed[index]
		field := fmt.Sprintf("artifacts[%d]", index)
		if sealed.Path != observed.Path {
			return reject("artifacts", CodeArtifactMissing)
		}
		if sealed.Kind != observed.Kind {
			return reject(field+".kind", CodeInvalidField)
		}
		if sealed.MediaType != observed.MediaType {
			return reject(field+".media_type", CodeInvalidField)
		}
		if sealed.Authority != observed.Authority {
			return reject(field+".authority", CodeAuthorityDenied)
		}
		if sealed.Bytes != observed.Bytes {
			return reject(field+".bytes", CodeDigestMismatch)
		}
		if sealed.SHA256 != observed.SHA256 {
			return reject(field+".sha256", CodeDigestMismatch)
		}
	}
	return nil
}

// VerifyBytes combines hostile parsing and verification.
func VerifyBytes(data []byte, expected CandidateIdentityV1, outputs []ArtifactInput) error {
	manifest, err := Parse(data)
	if err != nil {
		return err
	}
	return Verify(manifest, expected, outputs)
}

// ValidateSchema checks that the repository JSON Schema expresses the same
// closed fields, limits, enums, and canonicalization rules as this parser.
func ValidateSchema(data []byte) error {
	if len(data) == 0 || len(data) > MaxManifestBytes || !utf8.Valid(data) {
		return schemaReject("document")
	}
	if err := rejectDuplicateNames(data); err != nil {
		return err
	}
	root, ok := decodeObject(data)
	if !ok || !keysExact(root, []string{"$schema", "$id", "title", "description", "type", "additionalProperties", "required", "properties", "$defs", "x-aurumcode"}) {
		return schemaReject("root")
	}
	if !rawStringIs(root["$schema"], "https://json-schema.org/draft/2020-12/schema") ||
		!rawStringIs(root["$id"], "https://aurumcode.local/schemas/evidence-bundle.schema.json") ||
		!rawStringIs(root["title"], "AurumCode content-addressed evidence bundle") ||
		!rawStringIs(root["description"], "Closed, canonical and non-authorizing digest manifest for sanitized evidence outputs.") ||
		!rawStringIs(root["type"], "object") || !rawBoolIs(root["additionalProperties"], false) ||
		!rawStringsExact(root["required"], []string{"schema", "version", "candidate_identity", "artifacts", "manifest_digest"}) {
		return schemaReject("root")
	}
	properties, ok := rawObject(root["properties"])
	if !ok || !keysExact(properties, []string{"schema", "version", "candidate_identity", "artifacts", "manifest_digest"}) {
		return schemaReject("properties")
	}
	if !objectStringIs(properties["schema"], "const", SchemaName) || !objectIntIs(properties["version"], "const", SchemaVersion) ||
		!objectStringIs(properties["candidate_identity"], "$ref", "#/$defs/candidate-identity-v1") ||
		!objectStringIs(properties["manifest_digest"], "$ref", "#/$defs/digest") {
		return schemaReject("properties")
	}
	artifacts, ok := rawObject(properties["artifacts"])
	if !ok || !keysExact(artifacts, []string{"type", "minItems", "maxItems", "uniqueItems", "items"}) || !rawStringIs(artifacts["type"], "array") ||
		!rawIntIs(artifacts["minItems"], 1) || !rawIntIs(artifacts["maxItems"], MaxArtifacts) ||
		!rawBoolIs(artifacts["uniqueItems"], true) ||
		!objectStringIs(artifacts["items"], "$ref", "#/$defs/artifact") {
		return schemaReject("properties.artifacts")
	}

	definitions, ok := rawObject(root["$defs"])
	if !ok || !keysExact(definitions, []string{"digest", "candidate-identity-v1", "artifact"}) {
		return schemaReject("$defs")
	}
	digest, ok := rawObject(definitions["digest"])
	if !ok || !keysExact(digest, []string{"type", "pattern"}) || !rawStringIs(digest["type"], "string") || !rawStringIs(digest["pattern"], `^sha256:[0-9a-f]{64}$`) {
		return schemaReject("$defs.digest")
	}
	if !validateCandidateSchema(definitions["candidate-identity-v1"]) {
		return schemaReject("$defs.candidate-identity-v1")
	}
	if !validateArtifactSchema(definitions["artifact"]) {
		return schemaReject("$defs.artifact")
	}
	if !validateSchemaExtensions(root["x-aurumcode"]) {
		return schemaReject("x-aurumcode")
	}
	return nil
}

type unsignedManifest struct {
	Schema            string              `json:"schema"`
	Version           int                 `json:"version"`
	CandidateIdentity CandidateIdentityV1 `json:"candidate_identity"`
	Artifacts         []Artifact          `json:"artifacts"`
}

func recordsFor(inputs []ArtifactInput) ([]Artifact, error) {
	if len(inputs) == 0 {
		return nil, reject("artifacts", CodeMissingField)
	}
	if len(inputs) > MaxArtifacts {
		return nil, reject("artifacts", CodeLimitExceeded)
	}
	records := make([]Artifact, len(inputs))
	seen := make(map[string]struct{}, len(inputs))
	var total int64
	for index, input := range inputs {
		field := fmt.Sprintf("artifacts[%d]", index)
		if err := validatePath(input.Path, field+".path"); err != nil {
			return nil, err
		}
		if _, duplicate := seen[input.Path]; duplicate {
			return nil, reject(field+".path", CodeDuplicateField)
		}
		seen[input.Path] = struct{}{}
		if !contains(artifactKinds, input.Kind) {
			return nil, reject(field+".kind", CodeInvalidField)
		}
		if !contains(mediaTypes, input.MediaType) {
			return nil, reject(field+".media_type", CodeInvalidField)
		}
		size := int64(len(input.Data))
		if size > MaxArtifactBytes {
			return nil, reject(field+".bytes", CodeLimitExceeded)
		}
		if total > MaxArtifactBytes-size {
			return nil, reject("artifacts", CodeLimitExceeded)
		}
		total += size
		records[index] = Artifact{
			Path: input.Path, Kind: input.Kind, MediaType: input.MediaType,
			Authority: "none", Bytes: size, SHA256: digestBytes(input.Data),
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Path < records[j].Path })
	return records, nil
}

func validateManifest(manifest Manifest) error {
	if manifest.Schema == "" {
		return reject("schema", CodeMissingField)
	}
	if manifest.Schema != SchemaName {
		return reject("schema", CodeInvalidField)
	}
	if manifest.Version == 0 {
		return reject("version", CodeMissingField)
	}
	if manifest.Version != SchemaVersion {
		return reject("version", CodeInvalidField)
	}
	if err := validateCandidate(manifest.CandidateIdentity); err != nil {
		return err
	}
	if len(manifest.Artifacts) == 0 {
		return reject("artifacts", CodeMissingField)
	}
	if len(manifest.Artifacts) > MaxArtifacts {
		return reject("artifacts", CodeLimitExceeded)
	}
	var previous string
	var total int64
	for index, artifact := range manifest.Artifacts {
		field := fmt.Sprintf("artifacts[%d]", index)
		if err := validatePath(artifact.Path, field+".path"); err != nil {
			return err
		}
		if index > 0 && artifact.Path <= previous {
			if artifact.Path == previous {
				return reject(field+".path", CodeDuplicateField)
			}
			return reject("artifacts", CodeNonCanonical)
		}
		previous = artifact.Path
		if !contains(artifactKinds, artifact.Kind) {
			return reject(field+".kind", CodeInvalidField)
		}
		if !contains(mediaTypes, artifact.MediaType) {
			return reject(field+".media_type", CodeInvalidField)
		}
		if artifact.Authority != "none" {
			return reject(field+".authority", CodeAuthorityDenied)
		}
		if artifact.Bytes < 0 || artifact.Bytes > MaxArtifactBytes {
			return reject(field+".bytes", CodeLimitExceeded)
		}
		if total > MaxArtifactBytes-artifact.Bytes {
			return reject("artifacts", CodeLimitExceeded)
		}
		total += artifact.Bytes
		if !digestPattern.MatchString(artifact.SHA256) {
			return reject(field+".sha256", CodeInvalidField)
		}
	}
	if manifest.ManifestDigest == "" {
		return reject("manifest_digest", CodeMissingField)
	}
	if !digestPattern.MatchString(manifest.ManifestDigest) {
		return reject("manifest_digest", CodeInvalidField)
	}
	expected, err := calculateManifestDigest(manifest)
	if err != nil {
		return err
	}
	if manifest.ManifestDigest != expected {
		return reject("manifest_digest", CodeDigestMismatch)
	}
	return nil
}

func validateCandidate(candidate CandidateIdentityV1) error {
	values := []string{
		candidate.RepositoryIdentity,
		candidate.BaseTreeDigest,
		candidate.HeadTreeDigest,
		candidate.ChangeDigest,
		candidate.TaskSpecDigest,
		candidate.ConfigurationDigest,
		candidate.PolicyDigest,
		candidate.PromptAndRubricDigest,
		candidate.SkillSetDigest,
		candidate.ProviderModelBackendIdentityDigest,
		candidate.ToolchainAndToolSetDigest,
		candidate.DependencyLockDigest,
		candidate.ContainerImageSetDigest,
		candidate.TestManifestDigest,
		candidate.RoleContextManifestDigest,
	}
	for index, value := range values {
		if value == "" {
			return reject("candidate_identity."+candidateFields[index], CodeMissingField)
		}
		if !digestPattern.MatchString(value) {
			return reject("candidate_identity."+candidateFields[index], CodeInvalidField)
		}
	}
	return nil
}

func validatePath(path, field string) error {
	if path == "" {
		return reject(field, CodeMissingField)
	}
	if len(path) > MaxPathBytes {
		return reject(field, CodeLimitExceeded)
	}
	if !utf8.ValidString(path) || strings.HasPrefix(path, "/") || strings.Contains(path, "\\") || strings.Contains(path, "//") ||
		(len(path) >= 2 && ((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) && path[1] == ':') {
		return reject(field, CodeUnsafePath)
	}
	for _, component := range strings.Split(path, "/") {
		if component == "" || component == "." || component == ".." {
			return reject(field, CodeUnsafePath)
		}
	}
	for _, character := range path {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return reject(field, CodeUnsafePath)
		}
	}
	return nil
}

func calculateManifestDigest(manifest Manifest) (string, error) {
	unsigned := unsignedManifest{
		Schema: manifest.Schema, Version: manifest.Version,
		CandidateIdentity: manifest.CandidateIdentity, Artifacts: manifest.Artifacts,
	}
	encoded, err := json.Marshal(unsigned)
	if err != nil {
		return "", reject("manifest_digest", CodeMalformed)
	}
	preimage := make([]byte, 0, len(manifestDigestDomain)+1+len(encoded))
	preimage = append(preimage, manifestDigestDomain...)
	preimage = append(preimage, '\n')
	preimage = append(preimage, encoded...)
	return digestBytes(preimage), nil
}

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func rejectDuplicateNames(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	first, err := decoder.Token()
	if err != nil {
		return reject("document", CodeMalformed)
	}
	if err := scanJSONValue(decoder, first); err != nil {
		return err
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder, token json.Token) error {
	delimiter, compound := token.(json.Delim)
	if !compound {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			nameToken, err := decoder.Token()
			if err != nil {
				return reject("document", CodeMalformed)
			}
			name, ok := nameToken.(string)
			if !ok {
				return reject("document", CodeMalformed)
			}
			if _, duplicate := seen[name]; duplicate {
				return reject("document", CodeDuplicateField)
			}
			seen[name] = struct{}{}
			value, err := decoder.Token()
			if err != nil {
				return reject("document", CodeMalformed)
			}
			if err := scanJSONValue(decoder, value); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return reject("document", CodeMalformed)
		}
	case '[':
		for decoder.More() {
			value, err := decoder.Token()
			if err != nil {
				return reject("document", CodeMalformed)
			}
			if err := scanJSONValue(decoder, value); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return reject("document", CodeMalformed)
		}
	default:
		return reject("document", CodeMalformed)
	}
	return nil
}

func validateCandidateSchema(raw json.RawMessage) bool {
	object, ok := rawObject(raw)
	if !ok || !keysExact(object, []string{"type", "additionalProperties", "required", "properties"}) ||
		!rawStringIs(object["type"], "object") || !rawBoolIs(object["additionalProperties"], false) ||
		!rawStringsExact(object["required"], candidateFields) {
		return false
	}
	properties, ok := rawObject(object["properties"])
	if !ok || !keysExact(properties, candidateFields) {
		return false
	}
	for _, field := range candidateFields {
		if !objectStringIs(properties[field], "$ref", "#/$defs/digest") {
			return false
		}
	}
	return true
}

func validateArtifactSchema(raw json.RawMessage) bool {
	object, ok := rawObject(raw)
	required := []string{"path", "kind", "media_type", "authority", "bytes", "sha256"}
	if !ok || !keysExact(object, []string{"type", "additionalProperties", "required", "properties"}) ||
		!rawStringIs(object["type"], "object") || !rawBoolIs(object["additionalProperties"], false) || !rawStringsExact(object["required"], required) {
		return false
	}
	properties, ok := rawObject(object["properties"])
	if !ok || !keysExact(properties, required) {
		return false
	}
	path, ok := rawObject(properties["path"])
	if !ok || !keysExact(path, []string{"type", "minLength", "maxLength", "pattern"}) || !rawStringIs(path["type"], "string") ||
		!rawIntIs(path["minLength"], 1) || !rawIntIs(path["maxLength"], MaxPathBytes) || !rawStringIs(path["pattern"], pathPattern) {
		return false
	}
	if !objectStringsExact(properties["kind"], "enum", artifactKinds) || !objectStringsExact(properties["media_type"], "enum", mediaTypes) ||
		!objectStringIs(properties["authority"], "const", "none") || !objectStringIs(properties["sha256"], "$ref", "#/$defs/digest") {
		return false
	}
	size, ok := rawObject(properties["bytes"])
	return ok && keysExact(size, []string{"type", "minimum", "maximum"}) && rawStringIs(size["type"], "integer") &&
		rawIntIs(size["minimum"], 0) && rawIntIs(size["maximum"], MaxArtifactBytes)
}

func validateSchemaExtensions(raw json.RawMessage) bool {
	extension, ok := rawObject(raw)
	if !ok || !keysExact(extension, []string{"max_document_bytes", "max_total_artifact_bytes", "path_policy", "canonicalization", "manifest_digest", "authorization"}) ||
		!rawIntIs(extension["max_document_bytes"], MaxManifestBytes) || !rawIntIs(extension["max_total_artifact_bytes"], MaxArtifactBytes) {
		return false
	}
	pathPolicy, ok := rawObject(extension["path_policy"])
	if !ok || !keysExact(pathPolicy, []string{"max_path_bytes", "relative_posix", "reject_backslash", "reject_traversal", "reject_whitespace", "reject_control", "reject_empty_components"}) ||
		!rawIntIs(pathPolicy["max_path_bytes"], MaxPathBytes) || !rawBoolIs(pathPolicy["relative_posix"], true) ||
		!rawBoolIs(pathPolicy["reject_backslash"], true) || !rawBoolIs(pathPolicy["reject_traversal"], true) ||
		!rawBoolIs(pathPolicy["reject_whitespace"], true) || !rawBoolIs(pathPolicy["reject_control"], true) || !rawBoolIs(pathPolicy["reject_empty_components"], true) {
		return false
	}
	canonical, ok := rawObject(extension["canonicalization"])
	if !ok || !keysExact(canonical, []string{"name", "encoding", "whitespace", "object_key_order", "artifact_order", "unique_artifact_path", "reject_noncanonical_input"}) ||
		!rawStringIs(canonical["name"], "aurum-evidence-json-v1") || !rawStringIs(canonical["encoding"], "utf-8") ||
		!rawStringIs(canonical["whitespace"], "compact") || !rawStringIs(canonical["object_key_order"], "schema-order") ||
		!rawStringIs(canonical["artifact_order"], "path-byte-ascending") || !rawBoolIs(canonical["unique_artifact_path"], true) ||
		!rawBoolIs(canonical["reject_noncanonical_input"], true) {
		return false
	}
	digest, ok := rawObject(extension["manifest_digest"])
	if !ok || !keysExact(digest, []string{"algorithm", "domain", "preimage"}) || !rawStringIs(digest["algorithm"], "sha256") ||
		!rawStringIs(digest["domain"], manifestDigestDomain) || !rawStringIs(digest["preimage"], "canonical-document-with-manifest_digest-omitted") {
		return false
	}
	authority, ok := rawObject(extension["authorization"])
	return ok && keysExact(authority, []string{"artifact_authority", "manifest_is_observation_only"}) &&
		rawStringIs(authority["artifact_authority"], "none") && rawBoolIs(authority["manifest_is_observation_only"], true)
}

func decodeObject(data []byte) (map[string]json.RawMessage, bool) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var object map[string]json.RawMessage
	if err := decoder.Decode(&object); err != nil || object == nil {
		return nil, false
	}
	var trailing json.RawMessage
	return object, decoder.Decode(&trailing) == io.EOF
}

func rawObject(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, false
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, false
	}
	return object, true
}

func rawStringIs(raw json.RawMessage, expected string) bool {
	var value string
	return len(raw) > 0 && json.Unmarshal(raw, &value) == nil && value == expected
}

func rawBoolIs(raw json.RawMessage, expected bool) bool {
	var value bool
	return len(raw) > 0 && json.Unmarshal(raw, &value) == nil && value == expected
}

func rawIntIs(raw json.RawMessage, expected int) bool {
	var value int
	return len(raw) > 0 && json.Unmarshal(raw, &value) == nil && value == expected
}

func rawStringsExact(raw json.RawMessage, expected []string) bool {
	var values []string
	if json.Unmarshal(raw, &values) != nil || len(values) != len(expected) {
		return false
	}
	for index := range values {
		if values[index] != expected[index] {
			return false
		}
	}
	return true
}

func objectStringIs(raw json.RawMessage, key, expected string) bool {
	object, ok := rawObject(raw)
	return ok && len(object) == 1 && rawStringIs(object[key], expected)
}

func objectIntIs(raw json.RawMessage, key string, expected int) bool {
	object, ok := rawObject(raw)
	return ok && len(object) == 1 && rawIntIs(object[key], expected)
}

func objectStringsExact(raw json.RawMessage, key string, expected []string) bool {
	object, ok := rawObject(raw)
	return ok && len(object) == 1 && rawStringsExact(object[key], expected)
}

func keysExact(object map[string]json.RawMessage, expected []string) bool {
	if len(object) != len(expected) {
		return false
	}
	for _, key := range expected {
		if _, exists := object[key]; !exists {
			return false
		}
	}
	return true
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func schemaReject(field string) *Error {
	return reject("schema."+field, CodeSchemaContract)
}
