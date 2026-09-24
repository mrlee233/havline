package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestNewSkipsNonPositiveIntervals：time.NewTicker 对非正间隔会 panic，这类任务必须在建表时丢掉。
func TestNewSkipsNonPositiveIntervals(t *testing.T) {
	s := New(
		Job{Name: "zero", Interval: 0, Run: func(context.Context) {}},
		Job{Name: "negative", Interval: -time.Second, Run: func(context.Context) {}},
		Job{Name: "ok", Interval: time.Hour, Run: func(context.Context) {}},
	)
	if len(s.jobs) != 1 || s.jobs[0].Name != "ok" {
		t.Fatalf("非正间隔的任务应被跳过，实际保留 %d 个", len(s.jobs))
	}
}

// TestRunJobRecoversFromPanic：单个任务 panic 不能带走整个进程，且要能继续下一轮。
func TestRunJobRecoversFromPanic(t *testing.T) {
	var calls int32
	s := New(Job{Name: "panic", Interval: 5 * time.Millisecond, Run: func(context.Context) {
		atomic.AddInt32(&calls, 1)
		panic("boom")
	}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && atomic.LoadInt32(&calls) < 3 {
		time.Sleep(5 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&calls); got < 3 {
		t.Fatalf("panic 后应继续后续轮次，实际只执行了 %d 次", got)
	}
}
