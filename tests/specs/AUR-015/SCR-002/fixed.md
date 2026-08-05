Fixture: FIXED
Rule: SCR-002 — Broken Access Control (CWE-862)

The handler resolves the caller's authorization first and fails closed
(denies) when the check itself cannot be completed.

```go
func DeleteInvoice(w http.ResponseWriter, r *http.Request) {
    id := r.URL.Query().Get("id")
    caller := auth.CallerFromContext(r.Context())
    allowed, err := authz.CanDelete(caller, id)
    if err != nil || !allowed {
        w.WriteHeader(http.StatusForbidden) // fail closed, never allow on error
        return
    }
    store.Delete(id)
    w.WriteHeader(http.StatusNoContent)
}
```

A caller who does not own the invoice, or whose authorization check errors,
is denied rather than defaulted to allow.
