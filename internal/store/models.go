package store

import (
	"time"

	"discord-osint/internal/matcher"
)

// RunRecord represents a persistent run with its immutable target snapshot.
type RunRecord struct {
	RunID                   string    `json:"run_id"`
	TargetInput             string    `json:"target_input"`
	TargetUserID            string    `json:"target_user_id"`
	TargetUsername          string    `json:"target_username"`
	TargetUsernameAliasNote string    `json:"target_username_alias_note"`
	TargetDisplayName       string    `json:"target_display_name"`
	TargetAvatarURL         string    `json:"target_avatar_url"`
	TargetResolutionSource  string    `json:"target_resolution_source"`
	TargetVerifiedAt        time.Time `json:"target_verified_at"`
	TargetConfirmed         bool      `json:"target_confirmed"`
	Tag                     string    `json:"tag"`
	CreatedAt               time.Time `json:"created_at"`
	Status                  string    `json:"status"` // running | complete | error | aborted
}

// RunSummary aggregates progress, matches, and metadata for a run.
type RunSummary struct {
	RunID             string    `json:"run_id"`
	TargetUsername    string    `json:"target_username"`
	TargetUserID      string    `json:"target_user_id"`
	TargetDisplayName string    `json:"target_display_name"`
	Tag               string    `json:"tag"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	TotalServers      int       `json:"total_servers"`
	CompletedServers  int       `json:"completed_servers"`
	PendingServers    int       `json:"pending_servers"`
	ConfirmedMatches  int       `json:"confirmed_matches"`
	CandidateMatches  int       `json:"candidate_matches"`
}

// ServerRecord represents a discovered server/invite.
type ServerRecord struct {
	Tag                    string    `json:"tag"`
	InviteCode             string    `json:"invite_code"`
	GuildID                string    `json:"guild_id"`
	GuildName              string    `json:"guild_name"`
	ApproximateMemberCount int       `json:"approximate_member_count"`
	DiscoveredAt           time.Time `json:"discovered_at"`
}

// GuildScanRecord represents the scan and match state of a guild.
type GuildScanRecord struct {
	RunID               string    `json:"run_id"`
	GuildID             string    `json:"guild_id"`
	GuildName           string    `json:"guild_name"`
	InviteCode          string    `json:"invite_code"`
	ScanStatus          string    `json:"scan_status"` // pending | running | complete | blocked | permission_limited | rate_limited | error
	BestMatchStatus     string    `json:"best_match_status"` // unknown | not_found | candidate | confirmed
	BestMatchReason     string    `json:"best_match_reason"`
	BestObservedUserID  string    `json:"best_observed_user_id"`
	BestObservedUsername string   `json:"best_observed_username"`
	MemberCoverage      string    `json:"member_coverage"` // complete | bounded | partial
	MembersExamined     int       `json:"members_examined"`
	MemberStopReason    string    `json:"member_stop_reason"`
	MessageCoverage     string    `json:"message_coverage"` // complete | bounded | partial
	MessagesExamined    int       `json:"messages_examined"`
	OldestCheckedAt     string    `json:"oldest_checked_at"`
	NewestCheckedAt     string    `json:"newest_checked_at"`
	ChannelsDiscovered  int       `json:"channels_discovered"`
	ChannelsScanned     int       `json:"channels_scanned"`
	ChannelsUnreadable  int       `json:"channels_unreadable"`
	MessageStopReason   string    `json:"message_stop_reason"`
	OnboardingStatus    string    `json:"onboarding_status"`
	GateType            string    `json:"gate_type"`
	GateStatus          string    `json:"gate_status"`
	StartedAt           time.Time `json:"started_at"`
	CompletedAt         time.Time `json:"completed_at"`
}

// MessageRecord stores evidence messages collected during scanning.
type MessageRecord struct {
	RunID             string    `json:"run_id"`
	GuildID           string    `json:"guild_id"`
	ChannelID         string    `json:"channel_id"`
	MessageID         string    `json:"message_id"`
	AuthorID          string    `json:"author_id"`
	AuthorUsername    string    `json:"author_username"`
	Content           string    `json:"content"`
	Timestamp         time.Time `json:"timestamp"`
	CollectedAt       time.Time `json:"collected_at"`
	CollectorVersion  string    `json:"collector_version"`
	AcquisitionMethod string    `json:"acquisition_method"`
	CoverageStatus    string    `json:"coverage_status"`
}

// GateAttemptRecord tracks onboarding / channel gate attempts for resume idempotency.
type GateAttemptRecord struct {
	RunID            string    `json:"run_id"`
	GuildID          string    `json:"guild_id"`
	GateType         string    `json:"gate_type"`
	ChannelID        string    `json:"channel_id"`
	MessageID        string    `json:"message_id"`
	ActionTaken      string    `json:"action_taken"`
	OperatorDecision string    `json:"operator_decision"`
	AttemptedAt      time.Time `json:"attempted_at"`
}

// ObservationRecord aliases matcher.MatchObservation for store operations.
type ObservationRecord = matcher.MatchObservation
