package store

import "database/sql"

func FindUser(db *sql.DB, name string) (*sql.Rows, error) {
	q := "SELECT id FROM users WHERE name = '" + name + "'"
	return db.Query(q)
}
