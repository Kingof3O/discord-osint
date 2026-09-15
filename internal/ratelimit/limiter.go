package ratelimit

import (
	"context"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// TokenScrubber provides utilities to mask authentication tokens in logs and errors.
type TokenScrubber struct {
	tokens []string
	mu     sync.RWMutex
}

// NewTokenScrubber creates a scrubber initialized with tokens to redact.
func NewTokenScrubber(tokens ...string) *TokenScrubber {
	s := &TokenScrubber{}
	for _, tok := range tokens {
		s.AddToken(tok)
	}
	return s
}

// AddToken registers an auth token to be scrubbed.
func (s *TokenScrubber) AddToken(tok string) {
	tok = strings.TrimSpace(tok)
	if tok == "" || len(tok) < 6 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens = append(s.tokens, tok)
}

// Scrub replaces any occurrences of registered tokens with [REDACTED].
func (s *TokenScrubber) Scrub(input string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, tok := range s.tokens {
		input = strings.ReplaceAll(input, tok, "[REDACTED]")
	}
	return input
}

// RateLimiter manages request tokens with backoff and randomized jitter.
type RateLimiter struct {
	rate        float64 // tokens per second
	burst       float64
	tokens      float64
	lastChecked time.Time
	mu          sync.Mutex
}

// NewRateLimiter creates a token bucket limiter.
func NewRateLimiter(ratePerSec float64, burst float64) *RateLimiter {
	return &RateLimiter{
		rate:        ratePerSec,
		burst:       burst,
		tokens:      burst,
		lastChecked: time.Now(),
	}
}

// Wait blocks until a token is available or context is cancelled.
func (l *RateLimiter) Wait(ctx context.Context) error {
	l.mu.Lock()
	now := time.Now()
	elapsed := now.Sub(l.lastChecked).Seconds()
	l.lastChecked = now

	l.tokens += elapsed * l.rate
	if l.tokens > l.burst {
		l.tokens = l.burst
	}

	if l.tokens >= 1.0 {
		l.tokens -= 1.0
		l.mu.Unlock()
		return nil
	}

	needed := 1.0 - l.tokens
	waitDuration := time.Duration(needed/l.rate*float64(time.Second)) + Jitter(50*time.Millisecond)
	l.tokens = 0
	l.lastChecked = l.lastChecked.Add(waitDuration)
	l.mu.Unlock()

	timer := time.NewTimer(waitDuration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Jitter adds a random duration up to maxJitter to prevent synchronized retries.
func Jitter(maxJitter time.Duration) time.Duration {
	if maxJitter <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(maxJitter)))
}
