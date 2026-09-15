package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"discord-osint/internal/store"
)

func TestAIClient_CompleteAndCaching(t *testing.T) {
	var callCount int64

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&callCount, 1)

		if r.Header.Get("Authorization") != "Bearer test-key-123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		resp := chatCompletionResponse{
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{
				{
					Message: chatMessage{
						Role:    "assistant",
						Content: "Triage summary: Target is active in crypto communities discussing trading bots.",
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewClient(ClientOptions{
		APIKey:     "test-key-123",
		BaseURL:    ts.URL,
		HTTPClient: ts.Client(),
	})

	ctx := context.Background()

	// 1. First call - hits server
	ans1, err := client.Complete(ctx, "system prompt", "user query")
	if err != nil {
		t.Fatalf("first complete failed: %v", err)
	}
	if ans1 != "Triage summary: Target is active in crypto communities discussing trading bots." {
		t.Errorf("unexpected answer: %s", ans1)
	}
	if atomic.LoadInt64(&callCount) != 1 {
		t.Errorf("expected 1 call, got %d", atomic.LoadInt64(&callCount))
	}

	// 2. Second identical call - must hit cache
	ans2, err := client.Complete(ctx, "system prompt", "user query")
	if err != nil {
		t.Fatalf("second complete failed: %v", err)
	}
	if ans2 != ans1 {
		t.Errorf("expected identical cached answer, got: %s", ans2)
	}
	if atomic.LoadInt64(&callCount) != 1 {
		t.Errorf("cache failed! Expected callCount 1, got %d", atomic.LoadInt64(&callCount))
	}
}

func TestAIClient_TriageEvidence(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatCompletionResponse{
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{
				{Message: chatMessage{Content: "Summary of evidence."}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewClient(ClientOptions{
		APIKey:     "key",
		BaseURL:    ts.URL,
		HTTPClient: ts.Client(),
	})

	msgs := []store.MessageRecord{
		{
			Timestamp:      time.Now(),
			AuthorUsername: "john.doe",
			Content:        "Checking out this new trading server.",
		},
	}

	summary, err := client.TriageEvidence(context.Background(), "john.doe", msgs)
	if err != nil {
		t.Fatalf("TriageEvidence failed: %v", err)
	}
	if summary != "Summary of evidence." {
		t.Errorf("unexpected summary: %s", summary)
	}
}
