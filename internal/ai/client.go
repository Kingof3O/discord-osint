package ai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"discord-osint/internal/store"
)

// Client handles interaction with OpenAI or OpenCode compatible chat completion endpoints.
type Client struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
	cache      map[string]string
	cacheMu    sync.RWMutex
}

// ClientOptions configures the AI client.
type ClientOptions struct {
	APIKey     string
	BaseURL    string
	Model      string
	HTTPClient *http.Client
}

// NewClient initializes an AI client with prompt caching.
func NewClient(opts ClientOptions) *Client {
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = "https://api.opencode.ai/v1"
	}
	model := opts.Model
	if model == "" {
		model = "gpt-4o-mini"
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &Client{
		apiKey:     opts.APIKey,
		baseURL:    baseURL,
		model:      model,
		httpClient: httpClient,
		cache:      make(map[string]string),
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

// Complete sends a prompt to the LLM with caching.
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ai api returned HTTP %d: %s", resp.StatusCode, string(body))
	}

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
