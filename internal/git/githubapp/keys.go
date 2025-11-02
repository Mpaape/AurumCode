package githubapp

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
)

// LoadPrivateKeyFromFile loads RSA private key from PEM file
func LoadPrivateKeyFromFile(path string) (*rsa.PrivateKey, error) {
	keyData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file: %w", err)
	}

	return ParsePrivateKey(keyData)
}

// ParsePrivateKey parses RSA private key from PEM bytes
func ParsePrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	// Try PKCS1 format first
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err == nil {
		return key, nil
	}

	// Try PKCS8 format
	parsedKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	rsaKey, ok := parsedKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not RSA")
	}

	return rsaKey, nil
}

// LoadPrivateKeyFromEnv loads private key from environment variable
func LoadPrivateKeyFromEnv(envVar string) (*rsa.PrivateKey, error) {
	pemData := os.Getenv(envVar)
	if pemData == "" {
		return nil, fmt.Errorf("environment variable %s is not set", envVar)
	}

	return ParsePrivateKey([]byte(pemData))
}
