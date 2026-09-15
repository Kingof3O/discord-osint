package export

import (
	"fmt"
	"strings"
	"time"

	"discord-osint/internal/store"
	"discord-osint/internal/target"
)

// GenerateMarkdownReport compiles a human-readable investigation report in Markdown format.
func GenerateMarkdownReport(
	run *store.RunRecord,
	scans []store.GuildScanRecord,
	observations []store.ObservationRecord,
	messages []store.MessageRecord,
) string {
	var b strings.Builder

	b.WriteString("# Discord OSINT Investigation Report\n\n")
	b.WriteString(fmt.Sprintf("**Report Generated:** %s\n", time.Now().UTC().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("**Run ID:** `%s`\n\n", run.RunID))

	// 1. Target Profile Snapshot
	b.WriteString("## 1. Target Identity Snapshot\n\n")
	b.WriteString("| Attribute | Value |\n")
	b.WriteString("| :--- | :--- |\n")
	b.WriteString(fmt.Sprintf("| **Target Username** | `%s` |\n", run.TargetUsername))
	if run.TargetUserID != "" {
		b.WriteString(fmt.Sprintf("| **Target User ID** | `%s` |\n", run.TargetUserID))
		if ts, err := target.SnowflakeToTime(run.TargetUserID); err == nil {
			b.WriteString(fmt.Sprintf("| **Account Created** | %s |\n", ts.Format(time.RFC3339)))
		}
	} else {
		b.WriteString("| **Target User ID** | *(Unresolved &mdash; Username-Only Target)* |\n")
	}
	if run.TargetDisplayName != "" {
		b.WriteString(fmt.Sprintf("| **Display Name** | %s |\n", run.TargetDisplayName))
	}
	if run.TargetUsernameAliasNote != "" {
		b.WriteString(fmt.Sprintf("| **Alias Note** | %s |\n", run.TargetUsernameAliasNote))
	}
	b.WriteString(fmt.Sprintf("| **Resolution Source** | `%s` |\n", run.TargetResolutionSource))
	b.WriteString(fmt.Sprintf("| **Verified At** | %s |\n", run.TargetVerifiedAt.Format(time.RFC3339)))
	if run.TargetAvatarURL != "" {
		b.WriteString(fmt.Sprintf("| **Avatar URL** | [View Avatar](%s) |\n", run.TargetAvatarURL))
	}
	b.WriteString("\n")

	// 2. Executive Scan Statistics
	confirmedCount := 0
	candidateCount := 0
	notFoundCount := 0
	blockedCount := 0
	for _, s := range scans {
		switch s.BestMatchStatus {
		case "confirmed":
			confirmedCount++
		case "candidate":
			candidateCount++
		case "not_found":
			notFoundCount++
		}
		if s.ScanStatus == "blocked" {
			blockedCount++
		}
	}

	b.WriteString("## 2. Executive Summary\n\n")
	b.WriteString(fmt.Sprintf("- **Category Tag / Source:** `%s`\n", run.Tag))
	b.WriteString(fmt.Sprintf("- **Total Servers Examined:** %d\n", len(scans)))
	b.WriteString(fmt.Sprintf("- **Confirmed Target Presence:** **%d**\n", confirmedCount))
	b.WriteString(fmt.Sprintf("- **Candidate Target Presence:** **%d**\n", candidateCount))
	b.WriteString(fmt.Sprintf("- **Exhaustive Absence Verified (`not_found`):** %d\n", notFoundCount))
	b.WriteString(fmt.Sprintf("- **Gated / Blocked Servers:** %d\n", blockedCount))
	b.WriteString("\n")

	// 3. Server Findings Table
	b.WriteString("## 3. Server Scan Overview\n\n")
	b.WriteString("| Server Name | Invite | Scan Status | Match Status | Reason | Member Coverage |\n")
	b.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- |\n")
	for _, s := range scans {
		statusBadge := s.BestMatchStatus
		if s.BestMatchStatus == "confirmed" {
			statusBadge = "**CONFIRMED**"
		} else if s.BestMatchStatus == "candidate" {
			statusBadge = "*CANDIDATE*"
		}

		b.WriteString(fmt.Sprintf("| %s | `%s` | `%s` | %s | `%s` | `%s` |\n",
			s.GuildName, s.InviteCode, s.ScanStatus, statusBadge, s.BestMatchReason, s.MemberCoverage,
		))
	}
	b.WriteString("\n")

	// 4. Detailed Observations & Evidence
	b.WriteString("## 4. Observations & Evidence\n\n")
	if len(observations) == 0 {
		b.WriteString("No target candidates or confirmed identities were observed in the examined servers.\n\n")
	} else {
		for i, obs := range observations {
			b.WriteString(fmt.Sprintf("### Observation %d: %s (`%s`)\n\n", i+1, obs.ObservedUsername, obs.GuildID))
			b.WriteString(fmt.Sprintf("- **Match Status:** `%s` (Reason: `%s`)\n", obs.MatchStatus, obs.MatchReason))
			b.WriteString(fmt.Sprintf("- **Observed User ID:** `%s`\n", obs.ObservedUserID))
			if obs.ObservedNick != "" {
				b.WriteString(fmt.Sprintf("- **Guild Nickname:** %s\n", obs.ObservedNick))
			}
			if obs.ObservedJoinedAt != "" {
				b.WriteString(fmt.Sprintf("- **Target Joined Guild At:** %s\n", obs.ObservedJoinedAt))
			}
			if len(obs.ObservedRoles) > 0 {
				b.WriteString(fmt.Sprintf("- **Roles in Guild:** %s\n", strings.Join(obs.ObservedRoles, ", ")))
			}
			if obs.ObservedGuildAvatarURL != "" {
				b.WriteString(fmt.Sprintf("- **Guild Avatar:** [Avatar Link](%s)\n", obs.ObservedGuildAvatarURL))
			}
			b.WriteString(fmt.Sprintf("- **Acquisition Method:** `%s` at %s\n\n", obs.AcquisitionMethod, obs.ObservedAt.Format(time.RFC3339)))
		}
	}

	// 5. Collected Message Evidence Highlights
	b.WriteString("## 5. Message Evidence Highlights\n\n")
	if len(messages) == 0 {
		b.WriteString("No messages were collected during this run.\n\n")
	} else {
		b.WriteString(fmt.Sprintf("Total Messages Captured: **%d**\n\n", len(messages)))
		limit := 10
		if len(messages) < limit {
			limit = len(messages)
		}
		for _, m := range messages[:limit] {
			b.WriteString(fmt.Sprintf("> **[%s] %s** (Channel: `%s`)\n", m.Timestamp.Format(time.RFC3339), m.AuthorUsername, m.ChannelID))
			b.WriteString(fmt.Sprintf("> %s\n\n", m.Content))
		}
		if len(messages) > limit {
			b.WriteString(fmt.Sprintf("*(Showing first %d messages. Complete evidence exported to `messages.csv`)*\n\n", limit))
		}
	}

	b.WriteString("---\n*Generated by `discord-osint` production workflow with strict audit provenance.*\n")
	return b.String()
}
