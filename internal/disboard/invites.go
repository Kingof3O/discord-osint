package disboard

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

// ExtractInviteCode cleans and extracts the canonical invite code from a URL or raw string.
func ExtractInviteCode(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	// Try URL parsing
	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err == nil && parsed.Path != "" {
			parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
			if len(parts) > 0 {
				code := parts[len(parts)-1]
				if code != "" && code != "invite" {
					return code
				}
			}
		}
	}

	// Pattern cleanup
	prefixes := []string{
		"https://discord.gg/",
		"http://discord.gg/",
		"discord.gg/",
		"https://discord.com/invite/",
		"http://discord.com/invite/",
		"discord.com/invite/",
		"https://disboard.org/join/",
		"http://disboard.org/join/",
		"/join/",
	}

	for _, p := range prefixes {
		if strings.HasPrefix(strings.ToLower(raw), p) {
			raw = raw[len(p):]
			break
		}
	}

	// Strip trailing slashes, query params, hashes
	if idx := strings.IndexAny(raw, "?#/ \t"); idx != -1 {
		raw = raw[:idx]
	}

	return strings.TrimSpace(raw)
}

// LoadInvitesFromFile reads a text file containing invite codes or URLs, one per line.
func LoadInvitesFromFile(path string, tag string) ([]DiscoveredServer, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open invites file: %w", err)
	}
	defer file.Close()

	var servers []DiscoveredServer
	seen := make(map[string]bool)
	now := time.Now().UTC()

	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		code := ExtractInviteCode(line)
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true

		servers = append(servers, DiscoveredServer{
			Tag:          tag,
			InviteCode:   code,
			GuildName:    fmt.Sprintf("Direct Server (%s)", code),
			DiscoveredAt: now,
			Source:       "direct_input",
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading invites file: %w", err)
	}

	return servers, nil
}

// ParseDirectInvites converts slice of raw invite arguments into deduplicated DiscoveredServer models.
func ParseDirectInvites(inputs []string, tag string) []DiscoveredServer {
	var servers []DiscoveredServer
	seen := make(map[string]bool)
	now := time.Now().UTC()

	for _, input := range inputs {
		code := ExtractInviteCode(input)
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true

		servers = append(servers, DiscoveredServer{
			Tag:          tag,
			InviteCode:   code,
			GuildName:    fmt.Sprintf("Direct Server (%s)", code),
			DiscoveredAt: now,
			Source:       "direct_input",
		})
	}

	return servers
}
