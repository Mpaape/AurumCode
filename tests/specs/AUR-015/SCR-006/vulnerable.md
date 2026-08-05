Fixture: VULNERABLE
Rule: SCR-006 — Identification and Authentication Failures (CWE-287)

The handler accepts a session cookie without verifying its signature,
letting an unauthenticated caller cross into a session-bearing boundary.

```go
func CurrentUser(r *http.Request) (*User, error) {
    c, err := r.Cookie("session")
    if err != nil {
        return nil, err
    }
    return lookupUserByID(c.Value) // c.Value trusted as-is, never verified
}
```

Any client can set `session=1` and become user `1`.
