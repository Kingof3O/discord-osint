package onboarding

// GateType classifies the mechanism used to gate access to the server.
type GateType string

const (
	GateNone         GateType = "none"
	GateRules        GateType = "rules"
	GateReaction     GateType = "reaction"
	GateButton       GateType = "button"
	GateLinkExternal GateType = "link_external"
	GateNSFWConsent  GateType = "nsfw_consent"
	GateCode         GateType = "code"
	GateManualReview GateType = "manual_review"
)

// GateStatus records the result of handling a gate.
type GateStatus string

const (
	GateStatusPassed             GateStatus = "passed"
	GateStatusSkippedOperator    GateStatus = "skipped_operator"
	GateStatusSkippedRulesOnly   GateStatus = "skipped_rules_only"
	GateStatusSkippedUnsupported GateStatus = "skipped_unsupported"
	GateStatusFailed             GateStatus = "failed"
)

// OnboardingPrompt represents a question presented during guild onboarding.
type OnboardingPrompt struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Options      []string `json:"options"`
	Required     bool     `json:"required"`
	InOnboarding bool     `json:"in_onboarding"`
	Type         int      `json:"type"` // 0: multiple choice, 1: dropdown
}

// GateClassification represents detected gate parameters on a channel message.
type GateClassification struct {
	GateType      GateType `json:"gate_type"`
	ChannelID     string   `json:"channel_id"`
	ChannelName   string   `json:"channel_name"`
	MessageID     string   `json:"message_id"`
	ApplicationID string   `json:"application_id,omitempty"` // bot that authored the component
	Emoji         string   `json:"emoji,omitempty"`
	CustomID      string   `json:"custom_id,omitempty"`
	TargetURL     string   `json:"target_url,omitempty"`
	Explanation   string   `json:"explanation"`
}
