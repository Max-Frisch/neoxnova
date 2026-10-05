package handlers

import (
	"sync"
	"time"
)

const (
	lockoutThreshold = 5
	lockoutDuration  = 15 * time.Minute
)

// lockout tracks consecutive failed logins per account and applies a temporary
// lock. It complements the per-IP rate limiter: rate limiting slows all traffic,
// while lockout specifically frustrates credential stuffing on one account.
type lockout struct {
	mu          sync.Mutex
	entries     map[string]*lockEntry
	lastCleanup time.Time
}

type lockEntry struct {
	fails int
	until time.Time
}

func newLockout() *lockout {
	return &lockout{entries: make(map[string]*lockEntry)}
}

func (l *lockout) locked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleanupLocked()
	e := l.entries[key]
	return e != nil && time.Now().Before(e.until)
}

func (l *lockout) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.entries[key]
	if e == nil {
		e = &lockEntry{}
		l.entries[key] = e
	}
	e.fails++
	if e.fails >= lockoutThreshold {
		e.until = time.Now().Add(lockoutDuration)
	}
}

func (l *lockout) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

func (l *lockout) cleanupLocked() {
	now := time.Now()
	if now.Sub(l.lastCleanup) < 10*time.Minute {
		return
	}
	for k, e := range l.entries {
		if !e.until.IsZero() && now.After(e.until) {
			delete(l.entries, k)
		}
	}
	l.lastCleanup = now
}
