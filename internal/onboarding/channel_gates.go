package onboarding

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"discord-osint/internal/discord"
)

var knownVerificationDomains = []string{
	"altdentifier.com",
	"captcha.site",
	"wickbot.com",
	"doublecounter.net",
	"restorecord.com",
	"vulcan.bot",
}

// DetectChannelGates inspects channel messages for reaction, button, link, and NSFW gates.
func DetectChannelGates(channelID, channelName string, messages []discord.DiscordMessage) []GateClassification {
	var gates []GateClassification

	isVerificationChannel := false
	normChanName := strings.ToLower(channelName)
	for _, kw := range []string{"verify", "verification", "rules", "welcome", "start-here", "nsfw-access", "gate"} {
		if strings.Contains(normChanName, kw) {
			isVerificationChannel = true
			break
		}
	}

	for _, msg := range messages {
		contentLower := strings.ToLower(msg.Content)

		// 1. Detect Button Components
		for _, row := range msg.Components {
			for _, comp := range row.Components {
				if comp.Type == 2 && comp.CustomID != "" { // Button
					labelLower := strings.ToLower(comp.Label)
					isVerifyButton := strings.Contains(labelLower, "verify") ||
						strings.Contains(labelLower, "complete") ||
						strings.Contains(labelLower, "agree") ||
						strings.Contains(labelLower, "rules") ||
						strings.Contains(labelLower, "enter") ||
						isVerificationChannel

					if isVerifyButton {
						appID := msg.Author.ID
						gates = append(gates, GateClassification{
							GateType:      GateButton,
							ChannelID:     channelID,
							ChannelName:   channelName,
							MessageID:     msg.ID,
							ApplicationID: appID,
							CustomID:      comp.CustomID,
							Explanation:   fmt.Sprintf("Interactive button '%s' authored by %s", comp.Label, msg.Author.Username),
						})
					}
				}
			}
		}

		// 2. Detect External Verification Links
		for _, domain := range knownVerificationDomains {
			if strings.Contains(contentLower, domain) {
				gates = append(gates, GateClassification{
					GateType:    GateLinkExternal,
					ChannelID:   channelID,
					ChannelName: channelName,
					MessageID:   msg.ID,
					TargetURL:   domain,
					Explanation: fmt.Sprintf("External bot verification via %s", domain),
				})
				break
			}
		}

		// 3. Detect Reaction Checkmarks
		hasEmojiCheck := false
		for _, r := range msg.Reactions {
			if r.Emoji.Name == "✅" || r.Emoji.Name == "☑️" || r.Emoji.Name == "👍" {
				hasEmojiCheck = true
				break
			}
		}

		saysReact := strings.Contains(contentLower, "react") ||
			strings.Contains(contentLower, "reaction") ||
			strings.Contains(contentLower, "check mark") ||
			strings.Contains(contentLower, "verify")

		if hasEmojiCheck || (saysReact && isVerificationChannel) {
			gates = append(gates, GateClassification{
				GateType:    GateReaction,
				ChannelID:   channelID,
				ChannelName: channelName,
				MessageID:   msg.ID,
				Emoji:       "✅",
				Explanation: "Channel reaction checkmark verification (react ✅)",
			})
		}

		// 4. Detect NSFW Access Gate
		if strings.Contains(normChanName, "nsfw") || strings.Contains(contentLower, "18+") || strings.Contains(contentLower, "nsfw access") {
			gates = append(gates, GateClassification{
				GateType:    GateNSFWConsent,
				ChannelID:   channelID,
				ChannelName: channelName,
				MessageID:   msg.ID,
				Explanation: "NSFW Age Gate consent required",
			})
		}
	}

	return gates
}

// ExecuteGate handles operator approval and execution of channel verification gates.
func ExecuteGate(
	ctx context.Context,
	gate GateClassification,
	client *discord.Client,
	guildID string,
	sessionID string,
	reader io.Reader,
	writer io.Writer,
) (GateStatus, error) {
	if reader == nil {
		reader = os.Stdin
	}
	if writer == nil {
		writer = os.Stdout
	}

	scanner := bufio.NewScanner(reader)

	switch gate.GateType {
	case GateReaction:
		fmt.Fprintln(writer, "")
		fmt.Fprintf(writer, "[!] Channel Gate Detected in #%s: %s\n", gate.ChannelName, gate.Explanation)
		fmt.Fprintf(writer, "React with %s to message %s? [y/N]: ", gate.Emoji, gate.MessageID)
		if !scanner.Scan() {
			return GateStatusSkippedOperator, nil
		}
		ans := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if ans != "y" && ans != "yes" {
			return GateStatusSkippedOperator, nil
		}

		if err := client.PostReaction(ctx, gate.ChannelID, gate.MessageID, gate.Emoji); err != nil {
			return GateStatusFailed, fmt.Errorf("failed to post verification reaction: %w", err)
		}
		fmt.Fprintln(writer, "[+] Verification reaction posted successfully.")
		return GateStatusPassed, nil

	case GateButton:
		fmt.Fprintln(writer, "")
		fmt.Fprintf(writer, "[!] Channel Verification Button in #%s: %s\n", gate.ChannelName, gate.Explanation)
		fmt.Fprintf(writer, "Execute interaction for button (CustomID: %s)? [y/N]: ", gate.CustomID)
		if !scanner.Scan() {
			return GateStatusSkippedOperator, nil
		}
		ans := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if ans != "y" && ans != "yes" {
			return GateStatusSkippedOperator, nil
		}

		payload := map[string]any{
			"type":           3, // Message Component
			"guild_id":       guildID,
			"channel_id":     gate.ChannelID,
			"message_id":     gate.MessageID,
			"application_id": gate.ApplicationID,
			"session_id":     sessionID,
			"data": map[string]any{
				"component_type": 2, // Button
				"custom_id":      gate.CustomID,
			},
		}

		if err := client.PostInteraction(ctx, payload); err != nil {
			return GateStatusFailed, fmt.Errorf("failed to execute button interaction: %w", err)
		}
		fmt.Fprintln(writer, "[+] Button interaction executed successfully.")
		return GateStatusPassed, nil

	case GateLinkExternal:
		fmt.Fprintln(writer, "")
		fmt.Fprintf(writer, "[!] External Bot Verification in #%s: %s\n", gate.ChannelName, gate.Explanation)
		fmt.Fprintf(writer, "Open link in burner browser to verify: %s\n", gate.TargetURL)
		fmt.Fprintf(writer, "Verification completed in browser? [y/N]: ")
		if !scanner.Scan() {
			return GateStatusSkippedOperator, nil
		}
		ans := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if ans != "y" && ans != "yes" {
			return GateStatusSkippedOperator, nil
		}
		return GateStatusPassed, nil

	case GateNSFWConsent:
		fmt.Fprintln(writer, "")
		fmt.Fprintf(writer, "[!] NSFW Access Gate in #%s: %s\n", gate.ChannelName, gate.Explanation)
		fmt.Fprintf(writer, "Agree to 18+ NSFW disclaimer? [y/N]: ")
		if !scanner.Scan() {
			return GateStatusSkippedOperator, nil
		}
		ans := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if ans != "y" && ans != "yes" {
			return GateStatusSkippedOperator, nil
		}
		return GateStatusPassed, nil

	default:
		return GateStatusSkippedUnsupported, nil
	}
}
