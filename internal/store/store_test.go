package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"discord-osint/internal/matcher"
	"discord-osint/internal/target"
)

func newTestStore(t *testing.T) *Store {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.sqlite")
	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStore_CreateRunAtomic(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tgt := target.ConfirmedTarget{
		TargetInput:            "user123",
		TargetUserID:           "123456789012345678",
		TargetUsername:         "john.doe",
		TargetDisplayName:      "John Doe",
		TargetResolutionSource: "users-api",
		TargetVerifiedAt:       time.Now().UTC(),
		TargetConfirmed:        true,
	}

	err := s.CreateRunAtomic(ctx, "run_001", "gaming", tgt)
	if err != nil {
		t.Fatalf("CreateRunAtomic failed: %v", err)
	}

	run, err := s.GetRun(ctx, "run_001")
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}

	if run.RunID != "run_001" || run.TargetUserID != tgt.TargetUserID || run.TargetUsername != tgt.TargetUsername {
		t.Errorf("mismatched run record: %+v", run)
	}
}

func TestStore_CheckConstraintNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// 1. Attempting not_found with partial member coverage MUST FAIL check constraint
	invalidScan := GuildScanRecord{
		RunID:           "run_chk",
		GuildID:         "guild_1",
		ScanStatus:      "complete",
		BestMatchStatus: "not_found",
		MemberCoverage:  "partial", // Invalid combination!
		StartedAt:       time.Now().UTC(),
	}

	err := s.RecordGuildScanTransaction(ctx, invalidScan, nil, nil)
	if err == nil {
		t.Fatalf("expected check constraint violation for not_found with partial member coverage")
	}
	if !strings.Contains(err.Error(), "CHECK constraint failed") && !strings.Contains(err.Error(), "constraint") {
		t.Errorf("expected constraint error, got: %v", err)
	}

	// 2. not_found with complete member coverage MUST SUCCEED
	validScan := GuildScanRecord{
		RunID:           "run_chk",
		GuildID:         "guild_2",
		ScanStatus:      "complete",
		BestMatchStatus: "not_found",
		MemberCoverage:  "complete", // Valid!
		StartedAt:       time.Now().UTC(),
	}

	err = s.RecordGuildScanTransaction(ctx, validScan, nil, nil)
	if err != nil {
		t.Fatalf("expected success for not_found with complete coverage, got error: %v", err)
	}
}

func TestStore_PromotionOnly(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Step 1: Record candidate match
	scan1 := GuildScanRecord{
		RunID:           "run_promo",
		GuildID:         "guild_promo",
		ScanStatus:      "running",
		BestMatchStatus: "candidate",
		BestMatchReason: "username_exact",
		MemberCoverage:  "bounded",
		StartedAt:       time.Now().UTC(),
	}
	if err := s.RecordGuildScanTransaction(ctx, scan1, nil, nil); err != nil {
		t.Fatalf("failed to insert scan1: %v", err)
	}

	// Step 2: Promote candidate -> confirmed
	scan2 := GuildScanRecord{
		RunID:           "run_promo",
		GuildID:         "guild_promo",
		ScanStatus:      "running",
		BestMatchStatus: "confirmed",
		BestMatchReason: "user_id_exact",
		MemberCoverage:  "bounded",
		StartedAt:       time.Now().UTC(),
	}
	if err := s.RecordGuildScanTransaction(ctx, scan2, nil, nil); err != nil {
		t.Fatalf("failed to insert scan2: %v", err)
	}

	scans, err := s.GetRunGuildScans(ctx, "run_promo")
	if err != nil || len(scans) != 1 {
		t.Fatalf("expected 1 scan record, got err=%v, count=%d", err, len(scans))
	}
	if scans[0].BestMatchStatus != "confirmed" {
		t.Errorf("expected promoted to confirmed, got %s", scans[0].BestMatchStatus)
	}

	// Step 3: Attempt downgrade confirmed -> candidate (MUST NOT downgrade)
	scan3 := GuildScanRecord{
		RunID:           "run_promo",
		GuildID:         "guild_promo",
		ScanStatus:      "complete",
		BestMatchStatus: "candidate",
		BestMatchReason: "username_exact",
		MemberCoverage:  "bounded",
		StartedAt:       time.Now().UTC(),
	}
	if err := s.RecordGuildScanTransaction(ctx, scan3, nil, nil); err != nil {
		t.Fatalf("failed to insert scan3: %v", err)
	}

	scans, err = s.GetRunGuildScans(ctx, "run_promo")
	if err != nil || len(scans) != 1 {
		t.Fatalf("expected 1 scan record, got count %d", len(scans))
	}
	if scans[0].BestMatchStatus != "confirmed" {
		t.Errorf("DOWNGRADE VIOLATION: confirmed was overwritten by candidate! Current: %s", scans[0].BestMatchStatus)
	}
}

func TestStore_ObservationsUniqueConstraint(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	scan := GuildScanRecord{
		RunID:           "run_obs",
		GuildID:         "g100",
		ScanStatus:      "complete",
		BestMatchStatus: "confirmed",
		MemberCoverage:  "bounded",
		StartedAt:       time.Now().UTC(),
	}

	// 2 distinct candidate identities + 1 confirmed identity in the same guild
	obs1 := matcher.MatchObservation{
		GuildID:              "g100",
		ObservedUserID:       "111",
		ObservedUsernameNorm: "john",
		ObservedUsername:     "John",
		MatchStatus:          matcher.MatchCandidate,
		MatchReason:          matcher.ReasonUsernameExact,
		ObservedAt:           time.Now().UTC(),
	}
	obs2 := matcher.MatchObservation{
		GuildID:              "g100",
		ObservedUserID:       "222",
		ObservedUsernameNorm: "john",
		ObservedUsername:     "John",
		MatchStatus:          matcher.MatchCandidate,
		MatchReason:          matcher.ReasonUsernameExact,
		ObservedAt:           time.Now().UTC(),
	}
	obs3 := matcher.MatchObservation{
		GuildID:              "g100",
		ObservedUserID:       "999",
		ObservedUsernameNorm: "target",
		ObservedUsername:     "Target",
		MatchStatus:          matcher.MatchConfirmed,
		MatchReason:          matcher.ReasonUserIDExact,
		ObservedAt:           time.Now().UTC(),
	}

	err := s.RecordGuildScanTransaction(ctx, scan, []matcher.MatchObservation{obs1, obs2, obs3}, nil)
	if err != nil {
		t.Fatalf("failed to record observations: %v", err)
	}

	storedObs, err := s.GetRunObservations(ctx, "run_obs")
	if err != nil {
		t.Fatalf("failed to get observations: %v", err)
	}
	if len(storedObs) != 3 {
		t.Errorf("expected 3 distinct observations stored, got %d", len(storedObs))
	}

	// Re-inserting the same observations should not error and should not duplicate
	err = s.RecordGuildScanTransaction(ctx, scan, []matcher.MatchObservation{obs1}, nil)
	if err != nil {
		t.Fatalf("idempotent insert failed: %v", err)
	}
	storedObs, _ = s.GetRunObservations(ctx, "run_obs")
	if len(storedObs) != 3 {
		t.Errorf("expected still 3 observations after duplicate insert, got %d", len(storedObs))
	}
}

func TestStore_ResumeQuery(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// 1. Pending scan
	s.RecordGuildScanTransaction(ctx, GuildScanRecord{
		RunID: "run_res", GuildID: "g_pending", ScanStatus: "pending", StartedAt: time.Now().UTC(),
	}, nil, nil)
	// 2. Error scan
	s.RecordGuildScanTransaction(ctx, GuildScanRecord{
		RunID: "run_res", GuildID: "g_error", ScanStatus: "error", StartedAt: time.Now().UTC(),
	}, nil, nil)
	// 3. Complete scan (terminal)
	s.RecordGuildScanTransaction(ctx, GuildScanRecord{
		RunID: "run_res", GuildID: "g_complete", ScanStatus: "complete", MemberCoverage: "complete", BestMatchStatus: "not_found", StartedAt: time.Now().UTC(),
	}, nil, nil)
	// 4. Blocked scan (terminal)
	s.RecordGuildScanTransaction(ctx, GuildScanRecord{
		RunID: "run_res", GuildID: "g_blocked", ScanStatus: "blocked", StartedAt: time.Now().UTC(),
	}, nil, nil)

	resumable, err := s.GetResumableGuildScans(ctx, "run_res")
	if err != nil {
		t.Fatalf("GetResumableGuildScans failed: %v", err)
	}

	if len(resumable) != 2 {
		t.Fatalf("expected 2 resumable scans (pending and error), got %d", len(resumable))
	}
	for _, r := range resumable {
		if r.ScanStatus != "pending" && r.ScanStatus != "error" {
			t.Errorf("unexpected status in resumable: %s", r.ScanStatus)
		}
	}
}

func TestStore_ListRunsAndLatestIncomplete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// 1. Initial check - no runs
	latest, err := s.GetLatestIncompleteRun(ctx)
	if err != nil {
		t.Fatalf("GetLatestIncompleteRun failed: %v", err)
	}
	if latest != nil {
		t.Fatalf("expected nil latest run, got: %+v", latest)
	}

	// 2. Create Run 1 (completed)
	tgt1 := target.ConfirmedTarget{
		TargetInput:       "user1",
		TargetUserID:      "1001",
		TargetUsername:    "alice",
		TargetDisplayName: "Alice",
		TargetConfirmed:   true,
		TargetVerifiedAt:  time.Now().UTC(),
	}
	if err := s.CreateRunAtomic(ctx, "run_001", "gaming", tgt1); err != nil {
		t.Fatalf("CreateRunAtomic run_001: %v", err)
	}
	// Add a complete scan
	s.RecordGuildScanTransaction(ctx, GuildScanRecord{
		RunID: "run_001", GuildID: "g_1", ScanStatus: "complete", MemberCoverage: "complete", BestMatchStatus: "not_found", StartedAt: time.Now().UTC(),
	}, nil, nil)
	if err := s.UpdateRunStatus(ctx, "run_001", "complete"); err != nil {
		t.Fatalf("UpdateRunStatus run_001: %v", err)
	}

	// 3. Create Run 2 (incomplete, has pending server and candidate match)
	tgt2 := target.ConfirmedTarget{
		TargetInput:       "user2",
		TargetUserID:      "1002",
		TargetUsername:    "bob",
		TargetDisplayName: "Bob",
		TargetConfirmed:   true,
		TargetVerifiedAt:  time.Now().UTC(),
	}
	if err := s.CreateRunAtomic(ctx, "run_002", "crypto", tgt2); err != nil {
		t.Fatalf("CreateRunAtomic run_002: %v", err)
	}
	s.RecordGuildScanTransaction(ctx, GuildScanRecord{
		RunID: "run_002", GuildID: "g_2", ScanStatus: "pending", StartedAt: time.Now().UTC(),
	}, nil, nil)
	s.RecordGuildScanTransaction(ctx, GuildScanRecord{
		RunID: "run_002", GuildID: "g_3", ScanStatus: "complete", MemberCoverage: "complete", BestMatchStatus: "candidate", StartedAt: time.Now().UTC(),
	}, nil, nil)

	// List runs
	runs, err := s.ListRuns(ctx)
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}

	// Newest first -> run_002 should be first
	if runs[0].RunID != "run_002" {
		t.Errorf("expected run_002 first, got %s", runs[0].RunID)
	}
	if runs[0].TotalServers != 2 || runs[0].PendingServers != 1 || runs[0].CandidateMatches != 1 {
		t.Errorf("run_002 unexpected stats: %+v", runs[0])
	}

	// Run 1 check
	if runs[1].RunID != "run_001" {
		t.Errorf("expected run_001 second, got %s", runs[1].RunID)
	}
	if runs[1].Status != "complete" || runs[1].CompletedServers != 1 {
		t.Errorf("run_001 unexpected stats: %+v", runs[1])
	}

	// Test GetLatestIncompleteRun
	latest, err = s.GetLatestIncompleteRun(ctx)
	if err != nil {
		t.Fatalf("GetLatestIncompleteRun failed: %v", err)
	}
	if latest == nil || latest.RunID != "run_002" {
		t.Fatalf("expected run_002 as latest incomplete run, got: %+v", latest)
	}
}

func TestStore_MessageGuildAndChannelNames(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tgt := target.ConfirmedTarget{
		TargetUserID:     "target_999",
		TargetUsername:   "alpha_user",
		TargetVerifiedAt: time.Now().UTC(),
	}
	if err := s.CreateRunAtomic(ctx, "run_msg_test", "tag1", tgt); err != nil {
		t.Fatalf("CreateRunAtomic failed: %v", err)
	}

	scan := GuildScanRecord{
		RunID:          "run_msg_test",
		GuildID:        "guild_msg_1",
		GuildName:      "Alpha Guild",
		ScanStatus:     "complete",
		MemberCoverage: "complete",
		StartedAt:      time.Now().UTC(),
	}

	msg1 := MessageRecord{
		RunID:             "run_msg_test",
		GuildID:           "guild_msg_1",
		GuildName:         "Alpha Guild",
		ChannelID:         "chan_101",
		ChannelName:       "general",
		MessageID:         "m_1",
		AuthorID:          "target_999",
		AuthorUsername:    "alpha_user",
		Content:           "test msg 1",
		Timestamp:         time.Now().UTC(),
		CollectedAt:       time.Now().UTC(),
		CollectorVersion:  "v3.3",
		AcquisitionMethod: "search_api",
		CoverageStatus:    "bounded",
	}

	if err := s.RecordGuildScanTransaction(ctx, scan, nil, []MessageRecord{msg1}); err != nil {
		t.Fatalf("RecordGuildScanTransaction failed: %v", err)
	}

	msgs, err := s.GetRunMessages(ctx, "run_msg_test")
	if err != nil {
		t.Fatalf("GetRunMessages failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].GuildName != "Alpha Guild" || msgs[0].ChannelName != "general" {
		t.Errorf("unexpected message names: guild=%q, chan=%q", msgs[0].GuildName, msgs[0].ChannelName)
	}

	// Test UpdateMessageMetadata
	if err := s.UpdateMessageMetadata(ctx, "run_msg_test", "guild_msg_1", "chan_101", "Alpha Guild Updated", "general-chat"); err != nil {
		t.Fatalf("UpdateMessageMetadata failed: %v", err)
	}

	msgs, err = s.GetRunMessages(ctx, "run_msg_test")
	if err != nil {
		t.Fatalf("GetRunMessages after update failed: %v", err)
	}
	if msgs[0].GuildName != "Alpha Guild Updated" || msgs[0].ChannelName != "general-chat" {
		t.Errorf("unexpected updated message names: guild=%q, chan=%q", msgs[0].GuildName, msgs[0].ChannelName)
	}
}


