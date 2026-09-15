package captcha

import (
	"context"
	"errors"
	"time"
)

var (
	ErrCaptchaTimeout = errors.New("captcha solve timeout expired")
	ErrCaptchaSkipped = errors.New("captcha solve skipped by operator")
	ErrCaptchaAborted = errors.New("captcha solve aborted by operator")
)

// Challenge represents a CAPTCHA challenge presented by Discord or a gateway service.
type Challenge struct {
	Service   string `json:"service"` // "hcaptcha" | "recaptcha" | "turnstile"
	SiteKey   string `json:"site_key"`
	RqData    string `json:"rq_data,omitempty"`
	RqToken   string `json:"rq_token,omitempty"`
	GuildID   string `json:"guild_id,omitempty"`
	GuildName string `json:"guild_name,omitempty"`
}

// Solution contains the solved verification payload to attach to the retried request.
type Solution struct {
	Token   string `json:"token"`
	RqToken string `json:"rq_token,omitempty"`
}

// Solver defines the interface for acquiring CAPTCHA solutions.
type Solver interface {
	Solve(ctx context.Context, challenge Challenge) (Solution, error)
}

// Config holds settings for the interactive CAPTCHA solver bridge.
type Config struct {
	Port       int
	Timeout    time.Duration
	AutoOpen   bool
	DisableBell bool
}

// DefaultConfig returns production defaults for the solver bridge.
func DefaultConfig() Config {
	return Config{
		Port:       8765,
		Timeout:    300 * time.Second,
		AutoOpen:   true,
		DisableBell: false,
	}
}
