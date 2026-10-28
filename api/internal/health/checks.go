package health

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	StatusUp   = "up"
	StatusDown = "down"
)

// Checker verifies one external dependency without exposing its configuration.
type Checker interface {
	Name() string
	Check(ctx context.Context) error
}

type Result struct {
	Status string `json:"status"`
}

type Report struct {
	Status string            `json:"status"`
	Checks map[string]Result `json:"checks"`
}

type TCPChecker struct {
	name    string
	address string
}

func NewTCPChecker(name, address string) TCPChecker {
	return TCPChecker{name: name, address: address}
}

func (c TCPChecker) Name() string { return c.name }

func (c TCPChecker) Check(ctx context.Context) error {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", c.address)
	if err != nil {
		return err
	}
	return conn.Close()
}

type HTTPChecker struct {
	name   string
	url    string
	client *http.Client
}

func NewHTTPChecker(name, url string) HTTPChecker {
	return HTTPChecker{
		name: name,
		url:  url,
		client: &http.Client{
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (c HTTPChecker) Name() string { return c.name }

func (c HTTPChecker) Check(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected dependency status %d", resp.StatusCode)
	}
	return nil
}

// Run executes all checks concurrently under one bounded timeout.
func Run(parent context.Context, timeout time.Duration, checkers []Checker) Report {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	report := Report{
		Status: StatusUp,
		Checks: make(map[string]Result, len(checkers)),
	}

	type outcome struct {
		name string
		err  error
	}
	outcomes := make(chan outcome, len(checkers))
	var wg sync.WaitGroup
	for _, checker := range checkers {
		checker := checker
		wg.Add(1)
		go func() {
			defer wg.Done()
			outcomes <- outcome{name: checker.Name(), err: checker.Check(ctx)}
		}()
	}
	go func() {
		wg.Wait()
		close(outcomes)
	}()

	for item := range outcomes {
		status := StatusUp
		if item.err != nil {
			status = StatusDown
			report.Status = StatusDown
		}
		report.Checks[item.name] = Result{Status: status}
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		report.Status = StatusDown
	}
	return report
}
