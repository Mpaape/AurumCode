Fixture: VULNERABLE
Rule: SCR-001 — Injection (CWE-89)

Unsanitized request input is concatenated directly into a SQL statement,
letting an attacker cross the request/database trust boundary with
arbitrary SQL.

```go
func FindUser(db *sql.DB, name string) (*User, error) {
    query := "SELECT id, email FROM users WHERE name = '" + name + "'"
    row := db.QueryRow(query)
    return scanUser(row)
}
```

`name = "' OR '1'='1"` returns every row instead of one.
