package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func IntegrationAUR403(t *testing.T) { root := os.Getenv("AURUMCODE_ROOT"); if root == "" { t.Fatal("AURUMCODE_ROOT is required") }; b, err := os.ReadFile(filepath.Join(root, ".board/oci/profiles/registry.v1.json")); if err != nil { t.Fatal(err) }; h := sha256.Sum256(b); if len(hex.EncodeToString(h[:])) != 64 { t.Fatal("registry digest unavailable") }; profile, err := os.ReadFile(filepath.Join(root, ".board/oci/profiles/go-unit-offline-v1.json")); if err != nil || len(profile) == 0 { t.Fatal("profile unavailable") } }
