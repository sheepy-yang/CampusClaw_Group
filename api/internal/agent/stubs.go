package agent

import "context"

// --- 以下均为骨架 stub，按分工逐步替换为真实实现 ---

type StubOrchestrator struct{}

func (StubOrchestrator) Plan(_ context.Context, _ string, _ string) ([]string, error) {
	panic("TODO: Orchestrator.Plan")
}
func (StubOrchestrator) Run(_ context.Context, _ []string, _ map[string]any) (*StructuredResult, error) {
	panic("TODO: Orchestrator.Run")
}

type StubContentAgent struct{}

func (StubContentAgent) Generate(_ context.Context, _ map[string]any) (*StructuredResult, error) {
	panic("TODO: ContentAgent.Generate")
}

type StubVirtualStudentAgent struct{}

func (StubVirtualStudentAgent) Simulate(_ context.Context, _ string, _ []string, _ int) (*StructuredResult, error) {
	panic("TODO: VirtualStudentAgent.Simulate")
}

type StubVerifierAgent struct{}

func (StubVerifierAgent) Verify(_ context.Context, _ *StructuredResult, _ VerifyRules) (bool, []string, error) {
	panic("TODO: VerifierAgent.Verify")
}

type StubTutorAgent struct{}

func (StubTutorAgent) NextTurn(_ context.Context, _ map[string]any, _ string) (*StructuredResult, error) {
	panic("TODO: TutorAgent.NextTurn")
}
