package gormshared

import "time"

// nowUTC 返回 UTC 当前时间。所有落库时间统一 UTC（§Global Constraints）。
func nowUTC() time.Time { return time.Now().UTC() }
