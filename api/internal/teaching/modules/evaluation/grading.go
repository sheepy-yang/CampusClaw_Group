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

// SplitSteps 将转写文本划分为步骤（按换行 / 「步骤N」）。
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
		ok := !strings.Contains(line, "失误") && !strings.Contains(line, "错误") && !strings.Contains(line, "漏")
		comment := ""
		if !ok {
			comment = "疑似出错"
		}
		steps = append(steps, GradeStep{Index: idx, Text: line, OK: ok, Comment: comment})
	}
	if len(steps) == 0 && strings.TrimSpace(transcription) != "" {
		steps = append(steps, GradeStep{Index: 1, Text: strings.TrimSpace(transcription), OK: true})
	}
	return steps
}

// LocateFirstError 首错定位：返回第一个不 OK 的步骤标签。
func LocateFirstError(steps []GradeStep) string {
	for _, s := range steps {
		if !s.OK {
			return "步骤" + itoa(s.Index)
		}
	}
	return ""
}

// SuggestSubjectiveScore 根据步骤对错给出建议分（满分 maxScore）。
func SuggestSubjectiveScore(steps []GradeStep, maxScore float64) (suggested float64, confidence float64, cause string) {
	if maxScore <= 0 {
		maxScore = 10
	}
	if len(steps) == 0 {
		return 0, 0.4, "无可解析步骤"
	}
	okCount := 0
	for _, s := range steps {
		if s.OK {
			okCount++
		}
	}
	ratio := float64(okCount) / float64(len(steps))
	suggested = round1(ratio * maxScore)
	confidence = 0.55 + 0.35*ratio
	if confidence > 0.95 {
		confidence = 0.95
	}
	cause = ""
	if first := LocateFirstError(steps); first != "" {
		cause = "计算或步骤失误"
		for _, s := range steps {
			if !s.OK {
				if strings.Contains(s.Text, "秩") || strings.Contains(s.Text, "概念") {
					cause = "概念混淆"
				}
				break
			}
		}
		_ = first
	}
	return suggested, confidence, cause
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
