package logger

import (
	"runtime"
	"strings"
	"testing"
)

// TestCallerStackDebug 调试调用栈
func TestCallerStackDebug(t *testing.T) {
	pcs := make([]uintptr, 30)
	depth := runtime.Callers(0, pcs)
	frames := runtime.CallersFrames(pcs[:depth])
	
	t.Logf("Total frames: %d", depth)
	
	i := 0
	for {
		frame, more := frames.Next()
		file := frame.File
		fn := frame.Function
		
		isLoggerImpl := (strings.Contains(file, "/logger/") && !strings.HasSuffix(file, "_test.go")) || 
		                 (strings.Contains(fn, "github.com/go-admin-team/go-admin-core/logger") && !strings.Contains(fn, "_test."))
		isLogrus := strings.Contains(file, "/logrus@")
		isTesting := strings.Contains(file, "/testing/") || strings.Contains(fn, "testing.")
		isRuntime := strings.Contains(file, "/runtime/") || strings.Contains(fn, "runtime.")
		
		t.Logf("[%d] %s:%d %s | loggerImpl=%v logrus=%v testing=%v runtime=%v",
			i, file, frame.Line, fn, isLoggerImpl, isLogrus, isTesting, isRuntime)
		
		if !isLoggerImpl && !isLogrus && !isTesting && !isRuntime {
			t.Logf("✅ Found business code at frame %d: %s:%d", i, file, frame.Line)
			break
		}
		
		if !more {
			break
		}
		i++
	}
}

// TestCallerStackInLog 在 Log 方法中调试
func TestCallerStackInLog(t *testing.T) {
	log := NewLogrusLogger(WithLevel(DebugLevel))
	
	// 这会触发 Log 方法 → getEntryWithCaller
	t.Log("Before calling log.Log")
	log.Log(InfoLevel, "test message")  // 这一行是第 48 行
	t.Log("After calling log.Log")
}
