package ai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"

	"discord-osint/internal/store"
)

// Client handles interaction with OpenAI or OpenCode compatible chat completion endpoints.
type Client struct {
	apiKey         string
	baseURL        string
	model          string
	httpClient     *http.Client
	maxRetries     int
	initialBackoff time.Duration
	maxBackoff     time.Duration
	cache          map[string]string
	cacheMu        sync.RWMutex
}

// ClientOptions configures the AI client.
type ClientOptions struct {
	APIKey         string
	BaseURL        string
	Model          string
	HTTPClient     *http.Client
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

// NewClient initializes an AI client with prompt caching and exponential backoff retry.
func NewClient(opts ClientOptions) *Client {
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = "https://opencode.ai/zen/v1"
	}
	model := opts.Model
	if model == "" {
		model = "musespark-1.3"
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	maxRetries := opts.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 4
	}
	initialBackoff := opts.InitialBackoff
	if initialBackoff <= 0 {
		initialBackoff = 1 * time.Second
	}
	maxBackoff := opts.MaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = 30 * time.Second
	}

	return &Client{
		apiKey:         opts.APIKey,
		baseURL:        baseURL,
		model:          model,
		httpClient:     httpClient,
		maxRetries:     maxRetries,
		initialBackoff: initialBackoff,
		maxBackoff:     maxBackoff,
		cache:          make(map[string]string),
	}
}

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// Complete sends a prompt to the LLM with caching and exponential backoff on 429/5xx errors.
func (c *Client) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("AI api key not configured")
	}

	hashKey := c.hash(systemPrompt + "||" + userPrompt)
	c.cacheMu.RLock()
	if cached, ok := c.cache[hashKey]; ok {
		c.cacheMu.RUnlock()
		return cached, nil
	}
	c.cacheMu.RUnlock()

	reqPayload := chatCompletionRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.2,
	}

	bodyData, err := json.Marshal(reqPayload)
	if err != nil {
		return "", err
	}

	url := c.baseURL + "/chat/completions"

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyData))
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("ai request failed: %w", err)
			if attempt < c.maxRetries {
				if sleepErr := c.sleepBackoff(ctx, attempt, 0); sleepErr != nil {
					return "", sleepErr
				}
				continue
			}
			return "", lastErr
		}

		// Check status code
		if resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			var res chatCompletionResponse
			if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
				return "", fmt.Errorf("failed to decode ai response: %w", err)
			}
			if len(res.Choices) == 0 {
				return "", fmt.Errorf("ai returned 0 completion choices")
			}
			ans := res.Choices[0].Message.Content
			c.cacheMu.Lock()
			c.cache[hashKey] = ans
			c.cacheMu.Unlock()
			return ans, nil
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		lastErr = fmt.Errorf("ai api returned HTTP %d: %s", resp.StatusCode, string(body))

		// Check if error is retryable (429 or 5xx)
		if (resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode >= 500 && resp.StatusCode <= 599)) && attempt < c.maxRetries {
			var retryAfterSec int
			if retryHeader := resp.Header.Get("Retry-After"); retryHeader != "" {
				if sec, parseErr := strconv.Atoi(retryHeader); parseErr == nil && sec > 0 {
					retryAfterSec = sec
				}
			}
			if sleepErr := c.sleepBackoff(ctx, attempt, retryAfterSec); sleepErr != nil {
				return "", sleepErr
			}
			continue
		}

		// Non-retryable error (e.g. 400 Bad Request, 401 Unauthorized, 403 Forbidden)
		return "", lastErr
	}

	return "", fmt.Errorf("ai request failed after %d retries: %w", c.maxRetries, lastErr)
}

func (c *Client) sleepBackoff(ctx context.Context, attempt int, retryAfterSec int) error {
	var delay time.Duration
	if retryAfterSec > 0 {
		delay = time.Duration(retryAfterSec) * time.Second
	} else {
		multiplier := math.Pow(2, float64(attempt))
		backoff := float64(c.initialBackoff) * multiplier
		jitter := rand.Float64() * 0.25 * backoff
		delay = time.Duration(backoff + jitter)
	}

	if delay > c.maxBackoff {
		delay = c.maxBackoff
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

func (c *Client) hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// TriageEvidence synthesizes collected messages into an executive threat/investigative summary.
func (c *Client) TriageEvidence(ctx context.Context, targetUsername string, messages []store.MessageRecord) (string, error) {
	if len(messages) == 0 {
		return "No evidence messages collected to triage.", nil
	}

	var sb bytes.Buffer
	for i, m := range messages {
		if i >= 30 {
			break
		}
		sb.WriteString(fmt.Sprintf("[%s] %s: %s\n", m.Timestamp.Format(time.RFC3339), m.AuthorUsername, m.Content))
	}

	systemPrompt := "You are an OSINT intelligence analyst assistant. Summarize the target's observed activities and topics across Discord communities based solely on the provided evidence. Highlight high-relevance themes, tone, and sentiment."
	userPrompt := fmt.Sprintf("Target: %s\n\nObserved Evidence:\n%s", targetUsername, sb.String())

	return c.Complete(ctx, systemPrompt, userPrompt)
}
