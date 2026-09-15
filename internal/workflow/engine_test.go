package workflow

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"discord-osint/internal/config"
	"discord-osint/internal/disboard"
	"discord-osint/internal/store"
	"discord-osint/internal/target"
)

type testPromptUI struct {
	confirmed bool
}

func (m *testPromptUI) DisplayCandidate(c target.Candidate, warning string) {}
func (m *testPromptUI) DisplayUnresolved(username string, warning string)   {}
func (m *testPromptUI) SelectCandidate(candidates []target.Candidate) (int, error) {
	return 0, nil
}
func (m *testPromptUI) Confirm(prompt string) (bool, error) {
	return m.confirmed, nil
}

func TestEngine_RunLifecycle_Mock(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.sqlite")
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("store.New failed: %v", err)
	}
	defer s.Close()

	cfg := config.DefaultConfig()
	cfg.JoinDelaySec = [2]int{0, 0} // zero delay for fast test

	var out bytes.Buffer
	ui := &testPromptUI{confirmed: true}

	engine := NewEngine(cfg, s, nil, nil, nil, nil, nil, ui, &out)

	params := SearchParams{
		Tag:         "gaming",
		Username:    "target_user",
		DirectCodes: []string{"invite-alpha", "invite-beta"},
		Limit:       2,
		AutoConfirm: true,
	}

	runID, err := engine.Run(context.Background(), params)
	if err != nil {
		t.Fatalf("engine.Run failed: %v", err)
	}

	if runID == "" {
		t.Fatalf("expected non-empty runID")
	}

	// Verify run was recorded in store
	run, err := s.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}
	if run.Tag != "gaming" || run.TargetUsername != "target_user" {
		t.Errorf("mismatched run record: %+v", run)
	}

	// Verify scans were recorded
	scans, err := s.GetRunGuildScans(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRunGuildScans failed: %v", err)
	}
	if len(scans) != 2 {
		t.Errorf("expected 2 guild scans, got %d", len(scans))
	}
}

func TestEngine_ServerNames_DisboardDiscovery(t *testing.T) {
	// Mock Disboard server returning target server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keyword := r.URL.Query().Get("keyword")
		w.Header().Set("Content-Type", "text/html")
		if strings.Contains(keyword, "Alpha") {
			fmt.Fprint(w, `<div class="server-card" data-id="guild_target_1">
				<div class="server-name">Alpha DeFi Official</div>
				<div class="server-members">5,000 Members</div>
				<a class="server-join" href="/join/alpha-defi-code">Join</a>
			</div>`)
		} else {
			fmt.Fprint(w, `<div class="empty">No results</div>`)
		}
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.sqlite")
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("store.New failed: %v", err)
	}
	defer s.Close()

	scraper := disboard.NewScraper(ts.Client(), "")
	scraper.BaseURL = ts.URL
	scraper.RequestDelay = 1 * time.Millisecond

	cfg := config.DefaultConfig()
	cfg.JoinDelaySec = [2]int{0, 0}

	var out bytes.Buffer
	ui := &testPromptUI{confirmed: true}

	engine := NewEngine(cfg, s, nil, nil, nil, scraper, nil, ui, &out)

	params := SearchParams{
		Tag:         "crypto",
		Username:    "target_user",
		ServerNames: []string{"Alpha DeFi"},
		Limit:       1,
		AutoConfirm: true,
	}

	runID, err := engine.Run(context.Background(), params)
	if err != nil {
		t.Fatalf("engine.Run failed: %v", err)
	}

	// Verify server Alpha DeFi Official was discovered and recorded as scan
	scans, err := s.GetRunGuildScans(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRunGuildScans failed: %v", err)
	}

	if len(scans) != 1 {
		t.Fatalf("expected 1 scan for targeted server name, got %d", len(scans))
	}

	if scans[0].GuildName != "Alpha DeFi Official" || scans[0].InviteCode != "alpha-defi-code" {
		t.Errorf("unexpected scan record: %+v", scans[0])
	}

	// Output contains target announcement
	if !strings.Contains(out.String(), "Discovered TARGET server for \"Alpha DeFi\"") {
		t.Errorf("expected target announcement in output: %s", out.String())
	}
}

