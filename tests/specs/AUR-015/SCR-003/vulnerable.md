Fixture: VULNERABLE
Rule: SCR-003 — Cryptographic Failures (CWE-321)

A signing key is embedded as a literal in source, crossing from the
repository — an untrusted-for-secrets boundary — into the runtime
credential every signature trusts.

```go
const signingKey = "correct-horse-battery-staple-01"

func Sign(payload []byte) []byte {
    mac := hmac.New(sha256.New, []byte(signingKey))
    mac.Write(payload)
    return mac.Sum(nil)
}
```

Anyone with read access to the repository — including a forked copy, a CI
log, or a leaked artifact — recovers the signing key.
