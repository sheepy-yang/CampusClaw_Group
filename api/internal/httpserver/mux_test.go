package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/health"
	"github.com/sheepy-yang/CampusClaw_Group/api/internal/httpapi"
)

type fakeChecker struct {
	name string
	err  error
}

func (f fakeChecker) Name() string { return f.name }
func (f fakeChecker) Check(context.Context) error {
	return f.err
}

func newTestHandler(logs *bytes.Buffer, checkers ...health.Checker) http.Handler {
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	return NewMux(Dependencies{
		Logger:        logger,
		HealthTimeout: time.Second,
		HealthChecks:  checkers,
	})
}

func TestHealthAndGeneratedRequestID(t *testing.T) {
	var logs bytes.Buffer
	handler := newTestHandler(&logs, fakeChecker{name: "mysql"})
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	requestID := response.Header().Get("X-Request-ID")
	if !requestIDPattern.MatchString(requestID) {
		t.Fatalf("invalid generated request id %q", requestID)
	}
	if !strings.Contains(logs.String(), `"request_id":"`+requestID+`"`) {
		t.Fatalf("log does not contain request id: %s", logs.String())
	}
}

func TestHealthReturnsServiceUnavailable(t *testing.T) {
	var logs bytes.Buffer
	handler := newTestHandler(
		&logs,
		fakeChecker{name: "mysql"},
		fakeChecker{name: "redis", err: errors.New("unavailable")},
	)
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	var report health.Report
	if err := json.NewDecoder(response.Body).Decode(&report); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if report.Status != health.StatusDown || report.Checks["redis"].Status != health.StatusDown {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestRequestIDPropagationAndSanitization(t *testing.T) {
	var logs bytes.Buffer
	handler := newTestHandler(&logs)

	validRequest := httptest.NewRequest(http.MethodGet, "/missing", nil)
	validRequest.Header.Set("X-Request-ID", "lesson-07.test_1")
	validResponse := httptest.NewRecorder()
	handler.ServeHTTP(validResponse, validRequest)
	if got := validResponse.Header().Get("X-Request-ID"); got != "lesson-07.test_1" {
		t.Fatalf("valid request id = %q", got)
	}

	invalidRequest := httptest.NewRequest(http.MethodGet, "/missing", nil)
	invalidRequest.Header.Set("X-Request-ID", strings.Repeat("x", 129))
	invalidResponse := httptest.NewRecorder()
	handler.ServeHTTP(invalidResponse, invalidRequest)
	if got := invalidResponse.Header().Get("X-Request-ID"); got == strings.Repeat("x", 129) || !requestIDPattern.MatchString(got) {
		t.Fatalf("unsafe request id was not replaced: %q", got)
	}
}

func TestDeclaredBusinessRouteReturnsStructuredNotImplemented(t *testing.T) {
	var logs bytes.Buffer
	handler := newTestHandler(&logs)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/skills/proposition", nil)
	request.Header.Set("X-Request-ID", "business-placeholder")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotImplemented)
	}
	var apiErr httpapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&apiErr); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if apiErr.Code != "NOT_IMPLEMENTED" || apiErr.RequestID != "business-placeholder" {
		t.Fatalf("unexpected error response: %#v", apiErr)
	}
}
