// Package store は Runner の永続化層を提供する。
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/ogadra/bunshin/broker/model"
)

// segFirstCond / segSecondCond は AcquireIdle が (start, max] → [min, start] の順に走査する
// state-index の KeyConditionExpression 右辺。segment 1 が strict >、segment 2 が <= なので
// runnerId が start と完全一致する item は segment 2 で拾われる。
const (
	segFirstCond  = "runnerId > :v"
	segSecondCond = "runnerId <= :v"
)

// dynamoDBAPI は DynamoDB SDK から使う operation subset。
// DynamoRepository が触る API だけを絞ることで mock を書きやすくする。
// パッケージ外には露出させない (utilizer は同一パッケージの dynamo_test.go のみ)。
type dynamoDBAPI interface {
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
}

// DynamoConfig は NewDynamoRepositoryFromEnv に渡す SDK 設定。
// AccessKey / SecretKey が空文字なら IAM Role / instance profile などの
// default credential chain を SDK に任せる。Endpoint が空文字なら AWS の
// regional endpoint を使う (dynamodb-local 用途で override する経路)。
type DynamoConfig struct {
	Region    string
	AccessKey string
	SecretKey string
	Endpoint  string
}

type DynamoRepository struct {
	client    dynamoDBAPI
	tableName string
	randHexFn func() string
	marshalFn func(in interface{}) (map[string]types.AttributeValue, error)
}

func NewDynamoRepository(client dynamoDBAPI, tableName string) *DynamoRepository {
	return &DynamoRepository{
		client:    client,
		tableName: tableName,
		randHexFn: defaultRandHexFn,
		marshalFn: attributevalue.MarshalMap,
	}
}

func (r *DynamoRepository) Register(ctx context.Context, runnerID, privateHost string) error {
	if !runnerIDRe.MatchString(runnerID) {
		return ErrInvalidRunnerID
	}
	if privateHost == "" {
		return ErrInvalidPrivateHost
	}

	item, err := r.marshalFn(model.Runner{
		RunnerID:    runnerID,
		State:       model.StateIdle,
		PrivateHost: privateHost,
	})
	if err != nil {
		return fmt.Errorf("marshal runner: %w", err)
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           &r.tableName,
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(runnerId)"),
	})
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if isConditionalCheckFailed(err, &condErr) {
			return ErrConflict
		}
		return fmt.Errorf("put item: %w", err)
	}
	return nil
}

// AcquireIdle は (start, ∞) → [∅, start] の 2 segment を acquireQueryLimit 件ずつ paginate し、
// assignSession で precondition 競合した runner は tried に記録して次候補へ進む。
// 全 segment を辿り切れば idle 枯渇として ErrNoIdleRunner を返す (Firestore 側と同構造)。
func (r *DynamoRepository) AcquireIdle(ctx context.Context, sessionID string) (*model.Runner, error) {
	tried := map[string]struct{}{}
	start := r.randHexFn()
	for _, cond := range []string{segFirstCond, segSecondCond} {
		var exclusiveStart map[string]types.AttributeValue
		for {
			runners, next, err := r.queryIdlePage(ctx, cond, start, exclusiveStart)
			if err != nil {
				return nil, err
			}
			runner, err := r.assignFirst(ctx, sessionID, runners, tried)
			if runner != nil || err != nil {
				return runner, err
			}
			if len(next) == 0 {
				break
			}
			exclusiveStart = next
		}
	}
	return nil, ErrNoIdleRunner
}

func (r *DynamoRepository) assignFirst(ctx context.Context, sessionID string, candidates []model.Runner, tried map[string]struct{}) (*model.Runner, error) {
	for i := range candidates {
		runner := &candidates[i]
		if _, done := tried[runner.RunnerID]; done {
			continue
		}
		err := r.assignSession(ctx, runner.RunnerID, sessionID)
		if err == nil {
			runner.State = model.StateBusy
			runner.CurrentSessionID = sessionID
			return runner, nil
		}
		if !errors.Is(err, ErrConditionFailed) {
			return nil, err
		}
		tried[runner.RunnerID] = struct{}{}
	}
	return nil, nil
}

func (r *DynamoRepository) queryIdlePage(ctx context.Context, cond, value string, exclusiveStart map[string]types.AttributeValue) ([]model.Runner, map[string]types.AttributeValue, error) {
	out, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:                aws.String(r.tableName),
		IndexName:                aws.String("state-index"),
		KeyConditionExpression:   aws.String("#s = :s AND " + cond),
		ExpressionAttributeNames: map[string]string{"#s": "state"},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":s": &types.AttributeValueMemberS{Value: string(model.StateIdle)},
			":v": &types.AttributeValueMemberS{Value: value},
		},
		Limit:             aws.Int32(acquireQueryLimit),
		ExclusiveStartKey: exclusiveStart,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("query state-index: %w", err)
	}
	var runners []model.Runner
	if err := attributevalue.UnmarshalListOfMaps(out.Items, &runners); err != nil {
		return nil, nil, fmt.Errorf("unmarshal runners: %w", err)
	}
	return runners, out.LastEvaluatedKey, nil
}

// busy 遷移後も state 属性を残すことで GSI item が state-index の busy partition に載り、ListBusyRunners から辿れる。
func (r *DynamoRepository) assignSession(ctx context.Context, runnerID, sessionID string) error {
	_, err := r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: &r.tableName,
		Key: map[string]types.AttributeValue{
			"runnerId": &types.AttributeValueMemberS{Value: runnerID},
		},
		UpdateExpression:         aws.String("SET #s = :busy, currentSessionId = :sid"),
		ConditionExpression:      aws.String("#s = :idle"),
		ExpressionAttributeNames: map[string]string{"#s": "state"},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":sid":  &types.AttributeValueMemberS{Value: sessionID},
			":idle": &types.AttributeValueMemberS{Value: string(model.StateIdle)},
			":busy": &types.AttributeValueMemberS{Value: string(model.StateBusy)},
		},
	})
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if isConditionalCheckFailed(err, &condErr) {
			return ErrConditionFailed
		}
		return fmt.Errorf("update item: %w", err)
	}
	return nil
}

// busy 一覧は管理用の低頻度アクセス前提で全件収まる規模のため、pagination を API に出さず内部で辿り切る。
func (r *DynamoRepository) ListBusyRunners(ctx context.Context) ([]model.Runner, error) {
	var all []model.Runner
	var exclusiveStart map[string]types.AttributeValue
	for {
		out, err := r.client.Query(ctx, &dynamodb.QueryInput{
			TableName:                aws.String(r.tableName),
			IndexName:                aws.String("state-index"),
			KeyConditionExpression:   aws.String("#s = :s"),
			ExpressionAttributeNames: map[string]string{"#s": "state"},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":s": &types.AttributeValueMemberS{Value: string(model.StateBusy)},
			},
			ExclusiveStartKey: exclusiveStart,
		})
		if err != nil {
			return nil, fmt.Errorf("query state-index: %w", err)
		}
		var page []model.Runner
		if err := attributevalue.UnmarshalListOfMaps(out.Items, &page); err != nil {
			return nil, fmt.Errorf("unmarshal runners: %w", err)
		}
		all = append(all, page...)
		if len(out.LastEvaluatedKey) == 0 {
			return all, nil
		}
		exclusiveStart = out.LastEvaluatedKey
	}
}

func (r *DynamoRepository) FindBySessionID(ctx context.Context, sessionID string) (*model.Runner, error) {
	out, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              &r.tableName,
		IndexName:              aws.String("session-index"),
		KeyConditionExpression: aws.String("currentSessionId = :sid"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":sid": &types.AttributeValueMemberS{Value: sessionID},
		},
		Limit: aws.Int32(1),
	})
	if err != nil {
		return nil, fmt.Errorf("query session-index: %w", err)
	}
	if len(out.Items) == 0 {
		return nil, ErrNotFound
	}
	var runner model.Runner
	if err := attributevalue.UnmarshalMap(out.Items[0], &runner); err != nil {
		return nil, fmt.Errorf("unmarshal runner: %w", err)
	}
	return &runner, nil
}

func (r *DynamoRepository) FindByID(ctx context.Context, runnerID string) (*model.Runner, error) {
	out, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &r.tableName,
		Key: map[string]types.AttributeValue{
			"runnerId": &types.AttributeValueMemberS{Value: runnerID},
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("get item: %w", err)
	}
	if out.Item == nil {
		return nil, ErrNotFound
	}
	var runner model.Runner
	if err := attributevalue.UnmarshalMap(out.Item, &runner); err != nil {
		return nil, fmt.Errorf("unmarshal runner: %w", err)
	}
	return &runner, nil
}

func (r *DynamoRepository) Delete(ctx context.Context, runnerID string) error {
	_, err := r.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: &r.tableName,
		Key: map[string]types.AttributeValue{
			"runnerId": &types.AttributeValueMemberS{Value: runnerID},
		},
	})
	if err != nil {
		return fmt.Errorf("delete item: %w", err)
	}
	return nil
}

func isConditionalCheckFailed(err error, target **types.ConditionalCheckFailedException) bool {
	var condErr *types.ConditionalCheckFailedException
	if errors.As(err, &condErr) {
		*target = condErr
		return true
	}
	return false
}
