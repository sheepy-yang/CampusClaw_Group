package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/security"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

const (
	maxIdentifierRunes = 128
	maxAnswerRunes     = 100_000
	maxMetadataRunes   = 4_000
	maxScoreAllowed    = 10_000
)

var imageDigestPattern = regexp.MustCompile(`^sha256:[0-9a-fA-F]{64}$`)

func validateIdentifier(name, value string) error {
	if value == "" || strings.TrimSpace(value) != value || utf8.RuneCountInString(value) > maxIdentifierRunes {
		return fmt.Errorf("%w: %s", ErrInvalidArgument, name)
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return fmt.Errorf("%w: %s", ErrInvalidArgument, name)
		}
	}
	return nil
}

func validateActor(actor ActorScope) error {
	if err := validateIdentifier("actor user id", actor.UserID); err != nil {
		return err
	}
	return validateIdentifier("actor class id", actor.ClassID)
}

func validateModelInput(ctx context.Context, name, value string, required bool, maxRunes int) error {
	if required && strings.TrimSpace(value) == "" {
		return fmt.Errorf("%w: %s is required", ErrInvalidArgument, name)
	}
	if maxRunes > 0 && utf8.RuneCountInString(value) > maxRunes {
		return fmt.Errorf("%w: %s is too long", ErrInvalidArgument, name)
	}
	if err := security.CheckInput(ctx, value); err != nil {
		return fmt.Errorf("%w: %s", ErrUnsafeInput, name)
	}
	return nil
}

func validateImageDigest(value string) error {
	if value == "" {
		return nil
	}
	if !imageDigestPattern.MatchString(value) {
		return fmt.Errorf("%w: imageDigest must be a sha256 digest", ErrInvalidArgument)
	}
	return nil
}

func validateMaxScore(score float64) error {
	if math.IsNaN(score) || math.IsInf(score, 0) || score <= 0 || score > maxScoreAllowed {
		return fmt.Errorf("%w: maxScore", ErrInvalidArgument)
	}
	return nil
}

func validateScore(score, maxScore float64) error {
	if math.IsNaN(score) || math.IsInf(score, 0) || score < 0 || score > maxScore {
		return fmt.Errorf("%w: score must be between 0 and maxScore", ErrInvalidArgument)
	}
	return nil
}

func stableKey(prefix string, values ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return prefix + "-" + hex.EncodeToString(sum[:8])
}

func validLearningEvents(events []store.LearningEvent) []store.LearningEvent {
	seen := make(map[string]bool, len(events))
	out := make([]store.LearningEvent, 0, len(events))
	for _, event := range events {
		if event.ID == "" || event.StudentID == "" || event.ClassID == "" ||
			strings.TrimSpace(event.Summary) == "" || event.At.IsZero() || seen[event.ID] {
			continue
		}
		switch event.Kind {
		case "assignment", "practice", "help", "tutoring":
		default:
			continue
		}
		if _, err := store.NormalizeLearningOutcome(event.Outcome); err != nil {
			continue
		}
		seen[event.ID] = true
		out = append(out, event)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].At.Equal(out[j].At) {
			return out[i].ID < out[j].ID
		}
		return out[i].At.Before(out[j].At)
	})
	return out
}

func diagnosisEvidenceKey(classID, studentID string, events []store.LearningEvent) string {
	parts := []string{classID, studentID}
	for _, event := range events {
		parts = append(parts,
			event.ID,
			event.Kind,
			event.Summary,
			event.KP,
			string(event.Outcome),
			event.At.UTC().Format(time.RFC3339Nano),
		)
	}
	return stableKey("diagnosis", parts...)
}

func nextTaskAttempt(taskID, stepPrefix string) (int, []store.TaskStep) {
	task, ok := store.Default.GetTask(taskID)
	if !ok {
		return 1, nil
	}
	steps := append([]store.TaskStep(nil), task.Steps...)
	attempt := 1
	for _, step := range steps {
		if strings.HasPrefix(step.Name, stepPrefix) {
			attempt++
		}
	}
	return attempt, steps
}
