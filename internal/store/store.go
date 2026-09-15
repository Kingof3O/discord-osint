package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"discord-osint/internal/matcher"
	"discord-osint/internal/target"

	_ "modernc.org/sqlite"
)

var (
	ErrRunNotFound     = errors.New("run not found")
	ErrGuildScanExists = errors.New("guild scan already exists")
)

// Store manages SQLite persistence for discord-osint.
type Store struct {
	db *sql.DB
}

// New initializes the SQLite database, applies pragmas, and runs migrations.
func New(dbPath string) (*Store, error) {
	// Enable foreign keys, WAL mode, and busy timeout for concurrent safety
	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping sqlite database: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate schema: %w", err)
	}

	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	var version int
	row := s.db.QueryRow("PRAGMA user_version")
	if err := row.Scan(&version); err != nil {
		return err
	}

	if version < 1 {
		if _, err := s.db.Exec(schemaV1); err != nil {
			return fmt.Errorf("failed to apply schema v1: %w", err)
		}
		if _, err := s.db.Exec(fmt.Sprintf("PRAGMA user_version = %d", currentSchemaVersion)); err != nil {
			return fmt.Errorf("failed to set pragma user_version: %w", err)
		}
	}

	return nil
}

// CreateRunAtomic creates a run row with the immutable confirmed target snapshot atomically.
func (s *Store) CreateRunAtomic(ctx context.Context, runID string, tag string, t target.ConfirmedTarget) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	verifiedAt := t.TargetVerifiedAt.Format(time.RFC3339)

	query := `
	INSERT INTO runs (
		run_id, target_input, target_user_id, target_username,
		target_username_alias_note, target_display_name, target_avatar_url,
		target_resolution_source, target_verified_at, target_confirmed,
		tag, created_at, status
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, 'running');
	`

	_, err := s.db.ExecContext(ctx, query,
		runID, t.TargetInput, t.TargetUserID, t.TargetUsername,
		t.TargetUsernameAliasNote, t.TargetDisplayName, t.TargetAvatarURL,
		t.TargetResolutionSource, verifiedAt, tag, now,
	)
	if err != nil {
		return fmt.Errorf("failed to insert run record: %w", err)
	}
	return nil
}

// GetRun retrieves a run record by its runID.
func (s *Store) GetRun(ctx context.Context, runID string) (*RunRecord, error) {
	query := `
	SELECT run_id, target_input, target_user_id, target_username,
	       target_username_alias_note, target_display_name, target_avatar_url,
	       target_resolution_source, target_verified_at, target_confirmed,
	       tag, created_at, status
	FROM runs WHERE run_id = ?;
	`
	row := s.db.QueryRowContext(ctx, query, runID)
	var r RunRecord
	var verifiedAtStr, createdAtStr string
	var confirmedInt int

	err := row.Scan(
		&r.RunID, &r.TargetInput, &r.TargetUserID, &r.TargetUsername,
		&r.TargetUsernameAliasNote, &r.TargetDisplayName, &r.TargetAvatarURL,
		&r.TargetResolutionSource, &verifiedAtStr, &confirmedInt,
		&r.Tag, &createdAtStr, &r.Status,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRunNotFound
		}
		return nil, err
	}

	r.TargetConfirmed = confirmedInt == 1
	r.TargetVerifiedAt, _ = time.Parse(time.RFC3339, verifiedAtStr)
	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAtStr)
	return &r, nil
}

// UpdateRunStatus updates the overall execution status of a run (running | complete | error | aborted).
func (s *Store) UpdateRunStatus(ctx context.Context, runID, status string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE runs SET status = ? WHERE run_id = ?;`, status, runID)
	if err != nil {
		return fmt.Errorf("failed to update run status: %w", err)
	}
	return nil
}

// ListRuns returns all runs with aggregated scan statistics ordered from newest to oldest.
func (s *Store) ListRuns(ctx context.Context) ([]RunSummary, error) {
	query := `
	SELECT 
		r.run_id, r.target_username, r.target_user_id, r.target_display_name,
		r.tag, r.status, r.created_at,
		COUNT(g.id) AS total_servers,
		COALESCE(SUM(CASE WHEN g.scan_status IN ('complete', 'blocked') THEN 1 ELSE 0 END), 0) AS completed_servers,
		COALESCE(SUM(CASE WHEN g.scan_status IN ('pending', 'running', 'rate_limited', 'error') THEN 1 ELSE 0 END), 0) AS pending_servers,
		COALESCE(SUM(CASE WHEN g.best_match_status = 'confirmed' THEN 1 ELSE 0 END), 0) AS confirmed_matches,
		COALESCE(SUM(CASE WHEN g.best_match_status = 'candidate' THEN 1 ELSE 0 END), 0) AS candidate_matches
	FROM runs r
	LEFT JOIN guild_scans g ON r.run_id = g.run_id
	GROUP BY r.run_id
	ORDER BY r.created_at DESC, r.rowid DESC;
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query runs: %w", err)
	}
	defer rows.Close()

	var summaries []RunSummary
	for rows.Next() {
		var sm RunSummary
		var createdAtStr string
		err := rows.Scan(
			&sm.RunID, &sm.TargetUsername, &sm.TargetUserID, &sm.TargetDisplayName,
			&sm.Tag, &sm.Status, &createdAtStr,
			&sm.TotalServers, &sm.CompletedServers, &sm.PendingServers,
			&sm.ConfirmedMatches, &sm.CandidateMatches,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan run summary: %w", err)
		}
		sm.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAtStr)
		summaries = append(summaries, sm)
	}
	return summaries, nil
}

// GetLatestIncompleteRun retrieves the most recent run that has pending or non-terminal scans.
func (s *Store) GetLatestIncompleteRun(ctx context.Context) (*RunSummary, error) {
	runs, err := s.ListRuns(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range runs {
		if r.Status != "complete" || r.PendingServers > 0 {
			return &r, nil
		}
	}
	return nil, nil
}

// SaveDiscoveredServers persists a batch of servers under a category tag idempotently.
func (s *Store) SaveDiscoveredServers(ctx context.Context, servers []ServerRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
	INSERT INTO servers (tag, invite_code, guild_id, guild_name, approximate_member_count, discovered_at)
	VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT (tag, invite_code) DO UPDATE SET
	    guild_id = excluded.guild_id,
	    guild_name = excluded.guild_name,
	    approximate_member_count = excluded.approximate_member_count;
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, srv := range servers {
		now := srv.DiscoveredAt
		if now.IsZero() {
			now = time.Now().UTC()
		}
		_, err := stmt.ExecContext(ctx, srv.Tag, srv.InviteCode, srv.GuildID, srv.GuildName, srv.ApproximateMemberCount, now.Format(time.RFC3339))
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// RecordGuildScanTransaction writes a guild scan summary, observations, and messages atomically.
func (s *Store) RecordGuildScanTransaction(
	ctx context.Context,
	scan GuildScanRecord,
	observations []matcher.MatchObservation,
	messages []MessageRecord,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Insert or update guild_scans summary with promotion-only logic for match status
	upsertScanQuery := `
	INSERT INTO guild_scans (
		run_id, guild_id, guild_name, invite_code, scan_status,
		best_match_status, best_match_reason, best_observed_user_id, best_observed_username,
		member_coverage, members_examined, member_stop_reason,
		message_coverage, messages_examined, oldest_checked_at, newest_checked_at,
		channels_discovered, channels_scanned, channels_unreadable, message_stop_reason,
		onboarding_status, gate_type, gate_status, started_at, completed_at
	) VALUES (
		?, ?, ?, ?, ?,
		?, ?, ?, ?,
		?, ?, ?,
		?, ?, ?, ?,
		?, ?, ?, ?,
		?, ?, ?, ?, ?
	)
	ON CONFLICT(run_id, guild_id) DO UPDATE SET
		scan_status = excluded.scan_status,
		best_match_status = CASE
			WHEN excluded.best_match_status = 'confirmed' THEN 'confirmed'
			WHEN guild_scans.best_match_status = 'confirmed' THEN 'confirmed'
			WHEN excluded.best_match_status = 'candidate' THEN 'candidate'
			ELSE guild_scans.best_match_status
		END,
		best_match_reason = CASE
			WHEN excluded.best_match_status = 'confirmed' THEN excluded.best_match_reason
			WHEN guild_scans.best_match_status = 'confirmed' THEN guild_scans.best_match_reason
			WHEN excluded.best_match_status = 'candidate' THEN excluded.best_match_reason
			ELSE guild_scans.best_match_reason
		END,
		best_observed_user_id = CASE
			WHEN excluded.best_match_status = 'confirmed' THEN excluded.best_observed_user_id
			WHEN guild_scans.best_match_status = 'confirmed' THEN guild_scans.best_observed_user_id
			ELSE excluded.best_observed_user_id
		END,
		best_observed_username = CASE
			WHEN excluded.best_match_status = 'confirmed' THEN excluded.best_observed_username
			WHEN guild_scans.best_match_status = 'confirmed' THEN guild_scans.best_observed_username
			ELSE excluded.best_observed_username
		END,
		member_coverage = excluded.member_coverage,
		members_examined = excluded.members_examined,
		member_stop_reason = excluded.member_stop_reason,
		message_coverage = excluded.message_coverage,
		messages_examined = excluded.messages_examined,
		oldest_checked_at = excluded.oldest_checked_at,
		newest_checked_at = excluded.newest_checked_at,
		channels_discovered = excluded.channels_discovered,
		channels_scanned = excluded.channels_scanned,
		channels_unreadable = excluded.channels_unreadable,
		message_stop_reason = excluded.message_stop_reason,
		onboarding_status = excluded.onboarding_status,
		gate_type = excluded.gate_type,
		gate_status = excluded.gate_status,
		completed_at = excluded.completed_at;
	`

	startedStr := scan.StartedAt.Format(time.RFC3339)
	completedStr := ""
	if !scan.CompletedAt.IsZero() {
		completedStr = scan.CompletedAt.Format(time.RFC3339)
	}

	_, err = tx.ExecContext(ctx, upsertScanQuery,
		scan.RunID, scan.GuildID, scan.GuildName, scan.InviteCode, scan.ScanStatus,
		scan.BestMatchStatus, scan.BestMatchReason, scan.BestObservedUserID, scan.BestObservedUsername,
		scan.MemberCoverage, scan.MembersExamined, scan.MemberStopReason,
		scan.MessageCoverage, scan.MessagesExamined, scan.OldestCheckedAt, scan.NewestCheckedAt,
		scan.ChannelsDiscovered, scan.ChannelsScanned, scan.ChannelsUnreadable, scan.MessageStopReason,
		scan.OnboardingStatus, scan.GateType, scan.GateStatus, startedStr, completedStr,
	)
	if err != nil {
		return fmt.Errorf("failed to record guild scan: %w", err)
	}

	// 2. Insert Observations (ON CONFLICT DO NOTHING - preserves all distinct identities)
	obsQuery := `
	INSERT INTO match_observations (
		run_id, guild_id, observed_user_id, observed_username_norm, observed_username,
		observed_nick, observed_joined_at, observed_roles, observed_premium_since,
		observed_guild_avatar_url, match_status, match_reason, acquisition_method, observed_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT (run_id, guild_id, observed_user_id, observed_username_norm, observed_nick) DO NOTHING;
	`
	obsStmt, err := tx.PrepareContext(ctx, obsQuery)
	if err != nil {
		return err
	}
	defer obsStmt.Close()

	for _, obs := range observations {
		rolesJSON, _ := json.Marshal(obs.ObservedRoles)
		obsTime := obs.ObservedAt
		if obsTime.IsZero() {
			obsTime = time.Now().UTC()
		}

		_, err := obsStmt.ExecContext(ctx,
			scan.RunID, scan.GuildID, obs.ObservedUserID, obs.ObservedUsernameNorm, obs.ObservedUsername,
			obs.ObservedNick, obs.ObservedJoinedAt, string(rolesJSON), obs.ObservedPremiumSince,
			obs.ObservedGuildAvatarURL, string(obs.MatchStatus), string(obs.MatchReason),
			obs.AcquisitionMethod, obsTime.Format(time.RFC3339),
		)
		if err != nil {
			return fmt.Errorf("failed to insert match observation: %w", err)
		}
	}

	// 3. Insert Messages (ON CONFLICT DO NOTHING - idempotency)
	msgQuery := `
	INSERT INTO messages (
		run_id, guild_id, channel_id, message_id, author_id, author_username,
		content, timestamp, collected_at, collector_version, acquisition_method, coverage_status
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT (run_id, guild_id, channel_id, message_id) DO NOTHING;
	`
	msgStmt, err := tx.PrepareContext(ctx, msgQuery)
	if err != nil {
		return err
	}
	defer msgStmt.Close()

	for _, msg := range messages {
		_, err := msgStmt.ExecContext(ctx,
			msg.RunID, msg.GuildID, msg.ChannelID, msg.MessageID, msg.AuthorID, msg.AuthorUsername,
			msg.Content, msg.Timestamp.Format(time.RFC3339), msg.CollectedAt.Format(time.RFC3339),
			msg.CollectorVersion, msg.AcquisitionMethod, msg.CoverageStatus,
		)
		if err != nil {
			return fmt.Errorf("failed to insert message record: %w", err)
		}
	}

	return tx.Commit()
}

// GetResumableGuildScans returns all guild scans for a run that are in non-terminal states.
func (s *Store) GetResumableGuildScans(ctx context.Context, runID string) ([]GuildScanRecord, error) {
	query := `
	SELECT run_id, guild_id, guild_name, invite_code, scan_status,
	       best_match_status, best_match_reason, best_observed_user_id, best_observed_username,
	       member_coverage, members_examined, member_stop_reason,
	       message_coverage, messages_examined, oldest_checked_at, newest_checked_at,
	       channels_discovered, channels_scanned, channels_unreadable, message_stop_reason,
	       onboarding_status, gate_type, gate_status, started_at, completed_at
	FROM guild_scans
	WHERE run_id = ? AND scan_status IN ('pending', 'running', 'rate_limited', 'error');
	`
	rows, err := s.db.QueryContext(ctx, query, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []GuildScanRecord
	for rows.Next() {
		var g GuildScanRecord
		var startedStr, completedStr string
		err := rows.Scan(
			&g.RunID, &g.GuildID, &g.GuildName, &g.InviteCode, &g.ScanStatus,
			&g.BestMatchStatus, &g.BestMatchReason, &g.BestObservedUserID, &g.BestObservedUsername,
			&g.MemberCoverage, &g.MembersExamined, &g.MemberStopReason,
			&g.MessageCoverage, &g.MessagesExamined, &g.OldestCheckedAt, &g.NewestCheckedAt,
			&g.ChannelsDiscovered, &g.ChannelsScanned, &g.ChannelsUnreadable, &g.MessageStopReason,
			&g.OnboardingStatus, &g.GateType, &g.GateStatus, &startedStr, &completedStr,
		)
		if err != nil {
			return nil, err
		}
		g.StartedAt, _ = time.Parse(time.RFC3339, startedStr)
		if completedStr != "" {
			g.CompletedAt, _ = time.Parse(time.RFC3339, completedStr)
		}
		records = append(records, g)
	}
	return records, nil
}

// GetRunGuildScans returns all guild scans for export.
func (s *Store) GetRunGuildScans(ctx context.Context, runID string) ([]GuildScanRecord, error) {
	query := `
	SELECT run_id, guild_id, guild_name, invite_code, scan_status,
	       best_match_status, best_match_reason, best_observed_user_id, best_observed_username,
	       member_coverage, members_examined, member_stop_reason,
	       message_coverage, messages_examined, oldest_checked_at, newest_checked_at,
	       channels_discovered, channels_scanned, channels_unreadable, message_stop_reason,
	       onboarding_status, gate_type, gate_status, started_at, completed_at
	FROM guild_scans
	WHERE run_id = ?
	ORDER BY id ASC;
	`
	rows, err := s.db.QueryContext(ctx, query, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []GuildScanRecord
	for rows.Next() {
		var g GuildScanRecord
		var startedStr, completedStr string
		err := rows.Scan(
			&g.RunID, &g.GuildID, &g.GuildName, &g.InviteCode, &g.ScanStatus,
			&g.BestMatchStatus, &g.BestMatchReason, &g.BestObservedUserID, &g.BestObservedUsername,
			&g.MemberCoverage, &g.MembersExamined, &g.MemberStopReason,
			&g.MessageCoverage, &g.MessagesExamined, &g.OldestCheckedAt, &g.NewestCheckedAt,
			&g.ChannelsDiscovered, &g.ChannelsScanned, &g.ChannelsUnreadable, &g.MessageStopReason,
			&g.OnboardingStatus, &g.GateType, &g.GateStatus, &startedStr, &completedStr,
		)
		if err != nil {
			return nil, err
		}
		g.StartedAt, _ = time.Parse(time.RFC3339, startedStr)
		if completedStr != "" {
			g.CompletedAt, _ = time.Parse(time.RFC3339, completedStr)
		}
		records = append(records, g)
	}
	return records, nil
}

// GetRunObservations returns all observations for a run.
func (s *Store) GetRunObservations(ctx context.Context, runID string) ([]ObservationRecord, error) {
	query := `
	SELECT guild_id, observed_user_id, observed_username_norm, observed_username,
	       observed_nick, observed_joined_at, observed_roles, observed_premium_since,
	       observed_guild_avatar_url, match_status, match_reason, acquisition_method, observed_at
	FROM match_observations
	WHERE run_id = ?
	ORDER BY id ASC;
	`
	rows, err := s.db.QueryContext(ctx, query, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var obs []ObservationRecord
	for rows.Next() {
		var o ObservationRecord
		var rolesJSON, observedAtStr, matchStatusStr, matchReasonStr string
		err := rows.Scan(
			&o.GuildID, &o.ObservedUserID, &o.ObservedUsernameNorm, &o.ObservedUsername,
			&o.ObservedNick, &o.ObservedJoinedAt, &rolesJSON, &o.ObservedPremiumSince,
			&o.ObservedGuildAvatarURL, &matchStatusStr, &matchReasonStr, &o.AcquisitionMethod, &observedAtStr,
		)
		if err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(rolesJSON), &o.ObservedRoles)
		o.MatchStatus = matcher.MatchKind(matchStatusStr)
		o.MatchReason = matcher.MatchReason(matchReasonStr)
		o.ObservedAt, _ = time.Parse(time.RFC3339, observedAtStr)
		obs = append(obs, o)
	}
	return obs, nil
}

// GetRunMessages returns all collected messages for a run.
func (s *Store) GetRunMessages(ctx context.Context, runID string) ([]MessageRecord, error) {
	query := `
	SELECT run_id, guild_id, channel_id, message_id, author_id, author_username,
	       content, timestamp, collected_at, collector_version, acquisition_method, coverage_status
	FROM messages
	WHERE run_id = ?
	ORDER BY id ASC;
	`
	rows, err := s.db.QueryContext(ctx, query, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []MessageRecord
	for rows.Next() {
		var m MessageRecord
		var tsStr, colStr string
		err := rows.Scan(
			&m.RunID, &m.GuildID, &m.ChannelID, &m.MessageID, &m.AuthorID, &m.AuthorUsername,
			&m.Content, &tsStr, &colStr, &m.CollectorVersion, &m.AcquisitionMethod, &m.CoverageStatus,
		)
		if err != nil {
			return nil, err
		}
		m.Timestamp, _ = time.Parse(time.RFC3339, tsStr)
		m.CollectedAt, _ = time.Parse(time.RFC3339, colStr)
		msgs = append(msgs, m)
	}
	return msgs, nil
}
