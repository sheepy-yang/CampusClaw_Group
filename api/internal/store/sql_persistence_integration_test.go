package store

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func TestMySQLPersistenceRoundTripAndIdempotency(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN is not set")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	persistence := &SQLPersistence{db: db}
	if err := persistence.ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, table := range []string{"app_document_indexes", "app_documents", "app_idempotency", "app_completed_jobs"} {
			_, _ = db.Exec("DELETE FROM " + table)
		}
		_ = db.Close()
	})
	for _, table := range []string{"app_document_indexes", "app_documents", "app_idempotency", "app_completed_jobs"} {
		if _, err := db.Exec("DELETE FROM " + table); err != nil {
			t.Fatal(err)
		}
	}

	first := New()
	first.persistence = persistence
	first.SaveMaterial(Material{ID: "mat-sql", ClassID: "class-demo", Title: "SQL", Version: "v1", ContentHash: "hash"})
	first.SaveTask(TaskRecord{ID: "task-sql", ClassID: "class-demo", UserID: "stu-1", Status: "pending"})
	first.AppendEvent(LearningEvent{
		ID: "event-sql", StudentID: "stu-1", ClassID: "class-demo",
		Kind: "assignment", Summary: "SQL 持久化事件", KP: "持久化",
		Outcome: LearningOutcomeCorrect, At: time.Now().UTC(),
	})

	second := New()
	second.persistence = persistence
	second.Materials = map[string]Material{}
	second.Tasks = map[string]TaskRecord{}
	second.Events = nil
	if material, ok := second.GetMaterial("mat-sql"); !ok || material.Title != "SQL" {
		t.Fatalf("material round trip failed: %#v ok=%v", material, ok)
	}
	if task, ok := second.GetTask("task-sql"); !ok || task.UserID != "stu-1" {
		t.Fatalf("task round trip failed: %#v ok=%v", task, ok)
	}
	events := second.EventsFor("stu-1", "class-demo")
	if len(events) != 1 || events[0].ID != "event-sql" || events[0].KP != "持久化" {
		t.Fatalf("learning event round trip failed: %#v", events)
	}

	created, duplicate, err := first.ClaimIdempotency("stu-1", "submit", "sql-key", "hash-a", "sql-task-one")
	if err != nil || duplicate {
		t.Fatalf("first idempotency claim: %#v duplicate=%v err=%v", created, duplicate, err)
	}
	repeated, duplicate, err := second.ClaimIdempotency("stu-1", "submit", "sql-key", "hash-a", "sql-task-two")
	if err != nil || !duplicate || repeated.ID != created.ID {
		t.Fatalf("shared idempotency claim: %#v duplicate=%v err=%v", repeated, duplicate, err)
	}
}
