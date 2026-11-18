package cache

import (
	"context"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gredis"
)

// 微基准：对比 memory（进程内）与 kvrocks（经 SSH 隧道连 instabox 的 Apache Kvrocks）
// 的 Set/Get 原始延迟，用于评估"AI 对话上下文注入用 KV 是否更快"。
// 运行前提：SSH 隧道已建（127.0.0.1:6666 → instabox:6666）。无隧道时 kvrocks bench 会报错。

var benchCtx = context.Background()

func BenchmarkMemorySetGet(b *testing.B) {
	s := &memoryStore{}
	key, val := "bench:key", "hello world this is a cached value payload"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.Set(benchCtx, key, val, time.Minute)
		_, _, _ = s.Get(benchCtx, key)
	}
}

func BenchmarkKvrocksSetGet(b *testing.B) {
	r, _ := gredis.New(&gredis.Config{Address: "127.0.0.1:6666", Db: 0})
	key, val := "bench:key", "hello world this is a cached value payload"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.Do(benchCtx, "SET", key, val, "EX", 60)
		_, _ = r.Do(benchCtx, "GET", key)
	}
}

func BenchmarkMemoryGet(b *testing.B) {
	s := &memoryStore{}
	_ = s.Set(benchCtx, "bench:key", "hello world this is a cached value payload", time.Minute)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = s.Get(benchCtx, "bench:key")
	}
}

func BenchmarkKvrocksGet(b *testing.B) {
	r, _ := gredis.New(&gredis.Config{Address: "127.0.0.1:6666", Db: 0})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.Do(benchCtx, "GET", "bench:key")
	}
}
