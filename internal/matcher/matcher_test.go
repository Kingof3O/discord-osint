package matcher

import (
	"testing"
	"time"

	"discord-osint/internal/target"
)

func TestMatchTarget_UserIDExact(t *testing.T) {
	tgt := target.ConfirmedTarget{
		TargetUserID:   "123456789012345678",
		TargetUsername: "target_user",
	}

	joined := time.Date(2023, 5, 10, 12, 0, 0, 0, time.UTC)
	member := GuildMember{
		GuildID:        "999888777",
		UserID:         "123456789012345678",
		Username:       "different_username",
		Nick:           "CoolGuy",
		JoinedAt:       joined,
		Roles:          []string{"Admin", "VIP"},
		GuildAvatarURL: "https://cdn.discordapp.com/guilds/999/users/123/avatar.png",
	}

	obs, matched := MatchTarget(tgt, member, "gateway_chunk")
	if !matched {
		t.Fatalf("expected match, got none")
	}
	if obs.MatchStatus != MatchConfirmed {
		t.Errorf("expected confirmed status, got %s", obs.MatchStatus)
	}
	if obs.MatchReason != ReasonUserIDExact {
		t.Errorf("expected reason user_id_exact, got %s", obs.MatchReason)
	}
	if obs.ObservedJoinedAt != joined.Format(time.RFC3339) {
		t.Errorf("expected joined_at %s, got %s", joined.Format(time.RFC3339), obs.ObservedJoinedAt)
	}
	if len(obs.ObservedRoles) != 2 || obs.ObservedRoles[0] != "Admin" {
		t.Errorf("expected roles preserved, got %v", obs.ObservedRoles)
	}
}

func TestMatchTarget_IDMismatch(t *testing.T) {
	tgt := target.ConfirmedTarget{
		TargetUserID:   "111111111111111111",
		TargetUsername: "same_username",
	}

	member := GuildMember{
		GuildID:  "999888777",
		UserID:   "222222222222222222",
		Username: "same_username",
	}

	obs, matched := MatchTarget(tgt, member, "rest_members")
	if !matched {
		t.Fatalf("expected candidate match for same username with different ID")
	}
	if obs.MatchStatus != MatchCandidate {
		t.Errorf("expected candidate, got %s", obs.MatchStatus)
	}
	if obs.MatchReason != ReasonUsernameMatchIDMismatch {
		t.Errorf("expected reason username_match_id_mismatch, got %s", obs.MatchReason)
	}
}

func TestMatchTarget_UsernameOnly_Variants(t *testing.T) {
	tgt := target.ConfirmedTarget{
		TargetUserID:   "", // unresolved username-only
		TargetUsername: "john.doe",
	}

	// Exact
	m1 := GuildMember{Username: "john.doe", GuildID: "g1"}
	obs1, matched1 := MatchTarget(tgt, m1, "search")
	if !matched1 || obs1.MatchStatus != MatchCandidate || obs1.MatchReason != ReasonUsernameExact {
		t.Errorf("m1 failed: matched=%v, status=%s, reason=%s", matched1, obs1.MatchStatus, obs1.MatchReason)
	}

	// Case fold
	m2 := GuildMember{Username: "John.Doe", GuildID: "g1"}
	obs2, matched2 := MatchTarget(tgt, m2, "search")
	if !matched2 || obs2.MatchStatus != MatchCandidate || obs2.MatchReason != ReasonUsernameCaseFold {
		t.Errorf("m2 failed: matched=%v, status=%s, reason=%s", matched2, obs2.MatchStatus, obs2.MatchReason)
	}
}

func TestMatchTarget_NicknameSimilarity(t *testing.T) {
	tgt := target.ConfirmedTarget{
		TargetUserID:      "111",
		TargetUsername:    "alexander",
		TargetDisplayName: "Alexander The Great",
	}

	member := GuildMember{
		GuildID:     "g1",
		UserID:      "999",
		Username:    "random_handle",
		Nick:        "Alexander", // high similarity to target username
		DisplayName: "OtherName",
	}

	obs, matched := MatchTarget(tgt, member, "gateway")
	if !matched {
		t.Fatalf("expected nickname match")
	}
	if obs.MatchStatus != MatchCandidate {
		t.Errorf("expected candidate, got %s", obs.MatchStatus)
	}
	if obs.MatchReason != ReasonNicknameSimilar {
		t.Errorf("expected reason nickname_similar, got %s", obs.MatchReason)
	}
}

func TestMatchTarget_NoMatch(t *testing.T) {
	tgt := target.ConfirmedTarget{
		TargetUserID:   "111",
		TargetUsername: "alice",
	}

	member := GuildMember{
		GuildID:  "g1",
		UserID:   "222",
		Username: "bob",
		Nick:     "Robert",
	}

	_, matched := MatchTarget(tgt, member, "gateway")
	if matched {
		t.Fatalf("expected no match for completely different user")
	}
}
