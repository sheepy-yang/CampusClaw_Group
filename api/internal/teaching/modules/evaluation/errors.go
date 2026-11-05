package evaluation

import "errors"

// Sentinel errors let the HTTP integration map failures without exposing
// whether a cross-class resource exists.
var (
	ErrInvalidArgument       = errors.New("invalid evaluation argument")
	ErrNotFound              = errors.New("evaluation resource not found")
	ErrForbidden             = errors.New("evaluation operation forbidden")
	ErrStateConflict         = errors.New("invalid evaluation state transition")
	ErrDependencyUnavailable = errors.New("evaluation dependency unavailable")
	ErrUnsafeInput           = errors.New("unsafe evaluation input")
	ErrUntrustedReference    = errors.New("trusted reference answer required")
	ErrUntrustedEvidence     = errors.New("untrusted evaluation evidence")
	ErrInsufficientEvidence  = errors.New("insufficient evaluation evidence")
)
