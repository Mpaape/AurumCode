#!/usr/bin/env bash
# E2E check for AUR-460. DISTINCT ASSERTION: the exact CI-shaped command the
# achado measured against a real gateway -- `aurumcode review --base HEAD~1
# --modelo gpt-5.6-luna` -- against a LOCAL fixture gateway that reproduces
# the measured 400 whenever a request carries a "temperature" key. This is
# the outermost layer: tests/unit/AUR-460.go asserts the raw JSON in
# isolation (in-process, no socket) and tests/integration/AUR-460.go asserts
# the internal/review + internal/llm composition (also in-process); neither
# exercises cmd/aurumcode's real flag parsing, provider selection, or the
# real OS process boundary a live gateway sits behind. This program does,
# over a real loopback socket, with the compiled binary as a subprocess.
set -Eeuo pipefail
export LC_ALL=C
umask 077
ulimit -v 8388608 2>/dev/null || true
export GOMEMLIMIT=2GiB

readonly card='AUR-460'
selector="${1:-E2EAUR460}"
case "$selector" in
  E2EAUR460) ;;
  *) printf '%s/E2E/unknown-selector\n' "$card" >&2; exit 64 ;;
esac

fail() { printf '%s/E2E/%s\n' "$card" "$1" >&2; exit 1; }
infra() { printf '%s/E2E/infrastructure/%s\n' "$card" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
root="${AURUMCODE_ROOT:-$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)}" || infra root
demo="$root/tests/fixtures/repos/git-demo/repo.git"
[[ -d "$demo" ]] || infra missing-demo-fixture

work="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e2e460.XXXXXX")" || infra mktemp
gateway_pid=''
cleanup() {
  [[ -z "$gateway_pid" ]] || kill "$gateway_pid" >/dev/null 2>&1 || true
  rm -rf -- "$work" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM HUP

command -v go >/dev/null 2>&1 || infra missing-go

bin="${AURUMCODE_BIN:-}"
if [[ -z "$bin" ]]; then
  bin="$work/aurumcode"
  (cd "$root" && go build -mod=mod -o "$bin" ./cmd/aurumcode) >"$work/build.log" 2>&1 || { cat "$work/build.log" >&2; infra build-failed; }
fi

# --- the fixture gateway: reproduces the measured gpt-5.6-luna behavior --
# a 400 for ANY request carrying "temperature", a real completion otherwise.
mkdir -p "$work/gatewaysrc"
cat > "$work/gatewaysrc/main.go" <<'GATEWAY_EOF'
package main

import (
	"io"
	"net"
	"net/http"
	"os"
	"strings"
)

func main() {
	addrFile, captureFile := os.Args[1], os.Args[2]
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		os.Exit(1)
	}
	if err := os.WriteFile(addrFile, []byte(ln.Addr().String()), 0o600); err != nil {
		os.Exit(1)
	}
	http.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = os.WriteFile(captureFile, raw, 0o600)
		if strings.Contains(string(raw), `"temperature"`) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"Unsupported value: 'temperature' does not support 0.3 with this model. Only the default (1) value is supported."}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cmpl-e2e460","object":"chat.completion","created":0,"model":"gpt-5.6-luna","choices":[{"index":0,"message":{"role":"assistant","content":"{\"issues\":[{\"file\":\"config/demo-tokens.txt\",\"line\":3,\"severity\":\"error\",\"rule_id\":\"security/hardcoded-secret\",\"message\":\"A credential-shaped value was committed in plain text.\"}],\"summary\":\"One planted credential was found.\"}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`))
	})
	_ = http.Serve(ln, nil)
}
GATEWAY_EOF

gateway_bin="$work/fakegateway"
(cd "$work/gatewaysrc" && go mod init aur460gateway >/dev/null 2>&1 && go build -mod=mod -o "$gateway_bin" .) >"$work/gateway-build.log" 2>&1 || { cat "$work/gateway-build.log" >&2; infra gateway-build-failed; }

addr_file="$work/gateway.addr"
capture_file="$work/gateway.capture"
"$gateway_bin" "$addr_file" "$capture_file" &
gateway_pid=$!

# Bounded readiness poll: the addr file appears the instant Listen()
# succeeds; this is not a fixed sleep for an unknown-duration operation.
tries=0
while [[ ! -s "$addr_file" ]]; do
  tries=$((tries + 1))
  (( tries < 200 )) || infra gateway-never-became-ready
  kill -0 "$gateway_pid" >/dev/null 2>&1 || infra gateway-exited-early
  sleep 0.05
done
gateway_addr="$(cat "$addr_file")"

noprov() { env -u AURUMCODE_LLM_FIXTURE -u LLM_API_KEY -u LLM_BASE_URL -u LLM_MODEL "$@"; }

# --- THE MEASURED COMMAND, AGAINST A LOCAL GATEWAY THAT REPRODUCES THE
#     DEFECT: `--modelo gpt-5.6-luna` must now reach it and succeed. -------
set +e
out="$(cd "$demo" && noprov env LLM_API_KEY=test-key LLM_BASE_URL="http://$gateway_addr" \
  "$bin" review --base HEAD~1 --modelo gpt-5.6-luna 2>"$work/err.txt")"
rc=$?
set -e
err="$(cat "$work/err.txt")"

[[ "$rc" -eq 0 ]] || fail "gpt-5.6-luna-must-succeed:rc=$rc:stderr=$err"
[[ -s "$capture_file" ]] || fail gateway-never-received-a-request
if grep -Fq '"temperature"' "$capture_file"; then
  fail "request-carried-temperature:$(cat "$capture_file")"
fi
grep -Fq 'config/demo-tokens.txt:3:' <<<"$out" || fail "finding-lost:stdout=$out"
grep -Fq 'hardcoded-secret' <<<"$out" || fail "rule-citation-lost:stdout=$out"

# --- DETERMINISM: repeating the same call against the same gateway
#     produces the same stdout. -------------------------------------------
out2="$(cd "$demo" && noprov env LLM_API_KEY=test-key LLM_BASE_URL="http://$gateway_addr" \
  "$bin" review --base HEAD~1 --modelo gpt-5.6-luna 2>/dev/null)"
[[ "$out" == "$out2" ]] || fail nondeterministic-output

printf '%s/E2E/ok\n' "$card"
exit 0
