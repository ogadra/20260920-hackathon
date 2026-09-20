// Package store は Runner の永続化層を提供する。
package store

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math/rand/v2"
	"regexp"

	"github.com/ogadra/bunshin/broker/model"
)

var (
	ErrNotFound           = errors.New("runner not found")
	ErrNoIdleRunner       = errors.New("no idle runner available")
	ErrConditionFailed    = errors.New("condition check failed")
	ErrConflict           = errors.New("runner already exists with different attributes")
	ErrInvalidRunnerID    = errors.New("runnerID must be 32 lowercase hex characters")
	ErrInvalidPrivateHost = errors.New("privateHost must not be empty")
)

// AcquireIdle が 1 ページで取得する idle 候補の件数上限。dynamo / firestore 両実装で共有する。
// stale item に当たっても同ページ内の次候補で assign を試せるよう複数取る。
const acquireQueryLimit = 5

// AcquireIdle は runnerId の lex 順で 2 segment を走査するため、
// 32 桁小文字 hex 以外を書き込むと backend の順序が崩れ取りこぼす。Register で形式を強制する。
// dynamo / firestore 両実装で共有する。
var runnerIDRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

// defaultRandHexFn は AcquireIdle の走査開始位置を返す。dynamo / firestore 両実装で共有する。
// 暗号強度を要求しないので crypto/rand ではなく math/rand/v2 を使う。
func defaultRandHexFn() string {
	var b [16]byte
	binary.LittleEndian.PutUint64(b[:8], rand.Uint64())
	binary.LittleEndian.PutUint64(b[8:], rand.Uint64())
	return hex.EncodeToString(b[:])
}

type Repository interface {
	Register(ctx context.Context, runnerID, privateHost string) error
	AcquireIdle(ctx context.Context, sessionID string) (*model.Runner, error)
	ListBusyRunners(ctx context.Context) ([]model.Runner, error)
	FindBySessionID(ctx context.Context, sessionID string) (*model.Runner, error)
	FindByID(ctx context.Context, runnerID string) (*model.Runner, error)
	Delete(ctx context.Context, runnerID string) error
}
