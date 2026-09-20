package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
)

func saveLoadAWSConfig(t *testing.T) {
	t.Helper()
	orig := loadAWSConfig
	t.Cleanup(func() { loadAWSConfig = orig })
}

func TestNewDynamoRepositoryFromEnv_Success(t *testing.T) {
	saveLoadAWSConfig(t)
	loadAWSConfig = func(context.Context, ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{Region: "ap-northeast-1"}, nil
	}
	repo, err := NewDynamoRepositoryFromEnv(context.Background(), DynamoConfig{
		Region:    "ap-northeast-1",
		AccessKey: "localdev",
		SecretKey: "localdev",
		Endpoint:  "http://localhost:18000",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo == nil {
		t.Fatal("expected non-nil repo")
	}
}

func TestNewDynamoRepositoryFromEnv_NoCredsNoEndpoint(t *testing.T) {
	saveLoadAWSConfig(t)
	loadAWSConfig = func(context.Context, ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{Region: "ap-northeast-1"}, nil
	}
	repo, err := NewDynamoRepositoryFromEnv(context.Background(), DynamoConfig{Region: "ap-northeast-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo == nil {
		t.Fatal("expected non-nil repo")
	}
}

func TestNewDynamoRepositoryFromEnv_LoadConfigError(t *testing.T) {
	saveLoadAWSConfig(t)
	loadAWSConfig = func(context.Context, ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, errors.New("config load failed")
	}
	_, err := NewDynamoRepositoryFromEnv(context.Background(), DynamoConfig{Region: "ap-northeast-1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "load aws config") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "load aws config")
	}
}
