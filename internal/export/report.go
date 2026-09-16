package export

import (
	"fmt"
	"strconv"
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

// GenerateHTMLReport generates a standalone, self-contained HTML investigation report with modern Discord-style glassmorphism.
func GenerateHTMLReport(
	run *store.RunRecord,
	scans []store.GuildScanRecord,
	observations []store.ObservationRecord,
	messages []store.MessageRecord,
) string {
	var b strings.Builder

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

	accountCreated := "N/A"
	if run.TargetUserID != "" {
		if ts, err := target.SnowflakeToTime(run.TargetUserID); err == nil {
			accountCreated = ts.Format("2006-01-02 15:04:05 UTC")
		}
	}

	// Determine default Discord avatar using snowflake modulo 6
	defaultAvatarURL := "https://cdn.discordapp.com/embed/avatars/0.png"
	if run.TargetUserID != "" {
		if id, err := strconv.ParseUint(run.TargetUserID, 10, 64); err == nil {
			defaultAvatarURL = fmt.Sprintf("https://cdn.discordapp.com/embed/avatars/%d.png", (id>>22)%6)
		}
	}
	targetAvatarURL := run.TargetAvatarURL
	if targetAvatarURL == "" {
		for _, obs := range observations {
			if obs.ObservedGuildAvatarURL != "" {
				targetAvatarURL = obs.ObservedGuildAvatarURL
				break
			}
		}
	}
	if targetAvatarURL == "" {
		targetAvatarURL = defaultAvatarURL
	}

	initial := "U"
	if len(run.TargetUsername) > 0 {
		initial = strings.ToUpper(run.TargetUsername[:1])
	}

	displayName := run.TargetDisplayName
	if displayName == "" || displayName == run.TargetUsername {
		for _, obs := range observations {
			if obs.ObservedNick != "" {
				displayName = obs.ObservedNick
				break
			}
		}
	}
	if displayName == "" {
		displayName = run.TargetUsername
	}

	b.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Discord OSINT Report - ` + htmlEscape(run.TargetUsername) + ` (` + htmlEscape(run.RunID) + `)</title>
<style>
:root {
  --bg-base: #0b0c0e;
  --bg-glow-1: rgba(88, 101, 242, 0.16);
  --bg-glow-2: rgba(87, 242, 135, 0.10);
  --bg-glow-3: rgba(235, 69, 158, 0.08);
  --glass-card: rgba(28, 29, 34, 0.65);
  --glass-card-hover: rgba(36, 38, 44, 0.75);
  --glass-inner: rgba(18, 19, 23, 0.60);
  --glass-border: rgba(255, 255, 255, 0.08);
  --glass-border-hover: rgba(88, 101, 242, 0.45);
  --accent: #5865f2;
  --accent-glow: rgba(88, 101, 242, 0.35);
  --text-header: #ffffff;
  --text-normal: #dbdee1;
  --text-muted: #949ba4;
  --status-green: #57f287;
  --status-green-glow: rgba(87, 242, 135, 0.25);
  --status-yellow: #fee75c;
  --status-yellow-glow: rgba(254, 231, 92, 0.25);
  --status-red: #ed4245;
  --status-red-glow: rgba(237, 66, 69, 0.25);
  --status-gray: #80848e;
  --code-bg: rgba(0, 0, 0, 0.35);
  --radius-lg: 16px;
  --radius-md: 10px;
  --radius-sm: 6px;
}
* { box-sizing: border-box; margin: 0; padding: 0; }
body {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  background-color: var(--bg-base);
  background-image:
    radial-gradient(circle at 10% 15%, var(--bg-glow-1) 0%, transparent 40%),
    radial-gradient(circle at 90% 85%, var(--bg-glow-2) 0%, transparent 40%),
    radial-gradient(circle at 50% 50%, var(--bg-glow-3) 0%, transparent 50%);
  background-attachment: fixed;
  color: var(--text-normal);
  line-height: 1.5;
  padding: 32px 20px;
  min-height: 100vh;
}
.container { max-width: 1240px; margin: 0 auto; }

/* Top Navigation / Branding Bar */
.top-nav {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
  padding: 0 4px;
}
.brand-group {
  display: flex;
  align-items: center;
  gap: 12px;
}
.brand-icon {
  width: 38px;
  height: 38px;
  border-radius: 10px;
  background: linear-gradient(135deg, #5865f2 0%, #4752c4 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #ffffff;
  font-weight: 800;
  font-size: 16px;
  box-shadow: 0 4px 14px var(--accent-glow);
}
.brand-text h1 {
  font-size: 18px;
  font-weight: 700;
  color: var(--text-header);
  letter-spacing: -0.2px;
}
.brand-text p {
  font-size: 12px;
  color: var(--text-muted);
}
.nav-actions {
  display: flex;
  gap: 10px;
}
.action-btn {
  background: var(--glass-card);
  -webkit-backdrop-filter: blur(16px) saturate(180%);
  backdrop-filter: blur(16px) saturate(180%);
  border: 1px solid var(--glass-border);
  color: var(--text-normal);
  padding: 8px 16px;
  border-radius: var(--radius-sm);
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  transition: all 0.15s ease;
  text-decoration: none;
}
.action-btn:hover {
  background: var(--glass-card-hover);
  border-color: rgba(255, 255, 255, 0.18);
  color: var(--text-header);
}

/* Discord Profile Hero Card */
.discord-profile-card {
  background: var(--glass-card);
  -webkit-backdrop-filter: blur(20px) saturate(180%);
  backdrop-filter: blur(20px) saturate(180%);
  border: 1px solid var(--glass-border);
  border-radius: var(--radius-lg);
  box-shadow: 0 16px 40px rgba(0, 0, 0, 0.45), inset 0 1px 0 rgba(255, 255, 255, 0.08);
  overflow: hidden;
  margin-bottom: 28px;
}
.profile-banner {
  height: 130px;
  background: linear-gradient(135deg, #5865f2 0%, #2b3b75 40%, #171c2f 100%);
  position: relative;
}
.profile-body {
  padding: 0 28px 24px 28px;
  position: relative;
}
.avatar-row {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  margin-top: -50px;
  margin-bottom: 16px;
  flex-wrap: wrap;
  gap: 16px;
}
.avatar-wrapper {
  position: relative;
  width: 100px;
  height: 100px;
}
.discord-avatar {
  width: 100px;
  height: 100px;
  border-radius: 50%;
  border: 6px solid #16171b;
  background-color: var(--accent);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 38px;
  font-weight: 700;
  color: #ffffff;
  overflow: hidden;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.6);
}
.discord-avatar img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.presence-badge {
  position: absolute;
  bottom: 4px;
  right: 4px;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  border: 4px solid #16171b;
  background-color: var(--status-green);
  box-shadow: 0 0 8px var(--status-green-glow);
}
.target-title-block {
  margin-bottom: 16px;
}
.target-display-name {
  font-size: 26px;
  font-weight: 800;
  color: var(--text-header);
  display: flex;
  align-items: center;
  gap: 10px;
}
.target-handle {
  font-size: 15px;
  color: var(--text-muted);
  font-weight: 500;
  margin-top: 2px;
}
.chips-row {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 12px;
}
.discord-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 12px;
  background: rgba(88, 101, 242, 0.12);
  border: 1px solid rgba(88, 101, 242, 0.25);
  border-radius: 20px;
  font-size: 12px;
  font-weight: 600;
  color: #b5bac1;
}
.discord-chip.active {
  background: rgba(88, 101, 242, 0.22);
  border-color: var(--accent);
  color: #ffffff;
}
.profile-meta-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 12px;
  padding-top: 16px;
  border-top: 1px solid var(--glass-border);
}
.meta-item {
  background: var(--glass-inner);
  border: 1px solid var(--glass-border);
  border-radius: var(--radius-sm);
  padding: 10px 14px;
}
.meta-item-label {
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  font-weight: 700;
  color: var(--text-muted);
  margin-bottom: 4px;
}
.meta-item-value {
  font-size: 13px;
  color: var(--text-header);
  font-family: monospace;
  word-break: break-all;
}

/* KPI Stat Cards */
.grid-stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(190px, 1fr));
  gap: 16px;
  margin-bottom: 28px;
}
.stat-card {
  background: var(--glass-card);
  -webkit-backdrop-filter: blur(16px) saturate(180%);
  backdrop-filter: blur(16px) saturate(180%);
  border: 1px solid var(--glass-border);
  border-radius: var(--radius-md);
  padding: 18px;
  text-align: center;
  transition: all 0.2s ease;
  position: relative;
  overflow: hidden;
}
.stat-card:hover {
  transform: translateY(-2px);
  border-color: var(--glass-border-hover);
  box-shadow: 0 10px 28px rgba(0, 0, 0, 0.35);
}
.stat-label {
  font-size: 12px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--text-muted);
  margin-bottom: 6px;
}
.stat-val {
  font-size: 32px;
  font-weight: 800;
  color: var(--text-header);
  line-height: 1.1;
}

/* Glass Panels */
.glass-panel {
  background: var(--glass-card);
  -webkit-backdrop-filter: blur(20px) saturate(180%);
  backdrop-filter: blur(20px) saturate(180%);
  border: 1px solid var(--glass-border);
  border-radius: var(--radius-lg);
  box-shadow: 0 12px 36px rgba(0, 0, 0, 0.4);
  padding: 24px;
  margin-bottom: 28px;
}
.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--glass-border);
}
.panel-header h2 {
  font-size: 18px;
  font-weight: 700;
  color: var(--text-header);
  display: flex;
  align-items: center;
  gap: 10px;
}
.panel-count {
  font-size: 12px;
  background: rgba(255, 255, 255, 0.08);
  padding: 2px 8px;
  border-radius: 12px;
  color: var(--text-muted);
}

/* Tables */
.table-wrapper {
  overflow-x: auto;
  border-radius: var(--radius-md);
  border: 1px solid var(--glass-border);
  background: var(--glass-inner);
}
table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13.5px;
  text-align: left;
}
th, td {
  padding: 13px 16px;
  border-bottom: 1px solid var(--glass-border);
}
th {
  background: rgba(0, 0, 0, 0.25);
  color: var(--text-muted);
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
tr:last-child td { border-bottom: none; }
tr:hover td { background: rgba(255, 255, 255, 0.02); }

/* Badges */
.badge {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 3px 10px;
  border-radius: 6px;
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.4px;
}
.badge-confirmed {
  background: rgba(87, 242, 135, 0.15);
  color: var(--status-green);
  border: 1px solid rgba(87, 242, 135, 0.35);
  box-shadow: 0 0 10px var(--status-green-glow);
}
.badge-candidate {
  background: rgba(254, 231, 92, 0.15);
  color: var(--status-yellow);
  border: 1px solid rgba(254, 231, 92, 0.35);
  box-shadow: 0 0 10px var(--status-yellow-glow);
}
.badge-notfound {
  background: rgba(148, 155, 164, 0.15);
  color: var(--status-gray);
  border: 1px solid rgba(148, 155, 164, 0.25);
}
.badge-blocked {
  background: rgba(237, 66, 69, 0.15);
  color: var(--status-red);
  border: 1px solid rgba(237, 66, 69, 0.35);
  box-shadow: 0 0 10px var(--status-red-glow);
}
.code-chip {
  background: var(--code-bg);
  border: 1px solid var(--glass-border);
  border-radius: 4px;
  padding: 2px 7px;
  font-family: monospace;
  font-size: 12px;
  color: #c9cdfb;
}

/* Discord Style Observations */
.obs-list {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.obs-card {
  background: var(--glass-inner);
  border: 1px solid var(--glass-border);
  border-radius: var(--radius-md);
  padding: 18px;
  transition: all 0.15s ease;
}
.obs-card:hover {
  border-color: rgba(255, 255, 255, 0.15);
  background: rgba(24, 25, 30, 0.7);
}
.obs-top-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
  flex-wrap: wrap;
  gap: 8px;
}
.guild-header-title {
  font-size: 15px;
  font-weight: 700;
  color: var(--text-header);
  display: flex;
  align-items: center;
  gap: 8px;
}
.obs-details-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 8px;
  font-size: 13px;
  margin-bottom: 10px;
}
.role-pills-row {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
  align-items: center;
}
.role-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 12px;
  padding: 3px 9px;
  font-size: 12px;
  color: #dbdee1;
}
.role-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--accent);
}

/* Discord Style Messages Stream */
.discord-msg-stream {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.discord-msg {
  display: flex;
  gap: 16px;
  background: var(--glass-inner);
  border: 1px solid var(--glass-border);
  border-radius: var(--radius-md);
  padding: 16px;
  transition: all 0.15s ease;
}
.discord-msg:hover {
  background: rgba(36, 38, 44, 0.65);
  border-color: rgba(255, 255, 255, 0.14);
}
.msg-avatar {
  width: 42px;
  height: 42px;
  border-radius: 50%;
  background: var(--accent);
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
  color: #ffffff;
  font-size: 16px;
  flex-shrink: 0;
  overflow: hidden;
}
.msg-avatar img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.msg-body {
  flex: 1;
  min-width: 0;
}
.msg-meta-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 6px;
  flex-wrap: wrap;
}
.msg-author {
  font-weight: 700;
  color: var(--text-header);
  font-size: 14.5px;
}
.msg-chan-pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background: rgba(88, 101, 242, 0.15);
  color: #a3a9b7;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 12px;
  font-family: monospace;
}
.msg-timestamp {
  color: var(--text-muted);
  font-size: 12px;
}
.msg-text {
  color: var(--text-normal);
  font-size: 14.5px;
  line-height: 1.5;
  word-break: break-word;
  white-space: pre-wrap;
}

/* Footer */
footer {
  text-align: center;
  color: var(--text-muted);
  font-size: 12px;
  margin-top: 36px;
  padding: 16px;
  border-top: 1px solid var(--glass-border);
}
</style>
</head>
<body>
<div class="container">

  <!-- Top Bar -->
  <div class="top-nav">
    <div class="brand-group">
      <div class="brand-icon">DO</div>
      <div class="brand-text">
        <h1>Discord OSINT Investigation Report</h1>
        <p>Strict Forensic Audit Provenance &bull; Generated ` + htmlEscape(time.Now().UTC().Format("2006-01-02 15:04:05 UTC")) + `</p>
      </div>
    </div>
    <div class="nav-actions">
      <button onclick="window.print()" class="action-btn">
        <span>🖨️</span> Print / Export PDF
      </button>
    </div>
  </div>

  <!-- Discord Profile Card -->
  <div class="discord-profile-card">
    <div class="profile-banner"></div>
    <div class="profile-body">
      <div class="avatar-row">
        <div class="avatar-wrapper">
          <div class="discord-avatar">
            <img src="` + htmlEscape(targetAvatarURL) + `" alt="Avatar" onerror="this.src='` + htmlEscape(defaultAvatarURL) + `';this.onerror=function(){this.style.display='none';this.parentNode.innerText='` + htmlEscape(initial) + `'};">
          </div>
          <div class="presence-badge" title="Target Online/Active Status"></div>
        </div>
        <div class="chips-row">
          <span class="discord-chip active">🛡️ TARGET IDENTITY</span>
          <span class="discord-chip">SOURCE: ` + htmlEscape(run.TargetResolutionSource) + `</span>
          <span class="discord-chip">RUN: ` + htmlEscape(run.RunID) + `</span>
        </div>
      </div>

      <div class="target-title-block">
        <div class="target-display-name">
          ` + htmlEscape(displayName) + `
        </div>
        <div class="target-handle">@` + htmlEscape(run.TargetUsername) + `</div>
      </div>

      <div class="profile-meta-grid">
        <div class="meta-item">
          <div class="meta-item-label">Target Snowflake ID</div>
          <div class="meta-item-value">` + htmlEscape(run.TargetUserID) + `</div>
        </div>
        <div class="meta-item">
          <div class="meta-item-label">Account Created</div>
          <div class="meta-item-value">` + htmlEscape(accountCreated) + `</div>
        </div>
        <div class="meta-item">
          <div class="meta-item-label">Category Tag / Domain</div>
          <div class="meta-item-value">` + htmlEscape(run.Tag) + `</div>
        </div>
        <div class="meta-item">
          <div class="meta-item-label">Verification Provenance</div>
          <div class="meta-item-value">` + htmlEscape(run.TargetVerifiedAt.Format("2006-01-02 15:04:05 UTC")) + `</div>
        </div>
      </div>
    </div>
  </div>

  <!-- KPI Cards -->
  <div class="grid-stats">
    <div class="stat-card">
      <div class="stat-label">Servers Examined</div>
      <div class="stat-val">` + fmt.Sprintf("%d", len(scans)) + `</div>
    </div>
    <div class="stat-card" style="box-shadow: 0 4px 20px var(--status-green-glow);">
      <div class="stat-label" style="color: var(--status-green);">Confirmed Presence</div>
      <div class="stat-val" style="color: var(--status-green);">` + fmt.Sprintf("%d", confirmedCount) + `</div>
    </div>
    <div class="stat-card" style="box-shadow: 0 4px 20px var(--status-yellow-glow);">
      <div class="stat-label" style="color: var(--status-yellow);">Candidate Presence</div>
      <div class="stat-val" style="color: var(--status-yellow);">` + fmt.Sprintf("%d", candidateCount) + `</div>
    </div>
    <div class="stat-card">
      <div class="stat-label">Absence Verified</div>
      <div class="stat-val">` + fmt.Sprintf("%d", notFoundCount) + `</div>
    </div>
    <div class="stat-card" style="box-shadow: 0 4px 20px var(--status-red-glow);">
      <div class="stat-label" style="color: var(--status-red);">Gated / Blocked</div>
      <div class="stat-val" style="color: var(--status-red);">` + fmt.Sprintf("%d", blockedCount) + `</div>
    </div>
  </div>

  <!-- Scanned Servers Section -->
  <div class="glass-panel">
    <div class="panel-header">
      <h2>🌐 Scanned Server Findings <span class="panel-count">` + fmt.Sprintf("%d servers", len(scans)) + `</span></h2>
    </div>
    <div class="table-wrapper">
      <table>
        <thead>
          <tr>
            <th>Server Name</th>
            <th>Invite Code</th>
            <th>Scan Status</th>
            <th>Match Status</th>
            <th>Match Reason</th>
            <th>Member Coverage</th>
          </tr>
        </thead>
        <tbody>`)

	for _, s := range scans {
		badgeClass := "badge-notfound"
		switch s.BestMatchStatus {
		case "confirmed":
			badgeClass = "badge-confirmed"
		case "candidate":
			badgeClass = "badge-candidate"
		case "not_found":
			badgeClass = "badge-notfound"
		}
		if s.ScanStatus == "blocked" {
			badgeClass = "badge-blocked"
		}

		b.WriteString(fmt.Sprintf(`
          <tr>
            <td><strong>%s</strong></td>
            <td><span class="code-chip">%s</span></td>
            <td>%s</td>
            <td><span class="badge %s">%s</span></td>
            <td>%s</td>
            <td>%s</td>
          </tr>`,
			htmlEscape(s.GuildName),
			htmlEscape(s.InviteCode),
			htmlEscape(s.ScanStatus),
			badgeClass,
			htmlEscape(s.BestMatchStatus),
			htmlEscape(s.BestMatchReason),
			htmlEscape(s.MemberCoverage),
		))
	}

	b.WriteString(`
        </tbody>
      </table>
    </div>
  </div>

  <!-- Identity Observations & Findings -->
  <div class="glass-panel">
    <div class="panel-header">
      <h2>🔎 Target Identity Observations & Evidence <span class="panel-count">` + fmt.Sprintf("%d observations", len(observations)) + `</span></h2>
    </div>`)

	if len(observations) == 0 {
		b.WriteString(`<p style="color: var(--text-muted); font-size: 14px; text-align: center; padding: 24px;">No target identity matches observed in the scanned servers.</p>`)
	} else {
		b.WriteString(`<div class="obs-list">`)
		for _, obs := range observations {
			badgeClass := "badge-candidate"
			if obs.MatchStatus == "confirmed" {
				badgeClass = "badge-confirmed"
			}

			nickDisplay := "-"
			if obs.ObservedNick != "" {
				nickDisplay = obs.ObservedNick
			}

			joinedDisplay := "Unknown"
			if obs.ObservedJoinedAt != "" {
				joinedDisplay = obs.ObservedJoinedAt
			}

			b.WriteString(fmt.Sprintf(`
        <div class="obs-card">
          <div class="obs-top-bar">
            <div class="guild-header-title">
              <span>🏛️ Guild: <code>%s</code></span>
              <span style="color: var(--text-muted);">&bull;</span>
              <span>Observed: <strong>@%s</strong></span>
            </div>
            <span class="badge %s">%s</span>
          </div>

          <div class="obs-details-grid">
            <div><span style="color: var(--text-muted);">Observed Snowflake:</span> <code class="code-chip">%s</code></div>
            <div><span style="color: var(--text-muted);">Server Nickname:</span> <strong>%s</strong></div>
            <div><span style="color: var(--text-muted);">Joined Server:</span> %s</div>
            <div><span style="color: var(--text-muted);">Match Reason:</span> <code class="code-chip">%s</code></div>
          </div>

          <div style="font-size: 12px; color: var(--text-muted); margin-top: 6px;">
            Acquisition: <code>%s</code> at %s
          </div>`,
				htmlEscape(obs.GuildID),
				htmlEscape(obs.ObservedUsername),
				badgeClass,
				htmlEscape(string(obs.MatchStatus)),
				htmlEscape(obs.ObservedUserID),
				htmlEscape(nickDisplay),
				htmlEscape(joinedDisplay),
				htmlEscape(string(obs.MatchReason)),
				htmlEscape(obs.AcquisitionMethod),
				htmlEscape(obs.ObservedAt.Format(time.RFC3339)),
			))

			if len(obs.ObservedRoles) > 0 {
				b.WriteString(`<div class="role-pills-row"><span style="font-size: 12px; color: var(--text-muted); margin-right: 4px;">Assigned Roles:</span>`)
				for _, r := range obs.ObservedRoles {
					b.WriteString(`<span class="role-pill"><span class="role-dot"></span>` + htmlEscape(r) + `</span>`)
				}
				b.WriteString(`</div>`)
			}

			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
	}

	b.WriteString(`
  </div>

  <!-- Message Evidence Highlights -->
  <div class="glass-panel">
    <div class="panel-header">
      <h2>💬 Captured Message Evidence Highlights <span class="panel-count">` + fmt.Sprintf("%d captured", len(messages)) + `</span></h2>
    </div>`)

	if len(messages) == 0 {
		b.WriteString(`<p style="color: var(--text-muted); font-size: 14px; text-align: center; padding: 24px;">No message records captured during this investigation run.</p>`)
	} else {
		b.WriteString(`<div class="discord-msg-stream">`)
		for _, m := range messages {
			msgInitial := "U"
			if len(m.AuthorUsername) > 0 {
				msgInitial = strings.ToUpper(m.AuthorUsername[:1])
			}

			msgAvatar := defaultAvatarURL
			if m.AuthorID != "" && m.AuthorID == run.TargetUserID && targetAvatarURL != "" {
				msgAvatar = targetAvatarURL
			} else if m.AuthorID != "" {
				if id, err := strconv.ParseUint(m.AuthorID, 10, 64); err == nil {
					msgAvatar = fmt.Sprintf("https://cdn.discordapp.com/embed/avatars/%d.png", (id>>22)%6)
				}
			}

			b.WriteString(fmt.Sprintf(`
        <div class="discord-msg">
          <div class="msg-avatar">
            <img src="%s" alt="%s" onerror="this.style.display='none';this.parentNode.innerText='%s'">
          </div>
          <div class="msg-body">
            <div class="msg-meta-row">
              <span class="msg-author">%s</span>
              <span class="msg-chan-pill">#%s</span>
              <span class="msg-timestamp">%s</span>
            </div>
            <div class="msg-text">%s</div>
          </div>
        </div>`,
				htmlEscape(msgAvatar),
				htmlEscape(m.AuthorUsername),
				htmlEscape(msgInitial),
				htmlEscape(m.AuthorUsername),
				htmlEscape(m.ChannelID),
				htmlEscape(m.Timestamp.Format("2006-01-02 15:04:05 UTC")),
				htmlEscape(m.Content),
			))
		}
		b.WriteString(`</div>`)
	}

	b.WriteString(`
  </div>

  <footer>
    Discord OSINT Investigation Platform &bull; Anti-Bot Hardened Provenance Engine &bull; Strictly Partitioned Sandbox Run
  </footer>

</div>
</body>
</html>`)

	return b.String()
}

func htmlEscape(s string) string {
	var b strings.Builder
	for _, c := range s {
		switch c {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&#39;")
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}

