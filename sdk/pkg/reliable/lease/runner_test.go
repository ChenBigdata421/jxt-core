package lease

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// D21（本轮评审）：原稿用一个假类型作 *gorm.DB 占位，并靠一条「稍后替换」注记补齐——
// 那是占位，且 T12 的门禁抓不到。现直接写真实签名 `*gorm.DB`（fake 不解引用，
// 调用方传 nil 即可），并用编译期断言钉住接口完整性。
var _ store.Store = (*fakeStore)(nil)

// fakeStore 只实现 Runner 用到的方法（其余 no-op）；记录 ObserveExpiredLeases 调用。
type fakeStore struct {
	observed int
	err      error
	calls    int32
}

func (f *fakeStore) TryClaim(context.Context, reliable.ClaimInput, time.Duration) (reliable.ClaimToken, reliable.Decision, error) {
	return "", 0, nil
}
func (f *fakeStore) ObserveExpiredLeases(_ context.Context, _ time.Time) (int, error) {
	atomic.AddInt32(&f.calls, 1)
	if f.err != nil {
		return 0, f.err
	}
	return f.observed, nil
}

// 其余 store.Store 方法 no-op（runner 不调用）。
func (f *fakeStore) MarkSucceeded(context.Context, *gorm.DB, reliable.Key, reliable.ClaimToken) error {
	return nil
}
func (f *fakeStore) MarkFailed(context.Context, *gorm.DB, reliable.Key, reliable.ClaimToken, reliable.ErrorClass, reliable.ReplaySafety, int, error, []byte) error {
	return nil
}
func (f *fakeStore) RecordTerminal(context.Context, *gorm.DB, reliable.ClaimInput, reliable.ErrorClass, error, []byte) error {
	return nil
}
func (f *fakeStore) FindEligibleHeads(context.Context, time.Time, int) ([]store.Row, error) {
	return nil, nil
}
func (f *fakeStore) ClaimForReplay(context.Context, *gorm.DB, int64) (reliable.ClaimToken, store.Row, error) {
	return "", store.Row{}, nil
}
func (f *fakeStore) ReleaseClaim(context.Context, *gorm.DB, int64, reliable.ClaimToken) error {
	return nil
}
func (f *fakeStore) AdvanceDue(context.Context, *gorm.DB, int64) error               { return nil }
func (f *fakeStore) MoveToDeadLetter(context.Context, *gorm.DB, int64, string) error { return nil }
func (f *fakeStore) MoveToDeadLetterWithToken(context.Context, *gorm.DB, int64, reliable.ClaimToken, reliable.ErrorClass, string) error {
	return nil
}
func (f *fakeStore) ScheduleReplay(context.Context, *gorm.DB, int64, int64, string, string, string) error {
	return nil
}
func (f *fakeStore) Discard(context.Context, *gorm.DB, int64, int64, string, string) error {
	return nil
}
func (f *fakeStore) AcquireAggregateGate(context.Context, *gorm.DB, reliable.AggregateGateKey, string, time.Duration) (string, error) {
	return "", nil
}
func (f *fakeStore) ReleaseAggregateGate(context.Context, *gorm.DB, string) error { return nil }
func (f *fakeStore) ReclaimExpiredAggregateGates(context.Context, time.Time) (int, error) {
	return 0, nil
}
func (f *fakeStore) RecordAnomaly(context.Context, *gorm.DB, int, string, reliable.Key, string, string) error {
	return nil
}
func (f *fakeStore) GetByID(context.Context, int, int64) (store.Row, error)      { return store.Row{}, nil }
func (f *fakeStore) List(context.Context, store.ListFilter) ([]store.Row, error) { return nil, nil }

func TestRunnerTickObservesOrphans(t *testing.T) {
	fs := &fakeStore{observed: 3}
	r := NewRunner(fs, nil, nil, time.Second, func() time.Time { return time.Now().UTC() })
	n, err := r.Tick(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 3, n)
	// D20：每 tick 只扫一批，不循环（观测器不改行，循环会死转）。
	assert.Equal(t, int32(1), atomic.LoadInt32(&fs.calls), "D20: exactly one scan per tick")
}

func TestRunnerTickErrorDoesNotPanic(t *testing.T) {
	fs := &fakeStore{err: errors.New("db down")}
	r := NewRunner(fs, nil, nil, time.Second, nil)
	assert.NotPanics(t, func() { r.tick(context.Background()) })
}

func TestRunnerDefaults(t *testing.T) {
	r := NewRunner(&fakeStore{}, nil, nil, 0, nil)
	assert.Equal(t, 30*time.Second, r.interval)
	assert.NotNil(t, r.now)
}
