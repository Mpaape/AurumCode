Fixture: VULNERABLE
Rule: SCR-008 — Security Misconfiguration (CWE-16)

A permissive default (verbose debug output) ships unconditionally,
crossing from the deployment boundary into production without an explicit,
reviewed override.

```go
func newRouter() *http.ServeMux {
    mux := http.NewServeMux()
    mux.Handle("/debug/pprof/", http.DefaultServeMux) // always registered
    return mux
}
```

Every deployment, including production, exposes profiling and internal
state on an unauthenticated path.
