# AUR-019 candidate (r18) — Reviewer A — APPROVE
Lens: correctness/design. Independence: I1 (isolated context/process/nonce; same model family/provider as author).
G1 direct `bash tests/acceptance/AUR-019.sh AC-001` -> exit 0 {"result":"pass","required_providers":7,"tracked_capabilities":3}
G2 `oci-run --profile bootstrap-readonly-v1 --card AUR-019` -> "exit_code":0, stdout_sha256 749450782e00f790fcc77d7582218061638fb857962138bdf465a8a8d7fb787b
G3 `go1.21-alpine go test ./tests/integration/ -run TestAUR019Boundary` -> ok
G4 `go build ./... && go test ./...` full checkout -> ok
Re-verified 4 blockers dead (typed exit 1): date 2026-99-99 -> unsourced-provider; ancestor symlink -> undecided-provider; extra capability_foo -> undecided-provider; empty Authorization -> unfixtured-capability.
Adversarial attempts that failed closed: appended endpoint suffix, duplicate divergent provider key, delete required fixture, bedrock converse suffix, gemini key= empty, symlink components, duplicate provider name in matrix, stray matrix row, host off allowlist, NUL byte, sk- credential planted, unknown selector exit 64.
Verdict: APPROVE. Nits (non-blocking): symlink-vs-missing message wording at tests/acceptance/AUR-019.sh:1062-1063 and :930.
