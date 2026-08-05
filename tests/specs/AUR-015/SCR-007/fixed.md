Fixture: FIXED
Rule: SCR-007 — Security Logging and Monitoring Failures (CWE-532)

The log statement records the outcome and a non-reversible identifier, and
redacts the credential itself.

```go
func handleLogin(r *http.Request) {
    log.Printf("login attempt: user=%s auth=%s", r.Form.Get("user"), redact.Credential(r.Header.Get("Authorization")))
    // ...
}
```

`redact.Credential` returns a fixed placeholder (`"[redacted]"`), never the
bearer token or credential bytes, regardless of scheme.
