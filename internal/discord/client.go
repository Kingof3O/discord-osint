package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"discord-osint/internal/captcha"
	"discord-osint/internal/ratelimit"
)

var (
	ErrUnauthorized     = errors.New("unauthorized: invalid or expired discord token")
	ErrAccountLocked    = errors.New("account locked, quarantined, or requires phone verification")
	ErrRateLimited      = errors.New("discord rate limited (HTTP 429)")
	ErrForbidden        = errors.New("forbidden: missing permissions (HTTP 403)")
	ErrCaptchaChallenge = errors.New("captcha challenge triggered on request")
)

// CaptchaChallengeError wraps challenge details when Discord returns hCaptcha requirements.
type CaptchaChallengeError struct {
	Challenge captcha.Challenge
}

func (e *CaptchaChallengeError) Error() string {
	return fmt.Sprintf("captcha challenge required: service=%s sitekey=%s", e.Challenge.Service, e.Challenge.SiteKey)
}

// Client manages authenticated REST communication with Discord.
type Client struct {
	token           string
	baseURL         string
	httpClient      *http.Client
	limiter         *ratelimit.RateLimiter
	scrubber        *ratelimit.TokenScrubber
	superProperties string
	userAgent       string
}

// ClientOptions configures the Discord client.
type ClientOptions struct {
	Token      string
	BaseURL    string
	ProxyURL   string
	HTTPClient *http.Client
	RateLimit  float64 // req/sec, default 5.0
}

// NewClient initializes a hardened Discord REST client.
func NewClient(opts ClientOptions) (*Client, error) {
	if opts.Token == "" {
		return nil, errors.New("discord token is required")
	}

	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = "https://discord.com/api/v10"
	}

	httpClient := opts.HTTPClient
	if httpClient == nil {
		transport := &http.Transport{
			MaxIdleConns:        100,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
		}

		if opts.ProxyURL != "" {
			proxy, err := url.Parse(opts.ProxyURL)
			if err != nil {
				return nil, fmt.Errorf("invalid proxy URL: %w", err)
			}
			transport.Proxy = http.ProxyURL(proxy)
		}

		httpClient = &http.Client{
			Transport: transport,
			Timeout:   20 * time.Second,
		}
	}

	rate := opts.RateLimit
	if rate <= 0 {
		rate = 5.0 // 5 req/s
	}

	props := DefaultSuperProperties()
	superProps := BuildSuperPropertiesHeader()

	scrubber := ratelimit.NewTokenScrubber(opts.Token)

	return &Client{
		token:           opts.Token,
		baseURL:         baseURL,
		httpClient:      httpClient,
		limiter:         ratelimit.NewRateLimiter(rate, 8.0),
		scrubber:        scrubber,
		superProperties: superProps,
		userAgent:       props.BrowserUserAgent,
	}, nil
}

// TokenScrubber returns the scrubber for log sanitization.
func (c *Client) TokenScrubber() *ratelimit.TokenScrubber {
	return c.scrubber
}

// do executes an authenticated request with rate limiting, headers, and anti-bot properties.
func (c *Client) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}

	// Set anti-bot browser headers
	req.Header.Set("Authorization", c.token)
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("X-Super-Properties", c.superProperties)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Ch-Ua", `"Chromium";v="128", "Not;A=Brand";v="24", "Discord";v="1"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"macOS"`)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network request failed: %s", c.scrubber.Scrub(err.Error()))
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfterStr := resp.Header.Get("Retry-After")
		var waitDur time.Duration = 2 * time.Second
		if s, err := strconv.ParseFloat(retryAfterStr, 64); err == nil && s > 0 {
			waitDur = time.Duration(s * float64(time.Second))
		}
		resp.Body.Close()
		time.Sleep(waitDur)
		return nil, ErrRateLimited
	}

	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		return nil, ErrUnauthorized
	}

	return resp, nil
}

// GetMe fetches the authenticated burner user account details to check health.
func (c *Client) GetMe(ctx context.Context) (*User, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/users/@me", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get current user failed (%d): %s", resp.StatusCode, c.scrubber.Scrub(string(body)))
	}

	var u User
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, fmt.Errorf("failed to decode user: %w", err)
	}

	return &u, nil
}

// ResolveInvite fetches invite metadata without joining the guild (read-only preflight).
func (c *Client) ResolveInvite(ctx context.Context, inviteCode string) (*InviteMetadata, error) {
	endpoint := fmt.Sprintf("%s/invites/%s?with_counts=true&with_expiration=true", c.baseURL, inviteCode)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("invite code %q is invalid or expired", inviteCode)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("resolve invite failed (%d): %s", resp.StatusCode, c.scrubber.Scrub(string(body)))
	}

	var meta InviteMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, fmt.Errorf("failed to decode invite metadata: %w", err)
	}

	return &meta, nil
}

// JoinGuild attempts to join a Discord server via its invite code.
func (c *Client) JoinGuild(ctx context.Context, inviteCode string, captchaToken, captchaRqToken string) (*JoinResponse, error) {
	endpoint := fmt.Sprintf("%s/invites/%s", c.baseURL, inviteCode)

	payload := map[string]any{
		"session_id": nil,
	}
	if captchaToken != "" {
		payload["captcha_key"] = captchaToken
	}
	if captchaRqToken != "" {
		payload["captcha_rqtoken"] = captchaRqToken
	}
	bodyData, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Context-Properties", BuildJoinContextProperties())

	if captchaToken != "" {
		req.Header.Set("X-Captcha-Key", captchaToken)
	}
	if captchaRqToken != "" {
		req.Header.Set("X-Captcha-Rqtoken", captchaRqToken)
	}

	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	// Check if Discord requires a CAPTCHA challenge
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusForbidden {
		var joinResp JoinResponse
		_ = json.Unmarshal(bodyBytes, &joinResp)

		if joinResp.CaptchaSiteKey != "" || len(joinResp.CaptchaKey) > 0 {
			siteKey := joinResp.CaptchaSiteKey
			if siteKey == "" && len(joinResp.CaptchaKey) > 0 {
				siteKey = joinResp.CaptchaKey[0]
			}
			service := joinResp.CaptchaService
			if service == "" {
				service = "hcaptcha"
			}
			return nil, &CaptchaChallengeError{
				Challenge: captcha.Challenge{
					Service: service,
					SiteKey: siteKey,
					RqData:  joinResp.CaptchaRqData,
					RqToken: joinResp.CaptchaRqToken,
				},
			}
		}

		if strings.Contains(string(bodyBytes), "phone") || strings.Contains(string(bodyBytes), "verify your account") {
			return nil, ErrAccountLocked
		}

		return nil, fmt.Errorf("join guild failed (%d): %s", resp.StatusCode, c.scrubber.Scrub(string(bodyBytes)))
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("join guild unexpected status (%d): %s", resp.StatusCode, c.scrubber.Scrub(string(bodyBytes)))
	}

	var joinResp JoinResponse
	if err := json.Unmarshal(bodyBytes, &joinResp); err != nil {
		return nil, fmt.Errorf("failed to decode join response: %w", err)
	}

	return &joinResp, nil
}

// LeaveGuild removes the current burner account from a guild.
func (c *Client) LeaveGuild(ctx context.Context, guildID string) error {
	endpoint := fmt.Sprintf("%s/users/@me/guilds/%s", c.baseURL, guildID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := c.do(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("leave guild failed (%d): %s", resp.StatusCode, c.scrubber.Scrub(string(body)))
	}

	return nil
}

// SubmitRulesScreening automatically fetches and accepts server rules verification to clear is_pending status.
func (c *Client) SubmitRulesScreening(ctx context.Context, guildID string) error {
	endpoint := fmt.Sprintf("%s/guilds/%s/member-verification", c.baseURL, guildID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := c.do(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil // Guild does not require membership screening
	}

	var form struct {
		Version    string `json:"version"`
		FormFields []struct {
			FieldType string `json:"field_type"`
		} `json:"form_fields"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&form); err != nil {
		return err
	}

	if len(form.FormFields) == 0 {
		return nil
	}

	var fields []map[string]any
	for _, f := range form.FormFields {
		fields = append(fields, map[string]any{
			"field_type": f.FieldType,
			"response":   true,
		})
	}

	payload := map[string]any{
		"version":     form.Version,
		"form_fields": fields,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	putEndpoint := fmt.Sprintf("%s/guilds/%s/requests/@me", c.baseURL, guildID)
	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, putEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	putReq.Header.Set("Content-Type", "application/json")

	putResp, err := c.do(ctx, putReq)
	if err != nil {
		return err
	}
	defer putResp.Body.Close()

	return nil
}

// SearchGuildMembers searches members in a guild by username or display name prefix.
func (c *Client) SearchGuildMembers(ctx context.Context, guildID string, query string, limit int) ([]GuildMemberResponse, error) {
	if limit <= 0 {
		limit = 50
	}
	endpoint := fmt.Sprintf("%s/guilds/%s/members/search?query=%s&limit=%d", c.baseURL, guildID, url.QueryEscape(query), limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return nil, ErrForbidden
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("member search failed (%d): %s", resp.StatusCode, c.scrubber.Scrub(string(body)))
	}

	var members []GuildMemberResponse
	if err := json.NewDecoder(resp.Body).Decode(&members); err != nil {
		return nil, fmt.Errorf("failed to decode member search: %w", err)
	}

	return members, nil
}

type searchMessagesResponse struct {
	TotalResults int                `json:"total_results"`
	Messages     [][]DiscordMessage `json:"messages"` // Discord returns array of message blocks
}

// SearchGuildMessages searches for messages authored by a specific user or matching a text query in a guild.
func (c *Client) SearchGuildMessages(ctx context.Context, guildID string, authorID string, query string) ([]DiscordMessage, error) {
	v := url.Values{}
	if authorID != "" {
		v.Set("author_id", authorID)
	}
	if query != "" {
		v.Set("content", query)
	}

	endpoint := fmt.Sprintf("%s/guilds/%s/messages/search?%s", c.baseURL, guildID, v.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return nil, ErrForbidden
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("messages search failed (%d): %s", resp.StatusCode, c.scrubber.Scrub(string(body)))
	}

	var res searchMessagesResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to decode search messages: %w", err)
	}

	var flatMessages []DiscordMessage
	for _, group := range res.Messages {
		flatMessages = append(flatMessages, group...)
	}

	return flatMessages, nil
}

// GetChannelMessages retrieves recent messages from a channel.
func (c *Client) GetChannelMessages(ctx context.Context, channelID string, limit int) ([]DiscordMessage, error) {
	if limit <= 0 {
		limit = 20
	}
	endpoint := fmt.Sprintf("%s/channels/%s/messages?limit=%d", c.baseURL, channelID, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return nil, ErrForbidden
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get channel messages failed (%d): %s", resp.StatusCode, c.scrubber.Scrub(string(body)))
	}

	var msgs []DiscordMessage
	if err := json.NewDecoder(resp.Body).Decode(&msgs); err != nil {
		return nil, fmt.Errorf("failed to decode channel messages: %w", err)
	}

	return msgs, nil
}

// PostReaction adds a reaction emoji to a message.
func (c *Client) PostReaction(ctx context.Context, channelID, messageID, emoji string) error {
	encodedEmoji := url.PathEscape(emoji)
	endpoint := fmt.Sprintf("%s/channels/%s/messages/%s/reactions/%s/@me", c.baseURL, channelID, messageID, encodedEmoji)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := c.do(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("post reaction failed (%d): %s", resp.StatusCode, c.scrubber.Scrub(string(body)))
	}

	return nil
}

// PostInteraction executes a component button interaction on Discord.
func (c *Client) PostInteraction(ctx context.Context, payload map[string]any) error {
	endpoint := fmt.Sprintf("%s/interactions", c.baseURL)
	bodyData, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.do(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("post interaction failed (%d): %s", resp.StatusCode, c.scrubber.Scrub(string(body)))
	}

	return nil
}
