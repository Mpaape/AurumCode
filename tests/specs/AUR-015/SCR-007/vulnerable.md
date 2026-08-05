Fixture: VULNERABLE
Rule: SCR-007 — Security Logging and Monitoring Failures (CWE-532)

A request's raw authorization header is logged, crossing sensitive data
from the request boundary into a log sink read by a wider audience.

```go
func handleLogin(r *http.Request) {
    log.Printf("login attempt: user=%s auth=%s", r.Form.Get("user"), r.Header.Get("Authorization"))
    // ...
}
```

The bearer token or basic-auth credential is now readable by anyone with
log access, including log aggregation and support tooling.
