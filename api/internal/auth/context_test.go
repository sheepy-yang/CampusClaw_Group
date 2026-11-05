package auth

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestSignedSessionRejectsTamperingAndLogoutRevokes(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("SESSION_SECRET", "test-session-secret")
	token, principal, err := LoginWithPassword("teacher", "demo123")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Authenticate(context.Background(), "Bearer "+token)
	if err != nil || got != principal {
		t.Fatalf("authenticate principal=%#v err=%v", got, err)
	}
	if _, err := Authenticate(context.Background(), "Bearer "+token+"x"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("tampered token error = %v", err)
	}
	Logout("Bearer " + token)
	if _, err := Authenticate(context.Background(), "Bearer "+token); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked token error = %v", err)
	}
}

func TestProductionDisablesBuiltInDemoCredentials(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("AUTH_USERS_JSON", "")
	if _, _, err := LoginWithPassword("teacher", "demo123"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("production demo login error = %v", err)
	}
}

func TestProductionUsesConfiguredPasswordHashes(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("SESSION_SECRET", "production-test-secret")
	hash, err := bcrypt.GenerateFromPassword([]byte("strong-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal([]configuredCredential{{
		Username: "teacher-a", PasswordHash: string(hash), UserID: "teacher-a",
		ClassID: "class-a", Role: RoleTeacher, Name: "教师 A",
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTH_USERS_JSON", string(encoded))
	token, principal, err := LoginWithPassword("teacher-a", "strong-password")
	if err != nil || token == "" || principal.ClassID != "class-a" {
		t.Fatalf("token=%q principal=%#v err=%v", token, principal, err)
	}
}
