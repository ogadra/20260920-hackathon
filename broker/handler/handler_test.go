// Package handler は broker の HTTP ハンドラーのテストを提供する。
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ogadra/bunshin/broker/model"
	"github.com/ogadra/bunshin/broker/service"
	"github.com/ogadra/bunshin/broker/store"
)

// mockService は service.Service のモック実装。
type mockService struct {
	closeSessionFn     func(ctx context.Context, sessionID string) error
	resolveSessionFn   func(ctx context.Context, sessionID string) (*service.ResolveResult, error)
	lookupSessionFn    func(ctx context.Context, hex string) (*service.LookupResult, error)
	registerRunnerFn   func(ctx context.Context, runnerID, privateHost string) error
	deregisterRunnerFn func(ctx context.Context, runnerID string) error
	listBusyRunnersFn  func(ctx context.Context) ([]model.Runner, error)
}

// CloseSession はモック CloseSession を呼び出す。
func (m *mockService) CloseSession(ctx context.Context, sessionID string) error {
	return m.closeSessionFn(ctx, sessionID)
}

// ResolveSession はモック ResolveSession を呼び出す。
func (m *mockService) ResolveSession(ctx context.Context, sessionID string) (*service.ResolveResult, error) {
	return m.resolveSessionFn(ctx, sessionID)
}

// LookupSession はモック LookupSession を呼び出す。
func (m *mockService) LookupSession(ctx context.Context, hex string) (*service.LookupResult, error) {
	return m.lookupSessionFn(ctx, hex)
}

// RegisterRunner はモック RegisterRunner を呼び出す。
func (m *mockService) RegisterRunner(ctx context.Context, runnerID, privateHost string) error {
	return m.registerRunnerFn(ctx, runnerID, privateHost)
}

// DeregisterRunner はモック DeregisterRunner を呼び出す。
func (m *mockService) DeregisterRunner(ctx context.Context, runnerID string) error {
	return m.deregisterRunnerFn(ctx, runnerID)
}

// ListBusyRunners はモック ListBusyRunners を呼び出す。
func (m *mockService) ListBusyRunners(ctx context.Context) ([]model.Runner, error) {
	return m.listBusyRunnersFn(ctx)
}

// newTestRouter はテスト用のルーターを構築する。
func newTestRouter(h *Handler) *gin.Engine {
	r := gin.New()
	r.Use(RequestIDMiddleware(func() (string, error) {
		return "test-req-id", nil
	}))
	r.DELETE("/sessions/:sessionId", h.DeleteSession)
	r.GET("/resolve/session", h.GetResolveSession)
	r.GET("/resolve/app", h.GetResolveApp)
	r.POST("/internal/runners/register", h.PostRegister)
	r.DELETE("/internal/runners/:runnerId", h.DeleteRunner)
	r.GET("/runners/busy", h.GetListBusyRunners)
	return r
}

// TestDeleteSession_Success はセッション終了の成功を検証する。
func TestDeleteSession_Success(t *testing.T) {
	t.Parallel()
	h := NewHandler(&mockService{
		closeSessionFn: func(_ context.Context, sessionID string) error {
			if sessionID != "sess-abc" {
				t.Errorf("sessionID = %q, want %q", sessionID, "sess-abc")
			}
			return nil
		},
	}, []string{}, "ap-northeast-1")
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/sessions/sess-abc", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

// TestDeleteSession_NotFound はセッションが見つからない場合に 404 を返すことを検証する。
func TestDeleteSession_NotFound(t *testing.T) {
	t.Parallel()
	h := NewHandler(&mockService{
		closeSessionFn: func(_ context.Context, _ string) error {
			return store.ErrNotFound
		},
	}, []string{}, "ap-northeast-1")
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/sessions/sess-missing", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestDeleteSession_InternalError は内部エラー時に 500 を返すことを検証する。
func TestDeleteSession_InternalError(t *testing.T) {
	t.Parallel()
	h := NewHandler(&mockService{
		closeSessionFn: func(_ context.Context, _ string) error {
			return errors.New("unexpected")
		},
	}, []string{}, "ap-northeast-1")
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/sessions/sess-abc", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// TestGetResolveSession_ExistingSession は既存セッションの解決成功を検証する。
func TestGetResolveSession_ExistingSession(t *testing.T) {
	t.Parallel()
	h := NewHandler(&mockService{
		resolveSessionFn: func(_ context.Context, sessionID string) (*service.ResolveResult, error) {
			if sessionID != "ap-northeast-1_sess-abc" {
				t.Errorf("sessionID = %q, want %q", sessionID, "ap-northeast-1_sess-abc")
			}
			return &service.ResolveResult{SessionHex: "sess-abc", RunnerHost: "10.0.0.1", Created: false}, nil
		},
	}, []string{}, "ap-northeast-1")
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/resolve/session", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "ap-northeast-1_sess-abc"})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get(runnerHostHeader); got != "10.0.0.1" {
		t.Errorf("%s = %q, want %q", runnerHostHeader, got, "10.0.0.1")
	}
	if got := rec.Header().Get(stackNameHeader); got != "ap-northeast-1" {
		t.Errorf("%s = %q, want %q", stackNameHeader, got, "ap-northeast-1")
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session_id" {
			t.Error("should not set session_id cookie for existing session")
		}
	}
}

// TestGetResolveSession_MissingCookie_CreatesSession は cookie がない場合にセッションを新規作成することを検証する。
func TestGetResolveSession_MissingCookie_CreatesSession(t *testing.T) {
	t.Parallel()
	h := NewHandler(&mockService{
		resolveSessionFn: func(_ context.Context, sessionID string) (*service.ResolveResult, error) {
			if sessionID != "" {
				t.Errorf("sessionID = %q, want empty", sessionID)
			}
			return &service.ResolveResult{SessionID: "ap-northeast-1_new-sess", SessionHex: "new-sess", RunnerHost: "10.0.0.2", Created: true}, nil
		},
	}, []string{}, "ap-northeast-1")
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/resolve/session", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get(runnerHostHeader); got != "10.0.0.2" {
		t.Errorf("%s = %q, want %q", runnerHostHeader, got, "10.0.0.2")
	}
	found := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session_id" && c.Value == "ap-northeast-1_new-sess" {
			found = true
		}
	}
	if !found {
		t.Error("expected session_id cookie for new session")
	}
}

// TestGetResolveSession_NoIdleRunner は idle runner がない場合に 503 を返すことを検証する。
func TestGetResolveSession_NoIdleRunner(t *testing.T) {
	t.Parallel()
	h := NewHandler(&mockService{
		resolveSessionFn: func(_ context.Context, _ string) (*service.ResolveResult, error) {
			return nil, store.ErrNoIdleRunner
		},
	}, []string{}, "ap-northeast-1")
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/resolve/session", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if got := rec.Header().Get(fallbackStackHeader); got != "" {
		t.Errorf("%s = %q, want empty when no fallback configured", fallbackStackHeader, got)
	}
	if got := rec.Header().Get(sessionHexHeader); got != "" {
		t.Errorf("%s = %q, want empty on error", sessionHexHeader, got)
	}
	if got := rec.Header().Get(stackNameHeader); got != "" {
		t.Errorf("%s = %q, want empty on error", stackNameHeader, got)
	}
}

func noIdleResolve(fallback []string, reqHeaders map[string]string) *httptest.ResponseRecorder {
	return noIdleResolveWithCookie(fallback, reqHeaders, "")
}

func noIdleResolveWithCookie(fallback []string, reqHeaders map[string]string, sessionID string) *httptest.ResponseRecorder {
	h := NewHandler(&mockService{
		resolveSessionFn: func(_ context.Context, _ string) (*service.ResolveResult, error) {
			return nil, store.ErrNoIdleRunner
		},
	}, fallback, "ap-northeast-1")
	req := httptest.NewRequest(http.MethodGet, "/resolve/session", nil)
	for k, v := range reqHeaders {
		req.Header.Set(k, v)
	}
	if sessionID != "" {
		req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})
	}
	rec := httptest.NewRecorder()
	newTestRouter(h).ServeHTTP(rec, req)
	return rec
}

// TestGetResolveSession_FallbackOrigin は origin の枯渇で先頭を X-Fallback-Stack、残りを X-Fallback-Remaining に出すことを検証する。
func TestGetResolveSession_FallbackOrigin(t *testing.T) {
	t.Parallel()
	rec := noIdleResolve([]string{"ap-northeast-3", "ap-northeast-2"}, nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if got := rec.Header().Get(fallbackStackHeader); got != "ap-northeast-3" {
		t.Errorf("%s = %q, want %q", fallbackStackHeader, got, "ap-northeast-3")
	}
	if got := rec.Header().Get(fallbackRemainingHeader); got != "ap-northeast-2" {
		t.Errorf("%s = %q, want %q", fallbackRemainingHeader, got, "ap-northeast-2")
	}
}

// TestGetResolveSession_FallbackForwarded は転送済みで残りがある場合、remaining の先頭を次の forward にすることを検証する。
func TestGetResolveSession_FallbackForwarded(t *testing.T) {
	t.Parallel()
	rec := noIdleResolve([]string{}, map[string]string{
		fallbackStackHeader:     "ap-northeast-3",
		fallbackRemainingHeader: "ap-northeast-2",
	})
	if got := rec.Header().Get(fallbackStackHeader); got != "ap-northeast-2" {
		t.Errorf("%s = %q, want %q", fallbackStackHeader, got, "ap-northeast-2")
	}
	if got := rec.Header().Get(fallbackRemainingHeader); got != "" {
		t.Errorf("%s = %q, want empty", fallbackRemainingHeader, got)
	}
}

// TestGetResolveSession_FallbackForwardedKeepsTail は remaining が複数のとき先頭を pop し残りを維持することを検証する。
func TestGetResolveSession_FallbackForwardedKeepsTail(t *testing.T) {
	t.Parallel()
	rec := noIdleResolve([]string{}, map[string]string{
		fallbackStackHeader:     "ap-northeast-3",
		fallbackRemainingHeader: "ap-northeast-2,us-east-1",
	})
	if got := rec.Header().Get(fallbackStackHeader); got != "ap-northeast-2" {
		t.Errorf("%s = %q, want %q", fallbackStackHeader, got, "ap-northeast-2")
	}
	if got := rec.Header().Get(fallbackRemainingHeader); got != "us-east-1" {
		t.Errorf("%s = %q, want %q", fallbackRemainingHeader, got, "us-east-1")
	}
}

// TestGetResolveSession_FallbackLastStack は転送済みで残りが無い(最後の stack)場合に forward を出さないことを検証する。
func TestGetResolveSession_FallbackLastStack(t *testing.T) {
	t.Parallel()
	rec := noIdleResolve([]string{}, map[string]string{fallbackStackHeader: "ap-northeast-2"})
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if got := rec.Header().Get(fallbackStackHeader); got != "" {
		t.Errorf("%s = %q, want empty for last stack", fallbackStackHeader, got)
	}
}

// TestGetResolveSession_InternalError は内部エラー時に 500 を返すことを検証する。
func TestGetResolveSession_InternalError(t *testing.T) {
	t.Parallel()
	h := NewHandler(&mockService{
		resolveSessionFn: func(_ context.Context, _ string) (*service.ResolveResult, error) {
			return nil, errors.New("unexpected")
		},
	}, []string{}, "ap-northeast-1")
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/resolve/session", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "sess-abc"})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if got := rec.Header().Get(sessionHexHeader); got != "" {
		t.Errorf("%s = %q, want empty on error", sessionHexHeader, got)
	}
	if got := rec.Header().Get(stackNameHeader); got != "" {
		t.Errorf("%s = %q, want empty on error", stackNameHeader, got)
	}
}

// TestPostRegister_Success は runner 登録の成功を検証する。
func TestPostRegister_Success(t *testing.T) {
	t.Parallel()
	h := NewHandler(&mockService{
		registerRunnerFn: func(_ context.Context, runnerID, privateHost string) error {
			if runnerID != "r1" {
				t.Errorf("runnerID = %q, want %q", runnerID, "r1")
			}
			if privateHost != "10.0.0.1" {
				t.Errorf("privateHost = %q, want %q", privateHost, "10.0.0.1")
			}
			return nil
		},
	}, []string{}, "ap-northeast-1")
	r := newTestRouter(h)

	body := strings.NewReader(`{"runnerId":"r1","privateHost":"10.0.0.1"}`)
	req := httptest.NewRequest(http.MethodPost, "/internal/runners/register", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
}

// TestPostRegister_InvalidBody はリクエストボディが不正な場合に 400 を返すことを検証する。
func TestPostRegister_InvalidBody(t *testing.T) {
	t.Parallel()
	h := NewHandler(&mockService{}, []string{}, "ap-northeast-1")
	r := newTestRouter(h)

	body := strings.NewReader(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/internal/runners/register", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestPostRegister_InternalError は内部エラー時に 500 を返すことを検証する。
func TestPostRegister_InternalError(t *testing.T) {
	t.Parallel()
	h := NewHandler(&mockService{
		registerRunnerFn: func(_ context.Context, _, _ string) error {
			return errors.New("unexpected")
		},
	}, []string{}, "ap-northeast-1")
	r := newTestRouter(h)

	body := strings.NewReader(`{"runnerId":"r1","privateHost":"10.0.0.1"}`)
	req := httptest.NewRequest(http.MethodPost, "/internal/runners/register", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// TestPostRegister_Conflict は同一 runnerId が別属性で登録済みの場合に 409 を返すことを検証する。
func TestPostRegister_Conflict(t *testing.T) {
	t.Parallel()
	h := NewHandler(&mockService{
		registerRunnerFn: func(_ context.Context, _, _ string) error {
			return store.ErrConflict
		},
	}, []string{}, "ap-northeast-1")
	r := newTestRouter(h)

	body := strings.NewReader(`{"runnerId":"r1","privateHost":"10.0.0.1"}`)
	req := httptest.NewRequest(http.MethodPost, "/internal/runners/register", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

// TestDeleteRunner_Success は runner 削除の成功を検証する。
func TestDeleteRunner_Success(t *testing.T) {
	t.Parallel()
	h := NewHandler(&mockService{
		deregisterRunnerFn: func(_ context.Context, runnerID string) error {
			if runnerID != "r1" {
				t.Errorf("runnerID = %q, want %q", runnerID, "r1")
			}
			return nil
		},
	}, []string{}, "ap-northeast-1")
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/internal/runners/r1", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

// TestDeleteRunner_InternalError は内部エラー時に 500 を返すことを検証する。
func TestDeleteRunner_InternalError(t *testing.T) {
	t.Parallel()
	h := NewHandler(&mockService{
		deregisterRunnerFn: func(_ context.Context, _ string) error {
			return errors.New("unexpected")
		},
	}, []string{}, "ap-northeast-1")
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/internal/runners/r1", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// TestWriteError_IncludesRequestID はエラーレスポンスに requestId が含まれることを検証する。
func TestWriteError_IncludesRequestID(t *testing.T) {
	t.Parallel()
	h := NewHandler(&mockService{
		resolveSessionFn: func(_ context.Context, _ string) (*service.ResolveResult, error) {
			return nil, errors.New("unexpected")
		},
	}, []string{}, "ap-northeast-1")
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/resolve/session", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "sess-missing"})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), "test-req-id") {
		t.Errorf("body = %q, want to contain %q", rec.Body.String(), "test-req-id")
	}
}

// TestNewHandler は NewHandler のコンストラクタを検証する。
func TestNewHandler(t *testing.T) {
	t.Parallel()
	svc := &mockService{}
	h := NewHandler(svc, []string{}, "ap-northeast-1")
	if h.svc != svc {
		t.Error("svc mismatch")
	}
	if h.stackSelf != "ap-northeast-1" {
		t.Errorf("stackSelf = %q, want %q", h.stackSelf, "ap-northeast-1")
	}
}

// TestNewHandler_NilPanics は NewHandler に nil を渡すと panic することを検証する。
func TestNewHandler_NilPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil service")
		}
	}()
	NewHandler(nil, []string{}, "ap-northeast-1")
}

// TestNewHandler_EmptyStackPanicsはNewHandlerに空stackSelfを渡すとpanicすることを検証する。
func TestNewHandler_EmptyStackPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for empty stackSelf")
		}
	}()
	NewHandler(&mockService{}, []string{}, "")
}

// TestPostRegister_InvalidHost は host label 形式ではない privateHost に対して 400 を返すことを検証する。
func TestPostRegister_InvalidHost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		host string
	}{
		{"has colon", "10.0.0.1:8080"},
		{"has scheme prefix", "http://10.0.0.1"},
		{"has slash", "runner-1/"},
		{"underscore", "runner_01"},
		{"has space", "run ner"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := NewHandler(&mockService{
				registerRunnerFn: func(_ context.Context, _, _ string) error {
					t.Fatal("service should not be called for invalid host")
					return nil
				},
			}, []string{}, "ap-northeast-1")
			r := newTestRouter(h)

			body := strings.NewReader(`{"runnerId":"r1","privateHost":"` + tt.host + `"}`)
			req := httptest.NewRequest(http.MethodPost, "/internal/runners/register", body)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d for host %q", rec.Code, http.StatusBadRequest, tt.host)
			}
		})
	}
}

// TestValidateRunnerHost は validateRunnerHost の境界値を検証する。
// port / scheme を含まない host label のみを許可する。
func TestValidateRunnerHost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		host    string
		wantErr bool
	}{
		{"ipv4", "10.0.0.1", false},
		{"dns hostname", "runner.local", false},
		{"runner with hyphen", "runner-1", false},
		{"digits mixed", "runner01.internal.example", false},
		{"empty", "", true},
		{"colon", "runner:3000", true},
		{"scheme", "http://runner", true},
		{"slash", "runner/", true},
		{"userinfo", "user@runner", true},
		{"space", "run ner", true},
		{"underscore", "runner_01", true},
		{"query", "runner?x=1", true},
		{"ipv6 brackets", "[::1]", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateRunnerHost(tt.host)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRunnerHost(%q) error = %v, wantErr %v", tt.host, err, tt.wantErr)
			}
		})
	}
}

// TestGetResolveSession_CookieSecure はセッション新規作成時の session_id cookie が Secure=true であることを検証する。
func TestGetResolveSession_CookieSecure(t *testing.T) {
	t.Parallel()
	h := NewHandler(&mockService{
		resolveSessionFn: func(_ context.Context, _ string) (*service.ResolveResult, error) {
			return &service.ResolveResult{SessionID: "ap-northeast-1_new-sess", SessionHex: "new-sess", RunnerHost: "10.0.0.1", Created: true}, nil
		},
	}, []string{}, "ap-northeast-1")
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/resolve/session", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	for _, c := range rec.Result().Cookies() {
		if c.Name == "session_id" {
			if !c.Secure {
				t.Error("expected Secure=true on session_id cookie")
			}
			if !c.HttpOnly {
				t.Error("expected HttpOnly=true on session_id cookie")
			}
			if c.SameSite != http.SameSiteStrictMode {
				t.Errorf("expected SameSite=Strict on session_id cookie, got %v", c.SameSite)
			}
			return
		}
	}
	t.Error("session_id cookie not found")
}

// TestGetResolveSession_SessionHexHeaderは新規・既存どちらのセッションでも
// X-Session-Hexヘッダーにserviceが返したhexがそのまま設定されることを検証する。
func TestGetResolveSession_SessionHexHeader(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		result  *service.ResolveResult
		wantHex string
	}{
		{
			name: "created session",
			result: &service.ResolveResult{
				SessionHex: "0123456789abcdef0123456789abcdef",
				RunnerHost: "10.0.0.1",
				Created:    true,
			},
			wantHex: "0123456789abcdef0123456789abcdef",
		},
		{
			name: "existing session",
			result: &service.ResolveResult{
				SessionHex: "fedcba9876543210fedcba9876543210",
				RunnerHost: "10.0.0.2",
				Created:    false,
			},
			wantHex: "fedcba9876543210fedcba9876543210",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := NewHandler(&mockService{
				resolveSessionFn: func(_ context.Context, _ string) (*service.ResolveResult, error) {
					return tt.result, nil
				},
			}, []string{}, "ap-northeast-1")
			r := newTestRouter(h)

			req := httptest.NewRequest(http.MethodGet, "/resolve/session", nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if got := rec.Header().Get(sessionHexHeader); got != tt.wantHex {
				t.Errorf("%s = %q, want %q", sessionHexHeader, got, tt.wantHex)
			}
			if got := rec.Header().Get(stackNameHeader); got != "ap-northeast-1" {
				t.Errorf("%s = %q, want %q", stackNameHeader, got, "ap-northeast-1")
			}
		})
	}
}

// TestGetResolveSession_Reassigned はセッション再割当て時に X-Session-Reassigned ヘッダーが設定されることを検証する。
func TestGetResolveSession_Reassigned(t *testing.T) {
	t.Parallel()
	h := NewHandler(&mockService{
		resolveSessionFn: func(_ context.Context, _ string) (*service.ResolveResult, error) {
			return &service.ResolveResult{
				SessionID:  "ap-northeast-1_new-sess",
				SessionHex: "new-sess",
				RunnerHost: "10.0.0.2",
				Created:    true,
				Reassigned: true,
			}, nil
		},
	}, []string{}, "ap-northeast-1")
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/resolve/session", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "old-sess"})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get(reassignedHeader); got != "true" {
		t.Errorf("%s = %q, want %q", reassignedHeader, got, "true")
	}
	if got := rec.Header().Get(runnerHostHeader); got != "10.0.0.2" {
		t.Errorf("%s = %q, want %q", runnerHostHeader, got, "10.0.0.2")
	}
	if got := rec.Header().Get(sessionHexHeader); got != "new-sess" {
		t.Errorf("%s = %q, want %q", sessionHexHeader, got, "new-sess")
	}
}

// TestGetResolveSession_NotReassigned はセッション再割当てなしの場合に X-Session-Reassigned ヘッダーが設定されないことを検証する。
func TestGetResolveSession_NotReassigned(t *testing.T) {
	t.Parallel()
	h := NewHandler(&mockService{
		resolveSessionFn: func(_ context.Context, _ string) (*service.ResolveResult, error) {
			return &service.ResolveResult{
				SessionHex: "sess-1",
				RunnerHost: "10.0.0.1",
				Created:    false,
				Reassigned: false,
			}, nil
		},
	}, []string{}, "ap-northeast-1")
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/resolve/session", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "ap-northeast-1_sess-1"})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get(reassignedHeader); got != "" {
		t.Errorf("%s = %q, want empty", reassignedHeader, got)
	}
	if got := rec.Header().Get(runnerHostHeader); got != "10.0.0.1" {
		t.Errorf("%s = %q, want %q", runnerHostHeader, got, "10.0.0.1")
	}
}

// log.SetOutput はパッケージ全体のグローバル状態を触るため、これを使うテストでは t.Parallel() を付けない。
func captureStdLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	origOut, origFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(origOut)
		log.SetFlags(origFlags)
	})
	return &buf
}

// TestSignalFallback_LogsUnavailable は fallback pool 枯渇時に
// fallback_signal unavailable ログが request_id と session_id 付きで出ることを検証する。
func TestSignalFallback_LogsUnavailable(t *testing.T) {
	buf := captureStdLog(t)
	rec := noIdleResolveWithCookie([]string{}, nil, "sess-xyz")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	for _, want := range []string{
		"fallback_signal unavailable",
		"request_id=test-req-id",
		"session_id=sess-xyz",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("log = %q, want substring %q", buf.String(), want)
		}
	}
}

// TestSignalFallback_LogsNextStack は fallback 転送発生時に
// next_stack / remaining / request_id / session_id 付きのログが出ることを検証する。
func TestSignalFallback_LogsNextStack(t *testing.T) {
	buf := captureStdLog(t)
	rec := noIdleResolveWithCookie([]string{"ap-northeast-3", "ap-northeast-2"}, nil, "sess-abc")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	for _, want := range []string{
		"fallback_signal next_stack=ap-northeast-3",
		"remaining=ap-northeast-2",
		"request_id=test-req-id",
		"session_id=sess-abc",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("log = %q, want substring %q", buf.String(), want)
		}
	}
}

// TestGetListBusyRunners_Success はサービスが返した runner を JSON で返すことを検証する。
// currentSessionId は cookie 値そのものなのでレスポンスに含めない。
func TestGetListBusyRunners_Success(t *testing.T) {
	t.Parallel()
	h := NewHandler(&mockService{
		listBusyRunnersFn: func(context.Context) ([]model.Runner, error) {
			return []model.Runner{
				{RunnerID: "r1", State: model.StateBusy, CurrentSessionID: "sess-1"},
				{RunnerID: "r2", State: model.StateBusy, CurrentSessionID: "sess-2"},
			}, nil
		},
	}, []string{}, "ap-northeast-1")
	rec := httptest.NewRecorder()
	newTestRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/runners/busy", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if strings.Contains(rec.Body.String(), "sess-") {
		t.Errorf("response leaks session id: %s", rec.Body.String())
	}
	var resp struct {
		Runners []struct {
			RunnerID string `json:"runnerId"`
		} `json:"runners"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if len(resp.Runners) != 2 {
		t.Fatalf("len(runners) = %d, want 2", len(resp.Runners))
	}
	if resp.Runners[0].RunnerID != "r1" {
		t.Errorf("runners[0].RunnerID = %q, want r1", resp.Runners[0].RunnerID)
	}
	if resp.Runners[1].RunnerID != "r2" {
		t.Errorf("runners[1].RunnerID = %q, want r2", resp.Runners[1].RunnerID)
	}
}

// TestGetListBusyRunners_Empty は busy runner がいない場合に空配列を返すことを検証する。
func TestGetListBusyRunners_Empty(t *testing.T) {
	t.Parallel()
	h := NewHandler(&mockService{
		listBusyRunnersFn: func(context.Context) ([]model.Runner, error) {
			return nil, nil
		},
	}, []string{}, "ap-northeast-1")
	rec := httptest.NewRecorder()
	newTestRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/runners/busy", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"runners":[]`) {
		t.Errorf("body = %q, want to contain \"runners\":[]", rec.Body.String())
	}
}

// TestGetListBusyRunners_ServiceError はサービスがエラーを返した場合に 500 を返すことを検証する。
func TestGetListBusyRunners_ServiceError(t *testing.T) {
	t.Parallel()
	h := NewHandler(&mockService{
		listBusyRunnersFn: func(context.Context) ([]model.Runner, error) {
			return nil, errors.New("boom")
		},
	}, []string{}, "ap-northeast-1")
	rec := httptest.NewRecorder()
	newTestRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/runners/busy", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// TestGetResolveApp_Existing は既存 session の Host 先頭 hex ラベルから runner host を返すことを検証する。
func TestGetResolveApp_Existing(t *testing.T) {
	t.Parallel()
	h := NewHandler(&mockService{
		lookupSessionFn: func(_ context.Context, hex string) (*service.LookupResult, error) {
			if hex != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
				t.Errorf("hex = %q, want %q", hex, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
			}
			return &service.LookupResult{RunnerHost: "10.0.0.1"}, nil
		},
	}, []string{}, "ap-northeast-1")
	req := httptest.NewRequest(http.MethodGet, "/resolve/app", nil)
	req.Host = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.ap-northeast-1.internal.example.com"
	rec := httptest.NewRecorder()
	newTestRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get(runnerHostHeader); got != "10.0.0.1" {
		t.Errorf("%s = %q, want %q", runnerHostHeader, got, "10.0.0.1")
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session_id" {
			t.Errorf("lookup must not set session_id cookie")
		}
	}
}

// TestGetResolveApp_NotFound は不在 hex が 404 SESSION_NOT_FOUND を返すことを検証する。
func TestGetResolveApp_NotFound(t *testing.T) {
	t.Parallel()
	h := NewHandler(&mockService{
		lookupSessionFn: func(context.Context, string) (*service.LookupResult, error) {
			return nil, store.ErrNotFound
		},
	}, []string{}, "ap-northeast-1")
	req := httptest.NewRequest(http.MethodGet, "/resolve/app", nil)
	req.Host = "00112233445566778899aabbccddeeff.ap-northeast-1.internal.example.com"
	rec := httptest.NewRecorder()
	newTestRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if !strings.Contains(rec.Body.String(), `"code":"SESSION_NOT_FOUND"`) {
		t.Errorf("body = %q, want SESSION_NOT_FOUND", rec.Body.String())
	}
	if got := rec.Header().Get(runnerHostHeader); got != "" {
		t.Errorf("%s = %q, want empty", runnerHostHeader, got)
	}
}

// TestGetResolveApp_InvalidHost は Host の先頭 hex ラベルが 32 lowercase hex + dot ではない場合に
// 400 INVALID_REQUEST を返し LookupSession を呼ばないことを検証する。
func TestGetResolveApp_InvalidHost(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		host string
	}{
		{"no dot", strings.Repeat("a", 32)},
		{"too short label", "aaaa.ap-northeast-1.internal.example.com"},
		{"too long label", strings.Repeat("a", 33) + ".ap-northeast-1.internal.example.com"},
		{"uppercase label", strings.Repeat("A", 32) + ".ap-northeast-1.internal.example.com"},
		{"non hex label", strings.Repeat("g", 32) + ".ap-northeast-1.internal.example.com"},
		{"non hex first", "broker.internal.example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := NewHandler(&mockService{
				lookupSessionFn: func(context.Context, string) (*service.LookupResult, error) {
					t.Error("LookupSession must not be called for invalid host")
					return nil, nil
				},
			}, []string{}, "ap-northeast-1")
			req := httptest.NewRequest(http.MethodGet, "/resolve/app", nil)
			req.Host = tc.host
			rec := httptest.NewRecorder()
			newTestRouter(h).ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
			if !strings.Contains(rec.Body.String(), `"code":"INVALID_REQUEST"`) {
				t.Errorf("body = %q, want INVALID_REQUEST", rec.Body.String())
			}
		})
	}
}

// TestGetResolveApp_ServiceError は LookupSession のエラーが 500 INTERNAL_ERROR を返すことを検証する。
func TestGetResolveApp_ServiceError(t *testing.T) {
	t.Parallel()
	h := NewHandler(&mockService{
		lookupSessionFn: func(context.Context, string) (*service.LookupResult, error) {
			return nil, errors.New("boom")
		},
	}, []string{}, "ap-northeast-1")
	req := httptest.NewRequest(http.MethodGet, "/resolve/app", nil)
	req.Host = "00112233445566778899aabbccddeeff.ap-northeast-1.internal.example.com"
	rec := httptest.NewRecorder()
	newTestRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rec.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Errorf("body = %q, want INTERNAL_ERROR", rec.Body.String())
	}
}
