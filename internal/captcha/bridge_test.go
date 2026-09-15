package captcha

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestBridge_HTTPPostSubmit(t *testing.T) {
	cfg := Config{
		Port:        0, // Dynamic port
		Timeout:     5 * time.Second,
		AutoOpen:    false,
		DisableBell: true,
	}

	bridge := NewInteractiveBridge(cfg)
	var output bytes.Buffer
	var input bytes.Buffer
	bridge.SetIO(&input, &output)

	ch := Challenge{
		Service:   "hcaptcha",
		SiteKey:   "test-sitekey-123",
		RqData:    "test-rqdata-456",
		GuildID:   "guild_999",
		GuildName: "Test Guild",
		RqToken:   "rq-token-789",
	}

	// Trigger solve in a goroutine
	type solveResult struct {
		sol Solution
		err error
	}
	resultChan := make(chan solveResult, 1)
	go func() {
		sol, err := bridge.Solve(context.Background(), ch)
		resultChan <- solveResult{sol, err}
	}()

	// Wait briefly for server to bind and print port
	var portStr string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		out := output.String()
		if idx := strings.Index(out, "http://127.0.0.1:"); idx != -1 {
			start := idx + len("http://127.0.0.1:")
			end := strings.Index(out[start:], "/captcha")
			if end != -1 {
				portStr = out[start : start+end]
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}

	if portStr == "" {
		t.Fatalf("failed to locate bridge port in output: %s", output.String())
	}

	// 1. Verify GET /captcha HTML response
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/captcha", portStr))
	if err != nil {
		t.Fatalf("failed to GET /captcha: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "test-sitekey-123") {
		t.Errorf("expected HTML to contain sitekey, got: %s", string(body))
	}

	// 2. Submit token via POST /submit
	postPayload := `{"token":"simulated-hcaptcha-response-token-12345"}`
	submitResp, err := http.Post(
		fmt.Sprintf("http://127.0.0.1:%s/submit", portStr),
		"application/json",
		strings.NewReader(postPayload),
	)
	if err != nil {
		t.Fatalf("failed to POST /submit: %v", err)
	}
	submitResp.Body.Close()
	if submitResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from /submit, got %d", submitResp.StatusCode)
	}

	// 3. Verify solve returns the solution
	select {
	case res := <-resultChan:
		if res.err != nil {
			t.Fatalf("expected clean solve, got: %v", res.err)
		}
		if res.sol.Token != "simulated-hcaptcha-response-token-12345" {
			t.Errorf("expected token mismatch, got %s", res.sol.Token)
		}
		if res.sol.RqToken != "rq-token-789" {
			t.Errorf("expected rqtoken preserved, got %s", res.sol.RqToken)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for solve result")
	}
}

func TestBridge_CLIInput_PasteToken(t *testing.T) {
	cfg := Config{
		Port:        0,
		Timeout:     5 * time.Second,
		AutoOpen:    false,
		DisableBell: true,
	}

	bridge := NewInteractiveBridge(cfg)
	var output bytes.Buffer
	input := bytes.NewBufferString("pasted-secret-captcha-token-from-user-terminal\n")
	bridge.SetIO(input, &output)

	ch := Challenge{
		Service: "hcaptcha",
		SiteKey: "sitekey-abc",
	}

	sol, err := bridge.Solve(context.Background(), ch)
	if err != nil {
		t.Fatalf("expected token solve, got err: %v", err)
	}
	if sol.Token != "pasted-secret-captcha-token-from-user-terminal" {
		t.Errorf("expected pasted token, got %q", sol.Token)
	}
}

func TestBridge_CLIInput_Skip(t *testing.T) {
	cfg := Config{
		Port:        0,
		Timeout:     5 * time.Second,
		AutoOpen:    false,
		DisableBell: true,
	}

	bridge := NewInteractiveBridge(cfg)
	var output bytes.Buffer
	input := bytes.NewBufferString("s\n")
	bridge.SetIO(input, &output)

	ch := Challenge{Service: "hcaptcha", SiteKey: "sitekey-xyz"}
	_, err := bridge.Solve(context.Background(), ch)
	if !errors.Is(err, ErrCaptchaSkipped) {
		t.Fatalf("expected ErrCaptchaSkipped, got %v", err)
	}
}

func TestBridge_CLIInput_Abort(t *testing.T) {
	cfg := Config{
		Port:        0,
		Timeout:     5 * time.Second,
		AutoOpen:    false,
		DisableBell: true,
	}

	bridge := NewInteractiveBridge(cfg)
	var output bytes.Buffer
	input := bytes.NewBufferString("q\n")
	bridge.SetIO(input, &output)

	ch := Challenge{Service: "hcaptcha", SiteKey: "sitekey-xyz"}
	_, err := bridge.Solve(context.Background(), ch)
	if !errors.Is(err, ErrCaptchaAborted) {
		t.Fatalf("expected ErrCaptchaAborted, got %v", err)
	}
}

func TestBridge_Timeout(t *testing.T) {
	cfg := Config{
		Port:        0,
		Timeout:     100 * time.Millisecond, // Very fast timeout
		AutoOpen:    false,
		DisableBell: true,
	}

	bridge := NewInteractiveBridge(cfg)
	var output bytes.Buffer
	var emptyInput bytes.Buffer
	bridge.SetIO(&emptyInput, &output)

	ch := Challenge{Service: "hcaptcha", SiteKey: "sitekey-timeout"}
	_, err := bridge.Solve(context.Background(), ch)
	if !errors.Is(err, ErrCaptchaTimeout) {
		t.Fatalf("expected ErrCaptchaTimeout, got %v", err)
	}
}
