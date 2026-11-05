package content

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/agent"
)

func TestStoreEvidenceProviderResolvesImmutableClassScopedRanges(t *testing.T) {
	// Scenario (normal): server-owned class scope resolves deterministic source
	// versions, exact trimmed ranges, hashes, and stable chunk order.
	rawChunk := "  高斯消元通过初等行变换求阶梯型。  "
	content := "导言。" + rawChunk + "结语。"
	reader := materialReaderFixture{
		"class-a": {
			{
				ID:      "material-a",
				ClassID: "class-a",
				Content: content,
				Chunks: []MaterialChunk{
					{ID: "chunk-2", Text: "矩阵可表示方程组。"},
					{ID: "chunk-1", Text: rawChunk},
				},
			},
		},
	}
	// Make both chunks valid immutable ranges.
	material := reader["class-a"][0]
	material.Content += "矩阵可表示方程组。"
	reader["class-a"] = []Material{material}

	sources, err := NewStoreEvidenceProvider(reader).Resolve(
		context.Background(),
		"class-a",
		[]EvidenceRef{{MaterialID: "material-a"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 || sources[0].ChunkID != "chunk-1" || sources[1].ChunkID != "chunk-2" {
		t.Fatalf("sources are not deterministically sorted: %#v", sources)
	}
	first := sources[0]
	wantText := strings.TrimSpace(rawChunk)
	wantStart := strings.Index(material.Content, rawChunk) + len(rawChunk) - len(strings.TrimLeft(rawChunk, " \t\r\n"))
	if first.Text != wantText ||
		first.CharacterStart != wantStart ||
		first.CharacterEnd != wantStart+len(wantText) ||
		first.ContentHash != agent.ContentHash(wantText) ||
		!strings.HasPrefix(first.SourceVersion, "sha256:") {
		t.Fatalf("invalid immutable evidence snapshot: %#v", first)
	}
}

func TestStoreEvidenceProviderHidesMissingAndCrossClassSources(t *testing.T) {
	// Scenario (authorization/failure): missing chunks and another class's
	// material are indistinguishable to the caller.
	reader := materialReaderFixture{
		"class-a": {{
			ID:      "material-a",
			ClassID: "class-a",
			Content: "本班材料。",
			Chunks:  []MaterialChunk{{ID: "chunk-a", Text: "本班材料。"}},
		}},
		"class-b": {{
			ID:      "material-b",
			ClassID: "class-b",
			Content: "他班材料。",
			Chunks:  []MaterialChunk{{ID: "chunk-b", Text: "他班材料。"}},
		}},
	}
	provider := NewStoreEvidenceProvider(reader)
	cases := []struct {
		name  string
		class string
		ref   EvidenceRef
	}{
		{name: "cross class material", class: "class-b", ref: EvidenceRef{MaterialID: "material-a"}},
		{name: "missing chunk", class: "class-a", ref: EvidenceRef{MaterialID: "material-a", ChunkID: "missing"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := provider.Resolve(context.Background(), testCase.class, []EvidenceRef{testCase.ref})
			if !errors.Is(err, ErrEvidenceNotFound) {
				t.Fatalf("expected indistinguishable not found, got %v", err)
			}
		})
	}
}

func TestStoreEvidenceProviderRejectsInvalidMaterialRange(t *testing.T) {
	// Scenario (failure): a chunk that is not present in its material is not
	// assigned a fabricated character range.
	reader := materialReaderFixture{
		"class-a": {{
			ID:      "material-a",
			ClassID: "class-a",
			Content: "原始材料。",
			Chunks:  []MaterialChunk{{ID: "chunk-a", Text: "不在原文中的片段。"}},
		}},
	}
	_, err := NewStoreEvidenceProvider(reader).Resolve(context.Background(), "class-a", nil)
	if err == nil {
		t.Fatal("expected invalid range error")
	}
}

func TestStoreSkillGateUsesExistingSkillIdentifiers(t *testing.T) {
	// Scenario (normal): new production contracts map to the repository's
	// existing preview/proposition/lesson-plan switches.
	fixture := &skillStoreFixture{enabled: map[string]bool{
		"preview_sheet": true,
		"proposition":   false,
		"lesson_plan":   true,
	}}
	gate := NewStoreSkillGate(fixture)
	cases := []struct {
		skill agent.ProductionSkill
		want  bool
	}{
		{skill: agent.SkillCourseCards, want: true},
		{skill: agent.SkillExercises, want: false},
		{skill: agent.SkillLessonPlan, want: true},
	}
	for _, testCase := range cases {
		got, err := gate.Enabled(context.Background(), "class-a", testCase.skill)
		if err != nil {
			t.Fatal(err)
		}
		if got != testCase.want {
			t.Fatalf("skill %s enabled=%v want=%v", testCase.skill, got, testCase.want)
		}
	}
}

type materialReaderFixture map[string][]Material

func (r materialReaderFixture) MaterialsByClass(classID string) []Material {
	return append([]Material(nil), r[classID]...)
}

type skillStoreFixture struct {
	enabled map[string]bool
}

func (s *skillStoreFixture) SkillEnabled(_ string, skill string) bool {
	return s.enabled[skill]
}
