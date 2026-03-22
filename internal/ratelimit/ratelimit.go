package ratelimit

import (
	"sync"
	"sync/atomic"
	"time"
)

// Limiter provides basic rate limiting and cardinality tracking for abuse protection.
type Limiter struct {
	// Rate limiting: max captures per second
	maxPerSecond int64
	count        atomic.Int64
	resetTicker  *time.Ticker
	done         chan struct{}

	// Path cardinality: max unique paths to track
	maxPaths int
	paths    map[string]struct{}
	pathsMu  sync.Mutex

	// Total captured: max exchanges before stopping
	maxTotal int64
	total    atomic.Int64

	// Stats
	dropped atomic.Int64
}

type Config struct {
	MaxPerSecond int64 // Max captures per second (0 = unlimited)
	MaxPaths     int   // Max unique paths before rejecting new ones (0 = unlimited)
	MaxTotal     int64 // Max total exchanges before stopping capture (0 = unlimited)
}

func DefaultConfig() Config {
	return Config{
		MaxPerSecond: 1000,
		MaxPaths:     10000,
		MaxTotal:     1_000_000,
	}
}

func New(cfg Config) *Limiter {
	l := &Limiter{
		maxPerSecond: cfg.MaxPerSecond,
		maxPaths:     cfg.MaxPaths,
		maxTotal:     cfg.MaxTotal,
		paths:        make(map[string]struct{}),
		done:         make(chan struct{}),
	}

	if cfg.MaxPerSecond > 0 {
		l.resetTicker = time.NewTicker(time.Second)
		go func() {
			for {
				select {
				case <-l.resetTicker.C:
					l.count.Store(0)
				case <-l.done:
					return
				}
			}
		}()
	}

	return l
}

// Allow checks if this request should be captured.
// Returns false if rate limited, path cardinality exceeded, or total limit hit.
func (l *Limiter) Allow(path string) bool {
	// Check total limit
	if l.maxTotal > 0 && l.total.Load() >= l.maxTotal {
		l.dropped.Add(1)
		return false
	}

	// Check rate limit
	if l.maxPerSecond > 0 && l.count.Add(1) > l.maxPerSecond {
		l.dropped.Add(1)
		return false
	}

	// Check path cardinality
	if l.maxPaths > 0 {
		l.pathsMu.Lock()
		_, known := l.paths[path]
		if !known && len(l.paths) >= l.maxPaths {
			l.pathsMu.Unlock()
			l.dropped.Add(1)
			return false
		}
		if !known {
			l.paths[path] = struct{}{}
		}
		l.pathsMu.Unlock()
	}

	l.total.Add(1)
	return true
}

// Dropped returns the number of dropped requests.
func (l *Limiter) Dropped() int64 {
	return l.dropped.Load()
}

// Close stops the limiter's background goroutine.
func (l *Limiter) Close() {
	if l.resetTicker != nil {
		l.resetTicker.Stop()
	}
	close(l.done)
}
