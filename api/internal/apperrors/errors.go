package apperrors

import "errors"

// ErrNotImplemented marks a declared framework boundary whose business change
// has not been applied yet. HTTP adapters map it to a stable 501 response.
var ErrNotImplemented = errors.New("capability is declared but not implemented in the skeleton")
