Fixture: FIXED
Rule: SCR-006 — Identification and Authentication Failures (CWE-287)

The cookie is a signed, verified token; an invalid or unverifiable token
fails closed rather than resolving to a default identity.

```go
func CurrentUser(r *http.Request) (*User, error) {
    c, err := r.Cookie("session")
    if err != nil {
        return nil, err
    }
    claims, err := verifySessionToken(c.Value, sessionSigningKey)
    if err != nil {
        return nil, errors.New("invalid session") // fail closed, no lookup
    }
    return lookupUserByID(claims.UserID)
}
```

A forged or tampered cookie is rejected before any user lookup happens.
