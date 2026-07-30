package reliable

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPermanentWrapsAndUnwraps(t *testing.T) {
	root := errors.New("boom")
	wrapped := Permanent(root)
	assert.True(t, IsPermanent(wrapped))
	assert.False(t, IsRetryable(wrapped))
	assert.ErrorIs(t, wrapped, root, "Unwrap must expose root cause")
	_ = fmt.Sprintf("%v", root)
}

func TestRetryableWrapsAndUnwraps(t *testing.T) {
	root := errors.New("transient")
	wrapped := Retryable(root)
	assert.True(t, IsRetryable(wrapped))
	assert.False(t, IsPermanent(wrapped))
	assert.ErrorIs(t, wrapped, root)
}

func TestSentinelsAreIdentityEqual(t *testing.T) {
	// sentinels 必须是同一指针——C5 现网 bug 正是 ErrDuplicateKey 分裂成两个变量导致 errors.Is 恒 false。
	assert.Same(t, ErrDuplicateKey, ErrDuplicateKey)
	assert.True(t, errors.Is(fmt.Errorf("wrap: %w", ErrSkip), ErrSkip))
	assert.True(t, errors.Is(fmt.Errorf("wrap: %w", ErrRetryLater), ErrRetryLater))
}
