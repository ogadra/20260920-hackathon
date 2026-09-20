// Package service はブローカーのビジネスロジックのテストを提供する。
package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ogadra/bunshin/broker/model"
	"github.com/ogadra/bunshin/broker/store"
)

// errorReader は常にエラーを返す io.Reader。
type errorReader struct{}

// Read は常にエラーを返す。
func (e *errorReader) Read(_ []byte) (int, error) {
	return 0, errors.New("rand read error")
}

// mockRepository は store.Repository のモック実装。
type mockRepository struct {
	registerFn        func(ctx context.Context, runnerID, privateHost string) error
	acquireIdleFn     func(ctx context.Context, sessionID string) (*model.Runner, error)
	listBusyRunnersFn func(ctx context.Context) ([]model.Runner, error)
	findBySessionIDFn func(ctx context.Context, sessionID string) (*model.Runner, error)
	findByIDFn        func(ctx context.Context, runnerID string) (*model.Runner, error)
	deleteFn          func(ctx context.Context, runnerID string) error
}

// Register はモック Register を呼び出す。
func (m *mockRepository) Register(ctx context.Context, runnerID, privateHost string) error {
	return m.registerFn(ctx, runnerID, privateHost)
}

// AcquireIdle はモック AcquireIdle を呼び出す。
func (m *mockRepository) AcquireIdle(ctx context.Context, sessionID string) (*model.Runner, error) {
	return m.acquireIdleFn(ctx, sessionID)
}

// ListBusyRunners はモック ListBusyRunners を呼び出す。
func (m *mockRepository) ListBusyRunners(ctx context.Context) ([]model.Runner, error) {
	return m.listBusyRunnersFn(ctx)
}

// FindBySessionID はモック FindBySessionID を呼び出す。
func (m *mockRepository) FindBySessionID(ctx context.Context, sessionID string) (*model.Runner, error) {
	return m.findBySessionIDFn(ctx, sessionID)
}

// FindByID はモック FindByID を呼び出す。
func (m *mockRepository) FindByID(ctx context.Context, runnerID string) (*model.Runner, error) {
	return m.findByIDFn(ctx, runnerID)
}

// Delete はモック Delete を呼び出す。
func (m *mockRepository) Delete(ctx context.Context, runnerID string) error {
	return m.deleteFn(ctx, runnerID)
}

// mockChecker は healthcheck.Checker のモック実装。
type mockChecker struct {
	checkFn func(ctx context.Context, privateHost string) error
}

// Check はモック Check を呼び出す。
func (m *mockChecker) Check(ctx context.Context, privateHost string) error {
	return m.checkFn(ctx, privateHost)
}

// healthyChecker は常に健全を返す checker を注入する Option。
func healthyChecker() Option {
	return WithChecker(&mockChecker{checkFn: func(context.Context, string) error { return nil }})
}

// noCallChecker は Check が呼ばれた瞬間に t.Fatal を起こす checker を注入する Option。
// healthcheck を行わない契約 (LookupSession 等) を回帰的に守るために使う。
func noCallChecker(t *testing.T) Option {
	t.Helper()
	return WithChecker(&mockChecker{checkFn: func(context.Context, string) error {
		t.Fatal("checker.Check must not be called")
		return nil
	}})
}

// suppressLog はテスト中のログ出力を抑制し、テスト終了時に復元する。
func suppressLog(t *testing.T) {
	t.Helper()
	orig := logPrintf
	t.Cleanup(func() { logPrintf = orig })
	logPrintf = func(string, ...any) {}
}

// TestBrokerService_ImplementsService は BrokerService が Service インターフェースを満たすことを検証する。
func TestBrokerService_ImplementsService(t *testing.T) {
	t.Parallel()
	var _ Service = (*BrokerService)(nil)
}

// TestNewBrokerService はコンストラクタの動作を検証する。
func TestNewBrokerService(t *testing.T) {
	t.Parallel()
	repo := &mockRepository{}
	svc := NewBrokerService(repo, "ap-northeast-1", healthyChecker())
	if svc.repo != repo {
		t.Error("repo mismatch")
	}
	if svc.sessionFn == nil {
		t.Error("sessionFn is nil")
	}
	if svc.stackPrefix != "ap-northeast-1" {
		t.Errorf("stackPrefix = %q, want %q", svc.stackPrefix, "ap-northeast-1")
	}
}

// TestNewBrokerService_EmptyStackPrefixPanics は stackPrefix が空文字列のとき panic することを検証する。
func TestNewBrokerService_EmptyStackPrefixPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for empty stackPrefix")
		}
	}()
	NewBrokerService(&mockRepository{}, "")
}

// TestNewBrokerService_WithSessionFn は WithSessionFn オプションで sessionFn が差し替わることを検証する。
func TestNewBrokerService_WithSessionFn(t *testing.T) {
	t.Parallel()
	called := false
	fn := func() (string, error) {
		called = true
		return "test-session", nil
	}
	svc := NewBrokerService(&mockRepository{}, "ap-northeast-1", WithSessionFn(fn), healthyChecker())
	got, err := svc.sessionFn()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "test-session" {
		t.Errorf("sessionFn() = %q, want %q", got, "test-session")
	}
	if !called {
		t.Error("custom sessionFn was not called")
	}
}

// TestWithSessionFn_Nil は WithSessionFn に nil を渡してもデフォルト関数が維持されることを検証する。
func TestWithSessionFn_Nil(t *testing.T) {
	t.Parallel()
	svc := NewBrokerService(&mockRepository{}, "ap-northeast-1", WithSessionFn(nil), healthyChecker())
	if svc.sessionFn == nil {
		t.Fatal("sessionFn should not be nil when WithSessionFn(nil) is passed")
	}
	id, err := svc.sessionFn()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(id) != 32 {
		t.Errorf("len(id) = %d, want 32", len(id))
	}
}

// TestWithChecker は WithChecker オプションで checker が差し替わることを検証する。
func TestWithChecker(t *testing.T) {
	t.Parallel()
	checker := &mockChecker{}
	svc := NewBrokerService(&mockRepository{}, "ap-northeast-1", WithChecker(checker))
	if svc.checker != checker {
		t.Error("checker mismatch")
	}
}

// TestNewBrokerService_NilCheckerPanics は checker 未設定のとき panic することを検証する。
func TestNewBrokerService_NilCheckerPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil checker")
		}
	}()
	NewBrokerService(&mockRepository{}, "ap-northeast-1")
}

// TestDefaultSessionFn はデフォルトセッション ID 生成関数が 32 文字の hex 文字列を返すことを検証する。
func TestDefaultSessionFn(t *testing.T) {
	t.Parallel()
	id, err := defaultSessionFn()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(id) != 32 {
		t.Errorf("len(id) = %d, want 32", len(id))
	}
}

// TestDefaultSessionFn_Unique はデフォルトセッション ID 生成関数が一意の値を返すことを検証する。
func TestDefaultSessionFn_Unique(t *testing.T) {
	t.Parallel()
	id1, err := defaultSessionFn()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	id2, err := defaultSessionFn()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id1 == id2 {
		t.Error("expected unique session IDs")
	}
}

// TestDefaultSessionFn_RandReadError は randReader がエラーを返す場合に defaultSessionFn がエラーを返すことを検証する。
func TestDefaultSessionFn_RandReadError(t *testing.T) {
	orig := randReader
	t.Cleanup(func() { randReader = orig })
	randReader = &errorReader{}

	_, err := defaultSessionFn()
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestCreateSession_Success はセッション作成の成功ケースを検証する。
func TestCreateSession_Success(t *testing.T) {
	t.Parallel()
	repo := &mockRepository{
		acquireIdleFn: func(_ context.Context, sessionID string) (*model.Runner, error) {
			if sessionID != "ap-northeast-1_fixed-session" {
				t.Errorf("sessionID = %q, want %q", sessionID, "ap-northeast-1_fixed-session")
			}
			return &model.Runner{
				RunnerID:         "r1",
				CurrentSessionID: sessionID,
				PrivateHost:      "10.0.0.1",
			}, nil
		},
	}
	svc := NewBrokerService(repo, "ap-northeast-1", WithSessionFn(func() (string, error) {
		return "fixed-session", nil
	}), healthyChecker())

	result, err := svc.createSession(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SessionID != "ap-northeast-1_fixed-session" {
		t.Errorf("SessionID = %q, want %q", result.SessionID, "ap-northeast-1_fixed-session")
	}
	if result.SessionHex != "fixed-session" {
		t.Errorf("SessionHex = %q, want %q", result.SessionHex, "fixed-session")
	}
	if result.Runner.RunnerID != "r1" {
		t.Errorf("RunnerID = %q, want %q", result.Runner.RunnerID, "r1")
	}
}

// TestCreateSession_StackPrefix は session ID へ <stack>_<id> 形式で発行スタックが同梱されることを検証する。
func TestCreateSession_StackPrefix(t *testing.T) {
	t.Parallel()
	repo := &mockRepository{
		acquireIdleFn: func(_ context.Context, sessionID string) (*model.Runner, error) {
			if sessionID != "ap-northeast-3_fixed-session" {
				t.Errorf("sessionID = %q, want %q", sessionID, "ap-northeast-3_fixed-session")
			}
			return &model.Runner{RunnerID: "r1", CurrentSessionID: sessionID, PrivateHost: "10.0.0.1"}, nil
		},
	}
	svc := NewBrokerService(repo, "ap-northeast-3", WithSessionFn(func() (string, error) { return "fixed-session", nil }), healthyChecker())

	result, err := svc.createSession(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SessionID != "ap-northeast-3_fixed-session" {
		t.Errorf("SessionID = %q, want %q", result.SessionID, "ap-northeast-3_fixed-session")
	}
	if result.SessionHex != "fixed-session" {
		t.Errorf("SessionHex = %q, want %q", result.SessionHex, "fixed-session")
	}
}

// TestCreateSession_SessionFnError はセッション ID 生成のエラーを検証する。
func TestCreateSession_SessionFnError(t *testing.T) {
	t.Parallel()
	svc := NewBrokerService(&mockRepository{}, "ap-northeast-1", WithSessionFn(func() (string, error) {
		return "", errors.New("rand error")
	}), healthyChecker())

	_, err := svc.createSession(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestCreateSession_AcquireIdleError は AcquireIdle の枯渇エラーが伝搬されることを検証する。
func TestCreateSession_AcquireIdleError(t *testing.T) {
	t.Parallel()
	repo := &mockRepository{
		acquireIdleFn: func(_ context.Context, _ string) (*model.Runner, error) {
			return nil, store.ErrNoIdleRunner
		},
	}
	svc := NewBrokerService(repo, "ap-northeast-1", WithSessionFn(func() (string, error) {
		return "sess-1", nil
	}), healthyChecker())

	_, err := svc.createSession(context.Background())
	if !errors.Is(err, store.ErrNoIdleRunner) {
		t.Fatalf("expected ErrNoIdleRunner, got: %v", err)
	}
}

// TestCreateSession_AcquireIdleInternalError は AcquireIdle の内部エラーが即座に伝搬されることを検証する。
func TestCreateSession_AcquireIdleInternalError(t *testing.T) {
	t.Parallel()
	repo := &mockRepository{
		acquireIdleFn: func(_ context.Context, _ string) (*model.Runner, error) {
			return nil, errors.New("db error")
		},
	}
	svc := NewBrokerService(repo, "ap-northeast-1", WithSessionFn(func() (string, error) {
		return "sess-1", nil
	}), healthyChecker())

	_, err := svc.createSession(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "db error" {
		t.Errorf("error = %q, want %q", err.Error(), "db error")
	}
}

// TestCloseSession_Success はセッション終了の成功ケースを検証する。
func TestCloseSession_Success(t *testing.T) {
	t.Parallel()
	deleteCalled := false
	repo := &mockRepository{
		findBySessionIDFn: func(_ context.Context, sessionID string) (*model.Runner, error) {
			return &model.Runner{RunnerID: "r1", CurrentSessionID: sessionID}, nil
		},
		deleteFn: func(_ context.Context, runnerID string) error {
			deleteCalled = true
			if runnerID != "r1" {
				t.Errorf("runnerID = %q, want %q", runnerID, "r1")
			}
			return nil
		},
	}
	svc := NewBrokerService(repo, "ap-northeast-1", healthyChecker())

	err := svc.CloseSession(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleteCalled {
		t.Error("Delete was not called")
	}
}

// TestCloseSession_FindError は FindBySessionID のエラーを検証する。
func TestCloseSession_FindError(t *testing.T) {
	t.Parallel()
	repo := &mockRepository{
		findBySessionIDFn: func(_ context.Context, _ string) (*model.Runner, error) {
			return nil, store.ErrNotFound
		},
	}
	svc := NewBrokerService(repo, "ap-northeast-1", healthyChecker())

	err := svc.CloseSession(context.Background(), "sess-missing")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

// TestCloseSession_DeleteError は Delete のエラーを検証する。
func TestCloseSession_DeleteError(t *testing.T) {
	t.Parallel()
	repo := &mockRepository{
		findBySessionIDFn: func(_ context.Context, _ string) (*model.Runner, error) {
			return &model.Runner{RunnerID: "r1"}, nil
		},
		deleteFn: func(_ context.Context, _ string) error {
			return errors.New("delete error")
		},
	}
	svc := NewBrokerService(repo, "ap-northeast-1", healthyChecker())

	err := svc.CloseSession(context.Background(), "sess-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestRegisterRunner_Success は runner 登録の成功ケースを検証する。
func TestRegisterRunner_Success(t *testing.T) {
	t.Parallel()
	repo := &mockRepository{
		registerFn: func(_ context.Context, runnerID, privateHost string) error {
			if runnerID != "r1" {
				t.Errorf("runnerID = %q, want %q", runnerID, "r1")
			}
			if privateHost != "10.0.0.1" {
				t.Errorf("privateHost = %q, want %q", privateHost, "10.0.0.1")
			}
			return nil
		},
	}
	svc := NewBrokerService(repo, "ap-northeast-1", healthyChecker())

	err := svc.RegisterRunner(context.Background(), "r1", "10.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestRegisterRunner_Error は Register のエラーを検証する。
func TestRegisterRunner_Error(t *testing.T) {
	t.Parallel()
	repo := &mockRepository{
		registerFn: func(_ context.Context, _, _ string) error {
			return errors.New("register error")
		},
	}
	svc := NewBrokerService(repo, "ap-northeast-1", healthyChecker())

	err := svc.RegisterRunner(context.Background(), "r1", "10.0.0.1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestDeregisterRunner_Success は runner 削除の成功ケースを検証する。
func TestDeregisterRunner_Success(t *testing.T) {
	t.Parallel()
	repo := &mockRepository{
		deleteFn: func(_ context.Context, runnerID string) error {
			if runnerID != "r1" {
				t.Errorf("runnerID = %q, want %q", runnerID, "r1")
			}
			return nil
		},
	}
	svc := NewBrokerService(repo, "ap-northeast-1", healthyChecker())

	err := svc.DeregisterRunner(context.Background(), "r1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestDeregisterRunner_Error は Delete のエラーを検証する。
func TestDeregisterRunner_Error(t *testing.T) {
	t.Parallel()
	repo := &mockRepository{
		deleteFn: func(_ context.Context, _ string) error {
			return errors.New("delete error")
		},
	}
	svc := NewBrokerService(repo, "ap-northeast-1", healthyChecker())

	err := svc.DeregisterRunner(context.Background(), "r1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestListBusyRunners_Passthrough は repository の返却値をそのまま返すことを検証する。
func TestListBusyRunners_Passthrough(t *testing.T) {
	t.Parallel()
	want := []model.Runner{
		{RunnerID: "r1", State: model.StateBusy, CurrentSessionID: "sess-1"},
	}
	repo := &mockRepository{
		listBusyRunnersFn: func(context.Context) ([]model.Runner, error) {
			return want, nil
		},
	}
	svc := NewBrokerService(repo, "ap-northeast-1", healthyChecker())

	got, err := svc.ListBusyRunners(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].RunnerID != "r1" {
		t.Errorf("got = %v, want %v", got, want)
	}
}

// TestListBusyRunners_Error は repository のエラーが伝搬されることを検証する。
func TestListBusyRunners_Error(t *testing.T) {
	t.Parallel()
	repo := &mockRepository{
		listBusyRunnersFn: func(context.Context) ([]model.Runner, error) {
			return nil, errors.New("list error")
		},
	}
	svc := NewBrokerService(repo, "ap-northeast-1", healthyChecker())

	_, err := svc.ListBusyRunners(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestResolveSession_ExistingSession は既存セッションの解決を検証する。
func TestResolveSession_ExistingSession(t *testing.T) {
	t.Parallel()
	repo := &mockRepository{
		findBySessionIDFn: func(_ context.Context, _ string) (*model.Runner, error) {
			return &model.Runner{RunnerID: "r1", PrivateHost: "10.0.0.1"}, nil
		},
	}
	svc := NewBrokerService(repo, "ap-northeast-1", healthyChecker())

	result, err := svc.ResolveSession(context.Background(), "ap-northeast-1_sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Created {
		t.Error("expected Created=false for existing session")
	}
	if result.SessionID != "ap-northeast-1_sess-1" {
		t.Errorf("SessionID = %q, want %q", result.SessionID, "ap-northeast-1_sess-1")
	}
	if result.SessionHex != "sess-1" {
		t.Errorf("SessionHex = %q, want %q", result.SessionHex, "sess-1")
	}
	if result.RunnerHost != "10.0.0.1" {
		t.Errorf("RunnerHost = %q, want %q", result.RunnerHost, "10.0.0.1")
	}
}

// TestResolveSession_NotFound_CreatesNew はセッションが見つからない場合に新規作成することを検証する。
func TestResolveSession_NotFound_CreatesNew(t *testing.T) {
	t.Parallel()
	repo := &mockRepository{
		findBySessionIDFn: func(_ context.Context, _ string) (*model.Runner, error) {
			return nil, store.ErrNotFound
		},
		acquireIdleFn: func(_ context.Context, sessionID string) (*model.Runner, error) {
			return &model.Runner{
				RunnerID:         "r2",
				CurrentSessionID: sessionID,
				PrivateHost:      "10.0.0.2",
			}, nil
		},
	}
	svc := NewBrokerService(repo, "ap-northeast-1", WithSessionFn(func() (string, error) {
		return "new-session", nil
	}), healthyChecker())

	result, err := svc.ResolveSession(context.Background(), "sess-missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Created {
		t.Error("expected Created=true for new session")
	}
	if result.SessionID != "ap-northeast-1_new-session" {
		t.Errorf("SessionID = %q, want %q", result.SessionID, "ap-northeast-1_new-session")
	}
	if result.SessionHex != "new-session" {
		t.Errorf("SessionHex = %q, want %q", result.SessionHex, "new-session")
	}
	if result.RunnerHost != "10.0.0.2" {
		t.Errorf("RunnerHost = %q, want %q", result.RunnerHost, "10.0.0.2")
	}
}

// TestResolveSession_FindInternalError は FindBySessionID の内部エラーを検証する。
func TestResolveSession_FindInternalError(t *testing.T) {
	t.Parallel()
	repo := &mockRepository{
		findBySessionIDFn: func(_ context.Context, _ string) (*model.Runner, error) {
			return nil, errors.New("db error")
		},
	}
	svc := NewBrokerService(repo, "ap-northeast-1", healthyChecker())

	_, err := svc.ResolveSession(context.Background(), "sess-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestResolveSession_CreateError は新規作成時のエラーを検証する。
func TestResolveSession_CreateError(t *testing.T) {
	t.Parallel()
	repo := &mockRepository{
		findBySessionIDFn: func(_ context.Context, _ string) (*model.Runner, error) {
			return nil, store.ErrNotFound
		},
		acquireIdleFn: func(_ context.Context, _ string) (*model.Runner, error) {
			return nil, store.ErrNoIdleRunner
		},
	}
	svc := NewBrokerService(repo, "ap-northeast-1", WithSessionFn(func() (string, error) {
		return "new-session", nil
	}), healthyChecker())

	_, err := svc.ResolveSession(context.Background(), "sess-missing")
	if !errors.Is(err, store.ErrNoIdleRunner) {
		t.Fatalf("expected ErrNoIdleRunner, got: %v", err)
	}
}

// TestResolveSession_EmptySessionID は空のセッション ID で FindBySessionID をスキップして新規作成されることを検証する。
func TestResolveSession_EmptySessionID(t *testing.T) {
	t.Parallel()
	repo := &mockRepository{
		findBySessionIDFn: func(_ context.Context, _ string) (*model.Runner, error) {
			t.Fatal("FindBySessionID should not be called for empty session ID")
			return nil, store.ErrNotFound
		},
		acquireIdleFn: func(_ context.Context, sessionID string) (*model.Runner, error) {
			return &model.Runner{
				RunnerID:         "r1",
				CurrentSessionID: sessionID,
				PrivateHost:      "10.0.0.1",
			}, nil
		},
	}
	svc := NewBrokerService(repo, "ap-northeast-1", WithSessionFn(func() (string, error) {
		return "new-session", nil
	}), healthyChecker())

	result, err := svc.ResolveSession(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Created {
		t.Error("expected Created=true")
	}
}

// TestResolveSession_ExistingHealthy は既存セッションの runner が健全な場合にそのまま返すことを検証する。
func TestResolveSession_ExistingHealthy(t *testing.T) {
	t.Parallel()
	repo := &mockRepository{
		findBySessionIDFn: func(_ context.Context, _ string) (*model.Runner, error) {
			return &model.Runner{RunnerID: "r1", PrivateHost: "10.0.0.1"}, nil
		},
	}
	checker := &mockChecker{checkFn: func(_ context.Context, _ string) error { return nil }}
	svc := NewBrokerService(repo, "ap-northeast-1", WithChecker(checker))

	result, err := svc.ResolveSession(context.Background(), "ap-northeast-1_sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Created {
		t.Error("expected Created=false")
	}
	if result.Reassigned {
		t.Error("expected Reassigned=false")
	}
	if result.SessionID != "ap-northeast-1_sess-1" {
		t.Errorf("SessionID = %q, want %q", result.SessionID, "ap-northeast-1_sess-1")
	}
	if result.SessionHex != "sess-1" {
		t.Errorf("SessionHex = %q, want %q", result.SessionHex, "sess-1")
	}
}

// TestResolveSession_StoredIDWithoutPrefixはstoreから見つかったsession IDが
// prefixを欠く場合にデータ不整合としてエラーを返すことを検証する。
func TestResolveSession_StoredIDWithoutPrefix(t *testing.T) {
	t.Parallel()
	repo := &mockRepository{
		findBySessionIDFn: func(_ context.Context, _ string) (*model.Runner, error) {
			return &model.Runner{RunnerID: "r1", PrivateHost: "10.0.0.1"}, nil
		},
	}
	svc := NewBrokerService(repo, "ap-northeast-1", healthyChecker())

	_, err := svc.ResolveSession(context.Background(), "noprefix")
	if err == nil {
		t.Fatal("expected error for session id without stack prefix")
	}
	if !strings.Contains(err.Error(), "missing stack prefix") {
		t.Errorf("err = %v, want to contain %q", err, "missing stack prefix")
	}
}

// TestResolveSession_ExistingUnhealthy_Reassigned は既存 runner が不健全な場合に再割当てされることを検証する。
func TestResolveSession_ExistingUnhealthy_Reassigned(t *testing.T) {
	suppressLog(t)
	deletedRunnerIDs := []string{}
	repo := &mockRepository{
		findBySessionIDFn: func(_ context.Context, _ string) (*model.Runner, error) {
			return &model.Runner{RunnerID: "r-dead", PrivateHost: "10.0.0.1"}, nil
		},
		acquireIdleFn: func(_ context.Context, sessionID string) (*model.Runner, error) {
			return &model.Runner{
				RunnerID:         "r-new",
				CurrentSessionID: sessionID,
				PrivateHost:      "10.0.0.2",
			}, nil
		},
		deleteFn: func(_ context.Context, runnerID string) error {
			deletedRunnerIDs = append(deletedRunnerIDs, runnerID)
			return nil
		},
	}
	checker := &mockChecker{checkFn: func(_ context.Context, url string) error {
		if url == "10.0.0.1" {
			return errors.New("unreachable")
		}
		return nil
	}}
	svc := NewBrokerService(repo, "ap-northeast-1", WithChecker(checker), WithSessionFn(func() (string, error) {
		return "new-session", nil
	}))

	result, err := svc.ResolveSession(context.Background(), "sess-old")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Created {
		t.Error("expected Created=true")
	}
	if !result.Reassigned {
		t.Error("expected Reassigned=true")
	}
	if result.RunnerHost != "10.0.0.2" {
		t.Errorf("RunnerHost = %q, want %q", result.RunnerHost, "10.0.0.2")
	}
	if len(deletedRunnerIDs) == 0 || deletedRunnerIDs[0] != "r-dead" {
		t.Errorf("expected dead runner to be deleted, got %v", deletedRunnerIDs)
	}
}

// TestCreateSession_RetryOnUnhealthy は健康チェック失敗時に AcquireIdle を再度呼び出し健全な runner を返すことを検証する。
func TestCreateSession_RetryOnUnhealthy(t *testing.T) {
	suppressLog(t)
	acquireCount := 0
	deleteCount := 0
	repo := &mockRepository{
		acquireIdleFn: func(_ context.Context, sessionID string) (*model.Runner, error) {
			acquireCount++
			if acquireCount == 3 {
				return &model.Runner{
					RunnerID:         "r-healthy",
					CurrentSessionID: sessionID,
					PrivateHost:      "10.0.0.3",
				}, nil
			}
			return &model.Runner{
				RunnerID:         "r-dead-" + string(rune('0'+acquireCount)),
				CurrentSessionID: sessionID,
				PrivateHost:      "10.0.0.1",
			}, nil
		},
		deleteFn: func(_ context.Context, _ string) error {
			deleteCount++
			return nil
		},
	}
	checker := &mockChecker{checkFn: func(_ context.Context, url string) error {
		if url == "10.0.0.1" {
			return errors.New("unreachable")
		}
		return nil
	}}
	svc := NewBrokerService(repo, "ap-northeast-1", WithChecker(checker), WithSessionFn(func() (string, error) {
		return "new-session", nil
	}))

	result, err := svc.createSession(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Runner.RunnerID != "r-healthy" {
		t.Errorf("RunnerID = %q, want %q", result.Runner.RunnerID, "r-healthy")
	}
	if acquireCount != 3 {
		t.Errorf("acquireCount = %d, want 3", acquireCount)
	}
	if deleteCount != 2 {
		t.Errorf("deleteCount = %d, want 2", deleteCount)
	}
}

// TestCreateSession_AllUnhealthyThenNoIdle は不健全な runner を削除して枯渇に至った場合に ErrNoIdleRunner を返すことを検証する。
func TestCreateSession_AllUnhealthyThenNoIdle(t *testing.T) {
	suppressLog(t)
	acquireCount := 0
	repo := &mockRepository{
		acquireIdleFn: func(_ context.Context, sessionID string) (*model.Runner, error) {
			acquireCount++
			if acquireCount > 2 {
				return nil, store.ErrNoIdleRunner
			}
			return &model.Runner{
				RunnerID:         "r-dead",
				CurrentSessionID: sessionID,
				PrivateHost:      "10.0.0.1",
			}, nil
		},
		deleteFn: func(_ context.Context, _ string) error { return nil },
	}
	checker := &mockChecker{checkFn: func(_ context.Context, _ string) error {
		return errors.New("unreachable")
	}}
	svc := NewBrokerService(repo, "ap-northeast-1", WithChecker(checker), WithSessionFn(func() (string, error) {
		return "new-session", nil
	}))

	_, err := svc.createSession(context.Background())
	if !errors.Is(err, store.ErrNoIdleRunner) {
		t.Fatalf("expected ErrNoIdleRunner, got: %v", err)
	}
}

// TestCreateSession_SessionFnError_WithChecker はチェッカー付きでセッション ID 生成エラーが伝搬されることを検証する。
func TestCreateSession_SessionFnError_WithChecker(t *testing.T) {
	t.Parallel()
	svc := NewBrokerService(&mockRepository{}, "ap-northeast-1", WithSessionFn(func() (string, error) {
		return "", errors.New("rand error")
	}), WithChecker(&mockChecker{checkFn: func(_ context.Context, _ string) error { return nil }}))

	_, err := svc.createSession(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestCreateSession_ContextCanceled はヘルスチェック中にコンテキストがキャンセルされた場合にランナーを削除せずエラーを返すことを検証する。
func TestCreateSession_ContextCanceled(t *testing.T) {
	t.Parallel()
	repo := &mockRepository{
		acquireIdleFn: func(_ context.Context, sessionID string) (*model.Runner, error) {
			return &model.Runner{
				RunnerID:         "r1",
				CurrentSessionID: sessionID,
				PrivateHost:      "10.0.0.1",
			}, nil
		},
		deleteFn: func(_ context.Context, _ string) error {
			t.Fatal("Delete should not be called on context cancel")
			return nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	checker := &mockChecker{checkFn: func(_ context.Context, _ string) error {
		return context.Canceled
	}}
	svc := NewBrokerService(repo, "ap-northeast-1", WithChecker(checker), WithSessionFn(func() (string, error) {
		return "sess-1", nil
	}))

	_, err := svc.createSession(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

// TestCreateSession_DeleteError はヘルスチェック失敗後の Delete エラーが伝搬されることを検証する。
func TestCreateSession_DeleteError(t *testing.T) {
	suppressLog(t)
	repo := &mockRepository{
		acquireIdleFn: func(_ context.Context, sessionID string) (*model.Runner, error) {
			return &model.Runner{
				RunnerID:         "r1",
				CurrentSessionID: sessionID,
				PrivateHost:      "10.0.0.1",
			}, nil
		},
		deleteFn: func(_ context.Context, _ string) error {
			return errors.New("delete failed")
		},
	}
	checker := &mockChecker{checkFn: func(_ context.Context, _ string) error {
		return errors.New("unreachable")
	}}
	svc := NewBrokerService(repo, "ap-northeast-1", WithChecker(checker), WithSessionFn(func() (string, error) {
		return "sess-1", nil
	}))

	_, err := svc.createSession(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestResolveSession_ContextCanceled_ExistingRunner は既存 runner のヘルスチェック中にコンテキストがキャンセルされた場合にエラーを返すことを検証する。
func TestResolveSession_ContextCanceled_ExistingRunner(t *testing.T) {
	t.Parallel()
	repo := &mockRepository{
		findBySessionIDFn: func(_ context.Context, _ string) (*model.Runner, error) {
			return &model.Runner{RunnerID: "r1", PrivateHost: "10.0.0.1"}, nil
		},
		deleteFn: func(_ context.Context, _ string) error {
			t.Fatal("Delete should not be called on context cancel")
			return nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	checker := &mockChecker{checkFn: func(_ context.Context, _ string) error {
		return context.Canceled
	}}
	svc := NewBrokerService(repo, "ap-northeast-1", WithChecker(checker))

	_, err := svc.ResolveSession(ctx, "sess-1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

// TestResolveSession_DeleteError_ExistingRunner は既存 runner の Delete エラーが伝搬されることを検証する。
func TestResolveSession_DeleteError_ExistingRunner(t *testing.T) {
	suppressLog(t)
	repo := &mockRepository{
		findBySessionIDFn: func(_ context.Context, _ string) (*model.Runner, error) {
			return &model.Runner{RunnerID: "r1", PrivateHost: "10.0.0.1"}, nil
		},
		deleteFn: func(_ context.Context, _ string) error {
			return errors.New("delete failed")
		},
	}
	checker := &mockChecker{checkFn: func(_ context.Context, _ string) error {
		return errors.New("unreachable")
	}}
	svc := NewBrokerService(repo, "ap-northeast-1", WithChecker(checker))

	_, err := svc.ResolveSession(context.Background(), "sess-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestLookupSession_Existing は hex を stackPrefix と結合して FindBySessionID を呼ぶことを検証する。
// 見つかった runner の PrivateHost を LookupResult に載せて返すことも検証する。
func TestLookupSession_Existing(t *testing.T) {
	var gotSessionID string
	repo := &mockRepository{
		findBySessionIDFn: func(_ context.Context, sessionID string) (*model.Runner, error) {
			gotSessionID = sessionID
			return &model.Runner{RunnerID: "r1", PrivateHost: "10.0.0.1"}, nil
		},
	}
	svc := NewBrokerService(repo, "ap-northeast-1", noCallChecker(t))

	result, err := svc.LookupSession(context.Background(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotSessionID != "ap-northeast-1_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Errorf("FindBySessionID sessionID = %q, want stackPrefix + \"_\" + hex", gotSessionID)
	}
	if result.RunnerHost != "10.0.0.1" {
		t.Errorf("RunnerHost = %q, want %q", result.RunnerHost, "10.0.0.1")
	}
}

// TestLookupSession_NotFound は store.ErrNotFound をそのまま透過することを検証する。
func TestLookupSession_NotFound(t *testing.T) {
	repo := &mockRepository{
		findBySessionIDFn: func(context.Context, string) (*model.Runner, error) {
			return nil, store.ErrNotFound
		},
	}
	svc := NewBrokerService(repo, "ap-northeast-1", noCallChecker(t))

	_, err := svc.LookupSession(context.Background(), "00112233445566778899aabbccddeeff")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want store.ErrNotFound", err)
	}
}

// TestLookupSession_RepoError は repository のその他エラーを透過することを検証する。
func TestLookupSession_RepoError(t *testing.T) {
	want := errors.New("boom")
	repo := &mockRepository{
		findBySessionIDFn: func(context.Context, string) (*model.Runner, error) {
			return nil, want
		},
	}
	svc := NewBrokerService(repo, "ap-northeast-1", noCallChecker(t))

	_, err := svc.LookupSession(context.Background(), "00112233445566778899aabbccddeeff")
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}
