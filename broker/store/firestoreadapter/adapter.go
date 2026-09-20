// Package firestoreadapter は Firestore SDK を broker/store の FirestoreClientAPI に適合させる薄い変換層。
// SDK 具象型 (*firestore.Client / DocumentIterator / Transaction) を map[string]any とプレーンな
// interface に落として渡すことで store 側を SDK 依存なしに unit test 可能にしている。
// この package 自体は emulator / production への実接続でしか動かないため
// Dockerfile 側の -coverpkg で unit test coverage 対象から除外する (integration test で担保)。
package firestoreadapter

import (
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/ogadra/bunshin/broker/store"
)

// NewRepository は指定した projectID / databaseID の Firestore client を作り Repository を返す。
// 認証は ADC (GKE では Workload Identity で Pod に注入される)。emulator を使う場合は
// FIRESTORE_EMULATOR_HOST を環境変数に設定しておくと SDK が自動的に emulator に接続する。
func NewRepository(ctx context.Context, projectID, databaseID string) (*store.FirestoreRepository, error) {
	client, err := firestore.NewClientWithDatabase(ctx, projectID, databaseID)
	if err != nil {
		return nil, fmt.Errorf("firestore client: %w", err)
	}
	return store.NewFirestoreRepositoryWithAPI(&Adapter{client: client}), nil
}

// Adapter は store.FirestoreClientAPI を Firestore SDK で実装する。
type Adapter struct {
	client *firestore.Client
}

func (a *Adapter) Create(ctx context.Context, runnerID string, data map[string]any) error {
	_, err := a.client.Collection(store.FirestoreCollection).Doc(runnerID).Create(ctx, data)
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return store.ErrConflict
		}
		return fmt.Errorf("firestore create: %w", err)
	}
	return nil
}

func (a *Adapter) Get(ctx context.Context, runnerID string) (map[string]any, bool, error) {
	snap, err := a.client.Collection(store.FirestoreCollection).Doc(runnerID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("firestore get: %w", err)
	}
	return snap.Data(), true, nil
}

func (a *Adapter) Delete(ctx context.Context, runnerID string) error {
	if _, err := a.client.Collection(store.FirestoreCollection).Doc(runnerID).Delete(ctx); err != nil {
		return fmt.Errorf("firestore delete: %w", err)
	}
	return nil
}

// QueryIdleRange は currentSessionId == nil の doc を __name__ 昇順で走査する。
// after (exclusive) と upTo (inclusive) で範囲を絞り、limit で 1 ページ分だけ返す。
// 空文字列を渡すとその側の境界は無効化される。
func (a *Adapter) QueryIdleRange(ctx context.Context, after, upTo string, limit int) ([]store.FirestoreDocSnapshot, error) {
	q := a.client.Collection(store.FirestoreCollection).
		Where(store.FieldCurrentSessionID, "==", nil).
		OrderBy(firestore.DocumentID, firestore.Asc)
	if after != "" {
		q = q.StartAfter(after)
	}
	if upTo != "" {
		q = q.EndAt(upTo)
	}
	iter := q.Limit(limit).Documents(ctx)
	defer iter.Stop()

	var snaps []store.FirestoreDocSnapshot
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			return snaps, nil
		}
		if err != nil {
			return nil, fmt.Errorf("firestore query idle range: %w", err)
		}
		snaps = append(snaps, store.FirestoreDocSnapshot{ID: snap.Ref.ID, Data: snap.Data()})
	}
}

func (a *Adapter) IterBusy(ctx context.Context) store.FirestoreDocIter {
	return &docIter{
		iter: a.client.Collection(store.FirestoreCollection).
			Where(store.FieldCurrentSessionID, "!=", nil).
			Documents(ctx),
	}
}

func (a *Adapter) QueryBySession(ctx context.Context, sessionID string) (string, map[string]any, bool, error) {
	iter := a.client.Collection(store.FirestoreCollection).
		Where(store.FieldCurrentSessionID, "==", sessionID).
		Limit(1).
		Documents(ctx)
	defer iter.Stop()
	snap, err := iter.Next()
	if errors.Is(err, iterator.Done) {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, fmt.Errorf("firestore find by session: %w", err)
	}
	return snap.Ref.ID, snap.Data(), true, nil
}

func (a *Adapter) RunTx(ctx context.Context, fn func(tx store.FirestoreTx) error) error {
	err := a.client.RunTransaction(ctx, func(ctx context.Context, sdkTx *firestore.Transaction) error {
		return fn(&tx{client: a.client, tx: sdkTx})
	})
	if err != nil {
		if errors.Is(err, store.ErrConditionFailed) {
			return store.ErrConditionFailed
		}
		return fmt.Errorf("firestore run tx: %w", err)
	}
	return nil
}

func (a *Adapter) Close() error {
	return a.client.Close()
}

type docIter struct {
	iter *firestore.DocumentIterator
}

func (d *docIter) Next() (string, map[string]any, bool, error) {
	snap, err := d.iter.Next()
	if errors.Is(err, iterator.Done) {
		return "", nil, true, nil
	}
	if err != nil {
		return "", nil, false, fmt.Errorf("firestore iter next: %w", err)
	}
	return snap.Ref.ID, snap.Data(), false, nil
}

func (d *docIter) Stop() {
	d.iter.Stop()
}

type tx struct {
	client *firestore.Client
	tx     *firestore.Transaction
}

func (t *tx) Get(runnerID string) (map[string]any, bool, error) {
	snap, err := t.tx.Get(t.client.Collection(store.FirestoreCollection).Doc(runnerID))
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("firestore tx get: %w", err)
	}
	return snap.Data(), true, nil
}

func (t *tx) Update(runnerID, field string, value any) error {
	ref := t.client.Collection(store.FirestoreCollection).Doc(runnerID)
	if err := t.tx.Update(ref, []firestore.Update{{Path: field, Value: value}}); err != nil {
		return fmt.Errorf("firestore tx update: %w", err)
	}
	return nil
}
