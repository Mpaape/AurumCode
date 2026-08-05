Fixture: VULNERABLE
Rule: SCR-002 — Broken Access Control (CWE-862)

The handler trusts a client-supplied identifier without checking that the
caller is authorized to act on it — the authenticated-but-unauthorized
caller crosses straight into the privileged operation.

```go
func DeleteInvoice(w http.ResponseWriter, r *http.Request) {
    id := r.URL.Query().Get("id")
    store.Delete(id) // no ownership or role check
    w.WriteHeader(http.StatusNoContent)
}
```

Any authenticated user can delete any invoice by guessing or enumerating
`id`.
