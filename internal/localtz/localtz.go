package localtz

import (
	"bytes"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Keenetic links /etc/localtime to /var/TZ, a POSIX rule like "MSK-3" that Go cannot load, so the panel ran in UTC.
func Apply(path string) (string, bool) {
	if os.Getenv("TZ") != "" {
		return "", false
	}
	if name, offset := time.Now().Zone(); name != "UTC" || offset != 0 {
		return "", false
	}

	data, err := os.ReadFile(path)
	if err != nil || bytes.HasPrefix(data, []byte("TZif")) {
		return "", false
	}

	rule, _, _ := strings.Cut(string(data), "\n")
	loc, ok := Parse(strings.TrimSpace(rule))
	if !ok {
		return "", false
	}

	time.Local = loc
	return strings.TrimSpace(rule), true
}

var stdRule = regexp.MustCompile(`^([A-Za-z]{3,}|<[+\-0-9A-Za-z]+>)([+-]?)(\d{1,2})(?::(\d{2}))?(?::(\d{2}))?`)

// Only the standard-time part is used: a DST rule would need the full POSIX transition grammar.
func Parse(rule string) (*time.Location, bool) {
	m := stdRule.FindStringSubmatch(rule)
	if m == nil {
		return nil, false
	}

	hours, _ := strconv.Atoi(m[3])
	minutes, _ := strconv.Atoi(m[4])
	seconds, _ := strconv.Atoi(m[5])
	if hours > 24 || minutes > 59 || seconds > 59 {
		return nil, false
	}

	// POSIX offsets count west of Greenwich, so "MSK-3" is UTC+3
	offset := hours*3600 + minutes*60 + seconds
	if m[2] != "-" {
		offset = -offset
	}

	return time.FixedZone(strings.Trim(m[1], "<>"), offset), true
}
