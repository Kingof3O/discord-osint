package matcher

import (
	"strings"
	"time"

	"discord-osint/internal/target"
)

// MatchTarget evaluates a guild member against a confirmed target snapshot.
func MatchTarget(t target.ConfirmedTarget, m GuildMember, acquisitionMethod string) (MatchObservation, bool) {
	now := time.Now().UTC()

	obs := MatchObservation{
		GuildID:                m.GuildID,
		ObservedUserID:         m.UserID,
		ObservedUsernameNorm:   target.NormalizeUsername(m.Username),
		ObservedUsername:       m.Username,
		ObservedNick:           m.Nick,
		ObservedRoles:          m.Roles,
		ObservedGuildAvatarURL: m.GuildAvatarURL,
		ObservedAt:             now,
		AcquisitionMethod:      acquisitionMethod,
		MatchStatus:            MatchNone,
		MatchReason:            ReasonNone,
	}

	if !m.JoinedAt.IsZero() {
		obs.ObservedJoinedAt = m.JoinedAt.Format(time.RFC3339)
	}
	if !m.PremiumSince.IsZero() {
		obs.ObservedPremiumSince = m.PremiumSince.Format(time.RFC3339)
	}

	normTargetUser := target.NormalizeUsername(t.TargetUsername)
	normMemberUser := target.NormalizeUsername(m.Username)

	// Rule 1: Exact Snowflake User ID match -> ALWAYS CONFIRMED
	if t.TargetUserID != "" && m.UserID != "" && m.UserID == t.TargetUserID {
		obs.MatchStatus = MatchConfirmed
		obs.MatchReason = ReasonUserIDExact
		return obs, true
	}

	// Rule 2: User ID mismatch when target has an authoritative ID
	if t.TargetUserID != "" && m.UserID != "" && m.UserID != t.TargetUserID {
		if normTargetUser != "" && normMemberUser == normTargetUser {
			obs.MatchStatus = MatchCandidate
			obs.MatchReason = ReasonUsernameMatchIDMismatch
			return obs, true
		}
	}

	// Rule 3: Username-only target (TargetUserID is empty)
	if t.TargetUserID == "" {
		if m.Username == t.TargetUsername && t.TargetUsername != "" {
			obs.MatchStatus = MatchCandidate
			obs.MatchReason = ReasonUsernameExact
			return obs, true
		}
		if strings.EqualFold(m.Username, t.TargetUsername) && t.TargetUsername != "" {
			obs.MatchStatus = MatchCandidate
			obs.MatchReason = ReasonUsernameCaseFold
			return obs, true
		}
		if normMemberUser == normTargetUser && normTargetUser != "" {
			obs.MatchStatus = MatchCandidate
			obs.MatchReason = ReasonUsernameUnicodeFold
			return obs, true
		}
	}

	// Rule 4: Target alias note check (when both flags were supplied with mismatch)
	if t.TargetUsernameAliasNote != "" && strings.Contains(target.NormalizeUsername(t.TargetUsernameAliasNote), normMemberUser) && normMemberUser != "" {
		obs.MatchStatus = MatchCandidate
		obs.MatchReason = ReasonAliasUsernameMatch
		return obs, true
	}

	// Rule 5: Display name or Nickname similarity (never confirms, candidate only)
	compareTargets := []string{normTargetUser}
	if t.TargetDisplayName != "" {
		compareTargets = append(compareTargets, target.NormalizeDisplayName(t.TargetDisplayName))
	}

	for _, ct := range compareTargets {
		if ct == "" || len(ct) < 3 {
			continue
		}
		if m.Nick != "" {
			normNick := target.NormalizeDisplayName(m.Nick)
			if StringSimilarity(normNick, ct) >= 0.85 {
				obs.MatchStatus = MatchCandidate
				obs.MatchReason = ReasonNicknameSimilar
				return obs, true
			}
		}
		if m.DisplayName != "" {
			normDisp := target.NormalizeDisplayName(m.DisplayName)
			if StringSimilarity(normDisp, ct) >= 0.85 {
				obs.MatchStatus = MatchCandidate
				obs.MatchReason = ReasonNicknameSimilar
				return obs, true
			}
		}
	}

	return obs, false
}

// StringSimilarity calculates a normalized similarity ratio between 0.0 and 1.0.
func StringSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}
	if len(s1) == 0 || len(s2) == 0 {
		return 0.0
	}

	dist := levenshteinDistance([]rune(s1), []rune(s2))
	maxLen := len([]rune(s1))
	if len([]rune(s2)) > maxLen {
		maxLen = len([]rune(s2))
	}

	return 1.0 - float64(dist)/float64(maxLen)
}

func levenshteinDistance(r1, r2 []rune) int {
	len1 := len(r1)
	len2 := len(r2)

	row := make([]int, len2+1)
	for j := 0; j <= len2; j++ {
		row[j] = j
	}

	for i := 1; i <= len1; i++ {
		prev := row[0]
		row[0] = i
		for j := 1; j <= len2; j++ {
			tmp := row[j]
			cost := 0
			if r1[i-1] != r2[j-1] {
				cost = 1
			}
			row[j] = min3(row[j]+1, row[j-1]+1, prev+cost)
			prev = tmp
		}
	}

	return row[len2]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
