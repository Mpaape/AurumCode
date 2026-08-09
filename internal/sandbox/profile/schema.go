// Package profile validates the immutable OCI bootstrap profile before an
// engine-specific command can be constructed.
package profile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

const (
	SchemaName        = "aurum.container-profile"
	ProfileName       = "bootstrap-readonly-v1"
	ProfileVersion    = 1
	ImageLockSchema   = "aurum.oci-image-lock"
	ImageLockPath     = ".board/locks/oci/bootstrap-readonly-v1.lock.json"
	engineInvocations = 0
)

var (
	userPattern    = regexp.MustCompile(`^[1-9][0-9]*:[1-9][0-9]*$`)
	digestPattern  = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	imagePattern   = regexp.MustCompile(`^[a-z0-9]+(([._]|__|-+)[a-z0-9]+)*(/[a-z0-9]+(([._]|__|-+)[a-z0-9]+)*)*@sha256:[0-9a-f]{64}$`)
	requiredFields = []string{
		"schema", "version", "profile", "lock", "lock_digest", "network", "user",
		"cap_drop", "cap_add", "mounts", "devices", "pull", "tmpfs", "read_only_rootfs",
		"no_new_privileges", "privileged", "timeout_seconds", "memory_mb", "cpu_millis",
		"pids_limit", "tmpfs_mb", "stdout_limit_bytes", "stderr_limit_bytes", "max_input_files", "max_input_bytes",
	}
)

// ValidationResult is the complete observation from one validation attempt.
// EngineInvocations is intentionally part of the result so callers cannot
// mistake validation for execution.
type ValidationResult struct {
	Status            string `json:"status"`
	Code              string `json:"code"`
	DocumentDigest    string `json:"document_digest"`
	EngineInvocations int    `json:"engine_invocations"`
}

// Profile is the strict bootstrap-readonly-v1 wire representation.
type Profile struct {
	Schema           string `json:"schema"`
	Version          int    `json:"version"`
	Profile          string `json:"profile"`
	Lock             string `json:"lock"`
	LockDigest       string `json:"lock_digest"`
	Network          string `json:"network"`
	User             string `json:"user"`
	CapDrop          string `json:"cap_drop"`
	CapAdd           string `json:"cap_add"`
	Mounts           string `json:"mounts"`
	Devices          string `json:"devices"`
	Pull             string `json:"pull"`
	Tmpfs            string `json:"tmpfs"`
	CheckoutReadonly *bool  `json:"checkout_readonly,omitempty"`
	ReadOnlyRootfs   bool   `json:"read_only_rootfs"`
	NoNewPrivileges  bool   `json:"no_new_privileges"`
	Privileged       bool   `json:"privileged"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
	MemoryMB         int    `json:"memory_mb"`
	CPUMillis        int    `json:"cpu_millis"`
	PIDsLimit        int    `json:"pids_limit"`
	TmpfsMB          int    `json:"tmpfs_mb"`
	StdoutLimitBytes int    `json:"stdout_limit_bytes"`
	StderrLimitBytes int    `json:"stderr_limit_bytes"`
	MaxInputFiles    int    `json:"max_input_files"`
	MaxInputBytes    int    `json:"max_input_bytes"`
}

type imageLock struct {
	Schema  string `json:"schema"`
	Version int    `json:"version"`
	Profile string `json:"profile"`
	Image   string `json:"image"`
}

// ValidateBootstrapProfile validates one profile and one digest-pinned image
// lock. It does not contact an OCI engine, a registry, or the network.
func ValidateBootstrapProfile(document, schema, lockManifest []byte) ValidationResult {
	result := invalidResult(document, "schema_invalid")
	if !strictSchema(schema) {
		return result
	}

	fields, err := objectFields(document)
	if err != nil {
		return result
	}
	for _, key := range requiredFields {
		if _, ok := fields[key]; !ok {
			return result
		}
	}

	if code := classifySecurityViolation(fields); code != "" {
		return invalidResult(document, code)
	}

	var p Profile
	if err := strictDecode(document, &p); err != nil {
		return invalidResult(document, "schema_invalid")
	}
	if code := validateProfile(p); code != "" {
		return invalidResult(document, code)
	}

	var lock imageLock
	if code := validateImageLock(lockManifest, &lock); code != "" {
		return invalidResult(document, code)
	}
	lockDigest := digest(lockManifest)
	if p.LockDigest != lockDigest {
		return invalidResult(document, "lock_manifest_mismatch")
	}

	return ValidationResult{
		Status:            "valid",
		Code:              "valid",
		DocumentDigest:    digest(document),
		EngineInvocations: engineInvocations,
	}
}

func invalidResult(document []byte, code string) ValidationResult {
	return ValidationResult{
		Status:            "invalid",
		Code:              code,
		DocumentDigest:    digest(document),
		EngineInvocations: engineInvocations,
	}
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func strictDecode(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return nil
}

func objectFields(data []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := token.(json.Delim)
	if !ok || delim != '{' {
		return nil, fmt.Errorf("profile is not an object")
	}

	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyToken.(string)
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid object key")
		}
		if _, exists := fields[key]; exists {
			return nil, fmt.Errorf("duplicate object key")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		fields[key] = value
	}
	if token, err = decoder.Token(); err != nil {
		return nil, err
	} else if closing, ok := token.(json.Delim); !ok || closing != '}' {
		return nil, fmt.Errorf("object is not closed")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("trailing JSON value")
		}
		return nil, err
	}
	return fields, nil
}

func classifySecurityViolation(fields map[string]json.RawMessage) string {
	if raw, ok := fields["user"]; ok {
		user, valid := stringFieldValue(raw)
		if !valid {
			return "schema_invalid"
		}
		parts := strings.Split(user, ":")
		if len(parts) == 2 && (isZero(parts[0]) || isZero(parts[1])) {
			return "root_forbidden"
		}
	}

	if raw, ok := fields["privileged"]; ok {
		privileged, valid := boolValue(raw)
		if !valid {
			return "schema_invalid"
		}
		if privileged {
			return "privileged_forbidden"
		}
	}

	if raw, ok := fields["checkout_readonly"]; ok {
		readonly, valid := boolValue(raw)
		if !valid {
			return "schema_invalid"
		}
		if !readonly {
			return "checkout_must_be_readonly"
		}
	}

	if raw, ok := fields["mounts"]; ok {
		mounts, valid := stringFieldValue(raw)
		if !valid {
			return "schema_invalid"
		}
		lower := strings.ToLower(mounts)
		if strings.Contains(lower, "docker.sock") || strings.Contains(lower, "podman.sock") || strings.Contains(lower, "containerd.sock") {
			return "engine_socket_forbidden"
		}
		if strings.Contains(lower, "checkout") && (strings.Contains(lower, ":rw") || strings.Contains(lower, "readonly=false")) {
			return "checkout_must_be_readonly"
		}
		if mounts != "none" {
			return "host_mount_forbidden"
		}
	}

	if raw, ok := fields["devices"]; ok {
		devices, valid := stringFieldValue(raw)
		if !valid {
			return "schema_invalid"
		}
		if devices != "none" {
			return "device_forbidden"
		}
	}

	for _, key := range []string{"cap_add", "cap_drop"} {
		raw, ok := fields[key]
		if !ok {
			continue
		}
		value, valid := stringFieldValue(raw)
		if !valid {
			return "schema_invalid"
		}
		if (key == "cap_add" && value != "none") || (key == "cap_drop" && value != "ALL") {
			return "capability_forbidden"
		}
	}

	if raw, ok := fields["network"]; ok {
		network, valid := stringFieldValue(raw)
		if !valid {
			return "schema_invalid"
		}
		if network != "none" {
			return "egress_forbidden"
		}
	}

	limits := map[string][2]int64{
		"timeout_seconds":    {1, 120},
		"memory_mb":          {1, 256},
		"cpu_millis":         {1, 1000},
		"pids_limit":         {1, 128},
		"tmpfs_mb":           {1, 128},
		"stdout_limit_bytes": {1, 65536},
		"stderr_limit_bytes": {1, 65536},
		"max_input_files":    {1, 10000},
		"max_input_bytes":    {1, 67108864},
	}
	for key, bounds := range limits {
		raw, ok := fields[key]
		if !ok {
			return "resources_must_be_bounded"
		}
		value, valid := integerValue(raw)
		if !valid {
			return "schema_invalid"
		}
		if value < bounds[0] || value > bounds[1] {
			return "resources_must_be_bounded"
		}
	}

	return ""
}

func isZero(value string) bool {
	parsed, err := strconv.ParseUint(value, 10, 64)
	return err == nil && parsed == 0
}

func stringFieldValue(raw json.RawMessage) (string, bool) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", false
	}
	parsed, ok := value.(string)
	if !ok {
		return "", false
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return "", false
	}
	return parsed, true
}

func integerValue(raw json.RawMessage) (int64, bool) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return 0, false
	}
	number, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	parsed, err := number.Int64()
	if err != nil {
		return 0, false
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return 0, false
	}
	return parsed, true
}

func validateProfile(profile Profile) string {
	if profile.Schema != SchemaName || profile.Version != ProfileVersion || profile.Profile != ProfileName || profile.Lock != ImageLockPath {
		return "schema_invalid"
	}
	if !digestPattern.MatchString(profile.LockDigest) || !userPattern.MatchString(profile.User) {
		return "schema_invalid"
	}
	if profile.Network != "none" || profile.Pull != "never" || profile.Tmpfs != "rw,nosuid,nodev" {
		return "schema_invalid"
	}
	if profile.CapDrop != "ALL" || profile.CapAdd != "none" || profile.Mounts != "none" || profile.Devices != "none" {
		return "schema_invalid"
	}
	if !profile.ReadOnlyRootfs || !profile.NoNewPrivileges || profile.Privileged {
		return "schema_invalid"
	}
	limits := [][3]int64{
		{int64(profile.TimeoutSeconds), 1, 120},
		{int64(profile.MemoryMB), 1, 256},
		{int64(profile.CPUMillis), 1, 1000},
		{int64(profile.PIDsLimit), 1, 128},
		{int64(profile.TmpfsMB), 1, 128},
		{int64(profile.StdoutLimitBytes), 1, 65536},
		{int64(profile.StderrLimitBytes), 1, 65536},
		{int64(profile.MaxInputFiles), 1, 10000},
		{int64(profile.MaxInputBytes), 1, 67108864},
	}
	for _, limit := range limits {
		if limit[0] < limit[1] || limit[0] > limit[2] {
			return "resources_must_be_bounded"
		}
	}
	return ""
}

func validateImageLock(data []byte, lock *imageLock) string {
	fields, err := objectFields(data)
	if err != nil {
		return "lock_manifest_invalid"
	}
	imageRaw, ok := fields["image"]
	if !ok {
		return "lock_manifest_invalid"
	}
	if _, ok := stringFieldValue(imageRaw); !ok {
		return "lock_manifest_invalid"
	}
	if err := strictDecode(data, lock); err != nil {
		return "lock_manifest_invalid"
	}
	if lock.Schema != ImageLockSchema || lock.Version != ProfileVersion || lock.Profile != ProfileName {
		return "lock_manifest_invalid"
	}
	if !imagePattern.MatchString(lock.Image) {
		return "image_digest_required"
	}
	return ""
}

func strictSchema(data []byte) bool {
	schema, err := objectFields(data)
	if err != nil {
		return false
	}
	additionalProperties, ok := boolValue(schema["additionalProperties"])
	if stringValue(schema["type"]) != "object" || !ok || additionalProperties {
		return false
	}
	var required []string
	if json.Unmarshal(schema["required"], &required) != nil {
		return false
	}
	for _, key := range requiredFields {
		if !contains(required, key) {
			return false
		}
	}
	var properties map[string]json.RawMessage
	if json.Unmarshal(schema["properties"], &properties) != nil {
		return false
	}
	for key, expected := range map[string]string{
		"schema":   SchemaName,
		"profile":  ProfileName,
		"lock":     ImageLockPath,
		"network":  "none",
		"cap_drop": "ALL",
		"cap_add":  "none",
		"mounts":   "none",
		"devices":  "none",
		"pull":     "never",
		"tmpfs":    "rw,nosuid,nodev",
	} {
		if stringValue(propertyRaw(properties, key, "const")) != expected {
			return false
		}
	}
	version, versionOK := numberValue(propertyRaw(properties, "version", "const"))
	readOnly, readOnlyOK := boolValue(propertyRaw(properties, "read_only_rootfs", "const"))
	noNewPrivileges, noNewPrivilegesOK := boolValue(propertyRaw(properties, "no_new_privileges", "const"))
	privileged, privilegedOK := boolValue(propertyRaw(properties, "privileged", "const"))
	if !versionOK || version != 1 || !readOnlyOK || !readOnly || !noNewPrivilegesOK || !noNewPrivileges || !privilegedOK || privileged {
		return false
	}
	checkoutReadonly, checkoutReadonlyOK := boolValue(propertyRaw(properties, "checkout_readonly", "const"))
	if !checkoutReadonlyOK || !checkoutReadonly ||
		stringValue(propertyRaw(properties, "user", "type")) != "string" ||
		stringValue(propertyRaw(properties, "user", "pattern")) != userPattern.String() ||
		stringValue(propertyRaw(properties, "lock_digest", "type")) != "string" ||
		stringValue(propertyRaw(properties, "lock_digest", "pattern")) != digestPattern.String() {
		return false
	}
	for key, bounds := range map[string][2]int64{
		"timeout_seconds":    {1, 120},
		"memory_mb":          {1, 256},
		"cpu_millis":         {1, 1000},
		"pids_limit":         {1, 128},
		"tmpfs_mb":           {1, 128},
		"stdout_limit_bytes": {1, 65536},
		"stderr_limit_bytes": {1, 65536},
		"max_input_files":    {1, 10000},
		"max_input_bytes":    {1, 67108864},
	} {
		property := properties[key]
		var object map[string]json.RawMessage
		if json.Unmarshal(property, &object) != nil {
			return false
		}
		if stringValue(object["type"]) != "integer" {
			return false
		}
		minimum, minimumOK := numberValue(object["minimum"])
		maximum, maximumOK := numberValue(object["maximum"])
		if !minimumOK || !maximumOK || minimum != bounds[0] || maximum != bounds[1] {
			return false
		}
	}
	var definitions map[string]json.RawMessage
	if json.Unmarshal(schema["$defs"], &definitions) != nil {
		return false
	}
	lockDefinition, ok := definitions["ociImageLock"]
	if !ok {
		return false
	}
	var lockObject map[string]json.RawMessage
	if json.Unmarshal(lockDefinition, &lockObject) != nil ||
		stringValue(lockObject["type"]) != "object" {
		return false
	}
	lockAdditionalProperties, lockAdditionalPropertiesOK := boolValue(lockObject["additionalProperties"])
	if !lockAdditionalPropertiesOK || lockAdditionalProperties {
		return false
	}
	var lockRequired []string
	if json.Unmarshal(lockObject["required"], &lockRequired) != nil {
		return false
	}
	for _, key := range []string{"schema", "version", "profile", "image"} {
		if !contains(lockRequired, key) {
			return false
		}
	}
	var lockProperties map[string]json.RawMessage
	if json.Unmarshal(lockObject["properties"], &lockProperties) != nil {
		return false
	}
	if stringValue(propertyRaw(lockProperties, "schema", "const")) != ImageLockSchema ||
		stringValue(propertyRaw(lockProperties, "profile", "const")) != ProfileName ||
		!numberValueMatches(propertyRaw(lockProperties, "version", "const"), ProfileVersion) ||
		stringValue(propertyRaw(lockProperties, "image", "type")) != "string" ||
		stringValue(propertyRaw(lockProperties, "image", "pattern")) != imagePattern.String() {
		return false
	}
	return true
}

func numberValueMatches(raw json.RawMessage, expected int) bool {
	value, ok := numberValue(raw)
	return ok && value == int64(expected)
}

func propertyRaw(properties map[string]json.RawMessage, key, field string) json.RawMessage {
	var property map[string]json.RawMessage
	if json.Unmarshal(properties[key], &property) != nil {
		return nil
	}
	return property[field]
}

func stringValue(raw json.RawMessage) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}

func boolValue(raw json.RawMessage) (bool, bool) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var value any
	if err := decoder.Decode(&value); err != nil {
		return false, false
	}
	parsed, ok := value.(bool)
	if !ok {
		return false, false
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return false, false
	}
	return parsed, true
}

func numberValue(raw json.RawMessage) (int64, bool) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return 0, false
	}
	parsed, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	number, err := parsed.Int64()
	if err != nil {
		return 0, false
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return 0, false
	}
	return number, true
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
