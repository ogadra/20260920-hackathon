// Package store は Runner の永続化層を提供する。
package store

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// loadAWSConfig は LoadDefaultConfig 失敗経路を unit test でカバーするための test seam。
var loadAWSConfig = awsconfig.LoadDefaultConfig

// NewDynamoRepositoryFromEnv は DynamoConfig を元に DynamoDB SDK を組み立て Repository を返す。
func NewDynamoRepositoryFromEnv(ctx context.Context, cfg DynamoConfig) (*DynamoRepository, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
	}
	if cfg.AccessKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")))
	}
	awsCfg, err := loadAWSConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	var ddbOpts []func(*dynamodb.Options)
	if cfg.Endpoint != "" {
		ddbOpts = append(ddbOpts, func(o *dynamodb.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		})
	}
	return NewDynamoRepository(dynamodb.NewFromConfig(awsCfg, ddbOpts...), "bunshin-runners"), nil
}
