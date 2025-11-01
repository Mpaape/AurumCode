package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrInvalidSignature indicates the signature is invalid
	ErrInvalidSignature = errors.New("invalid webhook signature")

	// ErrMissingSignature indicates no signature header was provided
	ErrMissingSignature = errors.New("missing signature header")

	// ErrMalformedSignature indicates the signature format is invalid
	ErrMalformedSignature = errors.New("malformed signature header")
)

const (
	// SignaturePrefix is the expected prefix for GitHub signatures
	SignaturePrefix = "sha256="
)

// ValidateGitHubSignature validates a GitHub webhook signature using HMAC SHA-256
// headerValue is the value of the X-Hub-Signature-256 header
// payload is the raw request body
// secret is the GITHUB_WEBHOOK_SECRET
func ValidateGitHubSignature(headerValue string, payload []byte, secret string) error {
	if headerValue == "" {
		return ErrMissingSignature
	}

	// Check prefix
	if !strings.HasPrefix(headerValue, SignaturePrefix) {
		return fmt.Errorf("%w: expected prefix '%s'", ErrMalformedSignature, SignaturePrefix)
	}

	// Extract hex signature
	hexSignature := strings.TrimPrefix(headerValue, SignaturePrefix)
	if hexSignature == "" {
		return fmt.Errorf("%w: empty signature after prefix", ErrMalformedSignature)
	}

	// Decode hex signature
	expectedSignature, err := hex.DecodeString(hexSignature)
	if err != nil {
		return fmt.Errorf("%w: invalid hex encoding: %v", ErrMalformedSignature, err)
	}

	// Compute HMAC SHA-256
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	actualSignature := mac.Sum(nil)

	// Constant-time comparison to prevent timing attacks
	if !hmac.Equal(expectedSignature, actualSignature) {
		return ErrInvalidSignature
	}

	return nil
}

// ComputeGitHubSignature computes the HMAC SHA-256 signature for a payload
// This is useful for testing and generating signatures
func ComputeGitHubSignature(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	signature := mac.Sum(nil)
	return SignaturePrefix + hex.EncodeToString(signature)
}

// ValidateSignatureConstantTime performs constant-time comparison of signatures
// This is an internal helper for additional security
func ValidateSignatureConstantTime(expected, actual []byte) bool {
	return subtle.ConstantTimeCompare(expected, actual) == 1
}
