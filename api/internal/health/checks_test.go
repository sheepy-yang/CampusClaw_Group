package health

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeChecker struct {
	name string
	err  error
}

func (f fakeChecker) Name() string { return f.name }
func (f fakeChecker) Check(context.Context) error {
	return f.err
}

func TestRunAllDependenciesUp(t *testing.T) {
	report := Run(context.Background(), time.Second, []Checker{
		fakeChecker{name: "mysql"},
		fakeChecker{name: "redis"},
	})

	if report.Status != StatusUp {
		t.Fatalf("status = %q, want %q", report.Status, StatusUp)
	}
	for name, result := range report.Checks {
		if result.Status != StatusUp {
			t.Errorf("%s status = %q, want %q", name, result.Status, StatusUp)
		}
	}
}

func TestRunReportsFailedDependency(t *testing.T) {
	report := Run(context.Background(), time.Second, []Checker{
		fakeChecker{name: "mysql"},
		fakeChecker{name: "redis", err: errors.New("connection refused")},
	})

	if report.Status != StatusDown {
		t.Fatalf("status = %q, want %q", report.Status, StatusDown)
	}
	if report.Checks["mysql"].Status != StatusUp {
		t.Errorf("mysql status = %q", report.Checks["mysql"].Status)
	}
	if report.Checks["redis"].Status != StatusDown {
		t.Errorf("redis status = %q", report.Checks["redis"].Status)
	}
}
