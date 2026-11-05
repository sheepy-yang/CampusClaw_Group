package evaluation

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ObjectiveAnswerSource must resolve references from server-owned assignment
// data. Implementations must not use a standard answer supplied by request JSON.
type ObjectiveAnswerSource interface {
	ReferenceAnswer(ctx context.Context, classID, assignmentID, questionID string) (questionType, standardAnswer string, err error)
}

type TranscriptionResult struct {
	Text       string
	Confidence float64
}

// ImageTranscriber resolves an opaque image digest through a server-side,
// authorized object/model adapter.
type ImageTranscriber interface {
	Transcribe(ctx context.Context, classID, studentID, imageDigest string) (TranscriptionResult, error)
}

type Dependencies struct {
	Jobs             JobSink
	ObjectiveAnswers ObjectiveAnswerSource
	Now              func() time.Time
}

func NewWithDependencies(deps Dependencies) *Services {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Services{
		Jobs:             deps.Jobs,
		ObjectiveAnswers: deps.ObjectiveAnswers,
		now:              deps.Now,
	}
}

func (s *Services) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func (s *Services) requireJobs() (JobSink, error) {
	if s == nil || s.Jobs == nil {
		return nil, fmt.Errorf("%w: job sink", ErrDependencyUnavailable)
	}
	return s.Jobs, nil
}

func validObjectiveType(questionType string) bool {
	switch strings.ToLower(strings.TrimSpace(questionType)) {
	case "choice", "single", "multi", "fill", "blank":
		return true
	default:
		return false
	}
}
