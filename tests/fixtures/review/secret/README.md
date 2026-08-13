# AUR-432 secret-review fixture

Deterministic inputs for proving that a secret present in reviewed code
never reaches the model, stdout, stderr, the prompt capture, or the
report of `aurumcode review --base HEAD~1`.

Every value in this directory is synthetic, is accepted by nothing, and
must never be replaced with a real credential.

Two rules keep this directory compatible with the sealed acceptance
runner (`.board/bin/oci-run`), whose credential-shape gate refuses any
tracked input matching a real credential shape (`sk-…`, `AKIA…`,
`gh?_…`, private-key banners):

1. A tracked file here may only carry plainly-fake, non-credential-shaped
   values (the `AURUM-FAKE-*` family already planted by
   `tests/fixtures/repos/git-demo`).
2. A credential-shaped value needed by a test is assembled at runtime —
   see `assemble-credential.sh` and the split literals in
   `tests/unit/AUR-432.go` — and exists only in scratch space, never in a
   tracked file.

## Files

- `response-echoes-secret.json` — an adversarial deterministic model
  response whose finding message echoes the planted secret values back.
  The engine must redact the echoed values before the finding reaches any
  sink, while keeping the finding itself useful (file, line, key name).
- `response-cites-secret-line.json` — a well-behaved deterministic model
  response citing the planted secret line without repeating its value.
  Used to pin the published, byte-stable output format for a
  secret-free report.
- `assemble-credential.sh` — prints the runtime-assembled synthetic
  credential-shaped value (an `sk-…` shape built from split literals) for
  shell tests that need a scanner-refused shape inside a reviewed diff.

The reviewed repository these responses cite is
`tests/fixtures/repos/git-demo/repo.git`, whose HEAD commit plants the
synthetic `AURUM-FAKE-*` values in `config/demo-tokens.txt`.
