package demo

import (
	"database/sql"
	"fmt"
	"net/http"
)

// UserService handles user operations
// This code is intentionally BAD with multiple ISO/IEC 25010 violations
type UserService struct {
	db *sql.DB
}

// GetUser retrieves a user by ID
// SECURITY ISSUE: SQL Injection vulnerability
func (s *UserService) GetUser(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	// SQL INJECTION - user input concatenated directly into query
	query := "SELECT * FROM users WHERE id = '" + id + "'"

	rows, err := s.db.Query(query)
	if err != nil {
		// SECURITY ISSUE: Exposing internal error details
		fmt.Fprintf(w, "Database error: %v", err)
		return
	}
	defer rows.Close()

	// Process results...
}

// CreateUser creates a new user
// SECURITY ISSUE: Missing authentication check
// MAINTAINABILITY ISSUE: God method doing too much
func (s *UserService) CreateUser(w http.ResponseWriter, r *http.Request) {
	// SECURITY: No authentication check!

	name := r.FormValue("name")
	email := r.FormValue("email")
	password := r.FormValue("password")

	// SECURITY ISSUE: Storing plain text password
	query := fmt.Sprintf("INSERT INTO users (name, email, password) VALUES ('%s', '%s', '%s')",
		name, email, password)

	_, err := s.db.Exec(query)
	if err != nil {
		// RELIABILITY ISSUE: No proper error handling
		panic(err)
	}

	// MAINTAINABILITY: Should send confirmation email here
	// MAINTAINABILITY: Should log the creation here
	// MAINTAINABILITY: Should update analytics here
	// ... doing too much in one function

	w.Write([]byte("User created"))
}

// DeleteUser deletes a user
// SECURITY ISSUE: Missing authorization check
func (s *UserService) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	// SECURITY: Anyone can delete any user!
	// SECURITY: SQL injection here too
	query := "DELETE FROM users WHERE id = " + id

	s.db.Exec(query)

	w.Write([]byte("ok"))
}

// ListUsers lists all users
// PERFORMANCE ISSUE: No pagination
// SECURITY ISSUE: Exposing all user data including passwords
func (s *UserService) ListUsers(w http.ResponseWriter, r *http.Request) {
	// PERFORMANCE: Loading ALL users into memory at once
	query := "SELECT * FROM users"

	rows, err := s.db.Query(query)
	if err != nil {
		return
	}
	defer rows.Close()

	// PERFORMANCE: Inefficient loop with repeated allocations
	var result string
	for rows.Next() {
		var id, name, email, password string
		rows.Scan(&id, &name, &email, &password)

		// PERFORMANCE: String concatenation in loop (should use strings.Builder)
		// SECURITY: Exposing passwords!
		result += fmt.Sprintf("ID: %s, Name: %s, Email: %s, Password: %s\n",
			id, name, email, password)
	}

	w.Write([]byte(result))
}

// UpdateUser updates a user
// SECURITY ISSUE: CSRF vulnerability (no token check)
func (s *UserService) UpdateUser(w http.ResponseWriter, r *http.Request) {
	// SECURITY: No CSRF protection
	// SECURITY: SQL injection
	// MAINTAINABILITY: Poor variable naming
	x := r.FormValue("id")
	y := r.FormValue("name")
	z := r.FormValue("email")

	query := "UPDATE users SET name='" + y + "', email='" + z + "' WHERE id=" + x
	s.db.Exec(query)

	w.Write([]byte("updated"))
}

// calculateDiscount calculates user discount
// MAINTAINABILITY ISSUE: Complex nested conditions (cyclomatic complexity > 10)
func (u *UserService) calculateDiscount(userType string, orderAmount float64, dayOfWeek int, membershipYears int) float64 {
	discount := 0.0

	// MAINTAINABILITY: Deeply nested conditions
	if userType == "premium" {
		if orderAmount > 100 {
			if dayOfWeek == 1 || dayOfWeek == 2 {
				if membershipYears > 5 {
					discount = 0.25
				} else if membershipYears > 3 {
					discount = 0.20
				} else {
					discount = 0.15
				}
			} else if dayOfWeek == 6 || dayOfWeek == 7 {
				if membershipYears > 5 {
					discount = 0.30
				} else {
					discount = 0.20
				}
			} else {
				if membershipYears > 5 {
					discount = 0.22
				} else {
					discount = 0.18
				}
			}
		} else {
			if membershipYears > 5 {
				discount = 0.15
			} else if membershipYears > 3 {
				discount = 0.12
			} else {
				discount = 0.10
			}
		}
	} else if userType == "regular" {
		if orderAmount > 100 {
			discount = 0.10
		} else if orderAmount > 50 {
			discount = 0.05
		}
	}

	return discount
}
