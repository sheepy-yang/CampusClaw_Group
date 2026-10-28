package auth

import "errors"

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	// ErrNotFound 跨班与不存在资源统一对外表现为 404。
	ErrNotFound = errors.New("not found")
)
