package store

import (
	"context"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestRedisPersistenceSurvivesMemoryReplacement(t *testing.T) {
	rawURL := os.Getenv("TEST_REDIS_URL")
	if rawURL == "" {
		t.Skip("TEST_REDIS_URL is not set")
	}
	options, err := redis.ParseURL(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(options)
	t.Cleanup(func() {
		_ = client.FlushDB(context.Background()).Err()
		_ = client.Close()
	})
	if err := client.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}

	first := New()
	first.persistence = &RedisPersistence{client: client}
	assignment := Assignment{ID: "asg-durable", ClassID: "class-demo", Title: "durable", Status: "published"}
	first.SaveAssignment(assignment)
	task := TaskRecord{ID: "task-durable", ClassID: "class-demo", UserID: "stu-1", ObjectID: "sub-durable", Status: "pending"}
	first.SaveTask(task)
	first.SaveSubmission(Submission{
		ID: "sub-durable", AssignmentID: assignment.ID, StudentID: "stu-1", ClassID: "class-demo", TaskID: task.ID, Status: "submitted",
	})

	second := New()
	second.persistence = &RedisPersistence{client: client}
	second.Tasks = map[string]TaskRecord{}
	second.Submissions = map[string]Submission{}
	second.Assignments = map[string]Assignment{}
	if restored, ok := second.GetTask(task.ID); !ok || restored.ObjectID != "sub-durable" {
		t.Fatalf("task was not restored: %#v ok=%v", restored, ok)
	}
	if restored, ok := second.GetSubmission("sub-durable"); !ok || restored.TaskID != task.ID {
		t.Fatalf("submission was not restored: %#v ok=%v", restored, ok)
	}
	if restored, ok := second.GetAssignment(assignment.ID); !ok || restored.Status != "published" {
		t.Fatalf("assignment was not restored: %#v ok=%v", restored, ok)
	}
}

func TestRedisIdempotencyIsSharedAcrossProcesses(t *testing.T) {
	rawURL := os.Getenv("TEST_REDIS_URL")
	if rawURL == "" {
		t.Skip("TEST_REDIS_URL is not set")
	}
	options, err := redis.ParseURL(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(options)
	t.Cleanup(func() {
		_ = client.FlushDB(context.Background()).Err()
		_ = client.Close()
	})
	if err := client.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	first := New()
	first.persistence = &RedisPersistence{client: client}
	second := New()
	second.persistence = &RedisPersistence{client: client}

	created, duplicate, err := first.ClaimIdempotency("stu-1", "assignment:a", "same-key", "hash-a", "task-one")
	if err != nil || duplicate {
		t.Fatalf("first claim: task=%#v duplicate=%v err=%v", created, duplicate, err)
	}
	repeated, duplicate, err := second.ClaimIdempotency("stu-1", "assignment:a", "same-key", "hash-a", "task-two")
	if err != nil || !duplicate || repeated.ID != created.ID {
		t.Fatalf("shared repeat: task=%#v duplicate=%v err=%v", repeated, duplicate, err)
	}
	if _, _, err := second.ClaimIdempotency("stu-1", "assignment:a", "same-key", "hash-b", "task-three"); err != ErrConflict {
		t.Fatalf("shared conflict error = %v", err)
	}
}
