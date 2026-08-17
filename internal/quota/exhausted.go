package quota

import "strings"

// IsQuotaExhausted detects vendor usage/quota limit messages in CLI output.
func IsQuotaExhausted(text string) bool {
	low := strings.ToLower(text)
	if low == "" {
		return false
	}
	if strings.Contains(low, "usage limit") || strings.Contains(low, "usage limits") {
		return true
	}
	if strings.Contains(low, "hit your usage") || (strings.Contains(low, "reached your") && strings.Contains(low, "limit")) {
		return true
	}
	if strings.Contains(low, "quota") && (strings.Contains(low, "exceed") || strings.Contains(low, "exhaust") || strings.Contains(low, "limit")) {
		return true
	}
	if strings.Contains(low, "rate limit") && (strings.Contains(low, "exceed") || strings.Contains(low, "too many")) {
		return true
	}
	return false
}
