// Package config は環境変数を読んで broker の依存を組み立てる composition-root 層。
// os.Getenv とクラウド SDK 初期化を集約し、上位 (main) からは値/結果のみを扱えるようにする。
package config

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ogadra/bunshin/broker/store"
	"github.com/ogadra/bunshin/broker/store/firestoreadapter"
)

// NewRepositoryFromEnv は BUNSHIN_STORE を読み対応する Repository を組み立てる。
// dynamodb / firestore 以外の値・未設定は起動失敗させる (default は持たない)。
// stack.go と揃えて TrimSpace 後の空文字は「missing」に fold する。
func NewRepositoryFromEnv(ctx context.Context) (store.Repository, error) {
	switch kind := strings.TrimSpace(os.Getenv("BUNSHIN_STORE")); kind {
	case "dynamodb":
		return newDynamoFromEnv(ctx)
	case "firestore":
		return newFirestoreFromEnv(ctx)
	case "":
		return nil, fmt.Errorf("missing required environment variable: BUNSHIN_STORE")
	default:
		return nil, fmt.Errorf("unsupported BUNSHIN_STORE %q (expected dynamodb or firestore)", kind)
	}
}

// NewDynamoRepositoryFn は DynamoDB backend factory の test seam。
// 上位パッケージ (broker main_test) の Handler 組み立てテストからも差し替えるため export する。
var NewDynamoRepositoryFn = func(ctx context.Context, cfg store.DynamoConfig) (store.Repository, error) {
	return store.NewDynamoRepositoryFromEnv(ctx, cfg)
}

// NewFirestoreRepositoryFn は NewDynamoRepositoryFn と同じ抽象レベルの test seam。
var NewFirestoreRepositoryFn = func(ctx context.Context, projectID, databaseID string) (store.Repository, error) {
	return firestoreadapter.NewRepository(ctx, projectID, databaseID)
}

func newDynamoFromEnv(ctx context.Context) (store.Repository, error) {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		return nil, fmt.Errorf("missing required environment variable: AWS_REGION")
	}
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	if (accessKey == "") != (secretKey == "") {
		return nil, fmt.Errorf("AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY must both be set or both be empty")
	}
	return NewDynamoRepositoryFn(ctx, store.DynamoConfig{
		Region:    region,
		AccessKey: accessKey,
		SecretKey: secretKey,
		Endpoint:  os.Getenv("DYNAMODB_ENDPOINT"),
	})
}

func newFirestoreFromEnv(ctx context.Context) (store.Repository, error) {
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		return nil, fmt.Errorf("missing required environment variable: GOOGLE_CLOUD_PROJECT")
	}
	databaseID := os.Getenv("FIRESTORE_DATABASE")
	if databaseID == "" {
		return nil, fmt.Errorf("missing required environment variable: FIRESTORE_DATABASE")
	}
	return NewFirestoreRepositoryFn(ctx, projectID, databaseID)
}
