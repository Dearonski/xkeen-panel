package infraupdate

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"xkeen-panel/internal/xkeen"
)

type harness struct {
	u       *Updater
	mu      sync.Mutex
	version string
	geoDir  string
	ran     []string
	run     func(args []string) (string, error)
}

func newHarness(t *testing.T, latest string) *harness {
	t.Helper()
	dir := t.TempDir()
	h := &harness{version: "2.0 Stable", geoDir: filepath.Join(dir, "dat")}
	os.MkdirAll(h.geoDir, 0755)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":%q,"body":"notes","html_url":"https://example/r"}`, latest)
	}))
	t.Cleanup(srv.Close)

	h.u = New(Options{
		Runtime: func() xkeen.Runtime {
			return xkeen.Runtime{Installed: true, Generation: 2, Dispatcher: "/opt/sbin/xkeen", Version: h.installed()}
		},
		GeoDir:  h.geoDir,
		Crontab: filepath.Join(dir, "crontab"),
		APIBase: srv.URL,
		Run: func(_ string, args ...string) (string, error) {
			h.mu.Lock()
			h.ran = append(h.ran, strings.Join(args, " "))
			run := h.run
			h.mu.Unlock()
			return run(args)
		},
	})
	return h
}

func (h *harness) installed() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.version
}

func (h *harness) install(version string) {
	h.mu.Lock()
	h.version = version
	h.mu.Unlock()
}

func (h *harness) wait(t *testing.T) State {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s := h.u.State(); !s.XKeen.Updating && !s.Geo.Updating {
			return s
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("update did not finish")
	return State{}
}

func (h *harness) writeGeo(t *testing.T, name string, at time.Time) {
	t.Helper()
	path := filepath.Join(h.geoDir, name)
	if err := os.WriteFile(path, make([]byte, 4096), 0644); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(path, at, at)
}

func TestCheckOffersNewerXKeen(t *testing.T) {
	h := newHarness(t, "2.1")

	state, err := h.u.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !state.XKeen.Available || state.XKeen.Current != "2.0" || state.XKeen.Latest.Tag != "2.1" {
		t.Errorf("state = %+v, want 2.1 offered over 2.0", state.XKeen)
	}

	h.install("2.1 Stable")
	if h.u.State().XKeen.Available {
		t.Error("the installed version is still offered as an update")
	}
}

func TestXKeenUpdateRunsDispatcher(t *testing.T) {
	h := newHarness(t, "2.1")
	h.run = func([]string) (string, error) {
		h.install("2.1 Stable")
		return "Обновление XKeen выполнено", nil
	}

	if err := h.u.Apply(ItemXKeen); err == nil {
		t.Fatal("an update started without a check of what is available")
	}
	if _, err := h.u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := h.u.Apply(ItemXKeen); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	state := h.wait(t)
	if len(h.ran) != 1 || h.ran[0] != "-uk" {
		t.Errorf("ran %v, want xkeen -uk once", h.ran)
	}
	if state.XKeen.ApplyError != "" || state.XKeen.Current != "2.1" || state.XKeen.Available {
		t.Errorf("state = %+v, want a clean update to 2.1", state.XKeen)
	}
}

func TestXKeenUpdateThatChangedNothingIsAnError(t *testing.T) {
	h := newHarness(t, "2.1")
	h.run = func([]string) (string, error) {
		return "Отсутствует интернет-соединение", nil
	}

	h.u.Check(context.Background())
	if err := h.u.Apply(ItemXKeen); err != nil {
		t.Fatal(err)
	}

	if got := h.wait(t).XKeen.ApplyError; !strings.Contains(got, "интернет-соединение") {
		t.Errorf("apply error = %q, want what xkeen printed: its exit code is zero either way", got)
	}
}

func TestGeoUpdate(t *testing.T) {
	h := newHarness(t, "2.0")
	old := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	h.writeGeo(t, "geoip_v2fly.dat", old)
	h.writeGeo(t, "geosite_v2fly.dat", old)
	os.WriteFile(filepath.Join(h.geoDir, "zkeen.dat"), []byte("stub"), 0644)

	if s := h.u.State().Geo; !s.Supported || s.Files != 2 || !s.UpdatedAt.Equal(old) {
		t.Fatalf("geo = %+v, want two real bases dated %s", s, old)
	}

	h.run = func([]string) (string, error) { return "Не обнаружены базы", nil }
	if err := h.u.Apply(ItemGeo); err != nil {
		t.Fatal(err)
	}
	if h.wait(t).Geo.ApplyError == "" {
		t.Error("untouched files passed as an update")
	}

	h.run = func([]string) (string, error) {
		h.writeGeo(t, "geoip_v2fly.dat", time.Now())
		return "завершено", nil
	}
	if err := h.u.Apply(ItemGeo); err != nil {
		t.Fatal(err)
	}
	if s := h.wait(t).Geo; s.ApplyError != "" || !s.UpdatedAt.After(old) {
		t.Errorf("geo = %+v, want fresh files and no error", s)
	}
	if h.ran[len(h.ran)-1] != "-ug" {
		t.Errorf("ran %v, want xkeen -ug", h.ran)
	}
}

func TestUnsupportedBuilds(t *testing.T) {
	h := newHarness(t, "2.1")
	h.install("2.1 Dev")
	if s := h.u.State().XKeen; s.Supported || s.Reason == "" {
		t.Errorf("a Dev build is offered for update: %+v", s)
	}
	if err := h.u.Apply("mihomo"); err == nil {
		t.Error("an unknown component was accepted")
	}
}

func TestGeoSchedule(t *testing.T) {
	path := filepath.Join(t.TempDir(), "root")
	for line, want := range map[string]string{
		"40 3 * * 3 /opt/sbin/xkeen -ug":   "по средам в 03:40",
		"5 4 * * * /opt/sbin/xkeen -ug":    "каждый день в 04:05",
		"0 3 1 * * /opt/sbin/xkeen -ug":    "0 3 1 * *",
		"0 5 * * * /opt/bin/something":     "",
		"# 40 3 * * 3 /opt/sbin/xkeen -ug": "",
	} {
		os.WriteFile(path, []byte(line+"\n"), 0644)
		if got := geoSchedule(path); got != want {
			t.Errorf("%q → %q, want %q", line, got, want)
		}
	}
}

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		cur, latest string
		want        bool
	}{
		{"2.0", "2.1", true}, {"2.1", "2.1", false}, {"2.1", "2.0", false}, {"2.0", "2.0.1", true}, {"2.9", "2.10", true}, {"2.0", "nightly", true}, {"", "2.1", false},
	} {
		if got := newer(c.cur, c.latest); got != c.want {
			t.Errorf("newer(%q, %q) = %v", c.cur, c.latest, got)
		}
	}
}
