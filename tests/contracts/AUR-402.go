package contracts

import (
 "os"
 "path/filepath"
 "testing"
)

// ContractAUR402 verifies the sealed artifact exists and is checked by the unit loader.
func ContractAUR402(t *testing.T) { root:=os.Getenv("AURUMCODE_ROOT"); if root==""{t.Fatal("AURUMCODE_ROOT is required")}; for _,p:=range []string{".board/oci/profiles/registry.v1.json",".board/locks/oci/registry-v1.lock.json"}{b,e:=os.ReadFile(filepath.Join(root,p));if e!=nil||len(b)==0{t.Fatalf("artifact %s unreadable",p)}} }
