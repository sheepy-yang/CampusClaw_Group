package tutoring

import (
	"context"
	"errors"
	"sort"
	"strings"
	"unicode"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/auth"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/security"
)

func (s *Service) CreateInteraction(ctx context.Context, principal auth.Principal, prompt string) (ClassroomInteraction, error) {
	if principal.Role != auth.RoleTeacher {
		return ClassroomInteraction{}, forbidden("only teachers can create classroom interactions")
	}
	if !s.repository.skillEnabled(principal.ClassID, "classroom_cluster") {
		return ClassroomInteraction{}, forbidden("classroom clustering is disabled for this class")
	}
	prompt = strings.TrimSpace(prompt)
	if runeLength(prompt) < 1 || runeLength(prompt) > 500 {
		return ClassroomInteraction{}, invalid("prompt must contain 1 to 500 characters")
	}
	if err := security.CheckInput(ctx, prompt); err != nil {
		return ClassroomInteraction{}, unsafeInput(err)
	}
	now := s.now().UTC()
	interaction := ClassroomInteraction{
		ID:        s.repository.newID("interaction"),
		ClassID:   principal.ClassID,
		TeacherID: principal.UserID,
		Prompt:    prompt,
		Status:    "open",
		Answers:   make(map[string]ClassroomAnswer),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repository.saveInteraction(interaction); err != nil {
		return ClassroomInteraction{}, conflict("interaction changed concurrently")
	}
	s.log(ctx, principal, "tutoring.classroom.created", map[string]any{
		"sessionId": interaction.ID,
	})
	return interaction, nil
}

func (s *Service) ActiveInteraction(_ context.Context, principal auth.Principal) (*ClassroomInteraction, error) {
	if principal.Role != auth.RoleStudent && principal.Role != auth.RoleTeacher {
		return nil, forbidden("forbidden")
	}
	if !s.repository.skillEnabled(principal.ClassID, "classroom_cluster") {
		return nil, forbidden("classroom clustering is disabled for this class")
	}
	interaction, ok := s.repository.activeInteraction(principal.ClassID)
	if !ok {
		return nil, nil
	}
	return &interaction, nil
}

func (s *Service) SubmitClassroomAnswer(ctx context.Context, principal auth.Principal, interactionID, text string) (map[string]any, error) {
	if principal.Role != auth.RoleStudent {
		return nil, forbidden("only students can submit classroom answers")
	}
	if !s.repository.skillEnabled(principal.ClassID, "classroom_cluster") {
		return nil, forbidden("classroom clustering is disabled for this class")
	}
	text = strings.TrimSpace(text)
	if runeLength(text) < 1 || runeLength(text) > 1000 {
		return nil, invalid("answer text must contain 1 to 1000 characters")
	}
	if err := security.CheckInput(ctx, text); err != nil {
		return nil, unsafeInput(err)
	}
	var answer ClassroomAnswer
	interaction, err := s.repository.updateInteraction(strings.TrimSpace(interactionID), func(interaction *ClassroomInteraction) error {
		if interaction.ClassID != principal.ClassID {
			return notFound()
		}
		if interaction.Status != "open" {
			return conflict("classroom interaction is closed")
		}
		answer = interaction.Answers[principal.UserID]
		if answer.AnonymousID == "" {
			answer.AnonymousID = s.repository.newID("answer")
			answer.StudentID = principal.UserID
		}
		answer.Text = text
		answer.UpdatedAt = s.now().UTC()
		interaction.Answers[principal.UserID] = answer
		interaction.UpdatedAt = answer.UpdatedAt
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.log(ctx, principal, "tutoring.classroom.answer_submitted", map[string]any{
		"sessionId": interaction.ID, "answerId": answer.AnonymousID,
	})
	return map[string]any{
		"sessionId": interaction.ID,
		"answerId":  answer.AnonymousID,
		"count":     len(interaction.Answers),
		"updated":   true,
	}, nil
}

func (s *Service) CloseInteraction(ctx context.Context, principal auth.Principal, interactionID string) (ClassroomInteraction, error) {
	if principal.Role != auth.RoleTeacher {
		return ClassroomInteraction{}, forbidden("only teachers can close classroom interactions")
	}
	if !s.repository.skillEnabled(principal.ClassID, "classroom_cluster") {
		return ClassroomInteraction{}, forbidden("classroom clustering is disabled for this class")
	}
	interaction, err := s.repository.updateInteraction(strings.TrimSpace(interactionID), func(interaction *ClassroomInteraction) error {
		if interaction.ClassID != principal.ClassID {
			return notFound()
		}
		if interaction.Status == "closed" {
			return nil
		}
		if interaction.Status != "open" {
			return conflict("classroom interaction has an invalid state")
		}
		now := s.now().UTC()
		interaction.Status = "closed"
		interaction.ClosedAt = &now
		interaction.UpdatedAt = now
		return nil
	})
	if err != nil {
		return ClassroomInteraction{}, err
	}
	s.log(ctx, principal, "tutoring.classroom.closed", map[string]any{
		"sessionId": interaction.ID, "answerCount": len(interaction.Answers),
	})
	return interaction, nil
}

func (s *Service) ClusterInteraction(ctx context.Context, principal auth.Principal, interactionID string) (ClassroomAnalysis, error) {
	if principal.Role != auth.RoleTeacher {
		return ClassroomAnalysis{}, forbidden("only teachers can cluster classroom answers")
	}
	if !s.repository.skillEnabled(principal.ClassID, "classroom_cluster") {
		return ClassroomAnalysis{}, forbidden("classroom clustering is disabled for this class")
	}
	interaction, ok := s.repository.interaction(strings.TrimSpace(interactionID))
	if !ok || interaction.ClassID != principal.ClassID {
		return ClassroomAnalysis{}, notFound()
	}
	if interaction.Status != "closed" {
		return ClassroomAnalysis{}, conflict("close the classroom interaction before clustering")
	}
	if interaction.Analysis != nil {
		return *interaction.Analysis, nil
	}

	answers := make([]ClassroomAnswer, 0, len(interaction.Answers))
	for _, answer := range interaction.Answers {
		if strings.TrimSpace(answer.Text) != "" {
			answers = append(answers, answer)
		}
	}
	sort.Slice(answers, func(i, j int) bool { return answers[i].AnonymousID < answers[j].AnonymousID })
	if len(answers) < 5 {
		analysis := ClassroomAnalysis{
			Skipped:       true,
			Reason:        "fewer than five valid answers",
			AnswerCount:   len(answers),
			Anonymous:     true,
			CountsAsGrade: false,
		}
		if _, err := s.repository.updateInteraction(interaction.ID, func(current *ClassroomInteraction) error {
			current.Analysis = &analysis
			current.UpdatedAt = s.now().UTC()
			return nil
		}); err != nil {
			return ClassroomAnalysis{}, err
		}
		s.log(ctx, principal, "tutoring.classroom.cluster_skipped", map[string]any{
			"sessionId": interaction.ID, "answerCount": len(answers),
		})
		return analysis, nil
	}

	groups := deterministicGroups(answers)
	for index := range groups {
		groups[index].Texts = security.AnonymizeForModel(ctx, groups[index].Texts)
	}
	var clusters []ClassroomCluster
	mode := "deterministic"
	if s.clusterModel == nil {
		clusters = deterministicClusterSummaries(groups)
	} else {
		summaries, err := s.clusterModel.SummarizeClusters(ctx, groups)
		if err != nil {
			return ClassroomAnalysis{}, modelUnavailable(err)
		}
		clusters, err = validateClusterSummaries(ctx, groups, summaries)
		if err != nil {
			return ClassroomAnalysis{}, invalidModelOutput(err)
		}
		mode = "model_summary"
	}
	analysis := ClassroomAnalysis{
		Mode:          mode,
		AnswerCount:   len(answers),
		Anonymous:     true,
		CountsAsGrade: false,
		Clusters:      clusters,
	}
	if _, err := s.repository.updateInteraction(interaction.ID, func(current *ClassroomInteraction) error {
		if current.ClassID != principal.ClassID || current.Status != "closed" {
			return conflict("classroom interaction changed before analysis was saved")
		}
		current.Analysis = &analysis
		current.UpdatedAt = s.now().UTC()
		return nil
	}); err != nil {
		return ClassroomAnalysis{}, err
	}
	s.log(ctx, principal, "tutoring.classroom.clustered", map[string]any{
		"sessionId": interaction.ID, "answerCount": len(answers),
		"clusterCount": len(clusters), "mode": mode,
	})
	return analysis, nil
}

func (s *Service) ClassroomAnalysis(_ context.Context, principal auth.Principal, interactionID string) (ClassroomAnalysis, error) {
	if principal.Role != auth.RoleTeacher {
		return ClassroomAnalysis{}, forbidden("only teachers can view classroom analysis")
	}
	if !s.repository.skillEnabled(principal.ClassID, "classroom_cluster") {
		return ClassroomAnalysis{}, forbidden("classroom clustering is disabled for this class")
	}
	interaction, ok := s.repository.interaction(strings.TrimSpace(interactionID))
	if !ok || interaction.ClassID != principal.ClassID || interaction.Analysis == nil {
		return ClassroomAnalysis{}, notFound()
	}
	return *interaction.Analysis, nil
}

func deterministicGroups(answers []ClassroomAnswer) []ClusterDraft {
	type groupState struct {
		draft  ClusterDraft
		tokens map[string]struct{}
	}
	var groups []groupState
	for _, answer := range answers {
		tokens := textTokens(answer.Text)
		bestIndex, bestScore := -1, 0.0
		for index := range groups {
			score := jaccard(tokens, groups[index].tokens)
			if score > bestScore {
				bestIndex, bestScore = index, score
			}
		}
		if bestIndex < 0 || (bestScore < 0.25 && len(groups) < 4) {
			groups = append(groups, groupState{
				draft: ClusterDraft{
					ID:        "group-" + string(rune('1'+len(groups))),
					MemberIDs: []string{answer.AnonymousID},
					Texts:     []string{answer.Text},
				},
				tokens: tokens,
			})
			continue
		}
		groups[bestIndex].draft.MemberIDs = append(groups[bestIndex].draft.MemberIDs, answer.AnonymousID)
		groups[bestIndex].draft.Texts = append(groups[bestIndex].draft.Texts, answer.Text)
		for token := range tokens {
			groups[bestIndex].tokens[token] = struct{}{}
		}
	}
	result := make([]ClusterDraft, len(groups))
	for index := range groups {
		result[index] = groups[index].draft
	}
	return result
}

func textTokens(text string) map[string]struct{} {
	runes := []rune(strings.ToLower(strings.TrimSpace(text)))
	tokens := make(map[string]struct{})
	var word []rune
	flushWord := func() {
		if len(word) > 0 {
			tokens[string(word)] = struct{}{}
			word = nil
		}
	}
	var compact []rune
	for _, character := range runes {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			word = append(word, character)
			compact = append(compact, character)
		} else {
			flushWord()
		}
	}
	flushWord()
	for index := 0; index+1 < len(compact); index++ {
		tokens[string(compact[index:index+2])] = struct{}{}
	}
	return tokens
}

func jaccard(left, right map[string]struct{}) float64 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	intersection := 0
	union := make(map[string]struct{}, len(left)+len(right))
	for token := range left {
		union[token] = struct{}{}
		if _, ok := right[token]; ok {
			intersection++
		}
	}
	for token := range right {
		union[token] = struct{}{}
	}
	return float64(intersection) / float64(len(union))
}

func deterministicClusterSummaries(groups []ClusterDraft) []ClassroomCluster {
	result := make([]ClassroomCluster, len(groups))
	for index, group := range groups {
		result[index] = ClassroomCluster{
			ID:        group.ID,
			MemberIDs: append([]string(nil), group.MemberIDs...),
			Size:      len(group.MemberIDs),
			Label:     "回答类型 " + string(rune('1'+index)),
			FollowUp:  "请说明这一回答背后的概念依据。",
		}
	}
	return result
}

func validateClusterSummaries(ctx context.Context, groups []ClusterDraft, summaries []ClusterSummaryDraft) ([]ClassroomCluster, error) {
	if len(summaries) != len(groups) {
		return nil, errors.New("model changed the deterministic group count")
	}
	expected := make(map[string]map[string]struct{}, len(groups))
	for _, group := range groups {
		members := make(map[string]struct{}, len(group.MemberIDs))
		for _, member := range group.MemberIDs {
			members[member] = struct{}{}
		}
		expected[group.ID] = members
	}
	seenGroups := make(map[string]bool)
	seenMembers := make(map[string]bool)
	result := make([]ClassroomCluster, 0, len(summaries))
	for _, summary := range summaries {
		members, ok := expected[summary.GroupID]
		if !ok || seenGroups[summary.GroupID] {
			return nil, errors.New("model invented or duplicated a group")
		}
		seenGroups[summary.GroupID] = true
		if len(summary.MemberIDs) != len(members) {
			return nil, errors.New("model changed group membership")
		}
		for _, member := range summary.MemberIDs {
			if _, ok := members[member]; !ok || seenMembers[member] {
				return nil, errors.New("model invented or duplicated a member")
			}
			seenMembers[member] = true
		}
		label := strings.TrimSpace(summary.Label)
		followUp := strings.TrimSpace(summary.FollowUp)
		if runeLength(label) < 1 || runeLength(label) > 100 || runeLength(followUp) > 300 {
			return nil, errors.New("model returned an invalid summary length")
		}
		if err := security.CheckInput(ctx, label+"\n"+followUp); err != nil {
			return nil, err
		}
		result = append(result, ClassroomCluster{
			ID:        summary.GroupID,
			MemberIDs: append([]string(nil), summary.MemberIDs...),
			Size:      len(members),
			Label:     label,
			FollowUp:  followUp,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}
