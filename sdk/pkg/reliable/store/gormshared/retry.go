package gormshared

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
)

// retryTransient 对驱动已识别的瞬态错误做有界毫秒级重试（评审 D1/P0 + R5 第二层）。
//
// 背景：2026-08-23 事故里 MarkFailed 的 SELECT/UPDATE 撞上死锁风暴（Error 1213，每分钟 5~14 次），
// 31 秒烧完 5 次 attempt 预算后消息被判死。InnoDB 回滚牺牲者后另一方已持有全部锁继续——冲突
// 窗口已消失，毫秒级重试的成功率极高。在 store 层就地吸收瞬态错误：
//   - 判定用注入的 s.classifier.ClassifyDriver（MySQL 1213/1205/1040/1053/1452，PG 40001 等
//     双方言自动覆盖，不硬编码错误码）；
//   - 不消耗 attempt 预算（基础设施层自愈，非业务失败——重试次数与 MarkFailed 的
//     maxAttempts 语义完全解耦）；
//   - ctx 取消/超时立即上抛（关停不被拖住）；
//   - 非瞬态码（POISON/CONFLICT/未识别）单次即返——绝不能把 fencing 冲突也重试掉。
//
// 前置依赖：R5 修复（mark.go 的 SELECT 不再把一切错误伪装成 ErrConflict）——否则被伪装的
// 哨兵在这里被 classifier 判"未识别"而错过重试，等于蒙眼开车。
func (s *GormStore) retryTransient(ctx context.Context, op func() error) error {
	const maxRetries = 3
	var err error
	for i := 0; i <= maxRetries; i++ {
		if err = op(); err == nil {
			return nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if c, ok := s.classifier.ClassifyDriver(err); !ok || c != reliable.ClassRetryable {
			return err
		}
		if i == maxRetries {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(50+rand.IntN(150)) * time.Millisecond): // 50~200ms 随机退避
		}
	}
	return err
}
