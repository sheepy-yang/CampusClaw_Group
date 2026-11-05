package evaluation

import (
	"strings"
	"unicode/utf8"
)

// CompareFragments 比较作答与参考资料 / 历史版本，返回相似片段证据。
// 不生成学术不端结论。
func CompareFragments(answer string, sources map[string]string) []SimilarityItem {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return nil
	}
	var hits []SimilarityItem
	for srcName, srcText := range sources {
		frags := windowFragments(srcText, 8, 24)
		for _, frag := range frags {
			if utf8.RuneCountInString(frag) < 6 {
				continue
			}
			sim := jaccardRunes(answer, frag)
			if sim < 0.45 {
				continue
			}
			loc := locateIn(answer, frag)
			hits = append(hits, SimilarityItem{
				Fragment: frag, Source: srcName, Similarity: round2(sim), Location: loc,
			})
		}
	}
	// 按相似度降序，最多 8 条
	for i := 0; i < len(hits); i++ {
		for j := i + 1; j < len(hits); j++ {
			if hits[j].Similarity > hits[i].Similarity {
				hits[i], hits[j] = hits[j], hits[i]
			}
		}
	}
	if len(hits) > 8 {
		hits = hits[:8]
	}
	return hits
}

func windowFragments(text string, minRunes, maxRunes int) []string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return nil
	}
	var out []string
	step := minRunes
	if step < 4 {
		step = 4
	}
	for i := 0; i < len(runes); i += step {
		end := i + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		if end-i >= minRunes {
			out = append(out, string(runes[i:end]))
		}
		if end == len(runes) {
			break
		}
	}
	// 也按句号切
	for _, part := range strings.FieldsFunc(text, func(r rune) bool {
		return r == '。' || r == '；' || r == '\n' || r == '.'
	}) {
		part = strings.TrimSpace(part)
		if utf8.RuneCountInString(part) >= minRunes {
			out = append(out, part)
		}
	}
	return out
}

func jaccardRunes(a, b string) float64 {
	sa := runeSet(a)
	sb := runeSet(b)
	if len(sa) == 0 || len(sb) == 0 {
		return 0
	}
	inter := 0
	for r := range sa {
		if sb[r] {
			inter++
		}
	}
	union := len(sa) + len(sb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func runeSet(s string) map[rune]bool {
	m := map[rune]bool{}
	for _, r := range s {
		if unicodeIsIgnorable(r) {
			continue
		}
		m[r] = true
	}
	return m
}

func unicodeIsIgnorable(r rune) bool {
	return r == ' ' || r == '\n' || r == '\t' || r == '，' || r == ',' || r == '、'
}

func locateIn(answer, frag string) string {
	idx := strings.Index(answer, frag)
	if idx < 0 {
		// 取 frag 前缀试探
		rs := []rune(frag)
		if len(rs) > 6 {
			prefix := string(rs[:6])
			idx = strings.Index(answer, prefix)
		}
	}
	if idx < 0 {
		return "全文"
	}
	pos := utf8.RuneCountInString(answer[:idx]) + 1
	return "约第" + itoa(pos) + "字起"
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}

// BuildSimilarityReport 组装审查报告；Verdict 恒为 nil。
func BuildSimilarityReport(attemptID string, items []SimilarityItem) SimilarityReport {
	return SimilarityReport{
		AttemptID: attemptID,
		Evidence:  items,
		Note:      "AI 仅归纳需关注的证据，不直接生成学术不端结论；由教师复核与处置",
		Verdict:   nil,
	}
}
