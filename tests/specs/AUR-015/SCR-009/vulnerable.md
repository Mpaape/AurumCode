Fixture: VULNERABLE
Rule: SCR-009 — Server-Side Request Forgery (CWE-918)

The server fetches a URL supplied by the caller with no destination check,
letting the request cross from attacker-supplied input directly into an
outbound network call.

```go
func FetchPreview(w http.ResponseWriter, r *http.Request) {
    target := r.URL.Query().Get("url")
    resp, err := http.Get(target) // any host, including internal-only ones
    if err != nil {
        http.Error(w, "fetch failed", http.StatusBadGateway)
        return
    }
    defer resp.Body.Close()
    io.Copy(w, resp.Body)
}
```

`url=http://169.254.169.254/latest/meta-data/` reaches the cloud metadata
endpoint through the server.
