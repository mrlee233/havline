package api

import (
	"testing"
	"time"
)

func TestStatusPageLimiterBlocksAfterLimit(t *testing.T) {
	limiter := newStatusPageLimiter()
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }

	for i := 0; i < statusPageLimit; i++ {
		if !limiter.allow("1.2.3.4") {
			t.Fatalf("第 %d 次请求应放行", i+1)
		}
	}
	if limiter.allow("1.2.3.4") {
		t.Fatal("超过配额应拒绝")
	}
	// 限流按 IP 计：别的访客不该被牵连
	if !limiter.allow("5.6.7.8") {
		t.Fatal("其它 IP 不应被牵连")
	}
	// 窗口滑过后恢复
	now = now.Add(statusPageWindow + time.Second)
	if !limiter.allow("1.2.3.4") {
		t.Fatal("窗口过后应恢复放行")
	}
}

func TestHashStatusPageCodeHidesPlaintext(t *testing.T) {
	hash := hashStatusPageCode("my-secret")
	if hash == "my-secret" || len(hash) != 64 {
		t.Fatalf("访问码应以 SHA-256 存储，实际 %q", hash)
	}
	if hashStatusPageCode("  my-secret  ") != hash {
		t.Fatal("前后空白裁剪后应得到同一哈希")
	}
	if hashStatusPageCode("other") == hash {
		t.Fatal("不同访问码不应得到同一哈希")
	}
}
