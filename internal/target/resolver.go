package target

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// MockResolver allows controlled testing of all verification resolution paths.
type MockResolver struct {
	UsernameResolutions map[string]Resolution
	IDResolutions       map[string]Candidate
}

// NewMockResolver creates an initialized mock resolver.
func NewMockResolver() *MockResolver {
	return &MockResolver{
		UsernameResolutions: make(map[string]Resolution),
		IDResolutions:       make(map[string]Candidate),
	}
}

func (m *MockResolver) ResolveUsername(_ context.Context, username string) (Resolution, error) {
	norm := NormalizeUsername(username)
	if res, ok := m.UsernameResolutions[norm]; ok {
		return res, nil
	}
	return Resolution{
		Input:      username,
		Status:     Unresolved,
		Candidates: nil,
	}, nil
}

func (m *MockResolver) ResolveUserID(_ context.Context, userID string) (Candidate, error) {
	if c, ok := m.IDResolutions[userID]; ok {
		return c, nil
	}
	// If snowflake is valid, generate a candidate from it
	if IsValidSnowflake(userID) {
		createdAt, _ := SnowflakeToTime(userID)
		return Candidate{
			UserID:      userID,
			Username:    "user_" + userID,
			DisplayName: "Unknown User",
			Source:      "snowflake-fallback",
			CreatedAt:   createdAt,
		}, nil
	}
	return Candidate{}, fmt.Errorf("user ID not found: %s", userID)
}

// DiscordHTTPResolver resolves targets using the Discord REST API.
type DiscordHTTPResolver struct {
	Token      string
	HTTPClient *http.Client
	BaseURL    string
}

// NewDiscordHTTPResolver creates a resolver using Discord's REST API.
func NewDiscordHTTPResolver(token string, client *http.Client) *DiscordHTTPResolver {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &DiscordHTTPResolver{
		Token:      token,
		HTTPClient: client,
		BaseURL:    "https://discord.com/api/v10",
	}
}

type discordUserResponse struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Discriminator string `json:"discriminator"`
	GlobalName    string `json:"global_name"`
	Avatar        string `json:"avatar"`
	Bot           bool   `json:"bot"`
}

func (r *DiscordHTTPResolver) ResolveUserID(ctx context.Context, userID string) (Candidate, error) {
	if !IsValidSnowflake(userID) {
		return Candidate{}, fmt.Errorf("invalid snowflake ID: %s", userID)
	}

	url := fmt.Sprintf("%s/users/%s", r.BaseURL, userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Candidate{}, err
	}

	if r.Token != "" {
		req.Header.Set("Authorization", r.Token)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")

	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return Candidate{}, fmt.Errorf("discord user lookup request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return Candidate{}, fmt.Errorf("user ID %s not found on Discord", userID)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return Candidate{}, fmt.Errorf("discord user lookup failed with HTTP %d: %s", resp.StatusCode, string(body))
	}

	var u discordUserResponse
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return Candidate{}, fmt.Errorf("failed to decode user response: %w", err)
	}

	createdAt, _ := SnowflakeToTime(u.ID)
	avatarURL := ""
	if u.Avatar != "" {
		ext := "png"
		if strings.HasPrefix(u.Avatar, "a_") {
			ext = "gif"
		}
		avatarURL = fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.%s", u.ID, u.Avatar, ext)
	}

	dispName := u.GlobalName
	if dispName == "" {
		dispName = u.Username
	}

	return Candidate{
		UserID:      u.ID,
		Username:    u.Username,
		DisplayName: dispName,
		AvatarURL:   avatarURL,
		Source:      "users-api",
		CreatedAt:   createdAt,
	}, nil
}

func (r *DiscordHTTPResolver) ResolveUsername(ctx context.Context, username string) (Resolution, error) {
	// Discord does not expose a global public username -> snowflake search endpoint for user accounts without mutual servers.
	// We return Unresolved with the normalized input.
	return Resolution{
		Input:      username,
		Status:     Unresolved,
		Candidates: nil,
	}, nil
}
