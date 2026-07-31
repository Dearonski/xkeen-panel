package xkeen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStripJSONComments(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"line comment", "{\n// hi\n\"a\": 1\n}", "{\n\n\"a\": 1\n}"},
		{"trailing comment", "{\"a\": 1 // hi\n}", "{\"a\": 1 \n}"},
		{"block comment", "{/* hi */\"a\": 1}", "{\"a\": 1}"},
		{"url inside string", `{"u": "https://e.com"}`, `{"u": "https://e.com"}`},
		{"comment marker inside string", `{"u": "a // b"}`, `{"u": "a // b"}`},
		{"escaped quote before marker", `{"u": "a\" // b"}`, `{"u": "a\" // b"}`},
		{"escaped backslash ends string", `{"u": "a\\", "v": 1 // c` + "\n}", `{"u": "a\\", "v": 1 ` + "\n}"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := string(StripJSONComments([]byte(c.in))); got != c.want {
				t.Errorf("StripJSONComments(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// Live 05_routing.json from the router: leading comment plus inline comments
// after regexp strings containing escapes.
func TestStripJSONCommentsLiveRouting(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "routing_single.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	if !strings.HasPrefix(string(data), "//") {
		t.Fatal("fixture must start with a comment")
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err == nil {
		t.Fatal("raw file must not parse without strip — fixture lost its comments")
	}

	if err := json.Unmarshal(StripJSONComments(data), &cfg); err != nil {
		t.Fatalf("does not parse after strip: %v", err)
	}

	routing, ok := cfg["routing"].(map[string]interface{})
	if !ok {
		t.Fatal("routing not found")
	}
	rules, ok := routing["rules"].([]interface{})
	if !ok || len(rules) == 0 {
		t.Fatal("rules not found")
	}
}
