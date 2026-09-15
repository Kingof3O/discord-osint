package disboard

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExtractInviteCode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://discord.gg/gaminghub", "gaminghub"},
		{"http://discord.gg/alpha-beta", "alpha-beta"},
		{"discord.gg/code123", "code123"},
		{"https://discord.com/invite/crypto-pros?param=1", "crypto-pros"},
		{"https://disboard.org/join/123456789", "123456789"},
		{"/join/999888777", "999888777"},
		{"   plaincode   ", "plaincode"},
		{"", ""},
	}

	for _, tt := range tests {
		got := ExtractInviteCode(tt.input)
		if got != tt.expected {
			t.Errorf("ExtractInviteCode(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestLoadInvitesFromFile(t *testing.T) {
	content := `
# A sample list of Discord invites
https://discord.gg/server-one
discord.gg/server-two

# Duplicate should be ignored
https://discord.gg/server-one

server-three
`
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "invites.txt")
	if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	servers, err := LoadInvitesFromFile(filePath, "test-tag")
	if err != nil {
		t.Fatalf("LoadInvitesFromFile failed: %v", err)
	}

	if len(servers) != 3 {
		t.Fatalf("expected 3 deduplicated servers, got %d", len(servers))
	}

	expectedCodes := []string{"server-one", "server-two", "server-three"}
	for i, exp := range expectedCodes {
		if servers[i].InviteCode != exp {
			t.Errorf("servers[%d].InviteCode = %q, expected %q", i, servers[i].InviteCode, exp)
		}
		if servers[i].Tag != "test-tag" {
			t.Errorf("expected tag test-tag, got %q", servers[i].Tag)
		}
		if servers[i].Source != "direct_input" {
			t.Errorf("expected source direct_input, got %q", servers[i].Source)
		}
	}
}

const mockDisboardHTML = `
<!DOCTYPE html>
<html>
<body>
<div class="server-card" data-id="111222333">
    <div class="server-name">Gaming Legends</div>
    <div class="server-members">12,450 Members</div>
    <a class="server-join" href="https://discord.gg/gaming-legends">Join</a>
</div>
<div class="server-card" data-id="444555666">
    <div class="server-name">Crypto Alpha Club</div>
    <div class="server-members">3,200 Online</div>
    <a class="server-join" href="/join/444555666">Join</a>
</div>
</body>
</html>
`

func TestParseHTML_DisboardFixture(t *testing.T) {
	s := NewScraper(nil, "")
	servers, err := s.ParseHTML(strings.NewReader(mockDisboardHTML), "gaming")
	if err != nil {
		t.Fatalf("ParseHTML failed: %v", err)
	}

	if len(servers) != 2 {
		t.Fatalf("expected 2 servers parsed, got %d", len(servers))
	}

	// Server 1
	if servers[0].GuildName != "Gaming Legends" {
		t.Errorf("expected name 'Gaming Legends', got %q", servers[0].GuildName)
	}
	if servers[0].InviteCode != "gaming-legends" {
		t.Errorf("expected invite 'gaming-legends', got %q", servers[0].InviteCode)
	}
	if servers[0].ApproximateMemberCount != 12450 {
		t.Errorf("expected 12450 members, got %d", servers[0].ApproximateMemberCount)
	}
	if servers[0].GuildID != "111222333" {
		t.Errorf("expected guild ID 111222333, got %q", servers[0].GuildID)
	}

	// Server 2
	if servers[1].GuildName != "Crypto Alpha Club" {
		t.Errorf("expected name 'Crypto Alpha Club', got %q", servers[1].GuildName)
	}
	if servers[1].InviteCode != "444555666" {
		t.Errorf("expected invite '444555666', got %q", servers[1].InviteCode)
	}
}

func TestScraper_DiscoverWithMockServer(t *testing.T) {
	page1 := `
	<div class="server-card" data-id="101"><div class="server-name">Server 1</div><a href="/join/code-1">Join</a></div>
	<div class="server-card" data-id="102"><div class="server-name">Server 2</div><a href="/join/code-2">Join</a></div>
	`
	page2 := `
	<div class="server-card" data-id="103"><div class="server-name">Server 3</div><a href="/join/code-3">Join</a></div>
	<div class="server-card" data-id="104"><div class="server-name">Server 4</div><a href="/join/code-4">Join</a></div>
	`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		page := query.Get("page")
		w.Header().Set("Content-Type", "text/html")
		if page == "1" || page == "" {
			fmt.Fprint(w, page1)
		} else if page == "2" {
			fmt.Fprint(w, page2)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	scraper := NewScraper(ts.Client(), "")
	scraper.BaseURL = ts.URL
	scraper.RequestDelay = 10 * time.Millisecond // fast delay for test

	// Request limit = 3
	results, err := scraper.Discover(context.Background(), "gaming", 3)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 servers returned (bounded by limit), got %d", len(results))
	}
	if results[0].InviteCode != "code-1" || results[1].InviteCode != "code-2" || results[2].InviteCode != "code-3" {
		t.Errorf("unexpected results: %+v", results)
	}
}
