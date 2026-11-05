package evaluation

import (
	"strings"
	"unicode"
)

// GradeObjective 客观题确定性规则判分，不调用大模型。
// qType: choice | fill | blank
func GradeObjective(qType, studentAnswer, standardAnswer string) (score float64, max float64, correct bool) {
	max = 1
	got := normalizeAnswer(studentAnswer)
	want := normalizeAnswer(standardAnswer)
	if got == "" {
		return 0, max, false
	}
	switch strings.ToLower(qType) {
	case "choice", "single", "multi":
		correct = got == want
	default: // fill / blank：允许多个标准答案用 | 分隔
		for _, alt := range strings.Split(want, "|") {
			if got == strings.TrimSpace(alt) {
				correct = true
				break
			}
		}
	}
	if correct {
		return max, max, true
	}
	return 0, max, false
}

func normalizeAnswer(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	var b strings.Builder
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// SplitSteps 只负责划分转写文本，不根据答案里是否出现“错误”等字样推断对错。
func SplitSteps(transcription string) []GradeStep {
	lines := strings.Split(transcription, "\n")
	var steps []GradeStep
	idx := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx++
		steps = append(steps, GradeStep{
			Index: idx, Text: line, Verified: false,
			Comment: "待教师依据已发布评分规则核验",
		})
	}
	if len(steps) == 0 && strings.TrimSpace(transcription) != "" {
		steps = append(steps, GradeStep{
			Index: 1, Text: strings.TrimSpace(transcription), Verified: false,
			Comment: "待教师依据已发布评分规则核验",
		})
	}
	return steps
}

// LocateFirstError 首错定位：返回第一个不 OK 的步骤标签。
func LocateFirstError(steps []GradeStep) string {
	for _, s := range steps {
		if s.Verified && !s.OK {
			return "步骤" + itoa(s.Index)
		}
	}
	return ""
}

// SuggestSubjectiveScore 保留给无评分规则的调用方；没有可信规则时不猜测步骤对错。
func SuggestSubjectiveScore(steps []GradeStep, maxScore float64) (suggested float64, confidence float64, cause string) {
	if maxScore <= 0 {
		maxScore = 10
	}
	if len(steps) == 0 {
		return 0, 0.4, "无可解析步骤"
	}
	verifiedCount := 0
	okCount := 0
	for _, s := range steps {
		if s.Verified {
			verifiedCount++
		}
		if s.Verified && s.OK {
			okCount++
		}
	}
	if verifiedCount == 0 {
		return 0, 0.3, "缺少已发布评分规则的核验结果"
	}
	ratio := float64(okCount) / float64(verifiedCount)
	suggested = round1(ratio * maxScore)
	confidence = 0.5 + 0.25*ratio
	if confidence > 0.75 {
		confidence = 0.75
	}
	cause = "与教师发布的评分依据匹配不足"
	if first := LocateFirstError(steps); first != "" {
		_ = first
	} else {
		cause = ""
	}
	return suggested, confidence, cause
}

// SuggestSubjectiveScoreFromRubric 仅使用教师已发布的参考答案生成低置信度建议；
// 主观题无论匹配度如何都必须由教师最终确认。
func SuggestSubjectiveScoreFromRubric(answer, rubric string, maxScore float64) (suggested float64, confidence float64, cause string) {
	if maxScore <= 0 {
		maxScore = 10
	}
	got := normalizedComparableRunes(answer)
	want := normalizedComparableRunes(rubric)
	if len(got) == 0 || len(want) == 0 {
		return 0, 0.3, "缺少可核验的作答或评分依据"
	}
	ratio := ngramRecall(got, want)
	if string(got) == string(want) || (len(want) >= 2 && strings.Contains(string(got), string(want))) {
		ratio = 1
	}
	suggested = round1(ratio * maxScore)
	confidence = 0.45 + 0.3*ratio
	if confidence > 0.75 {
		confidence = 0.75
	}
	if ratio < 0.6 {
		cause = "与教师发布的评分依据匹配不足"
	}
	return suggested, confidence, cause
}

func normalizedComparableRunes(value string) []rune {
	out := make([]rune, 0, len([]rune(value)))
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			out = append(out, r)
		}
	}
	return out
}

func ngramRecall(got, want []rune) float64 {
	width := 2
	if len(want) < width {
		if strings.Contains(string(got), string(want)) {
			return 1
		}
		return 0
	}
	expected := map[string]struct{}{}
	for i := 0; i <= len(want)-width; i++ {
		expected[string(want[i:i+width])] = struct{}{}
	}
	observed := map[string]struct{}{}
	for i := 0; i <= len(got)-width; i++ {
		observed[string(got[i:i+width])] = struct{}{}
	}
	matched := 0
	for gram := range expected {
		if _, ok := observed[gram]; ok {
			matched++
		}
	}
	return float64(matched) / float64(len(expected))
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// NeedsTeacherReview 低置信度、工具冲突或依据不足 → 待教师复核。
func NeedsTeacherReview(confidence float64, toolConflict bool, evidenceWeak bool) bool {
	if confidence < LowConfidenceThreshold {
		return true
	}
	if toolConflict || evidenceWeak {
		return true
	}
	return false
}
