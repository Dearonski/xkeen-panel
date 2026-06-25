package xkeen

import "testing"

func TestDetectCountry(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"🇳🇱 Amsterdam #1", "NL"},
		{"🇷🇺 Москва", "RU"},
		{"RU-MSK-01", "RU"},
		{"Germany Frankfurt", "DE"},
		{"vless-nl-fast", "NL"},
		{"Нидерланды-02", "NL"},
		{"Belarus Minsk", "BY"},
		{"node-x7zق", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := detectCountry(c.name); got != c.want {
			t.Errorf("detectCountry(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}
