package discord

import (
	"encoding/base64"
	"encoding/json"
)

// ClientSuperProperties represents the client metadata sent by genuine Discord clients.
type ClientSuperProperties struct {
	OS                string `json:"os"`
	Browser           string `json:"browser"`
	ReleaseChannel    string `json:"release_channel"`
	ClientVersion     string `json:"client_version"`
	OSVersion         string `json:"os_version"`
	OSArch            string `json:"os_arch"`
	AppArch           string `json:"app_arch"`
	SystemLocale      string `json:"system_locale"`
	BrowserUserAgent  string `json:"browser_user_agent"`
	BrowserVersion    string `json:"browser_version"`
	ClientBuildNumber int    `json:"client_build_number"`
}

// DefaultSuperProperties returns realistic Discord Desktop client properties.
func DefaultSuperProperties() ClientSuperProperties {
	return ClientSuperProperties{
		OS:                "Mac OS X",
		Browser:           "Discord Client",
		ReleaseChannel:    "stable",
		ClientVersion:     "1.0.9168",
		OSVersion:         "23.6.0",
		OSArch:            "arm64",
		AppArch:           "arm64",
		SystemLocale:      "en-US",
		BrowserUserAgent:  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) discord/1.0.9168 Chrome/128.0.6613.186 Electron/32.2.5 Safari/537.36",
		BrowserVersion:    "32.2.5",
		ClientBuildNumber: 331456,
	}
}

// BuildSuperPropertiesHeader encodes super properties into base64 for the X-Super-Properties header.
func BuildSuperPropertiesHeader() string {
	props := DefaultSuperProperties()
	data, _ := json.Marshal(props)
	return base64.StdEncoding.EncodeToString(data)
}

// BuildJoinContextProperties generates the base64 X-Context-Properties header expected by Discord on joins.
func BuildJoinContextProperties() string {
	ctxProps := map[string]any{
		"location":              "Join Guild",
		"location_guild_id":     nil,
		"location_channel_id":   nil,
		"location_channel_type": nil,
	}
	data, _ := json.Marshal(ctxProps)
	return base64.StdEncoding.EncodeToString(data)
}
