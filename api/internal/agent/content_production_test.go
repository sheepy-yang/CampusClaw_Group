package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestDeterministicContentAgentProducesGroundedSkills(t *testing.T) {
	// Scenario (normal): each production skill returns deterministic,
	// independently reviewed content grounded only in supplied evidence.
	producer := NewDeterministicContentAgent()
	reviewer := NewDeterministicQualityAgent()
	sources := productionTestSources()
	cases := []struct {
		name    string
		request ProductionRequest
	}{
		{
			name: "course cards",
			request: ProductionRequest{
				Skill: SkillCourseCards,
				Topic: "线性方程组",
				Count: 3,
			},
		},
		{
			name: "exercises",
			request: ProductionRequest{
				Skill:         SkillExercises,
				Topic:         "线性方程组",
				Count:         3,
				ExerciseTypes: []string{"choice", "fill", "short_answer"},
				Difficulty:    "medium",
			},
		},
		{
			name: "lesson plan",
			request: ProductionRequest{
				Skill:           SkillLessonPlan,
				Topic:           "线性方程组",
				DurationMinutes: 45,
				Objectives:      []string{"理解矩阵表示", "掌握高斯消元"},
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := testCase.request
			request.Sources = sources
			first, err := producer.Produce(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			second, err := producer.Produce(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatal("deterministic fallback returned different results")
			}
			if first.Usage.Mode != "deterministic_fallback" ||
				first.Usage.SourceCount != len(sources) ||
				first.ModelMeta.SkillVersion == "" {
				t.Fatalf("missing usage metadata: %#v", first.Usage)
			}
			for _, citation := range first.Citations {
				if citation.ContentHash != ContentHash(citation.Snippet) {
					t.Fatalf("citation is not bound to its snippet: %#v", citation)
				}
			}
			report, err := reviewer.Review(context.Background(), QualityReviewRequest{
				Skill:   request.Skill,
				Payload: first.Payload,
				Sources: sources,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !report.Passed || len(report.Issues) != 0 {
				t.Fatalf("review unexpectedly failed: %#v", report)
			}
		})
	}
}

func TestQualityReviewRejectsFabricatedCitation(t *testing.T) {
	// Scenario (failure): a citation absent from the supplied evidence set is
	// rejected; the reviewer does not invent or fetch external evidence.
	sources := productionTestSources()
	output, err := NewDeterministicContentAgent().Produce(context.Background(), ProductionRequest{
		Skill:   SkillCourseCards,
		Topic:   "线性方程组",
		Count:   3,
		Sources: sources,
	})
	if err != nil {
		t.Fatal(err)
	}
	cards := output.Payload["cards"].([]map[string]any)
	cards[0]["citation"].(map[string]any)["sourceId"] = "outside-class-material"

	report, err := NewDeterministicQualityAgent().Review(context.Background(), QualityReviewRequest{
		Skill:   SkillCourseCards,
		Payload: output.Payload,
		Sources: sources,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || !hasQualityIssue(report, "UNKNOWN_CITATION") {
		t.Fatalf("fabricated citation passed review: %#v", report)
	}
}

func TestQualityReviewRejectsClaimsNotSupportedByCitation(t *testing.T) {
	// Scenario (failure): attaching a valid citation to unrelated card text,
	// answers, or lesson key points does not make the claim grounded.
	producer := NewDeterministicContentAgent()
	reviewer := NewDeterministicQualityAgent()
	sources := productionTestSources()
	cases := []struct {
		name    string
		request ProductionRequest
		mutate  func(map[string]any)
	}{
		{
			name: "course card summary",
			request: ProductionRequest{
				Skill:   SkillCourseCards,
				Topic:   "线性方程组",
				Count:   3,
				Sources: sources,
			},
			mutate: func(payload map[string]any) {
				payload["cards"].([]map[string]any)[0]["summary"] = "材料没有给出的外部结论"
			},
		},
		{
			name: "exercise answer",
			request: ProductionRequest{
				Skill:         SkillExercises,
				Topic:         "线性方程组",
				Count:         1,
				ExerciseTypes: []string{"short_answer"},
				Difficulty:    "medium",
				Sources:       sources,
			},
			mutate: func(payload map[string]any) {
				payload["questions"].([]map[string]any)[0]["answer"] = "未经材料支持的答案"
			},
		},
		{
			name: "lesson key point",
			request: ProductionRequest{
				Skill:           SkillLessonPlan,
				Topic:           "线性方程组",
				DurationMinutes: 45,
				Objectives:      []string{"掌握高斯消元"},
				Sources:         sources,
			},
			mutate: func(payload map[string]any) {
				payload["keyPoints"].([]map[string]any)[0]["text"] = "未经材料支持的重点"
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			output, err := producer.Produce(context.Background(), testCase.request)
			if err != nil {
				t.Fatal(err)
			}
			testCase.mutate(output.Payload)
			report, err := reviewer.Review(context.Background(), QualityReviewRequest{
				Skill:   testCase.request.Skill,
				Payload: output.Payload,
				Sources: sources,
			})
			if err != nil {
				t.Fatal(err)
			}
			if report.Passed ||
				(!hasQualityIssue(report, "UNGROUNDED_CONTENT") &&
					!hasQualityIssue(report, "ANSWER_NOT_GROUNDED")) {
				t.Fatalf("unsupported content passed review: %#v", report)
			}
		})
	}
}

func TestQualityReviewRejectsMalformedSchemas(t *testing.T) {
	// Scenario (failure/boundary): schema failures are explicit for all three
	// generated artifact kinds.
	sources := productionTestSources()
	validCitation := citationMap(sourceCitation(sources[0]))
	cases := []struct {
		name    string
		request QualityReviewRequest
		code    string
	}{
		{
			name: "fewer than three cards",
			request: QualityReviewRequest{
				Skill: SkillCourseCards,
				Payload: map[string]any{
					"cards": []map[string]any{{
						"title":    "卡片",
						"summary":  sources[0].Text,
						"citation": validCitation,
					}},
				},
				Sources: sources,
			},
			code: "INSUFFICIENT_CARDS",
		},
		{
			name: "exercise missing answer",
			request: QualityReviewRequest{
				Skill: SkillExercises,
				Payload: map[string]any{
					"questions": []map[string]any{{
						"type":        "short_answer",
						"stem":        "问题",
						"explanation": "依据材料",
						"citations":   []map[string]any{validCitation},
					}},
				},
				Sources: sources,
			},
			code: "MISSING_FIELD",
		},
		{
			name: "lesson duration mismatch",
			request: QualityReviewRequest{
				Skill: SkillLessonPlan,
				Payload: map[string]any{
					"title":           "教案",
					"durationMinutes": 45,
					"objectives":      []string{"目标"},
					"keyPoints": []map[string]any{{
						"text":     sources[0].Text,
						"citation": validCitation,
					}},
					"phases": []map[string]any{{
						"name":    "讲授",
						"minutes": 20,
					}},
				},
				Sources: sources,
			},
			code: "DURATION_MISMATCH",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			report, err := NewDeterministicQualityAgent().Review(context.Background(), testCase.request)
			if err != nil {
				t.Fatal(err)
			}
			if report.Passed || !hasQualityIssue(report, testCase.code) {
				t.Fatalf("expected %s: %#v", testCase.code, report)
			}
		})
	}
}

func TestDeterministicContentAgentRejectsInvalidEvidenceAndCancellation(t *testing.T) {
	// Scenario (failure): tampered evidence and canceled work fail explicitly.
	source := productionTestSources()[0]
	source.ContentHash = "tampered"
	_, err := NewDeterministicContentAgent().Produce(context.Background(), ProductionRequest{
		Skill:   SkillCourseCards,
		Topic:   "线性方程组",
		Count:   3,
		Sources: []GroundingSource{source},
	})
	if !errors.Is(err, ErrInvalidGroundingSource) {
		t.Fatalf("expected invalid source error, got %v", err)
	}
	_, err = NewDeterministicContentAgent().Produce(context.Background(), ProductionRequest{
		Skill:   SkillCourseCards,
		Topic:   "线性方程组",
		Count:   -1,
		Sources: productionTestSources(),
	})
	if !errors.Is(err, ErrInvalidProductionRequest) {
		t.Fatalf("expected invalid request error, got %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = NewDeterministicContentAgent().Produce(ctx, ProductionRequest{
		Skill:   SkillCourseCards,
		Topic:   "线性方程组",
		Count:   3,
		Sources: productionTestSources(),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled context, got %v", err)
	}
}

func productionTestSources() []GroundingSource {
	texts := []string{
		"线性方程组可用矩阵表示。",
		"高斯消元法通过初等行变换求阶梯型。",
		"系数矩阵与增广矩阵秩相等时方程组有解。",
	}
	sources := make([]GroundingSource, 0, len(texts))
	for i, text := range texts {
		sources = append(sources, GroundingSource{
			SourceID:       "material-1",
			SourceVersion:  "version-1",
			ChunkID:        "chunk-" + string(rune('1'+i)),
			CharacterStart: i * 20,
			CharacterEnd:   i*20 + len(text),
			ContentHash:    ContentHash(text),
			Text:           text,
		})
	}
	return sources
}

func hasQualityIssue(report QualityReport, code string) bool {
	for _, issue := range report.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
