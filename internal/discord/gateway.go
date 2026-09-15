package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// GatewaySession manages the persistent WebSocket connection to the Discord Gateway.
type GatewaySession struct {
	token      string
	gatewayURL string
	proxyURL   string
	conn       *websocket.Conn
	sessionID  string
	sequence   int64
	mu         sync.RWMutex
	cancel     context.CancelFunc
	closed     chan struct{}
}

// GatewayPayload represents the standard Discord gateway envelope.
type GatewayPayload struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d"`
	S  *int64          `json:"s,omitempty"`
	T  string          `json:"t,omitempty"`
}

// NewGatewaySession creates an unstarted Gateway session.
func NewGatewaySession(token string, proxyURL string, customGatewayURL string) *GatewaySession {
	gwURL := customGatewayURL
	if gwURL == "" {
		gwURL = "wss://gateway.discord.gg/?v=10&encoding=json"
	}
	return &GatewaySession{
		token:      token,
		gatewayURL: gwURL,
		proxyURL:   proxyURL,
		closed:     make(chan struct{}),
	}
}

// Connect dials the Gateway, completes the handshake, and runs background heartbeat and dispatch routines.
func (g *GatewaySession) Connect(ctx context.Context) error {
	dialer := websocket.DefaultDialer
	if g.proxyURL != "" {
		proxy, err := url.Parse(g.proxyURL)
		if err == nil {
			dialer.Proxy = http.ProxyURL(proxy)
		}
	}

	headers := http.Header{}
	headers.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) discord/1.0.9168")

	conn, _, err := dialer.DialContext(ctx, g.gatewayURL, headers)
	if err != nil {
		return fmt.Errorf("gateway dial failed: %w", err)
	}
	g.conn = conn

	ctx, cancel := context.WithCancel(ctx)
	g.cancel = cancel

	// Read initial Op 10 HELLO message
	var helloMsg GatewayPayload
	if err := conn.ReadJSON(&helloMsg); err != nil {
		cancel()
		conn.Close()
		return fmt.Errorf("failed to read gateway hello: %w", err)
	}

	if helloMsg.Op != 10 {
		cancel()
		conn.Close()
		return fmt.Errorf("expected op 10 hello, got %d", helloMsg.Op)
	}

	var helloData struct {
		HeartbeatInterval int `json:"heartbeat_interval"`
	}
	if err := json.Unmarshal(helloMsg.D, &helloData); err != nil {
		cancel()
		conn.Close()
		return fmt.Errorf("failed to parse hello payload: %w", err)
	}

	// Send Op 2 IDENTIFY
	identifyPayload := map[string]any{
		"op": 2,
		"d": map[string]any{
			"token": g.token,
			"properties": map[string]any{
				"os":      "Mac OS X",
				"browser": "Discord Client",
				"device":  "Discord Client",
			},
		},
	}
	if err := conn.WriteJSON(identifyPayload); err != nil {
		cancel()
		conn.Close()
		return fmt.Errorf("failed to send gateway identify: %w", err)
	}

	// Start background routines
	interval := time.Duration(helloData.HeartbeatInterval) * time.Millisecond
	go g.heartbeatLoop(ctx, interval)
	go g.readLoop(ctx)

	return nil
}

// SessionID returns the authoritative gateway session ID received during READY event.
func (g *GatewaySession) SessionID() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.sessionID
}

func (g *GatewaySession) setSessionID(id string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sessionID = id
}

func (g *GatewaySession) setSequence(s int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sequence = s
}

func (g *GatewaySession) getSequence() *int64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.sequence == 0 {
		return nil
	}
	seq := g.sequence
	return &seq
}

func (g *GatewaySession) heartbeatLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 40 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.mu.Lock()
			if g.conn == nil {
				g.mu.Unlock()
				return
			}
			payload := GatewayPayload{
				Op: 1, // Heartbeat
				D:  json.RawMessage("null"),
				S:  g.getSequence(),
			}
			_ = g.conn.WriteJSON(payload)
			g.mu.Unlock()
		}
	}
}

func (g *GatewaySession) readLoop(ctx context.Context) {
	defer close(g.closed)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			var msg GatewayPayload
			err := g.conn.ReadJSON(&msg)
			if err != nil {
				return
			}

			if msg.S != nil {
				g.setSequence(*msg.S)
			}

			// Dispatch event
			if msg.Op == 0 && msg.T == "READY" {
				var readyData struct {
					SessionID string `json:"session_id"`
				}
				if err := json.Unmarshal(msg.D, &readyData); err == nil {
					g.setSessionID(readyData.SessionID)
				}
			}
		}
	}
}

// SubscribeLazyGuild sends Opcode 14 to subscribe to member updates for active channels.
func (g *GatewaySession) SubscribeLazyGuild(guildID string, channels map[string][][2]int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.conn == nil {
		return fmt.Errorf("gateway connection is closed")
	}

	payload := map[string]any{
		"op": 14,
		"d": map[string]any{
			"guild_id": guildID,
			"channels": channels,
		},
	}
	return g.conn.WriteJSON(payload)
}

// Close gracefully terminates the gateway session.
func (g *GatewaySession) Close() error {
	if g.cancel != nil {
		g.cancel()
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.conn != nil {
		err := g.conn.Close()
		g.conn = nil
		return err
	}
	return nil
}
