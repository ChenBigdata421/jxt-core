package reliable

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

// B2（本轮评审）：原稿只实现 ClassifyDriver + IsDuplicateKey，不满足含 ErrorCode 的
// ErrorClassifier 接口 → 作为 driver 参数传给 Classify 时编译不过。三个方法必须齐。
type fakeClassifier struct {
	match error
	class ErrorClass
	dup   error
	code  string
}

func (f fakeClassifier) ClassifyDriver(err error) (ErrorClass, bool) {
	if errors.Is(err, f.match) {
		return f.class, true
	}
	return "", false
}
func (f fakeClassifier) IsDuplicateKey(err error) bool { return errors.Is(err, f.dup) }
func (f fakeClassifier) ErrorCode(err error) (string, bool) {
	if f.code != "" && errors.Is(err, f.match) {
		return f.code, true
	}
	return "", false
}

// 编译期确认 fake 满足接口（防止未来接口加方法时测试静默失效）。
var _ ErrorClassifier = fakeClassifier{}

func TestClassifyLevel1Domain(t *testing.T) {
	root := errors.New("x")
	assert.Equal(t, ClassPoison, Classify(Permanent(root), nil))
	assert.Equal(t, ClassRetryable, Classify(Retryable(root), nil))
	assert.Equal(t, ClassSkip, Classify(errors.Join(errors.New("idem"), ErrSkip), nil))
}

func TestClassifyLevel2Driver(t *testing.T) {
	deadlock := errors.New("driver: deadlock")
	drv := fakeClassifier{match: deadlock, class: ClassRetryable}
	assert.Equal(t, ClassRetryable, Classify(deadlock, drv))

	fk1452 := errors.New("fk parent missing")
	drv1452 := fakeClassifier{match: fk1452, class: ClassRetryable}
	assert.Equal(t, ClassPoison, Classify(Permanent(fk1452), drv1452), "Permanent 覆盖第 2 级")
}

func TestClassifyContextAndNet(t *testing.T) {
	assert.Equal(t, ClassRetryable, Classify(context.DeadlineExceeded, nil))
	assert.Equal(t, ClassRetryable, Classify(context.Canceled, nil))
	assert.Equal(t, ClassRetryable, Classify(timeoutNetErr{}, nil))
}

type timeoutNetErr struct{}

func (timeoutNetErr) Error() string   { return "i/o timeout" }
func (timeoutNetErr) Timeout() bool   { return true }
func (timeoutNetErr) Temporary() bool { return false }

var _ net.Error = timeoutNetErr{}

func TestClassifyFallbackUnrecoverable(t *testing.T) {
	assert.Equal(t, ClassUnrecoverable, Classify(errors.New("something novel"), nil))
}

func TestOutcomeForMatrix(t *testing.T) {
	cases := []struct {
		class  ErrorClass
		safety ReplaySafety
		wantDL bool
	}{
		{ClassRetryable, ReplayIdempotent, false},
		{ClassRetryable, ReplayNeedsTxClaim, false},
		{ClassRetryable, ReplayUnsafe, true},
		{ClassPoison, ReplayIdempotent, true},
		{ClassPoison, ReplayUnsafe, true},
		{ClassUnrecoverable, ReplayIdempotent, true},
		{ClassConflict, ReplayUnsafe, true},
		{ClassSkip, ReplayUnsafe, false},
	}
	for _, c := range cases {
		assert.Equal(t, c.wantDL, OutcomeFor(c.class, c.safety).DeadLetter,
			"class=%s safety=%v", c.class, c.safety)
	}
}
