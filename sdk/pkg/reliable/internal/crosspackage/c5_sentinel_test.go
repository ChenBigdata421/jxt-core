// Package crosspackage 的唯一存在理由：在 reliable 包之外断言 core sentinel 的 errors.Is/As 成立。
// 同包内测试用的是同一变量，恒真——而 C5 现网 bug 的真实失效机理正是「persistence 层另立了一个
// 同名 sentinel」，errors.Is 跨包恒 false。这条测试钉住「sentinel 有且仅有一个归属」（§8.3/PR1_SCOPE C5）。
package crosspackage

import (
	"errors"
	"fmt"
	"testing"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/stretchr/testify/assert"
)

func TestCoreSentinelsCrossPackage(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"ErrSkip", fmt.Errorf("x: %w", reliable.ErrSkip)},
		{"ErrDuplicateKey", fmt.Errorf("x: %w", reliable.ErrDuplicateKey)},
		{"ErrRetryLater", fmt.Errorf("x: %w", reliable.ErrRetryLater)},
		{"ErrNotPermitted", fmt.Errorf("x: %w", reliable.ErrNotPermitted)},
		{"ErrNotSelfReplayable", fmt.Errorf("x: %w", reliable.ErrNotSelfReplayable)},
		{"ErrConflict", fmt.Errorf("x: %w", reliable.ErrConflict)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.True(t, errors.Is(c.err, sentinelByName(c.name)), "errors.Is must hold across packages (C5)")
		})
	}
}

func TestTypedWrappersCrossPackage(t *testing.T) {
	root := errors.New("root")
	assert.True(t, reliable.IsPermanent(reliable.Permanent(root)))
	assert.True(t, reliable.IsRetryable(reliable.Retryable(root)))
	var p *reliable.PermanentError
	assert.True(t, errors.As(reliable.Permanent(root), &p))
}

func sentinelByName(name string) error {
	switch name {
	case "ErrSkip":
		return reliable.ErrSkip
	case "ErrDuplicateKey":
		return reliable.ErrDuplicateKey
	case "ErrRetryLater":
		return reliable.ErrRetryLater
	case "ErrNotPermitted":
		return reliable.ErrNotPermitted
	case "ErrNotSelfReplayable":
		return reliable.ErrNotSelfReplayable
	case "ErrConflict":
		return reliable.ErrConflict
	default:
		return nil
	}
}
