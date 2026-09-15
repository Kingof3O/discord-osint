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
