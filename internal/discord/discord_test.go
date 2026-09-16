package discord

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"discord-osint/internal/target"

	"github.com/gorilla/websocket"
)

func TestProperties(t *testing.T) {
	header := BuildSuperPropertiesHeader()
	data, err := base64.StdEncoding.DecodeString(header)
	if err != nil {
		t.Fatalf("failed to base64 decode super properties: %v", err)
	}

	var props ClientSuperProperties
	if err := json.Unmarshal(data, &props); err != nil {
		t.Fatalf("failed to unmarshal super properties JSON: %v", err)
	}

	if props.OS == "" || props.Browser == "" || props.ClientBuildNumber == 0 {
		t.Errorf("incomplete super properties: %+v", props)
	}

	ctxHeader := BuildJoinContextProperties()
	ctxData, err := base64.StdEncoding.DecodeString(ctxHeader)
	if err != nil {
		t.Fatalf("failed to decode context properties: %v", err)
	}
	if !strings.Contains(string(ctxData), "Join Guild") {
		t.Errorf("expected context properties to contain 'Join Guild', got %s", string(ctxData))
	}
}

func TestClient_JoinAndCaptchaDetection(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify standard anti-bot headers
		if r.Header.Get("Authorization") != "burner-token-123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("X-Super-Properties") == "" {
			t.Errorf("missing X-Super-Properties header")
		}

		if strings.HasSuffix(r.URL.Path, "/invites/captcha-guild") {
			if r.Header.Get("X-Captcha-Key") == "solved-token-xyz" {
				if r.Header.Get("X-Captcha-Rqtoken") != "sample-rqtoken" {
					t.Errorf("expected X-Captcha-Rqtoken header")
				}
				if r.Header.Get("X-Captcha-Session-Id") != "sample-session-id" {
					t.Errorf("expected X-Captcha-Session-Id header")
				}
				w.WriteHeader(http.StatusOK)
				resp := map[string]any{
					"code": "captcha-guild",
					"guild": map[string]any{
						"id":   "guild_solved_123",
						"name": "Solved Captcha Guild",
					},
				}
				_ = json.NewEncoder(w).Encode(resp)
				return
			}

			w.WriteHeader(http.StatusBadRequest)
			resp := map[string]any{
				"captcha_sitekey":    "a9b5fb07-92ff-493f-86fe-352a2843b3df",
				"captcha_service":    "hcaptcha",
				"captcha_session_id": "sample-session-id",
				"captcha_rqdata":     "sample-rqdata",
				"captcha_rqtoken":    "sample-rqtoken",
				"captcha_key":        []string{"You need to update your app to join this server."},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		if strings.HasSuffix(r.URL.Path, "/invites/clean-guild") {
			w.WriteHeader(http.StatusOK)
			resp := map[string]any{
				"code": "clean-guild",
				"guild": map[string]any{
					"id":   "guild_clean_123",
					"name": "Clean Guild",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	client, err := NewClient(ClientOptions{
		Token:   "burner-token-123",
		BaseURL: ts.URL,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// 1. Test clean join
	res, err := client.JoinGuild(context.Background(), "clean-guild", "gw-session-123", "", "", "")
	if err != nil {
		t.Fatalf("JoinGuild clean failed: %v", err)
	}
	if res.Guild == nil || res.Guild.ID != "guild_clean_123" {
		t.Errorf("unexpected join result: %+v", res)
	}

	// 2. Test join triggering CAPTCHA
	_, err = client.JoinGuild(context.Background(), "captcha-guild", "gw-session-123", "", "", "")
	if err == nil {
		t.Fatalf("expected CAPTCHA error, got nil")
	}

	var captchaErr *CaptchaChallengeError
	if !errors.As(err, &captchaErr) {
		t.Fatalf("expected CaptchaChallengeError, got: %T (%v)", err, err)
	}
	if captchaErr.Challenge.SiteKey != "a9b5fb07-92ff-493f-86fe-352a2843b3df" {
		t.Errorf("unexpected sitekey: %s", captchaErr.Challenge.SiteKey)
	}
	if captchaErr.Challenge.SessionID != "sample-session-id" {
		t.Errorf("unexpected sessionID: %s", captchaErr.Challenge.SessionID)
	}
	if captchaErr.Challenge.RqToken != "sample-rqtoken" {
		t.Errorf("unexpected rqtoken: %s", captchaErr.Challenge.RqToken)
	}
	if len(captchaErr.Challenge.Errors) == 0 || captchaErr.Challenge.Errors[0] != "You need to update your app to join this server." {
		t.Errorf("unexpected challenge errors: %v", captchaErr.Challenge.Errors)
	}

	// 3. Test retry with solved CAPTCHA token and session ID
	retryRes, err := client.JoinGuild(context.Background(), "captcha-guild", "gw-session-123", "solved-token-xyz", captchaErr.Challenge.RqToken, captchaErr.Challenge.SessionID)
	if err != nil {
		t.Fatalf("JoinGuild retry with CAPTCHA failed: %v", err)
	}
	if retryRes.Guild == nil || retryRes.Guild.ID != "guild_solved_123" {
		t.Errorf("unexpected join retry result: %+v", retryRes)
	}
}

func TestClient_SearchMembersAndTwoTier(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/members/search") {
			query := r.URL.Query().Get("query")
			if query == "target_user" {
				members := []GuildMemberResponse{
					{
						User: User{
							ID:         "123456789012345678",
							Username:   "target_user",
							GlobalName: "Target User",
						},
						Nick:     "TargetNick",
						JoinedAt: time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC),
						Roles:    []string{"VIP"},
					},
				}
				_ = json.NewEncoder(w).Encode(members)
				return
			}
			_ = json.NewEncoder(w).Encode([]GuildMemberResponse{})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	client, err := NewClient(ClientOptions{
		Token:   "tok",
		BaseURL: ts.URL,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	tgt := target.ConfirmedTarget{
		TargetUserID:   "123456789012345678",
		TargetUsername: "target_user",
	}

	result, err := DiscoverMembers(context.Background(), client, nil, "g123", tgt, 5000, false)
	if err != nil {
		t.Fatalf("DiscoverMembers failed: %v", err)
	}

	if result.StopReason != "targeted_search_hit" {
		t.Errorf("expected targeted_search_hit, got %s", result.StopReason)
	}
	if result.Coverage != "bounded" {
		t.Errorf("expected bounded coverage, got %s", result.Coverage)
	}
	if len(result.Members) != 1 || result.Members[0].UserID != "123456789012345678" {
		t.Errorf("unexpected members returned: %+v", result.Members)
	}
}

func TestGatewaySession_ConnectAndReady(t *testing.T) {
	upgrader := websocket.Upgrader{}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// 1. Send Op 10 HELLO
		hello := map[string]any{
			"op": 10,
			"d": map[string]any{
				"heartbeat_interval": 1000,
			},
		}
		if err := conn.WriteJSON(hello); err != nil {
			return
		}

		// 2. Expect Op 2 IDENTIFY
		var identify map[string]any
		if err := conn.ReadJSON(&identify); err != nil {
			return
		}

		// 3. Send Op 0 READY
		ready := map[string]any{
			"op": 0,
			"t":  "READY",
			"d": map[string]any{
				"session_id": "session_test_999",
				"user": map[string]any{
					"id":       "burner_user_id",
					"username": "burner",
				},
			},
		}
		_ = conn.WriteJSON(ready)

		// Hold connection open briefly
		time.Sleep(200 * time.Millisecond)
	}))
	defer wsServer.Close()

	wsURL := "ws" + strings.TrimPrefix(wsServer.URL, "http")

	gw := NewGatewaySession("burner-token", "", wsURL)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := gw.Connect(ctx); err != nil {
		t.Fatalf("Gateway Connect failed: %v", err)
	}
	defer gw.Close()

	// Wait briefly for READY event to process
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if gw.SessionID() == "session_test_999" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if gw.SessionID() != "session_test_999" {
		t.Errorf("expected session_id 'session_test_999', got %q", gw.SessionID())
	}
}

func TestTokenScrubber(t *testing.T) {
	c, _ := NewClient(ClientOptions{Token: "init_token_xyz"})
	scrubber := c.TokenScrubber()
	secret := "mfa.secret_token_12345"
	scrubber.AddToken(secret)

	logMsg := fmt.Sprintf("failed to connect with header Authorization: %s on endpoint", secret)
	scrubbed := scrubber.Scrub(logMsg)

	if strings.Contains(scrubbed, secret) {
		t.Errorf("token leaked in scrubbed output: %s", scrubbed)
	}
	if !strings.Contains(scrubbed, "[REDACTED]") {
		t.Errorf("expected [REDACTED] in output, got: %s", scrubbed)
	}
}

func TestClient_GetGuildChannels(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/guilds/g_channels_test/channels") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]Channel{
				{ID: "c1", Name: "general", GuildID: "g_channels_test", Type: 0},
				{ID: "c2", Name: "announcements", GuildID: "g_channels_test", Type: 5},
			})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/channels/c1") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(Channel{
				ID: "c1", Name: "general", GuildID: "g_channels_test", Type: 0,
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	client, err := NewClient(ClientOptions{
		Token:   "dummy_tok",
		BaseURL: ts.URL,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	channels, err := client.GetGuildChannels(context.Background(), "g_channels_test")
	if err != nil {
		t.Fatalf("GetGuildChannels failed: %v", err)
	}
	if len(channels) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(channels))
	}
	if channels[0].Name != "general" || channels[1].Name != "announcements" {
		t.Errorf("unexpected channels: %+v", channels)
	}

	ch, err := client.GetChannel(context.Background(), "c1")
	if err != nil {
		t.Fatalf("GetChannel failed: %v", err)
	}
	if ch.Name != "general" {
		t.Errorf("expected channel name 'general', got %q", ch.Name)
	}
}

