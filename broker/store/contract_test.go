// Package store_test は Repository の contract を DynamoDB Local と Firestore emulator の両方で検証する。
// 各 test は runContract で backend ごとに subtest 展開され、対象 backend の endpoint が未設定なら skip する。
package store_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/ogadra/bunshin/broker/model"
	"github.com/ogadra/bunshin/broker/store"
	"github.com/ogadra/bunshin/broker/store/firestoreadapter"
)

type backend struct {
	name  string
	setup func(t *testing.T) store.Repository
}

var contractBackends = []backend{
	{name: "DynamoDB", setup: setupDynamo},
	{name: "Firestore", setup: setupFirestore},
}

// runContract は fn を各 backend に対して subtest として並列実行する。
// 対象 backend の endpoint が未設定なら該当 subtest だけ skip する (broker-test-dynamodb と
// broker-test-firestore で片方の env だけ与える運用に対応)。
func runContract(t *testing.T, fn func(t *testing.T, repo store.Repository)) {
	for _, be := range contractBackends {
		t.Run(be.name, func(t *testing.T) {
			t.Parallel()
			repo := be.setup(t)
			fn(t, repo)
		})
	}
}

// dynamoTableSeq は t.Parallel() 下で同一 nanosecond に time.Now() が並ぶ race を排除するプロセス内カウンタ。
// setupFirestore の firestoreProjectSeq と対称。
var dynamoTableSeq atomic.Uint64

// setupDynamo は DynamoDB Local に isolated table を作った Repository を返す。
// tableName はナノ秒 timestamp + atomic seq で unique 化し、t.Cleanup で削除する。
func setupDynamo(t *testing.T) store.Repository {
	t.Helper()
	endpoint := os.Getenv("DYNAMODB_ENDPOINT")
	if endpoint == "" {
		t.Skip("DYNAMODB_ENDPOINT not set, skipping DynamoDB contract test")
	}
	tableName := fmt.Sprintf("runners-%d-%d", time.Now().UnixNano(), dynamoTableSeq.Add(1))
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion("ap-northeast-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
	)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
	if _, err := client.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: &tableName,
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("runnerId"), KeyType: types.KeyTypeHash},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("runnerId"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("state"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("currentSessionId"), AttributeType: types.ScalarAttributeTypeS},
		},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{
				IndexName: aws.String("state-index"),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("state"), KeyType: types.KeyTypeHash},
					{AttributeName: aws.String("runnerId"), KeyType: types.KeyTypeRange},
				},
				Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
				ProvisionedThroughput: &types.ProvisionedThroughput{
					ReadCapacityUnits:  aws.Int64(5),
					WriteCapacityUnits: aws.Int64(5),
				},
			},
			{
				IndexName: aws.String("session-index"),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("currentSessionId"), KeyType: types.KeyTypeHash},
				},
				Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
				ProvisionedThroughput: &types.ProvisionedThroughput{
					ReadCapacityUnits:  aws.Int64(5),
					WriteCapacityUnits: aws.Int64(5),
				},
			},
		},
		ProvisionedThroughput: &types.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(5),
			WriteCapacityUnits: aws.Int64(5),
		},
	}); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = client.DeleteTable(context.Background(), &dynamodb.DeleteTableInput{TableName: &tableName})
	})
	return store.NewDynamoRepository(client, tableName)
}

// firestoreProjectSeq は emulator 上の projectID を衝突不能にするためのプロセス内カウンタ。
// t.Parallel() 下で同一 nanosecond に time.Now() が並ぶ race を排除する。
var firestoreProjectSeq atomic.Uint64

// setupFirestore は Firestore emulator の isolated projectID に接続した Repository を返す。
// t.Cleanup で client を close し、複数 test の gRPC connection を残さない。
func setupFirestore(t *testing.T) store.Repository {
	t.Helper()
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set, skipping Firestore contract test")
	}
	// Firestore projectID は小文字英数 + ハイフンのみ、6-30 文字。
	// t.Name() は大文字と '_', '/' を含むため置換 & 小文字化する。
	// 衝突回避に unique seq を末尾に添え、余った長さで t.Name() を prefix する。
	suffix := fmt.Sprintf("-%d", firestoreProjectSeq.Add(1))
	const prefix = "bunshin-fs-"
	nameBudget := 30 - len(prefix) - len(suffix)
	safeName := strings.NewReplacer("_", "-", "/", "-").Replace(strings.ToLower(t.Name()))
	if len(safeName) > nameBudget {
		safeName = safeName[:nameBudget]
	}
	projectID := prefix + safeName + suffix
	repo, err := firestoreadapter.NewRepository(context.Background(), projectID, "(default)")
	if err != nil {
		t.Fatalf("firestoreadapter.NewRepository: %v", err)
	}
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Errorf("repo.Close: %v", err)
		}
	})
	return repo
}

func TestContract_RegisterAndFindByID(t *testing.T) {
	runContract(t, func(t *testing.T, repo store.Repository) {
		ctx := context.Background()
		if err := repo.Register(ctx, "11111111111111111111111111111111", "10.0.0.1"); err != nil {
			t.Fatalf("Register: %v", err)
		}
		runner, err := repo.FindByID(ctx, "11111111111111111111111111111111")
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if runner.RunnerID != "11111111111111111111111111111111" {
			t.Errorf("runnerID = %q, want %q", runner.RunnerID, "11111111111111111111111111111111")
		}
		if runner.State != model.StateIdle {
			t.Errorf("state = %q, want %q", runner.State, model.StateIdle)
		}
		if runner.PrivateHost != "10.0.0.1" {
			t.Errorf("privateHost = %q, want %q", runner.PrivateHost, "10.0.0.1")
		}
	})
}

func TestContract_RegisterConflict_SamePrivateHost(t *testing.T) {
	runContract(t, func(t *testing.T, repo store.Repository) {
		ctx := context.Background()
		if err := repo.Register(ctx, "11111111111111111111111111111111", "10.0.0.1"); err != nil {
			t.Fatalf("first Register: %v", err)
		}
		err := repo.Register(ctx, "11111111111111111111111111111111", "10.0.0.1")
		if !errors.Is(err, store.ErrConflict) {
			t.Fatalf("expected ErrConflict, got: %v", err)
		}
	})
}

func TestContract_RegisterConflict_DifferentPrivateHost(t *testing.T) {
	runContract(t, func(t *testing.T, repo store.Repository) {
		ctx := context.Background()
		if err := repo.Register(ctx, "11111111111111111111111111111111", "10.0.0.1"); err != nil {
			t.Fatalf("first Register: %v", err)
		}
		err := repo.Register(ctx, "11111111111111111111111111111111", "10.0.0.2")
		if !errors.Is(err, store.ErrConflict) {
			t.Fatalf("expected ErrConflict, got: %v", err)
		}
	})
}

func TestContract_AcquireIdle(t *testing.T) {
	runContract(t, func(t *testing.T, repo store.Repository) {
		ctx := context.Background()
		if err := repo.Register(ctx, "11111111111111111111111111111111", "10.0.0.1"); err != nil {
			t.Fatalf("Register: %v", err)
		}
		runner, err := repo.AcquireIdle(ctx, "sess-1")
		if err != nil {
			t.Fatalf("AcquireIdle: %v", err)
		}
		if runner.RunnerID != "11111111111111111111111111111111" {
			t.Errorf("runnerID = %q, want %q", runner.RunnerID, "11111111111111111111111111111111")
		}
		if runner.CurrentSessionID != "sess-1" {
			t.Errorf("currentSessionId = %q, want %q", runner.CurrentSessionID, "sess-1")
		}
		if runner.State != model.StateBusy {
			t.Errorf("state = %q, want %q", runner.State, model.StateBusy)
		}
		if runner.PrivateHost != "10.0.0.1" {
			t.Errorf("privateHost = %q, want %q", runner.PrivateHost, "10.0.0.1")
		}

		persisted, err := repo.FindByID(ctx, "11111111111111111111111111111111")
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if persisted.State != model.StateBusy {
			t.Errorf("persisted state = %q, want %q", persisted.State, model.StateBusy)
		}
		if persisted.CurrentSessionID != "sess-1" {
			t.Errorf("persisted currentSessionId = %q, want %q", persisted.CurrentSessionID, "sess-1")
		}
	})
}

func TestContract_AcquireIdle_Empty(t *testing.T) {
	runContract(t, func(t *testing.T, repo store.Repository) {
		_, err := repo.AcquireIdle(context.Background(), "sess-1")
		if !errors.Is(err, store.ErrNoIdleRunner) {
			t.Fatalf("expected ErrNoIdleRunner, got: %v", err)
		}
	})
}

// TestContract_AcquireIdle_WrapFromHead は乱数開始位置より lex 順で前にしか runner がない場合でも
// (start, max] を空振り → [min, start] へ wrap して取得できることを両 backend で検証する。
// randHexFn を "ffff…" に固定して "0a0a…" の runner が必ず wrap 経路でのみ取得されるようにする。
func TestContract_AcquireIdle_WrapFromHead(t *testing.T) {
	runContract(t, func(t *testing.T, repo store.Repository) {
		store.SetRandHexFnForTest(repo, func() string { return "ffffffffffffffffffffffffffffffff" })

		ctx := context.Background()
		if err := repo.Register(ctx, "0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a", "10.0.0.1"); err != nil {
			t.Fatalf("Register: %v", err)
		}
		runner, err := repo.AcquireIdle(ctx, "sess-wrap")
		if err != nil {
			t.Fatalf("AcquireIdle: %v", err)
		}
		if runner.RunnerID != "0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a" {
			t.Errorf("runnerID = %q, want %q", runner.RunnerID, "0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a")
		}
	})
}

func TestContract_AcquireIdle_FindBySessionID(t *testing.T) {
	runContract(t, func(t *testing.T, repo store.Repository) {
		ctx := context.Background()
		if err := repo.Register(ctx, "11111111111111111111111111111111", "10.0.0.1"); err != nil {
			t.Fatalf("Register: %v", err)
		}
		if _, err := repo.AcquireIdle(ctx, "sess-1"); err != nil {
			t.Fatalf("AcquireIdle: %v", err)
		}
		runner, err := repo.FindBySessionID(ctx, "sess-1")
		if err != nil {
			t.Fatalf("FindBySessionID: %v", err)
		}
		if runner.RunnerID != "11111111111111111111111111111111" {
			t.Errorf("runnerID = %q, want %q", runner.RunnerID, "11111111111111111111111111111111")
		}
	})
}

func TestContract_AcquireIdle_AlreadyBusy(t *testing.T) {
	runContract(t, func(t *testing.T, repo store.Repository) {
		ctx := context.Background()
		if err := repo.Register(ctx, "11111111111111111111111111111111", "10.0.0.1"); err != nil {
			t.Fatalf("Register: %v", err)
		}
		if _, err := repo.AcquireIdle(ctx, "sess-1"); err != nil {
			t.Fatalf("first AcquireIdle: %v", err)
		}
		_, err := repo.AcquireIdle(ctx, "sess-2")
		if !errors.Is(err, store.ErrNoIdleRunner) {
			t.Fatalf("expected ErrNoIdleRunner, got: %v", err)
		}
	})
}

func TestContract_Delete(t *testing.T) {
	runContract(t, func(t *testing.T, repo store.Repository) {
		ctx := context.Background()
		if err := repo.Register(ctx, "11111111111111111111111111111111", "10.0.0.1"); err != nil {
			t.Fatalf("Register: %v", err)
		}
		if err := repo.Delete(ctx, "11111111111111111111111111111111"); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		_, err := repo.FindByID(ctx, "11111111111111111111111111111111")
		if !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("expected ErrNotFound after delete, got: %v", err)
		}
	})
}

func TestContract_Delete_Idempotent(t *testing.T) {
	runContract(t, func(t *testing.T, repo store.Repository) {
		err := repo.Delete(context.Background(), "11111111111111111111111111111111")
		if err != nil {
			t.Fatalf("expected nil for idempotent delete, got: %v", err)
		}
	})
}

func TestContract_FindBySessionID_NotFound(t *testing.T) {
	runContract(t, func(t *testing.T, repo store.Repository) {
		_, err := repo.FindBySessionID(context.Background(), "sess-missing")
		if !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got: %v", err)
		}
	})
}

func TestContract_FindByID_NotFound(t *testing.T) {
	runContract(t, func(t *testing.T, repo store.Repository) {
		_, err := repo.FindByID(context.Background(), "22222222222222222222222222222222")
		if !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got: %v", err)
		}
	})
}

func TestContract_ListBusyRunners_All(t *testing.T) {
	runContract(t, func(t *testing.T, repo store.Repository) {
		ctx := context.Background()
		const total = 5
		want := make([]string, 0, total)
		for i := range total {
			id := fmt.Sprintf("%02d%030d", i, 0)
			want = append(want, id)
			if err := repo.Register(ctx, id, fmt.Sprintf("10.0.0.%d", i+1)); err != nil {
				t.Fatalf("Register %s: %v", id, err)
			}
			if _, err := repo.AcquireIdle(ctx, fmt.Sprintf("sess-%02d", i)); err != nil {
				t.Fatalf("AcquireIdle: %v", err)
			}
		}
		runners, err := repo.ListBusyRunners(ctx)
		if err != nil {
			t.Fatalf("ListBusyRunners: %v", err)
		}
		got := make([]string, 0, len(runners))
		for _, r := range runners {
			if r.State != model.StateBusy {
				t.Errorf("state = %q, want %q", r.State, model.StateBusy)
			}
			if r.PrivateHost == "" {
				t.Errorf("runner %s: privateHost empty", r.RunnerID)
			}
			got = append(got, r.RunnerID)
		}
		sort.Strings(got)
		sort.Strings(want)
		if len(got) != len(want) {
			t.Fatalf("len(got) = %d, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})
}

func TestContract_ListBusyRunners_Empty(t *testing.T) {
	runContract(t, func(t *testing.T, repo store.Repository) {
		runners, err := repo.ListBusyRunners(context.Background())
		if err != nil {
			t.Fatalf("ListBusyRunners: %v", err)
		}
		if len(runners) != 0 {
			t.Errorf("len(runners) = %d, want 0", len(runners))
		}
	})
}

// TestContract_ListBusyRunners_ExcludesIdle は busy な runner だけが返る contract を、
// 2 件登録し 1 件だけ AcquireIdle した状態で検証する。どちらが acquired 側になるかは
// random start に依存するため runnerID で判別する。
func TestContract_ListBusyRunners_ExcludesIdle(t *testing.T) {
	runContract(t, func(t *testing.T, repo store.Repository) {
		ctx := context.Background()
		if err := repo.Register(ctx, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "10.0.0.1"); err != nil {
			t.Fatalf("Register aaaa: %v", err)
		}
		if err := repo.Register(ctx, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "10.0.0.2"); err != nil {
			t.Fatalf("Register bbbb: %v", err)
		}
		acquired, err := repo.AcquireIdle(ctx, "sess-1")
		if err != nil {
			t.Fatalf("AcquireIdle: %v", err)
		}
		runners, err := repo.ListBusyRunners(ctx)
		if err != nil {
			t.Fatalf("ListBusyRunners: %v", err)
		}
		if len(runners) != 1 {
			t.Fatalf("len(runners) = %d, want 1 (idle 側は除外される)", len(runners))
		}
		if runners[0].RunnerID != acquired.RunnerID {
			t.Errorf("busy runnerID = %q, want %q", runners[0].RunnerID, acquired.RunnerID)
		}
	})
}
