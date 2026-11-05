package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ProductionSkill identifies the typed content-production contracts. These
// names intentionally differ from transport route names so application code
// does not depend on HTTP compatibility aliases.
type ProductionSkill string

const (
	SkillCourseCards ProductionSkill = "course_cards"
	SkillExercises   ProductionSkill = "exercises"
	SkillLessonPlan  ProductionSkill = "lesson_plan"
	SkillQuality     ProductionSkill = "content_quality_review"
)

const (
	DeterministicModelVersion  = "deterministic-local-v1"
	ContentPromptVersion       = "content-production-v1"
	ContentSkillVersion        = "content-production-v1"
	QualityReviewPromptVersion = "content-quality-review-v1"
	QualityReviewSkillVersion  = "content-quality-review-v1"
)

var (
	ErrUnsupportedProductionSkill = errors.New("unsupported content-production skill")
	ErrInvalidGroundingSource     = errors.New("invalid grounding source")
	ErrInvalidProductionRequest   = errors.New("invalid content-production request")
)

// GroundingSource is an immutable, server-resolved evidence snapshot.
type GroundingSource struct {
	SourceID       string `json:"sourceId"`
	SourceVersion  string `json:"sourceVersion"`
	ChunkID        string `json:"chunkId"`
	CharacterStart int    `json:"characterStart"`
	CharacterEnd   int    `json:"characterEnd"`
	ContentHash    string `json:"contentHash"`
	Text           string `json:"text"`
}

// ProductionRequest is the typed boundary consumed by a content agent.
type ProductionRequest struct {
	Skill           ProductionSkill
	Topic           string
	Count           int
	ExerciseTypes   []string
	Difficulty      string
	DurationMinutes int
	Objectives      []string
	Sources         []GroundingSource
}

type ProductionUsage struct {
	Mode            string `json:"mode"`
	Model           string `json:"model"`
	PromptVersion   string `json:"promptVersion"`
	SkillVersion    string `json:"skillVersion"`
	InputCharacters int    `json:"inputCharacters"`
	OutputItems     int    `json:"outputItems"`
	SourceCount     int    `json:"sourceCount"`
}

type ProductionOutput struct {
	Payload   map[string]any  `json:"payload"`
	Citations []Citation      `json:"citations"`
	ModelMeta ModelMeta       `json:"modelMeta"`
	Usage     ProductionUsage `json:"usage"`
}

type ContentProductionAgent interface {
	Produce(ctx context.Context, req ProductionRequest) (*ProductionOutput, error)
}

type IssueSeverity string

const (
	SeverityError   IssueSeverity = "error"
	SeverityWarning IssueSeverity = "warning"
)

type QualityIssue struct {
	Code     string        `json:"code"`
	Severity IssueSeverity `json:"severity"`
	Path     string        `json:"path"`
	Message  string        `json:"message"`
}

type QualityReviewRequest struct {
	Skill   ProductionSkill
	Payload map[string]any
	Sources []GroundingSource
}

type QualityReport struct {
	Passed         bool           `json:"passed"`
	Score          int            `json:"score"`
	Scope          string         `json:"scope"`
	Issues         []QualityIssue `json:"issues"`
	CheckedItems   int            `json:"checkedItems"`
	CheckedSources int            `json:"checkedSources"`
	ModelMeta      ModelMeta      `json:"modelMeta"`
}

type ContentQualityAgent interface {
	Review(ctx context.Context, req QualityReviewRequest) (QualityReport, error)
}

// DeterministicContentAgent is the offline fallback used by tests and local
// development. It only transforms supplied evidence and never invents a
// citation or claims external verification.
type DeterministicContentAgent struct{}

func NewDeterministicContentAgent() *DeterministicContentAgent {
	return &DeterministicContentAgent{}
}

func (DeterministicContentAgent) Produce(ctx context.Context, req ProductionRequest) (*ProductionOutput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sources, err := validateSources(req.Sources)
	if err != nil {
		return nil, err
	}
	if err := validateProductionRequest(req, sources); err != nil {
		return nil, err
	}

	var payload map[string]any
	var outputItems int
	switch req.Skill {
	case SkillCourseCards:
		payload, outputItems = produceCourseCards(req, sources)
	case SkillExercises:
		payload, outputItems = produceExercises(req, sources)
	case SkillLessonPlan:
		payload, outputItems = produceLessonPlan(req, sources)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedProductionSkill, req.Skill)
	}

	citations := make([]Citation, 0, len(sources))
	for _, source := range sources {
		citations = append(citations, sourceCitation(source))
	}
	meta := ModelMeta{
		Model:         DeterministicModelVersion,
		PromptVersion: ContentPromptVersion,
		SkillVersion:  ContentSkillVersion,
	}
	inputCharacters := utf8.RuneCountInString(req.Topic)
	for _, objective := range req.Objectives {
		inputCharacters += utf8.RuneCountInString(objective)
	}
	for _, source := range sources {
		inputCharacters += utf8.RuneCountInString(source.Text)
	}
	return &ProductionOutput{
		Payload:   payload,
		Citations: citations,
		ModelMeta: meta,
		Usage: ProductionUsage{
			Mode:            "deterministic_fallback",
			Model:           meta.Model,
			PromptVersion:   meta.PromptVersion,
			SkillVersion:    meta.SkillVersion,
			InputCharacters: inputCharacters,
			OutputItems:     outputItems,
			SourceCount:     len(sources),
		},
	}, nil
}

func produceCourseCards(req ProductionRequest, sources []GroundingSource) (map[string]any, int) {
	cards := make([]map[string]any, 0, req.Count)
	for i := 0; i < req.Count; i++ {
		source := sources[i%len(sources)]
		citation := citationMap(sourceCitation(source))
		cards = append(cards, map[string]any{
			"id":       fmt.Sprintf("card-%d", i+1),
			"title":    conciseTitle(source.Text, req.Topic, i+1),
			"summary":  strings.TrimSpace(source.Text),
			"citation": citation,
		})
	}
	return map[string]any{
		"title":     req.Topic,
		"cards":     cards,
		"citations": citationMaps(sources),
	}, len(cards)
}

func produceExercises(req ProductionRequest, sources []GroundingSource) (map[string]any, int) {
	questions := make([]map[string]any, 0, req.Count)
	for i := 0; i < req.Count; i++ {
		source := sources[i%len(sources)]
		exerciseType := req.ExerciseTypes[i%len(req.ExerciseTypes)]
		question := map[string]any{
			"id":          fmt.Sprintf("exercise-%d", i+1),
			"type":        exerciseType,
			"difficulty":  req.Difficulty,
			"explanation": "答案直接取自所绑定的课程材料片段。",
			"citations":   []map[string]any{citationMap(sourceCitation(source))},
		}
		switch exerciseType {
		case "choice":
			question["stem"] = fmt.Sprintf("以下哪一项与“%s”课程材料一致？", req.Topic)
			question["options"] = []string{
				strings.TrimSpace(source.Text),
				"材料明确给出了相反结论。",
				"材料未涉及任何相关概念。",
				"以上说法均不符合材料。",
			}
			question["answer"] = "A"
		case "fill":
			stem, answer := makeCloze(source.Text)
			question["stem"] = "补全课程材料中的表述：" + stem
			question["answer"] = answer
		default:
			question["stem"] = fmt.Sprintf("根据课程材料，概括“%s”的一个关键要点。", req.Topic)
			question["answer"] = strings.TrimSpace(source.Text)
		}
		questions = append(questions, question)
	}
	return map[string]any{
		"title":     req.Topic + "练习",
		"questions": questions,
		"citations": citationMaps(sources),
	}, len(questions)
}

func produceLessonPlan(req ProductionRequest, sources []GroundingSource) (map[string]any, int) {
	phaseMinutes := splitDuration(req.DurationMinutes)
	phases := []map[string]any{
		{"name": "导入", "minutes": phaseMinutes[0], "activities": []string{"回顾先备知识并说明本课目标"}},
		{"name": "讲授", "minutes": phaseMinutes[1], "activities": []string{"依据课程材料讲解核心概念与步骤"}},
		{"name": "练习", "minutes": phaseMinutes[2], "activities": []string{"完成分层练习并说明推理依据"}},
		{"name": "总结", "minutes": phaseMinutes[3], "activities": []string{"回顾目标并布置课后自测"}},
	}
	keyPointCount := len(sources)
	if keyPointCount > 4 {
		keyPointCount = 4
	}
	keyPoints := make([]map[string]any, 0, keyPointCount)
	for i := 0; i < keyPointCount; i++ {
		source := sources[i]
		keyPoints = append(keyPoints, map[string]any{
			"text":     strings.TrimSpace(source.Text),
			"citation": citationMap(sourceCitation(source)),
		})
	}
	return map[string]any{
		"title":           req.Topic + "教学设计",
		"topic":           req.Topic,
		"durationMinutes": req.DurationMinutes,
		"objectives":      append([]string(nil), req.Objectives...),
		"keyPoints":       keyPoints,
		"phases":          phases,
		"boardPlan":       "目标 → 核心概念 → 例题/练习 → 总结",
		"citations":       citationMaps(sources),
	}, len(phases)
}

// DeterministicQualityAgent independently checks schema and grounding. Its
// scope is deliberately explicit: it does not claim to verify facts against
// sources that were not supplied by the application.
type DeterministicQualityAgent struct{}

func NewDeterministicQualityAgent() *DeterministicQualityAgent {
	return &DeterministicQualityAgent{}
}

func (DeterministicQualityAgent) Review(ctx context.Context, req QualityReviewRequest) (QualityReport, error) {
	if err := ctx.Err(); err != nil {
		return QualityReport{}, err
	}
	sources, sourceErr := validateSources(req.Sources)
	report := QualityReport{
		Score:          100,
		Scope:          "schema_and_supplied_evidence",
		Issues:         []QualityIssue{},
		CheckedSources: len(req.Sources),
		ModelMeta: ModelMeta{
			Model:         DeterministicModelVersion,
			PromptVersion: QualityReviewPromptVersion,
			SkillVersion:  QualityReviewSkillVersion,
		},
	}
	if sourceErr != nil {
		report.Issues = append(report.Issues, qualityIssue(
			"INVALID_SOURCE_SNAPSHOT", SeverityError, "sources", sourceErr.Error(),
		))
		finalizeQualityReport(&report)
		return report, nil
	}
	if req.Payload == nil {
		report.Issues = append(report.Issues, qualityIssue(
			"EMPTY_PAYLOAD", SeverityError, "payload", "content payload is required",
		))
		finalizeQualityReport(&report)
		return report, nil
	}

	switch req.Skill {
	case SkillCourseCards:
		reviewCourseCards(&report, req.Payload, sources)
	case SkillExercises:
		reviewExercises(&report, req.Payload, sources)
	case SkillLessonPlan:
		reviewLessonPlan(&report, req.Payload, sources)
	default:
		return QualityReport{}, fmt.Errorf("%w: %s", ErrUnsupportedProductionSkill, req.Skill)
	}
	finalizeQualityReport(&report)
	return report, nil
}

func reviewCourseCards(report *QualityReport, payload map[string]any, sources []GroundingSource) {
	cards, ok := mapSlice(payload["cards"])
	if !ok || len(cards) < 3 {
		report.Issues = append(report.Issues, qualityIssue(
			"INSUFFICIENT_CARDS", SeverityError, "payload.cards", "at least three course cards are required",
		))
		return
	}
	report.CheckedItems = len(cards)
	for i, card := range cards {
		path := fmt.Sprintf("payload.cards[%d]", i)
		requireString(report, card, "title", path)
		requireString(report, card, "summary", path)
		reviewCitationValue(report, card["citation"], path+".citation", sources)
		requireGroundedText(
			report,
			stringValue(card["summary"]),
			citationSnippet(card["citation"]),
			path+".summary",
		)
	}
}

func reviewExercises(report *QualityReport, payload map[string]any, sources []GroundingSource) {
	questions, ok := mapSlice(payload["questions"])
	if !ok || len(questions) == 0 {
		report.Issues = append(report.Issues, qualityIssue(
			"MISSING_EXERCISES", SeverityError, "payload.questions", "at least one exercise is required",
		))
		return
	}
	report.CheckedItems = len(questions)
	for i, question := range questions {
		path := fmt.Sprintf("payload.questions[%d]", i)
		requireString(report, question, "stem", path)
		requireString(report, question, "answer", path)
		requireString(report, question, "explanation", path)
		exerciseType, _ := question["type"].(string)
		switch exerciseType {
		case "choice":
			options, validOptions := stringSlice(question["options"])
			answer := stringValue(question["answer"])
			if !validOptions || len(options) < 2 || !contains([]string{"A", "B", "C", "D"}, answer) {
				report.Issues = append(report.Issues, qualityIssue(
					"INVALID_CHOICE", SeverityError, path, "choice exercises require options and an A-D answer",
				))
			}
		case "fill":
			stem, _ := question["stem"].(string)
			if !strings.Contains(stem, "____") {
				report.Issues = append(report.Issues, qualityIssue(
					"INVALID_FILL", SeverityError, path+".stem", "fill exercises require a visible blank",
				))
			}
		case "short_answer":
		default:
			report.Issues = append(report.Issues, qualityIssue(
				"INVALID_EXERCISE_TYPE", SeverityError, path+".type", "unsupported exercise type",
			))
		}
		citations, ok := mapSlice(question["citations"])
		if !ok || len(citations) == 0 {
			report.Issues = append(report.Issues, qualityIssue(
				"MISSING_CITATION", SeverityError, path+".citations", "exercise citation is required",
			))
			continue
		}
		for j, citation := range citations {
			reviewCitationMap(report, citation, fmt.Sprintf("%s.citations[%d]", path, j), sources)
		}
		if len(citations) > 0 {
			requireGroundedAnswer(report, question, citations[0], path)
		}
	}
}

func reviewLessonPlan(report *QualityReport, payload map[string]any, sources []GroundingSource) {
	if title, _ := payload["title"].(string); strings.TrimSpace(title) == "" {
		report.Issues = append(report.Issues, qualityIssue(
			"MISSING_FIELD", SeverityError, "payload.title", "title is required",
		))
	}
	objectives, ok := stringSlice(payload["objectives"])
	if !ok || len(objectives) == 0 {
		report.Issues = append(report.Issues, qualityIssue(
			"MISSING_OBJECTIVES", SeverityError, "payload.objectives", "lesson objectives are required",
		))
	}
	keyPoints, ok := mapSlice(payload["keyPoints"])
	if !ok || len(keyPoints) == 0 {
		report.Issues = append(report.Issues, qualityIssue(
			"MISSING_KEY_POINTS", SeverityError, "payload.keyPoints", "grounded key points are required",
		))
	} else {
		for i, point := range keyPoints {
			path := fmt.Sprintf("payload.keyPoints[%d]", i)
			requireString(report, point, "text", path)
			reviewCitationValue(report, point["citation"], path+".citation", sources)
			requireGroundedText(
				report,
				stringValue(point["text"]),
				citationSnippet(point["citation"]),
				path+".text",
			)
		}
	}
	phases, ok := mapSlice(payload["phases"])
	if !ok || len(phases) == 0 {
		report.Issues = append(report.Issues, qualityIssue(
			"MISSING_PHASES", SeverityError, "payload.phases", "lesson phases are required",
		))
		return
	}
	report.CheckedItems = len(phases)
	total := 0
	for i, phase := range phases {
		path := fmt.Sprintf("payload.phases[%d]", i)
		requireString(report, phase, "name", path)
		minutes, ok := integerValue(phase["minutes"])
		if !ok || minutes <= 0 {
			report.Issues = append(report.Issues, qualityIssue(
				"INVALID_DURATION", SeverityError, path+".minutes", "phase minutes must be a positive integer",
			))
			continue
		}
		total += minutes
	}
	expected, ok := integerValue(payload["durationMinutes"])
	if !ok || expected <= 0 || total != expected {
		report.Issues = append(report.Issues, qualityIssue(
			"DURATION_MISMATCH", SeverityError, "payload.phases", "phase minutes must equal lesson duration",
		))
	}
}

func reviewCitationValue(report *QualityReport, value any, path string, sources []GroundingSource) {
	citation, ok := value.(map[string]any)
	if !ok {
		report.Issues = append(report.Issues, qualityIssue(
			"MISSING_CITATION", SeverityError, path, "grounding citation is required",
		))
		return
	}
	reviewCitationMap(report, citation, path, sources)
}

func reviewCitationMap(report *QualityReport, citation map[string]any, path string, sources []GroundingSource) {
	sourceID := stringValue(citation["sourceId"])
	sourceVersion := stringValue(citation["sourceVersion"])
	chunkID := stringValue(citation["chunkId"])
	contentHash := stringValue(citation["contentHash"])
	snippet := stringValue(citation["snippet"])
	characterStart, startOK := integerValue(citation["characterStart"])
	characterEnd, endOK := integerValue(citation["characterEnd"])
	for _, source := range sources {
		if source.SourceID != sourceID || source.SourceVersion != sourceVersion || source.ChunkID != chunkID {
			continue
		}
		if source.ContentHash != contentHash ||
			strings.TrimSpace(source.Text) != strings.TrimSpace(snippet) ||
			!startOK ||
			!endOK ||
			source.CharacterStart != characterStart ||
			source.CharacterEnd != characterEnd {
			report.Issues = append(report.Issues, qualityIssue(
				"CITATION_CONTENT_MISMATCH", SeverityError, path, "citation text, hash, or range does not match its source snapshot",
			))
			return
		}
		return
	}
	report.Issues = append(report.Issues, qualityIssue(
		"UNKNOWN_CITATION", SeverityError, path, "citation is not present in the supplied evidence set",
	))
}

func requireString(report *QualityReport, value map[string]any, key, path string) {
	if strings.TrimSpace(stringValue(value[key])) == "" {
		report.Issues = append(report.Issues, qualityIssue(
			"MISSING_FIELD", SeverityError, path+"."+key, key+" is required",
		))
	}
}

func finalizeQualityReport(report *QualityReport) {
	errorsCount := 0
	warningsCount := 0
	for _, issue := range report.Issues {
		if issue.Severity == SeverityError {
			errorsCount++
		} else {
			warningsCount++
		}
	}
	report.Score = 100 - errorsCount*20 - warningsCount*5
	if report.Score < 0 {
		report.Score = 0
	}
	report.Passed = errorsCount == 0
}

func qualityIssue(code string, severity IssueSeverity, path, message string) QualityIssue {
	return QualityIssue{Code: code, Severity: severity, Path: path, Message: message}
}

func validateProductionRequest(req ProductionRequest, sources []GroundingSource) error {
	if strings.TrimSpace(req.Topic) == "" {
		return fmt.Errorf("%w: topic is required", ErrInvalidProductionRequest)
	}
	switch req.Skill {
	case SkillCourseCards:
		if req.Count <= 0 || req.Count > len(sources) {
			return fmt.Errorf("%w: course-card count must be covered by distinct sources", ErrInvalidProductionRequest)
		}
	case SkillExercises:
		if req.Count <= 0 || req.Count > len(sources) {
			return fmt.Errorf("%w: exercise count must be covered by distinct sources", ErrInvalidProductionRequest)
		}
		if len(req.ExerciseTypes) == 0 {
			return fmt.Errorf("%w: exercise types are required", ErrInvalidProductionRequest)
		}
		for _, exerciseType := range req.ExerciseTypes {
			if !contains([]string{"choice", "fill", "short_answer"}, exerciseType) {
				return fmt.Errorf("%w: unsupported exercise type", ErrInvalidProductionRequest)
			}
		}
		if !contains([]string{"easy", "medium", "hard"}, req.Difficulty) {
			return fmt.Errorf("%w: unsupported difficulty", ErrInvalidProductionRequest)
		}
	case SkillLessonPlan:
		if req.DurationMinutes < 15 || req.DurationMinutes > 240 {
			return fmt.Errorf("%w: lesson duration must be between 15 and 240 minutes", ErrInvalidProductionRequest)
		}
		if len(req.Objectives) == 0 {
			return fmt.Errorf("%w: lesson objectives are required", ErrInvalidProductionRequest)
		}
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedProductionSkill, req.Skill)
	}
	return nil
}

func validateSources(sources []GroundingSource) ([]GroundingSource, error) {
	if len(sources) == 0 {
		return nil, fmt.Errorf("%w: no sources", ErrInvalidGroundingSource)
	}
	out := make([]GroundingSource, len(sources))
	seen := make(map[string]struct{}, len(sources))
	for i, source := range sources {
		source.SourceID = strings.TrimSpace(source.SourceID)
		source.SourceVersion = strings.TrimSpace(source.SourceVersion)
		source.ChunkID = strings.TrimSpace(source.ChunkID)
		source.Text = strings.TrimSpace(source.Text)
		key := source.SourceID + "\x00" + source.SourceVersion + "\x00" + source.ChunkID
		if source.SourceID == "" || source.SourceVersion == "" || source.ChunkID == "" || source.Text == "" {
			return nil, fmt.Errorf("%w at index %d: missing identity or text", ErrInvalidGroundingSource, i)
		}
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("%w at index %d: duplicate snapshot", ErrInvalidGroundingSource, i)
		}
		seen[key] = struct{}{}
		expectedHash := ContentHash(source.Text)
		if source.ContentHash == "" {
			source.ContentHash = expectedHash
		}
		if source.ContentHash != expectedHash {
			return nil, fmt.Errorf("%w at index %d: content hash mismatch", ErrInvalidGroundingSource, i)
		}
		if source.CharacterStart < 0 {
			return nil, fmt.Errorf("%w at index %d: invalid character range", ErrInvalidGroundingSource, i)
		}
		if source.CharacterEnd == 0 {
			source.CharacterEnd = source.CharacterStart + len(source.Text)
		}
		if source.CharacterEnd <= source.CharacterStart {
			return nil, fmt.Errorf("%w at index %d: invalid character range", ErrInvalidGroundingSource, i)
		}
		out[i] = source
	}
	return out, nil
}

func requireGroundedAnswer(
	report *QualityReport,
	question map[string]any,
	citation map[string]any,
	path string,
) {
	snippet := citationSnippet(citation)
	answer := stringValue(question["answer"])
	switch stringValue(question["type"]) {
	case "choice":
		options, ok := stringSlice(question["options"])
		index := answerIndex(answer)
		if !ok || index < 0 || index >= len(options) || strings.TrimSpace(options[index]) != snippet {
			report.Issues = append(report.Issues, qualityIssue(
				"ANSWER_NOT_GROUNDED", SeverityError, path+".answer", "the selected option must exactly match the cited source",
			))
		}
	case "fill":
		stem := stringValue(question["stem"])
		stem = strings.TrimPrefix(stem, "补全课程材料中的表述：")
		reconstructed := strings.Replace(stem, "____", answer, 1)
		if strings.TrimSpace(reconstructed) != snippet {
			report.Issues = append(report.Issues, qualityIssue(
				"ANSWER_NOT_GROUNDED", SeverityError, path+".answer", "the fill answer must reconstruct the cited source",
			))
		}
	case "short_answer":
		requireGroundedText(report, answer, snippet, path+".answer")
	}
}

func requireGroundedText(report *QualityReport, text, snippet, path string) {
	if strings.TrimSpace(text) == "" || strings.TrimSpace(text) != strings.TrimSpace(snippet) {
		report.Issues = append(report.Issues, qualityIssue(
			"UNGROUNDED_CONTENT", SeverityError, path, "content must exactly match its cited source in deterministic mode",
		))
	}
}

func citationSnippet(value any) string {
	citation, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	return stringValue(citation["snippet"])
}

func answerIndex(answer string) int {
	switch answer {
	case "A":
		return 0
	case "B":
		return 1
	case "C":
		return 2
	case "D":
		return 3
	default:
		return -1
	}
}

// ContentHash is shared by evidence adapters and the independent reviewer.
func ContentHash(text string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(text)))
	return hex.EncodeToString(sum[:])
}

func sourceCitation(source GroundingSource) Citation {
	return Citation{
		SourceID:       source.SourceID,
		SourceVersion:  source.SourceVersion,
		ChunkID:        source.ChunkID,
		CharacterStart: source.CharacterStart,
		CharacterEnd:   source.CharacterEnd,
		ContentHash:    source.ContentHash,
		Snippet:        source.Text,
		Score:          1,
	}
}

func citationMaps(sources []GroundingSource) []map[string]any {
	out := make([]map[string]any, 0, len(sources))
	for _, source := range sources {
		out = append(out, citationMap(sourceCitation(source)))
	}
	return out
}

func citationMap(citation Citation) map[string]any {
	return map[string]any{
		"sourceId":       citation.SourceID,
		"sourceVersion":  citation.SourceVersion,
		"chunkId":        citation.ChunkID,
		"characterStart": citation.CharacterStart,
		"characterEnd":   citation.CharacterEnd,
		"contentHash":    citation.ContentHash,
		"snippet":        citation.Snippet,
	}
}

func conciseTitle(text, topic string, index int) string {
	runes := []rune(strings.TrimSpace(text))
	for i, r := range runes {
		if unicode.IsPunct(r) || unicode.IsSpace(r) {
			runes = runes[:i]
			break
		}
	}
	if len(runes) > 18 {
		runes = runes[:18]
	}
	if len(runes) == 0 {
		return fmt.Sprintf("%s要点%d", topic, index)
	}
	return string(runes)
}

func makeCloze(text string) (string, string) {
	runes := []rune(strings.TrimSpace(text))
	answerLength := 4
	if len(runes) < answerLength {
		answerLength = len(runes)
	}
	answer := string(runes[:answerLength])
	return "____" + string(runes[answerLength:]), answer
}

func splitDuration(total int) [4]int {
	intro := total / 10
	instruction := total / 2
	practice := total * 3 / 10
	if intro < 1 {
		intro = 1
	}
	if instruction < 1 {
		instruction = 1
	}
	if practice < 1 {
		practice = 1
	}
	wrap := total - intro - instruction - practice
	if wrap < 1 {
		wrap = 1
		instruction = total - intro - practice - wrap
	}
	return [4]int{intro, instruction, practice, wrap}
}

func mapSlice(value any) ([]map[string]any, bool) {
	switch typed := value.(type) {
	case []map[string]any:
		return typed, true
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			asMap, ok := item.(map[string]any)
			if !ok {
				return nil, false
			}
			out = append(out, asMap)
		}
		return out, true
	default:
		return nil, false
	}
}

func stringSlice(value any) ([]string, bool) {
	switch typed := value.(type) {
	case []string:
		return typed, true
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			asString, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, asString)
		}
		return out, true
	default:
		return nil, false
	}
}

func integerValue(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float64:
		if typed != float64(int(typed)) {
			return 0, false
		}
		return int(typed), true
	case json.Number:
		n, err := typed.Int64()
		return int(n), err == nil
	default:
		return 0, false
	}
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
