package localtz

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	cases := []struct {
		rule   string
		name   string
		offset int
		ok     bool
	}{
		{"MSK-3", "MSK", 3 * 3600, true},
		{"UTC0", "UTC", 0, true},
		{"<+05>-5", "+05", 5 * 3600, true},
		{"IST-5:30", "IST", 5*3600 + 30*60, true},
		{"EST5EDT,M3.2.0,M11.1.0", "EST", -5 * 3600, true},
		{"CET-1CEST,M3.5.0,M10.5.0/3", "CET", 3600, true},
		{"", "", 0, false},
		{"garbage", "", 0, false},
		{"MSK-99", "", 0, false},
	}

	for _, c := range cases {
		loc, ok := Parse(c.rule)
		if ok != c.ok {
			t.Errorf("Parse(%q) ok = %v, want %v", c.rule, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		name, offset := timeIn(loc)
		if name != c.name || offset != c.offset {
			t.Errorf("Parse(%q) = %s %+d, want %s %+d", c.rule, name, offset, c.name, c.offset)
		}
	}
}

func timeIn(loc *time.Location) (string, int) {
	return time.Date(2026, 10, 1, 12, 0, 0, 0, loc).Zone()
}

// Runs in a child process: time.Local is fixed once per process, and the router starts in UTC.
func TestApplyReadsPOSIXRule(t *testing.T) {
	if os.Getenv("LOCALTZ_CHILD") == "1" {
		rule, ok := Apply(os.Getenv("LOCALTZ_FILE"))
		name, offset := time.Now().Zone()
		fmt.Printf("result: %s %v %s %d\n", rule, ok, name, offset)
		return
	}

	cases := map[string]string{
		"MSK-3\n":                  "result: MSK-3 true MSK 10800",
		"TZif2 binary zoneinfo...": "result:  false UTC 0",
		"not a rule":               "result:  false UTC 0",
	}

	for content, want := range cases {
		path := filepath.Join(t.TempDir(), "TZ")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		cmd := exec.Command(os.Args[0], "-test.run=^TestApplyReadsPOSIXRule$")
		cmd.Env = append(os.Environ(), "TZ=", "LOCALTZ_CHILD=1", "LOCALTZ_FILE="+path)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("child: %v\n%s", err, out)
		}
		if !strings.Contains(string(out), want) {
			t.Errorf("file %q: got %q, want %q", content, out, want)
		}
	}
}
