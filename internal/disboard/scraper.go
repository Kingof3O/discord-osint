package disboard

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

var digitRegex = regexp.MustCompile(`[0-9]+(?:,[0-9]+)*`)

// Scraper coordinates HTML fetching and parsing from Disboard.
type Scraper struct {
	HTTPClient   *http.Client
	BaseURL      string
	Cookies      string
	UserAgent    string
	RequestDelay time.Duration
}

// NewScraper initializes a Disboard scraper.
func NewScraper(client *http.Client, cookies string) *Scraper {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &Scraper{
		HTTPClient:   client,
		BaseURL:      "https://disboard.org",
		Cookies:      cookies,
		UserAgent:    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36",
		RequestDelay: 3 * time.Second,
	}
}

// Discover scrapes Disboard servers tagged with tag up to the requested limit.
func (s *Scraper) Discover(ctx context.Context, tag string, limit int) ([]DiscoveredServer, error) {
	if limit <= 0 {
		limit = 30
	}

	var results []DiscoveredServer
	seenCodes := make(map[string]bool)
	page := 1

	for len(results) < limit {
		pageURL := fmt.Sprintf("%s/servers/tag/%s?page=%d", s.BaseURL, url.PathEscape(tag), page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
		if err != nil {
			return results, fmt.Errorf("failed to create disboard request: %w", err)
		}

		req.Header.Set("User-Agent", s.UserAgent)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		if s.Cookies != "" {
			req.Header.Set("Cookie", s.Cookies)
		}

		resp, err := s.HTTPClient.Do(req)
		if err != nil {
			return results, fmt.Errorf("disboard request failed for page %d: %w", page, err)
		}

		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == 403 {
			resp.Body.Close()
			return results, fmt.Errorf("disboard blocked request with Cloudflare 403. Tip: pass 'disboard_cookies' with cf_clearance in config or use '--invites-file'")
		}
		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			break // Reached end of pagination
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return results, fmt.Errorf("disboard returned HTTP %d: %s", resp.StatusCode, string(body))
		}

		servers, err := s.ParseHTML(resp.Body, tag)
		resp.Body.Close()
		if err != nil {
			return results, fmt.Errorf("failed to parse disboard page %d: %w", page, err)
		}

		if len(servers) == 0 {
			break // No more servers found on this page
		}

		newFoundOnPage := 0
		for _, srv := range servers {
			if srv.InviteCode == "" || seenCodes[srv.InviteCode] {
				continue
			}
			seenCodes[srv.InviteCode] = true
			results = append(results, srv)
			newFoundOnPage++
			if len(results) >= limit {
				break
			}
		}

		if newFoundOnPage == 0 {
			break // No new unique servers on page
		}

		page++

		// Apply polite delay between pages
		if len(results) < limit && s.RequestDelay > 0 {
			select {
			case <-ctx.Done():
				return results, ctx.Err()
			case <-time.After(s.RequestDelay):
			}
		}
	}

	return results, nil
}

// ParseHTML extracts DiscoveredServer entries from Disboard HTML body.
func (s *Scraper) ParseHTML(r io.Reader, tag string) ([]DiscoveredServer, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, err
	}

	var servers []DiscoveredServer
	now := time.Now().UTC()

	// Select server card containers: .server-card, .column, or .server-info
	doc.Find(".server-card, .column .server-info, .listing-card").Each(func(_ int, card *goquery.Selection) {
		// Extract server name
		name := strings.TrimSpace(card.Find(".server-name, .server-title, .title, a[href*='/server/']").First().Text())
		if name == "" {
			name = "Unknown Disboard Server"
		}

		// Extract server ID if present
		serverID, _ := card.Attr("data-id")
		if serverID == "" {
			href, exists := card.Find("a[href*='/server/']").Attr("href")
			if exists {
				parts := strings.Split(strings.Trim(href, "/"), "/")
				if len(parts) >= 2 && parts[0] == "server" {
					serverID = parts[1]
				}
			}
		}

		// Extract invite code or join URL
		var inviteCode string
		joinHref, exists := card.Find("a[href*='/join/'], a[href*='discord.gg'], a.server-join").Attr("href")
		if exists {
			inviteCode = ExtractInviteCode(joinHref)
		}
		if inviteCode == "" && serverID != "" {
			// In Disboard, /join/{serverID} often redirects to the invite
			inviteCode = serverID
		}

		// Extract member count
		memberCount := 0
		memberText := card.Find(".server-members, .members, .member-count").Text()
		if match := digitRegex.FindString(memberText); match != "" {
			cleanNum := strings.ReplaceAll(match, ",", "")
			if n, err := strconv.Atoi(cleanNum); err == nil {
				memberCount = n
			}
		}

		if inviteCode != "" {
			servers = append(servers, DiscoveredServer{
				Tag:                    tag,
				InviteCode:             inviteCode,
				GuildID:                serverID,
				GuildName:              name,
				ApproximateMemberCount: memberCount,
				DiscoveredAt:           now,
				Source:                 "disboard",
			})
		}
	})

	return servers, nil
}
