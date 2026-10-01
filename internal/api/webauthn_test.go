package api

import "testing"

func TestKeenDNSOrigins(t *testing.T) {
	o := keenDNSOrigins("xkeen.example.link")

	if len(o) != 6 {
		t.Fatalf("got %d origins, want 6: %v", len(o), o)
	}

	got := map[string]bool{}
	for _, x := range o {
		got[x] = true
	}
	for _, want := range []string{
		"https://xkeen.example.link",
		"https://xkeen.example.link:8443",
		"https://xkeen.example.link:5443",
	} {
		if !got[want] {
			t.Errorf("origin %q missing from %v", want, o)
		}
	}
}
