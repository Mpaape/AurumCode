// Package core is plain PRODUCT code, the control case: no relationship to
// test scope at all.
package core

// Service does the product's actual work.
type Service struct{}

// Run executes the service.
func (s *Service) Run() error { return nil }
