package evaluation

import (
	"context"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/domain"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

// ActorScope contains only server-authenticated identity attributes. Callers
// must never populate it from request JSON.
type ActorScope struct {
	UserID  string
	ClassID string
	Teacher bool
}

// ResourceScope exposes the ownership metadata needed by the HTTP layer
// without returning the protected resource body.
type ResourceScope struct {
	ResourceID     string `json:"resourceId"`
	ClassID        string `json:"classId"`
	StudentID      string `json:"studentId,omitempty"`
	TeacherOnly    bool   `json:"teacherOnly"`
	StudentVisible bool   `json:"studentVisible"`
}

func (s *Services) SubmissionScope(_ context.Context, id string) (ResourceScope, error) {
	sub, ok := store.Default.GetSubmission(id)
	if !ok {
		return ResourceScope{}, ErrNotFound
	}
	return ResourceScope{
		ResourceID:     sub.ID,
		ClassID:        sub.ClassID,
		StudentID:      sub.StudentID,
		StudentVisible: true,
	}, nil
}

func (s *Services) GetSubmissionForActor(ctx context.Context, id string, actor ActorScope) (store.Submission, error) {
	scope, err := s.SubmissionScope(ctx, id)
	if err != nil {
		return store.Submission{}, err
	}
	if err := AuthorizeRead(actor, scope); err != nil {
		return store.Submission{}, err
	}
	sub, ok := store.Default.GetSubmission(id)
	if !ok {
		return store.Submission{}, ErrNotFound
	}
	return sub, nil
}

func (s *Services) ListSubmissionsForActor(_ context.Context, actor ActorScope, assignmentID string) ([]store.Submission, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	if !actor.Teacher {
		return nil, ErrForbidden
	}
	if assignmentID != "" {
		if err := validateIdentifier("assignmentID", assignmentID); err != nil {
			return nil, err
		}
	}
	return store.Default.ListSubmissions(actor.ClassID, assignmentID), nil
}

func (s *Services) ContentScope(_ context.Context, id string) (ResourceScope, error) {
	content, ok := store.Default.GetContent(id)
	if !ok {
		return ResourceScope{}, ErrNotFound
	}
	scope := ResourceScope{
		ResourceID:  content.ID,
		ClassID:     content.ClassID,
		TeacherOnly: content.Skill == string(domain.SkillClassReview),
	}
	if content.Payload != nil {
		scope.StudentID, _ = content.Payload["studentId"].(string)
		scope.StudentVisible, _ = content.Payload["studentVisible"].(bool)
	}
	if content.Status != string(domain.StatusPublished) {
		scope.StudentVisible = false
	}
	return scope, nil
}

// AuthorizeRead intentionally returns ErrNotFound for every ownership failure
// so callers can preserve non-enumerating 404 semantics.
func AuthorizeRead(actor ActorScope, resource ResourceScope) error {
	if err := validateActor(actor); err != nil {
		return err
	}
	if actor.ClassID != resource.ClassID {
		return ErrNotFound
	}
	if actor.Teacher {
		return nil
	}
	if resource.TeacherOnly || !resource.StudentVisible || resource.StudentID == "" || resource.StudentID != actor.UserID {
		return ErrNotFound
	}
	return nil
}

func AuthorizeTeacher(actor ActorScope, resource ResourceScope) error {
	if err := validateActor(actor); err != nil {
		return err
	}
	if !actor.Teacher {
		return ErrForbidden
	}
	if actor.ClassID != resource.ClassID {
		return ErrNotFound
	}
	return nil
}
