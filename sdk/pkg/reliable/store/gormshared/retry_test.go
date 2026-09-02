package gormshared

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// —— retryTransient（评审 D1/P0 + R5 修复的第二层）——
//
// 对驱动已识别的瞬态错误（1213/1205/40001 等 classifier 判 ClassRetryable）做有界毫秒级
// 重试，不消耗 attempt 预算。以下用 stub classifier + 计数 op 驱动全部行为分支。

// err1213 构造一个真实的 *mysqldriver.MySQLError（Number=1213），保证 classifier 的
// errors.As 路径与生产一致（不串字符串匹配）。
func err1213() error {
	return &mysqldriver.MySQLError{Number: 1213, Message: "Deadlock found when trying to get lock"}
}

// err1062 唯一键冲突——ClassConflict，必须单次即返。
func err1062() error {
	return &mysqldriver.MySQLError{Number: 1062, Message: "Duplicate entry"}
}

// flakyOp 返回一个前 failN 次失败、之后成功的 op，并记录调用次数。
func flakyOp(failN int, failErr error) (func() error, *int) {
	calls := 0
	return func() error {
		calls++
		if calls <= failN {
			return failErr
		}
		return nil
	}, &calls
}

// stubSlowClassifier 不参与（测试直接用 MySQLClassifier{}——真实实现，零 stub 漂移）。

func TestRetryTransient_ConvergesAfterTransientFailures(t *testing.T) {
	s := &GormStore{classifier: mysqlClassifierForTest()}
	op, calls := flakyOp(2, err1213())
	start := time.Now()
	err := s.retryTransient(context.Background(), op)
	require.NoError(t, err)
	assert.Equal(t, 3, *calls, "2 transient failures then success → 3 calls")
	// 2 次退避（50~200ms 随机）都发生：总耗时下限 > 100ms（两段各 ≥50ms）。
	assert.Greater(t, time.Since(start), 100*time.Millisecond, "two backoffs must have elapsed")
	assert.Less(t, time.Since(start), 2*time.Second, "bounded: no runaway wait")
}

func TestRetryTransient_ExhaustsAndPropagatesOriginal(t *testing.T) {
	s := &GormStore{classifier: mysqlClassifierForTest()}
	always := err1213()
	calls := 0
	err := s.retryTransient(context.Background(), func() error {
		calls++
		return always
	})
	require.Error(t, err)
	// 上抛的是原始 1213（可被外层 ClassifyDriver 识别），不是别的包装。
	var me *mysqldriver.MySQLError
	assert.True(t, errors.As(err, &me) && me.Number == 1213,
		"must propagate the original transient error verbatim")
	assert.Equal(t, 4, *(&calls), "1 initial + 3 retries = 4 calls")
}

func TestRetryTransient_CtxCancelled_ReturnsImmediately(t *testing.T) {
	s := &GormStore{classifier: mysqlClassifierForTest()}
	calls := 0
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := s.retryTransient(ctx, func() error {
		calls++
		return err1213()
	})
	assert.ErrorIs(t, err, context.Canceled, "cancelled ctx must surface ctx error, not retry")
	assert.Equal(t, 1, calls, "no retries after ctx cancellation")
}

func TestRetryTransient_NonTransient_NoRetry(t *testing.T) {
	s := &GormStore{classifier: mysqlClassifierForTest()}
	op, calls := flakyOp(math.MaxInt32, err1062())
	err := s.retryTransient(context.Background(), op)
	require.Error(t, err)
	var me *mysqldriver.MySQLError
	assert.True(t, errors.As(err, &me) && me.Number == 1062)
	assert.Equal(t, 1, *calls, "conflict-class driver error must not be retried")
}

func TestRetryTransient_UnknownError_NoRetry(t *testing.T) {
	s := &GormStore{classifier: mysqlClassifierForTest()}
	bogus := errors.New("some unknown error")
	op, calls := flakyOp(math.MaxInt32, bogus)
	err := s.retryTransient(context.Background(), op)
	assert.ErrorIs(t, err, bogus)
	assert.Equal(t, 1, *calls, "unrecognized error must not be retried")
}

// mysqlClassifierForTest 返回真实 MySQLClassifier（store/mysql import gormshared，
// 反向 import 会成环，故测试内以 reliable.ErrorClassifier 类型引入——零行为差异，
// ClassifyDriver/IsDuplicateKey/ErrorCode 全部真实实现）。
func mysqlClassifierForTest() reliable.ErrorClassifier {
	type classifier interface {
		reliable.ErrorClassifier
	}
	var c classifier = mysqlTestClassifier{}
	return c
}

// mysqlTestClassifier 内嵌真实实现（gormshared 无法 import store/mysql（环），
// 这里手写同一张映射表——数字集合以 store/mysql/classify.go 为准，单一事实源在那边，
// 若那边增删码位需同步这里）。1150 行重复是显式选择的测试替身边界。
type mysqlTestClassifier struct{}

func (mysqlTestClassifier) ClassifyDriver(err error) (reliable.ErrorClass, bool) {
	var me *mysqldriver.MySQLError
	if !errors.As(err, &me) {
		return "", false
	}
	switch me.Number {
	case 1062:
		return reliable.ClassConflict, true
	case 1213, 1205, 1040, 1053, 1452:
		return reliable.ClassRetryable, true
	case 1146, 1054, 1170:
		return reliable.ClassUnrecoverable, true
	}
	return "", false
}

func (mysqlTestClassifier) IsDuplicateKey(err error) bool {
	var me *mysqldriver.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

func (mysqlTestClassifier) ErrorCode(err error) (string, bool) {
	var me *mysqldriver.MySQLError
	if errors.As(err, &me) {
		return "mysql", true
	}
	return "", false
}
