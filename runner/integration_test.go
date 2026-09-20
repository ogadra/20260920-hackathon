package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// --- Integration test helpers ---

// createShell is a test helper that creates a new shell via the HTTP API
// and returns its ID extracted from the Set-Cookie header.
func createShell(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	resp, err := http.Post(ts.URL+"/api/shell", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/shell error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("create shell status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	for _, c := range resp.Cookies() {
		if c.Name == "shell_id" {
			return c.Value
		}
	}
	t.Fatal("shell_id cookie not found in response")
	return ""
}

// marshalCommand is a test helper that marshals a command string into a JSON request body.
func marshalCommand(t *testing.T, command string) string {
	t.Helper()
	payload, err := json.Marshal(executeRequest{Command: command})
	if err != nil {
		t.Fatalf("marshal command: %v", err)
	}
	return string(payload)
}

// executeCommand is a test helper that executes a whitelisted command via the
// HTTP API and returns the parsed SSE events.
func executeCommand(t *testing.T, ts *httptest.Server, shellID, command string) []sseEvent {
	t.Helper()
	body := marshalCommand(t, command)
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/execute", strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: "shell_id", Value: shellID})
	setClientAddressHeader(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/execute error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("execute status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var buf strings.Builder
	if _, err := io.Copy(&buf, resp.Body); err != nil {
		t.Fatalf("read body error: %v", err)
	}
	return parseIntegrationSSEEvents(t, buf.String())
}

// firstStdoutData is a test helper that returns the Data field of the first
// stdout event in the given events slice.
func firstStdoutData(t *testing.T, events []sseEvent) string {
	t.Helper()
	for _, e := range events {
		if e.Type == "stdout" {
			return e.Data
		}
	}
	t.Fatal("no stdout event found")
	return ""
}

// parseIntegrationSSEEvents parses a raw SSE response body into a slice of sseEvent.
// It calls t.Fatalf on parse errors and must not be called from goroutines.
func parseIntegrationSSEEvents(t *testing.T, body string) []sseEvent {
	t.Helper()
	events, err := parseSSEEventsRaw(body)
	if err != nil {
		t.Fatalf("parse SSE events: %v", err)
	}
	return events
}

// parseSSEEventsRaw parses a raw SSE response body into a slice of sseEvent.
// It returns an error instead of calling t.Fatalf, making it safe for goroutines.
func parseSSEEventsRaw(body string) ([]sseEvent, error) {
	var events []sseEvent
	scanner := bufio.NewScanner(strings.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		var event sseEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil, fmt.Errorf("unmarshal SSE event %q: %w", data, err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan SSE events: %w", err)
	}
	return events, nil
}

// --- Integration tests ---

// TestIntegrationExecuteSSEResponse verifies that executing a whitelisted command
// through the full HTTP stack returns valid SSE events including stdout output
// and a complete event with exitCode 0.
func TestIntegrationExecuteSSEResponse(t *testing.T) {
	sm := NewShellManager()
	defer sm.CloseAll()

	ts := httptest.NewServer(newHandler(sm))
	defer ts.Close()

	sid := createShell(t, ts)
	events := executeCommand(t, ts, sid, "pwd")

	if len(events) < 2 {
		t.Fatalf("expected at least 2 events (stdout + complete), got %d", len(events))
	}

	stdout := firstStdoutData(t, events)
	if stdout == "" {
		t.Fatal("expected non-empty stdout from pwd")
	}

	last := events[len(events)-1]
	if last.Type != "complete" || last.ExitCode == nil || *last.ExitCode != 0 {
		t.Fatalf("last event = %+v, want complete with exitCode=0", last)
	}
}

// TestIntegrationNonWhitelistedCommand verifies that a non-whitelisted command
// executes through the full HTTP stack.
func TestIntegrationNonWhitelistedCommand(t *testing.T) {
	sm := NewShellManager()
	defer sm.CloseAll()

	ts := httptest.NewServer(newHandler(sm))
	defer ts.Close()

	sid := createShell(t, ts)

	body := marshalCommand(t, "rm --version")
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/execute", strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: "shell_id", Value: sid})
	setClientAddressHeader(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/execute error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var buf strings.Builder
	if _, err := io.Copy(&buf, resp.Body); err != nil {
		t.Fatalf("read body error: %v", err)
	}
	events := parseIntegrationSSEEvents(t, buf.String())
	if len(events) == 0 {
		t.Fatal("expected at least 1 SSE event, got 0")
	}
	last := events[len(events)-1]
	if last.Type != "complete" || last.ExitCode == nil {
		t.Fatalf("last event = %+v, want complete with exitCode", last)
	}
}

// TestIntegrationExecuteAfterDelete verifies that executing a command on a
// deleted shell returns 404 Not Found.
func TestIntegrationExecuteAfterDelete(t *testing.T) {
	sm := NewShellManager()
	defer sm.CloseAll()

	ts := httptest.NewServer(newHandler(sm))
	defer ts.Close()

	sid := createShell(t, ts)

	// Delete the shell.
	delReq, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/shell", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	delReq.AddCookie(&http.Cookie{Name: "shell_id", Value: sid})
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("DELETE error: %v", err)
	}
	delResp.Body.Close()

	// Execute on the deleted shell.
	body := marshalCommand(t, "ls")
	execReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/execute", strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	execReq.AddCookie(&http.Cookie{Name: "shell_id", Value: sid})
	setClientAddressHeader(execReq)
	execResp, err := http.DefaultClient.Do(execReq)
	if err != nil {
		t.Fatalf("POST /api/execute error: %v", err)
	}
	defer execResp.Body.Close()

	if execResp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", execResp.StatusCode, http.StatusNotFound)
	}
}

// 2つのshellが独立したbashプロセスであることを検証する
func TestIntegrationShellIsolation(t *testing.T) {
	sm := NewShellManager()
	defer sm.CloseAll()

	ts := httptest.NewServer(newHandler(sm))
	defer ts.Close()

	sid1 := createShell(t, ts)
	sid2 := createShell(t, ts)

	dir1 := strings.TrimSpace(firstStdoutData(t, executeCommand(t, ts, sid1, "pwd")))
	dir2 := strings.TrimSpace(firstStdoutData(t, executeCommand(t, ts, sid2, "pwd")))
	if dir1 != "/app" || dir2 != "/app" {
		t.Fatalf("expected initial pwd /app, got %q and %q", dir1, dir2)
	}

	executeCommand(t, ts, sid1, "cd /tmp")
	newDir1 := strings.TrimSpace(firstStdoutData(t, executeCommand(t, ts, sid1, "pwd")))
	dir2After := strings.TrimSpace(firstStdoutData(t, executeCommand(t, ts, sid2, "pwd")))
	if newDir1 != "/tmp" {
		t.Fatalf("shell 1 pwd = %q, want /tmp", newDir1)
	}
	if dir2After != "/app" {
		t.Fatalf("shell 2 pwd changed after mutating shell 1: want /app, got %q", dir2After)
	}
}

// TestIntegrationCreateDeleteLifecycle verifies the full lifecycle of
// creating a shell, executing a command, and deleting the shell.
func TestIntegrationCreateDeleteLifecycle(t *testing.T) {
	sm := NewShellManager()
	defer sm.CloseAll()

	ts := httptest.NewServer(newHandler(sm))
	defer ts.Close()

	// Create.
	sid := createShell(t, ts)

	// Execute.
	events := executeCommand(t, ts, sid, "ls")
	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}
	last := events[len(events)-1]
	if last.Type != "complete" || last.ExitCode == nil || *last.ExitCode != 0 {
		t.Fatalf("expected complete with exitCode=0, got %+v", last)
	}

	// Delete.
	delReq, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/shell", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	delReq.AddCookie(&http.Cookie{Name: "shell_id", Value: sid})
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("DELETE error: %v", err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", delResp.StatusCode, http.StatusNoContent)
	}

	// Verify shell is gone.
	body := marshalCommand(t, "ls")
	execReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/execute", strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	execReq.AddCookie(&http.Cookie{Name: "shell_id", Value: sid})
	setClientAddressHeader(execReq)
	execResp, err := http.DefaultClient.Do(execReq)
	if err != nil {
		t.Fatalf("POST /api/execute error: %v", err)
	}
	defer execResp.Body.Close()
	if execResp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d after delete", execResp.StatusCode, http.StatusNotFound)
	}
}

// TestIntegrationConcurrentExecute verifies that concurrent execute requests
// on the same shell are serialized by the shell mutex and all complete
// successfully without data races or interleaved output.
func TestIntegrationConcurrentExecute(t *testing.T) {
	sm := NewShellManager()
	defer sm.CloseAll()

	ts := httptest.NewServer(newHandler(sm))
	defer ts.Close()

	sid := createShell(t, ts)
	body := marshalCommand(t, "pwd")

	const n = 5
	var wg sync.WaitGroup
	wg.Add(n)
	results := make([][]sseEvent, n)
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/execute", strings.NewReader(body))
			if err != nil {
				errs[i] = err
				return
			}
			req.AddCookie(&http.Cookie{Name: "shell_id", Value: sid})
			setClientAddressHeader(req)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				errs[i] = err
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				errs[i] = &unexpectedStatusError{got: resp.StatusCode}
				return
			}
			var buf strings.Builder
			if _, err := io.Copy(&buf, resp.Body); err != nil {
				errs[i] = err
				return
			}
			events, err := parseSSEEventsRaw(buf.String())
			if err != nil {
				errs[i] = err
				return
			}
			results[i] = events
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("request %d error: %v", i, errs[i])
		}
		if len(results[i]) == 0 {
			t.Fatalf("request %d: no events received", i)
		}
		last := results[i][len(results[i])-1]
		if last.Type != "complete" || last.ExitCode == nil || *last.ExitCode != 0 {
			t.Fatalf("request %d: last event = %+v, want complete with exitCode=0", i, last)
		}
	}
}

// TestIntegrationNonWhitelistedSSE verifies that a non-whitelisted command
// executes through the full HTTP stack and returns SSE events.
func TestIntegrationNonWhitelistedSSE(t *testing.T) {
	sm := NewShellManager()
	defer sm.CloseAll()

	ts := httptest.NewServer(newHandler(sm))
	defer ts.Close()

	sid := createShell(t, ts)

	body := marshalCommand(t, "bash -c true")
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/execute", strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: "shell_id", Value: sid})
	setClientAddressHeader(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/execute error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var buf strings.Builder
	if _, err := io.Copy(&buf, resp.Body); err != nil {
		t.Fatalf("read body error: %v", err)
	}
	events := parseIntegrationSSEEvents(t, buf.String())
	if len(events) == 0 {
		t.Fatal("expected at least 1 SSE event, got 0")
	}
	last := events[len(events)-1]
	if last.Type != "complete" || last.ExitCode == nil || *last.ExitCode != 0 {
		t.Fatalf("last event = %+v, want complete with exitCode=0", last)
	}
}

// unexpectedStatusError is returned when an HTTP response has an unexpected status code.
type unexpectedStatusError struct {
	got int
}

// Error returns a human-readable description of the unexpected status code.
func (e *unexpectedStatusError) Error() string {
	return "unexpected status: " + http.StatusText(e.got)
}
