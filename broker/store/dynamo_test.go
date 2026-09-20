// Package store はリポジトリ層のテストを提供する。
package store

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/ogadra/bunshin/broker/model"
)

// testRunnerID は Register テストで使う 32 桁小文字 hex の runnerId。
const testRunnerID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// testPrivateHost は idleItem/busyItem のヘルパーが GSI 射影として含める privateHost 値。
// state-index の Projection = ALL が壊れて privateHost が消えた場合にテストで検出できるよう、ヘルパー側で必ず載せる。
const testPrivateHost = "10.0.0.1"

// mockDynamoDBAPI は dynamoDBAPI のモック実装。
type mockDynamoDBAPI struct {
	putItemFn    func(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	getItemFn    func(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	updateItemFn func(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	deleteItemFn func(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	queryFn      func(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
}

// PutItem はモック PutItem を呼び出す。
func (m *mockDynamoDBAPI) PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	return m.putItemFn(ctx, params, optFns...)
}

// GetItem はモック GetItem を呼び出す。
func (m *mockDynamoDBAPI) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	return m.getItemFn(ctx, params, optFns...)
}

// UpdateItem はモック UpdateItem を呼び出す。
func (m *mockDynamoDBAPI) UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	return m.updateItemFn(ctx, params, optFns...)
}

// DeleteItem はモック DeleteItem を呼び出す。
func (m *mockDynamoDBAPI) DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	return m.deleteItemFn(ctx, params, optFns...)
}

// Query はモック Query を呼び出す。
func (m *mockDynamoDBAPI) Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	return m.queryFn(ctx, params, optFns...)
}

// idleItem は state = StateIdle の GSI item を組み立てるヘルパー。
func idleItem(runnerID string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"runnerId":    &types.AttributeValueMemberS{Value: runnerID},
		"state":       &types.AttributeValueMemberS{Value: string(model.StateIdle)},
		"privateHost": &types.AttributeValueMemberS{Value: testPrivateHost},
	}
}

// busyItem は state = StateBusy の GSI item を組み立てるヘルパー。
func busyItem(runnerID, sessionID string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"runnerId":         &types.AttributeValueMemberS{Value: runnerID},
		"state":            &types.AttributeValueMemberS{Value: string(model.StateBusy)},
		"currentSessionId": &types.AttributeValueMemberS{Value: sessionID},
		"privateHost":      &types.AttributeValueMemberS{Value: testPrivateHost},
	}
}

// TestNewDynamoRepository はコンストラクタが必要な依存関係を設定することを検証する。
func TestNewDynamoRepository(t *testing.T) {
	t.Parallel()
	mock := &mockDynamoDBAPI{}
	repo := NewDynamoRepository(mock, "test-table")
	if repo.client != mock {
		t.Error("client mismatch")
	}
	if repo.tableName != "test-table" {
		t.Errorf("tableName = %q, want %q", repo.tableName, "test-table")
	}
	if repo.randHexFn == nil {
		t.Error("randHexFn is nil")
	}
	if repo.marshalFn == nil {
		t.Error("marshalFn is nil")
	}
}

// TestDefaultRandHexFn はデフォルト乱数関数が 32 文字の hex 文字列を返し、
// 十分に分散していることを検証する。
func TestDefaultRandHexFn(t *testing.T) {
	t.Parallel()
	seen := map[string]struct{}{}
	for range 100 {
		v := defaultRandHexFn()
		if len(v) != 32 {
			t.Fatalf("len(defaultRandHexFn()) = %d, want 32", len(v))
		}
		for _, c := range v {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Fatalf("non-hex character %q in %q", c, v)
			}
		}
		seen[v] = struct{}{}
	}
	if len(seen) < 90 {
		t.Errorf("distinct values = %d, want at least 90 in 100 iterations", len(seen))
	}
}

// TestRegister_MarshalError は MarshalMap がエラーを返す場合にエラーを返すことを検証する。
func TestRegister_MarshalError(t *testing.T) {
	t.Parallel()
	repo := NewDynamoRepository(&mockDynamoDBAPI{}, "t")
	repo.marshalFn = func(_ interface{}) (map[string]types.AttributeValue, error) {
		return nil, errors.New("marshal error")
	}

	err := repo.Register(context.Background(), testRunnerID, "10.0.0.1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestRegister_InvalidRunnerID は runnerID が 32 桁小文字 hex でない場合に ErrInvalidRunnerID を返すことを検証する。
func TestRegister_InvalidRunnerID(t *testing.T) {
	t.Parallel()
	repo := NewDynamoRepository(&mockDynamoDBAPI{
		putItemFn: func(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
			t.Fatal("PutItem should not be called for invalid runnerID")
			return nil, nil
		},
	}, "t")

	for _, invalid := range []string{
		"",
		"r1",
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",  // uppercase hex
		"gggggggggggggggggggggggggggggggg",  // out of hex range
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",   // 31 chars
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", // 33 chars
	} {
		err := repo.Register(context.Background(), invalid, "10.0.0.1")
		if !errors.Is(err, ErrInvalidRunnerID) {
			t.Errorf("runnerID=%q: got err=%v, want ErrInvalidRunnerID", invalid, err)
		}
	}
}

// TestRegister_Success は新規登録の成功ケースを検証する。state = "idle" が書き込まれる。
func TestRegister_Success(t *testing.T) {
	t.Parallel()
	called := false
	mock := &mockDynamoDBAPI{
		putItemFn: func(_ context.Context, params *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
			called = true
			if *params.ConditionExpression != "attribute_not_exists(runnerId)" {
				t.Errorf("unexpected condition: %s", *params.ConditionExpression)
			}
			state, ok := params.Item["state"].(*types.AttributeValueMemberS)
			if !ok || state.Value != string(model.StateIdle) {
				t.Errorf("state was not marshaled as idle, got %v", params.Item["state"])
			}
			v, ok := params.Item["privateHost"].(*types.AttributeValueMemberS)
			if !ok || v.Value != "10.0.0.1" {
				t.Errorf("privateHost was not marshaled correctly")
			}
			return &dynamodb.PutItemOutput{}, nil
		},
	}
	repo := NewDynamoRepository(mock, "t")

	err := repo.Register(context.Background(), testRunnerID, "10.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("PutItem was not called")
	}
}

// TestRegister_Conflict は既存 runnerId への PutItem が ConditionalCheckFailed で ErrConflict を返すことを検証する。
func TestRegister_Conflict(t *testing.T) {
	t.Parallel()
	mock := &mockDynamoDBAPI{
		putItemFn: func(_ context.Context, _ *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
			return nil, &types.ConditionalCheckFailedException{Message: aws.String("exists")}
		},
	}
	repo := NewDynamoRepository(mock, "t")

	err := repo.Register(context.Background(), testRunnerID, "10.0.0.1")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got: %v", err)
	}
}

// TestRegister_EmptyPrivateHost は privateHost が空の場合に ErrInvalidPrivateHost を返すことを検証する。
func TestRegister_EmptyPrivateHost(t *testing.T) {
	t.Parallel()
	repo := NewDynamoRepository(&mockDynamoDBAPI{}, "t")

	err := repo.Register(context.Background(), testRunnerID, "")
	if !errors.Is(err, ErrInvalidPrivateHost) {
		t.Fatalf("got %v, want ErrInvalidPrivateHost", err)
	}
}

// TestRegister_PutItemError は PutItem の予期せぬエラーを検証する。
func TestRegister_PutItemError(t *testing.T) {
	t.Parallel()
	mock := &mockDynamoDBAPI{
		putItemFn: func(_ context.Context, _ *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
			return nil, errors.New("network error")
		},
	}
	repo := NewDynamoRepository(mock, "t")

	err := repo.Register(context.Background(), testRunnerID, "10.0.0.1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// assertStateIdxRange は state-index query の KeyConditionExpression・:v の値・Limit を検証する。
func assertStateIdxRange(t *testing.T, params *dynamodb.QueryInput, wantCond, wantValue string) {
	t.Helper()
	if params.IndexName == nil || *params.IndexName != "state-index" {
		t.Fatalf("IndexName = %v, want state-index", params.IndexName)
	}
	wantExpr := "#s = :s AND " + wantCond
	if *params.KeyConditionExpression != wantExpr {
		t.Errorf("KeyConditionExpression = %q, want %q", *params.KeyConditionExpression, wantExpr)
	}
	if got := params.ExpressionAttributeValues[":s"].(*types.AttributeValueMemberS).Value; got != string(model.StateIdle) {
		t.Errorf(":s = %q, want %q", got, model.StateIdle)
	}
	if got := params.ExpressionAttributeValues[":v"].(*types.AttributeValueMemberS).Value; got != wantValue {
		t.Errorf(":v = %q, want %q", got, wantValue)
	}
	if params.Limit == nil || *params.Limit != acquireQueryLimit {
		t.Errorf("Limit = %v, want %d", params.Limit, acquireQueryLimit)
	}
}

// testStart は AcquireIdle テストで固定するランダム開始 runnerId。中間の 32 hex を使い
// (start, ∞) / [∅, start] の両 segment が非空になるようにする。
const testStart = "88888888888888888888888888888888"

// TestAcquireIdle_Success は最初の区間 [start, max] の先頭ページで idle runner を確保できることを検証する。
func TestAcquireIdle_Success(t *testing.T) {
	t.Parallel()
	queryCount := 0
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			queryCount++
			assertStateIdxRange(t, params, segFirstCond, testStart)
			return &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{
				idleItem("r1"), idleItem("r2"), idleItem("r3"), idleItem("r4"), idleItem("r5"),
			}}, nil
		},
		updateItemFn: func(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
			if params.Key["runnerId"].(*types.AttributeValueMemberS).Value != "r1" {
				t.Errorf("unexpected runnerId")
			}
			if *params.ConditionExpression != "#s = :idle" {
				t.Errorf("ConditionExpression = %q", *params.ConditionExpression)
			}
			if *params.UpdateExpression != "SET #s = :busy, currentSessionId = :sid" {
				t.Errorf("UpdateExpression = %q", *params.UpdateExpression)
			}
			return &dynamodb.UpdateItemOutput{}, nil
		},
	}
	repo := NewDynamoRepository(mock, "t")
	repo.randHexFn = func() string { return testStart }

	runner, err := repo.AcquireIdle(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner.RunnerID != "r1" {
		t.Errorf("runnerID = %q, want %q", runner.RunnerID, "r1")
	}
	if runner.CurrentSessionID != "sess-1" {
		t.Errorf("currentSessionId = %q, want %q", runner.CurrentSessionID, "sess-1")
	}
	if runner.PrivateHost != testPrivateHost {
		t.Errorf("privateHost = %q, want %q (GSI projection must carry privateHost)", runner.PrivateHost, testPrivateHost)
	}
	if !runner.IsBusy() {
		t.Errorf("expected runner to be busy, state = %q", runner.State)
	}
	if queryCount != 1 {
		t.Errorf("queryCount = %d, want 1 (found in first segment)", queryCount)
	}
}

// TestAcquireIdle_SecondSegment は最初の区間 [start, max] が空のとき、次の区間 [min, start] で確保することを検証する。
func TestAcquireIdle_SecondSegment(t *testing.T) {
	t.Parallel()
	sawFirst, sawSecond := false, false
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			switch *params.KeyConditionExpression {
			case "#s = :s AND " + segFirstCond:
				sawFirst = true
				assertStateIdxRange(t, params, segFirstCond, testStart)
				return &dynamodb.QueryOutput{Items: nil}, nil
			case "#s = :s AND " + segSecondCond:
				sawSecond = true
				assertStateIdxRange(t, params, segSecondCond, testStart)
				return &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{idleItem("r-low")}}, nil
			}
			t.Fatalf("unexpected KeyConditionExpression %q", *params.KeyConditionExpression)
			return nil, nil
		},
		updateItemFn: func(_ context.Context, _ *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
			return &dynamodb.UpdateItemOutput{}, nil
		},
	}
	repo := NewDynamoRepository(mock, "t")
	repo.randHexFn = func() string { return testStart }

	runner, err := repo.AcquireIdle(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner.RunnerID != "r-low" {
		t.Errorf("runnerID = %q, want %q", runner.RunnerID, "r-low")
	}
	if !sawFirst || !sawSecond {
		t.Errorf("expected both segments to be queried, sawFirst=%v sawSecond=%v", sawFirst, sawSecond)
	}
}

// TestAcquireIdle_ExactMatchStart は runnerId が乱数開始位置と完全一致する item が
// segment 1 (runnerId > :v) では除外され、segment 2 (runnerId <= :v) で拾われることを検証する。
// BETWEEN (両端 inclusive) から exclusive/inclusive の非対称な 2 segment に変えたことによる境界挙動。
func TestAcquireIdle_ExactMatchStart(t *testing.T) {
	t.Parallel()
	sawFirst, sawSecond := false, false
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			switch *params.KeyConditionExpression {
			case "#s = :s AND " + segFirstCond:
				sawFirst = true
				assertStateIdxRange(t, params, segFirstCond, testStart)
				return &dynamodb.QueryOutput{Items: nil}, nil
			case "#s = :s AND " + segSecondCond:
				sawSecond = true
				assertStateIdxRange(t, params, segSecondCond, testStart)
				return &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{idleItem(testStart)}}, nil
			}
			t.Fatalf("unexpected KeyConditionExpression %q", *params.KeyConditionExpression)
			return nil, nil
		},
		updateItemFn: func(_ context.Context, _ *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
			return &dynamodb.UpdateItemOutput{}, nil
		},
	}
	repo := NewDynamoRepository(mock, "t")
	repo.randHexFn = func() string { return testStart }

	runner, err := repo.AcquireIdle(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner.RunnerID != testStart {
		t.Errorf("runnerID = %q, want %q", runner.RunnerID, testStart)
	}
	if !sawFirst || !sawSecond {
		t.Errorf("expected both segments to be queried, sawFirst=%v sawSecond=%v", sawFirst, sawSecond)
	}
}

// TestAcquireIdle_NoIdleRunner は両 segment が空の場合に 2 query で ErrNoIdleRunner を返すことを検証する。
func TestAcquireIdle_NoIdleRunner(t *testing.T) {
	t.Parallel()
	sawFirst, sawSecond := false, false
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			switch *params.KeyConditionExpression {
			case "#s = :s AND " + segFirstCond:
				sawFirst = true
				assertStateIdxRange(t, params, segFirstCond, testStart)
			case "#s = :s AND " + segSecondCond:
				sawSecond = true
				assertStateIdxRange(t, params, segSecondCond, testStart)
			default:
				t.Fatalf("unexpected KeyConditionExpression %q", *params.KeyConditionExpression)
			}
			return &dynamodb.QueryOutput{Items: nil}, nil
		},
	}
	repo := NewDynamoRepository(mock, "t")
	repo.randHexFn = func() string { return testStart }

	_, err := repo.AcquireIdle(context.Background(), "sess-1")
	if !errors.Is(err, ErrNoIdleRunner) {
		t.Fatalf("expected ErrNoIdleRunner, got: %v", err)
	}
	if !sawFirst || !sawSecond {
		t.Errorf("expected both segments to be queried, sawFirst=%v sawSecond=%v", sawFirst, sawSecond)
	}
}

// TestAcquireIdle_QueryError は Query エラー時にエラーを返すことを検証する。
func TestAcquireIdle_QueryError(t *testing.T) {
	t.Parallel()
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			assertStateIdxRange(t, params, segFirstCond, testStart)
			return nil, errors.New("query error")
		},
	}
	repo := NewDynamoRepository(mock, "t")
	repo.randHexFn = func() string { return testStart }

	_, err := repo.AcquireIdle(context.Background(), "sess-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestAcquireIdle_RetryWithinBatch はページ内で条件失敗した候補をスキップし次を試行することを検証する。
func TestAcquireIdle_RetryWithinBatch(t *testing.T) {
	t.Parallel()
	tried := map[string]struct{}{}
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			assertStateIdxRange(t, params, segFirstCond, testStart)
			return &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{
				idleItem("r1"), idleItem("r2"), idleItem("r3"), idleItem("r4"), idleItem("r5"),
			}}, nil
		},
		updateItemFn: func(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
			rid := params.Key["runnerId"].(*types.AttributeValueMemberS).Value
			tried[rid] = struct{}{}
			if rid != "r2" {
				return nil, &types.ConditionalCheckFailedException{Message: aws.String("conflict")}
			}
			return &dynamodb.UpdateItemOutput{}, nil
		},
	}
	repo := NewDynamoRepository(mock, "t")
	repo.randHexFn = func() string { return testStart }

	runner, err := repo.AcquireIdle(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner.RunnerID != "r2" {
		t.Errorf("runnerID = %q, want %q", runner.RunnerID, "r2")
	}
	if _, ok := tried["r1"]; !ok {
		t.Errorf("expected r1 to have been tried before r2, tried=%v", tried)
	}
	if _, ok := tried["r2"]; !ok {
		t.Errorf("expected r2 to have been tried, tried=%v", tried)
	}
}

// TestAcquireIdle_SecondSegmentReachesMaskedIdle は最初の区間 [start, max] が stale item で満杯になり
// runnerId の小さい idle が隠れていても、次区間 [min, start] のページングで確保できることを検証する。
func TestAcquireIdle_SecondSegmentReachesMaskedIdle(t *testing.T) {
	t.Parallel()
	stale := []map[string]types.AttributeValue{
		idleItem("s1"), idleItem("s2"), idleItem("s3"), idleItem("s4"), idleItem("s5"),
	}
	cursor := map[string]types.AttributeValue{"runnerId": &types.AttributeValueMemberS{Value: "s5"}}
	seg1Queries, seg2Queries := 0, 0
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if *params.KeyConditionExpression == "#s = :s AND "+segFirstCond {
				seg1Queries++ // (start, ∞): stale で満杯、全 conflict
				assertStateIdxRange(t, params, segFirstCond, testStart)
				return &dynamodb.QueryOutput{Items: stale}, nil
			}
			seg2Queries++ // [∅, start]: 先頭ページは tried 済み stale、続きに masked idle
			assertStateIdxRange(t, params, segSecondCond, testStart)
			if params.ExclusiveStartKey == nil {
				return &dynamodb.QueryOutput{Items: stale, LastEvaluatedKey: cursor}, nil
			}
			return &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{idleItem("x-low")}}, nil
		},
		updateItemFn: func(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
			if params.Key["runnerId"].(*types.AttributeValueMemberS).Value == "x-low" {
				return &dynamodb.UpdateItemOutput{}, nil
			}
			return nil, &types.ConditionalCheckFailedException{Message: aws.String("already busy (stale index)")}
		},
	}
	repo := NewDynamoRepository(mock, "t")
	repo.randHexFn = func() string { return testStart }

	runner, err := repo.AcquireIdle(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner.RunnerID != "x-low" {
		t.Errorf("runnerID = %q, want %q", runner.RunnerID, "x-low")
	}
	if seg1Queries != 1 {
		t.Errorf("seg1Queries = %d, want 1", seg1Queries)
	}
	if seg2Queries != 2 {
		t.Errorf("seg2Queries = %d, want 2 (paginated)", seg2Queries)
	}
}

// TestAcquireIdle_StaleGSI は stale item を返し続ける GSI でも tried set で両 segment を走査し切り
// 有限時間で ErrNoIdleRunner に収束することを検証する。
func TestAcquireIdle_StaleGSI(t *testing.T) {
	t.Parallel()
	sawFirst, sawSecond := false, false
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			switch *params.KeyConditionExpression {
			case "#s = :s AND " + segFirstCond:
				sawFirst = true
				assertStateIdxRange(t, params, segFirstCond, testStart)
			case "#s = :s AND " + segSecondCond:
				sawSecond = true
				assertStateIdxRange(t, params, segSecondCond, testStart)
			default:
				t.Fatalf("unexpected KeyConditionExpression %q", *params.KeyConditionExpression)
			}
			return &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{idleItem("r-stale")}}, nil
		},
		updateItemFn: func(_ context.Context, _ *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
			return nil, &types.ConditionalCheckFailedException{Message: aws.String("stale")}
		},
	}
	repo := NewDynamoRepository(mock, "t")
	repo.randHexFn = func() string { return testStart }

	_, err := repo.AcquireIdle(context.Background(), "sess-1")
	if !errors.Is(err, ErrNoIdleRunner) {
		t.Fatalf("expected ErrNoIdleRunner, got: %v", err)
	}
	if !sawFirst || !sawSecond {
		t.Errorf("expected both segments to be queried, sawFirst=%v sawSecond=%v", sawFirst, sawSecond)
	}
}

// TestAcquireIdle_UpdateError は UpdateItem の条件失敗以外のエラーを検証する。
func TestAcquireIdle_UpdateError(t *testing.T) {
	t.Parallel()
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			assertStateIdxRange(t, params, segFirstCond, testStart)
			return &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{idleItem("r1")}}, nil
		},
		updateItemFn: func(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
			if got := params.Key["runnerId"].(*types.AttributeValueMemberS).Value; got != "r1" {
				t.Errorf("runnerId = %q, want %q", got, "r1")
			}
			return nil, errors.New("update error")
		},
	}
	repo := NewDynamoRepository(mock, "t")
	repo.randHexFn = func() string { return testStart }

	_, err := repo.AcquireIdle(context.Background(), "sess-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestAcquireIdle_UnmarshalError は Query 結果の unmarshal 失敗を検証する。
func TestAcquireIdle_UnmarshalError(t *testing.T) {
	t.Parallel()
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			assertStateIdxRange(t, params, segFirstCond, testStart)
			return &dynamodb.QueryOutput{
				Items: []map[string]types.AttributeValue{
					{"runnerId": &types.AttributeValueMemberL{Value: []types.AttributeValue{}}},
				},
			}, nil
		},
	}
	repo := NewDynamoRepository(mock, "t")
	repo.randHexFn = func() string { return testStart }

	_, err := repo.AcquireIdle(context.Background(), "sess-1")
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
}

// assertBusyQuery は ListBusyRunners が state-index に発行する query の共通形を検証する。
func assertBusyQuery(t *testing.T, params *dynamodb.QueryInput) {
	t.Helper()
	if params.IndexName == nil || *params.IndexName != "state-index" {
		t.Errorf("IndexName = %v, want state-index", params.IndexName)
	}
	if *params.KeyConditionExpression != "#s = :s" {
		t.Errorf("KeyConditionExpression = %q", *params.KeyConditionExpression)
	}
	if got := params.ExpressionAttributeValues[":s"].(*types.AttributeValueMemberS).Value; got != string(model.StateBusy) {
		t.Errorf(":s = %q, want %q", got, model.StateBusy)
	}
}

// TestListBusyRunners_SinglePage は 1 ページで完結する場合に全件返すことを検証する。
func TestListBusyRunners_SinglePage(t *testing.T) {
	t.Parallel()
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			assertBusyQuery(t, params)
			if params.ExclusiveStartKey != nil {
				t.Errorf("ExclusiveStartKey should be nil on first call, got %v", params.ExclusiveStartKey)
			}
			return &dynamodb.QueryOutput{
				Items: []map[string]types.AttributeValue{
					busyItem("r1", "sess-1"),
					busyItem("r2", "sess-2"),
				},
			}, nil
		},
	}
	repo := NewDynamoRepository(mock, "t")

	runners, err := repo.ListBusyRunners(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runners) != 2 {
		t.Fatalf("len(runners) = %d, want 2", len(runners))
	}
	if !runners[0].IsBusy() {
		t.Errorf("expected first runner to be busy, state = %q", runners[0].State)
	}
	if runners[0].PrivateHost != testPrivateHost {
		t.Errorf("runners[0].privateHost = %q, want %q (GSI projection must carry privateHost)", runners[0].PrivateHost, testPrivateHost)
	}
}

// TestListBusyRunners_Empty は busy runner がいない場合に空リストを返すことを検証する。
func TestListBusyRunners_Empty(t *testing.T) {
	t.Parallel()
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			assertBusyQuery(t, params)
			return &dynamodb.QueryOutput{Items: nil}, nil
		},
	}
	repo := NewDynamoRepository(mock, "t")

	runners, err := repo.ListBusyRunners(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runners) != 0 {
		t.Errorf("len(runners) = %d, want 0", len(runners))
	}
}

// TestListBusyRunners_MultiPage は LastEvaluatedKey が nil になるまでページを辿り、
// 続きの Query では ExclusiveStartKey が前ページの LastEvaluatedKey で満たされることを検証する。
func TestListBusyRunners_MultiPage(t *testing.T) {
	t.Parallel()
	page1LEK := map[string]types.AttributeValue{
		"runnerId": &types.AttributeValueMemberS{Value: "r2"},
		"state":    &types.AttributeValueMemberS{Value: string(model.StateBusy)},
	}
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			assertBusyQuery(t, params)
			if params.ExclusiveStartKey == nil {
				return &dynamodb.QueryOutput{
					Items: []map[string]types.AttributeValue{
						busyItem("r1", "sess-1"),
						busyItem("r2", "sess-2"),
					},
					LastEvaluatedKey: page1LEK,
				}, nil
			}
			if v := params.ExclusiveStartKey["runnerId"].(*types.AttributeValueMemberS).Value; v != "r2" {
				t.Errorf("ExclusiveStartKey.runnerId = %q, want %q", v, "r2")
			}
			return &dynamodb.QueryOutput{
				Items: []map[string]types.AttributeValue{busyItem("r3", "sess-3")},
			}, nil
		},
	}
	repo := NewDynamoRepository(mock, "t")

	runners, err := repo.ListBusyRunners(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runners) != 3 {
		t.Fatalf("len(runners) = %d, want 3", len(runners))
	}
	if runners[2].RunnerID != "r3" {
		t.Errorf("runners[2].RunnerID = %q, want r3", runners[2].RunnerID)
	}
	for i, r := range runners {
		if r.PrivateHost != testPrivateHost {
			t.Errorf("runners[%d].PrivateHost = %q, want %q (multi-page projection must carry privateHost)", i, r.PrivateHost, testPrivateHost)
		}
	}
}

// TestListBusyRunners_QueryError は Query エラー時にエラーを返すことを検証する。
func TestListBusyRunners_QueryError(t *testing.T) {
	t.Parallel()
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			assertBusyQuery(t, params)
			return nil, errors.New("query error")
		},
	}
	repo := NewDynamoRepository(mock, "t")

	_, err := repo.ListBusyRunners(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestListBusyRunners_UnmarshalError は Query 結果の unmarshal 失敗を検証する。
func TestListBusyRunners_UnmarshalError(t *testing.T) {
	t.Parallel()
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			assertBusyQuery(t, params)
			return &dynamodb.QueryOutput{
				Items: []map[string]types.AttributeValue{
					{"runnerId": &types.AttributeValueMemberL{Value: []types.AttributeValue{}}},
				},
			}, nil
		},
	}
	repo := NewDynamoRepository(mock, "t")

	_, err := repo.ListBusyRunners(context.Background())
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
}

// TestFindBySessionID_Success は session ID で runner が見つかるケースを検証する。
func TestFindBySessionID_Success(t *testing.T) {
	t.Parallel()
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if *params.IndexName != "session-index" {
				t.Errorf("unexpected index: %s", *params.IndexName)
			}
			return &dynamodb.QueryOutput{
				Items: []map[string]types.AttributeValue{
					{
						"runnerId":         &types.AttributeValueMemberS{Value: "r1"},
						"currentSessionId": &types.AttributeValueMemberS{Value: "sess-1"},
					},
				},
			}, nil
		},
	}
	repo := NewDynamoRepository(mock, "t")

	runner, err := repo.FindBySessionID(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner.RunnerID != "r1" {
		t.Errorf("runnerID = %q, want %q", runner.RunnerID, "r1")
	}
}

// TestFindBySessionID_NotFound は session が見つからない場合に ErrNotFound を返すことを検証する。
func TestFindBySessionID_NotFound(t *testing.T) {
	t.Parallel()
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			return &dynamodb.QueryOutput{Items: nil}, nil
		},
	}
	repo := NewDynamoRepository(mock, "t")

	_, err := repo.FindBySessionID(context.Background(), "sess-x")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

// TestFindBySessionID_QueryError は Query エラー時にエラーを返すことを検証する。
func TestFindBySessionID_QueryError(t *testing.T) {
	t.Parallel()
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			return nil, errors.New("query error")
		},
	}
	repo := NewDynamoRepository(mock, "t")

	_, err := repo.FindBySessionID(context.Background(), "sess-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestFindBySessionID_UnmarshalError は Query 結果の unmarshal 失敗を検証する。
func TestFindBySessionID_UnmarshalError(t *testing.T) {
	t.Parallel()
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			return &dynamodb.QueryOutput{
				Items: []map[string]types.AttributeValue{
					{"runnerId": &types.AttributeValueMemberL{Value: []types.AttributeValue{}}},
				},
			}, nil
		},
	}
	repo := NewDynamoRepository(mock, "t")

	_, err := repo.FindBySessionID(context.Background(), "sess-1")
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
}

// TestFindByID_Success は runner ID で runner が見つかるケースを検証する。
func TestFindByID_Success(t *testing.T) {
	t.Parallel()
	mock := &mockDynamoDBAPI{
		getItemFn: func(_ context.Context, _ *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
			return &dynamodb.GetItemOutput{Item: idleItem("r1")}, nil
		},
	}
	repo := NewDynamoRepository(mock, "t")

	runner, err := repo.FindByID(context.Background(), "r1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner.RunnerID != "r1" {
		t.Errorf("runnerID = %q, want %q", runner.RunnerID, "r1")
	}
	if !runner.IsIdle() {
		t.Error("expected runner to be idle")
	}
}

// TestFindByID_NotFound は runner が存在しない場合に ErrNotFound を返すことを検証する。
func TestFindByID_NotFound(t *testing.T) {
	t.Parallel()
	mock := &mockDynamoDBAPI{
		getItemFn: func(_ context.Context, _ *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
			return &dynamodb.GetItemOutput{Item: nil}, nil
		},
	}
	repo := NewDynamoRepository(mock, "t")

	_, err := repo.FindByID(context.Background(), "r-missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

// TestFindByID_GetItemError は GetItem の予期せぬエラーを検証する。
func TestFindByID_GetItemError(t *testing.T) {
	t.Parallel()
	mock := &mockDynamoDBAPI{
		getItemFn: func(_ context.Context, _ *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
			return nil, errors.New("get error")
		},
	}
	repo := NewDynamoRepository(mock, "t")

	_, err := repo.FindByID(context.Background(), "r1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestFindByID_UnmarshalError は GetItem 結果の unmarshal 失敗を検証する。
func TestFindByID_UnmarshalError(t *testing.T) {
	t.Parallel()
	mock := &mockDynamoDBAPI{
		getItemFn: func(_ context.Context, _ *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
			return &dynamodb.GetItemOutput{
				Item: map[string]types.AttributeValue{
					"runnerId": &types.AttributeValueMemberL{Value: []types.AttributeValue{}},
				},
			}, nil
		},
	}
	repo := NewDynamoRepository(mock, "t")

	_, err := repo.FindByID(context.Background(), "r1")
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
}

// TestDelete_Success は正常な削除を検証する。
func TestDelete_Success(t *testing.T) {
	t.Parallel()
	mock := &mockDynamoDBAPI{
		deleteItemFn: func(_ context.Context, params *dynamodb.DeleteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
			if params.Key["runnerId"].(*types.AttributeValueMemberS).Value != "r1" {
				t.Errorf("unexpected runnerId")
			}
			return &dynamodb.DeleteItemOutput{}, nil
		},
	}
	repo := NewDynamoRepository(mock, "t")

	err := repo.Delete(context.Background(), "r1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestDelete_Error は DeleteItem の予期せぬエラーを検証する。
func TestDelete_Error(t *testing.T) {
	t.Parallel()
	mock := &mockDynamoDBAPI{
		deleteItemFn: func(_ context.Context, _ *dynamodb.DeleteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
			return nil, errors.New("delete error")
		},
	}
	repo := NewDynamoRepository(mock, "t")

	err := repo.Delete(context.Background(), "r1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestDynamoRepository_ImplementsRepository は DynamoRepository が Repository インターフェースを満たすことを検証する。
func TestDynamoRepository_ImplementsRepository(t *testing.T) {
	t.Parallel()
	var _ Repository = (*DynamoRepository)(nil)
}
