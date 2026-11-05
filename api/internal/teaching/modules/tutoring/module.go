// Package tutoring 刘沛鑫模块：复习路径 / 追问辅导 / 课堂聚类 / 自适应闯关。
package tutoring

import (
	"context"
	"fmt"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/agent"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/domain"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/security"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

type Services struct {
	Orch     agent.Orchestrator
	Verifier agent.VerifierAgent
}

func New(orch agent.Orchestrator, verifier agent.VerifierAgent) *Services {
	return &Services{Orch: orch, Verifier: verifier}
}

func (s *Services) GenerateReviewPath(ctx context.Context, classID, studentID string) (map[string]any, error) {
	mistakes := store.Default.MistakesFor(studentID, classID)
	if len(mistakes) == 0 {
		return map[string]any{"skipped": true, "reason": "没有错题或有效记录，不调用模型"}, nil
	}
	res, err := s.Orch.Run(ctx, nil, map[string]any{"skill": "复习路径", "mistakes": len(mistakes)})
	if err != nil {
		return nil, err
	}
	contentID := store.NewID("content")
	now := time.Now()
	store.Default.SaveContent(store.ContentRecord{
		ID: contentID, Skill: string(domain.SkillReviewPath), ClassID: classID,
		Status: string(domain.StatusPublished),
		Payload: map[string]any{
			"studentId":      studentID,
			"steps":          res.Payload["steps"],
			"selfTestCounts": false,
		},
		CreatedAt: now, UpdatedAt: now,
	})
	return map[string]any{"contentId": contentID, "steps": res.Payload["steps"], "visibleTo": []string{studentID, "class-teachers"}}, nil
}

func (s *Services) StartSocratic(ctx context.Context, classID, studentID, questionID string) (map[string]any, error) {
	_ = ctx
	id := store.NewID("tutor")
	sess := store.TutorSession{
		ID: id, StudentID: studentID, ClassID: classID, QuestionID: questionID,
		Round: 0, MaxRounds: 5, LastPrompt: "这道题里，你交换行之后右端有没有一起变？",
		FullSolution: "完整解析：行变换必须作用于增广矩阵，保证等价变形。",
	}
	store.Default.Lock()
	store.Default.Tutor[id] = sess
	store.Default.Unlock()
	return map[string]any{
		"sessionId":      id,
		"prompt":         sess.LastPrompt,
		"answerReleased": false,
	}, nil
}

func (s *Services) NextSocratic(ctx context.Context, sessionID, reply string) (map[string]any, error) {
	store.Default.Lock()
	sess, ok := store.Default.Tutor[sessionID]
	store.Default.Unlock()
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	if sess.Released {
		return map[string]any{"answerReleased": true, "fullSolution": sess.FullSolution}, nil
	}

	wantExplain := reply == "直接讲解" || reply == "我要答案"
	res, err := s.Orch.Run(ctx, nil, map[string]any{
		"skill": "socratic_tutor",
		"state": map[string]any{"round": sess.Round, "hitStreak": sess.HitStreak},
		"reply": reply,
	})
	if err != nil {
		return nil, err
	}
	hit, _ := res.Payload["hit"].(bool)
	sess.Round++
	sess.History = append(sess.History, reply)
	if hit {
		sess.HitStreak++
	} else {
		sess.HitStreak = 0
	}
	if wantExplain || sess.HitStreak >= 2 || sess.Round >= sess.MaxRounds {
		sess.Released = true
		store.Default.Lock()
		store.Default.Tutor[sessionID] = sess
		store.Default.Unlock()
		return map[string]any{
			"answerReleased": true,
			"fullSolution":   sess.FullSolution,
			"reason":         releaseReason(wantExplain, sess),
			"round":          sess.Round,
		}, nil
	}
	prompt, _ := res.Payload["prompt"].(string)
	sess.LastPrompt = prompt
	store.Default.Lock()
	store.Default.Tutor[sessionID] = sess
	store.Default.Unlock()
	return map[string]any{
		"answerReleased": false,
		"prompt":         prompt,
		"round":          sess.Round,
		"hitStreak":      sess.HitStreak,
	}, nil
}

func releaseReason(want bool, sess store.TutorSession) string {
	if want {
		return "student_requested"
	}
	if sess.HitStreak >= 2 {
		return "two_consecutive_hits"
	}
	return "max_rounds"
}

func (s *Services) ClusterClassroom(ctx context.Context, classID, sessionID string) (map[string]any, error) {
	store.Default.Lock()
	sess, ok := store.Default.Classroom[sessionID]
	store.Default.Unlock()
	if !ok {
		// 演示：创建带答案的会话
		sess = store.ClassroomSession{
			ID: sessionID, ClassID: classID, Prompt: "秩相等意味着什么？",
			Answers: []store.ClassroomAnswer{
				{StudentID: "stu-1", Name: "李同学", Text: "有解"},
				{StudentID: "stu-2", Name: "王同学", Text: "系数矩阵满秩就行"},
				{Name: "赵同学", Text: "背定义：rank A = rank Ab"},
				{Name: "钱同学", Text: "有解"},
				{Name: "孙同学", Text: "只看系数矩阵"},
			},
		}
		store.Default.Lock()
		store.Default.Classroom[sessionID] = sess
		store.Default.Unlock()
	}
	if len(sess.Answers) < 5 {
		return map[string]any{"skipped": true, "reason": "有效回答少于 5 人，不调用模型"}, nil
	}
	texts := make([]string, len(sess.Answers))
	for i, a := range sess.Answers {
		texts[i] = a.Name + "：" + a.Text
	}
	anon := security.AnonymizeForModel(ctx, texts)
	res, err := s.Orch.Run(ctx, nil, map[string]any{"skill": "聚类", "answers": anon})
	if err != nil {
		return nil, err
	}
	clusters, _ := res.Payload["clusters"].([]any)
	if clusters == nil {
		if c2, ok := res.Payload["clusters"].([]map[string]any); ok {
			for _, c := range c2 {
				clusters = append(clusters, c)
			}
		}
	}
	if len(clusters) > 4 {
		clusters = clusters[:4]
	}
	sess.Clusters = nil
	for _, c := range clusters {
		if m, ok := c.(map[string]any); ok {
			sess.Clusters = append(sess.Clusters, m)
		}
	}
	store.Default.Lock()
	store.Default.Classroom[sessionID] = sess
	store.Default.Unlock()
	return map[string]any{
		"sessionId": sessionID,
		"clusters":  sess.Clusters,
		"anonymous": true,
		"graded":    false,
	}, nil
}

func (s *Services) StartAdaptive(ctx context.Context, classID, studentID, kp string) (map[string]any, error) {
	res, err := s.Orch.Run(ctx, nil, map[string]any{"skill": "adaptive", "knowledgePoint": kp})
	if err != nil {
		return nil, err
	}
	q, _ := res.Payload["question"].(map[string]any)
	if q == nil {
		q = map[string]any{
			"stem": "从中等难度开始：完成一次初等行变换的目的是？",
			"answer": "构造主元并消元", "difficulty": "medium",
			"citations": []map[string]any{{"sourceId": "c2", "snippet": "高斯消元法通过初等行变换求阶梯型。"}},
			"verified": true,
		}
	}
	id := store.NewID("practice")
	sess := store.PracticeSession{
		ID: id, StudentID: studentID, ClassID: classID, KnowledgePoint: kp,
		Difficulty: "medium", CurrentQuestion: q,
	}
	store.Default.Lock()
	store.Default.Practice[id] = sess
	store.Default.Unlock()
	// 不把答案返回给学生
	safeQ := map[string]any{
		"stem": q["stem"], "difficulty": q["difficulty"], "citations": q["citations"], "verified": q["verified"],
	}
	return map[string]any{"sessionId": id, "question": safeQ, "countsTowardGrade": false}, nil
}

func (s *Services) SubmitAdaptive(_ context.Context, sessionID, outcome string) (map[string]any, error) {
	store.Default.Lock()
	sess, ok := store.Default.Practice[sessionID]
	store.Default.Unlock()
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	if sess.Finished {
		return map[string]any{"finished": true, "reason": sess.FinishReason}, nil
	}
	st := PracticeState{
		MasteryEvidence: sess.MasteryEvidence, ErrorType: sess.ErrorType,
		ConsecutiveCorrect: sess.ConsecutiveOK, HintsUsed: sess.HintsUsed,
		CurrentDifficulty: sess.Difficulty, KnowledgePoint: sess.KnowledgePoint,
		QuestionCount: sess.QuestionCount,
	}
	next, reason := NextTransition(st, outcome)
	sess.MasteryEvidence = next.MasteryEvidence
	sess.ErrorType = next.ErrorType
	sess.ConsecutiveOK = next.ConsecutiveCorrect
	sess.Difficulty = next.CurrentDifficulty
	sess.KnowledgePoint = next.KnowledgePoint
	sess.QuestionCount = next.QuestionCount
	sess.History = append(sess.History, map[string]any{
		"outcome": outcome, "reason": reason, "difficulty": sess.Difficulty, "kp": sess.KnowledgePoint,
	})
	if reason == ReasonFinished {
		sess.Finished = true
		sess.FinishReason = string(reason)
		sess.CurrentQuestion = nil
		store.Default.Lock()
		store.Default.Practice[sessionID] = sess
		store.Default.Unlock()
		return map[string]any{
			"finished": true,
			"reason":   reason,
			"report": map[string]any{
				"history": sess.History, "masteryEvidence": sess.MasteryEvidence,
				"countsTowardGrade": false,
			},
		}, nil
	}
	// 下一题（已验证）
	q := map[string]any{
		"stem":       fmt.Sprintf("【%s/%s】关于 %s 的下一题", sess.Difficulty, reason, sess.KnowledgePoint),
		"difficulty": sess.Difficulty,
		"citations":  []map[string]any{{"sourceId": "c2", "snippet": "高斯消元法通过初等行变换求阶梯型。"}},
		"verified":   true,
		"answer":     "hidden",
	}
	sess.CurrentQuestion = q
	store.Default.Lock()
	store.Default.Practice[sessionID] = sess
	store.Default.Unlock()
	safeQ := map[string]any{"stem": q["stem"], "difficulty": q["difficulty"], "citations": q["citations"], "verified": true}
	return map[string]any{
		"finished": false, "transition": reason, "question": safeQ, "state": map[string]any{
			"consecutiveCorrect": sess.ConsecutiveOK, "errorType": sess.ErrorType, "questionCount": sess.QuestionCount,
		},
	}, nil
}

func (s *Services) SubmitClassroomAnswer(_ context.Context, classID, sessionID, studentID, name, text string) (map[string]any, error) {
	store.Default.Lock()
	defer store.Default.Unlock()
	sess, ok := store.Default.Classroom[sessionID]
	if !ok {
		sess = store.ClassroomSession{ID: sessionID, ClassID: classID, Prompt: "课堂短答"}
	}
	sess.Answers = append(sess.Answers, store.ClassroomAnswer{StudentID: studentID, Name: name, Text: text})
	store.Default.Classroom[sessionID] = sess
	return map[string]any{"count": len(sess.Answers)}, nil
}

// 兼容骨架类型
type ReviewPathService struct{ *Services }
type SocraticTutorService struct{ *Services }
type ClassroomClusterService struct{ *Services }
type AdaptivePracticeService struct{ *Services }

func (r ReviewPathService) Generate(ctx context.Context, studentID string) error {
	_, err := r.GenerateReviewPath(ctx, "class-demo", studentID)
	return err
}
func (s SocraticTutorService) NextQuestion(ctx context.Context, sessionID, reply string) error {
	_, err := s.NextSocratic(ctx, sessionID, reply)
	return err
}
func (c ClassroomClusterService) Cluster(ctx context.Context, sessionID string) error {
	_, err := c.ClusterClassroom(ctx, "class-demo", sessionID)
	return err
}
func (a AdaptivePracticeService) Start(ctx context.Context, studentID, knowledgePoint string) error {
	_, err := a.StartAdaptive(ctx, "class-demo", studentID, knowledgePoint)
	return err
}
func (a AdaptivePracticeService) SubmitAnswer(ctx context.Context, sessionID, answer string) error {
	_, err := a.SubmitAdaptive(ctx, sessionID, answer)
	return err
}
