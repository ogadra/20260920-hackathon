package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ogadra/bunshin/broker/config"
	"github.com/ogadra/bunshin/broker/handler"
	"github.com/ogadra/bunshin/broker/healthcheck"
	"github.com/ogadra/bunshin/broker/model"
	"github.com/ogadra/bunshin/broker/service"
	"github.com/ogadra/bunshin/broker/store"
)

type fakeDynamoRepo struct{ store.Repository }

// stubDynamoFactory は AWS SDK の LoadDefaultConfig を unit test から迂回するため
// config.NewDynamoRepositoryFn を fake factory に差し替える。
func stubDynamoFactory(t *testing.T) {
	t.Helper()
	orig := config.NewDynamoRepositoryFn
	t.Cleanup(func() { config.NewDynamoRepositoryFn = orig })
	config.NewDynamoRepositoryFn = func(context.Context, store.DynamoConfig) (store.Repository, error) {
		return fakeDynamoRepo{}, nil
	}
}

// setDynamoEnv は dynamodb 経路で必要な env をまとめて設定し、fake factory に差し替えて Handler を組み立てられるようにする。
func setDynamoEnv(t *testing.T) {
	t.Helper()
	t.Setenv("STACK_NAME", "ap-northeast-1")
	t.Setenv("BUNSHIN_STACKS", "ap-northeast-1,ap-northeast-3")
	t.Setenv("RUNNER_API_PORT", "3000")
	t.Setenv("BUNSHIN_STORE", "dynamodb")
	t.Setenv("DYNAMODB_ENDPOINT", "http://localhost:18000")
	t.Setenv("AWS_REGION", "ap-northeast-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "localdev")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "localdev")
	stubDynamoFactory(t)
}

// saveAndRestore は server lifecycle 用のパッケージレベル変数を退避し、テスト終了時に復元する。
func saveAndRestore(t *testing.T) {
	t.Helper()
	origStdout := stdout
	origAddr := addr
	origShutdownTimeout := shutdownTimeout
	origFatalf := fatalf
	origSignalNotify := signalNotify
	t.Cleanup(func() {
		stdout = origStdout
		addr = origAddr
		shutdownTimeout = origShutdownTimeout
		fatalf = origFatalf
		signalNotify = origSignalNotify
	})
}

// noopInit は init 失敗を再現しない no-op initHandler。Closer も noop を返す。
func noopInit() (*handler.Handler, io.Closer, error) { return nil, noopCloser{}, nil }

type closerRepo struct {
	store.Repository
	closeErr error
	closed   bool
}

func (c *closerRepo) Close() error {
	c.closed = true
	return c.closeErr
}

// TestRepoCloser_ReturnsCloserWhenImplemented は io.Closer 実装 (Firestore など) が
// そのまま返され Close が repo 側に流れることを検証する。
func TestRepoCloser_ReturnsCloserWhenImplemented(t *testing.T) {
	repo := &closerRepo{}
	closer := repoCloser(repo)
	if err := closer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !repo.closed {
		t.Error("expected underlying repo.Close to be called")
	}
}

// TestRepoCloser_NoopForNonCloser は Close を持たない Repository (DynamoRepository) は
// noopCloser にフォールバックし、Close 呼び出しがエラーにならないことを検証する。
func TestRepoCloser_NoopForNonCloser(t *testing.T) {
	closer := repoCloser(fakeDynamoRepo{})
	if err := closer.Close(); err != nil {
		t.Errorf("noop Close should return nil, got %v", err)
	}
}

// TestRunLogsCloseError は shutdown 後の repo.Close エラーが stdout に書き出されることを検証する。
func TestRunLogsCloseError(t *testing.T) {
	saveAndRestore(t)

	var buf bytes.Buffer
	stdout = &buf
	addr = ":0"
	shutdownTimeout = 1 * time.Second

	sigCh := make(chan os.Signal, 1)
	signalNotify = func(c chan<- os.Signal, _ ...os.Signal) {
		go func() {
			c <- <-sigCh
		}()
	}

	closeErr := errors.New("boom")
	initWithErrCloser := func() (*handler.Handler, io.Closer, error) {
		return nil, &closerRepo{closeErr: closeErr}, nil
	}

	done := make(chan error, 1)
	go func() { done <- run(initWithErrCloser) }()

	time.Sleep(100 * time.Millisecond)
	sigCh <- os.Interrupt
	if err := <-done; err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(buf.String(), "close repository: boom") {
		t.Errorf("expected close error log, got %q", buf.String())
	}
}

// TestHealthEndpoint は GET /health が 200 OK を返すことを検証する。
func TestHealthEndpoint(t *testing.T) {
	r := newRouter(nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

// TestRunGracefulShutdown は run がシグナル受信時にグレースフルシャットダウンすることを検証する。
func TestRunGracefulShutdown(t *testing.T) {
	saveAndRestore(t)

	var buf bytes.Buffer
	stdout = &buf
	addr = ":0"
	shutdownTimeout = 1 * time.Second

	sigCh := make(chan os.Signal, 1)
	signalNotify = func(c chan<- os.Signal, _ ...os.Signal) {
		go func() {
			sig := <-sigCh
			c <- sig
		}()
	}

	done := make(chan error, 1)
	go func() {
		done <- run(noopInit)
	}()

	time.Sleep(100 * time.Millisecond)
	sigCh <- os.Interrupt

	err := <-done
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if !strings.Contains(buf.String(), "broker listening on") {
		t.Errorf("expected listening message, got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "shutting down") {
		t.Errorf("expected shutdown message, got %q", buf.String())
	}
}

// TestRunListenError は run がリッスン失敗時にエラーを返すことを検証する。
func TestRunListenError(t *testing.T) {
	saveAndRestore(t)

	stdout = io.Discard
	shutdownTimeout = 1 * time.Second

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	addr = srv.Listener.Addr().String()

	signalNotify = func(c chan<- os.Signal, _ ...os.Signal) {}

	err := run(noopInit)
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// TestMainSuccess は main がサーバー起動後シグナルで正常終了することを検証する。
func TestMainSuccess(t *testing.T) {
	saveAndRestore(t)
	setDynamoEnv(t)

	stdout = io.Discard
	addr = ":0"
	shutdownTimeout = 1 * time.Second

	sigCh := make(chan os.Signal, 1)
	signalNotify = func(c chan<- os.Signal, _ ...os.Signal) {
		go func() {
			sig := <-sigCh
			c <- sig
		}()
	}

	done := make(chan struct{})
	go func() {
		main()
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	sigCh <- os.Interrupt
	<-done
}

// TestRunInitHandlerError は init 関数がエラーを返す場合に run がエラーを返すことを検証する。
func TestRunInitHandlerError(t *testing.T) {
	saveAndRestore(t)

	stdout = io.Discard
	errInit := func() (*handler.Handler, io.Closer, error) { return nil, nil, errors.New("init failed") }

	err := run(errInit)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "init handler") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "init handler")
	}
}

// TestDefaultInitHandler は defaultInitHandler が全環境変数指定時に Handler と Closer を返すことを検証する。
func TestDefaultInitHandler(t *testing.T) {
	setDynamoEnv(t)

	h, closer, err := defaultInitHandler()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if closer == nil {
		t.Fatal("expected non-nil closer")
	}
	if err := closer.Close(); err != nil {
		t.Errorf("closer.Close: %v", err)
	}
}

type fakeNoIdleService struct{}

func (fakeNoIdleService) CloseSession(context.Context, string) error { return nil }
func (fakeNoIdleService) ResolveSession(context.Context, string) (*service.ResolveResult, error) {
	return nil, store.ErrNoIdleRunner
}
func (fakeNoIdleService) LookupSession(context.Context, string) (*service.LookupResult, error) {
	return nil, store.ErrNotFound
}
func (fakeNoIdleService) RegisterRunner(context.Context, string, string) error { return nil }
func (fakeNoIdleService) DeregisterRunner(context.Context, string) error       { return nil }
func (fakeNoIdleService) ListBusyRunners(context.Context) ([]model.Runner, error) {
	return nil, nil
}

// TestDefaultInitHandler_FallbackSignal は BUNSHIN_STACKS から自スタックを除いた fallback を X-Fallback-Stack で返すことを検証する。
func TestDefaultInitHandler_FallbackSignal(t *testing.T) {
	origNewBrokerService := newBrokerService
	t.Cleanup(func() { newBrokerService = origNewBrokerService })
	newBrokerService = func(_ store.Repository, stack string, checker healthcheck.Checker) service.Service {
		if stack != "ap-northeast-1" {
			t.Errorf("stack = %q, want %q", stack, "ap-northeast-1")
		}
		if checker == nil {
			t.Error("checker is nil")
		}
		return fakeNoIdleService{}
	}

	setDynamoEnv(t)

	h, closer, err := defaultInitHandler()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = closer.Close() })

	req := httptest.NewRequest(http.MethodGet, "/resolve/session", nil)
	rec := httptest.NewRecorder()
	newRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if got := rec.Header().Get("X-Fallback-Stack"); got != "ap-northeast-3" {
		t.Errorf("X-Fallback-Stack = %q, want %q", got, "ap-northeast-3")
	}
	if got := rec.Header().Get("X-Fallback-Remaining"); got != "" {
		t.Errorf("X-Fallback-Remaining = %q, want empty", got)
	}
}

// TestDefaultInitHandler_StackError は config.NewStackFromEnv がエラーを返す場合に defaultInitHandler が伝播することを検証する。
func TestDefaultInitHandler_StackError(t *testing.T) {
	t.Setenv("STACK_NAME", "")

	_, _, err := defaultInitHandler()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "STACK_NAME") {
		t.Errorf("error = %q, want to propagate stack error", err.Error())
	}
}

// TestDefaultInitHandler_RunnerAPIPortError は RUNNER_API_PORT 未設定の場合に defaultInitHandler がエラーを伝播することを検証する。
func TestDefaultInitHandler_RunnerAPIPortError(t *testing.T) {
	t.Setenv("STACK_NAME", "ap-northeast-1")
	t.Setenv("BUNSHIN_STACKS", "ap-northeast-1")
	t.Setenv("RUNNER_API_PORT", "")

	_, _, err := defaultInitHandler()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "RUNNER_API_PORT") {
		t.Errorf("error = %q, want to propagate runner api port error", err.Error())
	}
}

// TestDefaultInitHandler_RepositoryError は config.NewRepositoryFromEnv がエラーを返す場合に defaultInitHandler が伝播することを検証する。
func TestDefaultInitHandler_RepositoryError(t *testing.T) {
	t.Setenv("STACK_NAME", "ap-northeast-1")
	t.Setenv("BUNSHIN_STACKS", "ap-northeast-1")
	t.Setenv("RUNNER_API_PORT", "3000")
	t.Setenv("BUNSHIN_STORE", "")

	_, _, err := defaultInitHandler()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "BUNSHIN_STORE") {
		t.Errorf("error = %q, want to propagate repository error", err.Error())
	}
}

// TestNewRouter_WithHandler は handler が non-nil の場合に全ルートが登録されることを検証する。
func TestNewRouter_WithHandler(t *testing.T) {
	setDynamoEnv(t)

	h, closer, err := defaultInitHandler()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = closer.Close() })

	r := newRouter(h)

	routes := r.Routes()
	expected := map[string]string{
		"GET /health":                        "",
		"DELETE /sessions/:sessionId":        "",
		"GET /resolve/session":               "",
		"GET /resolve/app":                   "",
		"POST /internal/runners/register":    "",
		"DELETE /internal/runners/:runnerId": "",
	}
	for _, route := range routes {
		key := route.Method + " " + route.Path
		delete(expected, key)
	}
	for key := range expected {
		t.Errorf("missing route: %s", key)
	}
}

// TestMainServerError は main がサーバー起動失敗時に fatalf を呼ぶことを検証する。
func TestMainServerError(t *testing.T) {
	saveAndRestore(t)
	setDynamoEnv(t)

	stdout = io.Discard
	shutdownTimeout = 1 * time.Second

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	addr = srv.Listener.Addr().String()

	signalNotify = func(c chan<- os.Signal, _ ...os.Signal) {}

	var got string
	fatalf = func(format string, args ...any) {
		got = fmt.Sprintf(format, args...)
	}

	main()

	if !strings.Contains(got, "server error") {
		t.Errorf("expected fatalf called with error, got %q", got)
	}
}
