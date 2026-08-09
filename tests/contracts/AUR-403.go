package contracts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func ContractAUR403(t *testing.T) {
	root := os.Getenv("AURUMCODE_ROOT"); if root == "" { t.Fatal("AURUMCODE_ROOT is required") }
	for _, path := range []string{".board/schemas/go-unit-offline-profile.schema.json", ".board/oci/profiles/go-unit-offline-v1.json", ".board/locks/oci/go-unit-offline-v1.lock.json"} { b, err := os.ReadFile(filepath.Join(root, path)); if err != nil || len(b) == 0 { t.Fatalf("unreadable %s", path) }; var value any; if json.Unmarshal(b, &value) != nil { t.Fatalf("invalid JSON %s", path) } }
}
