package agent

import (
	"context"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/apperrors"
)

// --- 以下均为骨架 stub，按分工逐步替换为真实实现 ---

type StubOrchestrator struct{}

func (StubOrchestrator) Plan(_ context.Context, _ string, _ string) ([]string, error) {
	return nil, apperrors.ErrNotImplemented
}
func (StubOrchestrator) Run(_ context.Context, _ []string, _ map[string]any) (*StructuredResult, error) {
	return nil, apperrors.ErrNotImplemented
}

type StubContentAgent struct{}

func (StubContentAgent) Generate(_ context.Context, _ map[string]any) (*StructuredResult, error) {
	return nil, apperrors.ErrNotImplemented
}

type StubVirtualStudentAgent struct{}

func (StubVirtualStudentAgent) Simulate(_ context.Context, _ string, _ []string, _ int) (*StructuredResult, error) {
	return nil, apperrors.ErrNotImplemented
}

type StubVerifierAgent struct{}

func (StubVerifierAgent) Verify(_ context.Context, _ *StructuredResult, _ VerifyRules) (bool, []string, error) {
	return false, nil, apperrors.ErrNotImplemented
}

type StubTutorAgent struct{}

func (StubTutorAgent) NextTurn(_ context.Context, _ map[string]any, _ string) (*StructuredResult, error) {
	return nil, apperrors.ErrNotImplemented
}
