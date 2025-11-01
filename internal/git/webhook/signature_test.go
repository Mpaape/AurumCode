package webhook

import (
	"errors"
	"testing"
)

func TestValidateGitHubSignature_Valid(t *testing.T) {
	secret := "test-secret"
	payload := []byte(`{"test": "payload"}`)

	// Compute valid signature
	signature := ComputeGitHubSignature(payload, secret)

	err := ValidateGitHubSignature(signature, payload, secret)
	if err != nil {
		t.Errorf("expected no error for valid signature, got: %v", err)
	}
}

func TestValidateGitHubSignature_InvalidSignature(t *testing.T) {
	secret := "test-secret"
	payload := []byte(`{"test": "payload"}`)

	// Compute signature with wrong secret
	wrongSignature := ComputeGitHubSignature(payload, "wrong-secret")

	err := ValidateGitHubSignature(wrongSignature, payload, secret)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("expected ErrInvalidSignature, got: %v", err)
	}
}

func TestValidateGitHubSignature_MissingSignature(t *testing.T) {
	secret := "test-secret"
	payload := []byte(`{"test": "payload"}`)

	err := ValidateGitHubSignature("", payload, secret)
	if !errors.Is(err, ErrMissingSignature) {
		t.Errorf("expected ErrMissingSignature, got: %v", err)
	}
}

func TestValidateGitHubSignature_MissingPrefix(t *testing.T) {
	secret := "test-secret"
	payload := []byte(`{"test": "payload"}`)

	// Signature without prefix
	signature := "abc123"

	err := ValidateGitHubSignature(signature, payload, secret)
	if !errors.Is(err, ErrMalformedSignature) {
		t.Errorf("expected ErrMalformedSignature, got: %v", err)
	}
}

func TestValidateGitHubSignature_EmptyAfterPrefix(t *testing.T) {
	secret := "test-secret"
	payload := []byte(`{"test": "payload"}`)

	// Just the prefix, no signature
	signature := SignaturePrefix

	err := ValidateGitHubSignature(signature, payload, secret)
	if !errors.Is(err, ErrMalformedSignature) {
		t.Errorf("expected ErrMalformedSignature, got: %v", err)
	}
}

func TestValidateGitHubSignature_InvalidHex(t *testing.T) {
	secret := "test-secret"
	payload := []byte(`{"test": "payload"}`)

	// Invalid hex characters
	signature := SignaturePrefix + "zzzzinvalidhex"

	err := ValidateGitHubSignature(signature, payload, secret)
	if !errors.Is(err, ErrMalformedSignature) {
		t.Errorf("expected ErrMalformedSignature for invalid hex, got: %v", err)
	}
}

func TestValidateGitHubSignature_DifferentPayload(t *testing.T) {
	secret := "test-secret"
	payload1 := []byte(`{"test": "payload1"}`)
	payload2 := []byte(`{"test": "payload2"}`)

	// Compute signature for payload1
	signature := ComputeGitHubSignature(payload1, secret)

	// Validate against payload2 (should fail)
	err := ValidateGitHubSignature(signature, payload2, secret)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("expected ErrInvalidSignature for different payload, got: %v", err)
	}
}

func TestComputeGitHubSignature(t *testing.T) {
	secret := "test-secret"
	payload := []byte(`{"test": "payload"}`)

	signature := ComputeGitHubSignature(payload, secret)

	// Should have correct prefix
	if len(signature) < len(SignaturePrefix) {
		t.Errorf("signature too short: %s", signature)
	}

	if signature[:len(SignaturePrefix)] != SignaturePrefix {
		t.Errorf("expected prefix '%s', got: %s", SignaturePrefix, signature[:len(SignaturePrefix)])
	}

	// Should be deterministic
	signature2 := ComputeGitHubSignature(payload, secret)
	if signature != signature2 {
		t.Errorf("signature not deterministic: %s != %s", signature, signature2)
	}
}

func TestValidateSignatureConstantTime(t *testing.T) {
	sig1 := []byte("signature")
	sig2 := []byte("signature")
	sig3 := []byte("different")

	if !ValidateSignatureConstantTime(sig1, sig2) {
		t.Error("expected identical signatures to match")
	}

	if ValidateSignatureConstantTime(sig1, sig3) {
		t.Error("expected different signatures to not match")
	}
}

// TestValidateGitHubSignature_RealExample tests with a realistic GitHub webhook example
func TestValidateGitHubSignature_RealExample(t *testing.T) {
	secret := "my-webhook-secret"
	payload := []byte(`{"zen":"Design for failure.","hook_id":123456}`)

	// Generate signature as GitHub would
	signature := ComputeGitHubSignature(payload, secret)

	// Should validate successfully
	err := ValidateGitHubSignature(signature, payload, secret)
	if err != nil {
		t.Errorf("expected no error for realistic example, got: %v", err)
	}
}

// BenchmarkValidateGitHubSignature measures signature validation performance
func BenchmarkValidateGitHubSignature(b *testing.B) {
	secret := "test-secret"
	payload := []byte(`{"test": "payload"}`)
	signature := ComputeGitHubSignature(payload, secret)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ValidateGitHubSignature(signature, payload, secret)
	}
}

// BenchmarkComputeGitHubSignature measures signature computation performance
func BenchmarkComputeGitHubSignature(b *testing.B) {
	secret := "test-secret"
	payload := []byte(`{"test": "payload"}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ComputeGitHubSignature(payload, secret)
	}
}
