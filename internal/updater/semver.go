package updater

import (
	"strconv"
	"strings"
)

type semver struct {
	major, minor, patch int
	suffix              string // git describe tail on a local build, e.g. "-3-gabc123-dirty"
}

// A `git describe` build keeps its release base, so it still updates once a newer release appears.
func parseSemver(s string) (semver, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")

	core, suffix := s, ""
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		core, suffix = s[:i], s[i:]
	}

	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return semver{}, false
	}

	var nums [3]int
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return semver{}, false
		}
		nums[i] = n
	}

	return semver{major: nums[0], minor: nums[1], patch: nums[2], suffix: suffix}, true
}

// A suffixed local build of the same base counts as equal: it is that release plus edits.
func newer(a, b semver) bool {
	if a.major != b.major {
		return b.major > a.major
	}
	if a.minor != b.minor {
		return b.minor > a.minor
	}
	return b.patch > a.patch
}
