package xkeen

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

const tiktokPattern = `"region":"([A-Z]{2})"`

func countryServer(t *testing.T, status int, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestNormalizeCountryPattern(t *testing.T) {
	if got, err := NormalizeCountryPattern("  "); err != nil || got != "" {
		t.Fatalf("empty pattern: got %q, %v", got, err)
	}
	if got, err := NormalizeCountryPattern(" " + tiktokPattern + " "); err != nil || got != tiktokPattern {
		t.Fatalf("valid pattern: got %q, %v", got, err)
	}
	for _, bad := range []string{`"region":"[A-Z]{2}"`, `"region":"(`, strings.Repeat("a", 201) + "()"} {
		if _, err := NormalizeCountryPattern(bad); err == nil {
			t.Errorf("want an error for %q", bad)
		}
	}
}

func TestOpenThroughFindsCountryPastFirstKilobytes(t *testing.T) {
	body := strings.Repeat("x", 200<<10) + `{"region":"eu-ttp2"},{"region":"FI"}`
	url := countryServer(t, http.StatusOK, body)

	got := openThrough(nil, url, regexp.MustCompile(tiktokPattern), 5*time.Second)
	if !got.OK || got.ExitCountry != "FI" {
		t.Fatalf("got %+v, want OK with FI", got)
	}
}

func TestOpenThroughFailsWithoutCountry(t *testing.T) {
	url := countryServer(t, http.StatusOK, "<html>captcha</html>")

	got := openThrough(nil, url, regexp.MustCompile(tiktokPattern), 5*time.Second)
	if got.OK || got.Error == "" {
		t.Fatalf("got %+v, want a failure naming the missing country", got)
	}
}

func TestOpenThroughKeepsStatusFailure(t *testing.T) {
	url := countryServer(t, http.StatusForbidden, `"region":"FI"`)

	got := openThrough(nil, url, regexp.MustCompile(tiktokPattern), 5*time.Second)
	if got.OK || got.Status != http.StatusForbidden || got.ExitCountry != "" {
		t.Fatalf("got %+v, want the 403 to stand", got)
	}
}

func TestMarkAvoidedCountries(t *testing.T) {
	results := []SiteResult{
		{Node: "a", OK: true, ExitCountry: "RU"},
		{Node: "b", OK: true, ExitCountry: "FI"},
		{Node: "c", OK: true},
		{Node: "d", OK: false, Status: 403, ExitCountry: "RU"},
	}

	markAvoidedCountries(results, []string{"ru"})

	if results[0].OK || results[0].Error != "сайт видит RU" {
		t.Errorf("RU exit: got %+v", results[0])
	}
	if !results[1].OK || !results[2].OK {
		t.Errorf("clean and unknown exits must pass: %+v %+v", results[1], results[2])
	}
	if results[3].Error != "" {
		t.Errorf("a failed check keeps its own reason: %+v", results[3])
	}
}
