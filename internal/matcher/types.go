package matcher

import (
	"time"
)

// MatchKind classifies the certainty of an identity match.
type MatchKind string

const (
	MatchNone      MatchKind = "none"
	MatchCandidate MatchKind = "candidate"
	MatchConfirmed MatchKind = "confirmed"
)

// MatchReason indicates the specific rule that produced the match.
type MatchReason string

const (
	ReasonNone                   MatchReason = "none"
	ReasonUserIDExact            MatchReason = "user_id_exact"
	ReasonUsernameExact          MatchReason = "username_exact"
	ReasonUsernameCaseFold       MatchReason = "username_case_fold"
	ReasonUsernameUnicodeFold    MatchReason = "username_unicode_fold"
	ReasonUsernameMatchIDMismatch MatchReason = "username_match_id_mismatch"
	ReasonAliasUsernameMatch     MatchReason = "alias_username_match"
	ReasonNicknameSimilar        MatchReason = "nickname_similar"
)

// GuildMember represents a member observed in a Discord guild.
type GuildMember struct {
	GuildID        string    `json:"guild_id"`
	UserID         string    `json:"user_id"`
	Username       string    `json:"username"`
	DisplayName    string    `json:"display_name"`
	Nick           string    `json:"nick"`
	JoinedAt       time.Time `json:"joined_at"`
	Roles          []string  `json:"roles"`
	PremiumSince   time.Time `json:"premium_since"`
	GuildAvatarURL string    `json:"guild_avatar_url"`
}

// MatchObservation records an observed target candidate or confirmation in a guild.
type MatchObservation struct {
	GuildID                string      `json:"guild_id"`
	ObservedUserID         string      `json:"observed_user_id"`
	ObservedUsernameNorm   string      `json:"observed_username_norm"`
	ObservedUsername       string      `json:"observed_username"`
	ObservedNick           string      `json:"observed_nick"`
	ObservedJoinedAt       string      `json:"observed_joined_at"`
	ObservedRoles          []string    `json:"observed_roles"`
	ObservedPremiumSince   string      `json:"observed_premium_since"`
	ObservedGuildAvatarURL string      `json:"observed_guild_avatar_url"`
	MatchStatus            MatchKind   `json:"match_status"`
	MatchReason            MatchReason `json:"match_reason"`
	ObservedAt             time.Time   `json:"observed_at"`
	AcquisitionMethod      string      `json:"acquisition_method"`
}
