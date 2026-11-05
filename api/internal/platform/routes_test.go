package platform

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/auth"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/store"
)

func newPlatformTestHandler(t *testing.T) http.Handler {
	t.Helper()
	store.Default = store.New()
	mux := http.NewServeMux()
	RegisterRoutes(mux)
	return mux
}

func platformRequest(t *testing.T, handler http.Handler, method, path, token string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, &payload)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		username := map[string]string{
			"teacher-demo": "teacher", "student-demo": "student", "student-demo-2": "student2",
			"teacher-other": "teacher_other", "student-other": "student_other",
		}[token]
		if username != "" {
			var err error
			token, _, err = auth.LoginWithPassword(username, "demo123")
			if err != nil {
				t.Fatalf("issue test session: %v", err)
			}
		}
		request.Header.Set("Authorization", "Bearer "+token)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeMap(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		t.Fatalf("decode response: %v body=%s", err, response.Body.String())
	}
	return value
}

func TestLoginRequiresCredentialsAndLogoutRevokesSession(t *testing.T) {
	handler := newPlatformTestHandler(t)
	roleOnly := platformRequest(t, handler, http.MethodPost, "/api/v1/auth/login", "", map[string]any{"role": "teacher"}, nil)
	if roleOnly.Code != http.StatusBadRequest {
		t.Fatalf("role-only login status = %d body=%s", roleOnly.Code, roleOnly.Body.String())
	}
	login := platformRequest(t, handler, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": "teacher", "password": "demo123",
	}, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", login.Code, login.Body.String())
	}
	token, _ := decodeMap(t, login)["token"].(string)
	if len(token) < 32 || token == "teacher-demo" {
		t.Fatalf("unsafe session token %q", token)
	}
	me := platformRequest(t, handler, http.MethodGet, "/api/v1/auth/me", token, nil, nil)
	if me.Code != http.StatusOK {
		t.Fatalf("session status = %d", me.Code)
	}
	logout := platformRequest(t, handler, http.MethodPost, "/api/v1/auth/logout", token, nil, nil)
	if logout.Code != http.StatusOK {
		t.Fatalf("logout status = %d", logout.Code)
	}
	revoked := platformRequest(t, handler, http.MethodGet, "/api/v1/auth/me", token, nil, nil)
	if revoked.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session status = %d", revoked.Code)
	}
}

func TestMaterialIngestionRetrievalAndClassIsolation(t *testing.T) {
	handler := newPlatformTestHandler(t)

	unauthorized := platformRequest(t, handler, http.MethodGet, "/api/v1/classes/class-demo/materials", "", nil, nil)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}
	forbidden := platformRequest(t, handler, http.MethodPost, "/api/v1/classes/class-demo/materials", "student-demo", map[string]any{
		"title": "测试", "content": "测试内容",
	}, nil)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("student upload status = %d", forbidden.Code)
	}

	created := platformRequest(t, handler, http.MethodPost, "/api/v1/classes/class-demo/materials", "teacher-demo", map[string]any{
		"title": "数据结构", "content": "栈是后进先出的线性表。队列是先进先出的线性表。",
	}, nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", created.Code, created.Body.String())
	}
	material := decodeMap(t, created)
	if material["contentHash"] == "" {
		t.Fatalf("created material lacks hash: %#v", material)
	}

	search := platformRequest(t, handler, http.MethodPost, "/api/v1/classes/class-demo/search", "student-demo", map[string]any{
		"query": "栈是什么", "limit": 5,
	}, nil)
	if search.Code != http.StatusOK {
		t.Fatalf("search status = %d body=%s", search.Code, search.Body.String())
	}
	result := decodeMap(t, search)
	hits, ok := result["hits"].([]any)
	if !ok || len(hits) == 0 {
		t.Fatalf("expected traceable hits: %#v", result)
	}
	hit := hits[0].(map[string]any)
	for _, field := range []string{"materialId", "materialVersion", "contentHash", "text"} {
		if hit[field] == nil || hit[field] == "" {
			t.Fatalf("hit lacks %s: %#v", field, hit)
		}
	}

	crossClass := platformRequest(t, handler, http.MethodPost, "/api/v1/classes/class-demo/search", "student-other", map[string]any{
		"query": "栈是什么",
	}, nil)
	if crossClass.Code != http.StatusNotFound {
		t.Fatalf("cross-class search status = %d", crossClass.Code)
	}
	otherSearch := platformRequest(t, handler, http.MethodPost, "/api/v1/classes/class-other/search", "student-other", map[string]any{
		"query": "栈是什么",
	}, nil)
	if otherSearch.Code != http.StatusOK {
		t.Fatalf("other search status = %d", otherSearch.Code)
	}
	otherResult := decodeMap(t, otherSearch)
	if otherHits, _ := otherResult["hits"].([]any); len(otherHits) != 0 {
		t.Fatalf("cross-class chunk leaked: %#v", otherHits)
	}
}

func TestGroundedAssistantStreamsAndDoesNotFabricateCitation(t *testing.T) {
	handler := newPlatformTestHandler(t)
	created := platformRequest(t, handler, http.MethodPost, "/api/v1/classes/class-demo/conversations", "student-demo", map[string]any{
		"title": "线代问答",
	}, nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("conversation status = %d body=%s", created.Code, created.Body.String())
	}
	conversationID := decodeMap(t, created)["id"].(string)

	stream := platformRequest(t, handler, http.MethodPost, "/api/v1/conversations/"+conversationID+"/messages", "student-demo", map[string]any{
		"content": "高斯消元法怎么做？",
	}, nil)
	if stream.Code != http.StatusOK {
		t.Fatalf("stream status = %d body=%s", stream.Code, stream.Body.String())
	}
	body := stream.Body.String()
	for _, event := range []string{"event: metadata", "event: citations", "event: delta", "event: done"} {
		if !strings.Contains(body, event) {
			t.Fatalf("stream lacks %q: %s", event, body)
		}
	}

	noHit := platformRequest(t, handler, http.MethodPost, "/api/v1/conversations/"+conversationID+"/messages", "student-demo", map[string]any{
		"content": "火星基地的氧气循环参数是多少？",
	}, nil)
	if noHit.Code != http.StatusOK {
		t.Fatalf("no-hit stream status = %d body=%s", noHit.Code, noHit.Body.String())
	}
	if strings.Contains(noHit.Body.String(), "event: citations") {
		t.Fatalf("no-hit response fabricated citations: %s", noHit.Body.String())
	}
	if !strings.Contains(noHit.Body.String(), "没有检索到") {
		t.Fatalf("no-hit response lacks grounded refusal: %s", noHit.Body.String())
	}
}

func TestAssignmentSubmissionIsIdempotentAndCreatesMistake(t *testing.T) {
	handler := newPlatformTestHandler(t)
	created := platformRequest(t, handler, http.MethodPost, "/api/v1/classes/class-demo/assignments", "teacher-demo", map[string]any{
		"title": "线代测验",
		"items": []map[string]any{{
			"id": "q1", "stem": "齐次线性方程是否至少有零解？", "type": "choice",
			"answer": "是", "knowledgePoint": "齐次线性方程组", "explanation": "零向量总是解",
		}},
	}, nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("create assignment status = %d body=%s", created.Code, created.Body.String())
	}
	assignmentID := decodeMap(t, created)["id"].(string)
	published := platformRequest(t, handler, http.MethodPost, "/api/v1/assignments/"+assignmentID+"/publish", "teacher-demo", nil, nil)
	if published.Code != http.StatusOK {
		t.Fatalf("publish status = %d body=%s", published.Code, published.Body.String())
	}

	studentView := platformRequest(t, handler, http.MethodGet, "/api/v1/assignments/"+assignmentID, "student-demo", nil, nil)
	if studentView.Code != http.StatusOK || strings.Contains(studentView.Body.String(), `"answer":"是"`) {
		t.Fatalf("student answer leakage: status=%d body=%s", studentView.Code, studentView.Body.String())
	}

	submitBody := map[string]any{"answers": map[string]string{"q1": "否"}}
	submitted := platformRequest(t, handler, http.MethodPost, "/api/v1/assignments/"+assignmentID+"/submissions", "student-demo", submitBody, map[string]string{
		"Idempotency-Key": "test-wrong-answer",
	})
	if submitted.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d body=%s", submitted.Code, submitted.Body.String())
	}
	submitResult := decodeMap(t, submitted)
	taskID := submitResult["taskId"].(string)
	submissionID := submitResult["submissionId"].(string)

	repeated := platformRequest(t, handler, http.MethodPost, "/api/v1/assignments/"+assignmentID+"/submissions", "student-demo", submitBody, map[string]string{
		"Idempotency-Key": "test-wrong-answer",
	})
	if repeated.Code != http.StatusAccepted {
		t.Fatalf("repeat status = %d", repeated.Code)
	}
	repeatResult := decodeMap(t, repeated)
	if repeatResult["taskId"] != taskID || repeatResult["submissionId"] != submissionID || repeatResult["duplicate"] != true {
		t.Fatalf("idempotent response mismatch: %#v", repeatResult)
	}
	conflict := platformRequest(t, handler, http.MethodPost, "/api/v1/assignments/"+assignmentID+"/submissions", "student-demo", map[string]any{
		"answers": map[string]string{"q1": "是"},
	}, map[string]string{"Idempotency-Key": "test-wrong-answer"})
	if conflict.Code != http.StatusConflict {
		t.Fatalf("idempotency conflict status = %d body=%s", conflict.Code, conflict.Body.String())
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		task, ok := store.Default.GetTask(taskID)
		if ok && task.Status == "succeeded" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("task did not succeed: %#v", task)
		}
		time.Sleep(10 * time.Millisecond)
	}
	submission := platformRequest(t, handler, http.MethodGet, "/api/v1/submissions/"+submissionID, "student-demo", nil, nil)
	if submission.Code != http.StatusOK {
		t.Fatalf("submission status = %d body=%s", submission.Code, submission.Body.String())
	}
	mistakes := store.Default.MistakesFor("stu-1", "class-demo")
	var submittedMistake *store.Mistake
	for index := range mistakes {
		if mistakes[index].SubmissionID == submissionID {
			submittedMistake = &mistakes[index]
			break
		}
	}
	if submittedMistake == nil || submittedMistake.KnowledgePoint != "齐次线性方程组" {
		t.Fatalf("mistake knowledge point was not preserved: %#v", mistakes)
	}
	var gradedEvent *store.LearningEvent
	for _, event := range store.Default.EventsFor("stu-1", "class-demo") {
		if strings.Contains(event.Summary, "齐次线性方程是否至少有零解") {
			current := event
			gradedEvent = &current
			break
		}
	}
	if gradedEvent == nil || gradedEvent.KP != "齐次线性方程组" || gradedEvent.Outcome != store.LearningOutcomeIncorrect {
		t.Fatalf("graded learning evidence was not preserved: %#v", gradedEvent)
	}
	mistake := platformRequest(t, handler, http.MethodGet, "/api/v1/submissions/"+submissionID+"/mistake-explanation", "student-demo", nil, nil)
	if mistake.Code != http.StatusOK || strings.Contains(mistake.Body.String(), `"standardAnswer"`) || !strings.Contains(mistake.Body.String(), `"answerRevealed":false`) {
		t.Fatalf("unsafe mistake explanation: status=%d body=%s", mistake.Code, mistake.Body.String())
	}
	otherStudent := platformRequest(t, handler, http.MethodGet, "/api/v1/submissions/"+submissionID, "student-demo-2", nil, nil)
	if otherStudent.Code != http.StatusNotFound {
		t.Fatalf("other student accessed submission: %d", otherStudent.Code)
	}
}
