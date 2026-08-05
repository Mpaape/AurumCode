Fixture: FIXED
Rule: SCR-001 — Injection (CWE-89)

The same lookup with a parameterized query. The database driver, not string
concatenation, carries the untrusted value across the trust boundary.

```go
func FindUser(db *sql.DB, name string) (*User, error) {
    row := db.QueryRow("SELECT id, email FROM users WHERE name = ?", name)
    return scanUser(row)
}
```

`name = "' OR '1'='1"` is treated as a literal value with no special
meaning; the query still matches at most one row.
