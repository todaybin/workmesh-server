// SPDX-License-Identifier: LicenseRef-WorkMesh-Pending
// Copyright (c) 2026 WorkMesh contributors

package cache

import (
	"sync"
	"time"
)

type item struct {
	value   any
	expires time.Time
}

// Cache 是默认进程内缓存；Redis 适配器可在后续实现中替换此接口。
type Cache struct {
	mu    sync.RWMutex
	items map[string]item
}

// New 创建一个无后台清理协程的缓存，过期检查在访问时完成以降低空闲开销。
func New() *Cache { return &Cache{items: make(map[string]item)} }

// Get 读取未过期的值。
func (c *Cache) Get(key string) (any, bool) {
	c.mu.RLock()
	entry, ok := c.items[key]
	c.mu.RUnlock()
	if !ok || (!entry.expires.IsZero() && time.Now().After(entry.expires)) {
		if ok {
			c.mu.Lock()
			delete(c.items, key)
			c.mu.Unlock()
		}
		return nil, false
	}
	return entry.value, true
}

// Set 写入一个带 TTL 的值；TTL 小于等于零表示不过期。
func (c *Cache) Set(key string, value any, ttl time.Duration) {
	var expires time.Time
	if ttl > 0 {
		expires = time.Now().Add(ttl)
	}
	c.mu.Lock()
	c.items[key] = item{value: value, expires: expires}
	c.mu.Unlock()
}

// Delete 删除缓存值。
func (c *Cache) Delete(key string) { c.mu.Lock(); delete(c.items, key); c.mu.Unlock() }
