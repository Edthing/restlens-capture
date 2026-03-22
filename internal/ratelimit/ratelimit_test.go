package ratelimit

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestLimiter_RateLimit(t *testing.T) {
	l := New(Config{MaxPerSecond: 10, MaxPaths: 0, MaxTotal: 0})
	defer l.Close()

	allowed := 0
	for i := 0; i < 20; i++ {
		if l.Allow("/test") {
			allowed++
		}
	}

	if allowed != 10 {
		t.Errorf("expected 10 allowed, got %d", allowed)
	}

	if l.Dropped() != 10 {
		t.Errorf("expected 10 dropped, got %d", l.Dropped())
	}
}

func TestLimiter_RateLimitResets(t *testing.T) {
	l := New(Config{MaxPerSecond: 5, MaxPaths: 0, MaxTotal: 0})
	defer l.Close()

	// Use up the limit
	for i := 0; i < 5; i++ {
		l.Allow("/test")
	}

	if l.Allow("/test") {
		t.Error("expected rate limit to be hit")
	}

	// Wait for reset
	time.Sleep(1100 * time.Millisecond)

	if !l.Allow("/test") {
		t.Error("expected rate limit to have reset")
	}
}

func TestLimiter_PathCardinality(t *testing.T) {
	l := New(Config{MaxPerSecond: 0, MaxPaths: 3, MaxTotal: 0})
	defer l.Close()

	if !l.Allow("/a") {
		t.Error("expected /a to be allowed")
	}
	if !l.Allow("/b") {
		t.Error("expected /b to be allowed")
	}
	if !l.Allow("/c") {
		t.Error("expected /c to be allowed")
	}

	// Known paths should still work
	if !l.Allow("/a") {
		t.Error("expected known path /a to be allowed")
	}

	// New path should be rejected
	if l.Allow("/d") {
		t.Error("expected /d to be rejected (cardinality limit)")
	}

	if l.Dropped() != 1 {
		t.Errorf("expected 1 dropped, got %d", l.Dropped())
	}
}

func TestLimiter_TotalLimit(t *testing.T) {
	l := New(Config{MaxPerSecond: 0, MaxPaths: 0, MaxTotal: 5})
	defer l.Close()

	for i := 0; i < 5; i++ {
		if !l.Allow("/test") {
			t.Errorf("request %d should be allowed", i)
		}
	}

	if l.Allow("/test") {
		t.Error("expected total limit to be hit")
	}

	if l.Dropped() != 1 {
		t.Errorf("expected 1 dropped, got %d", l.Dropped())
	}
}

func TestLimiter_Unlimited(t *testing.T) {
	l := New(Config{MaxPerSecond: 0, MaxPaths: 0, MaxTotal: 0})
	defer l.Close()

	for i := 0; i < 10000; i++ {
		if !l.Allow(fmt.Sprintf("/path/%d", i)) {
			t.Fatalf("request %d should be allowed with no limits", i)
		}
	}
}

func TestLimiter_ConcurrentAccess(t *testing.T) {
	l := New(Config{MaxPerSecond: 0, MaxPaths: 100, MaxTotal: 0})
	defer l.Close()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				l.Allow(fmt.Sprintf("/path/%d", n*10+j))
			}
		}(i)
	}
	wg.Wait()

	// Should have captured up to 100 unique paths
	l.pathsMu.Lock()
	pathCount := len(l.paths)
	l.pathsMu.Unlock()

	if pathCount > 100 {
		t.Errorf("expected max 100 paths, got %d", pathCount)
	}
}

func TestLimiter_CombinedLimits(t *testing.T) {
	l := New(Config{MaxPerSecond: 100, MaxPaths: 5, MaxTotal: 50})
	defer l.Close()

	allowed := 0
	for i := 0; i < 100; i++ {
		path := fmt.Sprintf("/path/%d", i%10)
		if l.Allow(path) {
			allowed++
		}
	}

	// Should be limited by either total (50) or path cardinality (5 unique)
	if allowed > 50 {
		t.Errorf("expected at most 50 allowed, got %d", allowed)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxPerSecond != 1000 {
		t.Errorf("expected 1000/s, got %d", cfg.MaxPerSecond)
	}
	if cfg.MaxPaths != 10000 {
		t.Errorf("expected 10000 paths, got %d", cfg.MaxPaths)
	}
	if cfg.MaxTotal != 1_000_000 {
		t.Errorf("expected 1M total, got %d", cfg.MaxTotal)
	}
}
