package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func setClientAddressHeader(req *http.Request) {
	req.Header.Set(clientAddressHeader, "203.0.113.50:12345")
}

// TestCreateShell verifies that POST /api/shell creates a new shell
// and returns a JSON body containing a non-empty shellId.
func TestCreateShell(t *testing.T) {
	sm := NewShellManager()
	defer sm.CloseAll()
	handler := newHandler(sm)

	req := httptest.NewRequest(http.MethodPost, "/api/shell", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}

	if body := w.Body.String(); body != "" {
		t.Fatalf("expected empty body, got %q", body)
	}

	cookies := w.Result().Cookies()
	var found bool
	for _, c := range cookies {
		if c.Name == "shell_id" {
			found = true
			if c.Value == "" {
				t.Fatal("cookie value is empty")
			}
			if c.Path != "/" {
				t.Errorf("cookie Path = %q, want %q", c.Path, "/")
			}
			if !c.HttpOnly {
				t.Error("cookie HttpOnly = false, want true")
			}
			if !c.Secure {
				t.Error("cookie Secure = false, want true")
			}
			if c.SameSite != http.SameSiteStrictMode {
				t.Errorf("cookie SameSite = %v, want %v", c.SameSite, http.SameSiteStrictMode)
			}
			break
		}
	}
	if !found {
		t.Fatalf("Set-Cookie shell_id not found in response, cookies = %v", cookies)
	}
}

// TestDeleteShell verifies that DELETE /api/shell with a valid shell_id cookie
// deletes the shell and returns 204 No Content.
func TestDeleteShell(t *testing.T) {
	sm := NewShellManager()
	defer sm.CloseAll()
	handler := newHandler(sm)

	id, _, err := sm.Create()
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/shell", nil)
	req.AddCookie(&http.Cookie{Name: "shell_id", Value: id})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}

	expired := false
	for _, c := range w.Result().Cookies() {
		if c.Name == "shell_id" && c.MaxAge < 0 {
			expired = true
		}
	}
	if !expired {
		t.Errorf("expected shell_id cookie to be expired (Max-Age<0), cookies = %v", w.Result().Cookies())
	}

	_, err = sm.Get(id)
	if err == nil {
		t.Fatal("shell should be deleted")
	}
}

// TestDeleteShellMissingCookie verifies that DELETE /api/shell without
// shell_id cookie returns 400 Bad Request.
func TestDeleteShellMissingCookie(t *testing.T) {
	sm := NewShellManager()
	handler := newHandler(sm)

	req := httptest.NewRequest(http.MethodDelete, "/api/shell", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestDeleteShellNotFound verifies that DELETE /api/shell with a nonexistent
// shell ID returns 404 Not Found.
func TestDeleteShellNotFound(t *testing.T) {
	sm := NewShellManager()
	handler := newHandler(sm)

	req := httptest.NewRequest(http.MethodDelete, "/api/shell", nil)
	req.AddCookie(&http.Cookie{Name: "shell_id", Value: "nonexistent"})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// TestDeleteShellCloseError verifies that DELETE /api/shell returns 500
// when the shell exists but Close fails.
func TestDeleteShellCloseError(t *testing.T) {
	sm := NewShellManager()
	sm.newShell = func() (Shell, error) {
		return &mockShell{closeErr: errors.New("close failed")}, nil
	}
	handler := newHandler(sm)

	id, _, err := sm.Create()
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/shell", nil)
	req.AddCookie(&http.Cookie{Name: "shell_id", Value: id})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// TestExecuteWhitelisted verifies that POST /api/execute with a whitelisted command
// streams SSE events for stdout and complete with exit code 0.
func TestExecuteWhitelisted(t *testing.T) {
	sm := NewShellManager()
	defer sm.CloseAll()
	handler := newHandler(sm)

	id, _, err := sm.Create()
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	body := strings.NewReader(`{"command":"ls"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/execute", body)
	req.AddCookie(&http.Cookie{Name: "shell_id", Value: id})
	setClientAddressHeader(req)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want %q", ct, "text/event-stream")
	}

	events := parseSSEEvents(t, w.Body.String())
	if len(events) < 1 {
		t.Fatalf("expected at least 1 event, got %d", len(events))
	}

	last := events[len(events)-1]
	if last.Type != "complete" {
		t.Fatalf("last event type = %q, want %q", last.Type, "complete")
	}
	if last.ExitCode == nil || *last.ExitCode != 0 {
		t.Fatalf("last event exitCode = %v, want 0", last.ExitCode)
	}
}

// TestExecuteNonWhitelisted verifies that POST /api/execute with a
// non-whitelisted command executes it.
func TestExecuteNonWhitelisted(t *testing.T) {
	sm := NewShellManager()
	defer sm.CloseAll()
	sm.newShell = func() (Shell, error) {
		return &mockShell{exitCode: 0}, nil
	}
	handler := newHandler(sm)

	id, _, err := sm.Create()
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	body := strings.NewReader(`{"command":"rm --version"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/execute", body)
	req.AddCookie(&http.Cookie{Name: "shell_id", Value: id})
	setClientAddressHeader(req)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

// TestExecuteNonWhitelistedWithArgs verifies that a non-whitelisted command
// with arguments executes it.
func TestExecuteNonWhitelistedWithArgs(t *testing.T) {
	sm := NewShellManager()
	defer sm.CloseAll()
	sm.newShell = func() (Shell, error) {
		return &mockShell{exitCode: 0}, nil
	}
	handler := newHandler(sm)

	id, _, err := sm.Create()
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	body := strings.NewReader(`{"command":"curl https://example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/execute", body)
	req.AddCookie(&http.Cookie{Name: "shell_id", Value: id})
	setClientAddressHeader(req)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

// TestExecuteMissingShellCookie verifies that POST /api/execute without
// shell_id cookie returns 400 Bad Request.
func TestExecuteMissingShellCookie(t *testing.T) {
	sm := NewShellManager()
	handler := newHandler(sm)

	body := strings.NewReader(`{"command":"ls"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/execute", body)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestExecuteShellNotFound verifies that POST /api/execute with a nonexistent
// shell ID returns 404 Not Found.
func TestExecuteShellNotFound(t *testing.T) {
	sm := NewShellManager()
	handler := newHandler(sm)

	body := strings.NewReader(`{"command":"ls"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/execute", body)
	req.AddCookie(&http.Cookie{Name: "shell_id", Value: "nonexistent"})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// TestExecuteInvalidJSON verifies that POST /api/execute with invalid JSON body
// returns 400 Bad Request.
func TestExecuteInvalidJSON(t *testing.T) {
	sm := NewShellManager()
	defer sm.CloseAll()
	handler := newHandler(sm)

	id, _, err := sm.Create()
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	body := strings.NewReader(`{invalid`)
	req := httptest.NewRequest(http.MethodPost, "/api/execute", body)
	req.AddCookie(&http.Cookie{Name: "shell_id", Value: id})
	setClientAddressHeader(req)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestExecuteEmptyCommand verifies that POST /api/execute with an empty command
// returns 400 Bad Request.
func TestExecuteEmptyCommand(t *testing.T) {
	sm := NewShellManager()
	defer sm.CloseAll()
	handler := newHandler(sm)

	id, _, err := sm.Create()
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	body := strings.NewReader(`{"command":""}`)
	req := httptest.NewRequest(http.MethodPost, "/api/execute", body)
	req.AddCookie(&http.Cookie{Name: "shell_id", Value: id})
	setClientAddressHeader(req)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestShellMethodNotAllowed verifies that unsupported HTTP methods on
// /api/shell return 405 Method Not Allowed.
func TestShellMethodNotAllowed(t *testing.T) {
	sm := NewShellManager()
	handler := newHandler(sm)

	req := httptest.NewRequest(http.MethodGet, "/api/shell", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// TestExecuteMethodNotAllowed verifies that unsupported HTTP methods on
// /api/execute return 405 Method Not Allowed.
func TestExecuteMethodNotAllowed(t *testing.T) {
	sm := NewShellManager()
	handler := newHandler(sm)

	req := httptest.NewRequest(http.MethodGet, "/api/execute", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// TestCreateShellError verifies that POST /api/shell returns 500
// when the shell manager fails to create a new shell.
func TestCreateShellError(t *testing.T) {
	sm := NewShellManager()
	sm.newShell = func() (Shell, error) {
		return nil, errors.New("shell broken")
	}
	handler := newHandler(sm)

	req := httptest.NewRequest(http.MethodPost, "/api/shell", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// mockShell is a test double for the Shell interface that returns
// preconfigured values from ExecuteStream and Close.
type mockShell struct {
	exitCode int
	stderr   string
	err      error
	closeErr error
}

// ExecuteStream sends no stdout lines and returns the preconfigured exit code, stderr, and error.
func (m *mockShell) ExecuteStream(_ context.Context, _ string, ch chan<- string) (int, string, error) {
	close(ch)
	return m.exitCode, m.stderr, m.err
}

// Close returns the preconfigured close error.
func (m *mockShell) Close() error {
	return m.closeErr
}

// TestExecuteWhitelistedWithStderr verifies that stderr output from a whitelisted command
// is sent as an SSE event of type "stderr" before the complete event.
func TestExecuteWhitelistedWithStderr(t *testing.T) {
	sm := NewShellManager()
	defer sm.CloseAll()
	sm.newShell = func() (Shell, error) {
		return &mockShell{exitCode: 0, stderr: "warning: something"}, nil
	}
	handler := newHandler(sm)

	id, _, err := sm.Create()
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	body := strings.NewReader(`{"command":"ls"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/execute", body)
	req.AddCookie(&http.Cookie{Name: "shell_id", Value: id})
	setClientAddressHeader(req)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	events := parseSSEEvents(t, w.Body.String())
	foundStderr := false
	for _, e := range events {
		if e.Type == "stderr" && strings.Contains(e.Data, "warning") {
			foundStderr = true
		}
	}
	if !foundStderr {
		t.Fatalf("did not find stderr event in %v", events)
	}
}

// TestExecuteWhitelistedNonZeroExit verifies that a whitelisted command returning
// a non-zero exit code sends the correct exit code in the complete event.
func TestExecuteWhitelistedNonZeroExit(t *testing.T) {
	sm := NewShellManager()
	defer sm.CloseAll()
	sm.newShell = func() (Shell, error) {
		return &mockShell{exitCode: 2}, nil
	}
	handler := newHandler(sm)

	id, _, err := sm.Create()
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	body := strings.NewReader(`{"command":"ls"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/execute", body)
	req.AddCookie(&http.Cookie{Name: "shell_id", Value: id})
	setClientAddressHeader(req)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	events := parseSSEEvents(t, w.Body.String())
	last := events[len(events)-1]
	if last.Type != "complete" || last.ExitCode == nil || *last.ExitCode != 2 {
		t.Fatalf("expected exitCode=2, got %v", last)
	}
}

// TestExecuteWhitelistedWithExecError verifies that when ExecuteStream returns an error
// on a whitelisted command, the audit log records the error via auditLog.
func TestExecuteWhitelistedWithExecError(t *testing.T) {
	sm := NewShellManager()
	defer sm.CloseAll()
	sm.newShell = func() (Shell, error) {
		return &mockShell{exitCode: -1, err: errors.New("broken")}, nil
	}
	handler := newHandler(sm)

	id, _, err := sm.Create()
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	body := strings.NewReader(`{"command":"ls"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/execute", body)
	req.AddCookie(&http.Cookie{Name: "shell_id", Value: id})
	setClientAddressHeader(req)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

// TestExecuteNonWhitelistedAuditLog verifies that executing a non-whitelisted
// command logs the "unclassified" class and the command string to the audit log.
func TestExecuteNonWhitelistedAuditLog(t *testing.T) {
	var buf bytes.Buffer
	oldOutput := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(oldOutput) })

	sm := NewShellManager()
	defer sm.CloseAll()
	sm.newShell = func() (Shell, error) {
		return &mockShell{exitCode: 0}, nil
	}
	handler := newHandler(sm)

	id, _, err := sm.Create()
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	body := strings.NewReader(`{"command":"curl https://example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/execute", body)
	req.AddCookie(&http.Cookie{Name: "shell_id", Value: id})
	setClientAddressHeader(req)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	logOutput := buf.String()
	if !strings.Contains(logOutput, "class=unclassified") {
		t.Fatalf("expected audit log to contain class=unclassified, got:\n%s", logOutput)
	}
	if !strings.Contains(logOutput, `command="curl https://example.com"`) {
		t.Fatalf("expected audit log to contain the command, got:\n%s", logOutput)
	}
	if !regexp.MustCompile(`(?m)^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}\.\d{3} \[AUDIT\]`).MatchString(logOutput) {
		t.Fatalf("expected audit log timestamp with milliseconds, got:\n%s", logOutput)
	}
}

// TestExecuteNonWhitelistedSSE verifies that a non-whitelisted command
// executes and returns SSE events.
func TestExecuteNonWhitelistedSSE(t *testing.T) {
	sm := NewShellManager()
	defer sm.CloseAll()
	sm.newShell = func() (Shell, error) {
		return &mockShell{exitCode: 0}, nil
	}
	handler := newHandler(sm)

	id, _, err := sm.Create()
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	body := strings.NewReader(`{"command":"curl https://example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/execute", body)
	req.AddCookie(&http.Cookie{Name: "shell_id", Value: id})
	setClientAddressHeader(req)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	events := parseSSEEvents(t, w.Body.String())
	last := events[len(events)-1]
	if last.Type != "complete" || last.ExitCode == nil || *last.ExitCode != 0 {
		t.Fatalf("expected complete with exitCode=0, got %+v", last)
	}
}

// TestHealth verifies that GET /health returns 200 OK with body "ok\n".
func TestHealth(t *testing.T) {
	sm := NewShellManager()
	handler := newHandler(sm)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Body.String(); got != "ok\n" {
		t.Fatalf("body = %q, want %q", got, "ok\n")
	}
}

func TestExecuteClientAddressHeader(t *testing.T) {
	var buf bytes.Buffer
	oldOutput := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(oldOutput) })

	sm := NewShellManager()
	defer sm.CloseAll()
	sm.newShell = func() (Shell, error) {
		return &mockShell{exitCode: 0}, nil
	}
	handler := newHandler(sm)

	id, _, err := sm.Create()
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	body := strings.NewReader(`{"command":"ls"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/execute", body)
	req.AddCookie(&http.Cookie{Name: "shell_id", Value: id})
	req.Header.Set(clientAddressHeader, "198.51.100.20:45678")
	req.Header.Set("CloudFront-Viewer-Address", "203.0.113.50:12345")
	req.Header.Set("X-Forwarded-For", "203.0.113.60")
	req.Header.Set("X-Forwarded-Port", "11111")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	logOutput := buf.String()
	if !strings.Contains(logOutput, "remote=198.51.100.20:45678") {
		t.Fatalf("expected audit log to contain remote=198.51.100.20:45678, got:\n%s", logOutput)
	}
	if strings.Contains(logOutput, "203.0.113.50") || strings.Contains(logOutput, "203.0.113.60") {
		t.Fatalf("expected audit log to ignore client-controlled forwarded headers, got:\n%s", logOutput)
	}
}

func TestExecuteIPv6ClientAddressHeader(t *testing.T) {
	var buf bytes.Buffer
	oldOutput := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(oldOutput) })

	sm := NewShellManager()
	defer sm.CloseAll()
	sm.newShell = func() (Shell, error) {
		return &mockShell{exitCode: 0}, nil
	}
	handler := newHandler(sm)

	id, _, err := sm.Create()
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	body := strings.NewReader(`{"command":"ls"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/execute", body)
	req.AddCookie(&http.Cookie{Name: "shell_id", Value: id})
	req.Header.Set(clientAddressHeader, "[2001:db8::1]:54321")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	logOutput := buf.String()
	if !strings.Contains(logOutput, "remote=[2001:db8::1]:54321") {
		t.Fatalf("expected audit log to contain remote=[2001:db8::1]:54321, got:\n%s", logOutput)
	}
}

func TestExecuteUnbracketedIPv6ClientAddressHeader(t *testing.T) {
	var buf bytes.Buffer
	oldOutput := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(oldOutput) })

	sm := NewShellManager()
	defer sm.CloseAll()
	sm.newShell = func() (Shell, error) {
		return &mockShell{exitCode: 0}, nil
	}
	handler := newHandler(sm)

	id, _, err := sm.Create()
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	body := strings.NewReader(`{"command":"ls"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/execute", body)
	req.AddCookie(&http.Cookie{Name: "shell_id", Value: id})
	req.Header.Set(clientAddressHeader, "2001:db8::1:54321")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	logOutput := buf.String()
	if !strings.Contains(logOutput, "remote=[2001:db8::1]:54321") {
		t.Fatalf("expected audit log to contain remote=[2001:db8::1]:54321, got:\n%s", logOutput)
	}
}

func TestExecuteMissingClientAddressHeader(t *testing.T) {
	sm := NewShellManager()
	defer sm.CloseAll()
	sm.newShell = func() (Shell, error) {
		return &mockShell{exitCode: 0}, nil
	}
	handler := newHandler(sm)

	id, _, err := sm.Create()
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	body := strings.NewReader(`{"command":"ls"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/execute", body)
	req.AddCookie(&http.Cookie{Name: "shell_id", Value: id})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), errMissingClientAddressHeader) {
		t.Fatalf("expected missing client address error, got %q", w.Body.String())
	}
}

func TestExecuteInvalidClientAddressHeader(t *testing.T) {
	sm := NewShellManager()
	defer sm.CloseAll()
	sm.newShell = func() (Shell, error) {
		return &mockShell{exitCode: 0}, nil
	}
	handler := newHandler(sm)

	id, _, err := sm.Create()
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	body := strings.NewReader(`{"command":"ls"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/execute", body)
	req.AddCookie(&http.Cookie{Name: "shell_id", Value: id})
	req.Header.Set(clientAddressHeader, "203.0.113.50")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), errInvalidClientAddressHeader) {
		t.Fatalf("expected invalid client address error, got %q", w.Body.String())
	}
}

func TestParseClientAddressRejectsInvalidPorts(t *testing.T) {
	cases := []string{
		"203.0.113.50:0",
		"203.0.113.50:not-a-port",
		"203.0.113.50:65536",
	}
	for _, value := range cases {
		t.Run(value, func(t *testing.T) {
			_, err := parseClientAddress(value)
			if err == nil || err.Error() != errInvalidClientAddressHeader {
				t.Fatalf("parseClientAddress(%q) error = %v, want %q", value, err, errInvalidClientAddressHeader)
			}
		})
	}
}

// setHandlerAppFilePath points handlerAppFilePath at path for the test and
// restores the original when the test ends.
func setHandlerAppFilePath(t *testing.T, path string) {
	t.Helper()
	orig := handlerAppFilePath
	handlerAppFilePath = path
	t.Cleanup(func() { handlerAppFilePath = orig })
}

// setHandlerAppMaxSize lowers handlerAppMaxSize for the test and restores the
// original when the test ends.
func setHandlerAppMaxSize(t *testing.T, size int64) {
	t.Helper()
	orig := handlerAppMaxSize
	handlerAppMaxSize = size
	t.Cleanup(func() { handlerAppMaxSize = orig })
}

func TestGetAppHandlerSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "DaiKichijoji.pm")
	want := "sub { return (200, 'text/plain', 'hi'); };\n"
	if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	setHandlerAppFilePath(t, path)

	sm := NewShellManager()
	defer sm.CloseAll()
	handler := newHandler(sm)

	req := httptest.NewRequest(http.MethodGet, "/api/app/handler", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain*", ct)
	}
}

func TestGetAppHandlerReadError(t *testing.T) {
	setHandlerAppFilePath(t, filepath.Join(t.TempDir(), "does-not-exist.pl"))

	sm := NewShellManager()
	defer sm.CloseAll()
	handler := newHandler(sm)

	req := httptest.NewRequest(http.MethodGet, "/api/app/handler", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestPutAppHandlerSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "DaiKichijoji.pm")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	setHandlerAppFilePath(t, path)

	sm := NewShellManager()
	defer sm.CloseAll()
	handler := newHandler(sm)

	body := "sub { return (200, 'text/plain', 'new'); };\n"
	req := httptest.NewRequest(http.MethodPut, "/api/app/handler", strings.NewReader(body))
	setClientAddressHeader(req)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != body {
		t.Errorf("file = %q, want %q", got, body)
	}

	// GET after PUT returns the new body.
	getReq := httptest.NewRequest(http.MethodGet, "/api/app/handler", nil)
	getW := httptest.NewRecorder()
	handler.ServeHTTP(getW, getReq)
	if got := getW.Body.String(); got != body {
		t.Errorf("GET body = %q, want %q", got, body)
	}
}

// TestPutAppHandlerAuditLog verifies the audit log includes the written content.
func TestPutAppHandlerAuditLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "DaiKichijoji.pm")
	setHandlerAppFilePath(t, path)

	var buf bytes.Buffer
	oldOutput := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(oldOutput) })

	sm := NewShellManager()
	defer sm.CloseAll()
	handler := newHandler(sm)

	body := "package DaiKichijoji;\nsub content { return 'hi' }\n1;\n"
	req := httptest.NewRequest(http.MethodPut, "/api/app/handler", strings.NewReader(body))
	setClientAddressHeader(req)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}

	logOutput := buf.String()
	if !strings.Contains(logOutput, "class=handler-update") {
		t.Fatalf("expected audit log to contain class=handler-update, got:\n%s", logOutput)
	}
	if !strings.Contains(logOutput, `command="package DaiKichijoji;\nsub content { return 'hi' }\n1;\n"`) {
		t.Fatalf("expected audit log to contain the full written content, got:\n%s", logOutput)
	}
}

func TestPutAppHandlerMissingClientAddress(t *testing.T) {
	sm := NewShellManager()
	defer sm.CloseAll()
	handler := newHandler(sm)

	req := httptest.NewRequest(http.MethodPut, "/api/app/handler", strings.NewReader("x"))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), errMissingClientAddressHeader) {
		t.Errorf("body = %q, want error containing %q", w.Body.String(), errMissingClientAddressHeader)
	}
}

func TestPutAppHandlerBodyTooLarge(t *testing.T) {
	setHandlerAppMaxSize(t, 4)
	dir := t.TempDir()
	setHandlerAppFilePath(t, filepath.Join(dir, "DaiKichijoji.pm"))

	sm := NewShellManager()
	defer sm.CloseAll()
	handler := newHandler(sm)

	req := httptest.NewRequest(http.MethodPut, "/api/app/handler", strings.NewReader("hello"))
	setClientAddressHeader(req)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "read body") {
		t.Errorf("body = %q, want error containing 'read body'", w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "DaiKichijoji.pm")); err == nil {
		t.Error("DaiKichijoji.pm should not be written on oversized body")
	}
}

// TestPutAppHandlerBodyTooLargePreservesExistingFile verifies that a PUT
// rejected for exceeding handlerAppMaxSize leaves an existing
// DaiKichijoji.pm untouched.
func TestPutAppHandlerBodyTooLargePreservesExistingFile(t *testing.T) {
	setHandlerAppMaxSize(t, 4)
	dir := t.TempDir()
	path := filepath.Join(dir, "DaiKichijoji.pm")
	old := "package DaiKichijoji;\nsub content { return 'old' }\n1;\n"
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	setHandlerAppFilePath(t, path)

	sm := NewShellManager()
	defer sm.CloseAll()
	handler := newHandler(sm)

	req := httptest.NewRequest(http.MethodPut, "/api/app/handler", strings.NewReader("way too much content"))
	setClientAddressHeader(req)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != old {
		t.Errorf("file = %q, want unchanged %q", got, old)
	}
}

func TestPutAppHandlerWriteError(t *testing.T) {
	setHandlerAppFilePath(t, filepath.Join(t.TempDir(), "no-such-parent", "DaiKichijoji.pm"))

	sm := NewShellManager()
	defer sm.CloseAll()
	handler := newHandler(sm)

	req := httptest.NewRequest(http.MethodPut, "/api/app/handler", strings.NewReader("body"))
	setClientAddressHeader(req)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestPutAppHandlerRenameError(t *testing.T) {
	dir := t.TempDir()
	// Point handlerAppFilePath at a directory; rename over a directory fails.
	target := filepath.Join(dir, "DaiKichijoji.pm")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "child"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	setHandlerAppFilePath(t, target)

	sm := NewShellManager()
	defer sm.CloseAll()
	handler := newHandler(sm)

	req := httptest.NewRequest(http.MethodPut, "/api/app/handler", strings.NewReader("body"))
	setClientAddressHeader(req)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	// tmp cleanup: os.CreateTempが作った".DaiKichijoji.pm.*.tmp"はrename失敗時に削除される。
	matches, err := filepath.Glob(filepath.Join(dir, ".DaiKichijoji.pm.*.tmp"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(matches) > 0 {
		t.Errorf("tmp files should be cleaned up after rename failure, found: %v", matches)
	}
}

// parseSSEEvents parses a raw SSE response body into a slice of sseEvent.
// It expects each event to be a "data: " line followed by a blank line.
func parseSSEEvents(t *testing.T, body string) []sseEvent {
	t.Helper()
	var events []sseEvent
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		var event sseEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			t.Fatalf("unmarshal SSE event %q: %v", data, err)
		}
		events = append(events, event)
	}
	return events
}
