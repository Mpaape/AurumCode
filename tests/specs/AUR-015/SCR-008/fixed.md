Fixture: FIXED
Rule: SCR-008 — Security Misconfiguration (CWE-16)

The permissive default is off unless an explicit, reviewed configuration
flag turns it on for a non-production environment.

```go
func newRouter(cfg Config) *http.ServeMux {
    mux := http.NewServeMux()
    if cfg.Environment != "production" && cfg.DebugEndpointsEnabled {
        mux.Handle("/debug/pprof/", http.DefaultServeMux)
    }
    return mux
}
```

Production ships with the debug surface absent unless both conditions are
explicitly satisfied.
