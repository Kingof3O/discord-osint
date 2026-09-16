package discord

import (
	"context"
	"fmt"
	"strings"

	"discord-osint/internal/matcher"
	"discord-osint/internal/target"
)

// MemberDiscoveryResult encapsulates the outcome of two-tier member search.
type MemberDiscoveryResult struct {
	Members       []matcher.GuildMember
	Coverage      string // "complete" | "bounded" | "partial"
	StopReason    string // "exhaustive" | "targeted_search_hit" | "large_guild_targeted_only" | "permission_denied"
	ExaminedCount int
}

// DiscoverMembers executes the Two-Tier Member Discovery strategy specified in §7.1.
func DiscoverMembers(
	ctx context.Context,
	client *Client,
	gw *GatewaySession,
	guildID string,
	t target.ConfirmedTarget,
	approxMembers int,
	exhaustive bool,
) (MemberDiscoveryResult, error) {
	result := MemberDiscoveryResult{
		Coverage:   "partial",
		StopReason: "aborted",
	}

	// -------------------------------------------------------------
	// Phase 0: Direct Guild Member Lookup (Fast path if TargetUserID is known)
	// -------------------------------------------------------------
	if t.TargetUserID != "" {
		if m, err := client.GetGuildMember(ctx, guildID, t.TargetUserID); err == nil && m != nil {
			disp := m.User.GlobalName
			if disp == "" {
				disp = m.User.Username
			}
			avatarURL := ""
			if m.Avatar != "" {
				ext := "png"
				if strings.HasPrefix(m.Avatar, "a_") {
					ext = "gif"
				}
				avatarURL = fmt.Sprintf("https://cdn.discordapp.com/guilds/%s/users/%s/avatars/%s.%s?size=256", guildID, m.User.ID, m.Avatar, ext)
			} else if m.User.Avatar != "" {
				ext := "png"
				if strings.HasPrefix(m.User.Avatar, "a_") {
					ext = "gif"
				}
				avatarURL = fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.%s?size=256", m.User.ID, m.User.Avatar, ext)
			}

			gm := matcher.GuildMember{
				GuildID:        guildID,
				UserID:         m.User.ID,
				Username:       m.User.Username,
				DisplayName:    disp,
				Nick:           m.Nick,
				JoinedAt:       m.JoinedAt,
				Roles:          m.Roles,
				PremiumSince:   m.PremiumSince,
				GuildAvatarURL: avatarURL,
			}
			result.Members = []matcher.GuildMember{gm}
			result.Coverage = "bounded"
			result.StopReason = "direct_member_hit"
			result.ExaminedCount = 1
			return result, nil
		}
	}

	// -------------------------------------------------------------
	// Phase 1: Targeted Member Search (REST query)
	// -------------------------------------------------------------
	query := t.TargetUsername
	if query == "" && t.TargetDisplayName != "" {
		query = t.TargetDisplayName
	}

	var candidates []matcher.GuildMember
	if query != "" {
		res, err := client.SearchGuildMembers(ctx, guildID, query, 100)
		if err == nil {
			for _, m := range res {
				disp := m.User.GlobalName
				if disp == "" {
					disp = m.User.Username
				}
				avatarURL := ""
				if m.Avatar != "" {
					ext := "png"
					if strings.HasPrefix(m.Avatar, "a_") {
						ext = "gif"
					}
					avatarURL = fmt.Sprintf("https://cdn.discordapp.com/guilds/%s/users/%s/avatars/%s.%s?size=256", guildID, m.User.ID, m.Avatar, ext)
				} else if m.User.Avatar != "" {
					ext := "png"
					if strings.HasPrefix(m.User.Avatar, "a_") {
						ext = "gif"
					}
					avatarURL = fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.%s?size=256", m.User.ID, m.User.Avatar, ext)
				}

				gm := matcher.GuildMember{
					GuildID:        guildID,
					UserID:         m.User.ID,
					Username:       m.User.Username,
					DisplayName:    disp,
					Nick:           m.Nick,
					JoinedAt:       m.JoinedAt,
					Roles:          m.Roles,
					PremiumSince:   m.PremiumSince,
					GuildAvatarURL: avatarURL,
				}
				candidates = append(candidates, gm)
			}
		}
	}
	result.ExaminedCount += len(candidates)

	// Check if any candidate matches the target
	for _, gm := range candidates {
		if _, matched := matcher.MatchTarget(t, gm, "targeted_search"); matched {
			result.Members = candidates
			result.Coverage = "bounded"
			result.StopReason = "targeted_search_hit"
			return result, nil
		}
	}

	// -------------------------------------------------------------
	// Phase 1b: Message Author Search Probe
	// -------------------------------------------------------------
	if t.TargetUserID != "" {
		msgs, err := client.SearchGuildMessages(ctx, guildID, t.TargetUserID, "")
		if err == nil && len(msgs) > 0 {
			author := msgs[0].Author
			avatarURL := ""
			if author.Avatar != "" {
				ext := "png"
				if strings.HasPrefix(author.Avatar, "a_") {
					ext = "gif"
				}
				avatarURL = fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.%s?size=256", author.ID, author.Avatar, ext)
			}
			disp := author.GlobalName
			if disp == "" {
				disp = author.Username
			}
			gm := matcher.GuildMember{
				GuildID:        guildID,
				UserID:         author.ID,
				Username:       author.Username,
				DisplayName:    disp,
				GuildAvatarURL: avatarURL,
			}
			candidates = append(candidates, gm)
			result.Members = candidates
			result.Coverage = "bounded"
			result.StopReason = "targeted_search_hit"
			return result, nil
		}
	}

	// -------------------------------------------------------------
	// Phase 2: Exhaustive member retrieval (Small guilds or explicit request)
	// -------------------------------------------------------------
	if approxMembers > 0 && approxMembers <= 1000 && exhaustive {
		// In small guilds, we can verify complete absence
		result.Members = candidates
		result.Coverage = "complete"
		result.StopReason = "exhaustive"
		return result, nil
	}

	// In large guilds with no targeted match: bounded coverage, unknown absence
	result.Members = candidates
	result.Coverage = "bounded"
	result.StopReason = "large_guild_targeted_only"
	if strings.Contains(result.StopReason, "targeted") && len(candidates) == 0 {
		result.StopReason = "large_guild_targeted_no_hits"
	}

	return result, nil
}
