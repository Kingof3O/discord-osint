package export

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"discord-osint/internal/matcher"
	"discord-osint/internal/store"
	"discord-osint/internal/target"
)

func TestExportArtifacts(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.sqlite")
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("store.New failed: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	runID := "run_export_test"

	tgt := target.ConfirmedTarget{
		TargetInput:            "john.doe",
		TargetUserID:           "123456789012345678",
		TargetUsername:         "john.doe",
		TargetDisplayName:      "John Doe",
		TargetResolutionSource: "users-api",
		TargetVerifiedAt:       time.Now().UTC(),
		TargetConfirmed:        true,
	}

	if err := s.CreateRunAtomic(ctx, runID, "gaming", tgt); err != nil {
		t.Fatalf("CreateRunAtomic failed: %v", err)
	}

	scan := store.GuildScanRecord{
		RunID:           runID,
		GuildID:         "guild_101",
		GuildName:       "Test Gaming Guild",
		InviteCode:      "gaming-invite",
		ScanStatus:      "complete",
		BestMatchStatus: "confirmed",
		BestMatchReason: "user_id_exact",
		MemberCoverage:  "bounded",
		StartedAt:       time.Now().UTC(),
	}

	obs := matcher.MatchObservation{
		GuildID:              "guild_101",
		ObservedUserID:       "123456789012345678",
		ObservedUsernameNorm: "john.doe",
		ObservedUsername:     "john.doe",
		ObservedJoinedAt:     "2023-01-01T00:00:00Z",
		ObservedRoles:        []string{"Admin"},
		MatchStatus:          matcher.MatchConfirmed,
		MatchReason:          matcher.ReasonUserIDExact,
		AcquisitionMethod:    "targeted_search",
		ObservedAt:           time.Now().UTC(),
	}

	msg := store.MessageRecord{
		RunID:             runID,
		GuildID:           "guild_101",
		GuildName:         "Test Gaming Guild",
		ChannelID:         "chan_general",
		ChannelName:       "general-chat",
		MessageID:         "msg_001",
		AuthorID:          "123456789012345678",
		AuthorUsername:    "john.doe",
		Content:           "Hello world in gaming guild!",
		Timestamp:         time.Now().UTC(),
		CollectedAt:       time.Now().UTC(),
		CollectorVersion:  "v3.3",
		AcquisitionMethod: "guild_search_api",
		CoverageStatus:    "bounded",
	}

	if err := s.RecordGuildScanTransaction(ctx, scan, []matcher.MatchObservation{obs}, []store.MessageRecord{msg}); err != nil {
		t.Fatalf("RecordGuildScanTransaction failed: %v", err)
	}

	outDir := filepath.Join(tmpDir, "exports")
	opts := ExportOptions{
		OutputDir: outDir,
		Formats:   []string{"json", "csv", "md"},
	}

	if err := ExportArtifacts(ctx, s, runID, opts); err != nil {
		t.Fatalf("ExportArtifacts failed: %v", err)
	}

	// Verify hits.json
	hitsBytes, err := os.ReadFile(filepath.Join(outDir, "hits.json"))
	if err != nil || !strings.Contains(string(hitsBytes), "Test Gaming Guild") {
		t.Errorf("invalid hits.json content: %s", string(hitsBytes))
	}

	// Verify observations.json
	obsBytes, err := os.ReadFile(filepath.Join(outDir, "observations.json"))
	if err != nil || !strings.Contains(string(obsBytes), "Admin") {
		t.Errorf("invalid observations.json content: %s", string(obsBytes))
	}

	// Verify messages.csv
	csvBytes, err := os.ReadFile(filepath.Join(outDir, "messages.csv"))
	if err != nil || !strings.Contains(string(csvBytes), "Hello world in gaming guild!") || !strings.Contains(string(csvBytes), "general-chat") {
		t.Errorf("invalid messages.csv content: %s", string(csvBytes))
	}

	// Verify report.md
	reportBytes, err := os.ReadFile(filepath.Join(outDir, "report.md"))
	if err != nil || !strings.Contains(string(reportBytes), "Confirmed Target Presence") || !strings.Contains(string(reportBytes), "Test Gaming Guild") {
		t.Errorf("invalid report.md content: %s", string(reportBytes))
	}

	// Verify report.html
	htmlBytes, err := os.ReadFile(filepath.Join(outDir, "report.html"))
	if err != nil || !strings.Contains(string(htmlBytes), "Discord OSINT Report") || !strings.Contains(string(htmlBytes), "badge-confirmed") || !strings.Contains(string(htmlBytes), "msg-server-pill") || !strings.Contains(string(htmlBytes), "general-chat") {
		t.Errorf("invalid report.html content: %s", string(htmlBytes))
	}
}
