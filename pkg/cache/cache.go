// Package cache 提供可插拔的短 TTL 缓存层，供 RBAC 角色、数据隔离策略、
// 每日简报单飞等"读多写少、可容忍秒级不一致"的场景共用。
//
// 双驱动（按配置 cache.driver 选择，默认 memory）：
//   - memory  ：进程内 map + RWMutex + 惰性过期。行为与早期内联缓存完全等价，
//     是"未配置外部缓存"时的兜底实现，零外部依赖、零回归。
//   - kvrocks ：GoFrame gredis 连 Apache Kvrocks（Redis 协议完全兼容），
//     SET EX / SET NX EX / GET / DEL。用于多实例部署时共享缓存、
//     保证跨实例一致（角色缓存、隔离策略、简报单飞去重）。
//     也可连标准 Redis；Kvrocks 与 Redis 走同一 Redis 协议，客户端无差别。
//
// 值统一为 string，结构体由调用方 JSON 序列化后存取（本包不感知业务类型）。
package cache

import (
	"context"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"vouchchain/internal/preflight"
)

// init 确保配置文件存在后再做任何配置读取。
// 背景：internal/middleware 的包级变量 isoStore = cache.New(...) 在包初始化阶段就执行，
// 内部调用 g.Cfg().MustGet，而 GoFrame 无配置文件时 MustGet 会 panic。
// Go 的包级变量初始化先于 main()，所以必须在 cache 包 init() 里先生成 config.yml/data。
func init() {
	if err := preflight.EnsureDefaultConfig(); err != nil {
		panic(err)
	}
}

// Store 缓存驱动接口。所有方法均带 ctx，超时由实现自控。
type Store interface {
	// Get 读 key；ok=false 表示不存在（miss）。err 非 nil 表示后端异常（调用方可降级）。
	Get(ctx context.Context, key string) (val string, ok bool, err error)
	// Set 写 key，ttl 过期后自动消失；ttl<=0 表示永不过期。
	Set(ctx context.Context, key string, val string, ttl time.Duration) error
	// SetNX 仅当 key 不存在时写入并返回 true；已存在则返回 false。
	SetNX(ctx context.Context, key string, val string, ttl time.Duration) (bool, error)
	// Del 删除 key；不存在的 key 静默成功。
	Del(ctx context.Context, key string) error
}

// New 返回进程级单例驱动实例（首次按配置决定，此后固定）。
// driver 取值：memory（默认）/ kvrocks（或 redis）。未配置或配置为空一律回落 memory。
// 单例保证：memory 模式同一进程共享同一份缓存；kvrocks 模式复用同一 Redis 连接。
func New(ctx context.Context) Store {
	storeOnce.Do(func() {
		driver := g.Cfg().MustGet(ctx, "cache.driver", "memory").String()
		switch driver {
		case "kvrocks", "redis":
			defaultStore = &redisStore{}
		default:
			defaultStore = &memoryStore{}
		}
	})
	return defaultStore
}

var (
	storeOnce    sync.Once
	defaultStore Store
)

// ---------- memory 驱动：进程内 map + 惰性过期（默认，行为与现状等价） ----------

type memEntry struct {
	val string
	exp time.Time // 零值 = 永不过期
}

type memoryStore struct {
	mu sync.RWMutex
	m  map[string]memEntry
}

func (s *memoryStore) Get(_ context.Context, key string) (string, bool, error) {
	s.mu.RLock()
	e, ok := s.m[key]
	s.mu.RUnlock()
	if !ok {
		return "", false, nil
	}
	if !e.exp.IsZero() && time.Now().After(e.exp) {
		s.Del(context.Background(), key) // 惰性清理
		return "", false, nil
	}
	return e.val, true, nil
}

func (s *memoryStore) Set(_ context.Context, key string, val string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]memEntry{}
	}
	e := memEntry{val: val}
	if ttl > 0 {
		e.exp = time.Now().Add(ttl)
	}
	s.m[key] = e
	return nil
}

func (s *memoryStore) SetNX(_ context.Context, key string, val string, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]memEntry{}
	}
	if e, ok := s.m[key]; ok {
		if e.exp.IsZero() || time.Now().Before(e.exp) {
			return false, nil
		}
	}
	e := memEntry{val: val}
	if ttl > 0 {
		e.exp = time.Now().Add(ttl)
	}
	s.m[key] = e
	return true, nil
}

func (s *memoryStore) Del(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
	return nil
}

// ---------- kvrocks 驱动：GoFrame gredis 连 Kvrocks（Redis 协议） ----------

// redisStore 复用 GoFrame 全局 g.Redis()（读配置 redis.default 节点），懒连接。
type redisStore struct{}

func (s *redisStore) Get(ctx context.Context, key string) (string, bool, error) {
	v, err := g.Redis().Do(ctx, "GET", key)
	if err != nil {
		return "", false, err
	}
	if v.IsNil() || v.String() == "" {
		return "", false, nil
	}
	return v.String(), true, nil
}

func (s *redisStore) Set(ctx context.Context, key string, val string, ttl time.Duration) error {
	if ttl <= 0 {
		_, err := g.Redis().Do(ctx, "SET", key, val)
		return err
	}
	_, err := g.Redis().Do(ctx, "SET", key, val, "EX", int64(ttl.Seconds()))
	return err
}

func (s *redisStore) SetNX(ctx context.Context, key string, val string, ttl time.Duration) (bool, error) {
	v, err := g.Redis().Do(ctx, "SET", key, val, "NX", "EX", int64(ttl.Seconds()))
	if err != nil {
		return false, err
	}
	return v.String() == "OK", nil
}

func (s *redisStore) Del(ctx context.Context, key string) error {
	_, err := g.Redis().Do(ctx, "DEL", key)
	return err
}
