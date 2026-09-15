package target

import (
	"context"
	"time"
)

// ResolutionStatus classifies the target resolution outcome.
type ResolutionStatus string

const (
	Resolved           ResolutionStatus = "resolved"
	MultipleCandidates ResolutionStatus = "multiple_candidates"
	Unresolved         ResolutionStatus = "unresolved"
)

// Candidate represents a potential target identity.
type Candidate struct {
	UserID      string
	Username    string
	DisplayName string
	AvatarURL   string
	Source      string
	CreatedAt   time.Time
}

// Resolution represents the output from a target resolver.
type Resolution struct {
	Input      string
	Status     ResolutionStatus
	Candidates []Candidate
}

// Resolver defines how a target is looked up prior to run creation.
type Resolver interface {
	ResolveUsername(ctx context.Context, username string) (Resolution, error)
	ResolveUserID(ctx context.Context, userID string) (Candidate, error)
}

// VerifyInput holds the CLI target flags.
type VerifyInput struct {
	TargetUserID   string
	TargetUsername string
	AutoConfirm    bool // --yes flag
}

// ConfirmedTarget represents an immutable, validated target snapshot.
type ConfirmedTarget struct {
	TargetInput              string    `json:"target_input"`
	TargetUserID             string    `json:"target_user_id"` // empty when username-only
	TargetUsername           string    `json:"target_username"`
	TargetUsernameAliasNote  string    `json:"target_username_alias_note,omitempty"` // populated on ID-username mismatch
	TargetDisplayName        string    `json:"target_display_name"`
	TargetAvatarURL          string    `json:"target_avatar_url"`
	TargetResolutionSource   string    `json:"target_resolution_source"`
	TargetVerifiedAt         time.Time `json:"target_verified_at"`
	TargetConfirmed          bool      `json:"target_confirmed"` // always true once confirmed
}

// PromptUI decouples interactive CLI terminal rendering from core logic.
type PromptUI interface {
	DisplayCandidate(c Candidate, warning string)
	DisplayUnresolved(username string, warning string)
	SelectCandidate(candidates []Candidate) (int, error) // returns index 0..N-1 or error/abort
	Confirm(prompt string) (bool, error)
}
