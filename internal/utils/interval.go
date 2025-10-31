package utils

import (
	"errors"
	"strconv"
	"strings"
)

// ParseIntervalToSeconds chuyển "30s", "10m", "1h", "1d" → số giây
func ParseIntervalToSeconds(s string) (int, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, errors.New("empty interval")
	}
	unit := s[len(s)-1]
	num := s
	mult := 1
	switch unit {
	case 's':
		num = s[:len(s)-1]
		mult = 1
	case 'm':
		num = s[:len(s)-1]
		mult = 60
	case 'h':
		num = s[:len(s)-1]
		mult = 3600
	case 'd':
		num = s[:len(s)-1]
		mult = 86400
	default:
		num = s
		mult = 1
	}
	val, err := strconv.ParseFloat(num, 64)
	if err != nil || val <= 0 {
		return 0, errors.New("invalid interval")
	}
	return int(val * float64(mult)), nil
}
