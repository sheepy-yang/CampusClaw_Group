// Package platform declares existing CampusClaw platform-service ports.
// Authentication, materials, assignments and audit receive their own business
// implementations only through dedicated OpenSpec changes.
package platform

import (
	"context"
	"io"
)

type ResourceScope struct {
	UserID  string
	ClassID string
	Role    string
}

type MaterialSummary struct {
	ID      string
	ClassID string
	Title   string
	Version string
}

type MaterialService interface {
	List(ctx context.Context, scope ResourceScope) ([]MaterialSummary, error)
	Upload(ctx context.Context, scope ResourceScope, filename string, body io.Reader) (MaterialSummary, error)
}

type AssignmentSummary struct {
	ID      string
	ClassID string
	Title   string
	Status  string
}

type AssignmentService interface {
	List(ctx context.Context, scope ResourceScope) ([]AssignmentSummary, error)
	Create(ctx context.Context, scope ResourceScope, title string) (AssignmentSummary, error)
}

type AuditRecord struct {
	ID        string
	ActorID   string
	ClassID   string
	Action    string
	RequestID string
}

type AuditReader interface {
	List(ctx context.Context, scope ResourceScope) ([]AuditRecord, error)
}
