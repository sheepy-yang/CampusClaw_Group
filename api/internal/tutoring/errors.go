package tutoring

import (
	"errors"
	"fmt"
	"net/http"
)

// Error is a stable application error that HTTP adapters can map without
// exposing internal or model/provider details.
type Error struct {
	Status  int
	Code    string
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

func (e *Error) Unwrap() error { return e.Cause }

func newError(status int, code, message string, cause error) error {
	return &Error{Status: status, Code: code, Message: message, Cause: cause}
}

func invalid(message string) error {
	return newError(http.StatusBadRequest, "INVALID_ARGUMENT", message, nil)
}

func unsafeInput(cause error) error {
	return newError(http.StatusBadRequest, "UNSAFE_INPUT", "input failed content safety checks", cause)
}

func forbidden(message string) error {
	return newError(http.StatusForbidden, "FORBIDDEN", message, nil)
}

func notFound() error {
	return newError(http.StatusNotFound, "NOT_FOUND", "resource not found", nil)
}

func conflict(message string) error {
	return newError(http.StatusConflict, "CONFLICT", message, nil)
}

func invalidModelOutput(cause error) error {
	return newError(http.StatusBadGateway, "MODEL_OUTPUT_INVALID", "model returned an invalid result", cause)
}

func modelUnavailable(cause error) error {
	return newError(http.StatusServiceUnavailable, "MODEL_UNAVAILABLE", "model is unavailable", cause)
}

func persistenceUnavailable(cause error) error {
	return newError(http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "state could not be durably saved", cause)
}

// ErrorInfo exposes only the stable public mapping of an application error.
func ErrorInfo(err error) (status int, code, message string) {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Status, appErr.Code, appErr.Message
	}
	return http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error"
}
