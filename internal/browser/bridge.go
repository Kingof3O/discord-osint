package browser

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
)

var (
	urlRegex = regexp.MustCompile(`https?://[^\s<>"]+`)
)

// Bridge handles opening external verification links and coordinating operator browser verification.
type Bridge struct {
	autoOpen      bool
	browserExec   string
	reader        io.Reader
	writer        io.Writer
	openBrowserFn func(targetURL string) error
	mu            sync.Mutex
}

// Options configures the browser bridge.
type Options struct {
	AutoOpen    bool
	BrowserExec string
	Reader      io.Reader
	Writer      io.Writer
}

// NewBridge creates a new browser verification bridge.
func NewBridge(opts Options) *Bridge {
	reader := opts.Reader
	if reader == nil {
		reader = os.Stdin
	}
	writer := opts.Writer
	if writer == nil {
		writer = os.Stdout
	}

	b := &Bridge{
		autoOpen:    opts.AutoOpen,
		browserExec: opts.BrowserExec,
		reader:      reader,
		writer:      writer,
	}
	b.openBrowserFn = b.defaultOpen
	return b
}

// SetOpenBrowserFn allows overriding the browser launcher for unit testing.
func (b *Bridge) SetOpenBrowserFn(fn func(targetURL string) error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.openBrowserFn = fn
}

// Open launches the target URL in the default or configured browser.
func (b *Bridge) Open(targetURL string) error {
	b.mu.Lock()
	fn := b.openBrowserFn
	b.mu.Unlock()
	if fn != nil {
		return fn(targetURL)
	}
	return b.defaultOpen(targetURL)
}

func (b *Bridge) defaultOpen(targetURL string) error {
	if b.browserExec != "" {
		cmd := exec.Command(b.browserExec, targetURL)
		return cmd.Start()
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", targetURL)
	case "linux":
		cmd = exec.Command("xdg-open", targetURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL)
	default:
		return fmt.Errorf("unsupported OS %s for auto-launching browser", runtime.GOOS)
	}
	return cmd.Start()
}

// ExtractVerificationURL finds the first http/https URL in text, prioritizing known verification services.
func ExtractVerificationURL(content string) string {
	matches := urlRegex.FindAllString(content, -1)
	if len(matches) == 0 {
		return ""
	}

	knownDomains := []string{
		"altdentifier.com",
		"wickbot.com",
		"doublecounter",
		"restorecord.com",
		"vulcan.bot",
		"captcha.site",
		"disboard.org",
	}

	// First pass: look for a known verification domain
	for _, m := range matches {
		lower := strings.ToLower(m)
		for _, kd := range knownDomains {
			if strings.Contains(lower, kd) {
				// Clean trailing punctuation if any
				return cleanURL(m)
			}
		}
	}

	// Fallback to first valid URL
	return cleanURL(matches[0])
}

func cleanURL(raw string) string {
	cleaned := strings.TrimRight(raw, ".,;:)>]}")
	if u, err := url.Parse(cleaned); err == nil && u.Scheme != "" && u.Host != "" {
		return u.String()
	}
	return raw
}

// HandleVerification coordinates presenting the external verification link to the operator.
func (b *Bridge) HandleVerification(ctx context.Context, targetURL, serviceName string) (bool, error) {
	fmt.Fprintf(b.writer, "\n[!] External Bot Verification Required: %s\n", serviceName)
	fmt.Fprintf(b.writer, "    Verification URL: %s\n", targetURL)

	if b.autoOpen {
		fmt.Fprintf(b.writer, "    [*] Attempting to launch browser automatically...\n")
		if err := b.Open(targetURL); err != nil {
			fmt.Fprintf(b.writer, "    [-] Failed to auto-launch browser (%v). Please open the URL manually.\n", err)
		} else {
			fmt.Fprintf(b.writer, "    [+] Browser window launched.\n")
		}
	} else {
		fmt.Fprintf(b.writer, "    [*] Auto-browser launch is disabled. Please open the URL manually.\n")
	}

	fmt.Fprintf(b.writer, "\n    Has verification been completed in the browser? [y/N]: ")

	ansCh := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(b.reader)
		if scanner.Scan() {
			ansCh <- strings.TrimSpace(scanner.Text())
		} else {
			ansCh <- "n"
		}
	}()

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case ans := <-ansCh:
		ansLower := strings.ToLower(ans)
		if ansLower == "y" || ansLower == "yes" {
			fmt.Fprintln(b.writer, "    [+] Verification confirmed by operator.")
			return true, nil
		}
		fmt.Fprintln(b.writer, "    [-] Verification skipped by operator.")
		return false, nil
	}
}
