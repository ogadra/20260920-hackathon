// Package store は Firestore Repository のテストを提供する。
package store

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/ogadra/bunshin/broker/model"
)

const (
	firestoreTestRunnerID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	firestoreTestStart    = "88888888888888888888888888888888"
)

// mockFirestoreClientAPI: 未セットの fn を呼ぶと nil deref で panic する。
type mockFirestoreClientAPI struct {
	createFn         func(ctx context.Context, runnerID string, data map[string]any) error
	getFn            func(ctx context.Context, runnerID string) (map[string]any, bool, error)
	deleteFn         func(ctx context.Context, runnerID string) error
	queryIdleRangeFn func(ctx context.Context, after, upTo string, limit int) ([]FirestoreDocSnapshot, error)
	iterBusyFn       func(ctx context.Context) FirestoreDocIter
	queryBySessionFn func(ctx context.Context, sessionID string) (string, map[string]any, bool, error)
	runTxFn          func(ctx context.Context, fn func(tx FirestoreTx) error) error
	closeFn          func() error
}

func (m *mockFirestoreClientAPI) Create(ctx context.Context, runnerID string, data map[string]any) error {
	return m.createFn(ctx, runnerID, data)
}

func (m *mockFirestoreClientAPI) Get(ctx context.Context, runnerID string) (map[string]any, bool, error) {
	return m.getFn(ctx, runnerID)
}

func (m *mockFirestoreClientAPI) Delete(ctx context.Context, runnerID string) error {
	return m.deleteFn(ctx, runnerID)
}

func (m *mockFirestoreClientAPI) QueryIdleRange(ctx context.Context, after, upTo string, limit int) ([]FirestoreDocSnapshot, error) {
	return m.queryIdleRangeFn(ctx, after, upTo, limit)
}

func (m *mockFirestoreClientAPI) IterBusy(ctx context.Context) FirestoreDocIter {
	return m.iterBusyFn(ctx)
}

func (m *mockFirestoreClientAPI) QueryBySession(ctx context.Context, sessionID string) (string, map[string]any, bool, error) {
	return m.queryBySessionFn(ctx, sessionID)
}

func (m *mockFirestoreClientAPI) RunTx(ctx context.Context, fn func(tx FirestoreTx) error) error {
	return m.runTxFn(ctx, fn)
}

func (m *mockFirestoreClientAPI) Close() error {
	if m.closeFn == nil {
		return nil
	}
	return m.closeFn()
}

type nextResult struct {
	id   string
	data map[string]any
	done bool
	err  error
}

type mockFirestoreDocIter struct {
	results     []nextResult
	nextIdx     int
	stopCounter *int
}

func (m *mockFirestoreDocIter) Next() (string, map[string]any, bool, error) {
	if m.nextIdx >= len(m.results) {
		return "", nil, true, nil
	}
	r := m.results[m.nextIdx]
	m.nextIdx++
	return r.id, r.data, r.done, r.err
}

func (m *mockFirestoreDocIter) Stop() {
	if m.stopCounter != nil {
		*m.stopCounter++
	}
}

type mockFirestoreTx struct {
	getFn    func(runnerID string) (map[string]any, bool, error)
	updateFn func(runnerID, field string, value any) error
}

func (m *mockFirestoreTx) Get(runnerID string) (map[string]any, bool, error) {
	return m.getFn(runnerID)
}

func (m *mockFirestoreTx) Update(runnerID, field string, value any) error {
	return m.updateFn(runnerID, field, value)
}

func TestFirestoreRepository_ImplementsRepository(t *testing.T) {
	t.Parallel()
	var _ Repository = (*FirestoreRepository)(nil)
}

func TestNewFirestoreRepositoryWithAPI(t *testing.T) {
	t.Parallel()
	api := &mockFirestoreClientAPI{}
	repo := NewFirestoreRepositoryWithAPI(api)
	if repo.api != api {
		t.Error("api mismatch")
	}
	if repo.randHexFn == nil {
		t.Error("randHexFn is nil")
	}
	if repo.logFn == nil {
		t.Error("logFn is nil")
	}
}

func TestRunnerDoc_ToModel_Idle(t *testing.T) {
	t.Parallel()
	doc := runnerDoc{RunnerID: "r1", PrivateHost: "10.0.0.1"}
	r := doc.toModel()
	if !r.IsIdle() {
		t.Errorf("expected idle, state = %q", r.State)
	}
	if r.PrivateHost != "10.0.0.1" {
		t.Errorf("PrivateHost = %q", r.PrivateHost)
	}
}

func TestRunnerDoc_ToModel_Busy(t *testing.T) {
	t.Parallel()
	doc := runnerDoc{RunnerID: "r1", PrivateHost: "10.0.0.1", CurrentSessionID: "sess-1"}
	r := doc.toModel()
	if !r.IsBusy() {
		t.Errorf("expected busy, state = %q", r.State)
	}
	if r.CurrentSessionID != "sess-1" {
		t.Errorf("CurrentSessionID = %q", r.CurrentSessionID)
	}
}

func TestSnapshotToDoc(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		data    map[string]any
		want    *runnerDoc
		wantErr bool
	}{
		{
			name: "idle (currentSessionId is nil)",
			data: map[string]any{FieldPrivateHost: "10.0.0.1", FieldCurrentSessionID: nil},
			want: &runnerDoc{RunnerID: "r1", PrivateHost: "10.0.0.1"},
		},
		{
			name: "busy",
			data: map[string]any{FieldPrivateHost: "10.0.0.1", FieldCurrentSessionID: "sess-1"},
			want: &runnerDoc{RunnerID: "r1", PrivateHost: "10.0.0.1", CurrentSessionID: "sess-1"},
		},
		{
			name:    "missing privateHost",
			data:    map[string]any{FieldCurrentSessionID: nil},
			wantErr: true,
		},
		{
			name:    "privateHost not string",
			data:    map[string]any{FieldPrivateHost: 42, FieldCurrentSessionID: nil},
			wantErr: true,
		},
		{
			name:    "missing currentSessionId",
			data:    map[string]any{FieldPrivateHost: "10.0.0.1"},
			wantErr: true,
		},
		{
			name:    "currentSessionId neither nil nor string",
			data:    map[string]any{FieldPrivateHost: "10.0.0.1", FieldCurrentSessionID: 42},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := snapshotToDoc("r1", tc.data)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestFirestoreRegister_InvalidRunnerID(t *testing.T) {
	t.Parallel()
	repo := NewFirestoreRepositoryWithAPI(&mockFirestoreClientAPI{
		createFn: func(context.Context, string, map[string]any) error {
			t.Fatal("Create should not be called for invalid runnerID")
			return nil
		},
	})
	err := repo.Register(context.Background(), "not-hex", "10.0.0.1")
	if !errors.Is(err, ErrInvalidRunnerID) {
		t.Errorf("got %v, want ErrInvalidRunnerID", err)
	}
}

func TestFirestoreRegister_EmptyPrivateHost(t *testing.T) {
	t.Parallel()
	repo := NewFirestoreRepositoryWithAPI(&mockFirestoreClientAPI{})
	err := repo.Register(context.Background(), firestoreTestRunnerID, "")
	if !errors.Is(err, ErrInvalidPrivateHost) {
		t.Errorf("got %v, want ErrInvalidPrivateHost", err)
	}
}

func TestFirestoreRegister_Success(t *testing.T) {
	t.Parallel()
	called := false
	repo := NewFirestoreRepositoryWithAPI(&mockFirestoreClientAPI{
		createFn: func(_ context.Context, runnerID string, data map[string]any) error {
			called = true
			if runnerID != firestoreTestRunnerID {
				t.Errorf("runnerID = %q", runnerID)
			}
			want := map[string]any{FieldPrivateHost: "10.0.0.1", FieldCurrentSessionID: nil}
			if !reflect.DeepEqual(data, want) {
				t.Errorf("data = %v, want %v", data, want)
			}
			return nil
		},
	})
	if err := repo.Register(context.Background(), firestoreTestRunnerID, "10.0.0.1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("Create was not called")
	}
}

func TestFirestoreRegister_Conflict(t *testing.T) {
	t.Parallel()
	repo := NewFirestoreRepositoryWithAPI(&mockFirestoreClientAPI{
		createFn: func(context.Context, string, map[string]any) error { return ErrConflict },
	})
	err := repo.Register(context.Background(), firestoreTestRunnerID, "10.0.0.1")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got: %v", err)
	}
}

func TestFirestoreRegister_CreateError(t *testing.T) {
	t.Parallel()
	repo := NewFirestoreRepositoryWithAPI(&mockFirestoreClientAPI{
		createFn: func(context.Context, string, map[string]any) error { return errors.New("network error") },
	})
	err := repo.Register(context.Background(), firestoreTestRunnerID, "10.0.0.1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// (start, ∞) segment の 1 ページ目で idle が取れて precondition が通るケース。
func TestFirestoreAcquireIdle_Success(t *testing.T) {
	t.Parallel()
	assigned := false
	api := &mockFirestoreClientAPI{
		queryIdleRangeFn: func(_ context.Context, after, upTo string, limit int) ([]FirestoreDocSnapshot, error) {
			if after != firestoreTestStart || upTo != "" || limit != acquireQueryLimit {
				t.Errorf("args = (%q,%q,%d)", after, upTo, limit)
			}
			return []FirestoreDocSnapshot{{ID: "r1", Data: idleData("10.0.0.1")}}, nil
		},
		runTxFn: func(_ context.Context, _ func(FirestoreTx) error) error {
			assigned = true
			return nil
		},
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	repo.randHexFn = func() string { return firestoreTestStart }

	runner, err := repo.AcquireIdle(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !assigned {
		t.Error("RunTx was not called")
	}
	if runner.RunnerID != "r1" || runner.CurrentSessionID != "sess-1" || !runner.IsBusy() || runner.PrivateHost != "10.0.0.1" {
		t.Errorf("runner = %+v", runner)
	}
}

// segment 1 が空で [∅, start] segment に wrap して取れるケース。
func TestFirestoreAcquireIdle_WrapFromHead(t *testing.T) {
	t.Parallel()
	calls := 0
	api := &mockFirestoreClientAPI{
		queryIdleRangeFn: func(_ context.Context, after, upTo string, _ int) ([]FirestoreDocSnapshot, error) {
			calls++
			switch calls {
			case 1:
				if after != firestoreTestStart || upTo != "" {
					t.Errorf("call1 (after,upTo) = (%q,%q), want (%q,\"\")", after, upTo, firestoreTestStart)
				}
				return nil, nil
			case 2:
				if after != "" || upTo != firestoreTestStart {
					t.Errorf("call2 (after,upTo) = (%q,%q), want (\"\",%q)", after, upTo, firestoreTestStart)
				}
				return []FirestoreDocSnapshot{{ID: "r-low", Data: idleData("10.0.0.1")}}, nil
			}
			t.Fatalf("unexpected extra QueryIdleRange call")
			return nil, nil
		},
		runTxFn: func(context.Context, func(FirestoreTx) error) error { return nil },
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	repo.randHexFn = func() string { return firestoreTestStart }

	runner, err := repo.AcquireIdle(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner.RunnerID != "r-low" {
		t.Errorf("runnerID = %q", runner.RunnerID)
	}
	if calls != 2 {
		t.Errorf("QueryIdleRange calls = %d, want 2", calls)
	}
}

func TestFirestoreAcquireIdle_NoIdleRunner(t *testing.T) {
	t.Parallel()
	api := &mockFirestoreClientAPI{
		queryIdleRangeFn: func(context.Context, string, string, int) ([]FirestoreDocSnapshot, error) {
			return nil, nil
		},
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	repo.randHexFn = func() string { return firestoreTestStart }

	_, err := repo.AcquireIdle(context.Background(), "sess-1")
	if !errors.Is(err, ErrNoIdleRunner) {
		t.Fatalf("expected ErrNoIdleRunner, got: %v", err)
	}
}

// 1st segment がエラーを返すケース。
func TestFirestoreAcquireIdle_QueryError(t *testing.T) {
	t.Parallel()
	api := &mockFirestoreClientAPI{
		queryIdleRangeFn: func(context.Context, string, string, int) ([]FirestoreDocSnapshot, error) {
			return nil, errors.New("query error")
		},
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	repo.randHexFn = func() string { return firestoreTestStart }

	_, err := repo.AcquireIdle(context.Background(), "sess-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// 1st segment が空 → wrap した 2nd segment がエラーを返すケース。
func TestFirestoreAcquireIdle_WrapSegmentError(t *testing.T) {
	t.Parallel()
	calls := 0
	want := errors.New("wrap query error")
	api := &mockFirestoreClientAPI{
		queryIdleRangeFn: func(context.Context, string, string, int) ([]FirestoreDocSnapshot, error) {
			calls++
			if calls == 1 {
				return nil, nil
			}
			return nil, want
		},
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	repo.randHexFn = func() string { return firestoreTestStart }

	_, err := repo.AcquireIdle(context.Background(), "sess-1")
	if !errors.Is(err, want) {
		t.Fatalf("expected %v, got: %v", want, err)
	}
	if calls != 2 {
		t.Errorf("QueryIdleRange calls = %d, want 2", calls)
	}
}

// 1 ページ内 precondition 失敗を tried に記録して次候補を試すケース。
func TestFirestoreAcquireIdle_SkipStaleInPage(t *testing.T) {
	t.Parallel()
	queryCalls := 0
	txCalls := 0
	api := &mockFirestoreClientAPI{
		queryIdleRangeFn: func(context.Context, string, string, int) ([]FirestoreDocSnapshot, error) {
			queryCalls++
			return []FirestoreDocSnapshot{
				{ID: "r-busy", Data: idleData("10.0.0.1")},
				{ID: "r-good", Data: idleData("10.0.0.2")},
			}, nil
		},
		runTxFn: func(_ context.Context, _ func(FirestoreTx) error) error {
			txCalls++
			if txCalls == 1 {
				return ErrConditionFailed
			}
			return nil
		},
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	repo.randHexFn = func() string { return firestoreTestStart }

	runner, err := repo.AcquireIdle(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner.RunnerID != "r-good" {
		t.Errorf("runnerID = %q, want r-good", runner.RunnerID)
	}
	if queryCalls != 1 {
		t.Errorf("QueryIdleRange calls = %d, want 1", queryCalls)
	}
}

// 1 ページ全 stale → cursor 前進 → 次ページで assign 通るケース。
func TestFirestoreAcquireIdle_AdvanceCursor(t *testing.T) {
	t.Parallel()
	queryCalls := 0
	firstPage := []FirestoreDocSnapshot{
		{ID: "r-stale-1", Data: idleData("10.0.0.1")},
		{ID: "r-stale-2", Data: idleData("10.0.0.2")},
		{ID: "r-stale-3", Data: idleData("10.0.0.3")},
		{ID: "r-stale-4", Data: idleData("10.0.0.4")},
		{ID: "r-stale-5", Data: idleData("10.0.0.5")},
	}
	api := &mockFirestoreClientAPI{
		queryIdleRangeFn: func(_ context.Context, after, _ string, _ int) ([]FirestoreDocSnapshot, error) {
			queryCalls++
			switch queryCalls {
			case 1:
				if after != firestoreTestStart {
					t.Errorf("call1 after = %q, want %q", after, firestoreTestStart)
				}
				return firstPage, nil
			case 2:
				if after != "r-stale-5" {
					t.Errorf("call2 after = %q, want r-stale-5", after)
				}
				return []FirestoreDocSnapshot{{ID: "r-good", Data: idleData("10.0.0.9")}}, nil
			}
			t.Fatalf("unexpected extra QueryIdleRange call")
			return nil, nil
		},
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	repo.randHexFn = func() string { return firestoreTestStart }

	// r-stale-* は precondition 失敗、r-good は成功。runTxFn は呼び出し順で分岐する。
	txCalls := 0
	api.runTxFn = func(context.Context, func(FirestoreTx) error) error {
		txCalls++
		if txCalls <= len(firstPage) {
			return ErrConditionFailed
		}
		return nil
	}

	runner, err := repo.AcquireIdle(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner.RunnerID != "r-good" {
		t.Errorf("runnerID = %q, want r-good", runner.RunnerID)
	}
	if queryCalls != 2 {
		t.Errorf("QueryIdleRange calls = %d, want 2", queryCalls)
	}
}

// precondition 以外の RunTx エラーは即返す。
func TestFirestoreAcquireIdle_AssignError(t *testing.T) {
	t.Parallel()
	api := &mockFirestoreClientAPI{
		queryIdleRangeFn: func(context.Context, string, string, int) ([]FirestoreDocSnapshot, error) {
			return []FirestoreDocSnapshot{{ID: "r1", Data: idleData("10.0.0.1")}}, nil
		},
		runTxFn: func(context.Context, func(FirestoreTx) error) error { return errors.New("assign error") },
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	repo.randHexFn = func() string { return firestoreTestStart }

	_, err := repo.AcquireIdle(context.Background(), "sess-1")
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrNoIdleRunner) {
		t.Errorf("expected non-ErrNoIdleRunner, got: %v", err)
	}
}

// 全 idle doc が precondition 失敗 → 2 segment 尽くして ErrNoIdleRunner。
// tried set が segment 間で共有され、同じ doc は 2 度 assign しない。
func TestFirestoreAcquireIdle_AllStale(t *testing.T) {
	t.Parallel()
	queryCalls := 0
	assignCalls := 0
	api := &mockFirestoreClientAPI{
		queryIdleRangeFn: func(_ context.Context, after, _ string, _ int) ([]FirestoreDocSnapshot, error) {
			queryCalls++
			if after == "r-stale" {
				return nil, nil
			}
			return []FirestoreDocSnapshot{{ID: "r-stale", Data: idleData("10.0.0.1")}}, nil
		},
		runTxFn: func(context.Context, func(FirestoreTx) error) error {
			assignCalls++
			return ErrConditionFailed
		},
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	repo.randHexFn = func() string { return firestoreTestStart }

	_, err := repo.AcquireIdle(context.Background(), "sess-1")
	if !errors.Is(err, ErrNoIdleRunner) {
		t.Fatalf("expected ErrNoIdleRunner, got: %v", err)
	}
	if assignCalls != 1 {
		t.Errorf("RunTx calls = %d, want 1 (tried set skips repeats)", assignCalls)
	}
	if queryCalls == 0 {
		t.Errorf("QueryIdleRange was never called")
	}
}

// 全 doc が malformed の場合は idle 枯渇として ErrNoIdleRunner を返し、log にスキップ理由を残す。
func TestFirestoreAcquireIdle_AllMalformedNoIdleRunner(t *testing.T) {
	t.Parallel()
	var logged []string
	api := &mockFirestoreClientAPI{
		queryIdleRangeFn: func(context.Context, string, string, int) ([]FirestoreDocSnapshot, error) {
			return []FirestoreDocSnapshot{{ID: "r1", Data: map[string]any{}}}, nil
		},
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	repo.randHexFn = func() string { return firestoreTestStart }
	repo.logFn = func(format string, args ...any) {
		logged = append(logged, format)
	}

	_, err := repo.AcquireIdle(context.Background(), "sess-1")
	if !errors.Is(err, ErrNoIdleRunner) {
		t.Fatalf("expected ErrNoIdleRunner, got: %v", err)
	}
	if len(logged) == 0 {
		t.Error("expected malformed skip to be logged")
	}
}

// batch に malformed doc と有効な doc が混在する場合、malformed は tried に記録して skip し、
// 有効な doc に assign する。単一の壊れた document で AcquireIdle 全体が失敗しない。
func TestFirestoreAcquireIdle_SkipMalformedContinueToNext(t *testing.T) {
	t.Parallel()
	updateCalled := ""
	tx := &mockFirestoreTx{
		getFn: func(runnerID string) (map[string]any, bool, error) {
			return map[string]any{FieldCurrentSessionID: nil}, true, nil
		},
		updateFn: func(runnerID, _ string, _ any) error {
			updateCalled = runnerID
			return nil
		},
	}
	queryCalls := 0
	api := &mockFirestoreClientAPI{
		queryIdleRangeFn: func(context.Context, string, string, int) ([]FirestoreDocSnapshot, error) {
			queryCalls++
			return []FirestoreDocSnapshot{
				{ID: "r-bad", Data: map[string]any{}},
				{ID: "r-good", Data: idleData("10.0.0.2")},
			}, nil
		},
		runTxFn: func(_ context.Context, fn func(FirestoreTx) error) error {
			return fn(tx)
		},
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	repo.randHexFn = func() string { return firestoreTestStart }
	repo.logFn = func(string, ...any) {}

	runner, err := repo.AcquireIdle(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner.RunnerID != "r-good" {
		t.Errorf("runnerID = %q, want r-good", runner.RunnerID)
	}
	if updateCalled != "r-good" {
		t.Errorf("assignSession called for %q, want r-good", updateCalled)
	}
}

func TestFirestoreAssignSession_Success(t *testing.T) {
	t.Parallel()
	updateCalled := false
	tx := &mockFirestoreTx{
		getFn: func(runnerID string) (map[string]any, bool, error) {
			if runnerID != "r1" {
				t.Errorf("runnerID = %q", runnerID)
			}
			return map[string]any{FieldCurrentSessionID: nil}, true, nil
		},
		updateFn: func(runnerID, field string, value any) error {
			updateCalled = true
			if runnerID != "r1" || field != FieldCurrentSessionID || value != "sess-1" {
				t.Errorf("Update args = (%q,%q,%v)", runnerID, field, value)
			}
			return nil
		},
	}
	api := &mockFirestoreClientAPI{
		runTxFn: func(_ context.Context, fn func(FirestoreTx) error) error {
			return fn(tx)
		},
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	if err := repo.assignSession(context.Background(), "r1", "sess-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updateCalled {
		t.Error("Update was not called")
	}
}

func TestFirestoreAssignSession_NotFound(t *testing.T) {
	t.Parallel()
	tx := &mockFirestoreTx{
		getFn: func(string) (map[string]any, bool, error) { return nil, false, nil },
	}
	api := &mockFirestoreClientAPI{
		runTxFn: func(_ context.Context, fn func(FirestoreTx) error) error { return fn(tx) },
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	err := repo.assignSession(context.Background(), "r1", "sess-1")
	if !errors.Is(err, ErrConditionFailed) {
		t.Errorf("expected ErrConditionFailed, got %v", err)
	}
}

func TestFirestoreAssignSession_AlreadyBusy(t *testing.T) {
	t.Parallel()
	tx := &mockFirestoreTx{
		getFn: func(string) (map[string]any, bool, error) {
			return map[string]any{FieldCurrentSessionID: "other-sess"}, true, nil
		},
	}
	api := &mockFirestoreClientAPI{
		runTxFn: func(_ context.Context, fn func(FirestoreTx) error) error { return fn(tx) },
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	err := repo.assignSession(context.Background(), "r1", "sess-1")
	if !errors.Is(err, ErrConditionFailed) {
		t.Errorf("expected ErrConditionFailed, got %v", err)
	}
}

func TestFirestoreAssignSession_MissingSessionField(t *testing.T) {
	t.Parallel()
	tx := &mockFirestoreTx{
		getFn: func(string) (map[string]any, bool, error) {
			return map[string]any{FieldPrivateHost: "10.0.0.1"}, true, nil
		},
	}
	api := &mockFirestoreClientAPI{
		runTxFn: func(_ context.Context, fn func(FirestoreTx) error) error { return fn(tx) },
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	err := repo.assignSession(context.Background(), "r1", "sess-1")
	if !errors.Is(err, ErrConditionFailed) {
		t.Errorf("expected ErrConditionFailed, got %v", err)
	}
}

func TestFirestoreAssignSession_TxGetError(t *testing.T) {
	t.Parallel()
	want := errors.New("tx get boom")
	tx := &mockFirestoreTx{
		getFn: func(string) (map[string]any, bool, error) { return nil, false, want },
	}
	api := &mockFirestoreClientAPI{
		runTxFn: func(_ context.Context, fn func(FirestoreTx) error) error { return fn(tx) },
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	err := repo.assignSession(context.Background(), "r1", "sess-1")
	if !errors.Is(err, want) {
		t.Errorf("expected %v, got %v", want, err)
	}
}

func TestFirestoreAssignSession_TxUpdateError(t *testing.T) {
	t.Parallel()
	want := errors.New("tx update boom")
	tx := &mockFirestoreTx{
		getFn: func(string) (map[string]any, bool, error) {
			return map[string]any{FieldCurrentSessionID: nil}, true, nil
		},
		updateFn: func(string, string, any) error { return want },
	}
	api := &mockFirestoreClientAPI{
		runTxFn: func(_ context.Context, fn func(FirestoreTx) error) error { return fn(tx) },
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	err := repo.assignSession(context.Background(), "r1", "sess-1")
	if !errors.Is(err, want) {
		t.Errorf("expected %v, got %v", want, err)
	}
}

func TestFirestoreAssignSession_RunTxError(t *testing.T) {
	t.Parallel()
	want := errors.New("run tx boom")
	api := &mockFirestoreClientAPI{
		runTxFn: func(context.Context, func(FirestoreTx) error) error { return want },
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	err := repo.assignSession(context.Background(), "r1", "sess-1")
	if !errors.Is(err, want) {
		t.Errorf("expected %v, got %v", want, err)
	}
}

func TestFirestoreListBusyRunners_Success(t *testing.T) {
	t.Parallel()
	stopCount := 0
	iter := &mockFirestoreDocIter{
		results: []nextResult{
			{id: "r1", data: busyData("10.0.0.1", "sess-1")},
			{id: "r2", data: busyData("10.0.0.2", "sess-2")},
			{done: true},
		},
		stopCounter: &stopCount,
	}
	api := &mockFirestoreClientAPI{
		iterBusyFn: func(context.Context) FirestoreDocIter { return iter },
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	runners, err := repo.ListBusyRunners(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runners) != 2 {
		t.Fatalf("len(runners) = %d, want 2", len(runners))
	}
	for i, r := range runners {
		if !r.IsBusy() {
			t.Errorf("runners[%d] not busy", i)
		}
	}
	if stopCount != 1 {
		t.Errorf("Stop called %d times, want 1", stopCount)
	}
}

func TestFirestoreListBusyRunners_Empty(t *testing.T) {
	t.Parallel()
	iter := &mockFirestoreDocIter{results: []nextResult{{done: true}}}
	api := &mockFirestoreClientAPI{
		iterBusyFn: func(context.Context) FirestoreDocIter { return iter },
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	runners, err := repo.ListBusyRunners(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runners) != 0 {
		t.Errorf("len(runners) = %d, want 0", len(runners))
	}
}

func TestFirestoreListBusyRunners_IterError(t *testing.T) {
	t.Parallel()
	stopCount := 0
	want := errors.New("iter boom")
	iter := &mockFirestoreDocIter{
		results: []nextResult{
			{id: "r1", data: busyData("10.0.0.1", "sess-1")},
			{err: want},
		},
		stopCounter: &stopCount,
	}
	api := &mockFirestoreClientAPI{
		iterBusyFn: func(context.Context) FirestoreDocIter { return iter },
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	_, err := repo.ListBusyRunners(context.Background())
	if !errors.Is(err, want) {
		t.Errorf("expected %v, got %v", want, err)
	}
	if stopCount != 1 {
		t.Errorf("Stop called %d times, want 1 (defer で必ず呼ばれる)", stopCount)
	}
}

// ListBusyRunners は malformed doc を skip し、残りの有効な doc を返す。
// 単一の壊れた document で /runners/busy 全体が失敗しないようにする。
func TestFirestoreListBusyRunners_SkipMalformed(t *testing.T) {
	t.Parallel()
	var logged []string
	iter := &mockFirestoreDocIter{
		results: []nextResult{
			{id: "r-bad", data: map[string]any{}},
			{id: "r-good", data: busyData("10.0.0.2", "sess-9")},
		},
	}
	api := &mockFirestoreClientAPI{
		iterBusyFn: func(context.Context) FirestoreDocIter { return iter },
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	repo.logFn = func(format string, args ...any) {
		logged = append(logged, format)
	}
	runners, err := repo.ListBusyRunners(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runners) != 1 {
		t.Fatalf("len(runners) = %d, want 1", len(runners))
	}
	if runners[0].RunnerID != "r-good" {
		t.Errorf("runnerID = %q, want r-good", runners[0].RunnerID)
	}
	if len(logged) == 0 {
		t.Error("expected malformed skip to be logged")
	}
}

func TestFirestoreFindBySessionID_Success(t *testing.T) {
	t.Parallel()
	api := &mockFirestoreClientAPI{
		queryBySessionFn: func(_ context.Context, sessionID string) (string, map[string]any, bool, error) {
			return "r1", busyData("10.0.0.1", sessionID), true, nil
		},
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	runner, err := repo.FindBySessionID(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner.RunnerID != "r1" || !runner.IsBusy() {
		t.Errorf("runner = %+v", runner)
	}
}

func TestFirestoreFindBySessionID_NotFound(t *testing.T) {
	t.Parallel()
	api := &mockFirestoreClientAPI{
		queryBySessionFn: func(context.Context, string) (string, map[string]any, bool, error) {
			return "", nil, false, nil
		},
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	_, err := repo.FindBySessionID(context.Background(), "sess-x")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestFirestoreFindBySessionID_Error(t *testing.T) {
	t.Parallel()
	api := &mockFirestoreClientAPI{
		queryBySessionFn: func(context.Context, string) (string, map[string]any, bool, error) {
			return "", nil, false, errors.New("find error")
		},
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	if _, err := repo.FindBySessionID(context.Background(), "sess-1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestFirestoreFindBySessionID_SnapshotError(t *testing.T) {
	t.Parallel()
	api := &mockFirestoreClientAPI{
		queryBySessionFn: func(context.Context, string) (string, map[string]any, bool, error) {
			return "r1", map[string]any{}, true, nil
		},
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	if _, err := repo.FindBySessionID(context.Background(), "sess-1"); err == nil {
		t.Fatal("expected snapshotToDoc error to propagate")
	}
}

func TestFirestoreFindByID_Success(t *testing.T) {
	t.Parallel()
	api := &mockFirestoreClientAPI{
		getFn: func(_ context.Context, runnerID string) (map[string]any, bool, error) {
			return idleData("10.0.0.1"), true, nil
		},
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	runner, err := repo.FindByID(context.Background(), "r1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner.RunnerID != "r1" || !runner.IsIdle() {
		t.Errorf("runner = %+v", runner)
	}
}

func TestFirestoreFindByID_NotFound(t *testing.T) {
	t.Parallel()
	api := &mockFirestoreClientAPI{
		getFn: func(context.Context, string) (map[string]any, bool, error) { return nil, false, nil },
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	_, err := repo.FindByID(context.Background(), "r-missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestFirestoreFindByID_Error(t *testing.T) {
	t.Parallel()
	api := &mockFirestoreClientAPI{
		getFn: func(context.Context, string) (map[string]any, bool, error) {
			return nil, false, errors.New("get error")
		},
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	if _, err := repo.FindByID(context.Background(), "r1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestFirestoreFindByID_SnapshotError(t *testing.T) {
	t.Parallel()
	api := &mockFirestoreClientAPI{
		getFn: func(context.Context, string) (map[string]any, bool, error) {
			return map[string]any{}, true, nil
		},
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	if _, err := repo.FindByID(context.Background(), "r1"); err == nil {
		t.Fatal("expected snapshotToDoc error to propagate")
	}
}

func TestFirestoreDelete_Success(t *testing.T) {
	t.Parallel()
	called := false
	api := &mockFirestoreClientAPI{
		deleteFn: func(_ context.Context, runnerID string) error {
			called = true
			if runnerID != "r1" {
				t.Errorf("runnerID = %q", runnerID)
			}
			return nil
		},
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	if err := repo.Delete(context.Background(), "r1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("Delete was not called")
	}
}

func TestFirestoreDelete_Error(t *testing.T) {
	t.Parallel()
	api := &mockFirestoreClientAPI{
		deleteFn: func(context.Context, string) error { return errors.New("delete error") },
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	if err := repo.Delete(context.Background(), "r1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestFirestoreRegister_ModelRoundtrip(t *testing.T) {
	t.Parallel()
	stored := map[string]map[string]any{}
	api := &mockFirestoreClientAPI{
		createFn: func(_ context.Context, runnerID string, data map[string]any) error {
			stored[runnerID] = data
			return nil
		},
		getFn: func(_ context.Context, runnerID string) (map[string]any, bool, error) {
			data, ok := stored[runnerID]
			if !ok {
				return nil, false, nil
			}
			return data, true, nil
		},
	}
	repo := NewFirestoreRepositoryWithAPI(api)
	if err := repo.Register(context.Background(), firestoreTestRunnerID, "10.0.0.9"); err != nil {
		t.Fatalf("Register: %v", err)
	}
	runner, err := repo.FindByID(context.Background(), firestoreTestRunnerID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if runner.PrivateHost != "10.0.0.9" {
		t.Errorf("PrivateHost = %q", runner.PrivateHost)
	}
	if runner.State != model.StateIdle {
		t.Errorf("state = %q, want idle", runner.State)
	}
}

func idleData(privateHost string) map[string]any {
	return map[string]any{FieldPrivateHost: privateHost, FieldCurrentSessionID: nil}
}

func busyData(privateHost, sessionID string) map[string]any {
	return map[string]any{FieldPrivateHost: privateHost, FieldCurrentSessionID: sessionID}
}

func TestFirestoreRepository_Close(t *testing.T) {
	t.Parallel()
	want := errors.New("close boom")
	called := false
	repo := NewFirestoreRepositoryWithAPI(&mockFirestoreClientAPI{
		closeFn: func() error {
			called = true
			return want
		},
	})
	if err := repo.Close(); !errors.Is(err, want) {
		t.Errorf("expected %v, got %v", want, err)
	}
	if !called {
		t.Error("Close was not called")
	}
}
