package auth

import (
	"sync"
	"time"
)

type bucket struct {
	count   int
	expires time.Time
}
type limiter struct {
	mu      sync.Mutex
	buckets map[string]bucket
}

// shortcut: single-instance quotas; use shared counters before running API replicas.
func (l *limiter) allow(key string, limit int, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.buckets == nil {
		l.buckets = make(map[string]bucket)
	}
	b, exists := l.buckets[key]
	if !exists || !now.Before(b.expires) {
		for key, item := range l.buckets {
			if !now.Before(item.expires) {
				delete(l.buckets, key)
			}
		}
		if len(l.buckets) >= 2048 {
			return false
		}
		b = bucket{expires: now.Add(15 * time.Minute)}
	}
	if b.count >= limit {
		return false
	}
	b.count++
	l.buckets[key] = b
	return true
}
