package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"discord-osint/internal/matcher"
	"discord-osint/internal/store"
	"discord-osint/internal/target"
)

func TestUIServer_RoutesAndLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.sqlite")
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Seed test run
	runID := "run_ui_test"
	tgt := target.ConfirmedTarget{
		TargetInput:            "testuser",
		TargetUserID:           "123456789012345678",
		TargetUsername:         "testuser",
		TargetDisplayName:      "Test User",
		TargetResolutionSource: "users-api",
		TargetVerifiedAt:       time.Now().UTC(),
		TargetConfirmed:        true,
	}
	if err := s.CreateRunAtomic(ctx, runID, "crypto", tgt); err != nil {
		t.Fatalf("CreateRunAtomic failed: %v", err)
	}

	scan := store.GuildScanRecord{
		RunID:           runID,
		GuildID:         "guild_1",
		GuildName:       "Alpha Crypto Guild",
		InviteCode:      "alpha",
		ScanStatus:      "complete",
		BestMatchStatus: "confirmed",
		BestMatchReason: "user_id_exact",
		MemberCoverage:  "complete",
		StartedAt:       time.Now().UTC(),
	}

	obs := matcher.MatchObservation{
		GuildID:              "guild_1",
		ObservedUserID:       "123456789012345678",
		ObservedUsernameNorm: "testuser",
		ObservedUsername:     "testuser",
		MatchStatus:          matcher.MatchConfirmed,
		MatchReason:          matcher.ReasonUserIDExact,
		AcquisitionMethod:    "targeted_search",
		ObservedAt:           time.Now().UTC(),
	}

	msg := store.MessageRecord{
		RunID:             runID,
		GuildID:           "guild_1",
		ChannelID:         "chan_1",
		MessageID:         "msg_1",
		AuthorID:          "123456789012345678",
		AuthorUsername:    "testuser",
		Content:           "Investigating target activity.",
		Timestamp:         time.Now().UTC(),
		CollectedAt:       time.Now().UTC(),
		CollectorVersion:  "v1.0",
		AcquisitionMethod: "channel_api",
		CoverageStatus:    "bounded",
	}

	if err := s.RecordGuildScanTransaction(ctx, scan, []matcher.MatchObservation{obs}, []store.MessageRecord{msg}); err != nil {
		t.Fatalf("RecordGuildScanTransaction failed: %v", err)
	}

	// Start server on a dynamic port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on dynamic port: %v", err)
	}
	defer ln.Close()

	server := NewServer(ServerOptions{
		Addr:  ln.Addr().String(),
		Store: s,
	})

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- server.StartListener(ln)
	}()

	baseURL := fmt.Sprintf("http://%s", ln.Addr().String())
	client := &http.Client{Timeout: 3 * time.Second}

	// 1. Health check
	resp, err := client.Get(baseURL + "/api/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("health status code: %d", resp.StatusCode)
	}
	var health map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&health)
	if health["status"] != "ok" {
		t.Errorf("expected status ok, got %v", health)
	}

	// 2. Index HTML
	resp, err = client.Get(baseURL + "/")
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(bodyBytes), "Discord OSINT") {
		t.Errorf("index html missing title: %s", string(bodyBytes))
	}

	// 3. API runs list
	resp, err = client.Get(baseURL + "/api/runs")
	if err != nil {
		t.Fatalf("GET /api/runs failed: %v", err)
	}
	defer resp.Body.Close()
	var runs []store.RunSummary
	if err := json.NewDecoder(resp.Body).Decode(&runs); err != nil {
		t.Fatalf("failed to decode runs: %v", err)
	}
	if len(runs) != 1 || runs[0].RunID != runID {
		t.Errorf("unexpected runs response: %+v", runs)
	}

	// 4. API run detail
	resp, err = client.Get(fmt.Sprintf("%s/api/runs/%s", baseURL, runID))
	if err != nil {
		t.Fatalf("GET /api/runs/{id} failed: %v", err)
	}
	defer resp.Body.Close()
	var detail RunDetailPayload
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("failed to decode detail payload: %v", err)
	}
	if detail.Run.RunID != runID || len(detail.Scans) != 1 || len(detail.Observations) != 1 || len(detail.Messages) != 1 {
		t.Errorf("unexpected detail payload: %+v", detail)
	}

	// 5. API HTML report view
	resp, err = client.Get(fmt.Sprintf("%s/api/runs/%s/report.html", baseURL, runID))
	if err != nil {
		t.Fatalf("GET /api/runs/{id}/report.html failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("report.html returned HTTP %d", resp.StatusCode)
	}
	reportHTMLBytes, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(reportHTMLBytes), "Discord OSINT Report") {
		t.Errorf("report.html missing content: %s", string(reportHTMLBytes))
	}

	// 6. Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("server shutdown failed: %v", err)
	}

	select {
	case err := <-serverErrCh:
		if err != nil {
			t.Fatalf("server exited with error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("server did not exit in time after shutdown")
	}
}
