package discord

import (
	"time"
)

// User represents a Discord user account.
type User struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Discriminator string `json:"discriminator"`
	GlobalName    string `json:"global_name"`
	Avatar        string `json:"avatar"`
	Bot           bool   `json:"bot"`
	MFAEnabled    bool   `json:"mfa_enabled"`
	Flags         int    `json:"flags"`
	Phone         string `json:"phone,omitempty"`
	Email         string `json:"email,omitempty"`
	Verified      bool   `json:"verified,omitempty"`
}

// GuildSummary represents basic guild information embedded in invites.
type GuildSummary struct {
	ID                       string   `json:"id"`
	Name                     string   `json:"name"`
	Icon                     string   `json:"icon"`
	Description              string   `json:"description"`
	ApproximateMemberCount   int      `json:"approximate_member_count"`
	ApproximatePresenceCount int      `json:"approximate_presence_count"`
	Features                 []string `json:"features"`
}

// InviteMetadata represents an invite lookup response.
type InviteMetadata struct {
	Code      string        `json:"code"`
	Guild     *GuildSummary `json:"guild"`
	Channel   *Channel      `json:"channel"`
	ExpiresAt *time.Time    `json:"expires_at"`
}

// Channel represents a Discord text or voice channel.
type Channel struct {
	ID        string `json:"id"`
	Type      int    `json:"type"` // 0: text, 2: voice, 4: category, 5: announcement
	GuildID   string `json:"guild_id"`
	Name      string `json:"name"`
	Topic     string `json:"topic"`
	NSFW      bool   `json:"nsfw"`
	Position  int    `json:"position"`
	ParentID  string `json:"parent_id,omitempty"`
}

// GuildMemberResponse represents a member record returned by Discord REST/Gateway.
type GuildMemberResponse struct {
	User         User      `json:"user"`
	Nick         string    `json:"nick"`
	Avatar       string    `json:"avatar"`
	Roles        []string  `json:"roles"`
	JoinedAt     time.Time `json:"joined_at"`
	PremiumSince time.Time `json:"premium_since,omitempty"`
	Deaf         bool      `json:"deaf"`
	Mute         bool      `json:"mute"`
	Pending      bool      `json:"pending"`
}

// MessageReaction represents a reaction on a message.
type MessageReaction struct {
	Count int `json:"count"`
	Me    bool `json:"me"`
	Emoji struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"emoji"`
}

// MessageComponent represents buttons or select menus on a message.
type MessageComponent struct {
	Type     int                `json:"type"` // 1: action row, 2: button, 3: select menu
	Style    int                `json:"style"`
	Label    string             `json:"label"`
	CustomID string             `json:"custom_id"`
	URL      string             `json:"url"`
	Disabled bool               `json:"disabled"`
	Components []MessageComponent `json:"components,omitempty"`
}

// DiscordMessage represents a message fetched from a channel or search endpoint.
type DiscordMessage struct {
	ID          string             `json:"id"`
	ChannelID   string             `json:"channel_id"`
	GuildID     string             `json:"guild_id"`
	Author      User               `json:"author"`
	Content     string             `json:"content"`
	Timestamp   time.Time          `json:"timestamp"`
	Components  []MessageComponent `json:"components,omitempty"`
	Reactions   []MessageReaction  `json:"reactions,omitempty"`
}

// JoinResponse represents the response when joining a guild via invite.
type JoinResponse struct {
	Guild               *GuildSummary `json:"guild"`
	Code                string        `json:"code"`
	CaptchaKey          []string      `json:"captcha_key,omitempty"`
	CaptchaSiteKey      string        `json:"captcha_sitekey,omitempty"`
	CaptchaService      string        `json:"captcha_service,omitempty"`
	CaptchaRqData       string        `json:"captcha_rqdata,omitempty"`
	CaptchaRqToken      string        `json:"captcha_rqtoken,omitempty"`
	Message             string        `json:"message,omitempty"`
}
