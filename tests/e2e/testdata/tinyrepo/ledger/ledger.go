// Package ledger keeps the smallest possible amount of documented Go code.
package ledger

// AddMoney sums two monetary amounts expressed in cents.
func AddMoney(a, b int64) int64 {
	return a + b
}

// Entry is a single documented ledger line.
type Entry struct {
	// Cents is the signed amount of the entry.
	Cents int64
}
