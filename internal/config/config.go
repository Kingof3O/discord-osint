package config

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration.
type Config struct {
	DiscordToken      string   `yaml:"discord_token"`
	Proxy             string   `yaml:"proxy"`
	Tag               string   `yaml:"tag"`
	Limit             int      `yaml:"limit"`
	JoinDelaySec      [2]int   `yaml:"join_delay_sec"`
	MaxJoinsPerHour   int      `yaml:"max_joins_per_hour"`
	StayAfterHit      bool     `yaml:"stay_after_hit"`
	Captcha           string   `yaml:"captcha"`             // "manual" | "skip"
	CaptchaTimeoutSec int      `yaml:"captcha_timeout_sec"` // seconds, default 300
	CaptchaBridgePort int      `yaml:"captcha_bridge_port"` // default 8765
	AutoOpenBrowser   bool     `yaml:"auto_open_browser"`   // default true
	BrowserExec       string   `yaml:"browser_exec"`        // custom browser binary
	Onboarding        string   `yaml:"onboarding"`          // "manual" | "assist" | "rules-only" | "skip"
	DisboardCookies   string   `yaml:"disboard_cookies"`
	OpenCodeAPIKey    string   `yaml:"opencode_api_key"`
	OpenCodeBaseURL   string   `yaml:"opencode_base_url"`
	OpenAIAPIKey      string   `yaml:"openai_api_key"`
	AIMode            string   `yaml:"ai_mode"` // "triage" | "none"
	DatabasePath      string   `yaml:"database_path"`
	LogLevel          string   `yaml:"log_level"`
}

// DefaultConfig returns safe, production-grade defaults.
func DefaultConfig() *Config {
	return &Config{
		Limit:             30,
		JoinDelaySec:      [2]int{30, 90},
		MaxJoinsPerHour:   8,
		StayAfterHit:      false,
		Captcha:           "manual",
		CaptchaTimeoutSec: 300,
		CaptchaBridgePort: 8765,
		AutoOpenBrowser:   true,
		Onboarding:        "manual",
		OpenCodeBaseURL:   "https://api.opencode.ai/v1",
		AIMode:            "triage",
		DatabasePath:      "run.sqlite",
		LogLevel:          "info",
	}
}

var envRegex = regexp.MustCompile(`\$\{([a-zA-Z_0-9]+)\}|\$([a-zA-Z_0-9]+)`)

// expandEnv replaces ${VAR} and $VAR with os.Getenv("VAR").
func expandEnv(s string) string {
	return envRegex.ReplaceAllStringFunc(s, func(m string) string {
		sub := envRegex.FindStringSubmatch(m)
		var varName string
		if len(sub) > 1 && sub[1] != "" {
			varName = sub[1]
		} else if len(sub) > 2 && sub[2] != "" {
			varName = sub[2]
		}
		if varName != "" {
			return os.Getenv(varName)
		}
		return m
	})
}

// Load loads and parses the YAML config from path, expanding environment variables.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found: %s", path)
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	expanded := expandEnv(string(data))
	if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
		return nil, fmt.Errorf("failed to parse yaml config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

// Validate ensures configuration invariants are met.
func (c *Config) Validate() error {
	if c.Limit < 0 {
		return fmt.Errorf("limit must be non-negative, got %d", c.Limit)
	}
	if c.JoinDelaySec[0] < 0 || c.JoinDelaySec[1] < 0 {
		return fmt.Errorf("join_delay_sec values must be non-negative, got [%d, %d]", c.JoinDelaySec[0], c.JoinDelaySec[1])
	}
	if c.JoinDelaySec[0] > c.JoinDelaySec[1] {
		return fmt.Errorf("join_delay_sec min (%d) cannot exceed max (%d)", c.JoinDelaySec[0], c.JoinDelaySec[1])
	}
	if c.MaxJoinsPerHour <= 0 {
		c.MaxJoinsPerHour = 8
	}
	if c.Captcha != "manual" && c.Captcha != "skip" {
		return fmt.Errorf("captcha mode must be 'manual' or 'skip', got %q", c.Captcha)
	}
	if c.CaptchaTimeoutSec <= 0 {
		c.CaptchaTimeoutSec = 300
	}
	if c.CaptchaBridgePort <= 0 || c.CaptchaBridgePort > 65535 {
		c.CaptchaBridgePort = 8765
	}
	switch c.Onboarding {
	case "manual", "assist", "rules-only", "skip":
	default:
		return fmt.Errorf("onboarding mode must be 'manual', 'assist', 'rules-only', or 'skip', got %q", c.Onboarding)
	}
	return nil
}

// JoinDelayDuration returns a duration between min and max.
func (c *Config) JoinDelayRange() (time.Duration, time.Duration) {
	return time.Duration(c.JoinDelaySec[0]) * time.Second, time.Duration(c.JoinDelaySec[1]) * time.Second
}
