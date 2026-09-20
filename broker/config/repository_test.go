// Package config は NewRepositoryFromEnv の env dispatch を検証する。
package config

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ogadra/bunshin/broker/store"
)

func setDynamoEnv(t *testing.T) {
	t.Helper()
	t.Setenv("BUNSHIN_STORE", "dynamodb")
	t.Setenv("DYNAMODB_ENDPOINT", "http://localhost:18000")
	t.Setenv("AWS_REGION", "ap-northeast-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "localdev")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "localdev")
}

func saveNewDynamoRepositoryFn(t *testing.T) {
	t.Helper()
	orig := NewDynamoRepositoryFn
	t.Cleanup(func() { NewDynamoRepositoryFn = orig })
}

type fakeDynamoRepo struct{ store.Repository }

func TestNewRepositoryFromEnv_DynamoInjected(t *testing.T) {
	setDynamoEnv(t)
	saveNewDynamoRepositoryFn(t)

	called := false
	NewDynamoRepositoryFn = func(_ context.Context, cfg store.DynamoConfig) (store.Repository, error) {
		called = true
		want := store.DynamoConfig{
			Region:    "ap-northeast-1",
			AccessKey: "localdev",
			SecretKey: "localdev",
			Endpoint:  "http://localhost:18000",
		}
		if cfg != want {
			t.Errorf("cfg = %+v, want %+v", cfg, want)
		}
		return fakeDynamoRepo{}, nil
	}

	repo, err := NewRepositoryFromEnv(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo == nil {
		t.Fatal("expected non-nil Repository")
	}
	if !called {
		t.Error("NewDynamoRepositoryFn was not called")
	}
}

func TestNewRepositoryFromEnv_DynamoFactoryError(t *testing.T) {
	setDynamoEnv(t)
	saveNewDynamoRepositoryFn(t)

	NewDynamoRepositoryFn = func(context.Context, store.DynamoConfig) (store.Repository, error) {
		return nil, errors.New("factory failed")
	}

	_, err := NewRepositoryFromEnv(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "factory failed") {
		t.Errorf("error = %q, want to contain factory error", err.Error())
	}
}

// TrimSpace 後の空文字は missing に fold される (stack.go と同じ扱い)。
func TestNewRepositoryFromEnv_MissingStore(t *testing.T) {
	tests := []struct {
		name  string
		store string
	}{
		{"empty", ""},
		{"whitespace only", "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BUNSHIN_STORE", tt.store)

			_, err := NewRepositoryFromEnv(context.Background())
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), "missing required environment variable: BUNSHIN_STORE") {
				t.Errorf("error = %q, want missing BUNSHIN_STORE", err.Error())
			}
		})
	}
}

// case 違いは dynamodb に fold されない (surrounding whitespace は TrimSpace で受容)。
func TestNewRepositoryFromEnv_UnsupportedStore(t *testing.T) {
	tests := []struct {
		name  string
		store string
	}{
		{"other engine", "cassandra"},
		{"different case", "DynamoDB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BUNSHIN_STORE", tt.store)

			_, err := NewRepositoryFromEnv(context.Background())
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), "unsupported BUNSHIN_STORE") {
				t.Errorf("error = %q, want unsupported BUNSHIN_STORE", err.Error())
			}
			if !strings.Contains(err.Error(), tt.store) {
				t.Errorf("error = %q, want to include %q", err.Error(), tt.store)
			}
		})
	}
}

func TestNewRepositoryFromEnv_DynamoMissingRegion(t *testing.T) {
	t.Setenv("BUNSHIN_STORE", "dynamodb")
	t.Setenv("AWS_REGION", "")

	_, err := NewRepositoryFromEnv(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "AWS_REGION") {
		t.Errorf("error = %q, want to contain AWS_REGION", err.Error())
	}
}

func TestNewRepositoryFromEnv_DynamoPartialCredentials(t *testing.T) {
	setDynamoEnv(t)
	t.Setenv("AWS_ACCESS_KEY_ID", "localdev")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")

	_, err := NewRepositoryFromEnv(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "AWS_ACCESS_KEY_ID") || !strings.Contains(err.Error(), "AWS_SECRET_ACCESS_KEY") {
		t.Errorf("error = %q, want to mention both credential keys", err.Error())
	}
}
