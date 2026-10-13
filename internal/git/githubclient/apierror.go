package githubclient

import (
	"errors"
	"fmt"
	"net/http"
)

// APIError is a GitHub answer outside the success range, kept as data: the
// status tells a permission denial from any other refusal, and the body is
// GitHub's own explanation. Its text is the "HTTP <status>: <body>" line the
// client always printed, so nothing that reads the message changes.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.Status, e.Body)
}

// PermissionDenied reports whether GitHub refused the credential: missing
// or invalid (401), or without the scope the endpoint needs (403, which a
// workflow token answers as "Resource not accessible by integration").
func (e *APIError) PermissionDenied() bool {
	return e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden
}

// apiError is the error of an answer the method did not accept.
func apiError(status int, body []byte) error {
	return &APIError{Status: status, Body: string(body)}
}

// IsPermissionDenied reports whether err wraps a GitHub answer that denied
// the credential (APIError.PermissionDenied).
func IsPermissionDenied(err error) bool {
	var api *APIError
	return errors.As(err, &api) && api.PermissionDenied()
}
