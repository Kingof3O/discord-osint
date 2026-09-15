package target

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const discordEpoch = 1420070400000 // 2015-01-01T00:00:00.000Z in milliseconds

// NormalizeUsername strips leading '@', trims spaces, and applies NFKC normalization and case-folding.
func NormalizeUsername(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "@")
	s = norm.NFKC.String(s)
	return strings.ToLower(s)
}

// NormalizeDisplayName normalizes display names/nicknames for fuzzy comparison.
func NormalizeDisplayName(s string) string {
	s = strings.TrimSpace(s)
	s = norm.NFKC.String(s)
	// Collapse multiple spaces into one
	var b strings.Builder
	lastWasSpace := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsSpace(r) {
			if !lastWasSpace {
				b.WriteRune(' ')
				lastWasSpace = true
			}
		} else {
			b.WriteRune(r)
			lastWasSpace = false
		}
	}
	return b.String()
}

// IsValidSnowflake checks if a string is a 17-20 digit integer representing a Discord Snowflake ID.
func IsValidSnowflake(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 17 || len(s) > 20 {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	id, err := strconv.ParseUint(s, 10, 64)
	return err == nil && id > 0
}

// SnowflakeToTime extracts the creation timestamp from a Discord Snowflake ID.
func SnowflakeToTime(snowflake string) (time.Time, error) {
	s := strings.TrimSpace(snowflake)
	id, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid snowflake %q: %w", snowflake, err)
	}

	ms := int64((id >> 22) + discordEpoch)
	return time.UnixMilli(ms).UTC(), nil
}
