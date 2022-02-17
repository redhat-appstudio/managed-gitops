package db

import (
	"strings"
	"unicode/utf8"
)

// These values should be equiv. with their related VARCHAR values from 'db-schema.sql' respectively
const (
	ApplicationstateHealthLength     = 30
	ApplicationstateMessageLength    = 1024
	ApplicationstateRevisionLength   = 1024
	ApplicationstateSyncstatusLength = 30
)

// TruncateVarchar converts string to "str..." if chars is > maxLength
// returns a relative number of dots '.' string if maxLength <= 3
// returns empty string if maxLength < 0 or if string is not UTF-8 encoded
// Notice: This is based on characters -- not bytes (default VARCHAR behavior)
func TruncateVarchar(s string, maxLength int) string {
	if maxLength <= 3 && maxLength >= 0 {
		return strings.Repeat(".", maxLength)
	}

	if maxLength < 0 || !utf8.ValidString(s) {
		return ""
	}

	var wb []string
	wb = strings.Split(s, "")

	if maxLength < len(wb) {
		maxLength = maxLength - 3
		return strings.Join(wb[:maxLength], "") + "..."
	}

	return s
}
