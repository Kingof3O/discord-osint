package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Limit != 30 {
		t.Fatalf("expected limit 30, got %d", cfg.Limit)
	}
	if cfg.MaxJoinsPerHour != 8 {
		t.Fatalf("expected max joins per hour 8, got %d", cfg.MaxJoinsPerHour)
	}
	if cfg.Captcha != "manual" {
		t.Fatalf("expected captcha manual, got %s", cfg.Captcha)
	}
	if cfg.CaptchaTimeoutSec != 300 {
		t.Fatalf("expected captcha timeout 300, got %d", cfg.CaptchaTimeoutSec)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected default config to be valid, got: %v", err)
	}
}

func TestLoadWithEnvExpansion(t *testing.T) {
	t.Setenv("TEST_DISCORD_TOKEN", "secret-token-xyz")
	t.Setenv("TEST_OPENCODE_KEY", "opencode-key-123")

	yamlContent := `
discord_token: ${TEST_DISCORD_TOKEN}
opencode_api_key: $TEST_OPENCODE_KEY
tag: crypto
limit: 20
join_delay_sec: [20, 40]
onboarding: assist
proxy: socks5://127.0.0.1:9050
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.DiscordToken != "secret-token-xyz" {
		t.Errorf("expected token 'secret-token-xyz', got %q", cfg.DiscordToken)
	}
	if cfg.OpenCodeAPIKey != "opencode-key-123" {
		t.Errorf("expected opencode key 'opencode-key-123', got %q", cfg.OpenCodeAPIKey)
	}
	if cfg.Tag != "crypto" {
		t.Errorf("expected tag 'crypto', got %q", cfg.Tag)
	}
	if cfg.Limit != 20 {
		t.Errorf("expected limit 20, got %d", cfg.Limit)
	}
	if cfg.JoinDelaySec[0] != 20 || cfg.JoinDelaySec[1] != 40 {
		t.Errorf("unexpected join delays: %v", cfg.JoinDelaySec)
	}
	if cfg.Proxy != "socks5://127.0.0.1:9050" {
		t.Errorf("expected proxy socks5, got %q", cfg.Proxy)
	}
}

func TestValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(c *Config)
		wantErr bool
	}{
		{
			name: "negative limit",
			modify: func(c *Config) {
				c.Limit = -1
			},
			wantErr: true,
		},
		{
			name: "invalid join delay order",
			modify: func(c *Config) {
				c.JoinDelaySec = [2]int{50, 30}
			},
			wantErr: true,
		},
		{
			name: "invalid captcha mode",
			modify: func(c *Config) {
				c.Captcha = "auto"
			},
			wantErr: true,
		},
		{
			name: "invalid onboarding mode",
			modify: func(c *Config) {
				c.Onboarding = "bypass"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modify(cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("expected wantErr=%v, got err=%v", tt.wantErr, err)
			}
		})
	}
}

func TestApplyEnvDirect(t *testing.T) {
	t.Setenv("DISCORD_TOKEN", "direct-env-token")
	t.Setenv("AI_MODEL", "musespark-1.3")
	t.Setenv("OPENCODE_BASE_URL", "https://opencode.ai/zen/v1")
	t.Setenv("OPENCODE_API_KEY", "zen-key-999")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") failed: %v", err)
	}

	if cfg.DiscordToken != "direct-env-token" {
		t.Errorf("expected direct-env-token, got %q", cfg.DiscordToken)
	}
	if cfg.AIModel != "musespark-1.3" {
		t.Errorf("expected musespark-1.3, got %q", cfg.AIModel)
	}
	if cfg.OpenCodeBaseURL != "https://opencode.ai/zen/v1" {
		t.Errorf("expected https://opencode.ai/zen/v1, got %q", cfg.OpenCodeBaseURL)
	}
	if cfg.OpenCodeAPIKey != "zen-key-999" {
		t.Errorf("expected zen-key-999, got %q", cfg.OpenCodeAPIKey)
	}
}

