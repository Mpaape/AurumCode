Fixture: FIXED
Rule: SCR-009 — Server-Side Request Forgery (CWE-918)

The destination is resolved and checked against an allowlist before the
outbound call is made; anything else fails closed.

```go
func FetchPreview(w http.ResponseWriter, r *http.Request) {
    target := r.URL.Query().Get("url")
    if err := allowlist.CheckDestination(target); err != nil {
        http.Error(w, "destination not allowed", http.StatusForbidden) // fail closed
        return
    }
    resp, err := http.Get(target)
    if err != nil {
        http.Error(w, "fetch failed", http.StatusBadGateway)
        return
    }
    defer resp.Body.Close()
    io.Copy(w, resp.Body)
}
```

Requests to link-local, loopback, or otherwise non-allowlisted hosts are
rejected before any outbound call is made.
