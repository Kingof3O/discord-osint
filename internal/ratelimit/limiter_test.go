package ratelimit

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestTokenScrubber(t *testing.T) {
	s := NewTokenScrubber("sensitive_token_123")
	s.AddToken("another_secret_456")

	input := "Request Authorization: sensitive_token_123 and another_secret_456 passed"
	scrubbed := s.Scrub(input)

	if strings.Contains(scrubbed, "sensitive_token_123") || strings.Contains(scrubbed, "another_secret_456") {
		t.Errorf("token leaked in scrubbed string: %s", scrubbed)
	}

	count := strings.Count(scrubbed, "[REDACTED]")
	if count != 2 {
		t.Errorf("expected 2 redactions, got %d in: %s", count, scrubbed)
	}
}

func TestRateLimiter(t *testing.T) {
	limiter := NewRateLimiter(50, 10)
	ctx := context.Background()

	// Initial burst should not block
	start := time.Now()
	for i := 0; i < 5; i++ {
		if err := limiter.Wait(ctx); err != nil {
			t.Fatalf("unexpected wait error: %v", err)
		}
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Errorf("burst took too long: %v", time.Since(start))
	}
}
