package disboard

import (
	"time"
)

// DiscoveredServer represents a server found via Disboard scraping or direct invite list.
type DiscoveredServer struct {
	Tag                    string    `json:"tag"`
	InviteCode             string    `json:"invite_code"`
	GuildID                string    `json:"guild_id,omitempty"`
	GuildName              string    `json:"guild_name"`
	ApproximateMemberCount int       `json:"approximate_member_count"`
	DiscoveredAt           time.Time `json:"discovered_at"`
	Source                 string    `json:"source"` // "disboard" | "direct_input"
}
