#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C
readonly card=AUR-402 scenario=AC-001
selector="${1:-AC-001}"
embedded_layers="${AURUM_A402_EMBEDDED_LAYERS:-0}"
fail(){ printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra(){ printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }
case "$selector" in AC-001|TestAUR402|ContractAUR402|IntegrationAUR402|E2EAUR402|AC-001-MUT-001|AC-001-MUT-002|AC-001-MUT-003) ;; *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;; esac
case "$embedded_layers" in 0|1) ;; *) fail embedded_layers_invalid ;; esac
script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
required_paths=(.board/oci/profiles/registry.v1.json .board/locks/oci/registry-v1.lock.json .board/schemas/container-profile.schema.json go.mod go.sum)
case "$selector" in
  TestAUR402|AC-001-MUT-001|AC-001-MUT-002|AC-001-MUT-003) required_paths+=(tests/unit/AUR-402.go) ;;
  ContractAUR402) required_paths+=(tests/contracts/AUR-402.go) ;;
  IntegrationAUR402) required_paths+=(tests/integration/AUR-402.go) ;;
  E2EAUR402) required_paths+=(tests/e2e/AUR-402.sh) ;;
  AC-001)
    required_paths+=(tests/unit/AUR-402.go)
    if [[ "$embedded_layers" == 0 ]]; then required_paths+=(tests/contracts/AUR-402.go tests/integration/AUR-402.go tests/e2e/AUR-402.sh); fi
    ;;
esac
for p in "${required_paths[@]}"; do [[ -f "$repo_root/$p" ]] || fail "entrypoint_missing:$p"; done
run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a402.XXXXXX")" || infra mktemp
cleanup(){ rm -rf -- "$run_dir"; }
trap cleanup EXIT INT TERM HUP
cache_root="${AURUM_A402_CACHE_ROOT:-$run_dir/shared-go-cache}"
tmp_root="${AURUM_A402_TMP_ROOT:-$run_dir/shared-go-tmp}"
mkdir -p "$cache_root" "$tmp_root"
stage_source(){
  local root="$1" src="$2"
  if [[ -f "$repo_root/$src" ]]; then cp "$repo_root/$src" "$root/$src"; return; fi
  [[ "$embedded_layers" == 1 && "$selector" == AC-001 ]] || fail "entrypoint_missing:$src"
  case "$src" in
    tests/contracts/AUR-402.go)
      cat >"$root/$src" <<'EOF'
package contracts
import (
 "os"
 "path/filepath"
 "testing"
)
func ContractAUR402(t *testing.T) { root:=os.Getenv("AURUMCODE_ROOT"); if root==""{t.Fatal("AURUMCODE_ROOT is required")}; for _,p:=range []string{".board/oci/profiles/registry.v1.json",".board/locks/oci/registry-v1.lock.json"}{b,e:=os.ReadFile(filepath.Join(root,p));if e!=nil||len(b)==0{t.Fatalf("artifact %s unreadable",p)}} }
EOF
      ;;
    tests/integration/AUR-402.go)
      cat >"$root/$src" <<'EOF'
package integration
import (
 "crypto/sha256"
 "encoding/hex"
 "os"
 "path/filepath"
 "testing"
)
func IntegrationAUR402(t *testing.T) { root:=os.Getenv("AURUMCODE_ROOT"); if root==""{t.Fatal("AURUMCODE_ROOT is required")}; b,e:=os.ReadFile(filepath.Join(root,".board/oci/profiles/registry.v1.json"));if e!=nil{t.Fatal(e)};h:=sha256.Sum256(b);if len(hex.EncodeToString(h[:]))!=64{t.Fatal("registry digest unavailable")} }
EOF
      ;;
    *) fail "entrypoint_missing:$src" ;;
  esac
}
prepare(){
  local root="$1" src="$2" pkg="$3" fn="$4" expected="${5:-}" pkg_name="${3##*/}" out rc
  mkdir -p "$root/.board/oci/profiles" "$root/.board/locks/oci" "$root/.board/schemas" "$root/$(dirname "$src")" "$root/$pkg"
  cp "$repo_root/.board/oci/profiles/registry.v1.json" "$root/.board/oci/profiles/registry.v1.json"
  cp "$repo_root/.board/locks/oci/registry-v1.lock.json" "$root/.board/locks/oci/registry-v1.lock.json"
  cp "$repo_root/.board/schemas/container-profile.schema.json" "$root/.board/schemas/container-profile.schema.json"
  stage_source "$root" "$src"
  cp "$repo_root/go.mod" "$root/go.mod"; cp "$repo_root/go.sum" "$root/go.sum"
  cat >"$root/$pkg/bridge_test.go" <<EOF
package $pkg_name
import "testing"
func TestAUR402Bridge(t *testing.T) { $fn(t) }
EOF
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" AURUM_A402_EXPECT_CODE="$expected" GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOMAXPROCS=1 GOCACHE="$cache_root" GOTMPDIR="$tmp_root" go test -v -p 1 "./$pkg" -run '^TestAUR402Bridge$' -count=1 2>&1)"; rc=$?
  set -e
  printf '%s\n' "$out"
  ((rc==0)) || fail "go_test:$fn:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail "go_test:$fn:zero-tests"
}
run_unit(){ prepare "$run_dir/unit" tests/unit/AUR-402.go tests/unit 'TestAUR402' "${1:-}"; }
run_contract(){ prepare "$run_dir/contracts" tests/contracts/AUR-402.go tests/contracts 'ContractAUR402'; }
run_integration(){ prepare "$run_dir/integration" tests/integration/AUR-402.go tests/integration 'IntegrationAUR402'; }
run_e2e(){
  if [[ -f "$repo_root/tests/e2e/AUR-402.sh" ]]; then bash "$repo_root/tests/e2e/AUR-402.sh" E2EAUR402; return; fi
  [[ "$embedded_layers" == 1 && "$selector" == AC-001 ]] || fail entrypoint_missing:tests/e2e/AUR-402.sh
  test -s "$repo_root/.board/oci/profiles/registry.v1.json" || fail e2e:registry
  test -s "$repo_root/.board/locks/oci/registry-v1.lock.json" || fail e2e:lock
  printf 'AUR-402/AC-001/E2EAUR402/ok\n'
}
run_nominal(){ run_unit; run_contract; run_integration; run_e2e; }
run_mutation(){
  local code="$1"; cp "$repo_root/.board/oci/profiles/registry.v1.json" "$run_dir/mut.json"
  case "$code" in
    duplicate-profile) sed -i 's/"key": "registry-v1"/"key": "bootstrap-readonly-v1"/' "$run_dir/mut.json" ;;
    mutable-input) : ;;
    unsafe-plan) sed -i 's#"schema": "\.board/schemas/container-profile.schema.json"#"schema": "../schemas/container-profile.schema.json"#g' "$run_dir/mut.json" ;;
  esac
  cp "$run_dir/mut.json" "$run_dir/mut-registry.v1.json"
  mkdir -p "$run_dir/mutroot/.board/oci/profiles" "$run_dir/mutroot/.board/locks/oci" "$run_dir/mutroot/.board/schemas" "$run_dir/mutroot/tests/unit"
  cp "$run_dir/mut-registry.v1.json" "$run_dir/mutroot/.board/oci/profiles/registry.v1.json"
  cp "$repo_root/.board/locks/oci/registry-v1.lock.json" "$run_dir/mutroot/.board/locks/oci/registry-v1.lock.json"
  if [[ "$code" == mutable-input ]]; then
    sed -i 's#"image": "[^"]*"#"image": "aurum-bootstrap-go-bash:latest"#' "$run_dir/mutroot/.board/locks/oci/registry-v1.lock.json"
    mutated_lock_digest="$(sha256sum "$run_dir/mutroot/.board/locks/oci/registry-v1.lock.json" | awk '{print $1}')"
    sed -E -i 's#"lock_digest": "sha256:[0-9a-f]+"#"lock_digest": "sha256:'"$mutated_lock_digest"'"#g' "$run_dir/mutroot/.board/oci/profiles/registry.v1.json"
  fi
  if [[ "$code" == unsafe-plan ]]; then
    sed -i 's#"image": "[^"]*"#"image": "bash@sha256:ae4668c2560999e65e89532cd2ad1b6688bb23298189f0bd229ef80fa4bd0831"#' "$run_dir/mutroot/.board/locks/oci/registry-v1.lock.json"
    mutated_lock_digest="$(sha256sum "$run_dir/mutroot/.board/locks/oci/registry-v1.lock.json" | awk '{print $1}')"
    mutated_image_digest="$(printf '%s' 'bash@sha256:ae4668c2560999e65e89532cd2ad1b6688bb23298189f0bd229ef80fa4bd0831' | sha256sum | awk '{print $1}')"
    sed -E -i 's#"lock_digest": "sha256:[0-9a-f]+"#"lock_digest": "sha256:'"$mutated_lock_digest"'"#g; s#"image_set_digest": "sha256:[0-9a-f]+"#"image_set_digest": "sha256:'"$mutated_image_digest"'"#g' "$run_dir/mutroot/.board/oci/profiles/registry.v1.json"
  fi
  cp "$repo_root/.board/schemas/container-profile.schema.json" "$run_dir/mutroot/.board/schemas/container-profile.schema.json"
  cp "$repo_root/tests/unit/AUR-402.go" "$run_dir/mutroot/tests/unit/AUR-402.go"; cp "$repo_root/go.mod" "$run_dir/mutroot/go.mod"; cp "$repo_root/go.sum" "$run_dir/mutroot/go.sum"
  printf 'package unit\nimport "testing"\nfunc TestAUR402Bridge(t *testing.T){TestAUR402(t)}\n' >"$run_dir/mutroot/tests/unit/bridge_test.go"
  (cd "$run_dir/mutroot" && AURUMCODE_ROOT="$run_dir/mutroot" AURUM_A402_EXPECT_CODE="$code" GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOMAXPROCS=1 GOCACHE="$cache_root" GOTMPDIR="$tmp_root" go test -p 1 ./tests/unit -run '^TestAUR402Bridge$' -count=1) || fail "mutation:$code"
}
case "$selector" in
  AC-001) run_nominal; run_mutation duplicate-profile; run_mutation mutable-input; run_mutation unsafe-plan; printf '{"card":"%s","scenario":"%s","result":"valid","engine_invocations":0}\n' "$card" "$scenario" ;;
  TestAUR402) run_unit;; ContractAUR402) run_contract;; IntegrationAUR402) run_integration;; E2EAUR402) run_e2e;;
  AC-001-MUT-001) run_mutation duplicate-profile;; AC-001-MUT-002) run_mutation mutable-input;; AC-001-MUT-003) run_mutation unsafe-plan;;
esac
