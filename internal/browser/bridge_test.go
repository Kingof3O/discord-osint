package browser

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

type safeBuffer struct {
	mu  sync.RWMutex
	buf bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.buf.String()
}

func TestExtractVerificationURL(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "AltDentifier in message",
			content:  "Please verify your account here: https://altdentifier.com/verify?code=xyz123 to get access.",
			expected: "https://altdentifier.com/verify?code=xyz123",
		},
		{
			name:     "WickBot URL with punctuation",
			content:  "Click (https://wickbot.com/verify/guild_99).",
			expected: "https://wickbot.com/verify/guild_99",
		},
		{
			name:     "DoubleCounter prioritized over google",
			content:  "Check https://google.com or verify at https://verify.doublecounter.net/auth?id=123",
			expected: "https://verify.doublecounter.net/auth?id=123",
		},
		{
			name:     "No URL present",
			content:  "Hello and welcome to the server! Type !verify to start.",
			expected: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractVerificationURL(tc.content)
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestBridge_HandleVerification_Confirm(t *testing.T) {
	var openedURL string
	var mu sync.Mutex

	out := &safeBuffer{}
	in := strings.NewReader("y\n")

	b := NewBridge(Options{
		AutoOpen: true,
		Reader:   in,
		Writer:   out,
	})

	b.SetOpenBrowserFn(func(targetURL string) error {
		mu.Lock()
		defer mu.Unlock()
		openedURL = targetURL
		return nil
	})

	ctx := context.Background()
	ok, err := b.HandleVerification(ctx, "https://altdentifier.com/verify", "AltDentifier")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Errorf("expected operator confirmation to return true")
	}

	mu.Lock()
	defer mu.Unlock()
	if openedURL != "https://altdentifier.com/verify" {
		t.Errorf("expected browser to open URL, got %q", openedURL)
	}
	if !strings.Contains(out.String(), "Verification confirmed") {
		t.Errorf("output missing confirmation text: %s", out.String())
	}
}

func TestBridge_HandleVerification_Deny(t *testing.T) {
	out := &safeBuffer{}
	in := strings.NewReader("n\n")

	b := NewBridge(Options{
		AutoOpen: false,
		Reader:   in,
		Writer:   out,
	})

	ctx := context.Background()
	ok, err := b.HandleVerification(ctx, "https://wickbot.com/verify", "WickBot")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Errorf("expected operator denial to return false")
	}
	if !strings.Contains(out.String(), "Verification skipped") {
		t.Errorf("output missing skipped text: %s", out.String())
	}
}

func TestBridge_HandleVerification_ContextCancel(t *testing.T) {
	out := &safeBuffer{}
	// Empty reader will block or wait
	r, w := ioPipe()
	defer w.Close()

	b := NewBridge(Options{
		AutoOpen: false,
		Reader:   r,
		Writer:   out,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	ok, err := b.HandleVerification(ctx, "https://restorecord.com", "RestoreCord")
	if err == nil {
		t.Fatalf("expected context timeout error, got nil")
	}
	if ok {
		t.Errorf("expected ok to be false")
	}
}

func ioPipe() (*pipeReader, *pipeWriter) {
	ch := make(chan []byte)
	return &pipeReader{ch: ch}, &pipeWriter{ch: ch}
}

type pipeReader struct {
	ch   chan []byte
	curr []byte
}

func (r *pipeReader) Read(p []byte) (n int, err error) {
	if len(r.curr) == 0 {
		data, ok := <-r.ch
		if !ok {
			return 0, io.EOF
		}
		r.curr = data
	}
	n = copy(p, r.curr)
	r.curr = r.curr[n:]
	return n, nil
}

type pipeWriter struct {
	ch chan []byte
}

func (w *pipeWriter) Write(p []byte) (n int, err error) {
	cp := make([]byte, len(p))
	copy(cp, p)
	w.ch <- cp
	return len(p), nil
}

func (w *pipeWriter) Close() error {
	close(w.ch)
	return nil
}
