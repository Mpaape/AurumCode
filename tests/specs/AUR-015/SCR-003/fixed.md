Fixture: FIXED
Rule: SCR-003 — Cryptographic Failures (CWE-321)

The key is read from the runtime secret channel at start-up, never
committed, and its absence fails closed instead of falling back to a
literal.

```go
func Sign(payload []byte, key []byte) ([]byte, error) {
    if len(key) < 32 {
        return nil, errors.New("signing key missing or too short")
    }
    mac := hmac.New(sha256.New, key)
    mac.Write(payload)
    return mac.Sum(nil), nil
}

// key comes from os.LookupEnv("SIGNING_KEY") or the secret manager at
// process start, never from a source-controlled constant.
```

The repository never carries a value that, alone, is sufficient to forge a
signature.
