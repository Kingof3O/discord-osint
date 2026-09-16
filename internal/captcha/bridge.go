package captcha

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// InteractiveBridge runs a transient local web bridge and CLI prompt for manual CAPTCHA solving.
type InteractiveBridge struct {
	cfg           Config
	reader        io.Reader
	writer        io.Writer
	openBrowserFn func(url string) error
}

// NewInteractiveBridge creates a new interactive CAPTCHA solver bridge.
func NewInteractiveBridge(cfg Config) *InteractiveBridge {
	return &InteractiveBridge{
		cfg:           cfg,
		reader:        os.Stdin,
		writer:        os.Stdout,
		openBrowserFn: defaultOpenBrowser,
	}
}

// SetIO allows overriding reader/writer for automated testing.
func (b *InteractiveBridge) SetIO(r io.Reader, w io.Writer) {
	b.reader = r
	b.writer = w
}

// SetOpenBrowserFn allows overriding browser opener for testing.
func (b *InteractiveBridge) SetOpenBrowserFn(fn func(url string) error) {
	b.openBrowserFn = fn
}

func defaultOpenBrowser(targetURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", targetURL)
	case "linux":
		cmd = exec.Command("xdg-open", targetURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL)
	default:
		return fmt.Errorf("unsupported platform for auto-open")
	}
	return cmd.Start()
}

const captchaHTMLTemplate = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Discord OSINT &mdash; CAPTCHA Challenge</title>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    {{ if eq .Service "turnstile" }}
    <script src="https://challenges.cloudflare.com/turnstile/v0/api.js" async defer></script>
    {{ else }}
    <script src="https://js.hcaptcha.com/1/api.js?onload=onHCaptchaLoaded&render=explicit" async defer></script>
    {{ end }}
    <style>
        body {
            background-color: #1e1f22;
            color: #f2f3f5;
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            display: flex;
            flex-direction: column;
            align-items: center;
            justify-content: center;
            min-height: 100vh;
            margin: 0;
        }
        .card {
            background: #2b2d31;
            padding: 32px;
            border-radius: 12px;
            box-shadow: 0 4px 20px rgba(0,0,0,0.4);
            max-width: 480px;
            text-align: center;
        }
        h2 { margin-top: 0; color: #5865f2; }
        p { color: #dbdee1; line-height: 1.5; font-size: 14px; }
        .widget-container { margin: 24px 0; display: flex; justify-content: center; }
        .status { margin-top: 16px; font-weight: bold; color: #23a55a; display: none; }
        .footer { font-size: 12px; color: #949ba4; margin-top: 16px; }
    </style>
</head>
<body>
    <div class="card">
        <h2>CAPTCHA Verification Required</h2>
        <p>A verification challenge was triggered for <strong>{{ if .GuildName }}{{ .GuildName }}{{ else }}Discord Server{{ end }}</strong>.</p>
        <p>Please complete the challenge below. The solved token will be transmitted automatically back to the CLI.</p>
        
        <div class="widget-container">
            {{ if eq .Service "turnstile" }}
            <div class="cf-turnstile" data-sitekey="{{ .SiteKey }}" data-callback="onSuccess"></div>
            {{ else }}
            <div id="hcaptcha-widget" class="h-captcha" data-sitekey="{{ .SiteKey }}" {{ if .RqData }}data-rqdata="{{ .RqData }}"{{ end }} data-callback="onSuccess"></div>
            {{ end }}
        </div>

        <div id="status" class="status">&#10004; Solved! Returning to CLI...</div>
        <div class="footer">discord-osint local bridge &bull; Port {{ .Port }}</div>
    </div>

    <script>
        function onSuccess(token) {
            document.getElementById('status').style.display = 'block';
            fetch('/submit', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ token: token })
            }).then(function(res) {
                if (res.ok) {
                    setTimeout(function() { window.close(); }, 1500);
                }
            }).catch(function(err) {
                console.error('Submission failed', err);
            });
        }

        window.onHCaptchaLoaded = function() {
            try {
                if (typeof hcaptcha !== 'undefined') {
                    var container = document.getElementById('hcaptcha-widget');
                    if (container && !container.hasChildNodes()) {
                        var sitekey = container.getAttribute('data-sitekey');
                        var rqdata = container.getAttribute('data-rqdata');
                        var opts = {
                            sitekey: sitekey,
                            callback: onSuccess
                        };
                        if (rqdata) {
                            opts.rqdata = rqdata;
                        }
                        hcaptcha.render(container, opts);
                    }
                }
            } catch (err) {
                console.warn('Programmatic hcaptcha.render note:', err);
            }
        };
    </script>
</body>
</html>`

type templateData struct {
	Service   string
	SiteKey   string
	SessionID string
	RqData    string
	GuildName string
	Port      int
}

// Solve listens on a local port, presents the bridge in browser, and waits for completion or CLI input.
func (b *InteractiveBridge) Solve(ctx context.Context, ch Challenge) (Solution, error) {
	// 1. Bind local listener
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", b.cfg.Port))
	if err != nil {
		// Fallback to automatic dynamic port if configured port is taken
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return Solution{}, fmt.Errorf("failed to bind captcha bridge listener: %w", err)
		}
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	bridgeURL := fmt.Sprintf("http://127.0.0.1:%d/captcha", port)

	// 2. Setup channels
	solutionChan := make(chan string, 1)
	errChan := make(chan error, 1)
	var once sync.Once

	sendSolution := func(tok string) {
		once.Do(func() {
			solutionChan <- tok
		})
	}
	sendErr := func(e error) {
		once.Do(func() {
			errChan <- e
		})
	}

	// 3. Setup HTTP multiplexer
	tmpl, err := template.New("captcha").Parse(captchaHTMLTemplate)
	if err != nil {
		return Solution{}, fmt.Errorf("failed to parse captcha template: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/captcha", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		data := templateData{
			Service:   ch.Service,
			SiteKey:   ch.SiteKey,
			SessionID: ch.SessionID,
			RqData:    ch.RqData,
			GuildName: ch.GuildName,
			Port:      port,
		}
		_ = tmpl.Execute(w, data)
	})

	mux.HandleFunc("/submit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var payload struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Token == "" {
			http.Error(w, "invalid token payload", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))

		sendSolution(payload.Token)
	})

	srv := &http.Server{Handler: mux}
	go func() {
		_ = srv.Serve(listener)
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	// 4. Print alert banner
	if !b.cfg.DisableBell {
		fmt.Fprint(b.writer, "\a") // Terminal bell sound
	}

	guildLabel := ch.GuildName
	if guildLabel == "" && ch.GuildID != "" {
		guildLabel = ch.GuildID
	}
	if guildLabel == "" {
		guildLabel = "Discord Guild"
	}

	fmt.Fprintln(b.writer, "")
	fmt.Fprintln(b.writer, "======================================================================")
	fmt.Fprintf(b.writer, " [!] CAPTCHA CHALLENGE TRIGGERED for: %s\n", guildLabel)
	fmt.Fprintf(b.writer, "     Service: %s\n", ch.Service)
	fmt.Fprintf(b.writer, "     Sitekey: %s\n", ch.SiteKey)
	fmt.Fprintf(b.writer, "     Bridge:  %s (opened in browser)\n", bridgeURL)
	fmt.Fprintln(b.writer, "======================================================================")
	fmt.Fprintf(b.writer, "Solve in your browser or paste the token response below.\n")
	fmt.Fprintf(b.writer, "Options: [paste-token] / 's' to skip this server / 'q' to abort run: ")

	// 5. Open browser if enabled
	if b.cfg.AutoOpen && b.openBrowserFn != nil {
		go func() {
			_ = b.openBrowserFn(bridgeURL)
		}()
	}

	// 6. Listen for CLI input in parallel
	go func() {
		scanner := bufio.NewScanner(b.reader)
		if scanner.Scan() {
			text := strings.TrimSpace(scanner.Text())
			lower := strings.ToLower(text)
			switch lower {
			case "s", "skip":
				sendErr(ErrCaptchaSkipped)
			case "q", "quit", "abort":
				sendErr(ErrCaptchaAborted)
			default:
				if len(text) > 15 {
					sendSolution(text)
				}
			}
		}
	}()

	// 7. Wait for completion, timeout, or context cancellation
	timeout := b.cfg.Timeout
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return Solution{}, ctx.Err()
	case <-timer.C:
		return Solution{}, ErrCaptchaTimeout
	case err := <-errChan:
		return Solution{}, err
	case token := <-solutionChan:
		fmt.Fprintf(b.writer, "\n[+] CAPTCHA Token received successfully (%d bytes).\n", len(token))
		return Solution{
			Token:     token,
			RqToken:   ch.RqToken,
			SessionID: ch.SessionID,
		}, nil
	}
}
