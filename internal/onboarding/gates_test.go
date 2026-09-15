package onboarding

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"discord-osint/internal/discord"
)

func TestDetectChannelGates_Buttons(t *testing.T) {
	msgs := []discord.DiscordMessage{
		{
			ID:        "msg_btn_1",
			ChannelID: "chan_verify",
			Content:   "Please click the button below to gain access to the server.",
			Author: discord.User{
				ID:       "bot_app_123",
				Username: "VerificationBot",
			},
			Components: []discord.MessageComponent{
				{
					Type: 1, // Action row
					Components: []discord.MessageComponent{
						{
							Type:     2, // Button
							Label:    "Verify Here",
							CustomID: "verify_btn_custom_id_abc",
						},
					},
				},
			},
		},
	}

	gates := DetectChannelGates("chan_verify", "verify", msgs)
	if len(gates) == 0 {
		t.Fatalf("expected button gate detected, got 0")
	}

	btnGate := gates[0]
	if btnGate.GateType != GateButton {
		t.Errorf("expected GateButton, got %s", btnGate.GateType)
	}
	if btnGate.CustomID != "verify_btn_custom_id_abc" {
		t.Errorf("expected custom_id, got %s", btnGate.CustomID)
	}
	if btnGate.ApplicationID != "bot_app_123" {
		t.Errorf("expected application_id bot_app_123, got %s", btnGate.ApplicationID)
	}
}

func TestDetectChannelGates_Reactions(t *testing.T) {
	msgs := []discord.DiscordMessage{
		{
			ID:        "msg_react_1",
			ChannelID: "chan_rules",
			Content:   "React with ✅ to accept rules and gain access.",
			Author:    discord.User{ID: "mod_1", Username: "Moderator"},
		},
	}

	gates := DetectChannelGates("chan_rules", "rules-and-verify", msgs)
	if len(gates) == 0 {
		t.Fatalf("expected reaction gate detected, got 0")
	}

	reactGate := gates[0]
	if reactGate.GateType != GateReaction {
		t.Errorf("expected GateReaction, got %s", reactGate.GateType)
	}
	if reactGate.Emoji != "✅" {
		t.Errorf("expected emoji ✅, got %s", reactGate.Emoji)
	}
}

func TestDetectChannelGates_ExternalLinks(t *testing.T) {
	msgs := []discord.DiscordMessage{
		{
			ID:        "msg_link_1",
			ChannelID: "chan_welcome",
			Content:   "To prevent alts, verify your account via https://altdentifier.com/verify?id=123",
			Author:    discord.User{ID: "alt_bot", Username: "AltDentifier"},
		},
	}

	gates := DetectChannelGates("chan_welcome", "welcome", msgs)
	if len(gates) == 0 {
		t.Fatalf("expected external link gate, got 0")
	}

	linkGate := gates[0]
	if linkGate.GateType != GateLinkExternal {
		t.Errorf("expected GateLinkExternal, got %s", linkGate.GateType)
	}
	if !strings.Contains(linkGate.TargetURL, "altdentifier.com") {
		t.Errorf("expected target URL altdentifier.com, got %s", linkGate.TargetURL)
	}
}

func TestHandleRulesScreening_RulesOnly(t *testing.T) {
	form := &MemberVerificationForm{
		Version: "2024-01-01",
		Enabled: true,
		FormFields: []FormField{
			{
				FieldType: "TERMS",
				Label:     "Server Rules Agreement",
				Required:  true,
				Values:    []string{"1. Be polite", "2. No raiding"},
			},
		},
	}

	submitted := false
	submitFn := func(ctx context.Context, payload map[string]any) error {
		submitted = true
		if payload["version"] != "2024-01-01" {
			t.Errorf("expected version 2024-01-01, got %v", payload["version"])
		}
		return nil
	}

	var out bytes.Buffer
	status, err := HandleRulesScreening(context.Background(), form, "rules-only", nil, &out, submitFn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != GateStatusPassed {
		t.Errorf("expected GateStatusPassed, got %s", status)
	}
	if !submitted {
		t.Errorf("expected submitFn to be called in rules-only mode")
	}
}

func TestHandleRulesScreening_ManualPrompt(t *testing.T) {
	form := &MemberVerificationForm{
		Version: "2024-02-01",
		Enabled: true,
		FormFields: []FormField{
			{FieldType: "TERMS", Label: "Community Guidelines", Required: true},
		},
	}

	// 1. Operator accepts with 'y'
	submitted := false
	submitFn := func(ctx context.Context, payload map[string]any) error {
		submitted = true
		return nil
	}
	inputYes := bytes.NewBufferString("y\n")
	var out bytes.Buffer

	status, err := HandleRulesScreening(context.Background(), form, "manual", inputYes, &out, submitFn)
	if err != nil || status != GateStatusPassed || !submitted {
		t.Errorf("manual accept failed: status=%s, submitted=%v, err=%v", status, submitted, err)
	}

	// 2. Operator rejects with 'n'
	submitted = false
	inputNo := bytes.NewBufferString("n\n")
	statusNo, err := HandleRulesScreening(context.Background(), form, "manual", inputNo, &out, submitFn)
	if err != nil || statusNo != GateStatusSkippedOperator || submitted {
		t.Errorf("manual reject failed: status=%s, submitted=%v, err=%v", statusNo, submitted, err)
	}
}

func TestExecuteGate_ExternalLink_Confirm(t *testing.T) {
	gate := GateClassification{
		GateType:    GateLinkExternal,
		ChannelID:   "chan_verify",
		ChannelName: "verify",
		TargetURL:   "https://altdentifier.com/verify?id=test",
		Explanation: "External bot verification via altdentifier.com",
	}

	input := bytes.NewBufferString("y\n")
	var out bytes.Buffer

	status, err := ExecuteGate(context.Background(), gate, nil, "guild_123", "session_123", input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != GateStatusPassed {
		t.Errorf("expected GateStatusPassed, got %s", status)
	}
}

func TestExecuteGate_ExternalLink_Deny(t *testing.T) {
	gate := GateClassification{
		GateType:    GateLinkExternal,
		ChannelID:   "chan_verify",
		ChannelName: "verify",
		TargetURL:   "https://wickbot.com/verify",
		Explanation: "External bot verification via wickbot.com",
	}

	input := bytes.NewBufferString("n\n")
	var out bytes.Buffer

	status, err := ExecuteGate(context.Background(), gate, nil, "guild_123", "session_123", input, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != GateStatusSkippedOperator {
		t.Errorf("expected GateStatusSkippedOperator, got %s", status)
	}
}
