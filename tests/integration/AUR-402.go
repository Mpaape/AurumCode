package integration

import (
 "crypto/sha256"
 "encoding/hex"
 "os"
 "path/filepath"
 "testing"
)

func IntegrationAUR402(t *testing.T) { root:=os.Getenv("AURUMCODE_ROOT"); if root==""{t.Fatal("AURUMCODE_ROOT is required")}; b,e:=os.ReadFile(filepath.Join(root,".board/oci/profiles/registry.v1.json"));if e!=nil{t.Fatal(e)};h:=sha256.Sum256(b);if len(hex.EncodeToString(h[:]))!=64{t.Fatal("registry digest unavailable")} }
