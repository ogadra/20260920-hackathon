// Package store は Runner の永続化層を提供する。
package store

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/ogadra/bunshin/broker/model"
)

// FirestoreCollection は runner document を格納する Firestore collection 名。
const FirestoreCollection = "runners"

// FieldPrivateHost / FieldCurrentSessionID は runner document の field 名。DynamoDB 側の属性名と揃える。
const (
	FieldPrivateHost      = "privateHost"
	FieldCurrentSessionID = "currentSessionId"
)

type runnerDoc struct {
	RunnerID         string
	PrivateHost      string
	CurrentSessionID string
}

func (d runnerDoc) toModel() *model.Runner {
	r := &model.Runner{
		RunnerID:         d.RunnerID,
		PrivateHost:      d.PrivateHost,
		CurrentSessionID: d.CurrentSessionID,
	}
	if d.CurrentSessionID == "" {
		r.State = model.StateIdle
	} else {
		r.State = model.StateBusy
	}
	return r
}

// FirestoreDocSnapshot は Firestore query から返る 1 doc 分。
type FirestoreDocSnapshot struct {
	ID   string
	Data map[string]any
}

// FirestoreClientAPI は SDK 呼出を semantic 単位にまとめる。
// SDK 型を露出せず map[string]any / bool / 独自 interface だけを扱うことで
// FirestoreRepository を mock でテストする。DynamoDBAPI 1 段構成と対称。
// 実装は broker/store/firestoreadapter package。
type FirestoreClientAPI interface {
	Create(ctx context.Context, runnerID string, data map[string]any) error
	Get(ctx context.Context, runnerID string) (data map[string]any, exists bool, err error)
	Delete(ctx context.Context, runnerID string) error
	QueryIdleRange(ctx context.Context, after, upTo string, limit int) ([]FirestoreDocSnapshot, error)
	IterBusy(ctx context.Context) FirestoreDocIter
	QueryBySession(ctx context.Context, sessionID string) (id string, data map[string]any, exists bool, err error)
	RunTx(ctx context.Context, fn func(tx FirestoreTx) error) error
	Close() error
}

type FirestoreDocIter interface {
	Next() (id string, data map[string]any, done bool, err error)
	Stop()
}

type FirestoreTx interface {
	Get(runnerID string) (data map[string]any, exists bool, err error)
	Update(runnerID, field string, value any) error
}

type FirestoreRepository struct {
	api       FirestoreClientAPI
	randHexFn func() string
	logFn     func(format string, args ...any)
}

// NewFirestoreRepositoryWithAPI は FirestoreClientAPI を差し替え可能にする test-friendly コンストラクタ。
// production では firestoreadapter package の adapter を渡して組み立てる。
func NewFirestoreRepositoryWithAPI(api FirestoreClientAPI) *FirestoreRepository {
	return &FirestoreRepository{
		api:       api,
		randHexFn: defaultRandHexFn,
		logFn:     log.Printf,
	}
}

// Close は underlying Firestore client の gRPC connection を閉じる。
// Repository interface には含めず FirestoreRepository 固有 API とする
// (DynamoDB SDK は per-repo で明示 close する対象を持たないため対称化しない)。
func (r *FirestoreRepository) Close() error {
	return r.api.Close()
}

func (r *FirestoreRepository) Register(ctx context.Context, runnerID, privateHost string) error {
	if !runnerIDRe.MatchString(runnerID) {
		return ErrInvalidRunnerID
	}
	if privateHost == "" {
		return ErrInvalidPrivateHost
	}
	return r.api.Create(ctx, runnerID, map[string]any{
		FieldPrivateHost:      privateHost,
		FieldCurrentSessionID: nil,
	})
}

// AcquireIdle は (start, ∞) → [∅, start] の 2 segment を acquireQueryLimit 件ずつ paginate し、
// AssignSession で precondition 競合した runner は tried に記録して次候補へ進む。
// 全 segment を辿り切れば idle 枯渇として ErrNoIdleRunner を返す (DynamoDB 側と同構造)。
func (r *FirestoreRepository) AcquireIdle(ctx context.Context, sessionID string) (*model.Runner, error) {
	tried := map[string]struct{}{}
	start := r.randHexFn()
	for _, seg := range [][2]string{{start, ""}, {"", start}} {
		after, upTo := seg[0], seg[1]
		for {
			snaps, err := r.api.QueryIdleRange(ctx, after, upTo, acquireQueryLimit)
			if err != nil {
				return nil, err
			}
			if len(snaps) == 0 {
				break
			}
			runner, err := r.assignFirstIdle(ctx, sessionID, snaps, tried)
			if runner != nil || err != nil {
				return runner, err
			}
			after = snaps[len(snaps)-1].ID
			if len(snaps) < acquireQueryLimit {
				break
			}
		}
	}
	return nil, ErrNoIdleRunner
}

func (r *FirestoreRepository) assignFirstIdle(ctx context.Context, sessionID string, candidates []FirestoreDocSnapshot, tried map[string]struct{}) (*model.Runner, error) {
	for _, s := range candidates {
		if _, done := tried[s.ID]; done {
			continue
		}
		doc, err := snapshotToDoc(s.ID, s.Data)
		if err != nil {
			r.logFn("firestore AcquireIdle: skip malformed runner doc: %v", err)
			tried[s.ID] = struct{}{}
			continue
		}
		err = r.assignSession(ctx, s.ID, sessionID)
		if err == nil {
			return &model.Runner{
				RunnerID:         s.ID,
				State:            model.StateBusy,
				PrivateHost:      doc.PrivateHost,
				CurrentSessionID: sessionID,
			}, nil
		}
		if !errors.Is(err, ErrConditionFailed) {
			return nil, err
		}
		tried[s.ID] = struct{}{}
	}
	return nil, nil
}

func (r *FirestoreRepository) assignSession(ctx context.Context, runnerID, sessionID string) error {
	return r.api.RunTx(ctx, func(tx FirestoreTx) error {
		data, exists, err := tx.Get(runnerID)
		if err != nil {
			return err
		}
		if !exists {
			return ErrConditionFailed
		}
		if v, ok := data[FieldCurrentSessionID]; !ok || v != nil {
			return ErrConditionFailed
		}
		return tx.Update(runnerID, FieldCurrentSessionID, sessionID)
	})
}

func (r *FirestoreRepository) ListBusyRunners(ctx context.Context) ([]model.Runner, error) {
	iter := r.api.IterBusy(ctx)
	defer iter.Stop()
	var runners []model.Runner
	for {
		id, data, done, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if done {
			return runners, nil
		}
		doc, err := snapshotToDoc(id, data)
		if err != nil {
			r.logFn("firestore ListBusyRunners: skip malformed runner doc: %v", err)
			continue
		}
		runners = append(runners, *doc.toModel())
	}
}

func (r *FirestoreRepository) FindBySessionID(ctx context.Context, sessionID string) (*model.Runner, error) {
	id, data, exists, err := r.api.QueryBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	doc, err := snapshotToDoc(id, data)
	if err != nil {
		return nil, err
	}
	return doc.toModel(), nil
}

func (r *FirestoreRepository) FindByID(ctx context.Context, runnerID string) (*model.Runner, error) {
	data, exists, err := r.api.Get(ctx, runnerID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	doc, err := snapshotToDoc(runnerID, data)
	if err != nil {
		return nil, err
	}
	return doc.toModel(), nil
}

func (r *FirestoreRepository) Delete(ctx context.Context, runnerID string) error {
	return r.api.Delete(ctx, runnerID)
}

// snapshotToDoc は Firestore document snapshot を厳格に runnerDoc に変換する。
// privateHost 不在 / 非 string、currentSessionId 不在、currentSessionId が nil でも string でもない
// 場合はエラーで返し、silent に空値を上位に流さない。
func snapshotToDoc(runnerID string, data map[string]any) (*runnerDoc, error) {
	priv, ok := data[FieldPrivateHost].(string)
	if !ok {
		return nil, fmt.Errorf("firestore doc %q: privateHost missing or not string", runnerID)
	}
	doc := &runnerDoc{RunnerID: runnerID, PrivateHost: priv}
	v, exists := data[FieldCurrentSessionID]
	if !exists {
		return nil, fmt.Errorf("firestore doc %q: currentSessionId field missing", runnerID)
	}
	if v == nil {
		return doc, nil
	}
	s, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("firestore doc %q: currentSessionId not string or null", runnerID)
	}
	doc.CurrentSessionID = s
	return doc, nil
}
