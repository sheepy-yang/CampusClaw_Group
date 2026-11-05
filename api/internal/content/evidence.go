package content

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/agent"
)

type EvidenceProvider interface {
	Resolve(
		ctx context.Context,
		classID string,
		refs []EvidenceRef,
	) ([]agent.GroundingSource, error)
}

type SkillGate interface {
	Enabled(ctx context.Context, classID string, skill agent.ProductionSkill) (bool, error)
}

type EvidenceProviderFunc func(
	ctx context.Context,
	classID string,
	refs []EvidenceRef,
) ([]agent.GroundingSource, error)

func (f EvidenceProviderFunc) Resolve(
	ctx context.Context,
	classID string,
	refs []EvidenceRef,
) ([]agent.GroundingSource, error) {
	return f(ctx, classID, refs)
}

type SkillGateFunc func(
	ctx context.Context,
	classID string,
	skill agent.ProductionSkill,
) (bool, error)

func (f SkillGateFunc) Enabled(
	ctx context.Context,
	classID string,
	skill agent.ProductionSkill,
) (bool, error) {
	return f(ctx, classID, skill)
}

type Material struct {
	ID      string
	ClassID string
	Content string
	Chunks  []MaterialChunk
}

type MaterialChunk struct {
	ID   string
	Text string
}

type MaterialReader interface {
	MaterialsByClass(classID string) []Material
}

type MaterialReaderFunc func(classID string) []Material

func (f MaterialReaderFunc) MaterialsByClass(classID string) []Material {
	return f(classID)
}

type StoreEvidenceProvider struct {
	materials MaterialReader
}

func NewStoreEvidenceProvider(materials MaterialReader) *StoreEvidenceProvider {
	return &StoreEvidenceProvider{materials: materials}
}

// Resolve obtains evidence from a server-scoped material reader. A missing
// source and a source from another class deliberately produce the same error.
func (p *StoreEvidenceProvider) Resolve(
	ctx context.Context,
	classID string,
	refs []EvidenceRef,
) ([]agent.GroundingSource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p == nil || p.materials == nil || strings.TrimSpace(classID) == "" {
		return nil, ErrEvidenceNotFound
	}
	materials := p.materials.MaterialsByClass(classID)
	sort.Slice(materials, func(i, j int) bool {
		return materials[i].ID < materials[j].ID
	})
	if len(materials) == 0 {
		return nil, ErrEvidenceNotFound
	}
	if len(refs) == 0 {
		for _, material := range materials {
			refs = append(refs, EvidenceRef{MaterialID: material.ID})
		}
	}

	materialByID := make(map[string]Material, len(materials))
	for _, material := range materials {
		materialByID[material.ID] = material
	}
	seen := make(map[string]struct{})
	var resolved []agent.GroundingSource
	for _, ref := range refs {
		material, ok := materialByID[strings.TrimSpace(ref.MaterialID)]
		if !ok {
			return nil, ErrEvidenceNotFound
		}
		chunks := append([]MaterialChunk(nil), material.Chunks...)
		sort.Slice(chunks, func(i, j int) bool {
			return chunks[i].ID < chunks[j].ID
		})
		foundChunk := false
		for _, chunk := range chunks {
			if ref.ChunkID != "" && chunk.ID != strings.TrimSpace(ref.ChunkID) {
				continue
			}
			foundChunk = true
			key := material.ID + "\x00" + chunk.ID
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			rawText := chunk.Text
			text := strings.TrimSpace(rawText)
			if text == "" {
				continue
			}
			start := strings.Index(material.Content, rawText)
			if start < 0 {
				return nil, fmt.Errorf("material %s chunk %s has no immutable text range", material.ID, chunk.ID)
			}
			leftTrimmed := strings.TrimLeftFunc(rawText, unicode.IsSpace)
			start += len(rawText) - len(leftTrimmed)
			materialHash := agent.ContentHash(material.Content)
			resolved = append(resolved, agent.GroundingSource{
				SourceID:       material.ID,
				SourceVersion:  "sha256:" + materialHash[:16],
				ChunkID:        chunk.ID,
				CharacterStart: start,
				CharacterEnd:   start + len(text),
				ContentHash:    agent.ContentHash(text),
				Text:           text,
			})
		}
		if !foundChunk {
			return nil, ErrEvidenceNotFound
		}
	}
	if len(resolved) == 0 {
		return nil, ErrEvidenceNotFound
	}
	return resolved, nil
}

type StoreSkillGate struct {
	store interface {
		SkillEnabled(classID string, skill string) bool
	}
}

func NewStoreSkillGate(skillStore interface {
	SkillEnabled(classID string, skill string) bool
}) *StoreSkillGate {
	return &StoreSkillGate{store: skillStore}
}

func (g *StoreSkillGate) Enabled(
	ctx context.Context,
	classID string,
	skill agent.ProductionSkill,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if g == nil || g.store == nil {
		return false, fmt.Errorf("skill gate is unavailable")
	}
	var configuredSkill string
	switch skill {
	case agent.SkillCourseCards:
		configuredSkill = "preview_sheet"
	case agent.SkillExercises:
		configuredSkill = "proposition"
	case agent.SkillLessonPlan:
		configuredSkill = "lesson_plan"
	default:
		return false, fmt.Errorf("%w: %s", agent.ErrUnsupportedProductionSkill, skill)
	}
	return g.store.SkillEnabled(classID, configuredSkill), nil
}

type AllowAllSkillGate struct{}

func (AllowAllSkillGate) Enabled(
	ctx context.Context,
	_ string,
	skill agent.ProductionSkill,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	switch skill {
	case agent.SkillCourseCards, agent.SkillExercises, agent.SkillLessonPlan:
		return true, nil
	default:
		return false, fmt.Errorf("%w: %s", agent.ErrUnsupportedProductionSkill, skill)
	}
}

type unavailableEvidenceProvider struct{}

func (unavailableEvidenceProvider) Resolve(
	context.Context,
	string,
	[]EvidenceRef,
) ([]agent.GroundingSource, error) {
	return nil, ErrEvidenceUnavailable
}

type unavailableSkillGate struct{}

func (unavailableSkillGate) Enabled(
	context.Context,
	string,
	agent.ProductionSkill,
) (bool, error) {
	return false, ErrSkillGateUnavailable
}
