package evaluation

import (
	"regexp"
	"sort"
	"strings"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

var directIdentityPattern = regexp.MustCompile(`(?i)([\p{Han}]{1,4}(同学|老师))|(\b(stu|student|teacher)[-_]?[a-z0-9]+\b)`)

func effectiveClassSubmissions(submissions []store.Submission) []store.Submission {
	byStudent := make(map[string]store.Submission, len(submissions))
	for _, sub := range submissions {
		if sub.StudentID == "" {
			continue
		}
		switch sub.Status {
		case SubNeedsTeacherReview, SubGradedSuggested, SubTeacherFinal:
			if !sub.Confirmed {
				continue
			}
		case "graded":
			// Deterministically graded platform submissions contain direct
			// text answers, so they do not require OCR confirmation.
		default:
			continue
		}
		current, exists := byStudent[sub.StudentID]
		if !exists || current.ID < sub.ID {
			byStudent[sub.StudentID] = sub
		}
	}
	out := make([]store.Submission, 0, len(byStudent))
	for _, sub := range byStudent {
		out = append(out, sub)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StudentID == out[j].StudentID {
			return out[i].ID < out[j].ID
		}
		return out[i].StudentID < out[j].StudentID
	})
	return out
}

func classReviewInputs(submissions []store.Submission) []string {
	inputs := make([]string, 0, len(submissions))
	for _, sub := range submissions {
		summary := strings.TrimSpace(sub.ErrorCause)
		if summary == "" && sub.FirstError != "" {
			summary = sub.FirstError + "需复核"
		}
		if summary == "" {
			if mistake, ok := store.Default.MistakeBySubmission(sub.ID); ok {
				summary = strings.TrimSpace(mistake.ErrorCause)
			}
		}
		if summary == "" {
			summary = "未发现可归纳错因"
		}
		inputs = append(inputs, summary)
	}
	return inputs
}

func containsNameLeak(value string) bool {
	return directIdentityPattern.MatchString(value)
}
