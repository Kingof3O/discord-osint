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

// GenerateHTMLReport generates a standalone, self-contained HTML investigation report with embedded styling.
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

	b.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Discord OSINT Report - ` + htmlEscape(run.TargetUsername) + ` (` + htmlEscape(run.RunID) + `)</title>
<style>
:root {
  --bg-primary: #1e1f22;
  --bg-secondary: #2b2d31;
  --bg-tertiary: #313338;
  --accent: #5865f2;
  --accent-hover: #4752c4;
  --text-normal: #dbdee1;
  --text-muted: #949ba4;
  --text-header: #ffffff;
  --status-green: #23a55a;
  --status-yellow: #f0b232;
  --status-red: #f23f43;
  --status-gray: #80848e;
  --border: #3f4147;
}
* { box-sizing: border-box; margin: 0; padding: 0; }
body {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  background-color: var(--bg-primary);
  color: var(--text-normal);
  line-height: 1.5;
  padding: 24px;
}
.container { max-width: 1200px; margin: 0 auto; }
header {
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 24px;
  margin-bottom: 24px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 16px;
}
.target-identity { display: flex; align-items: center; gap: 16px; }
.avatar {
  width: 64px;
  height: 64px;
  border-radius: 50%;
  background-color: var(--accent);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 24px;
  font-weight: bold;
  color: #fff;
  overflow: hidden;
}
.avatar img { width: 100%; height: 100%; object-fit: cover; }
h1 { font-size: 24px; color: var(--text-header); }
.meta-text { color: var(--text-muted); font-size: 14px; }
.badge {
  display: inline-block;
  padding: 4px 10px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
}
.badge-confirmed { background-color: rgba(35, 165, 90, 0.2); color: var(--status-green); border: 1px solid var(--status-green); }
.badge-candidate { background-color: rgba(240, 178, 50, 0.2); color: var(--status-yellow); border: 1px solid var(--status-yellow); }
.badge-notfound { background-color: rgba(128, 132, 142, 0.2); color: var(--status-gray); border: 1px solid var(--status-gray); }
.badge-blocked { background-color: rgba(242, 63, 67, 0.2); color: var(--status-red); border: 1px solid var(--status-red); }
.badge-running { background-color: rgba(88, 101, 242, 0.2); color: var(--accent); border: 1px solid var(--accent); }
.grid-stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 16px;
  margin-bottom: 24px;
}
.stat-card {
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 16px;
  text-align: center;
}
.stat-val { font-size: 28px; font-weight: bold; color: var(--text-header); margin-top: 4px; }
.section {
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 20px;
  margin-bottom: 24px;
}
.section h2 { font-size: 18px; color: var(--text-header); margin-bottom: 16px; border-bottom: 1px solid var(--border); padding-bottom: 8px; }
table { width: 100%; border-collapse: collapse; font-size: 14px; text-align: left; }
th, td { padding: 12px 14px; border-bottom: 1px solid var(--border); }
th { background-color: var(--bg-tertiary); color: var(--text-header); }
tr:hover { background-color: rgba(255, 255, 255, 0.02); }
.obs-card {
  background: var(--bg-tertiary);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 16px;
  margin-bottom: 12px;
}
.obs-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px; }
.msg-item {
  background: var(--bg-tertiary);
  border-left: 4px solid var(--accent);
  padding: 10px 14px;
  border-radius: 4px;
  margin-bottom: 8px;
  font-size: 13px;
}
.msg-meta { color: var(--text-muted); font-size: 12px; margin-bottom: 4px; }
footer { text-align: center; color: var(--text-muted); font-size: 12px; margin-top: 32px; }
</style>
</head>
<body>
<div class="container">
  <header>
    <div class="target-identity">
      <div class="avatar">`)

	if run.TargetAvatarURL != "" {
		b.WriteString(`<img src="` + htmlEscape(run.TargetAvatarURL) + `" alt="Avatar">`)
	} else {
		initial := "U"
		if len(run.TargetUsername) > 0 {
			initial = strings.ToUpper(run.TargetUsername[:1])
		}
		b.WriteString(htmlEscape(initial))
	}

	b.WriteString(`</div>
      <div>
        <h1>` + htmlEscape(run.TargetUsername) + `</h1>
        <div class="meta-text">User ID: <code>` + htmlEscape(run.TargetUserID) + `</code> | Created: ` + htmlEscape(accountCreated) + `</div>
        <div class="meta-text">Display: ` + htmlEscape(run.TargetDisplayName) + ` | Source: ` + htmlEscape(run.TargetResolutionSource) + `</div>
      </div>
    </div>
    <div>
      <div class="meta-text">Run ID: <code>` + htmlEscape(run.RunID) + `</code></div>
      <div class="meta-text">Category Tag: <code>` + htmlEscape(run.Tag) + `</code></div>
      <div class="meta-text">Generated: ` + htmlEscape(time.Now().UTC().Format("2006-01-02 15:04:05 UTC")) + `</div>
    </div>
  </header>

  <div class="grid-stats">
    <div class="stat-card">
      <div class="meta-text">Servers Examined</div>
      <div class="stat-val">` + fmt.Sprintf("%d", len(scans)) + `</div>
    </div>
    <div class="stat-card">
      <div class="meta-text">Confirmed Presence</div>
      <div class="stat-val" style="color: var(--status-green);">` + fmt.Sprintf("%d", confirmedCount) + `</div>
    </div>
    <div class="stat-card">
      <div class="meta-text">Candidate Presence</div>
      <div class="stat-val" style="color: var(--status-yellow);">` + fmt.Sprintf("%d", candidateCount) + `</div>
    </div>
    <div class="stat-card">
      <div class="meta-text">Absence Verified</div>
      <div class="stat-val">` + fmt.Sprintf("%d", notFoundCount) + `</div>
    </div>
    <div class="stat-card">
      <div class="meta-text">Gated / Blocked</div>
      <div class="stat-val" style="color: var(--status-red);">` + fmt.Sprintf("%d", blockedCount) + `</div>
    </div>
  </div>

  <div class="section">
    <h2>Server Scan Overview</h2>
    <table>
      <thead>
        <tr>
          <th>Server Name</th>
          <th>Invite</th>
          <th>Scan Status</th>
          <th>Match Status</th>
          <th>Reason</th>
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

		b.WriteString(fmt.Sprintf(`<tr>
          <td><strong>%s</strong></td>
          <td><code>%s</code></td>
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

	b.WriteString(`</tbody>
    </table>
  </div>

  <div class="section">
    <h2>Identity Observations & Findings</h2>`)

	if len(observations) == 0 {
		b.WriteString(`<p class="meta-text">No target identity matches observed in the scanned servers.</p>`)
	} else {
		for _, obs := range observations {
			badgeClass := "badge-candidate"
			if obs.MatchStatus == "confirmed" {
				badgeClass = "badge-confirmed"
			}
			rolesStr := "None"
			if len(obs.ObservedRoles) > 0 {
				rolesStr = strings.Join(obs.ObservedRoles, ", ")
			}

			b.WriteString(fmt.Sprintf(`
      <div class="obs-card">
        <div class="obs-header">
          <div>
            <strong>%s</strong> (Guild ID: <code>%s</code>)
          </div>
          <span class="badge %s">%s</span>
        </div>
        <div class="meta-text" style="margin-bottom: 4px;"><strong>Observed User ID:</strong> <code>%s</code> | <strong>Nick:</strong> %s</div>
        <div class="meta-text" style="margin-bottom: 4px;"><strong>Joined Guild At:</strong> %s | <strong>Roles:</strong> %s</div>
        <div class="meta-text"><strong>Match Reason:</strong> %s | <strong>Acquisition Method:</strong> %s at %s</div>
      </div>`,
				htmlEscape(obs.ObservedUsername),
				htmlEscape(obs.GuildID),
				badgeClass,
				htmlEscape(string(obs.MatchStatus)),
				htmlEscape(obs.ObservedUserID),
				htmlEscape(obs.ObservedNick),
				htmlEscape(obs.ObservedJoinedAt),
				htmlEscape(rolesStr),
				htmlEscape(string(obs.MatchReason)),
				htmlEscape(obs.AcquisitionMethod),
				htmlEscape(obs.ObservedAt.Format(time.RFC3339)),
			))
		}
	}

	b.WriteString(`</div>

  <div class="section">
    <h2>Message Evidence Highlights</h2>`)

	if len(messages) == 0 {
		b.WriteString(`<p class="meta-text">No message records captured during this investigation.</p>`)
	} else {
		limit := 15
		if len(messages) < limit {
			limit = len(messages)
		}
		for _, m := range messages[:limit] {
			b.WriteString(fmt.Sprintf(`
      <div class="msg-item">
        <div class="msg-meta">[%s] <strong>%s</strong> (Channel: <code>%s</code>)</div>
        <div>%s</div>
      </div>`,
				htmlEscape(m.Timestamp.Format("2006-01-02 15:04:05 UTC")),
				htmlEscape(m.AuthorUsername),
				htmlEscape(m.ChannelID),
				htmlEscape(m.Content),
			))
		}
		if len(messages) > limit {
			b.WriteString(fmt.Sprintf(`<p class="meta-text" style="margin-top: 12px;">Showing first %d messages. Full message corpus is preserved in <code>messages.csv</code>.</p>`, limit))
		}
	}

	b.WriteString(`</div>

  <footer>
    Discord OSINT Investigation Report &mdash; Generated with Anti-Bot Hardening & Strict Audit Provenance
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

