package utils

import "strings"

// Ghép phần ghi chú từ mảng chuỗi
func QuoteJoin(parts []string, start int) string {
	if start >= len(parts) {
		return ""
	}
	return strings.TrimSpace(strings.Join(parts[start:], " "))
}
