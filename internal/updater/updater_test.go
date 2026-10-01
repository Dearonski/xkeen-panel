package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testAsset = "xkeen-panel-aarch64"

// fakeRelease serves a GitHub-shaped latest release whose "binary" is a shell
// script answering -version, which is all the smoke test needs.
type fakeRelease struct {
	tag      string
	reported string // what the binary prints for -version
	sums     string // SHA256SUMS body; "" = computed, "-" = no file in the release
}

func (f fakeRelease) binary() []byte {
	reported := f.reported
	if reported == "" {
		reported = f.tag
	}
	return []byte("#!/bin/sh\necho " + reported + "\n")
}

func (f fakeRelease) serve(t *testing.T) *httptest.Server {
	t.Helper()

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			assets := fmt.Sprintf(`{"name":%q,"browser_download_url":%q}`, testAsset, srv.URL+"/dl/"+testAsset)
			if f.sums != "-" {
				assets += fmt.Sprintf(`,{"name":"SHA256SUMS","browser_download_url":%q}`, srv.URL+"/dl/SHA256SUMS")
			}
			fmt.Fprintf(w, `{"tag_name":%q,"body":"notes","html_url":"https://example/rel","assets":[%s]}`, f.tag, assets)
		case "/dl/" + testAsset:
			w.Write(f.binary())
		case "/dl/SHA256SUMS":
			sums := f.sums
			if sums == "" {
				sum := sha256.Sum256(f.binary())
				sums = hex.EncodeToString(sum[:]) + "  " + testAsset + "\nffff  xkeen-panel-mipsel\n"
			}
			fmt.Fprint(w, sums)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

type harness struct {
	u        *Updater
	bin      string
	dataDir  string
	restarts atomic.Int32
}

func newHarness(t *testing.T, current string, rel fakeRelease) *harness {
	t.Helper()
	dir := t.TempDir()
	h := &harness{bin: filepath.Join(dir, "xkeen-panel"), dataDir: filepath.Join(dir, "data")}

	if err := os.WriteFile(h.bin, []byte("old binary"), 0755); err != nil {
		t.Fatal(err)
	}

	srv := rel.serve(t)
	h.u = New(Options{
		Current:  current,
		BinPath:  h.bin,
		DataDir:  h.dataDir,
		AutoHour: 4,
		Repo:     "owner/repo",
		APIBase:  srv.URL,
		Client:   srv.Client(),
		Restart: func() error {
			h.restarts.Add(1)
			return nil
		},
	})
	h.u.asset = testAsset
	// Short enough not to outlive the test run; by then the temp dir is gone and the guard exits
	h.u.guardDelay = 3 * time.Second
	return h
}

func (h *harness) binContent(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(h.bin)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestParseSemver(t *testing.T) {
	cases := []struct {
		in   string
		ok   bool
		want semver
	}{
		{"v1.2.3", true, semver{major: 1, minor: 2, patch: 3}},
		{"1.10.0", true, semver{major: 1, minor: 10}},
		{"v1.2.0-5-gabc123-dirty", true, semver{major: 1, minor: 2, suffix: "-5-gabc123-dirty"}},
		{"dev", false, semver{}},
		{"v1.2", false, semver{}},
		{"", false, semver{}},
	}
	for _, c := range cases {
		got, ok := parseSemver(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("parseSemver(%q) = %+v, %v; want %+v, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestNewer(t *testing.T) {
	v := func(s string) semver { r, _ := parseSemver(s); return r }

	cases := []struct {
		current, release string
		want             bool
	}{
		{"v1.2.0", "v1.3.0", true},
		{"v1.9.0", "v1.10.0", true},
		{"v1.3.0", "v1.3.0", false},
		{"v1.3.1", "v1.3.0", false},
		{"v2.0.0", "v1.99.99", false},
		{"v1.3.0-4-gabc", "v1.3.0", false},
		{"v1.3.0-4-gabc", "v1.3.1", true},
	}
	for _, c := range cases {
		if got := newer(v(c.current), v(c.release)); got != c.want {
			t.Errorf("newer(%s, %s) = %v, want %v", c.current, c.release, got, c.want)
		}
	}
}

func TestAssetName(t *testing.T) {
	cases := map[[2]string]string{
		{"linux", "arm64"}:  "xkeen-panel-aarch64",
		{"linux", "mipsle"}: "xkeen-panel-mipsel",
		{"linux", "amd64"}:  "",
		{"darwin", "arm64"}: "",
	}
	for in, want := range cases {
		if got := AssetName(in[0], in[1]); got != want {
			t.Errorf("AssetName(%s, %s) = %q, want %q", in[0], in[1], got, want)
		}
	}
}

func TestCheckFindsNewerRelease(t *testing.T) {
	h := newHarness(t, "v1.3.0", fakeRelease{tag: "v1.3.1"})

	state, err := h.u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !state.Available || state.Latest == nil || state.Latest.Tag != "v1.3.1" || state.Latest.Notes != "notes" {
		t.Errorf("state = %+v, want v1.3.1 available with notes", state)
	}
	if !state.Auto {
		t.Error("automatic updates must be on by default")
	}
}

func TestCheckSameVersionIsNotAvailable(t *testing.T) {
	h := newHarness(t, "v1.3.1", fakeRelease{tag: "v1.3.1"})

	state, _ := h.u.Check(context.Background())
	if state.Available {
		t.Error("the running release is offered as an update")
	}
}

func TestDevBuildNeverUpdates(t *testing.T) {
	h := newHarness(t, "dev", fakeRelease{tag: "v9.0.0"})

	state, _ := h.u.Check(context.Background())
	if state.Supported || state.Available {
		t.Errorf("state = %+v, want a dev build unsupported", state)
	}
	if err := h.u.Apply(); err == nil {
		t.Error("a dev build replaced itself")
	}
}

func TestApplyInstallsAndRestarts(t *testing.T) {
	h := newHarness(t, "v1.3.0", fakeRelease{tag: "v1.3.1"})
	if _, err := h.u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}

	if err := h.u.Apply(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "restart", func() bool { return h.restarts.Load() == 1 })

	if got := h.binContent(t); !strings.Contains(got, "echo v1.3.1") {
		t.Errorf("binary = %q, want the release", got)
	}
	if prev, _ := os.ReadFile(h.bin + ".prev"); string(prev) != "old binary" {
		t.Errorf(".prev = %q, want the old binary kept for rollback", prev)
	}
	if _, err := os.Stat(h.u.pendingPath()); err != nil {
		t.Error("no pending marker — the guard could not tell a failed start")
	}
	if got := h.u.State().Previous; got != "v1.3.0" {
		t.Errorf("previous = %q, want v1.3.0 offered for rollback", got)
	}
}

func TestApplyRejectsBadChecksum(t *testing.T) {
	h := newHarness(t, "v1.3.0", fakeRelease{tag: "v1.3.1", sums: strings.Repeat("0", 64) + "  " + testAsset + "\n"})
	h.u.Check(context.Background())

	if err := h.u.Apply(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "failure", func() bool { return h.u.State().ApplyError != "" })

	assertUntouched(t, h)
	if !strings.Contains(h.u.State().ApplyError, "контрольная сумма") {
		t.Errorf("error = %q, want a checksum mismatch", h.u.State().ApplyError)
	}
}

func TestApplyRefusesReleaseWithoutSums(t *testing.T) {
	h := newHarness(t, "v1.3.0", fakeRelease{tag: "v1.3.1", sums: "-"})
	h.u.Check(context.Background())

	h.u.Apply()
	waitFor(t, "failure", func() bool { return h.u.State().ApplyError != "" })

	assertUntouched(t, h)
}

// A binary that runs but is not the release it claims to be — wrong asset,
// stale cache — must not replace a working panel.
func TestApplyRejectsVersionMismatch(t *testing.T) {
	h := newHarness(t, "v1.3.0", fakeRelease{tag: "v1.3.1", reported: "v1.2.0"})
	h.u.Check(context.Background())

	h.u.Apply()
	waitFor(t, "failure", func() bool { return h.u.State().ApplyError != "" })

	assertUntouched(t, h)
}

func assertUntouched(t *testing.T, h *harness) {
	t.Helper()
	if got := h.binContent(t); got != "old binary" {
		t.Errorf("binary = %q, want the old one untouched", got)
	}
	for _, path := range []string{h.bin + ".new", h.bin + ".prev"} {
		if _, err := os.Stat(path); err == nil {
			t.Errorf("%s left behind", filepath.Base(path))
		}
	}
	if h.restarts.Load() != 0 {
		t.Error("panel restarted after a failed install")
	}
	if h.u.State().Updating {
		t.Error("still marked as updating")
	}
}

// The guard is what saves a router whose new binary never comes up: nothing
// else is left running to notice.
func TestGuardRollsBackUnconfirmedUpdate(t *testing.T) {
	h := newHarness(t, "v1.3.0", fakeRelease{tag: "v1.3.1"})
	h.u.guardDelay = time.Second
	h.u.Check(context.Background())

	h.u.Apply()
	waitFor(t, "install", func() bool { return h.restarts.Load() == 1 })
	waitFor(t, "guard rollback", func() bool {
		_, err := os.Stat(h.u.failedPath())
		return err == nil
	})

	if got := h.binContent(t); got != "old binary" {
		t.Errorf("binary = %q, want the old one restored", got)
	}

	// The old version comes back up and must not try the same release again
	old := New(Options{Current: "v1.3.0", BinPath: h.bin, DataDir: h.dataDir, AutoHour: 4})
	old.Startup(context.Background(), func() bool { return true })

	state := old.State()
	if state.FailedStart != "v1.3.1" || state.Skip != "v1.3.1" {
		t.Errorf("state = %+v, want v1.3.1 recorded as failed and skipped", state)
	}
}

func TestGuardLeavesConfirmedUpdate(t *testing.T) {
	h := newHarness(t, "v1.3.0", fakeRelease{tag: "v1.3.1"})
	h.u.guardDelay = time.Second
	h.u.Check(context.Background())

	h.u.Apply()
	waitFor(t, "install", func() bool { return h.restarts.Load() == 1 })

	next := New(Options{Current: "v1.3.1", BinPath: h.bin, DataDir: h.dataDir})
	next.confirmAfter = 10 * time.Millisecond
	next.Startup(context.Background(), func() bool { return true })
	waitFor(t, "confirmation", func() bool {
		_, err := os.Stat(next.pendingPath())
		return os.IsNotExist(err)
	})

	time.Sleep(1500 * time.Millisecond)
	if got := h.binContent(t); !strings.Contains(got, "v1.3.1") {
		t.Errorf("binary = %q, want the confirmed release kept", got)
	}
}

func TestStartupDoesNotConfirmUnhealthyPanel(t *testing.T) {
	h := newHarness(t, "v1.3.1", fakeRelease{tag: "v1.3.1"})
	if err := writeJSON(h.u.pendingPath(), pendingMarker{From: "v1.3.0", To: "v1.3.1"}); err != nil {
		t.Fatal(err)
	}

	h.u.confirmAfter = 10 * time.Millisecond
	h.u.Startup(context.Background(), func() bool { return false })
	time.Sleep(100 * time.Millisecond)

	if _, err := os.Stat(h.u.pendingPath()); err != nil {
		t.Error("an unresponsive panel confirmed its own update")
	}
}

func TestRollbackRestoresPreviousAndSkipsCurrent(t *testing.T) {
	h := newHarness(t, "v1.3.1", fakeRelease{tag: "v1.3.1"})
	if err := os.WriteFile(h.bin+".prev", []byte("v1.3.0 binary"), 0755); err != nil {
		t.Fatal(err)
	}
	h.u.settings.Previous = "v1.3.0"

	if got := h.u.State().Previous; got != "v1.3.0" {
		t.Fatalf("previous = %q, want v1.3.0", got)
	}
	if err := h.u.Rollback(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "restart", func() bool { return h.restarts.Load() == 1 })

	if got := h.binContent(t); got != "v1.3.0 binary" {
		t.Errorf("binary = %q, want the previous version", got)
	}
	if h.u.State().Skip != "v1.3.1" {
		t.Error("automatic updates would reinstall the version rolled back from")
	}
}

func TestRollbackWithoutPrevious(t *testing.T) {
	h := newHarness(t, "v1.3.1", fakeRelease{tag: "v1.3.1"})
	h.u.settings.Previous = "v1.3.0" // recorded, but the file is gone

	if err := h.u.Rollback(); err == nil {
		t.Error("rollback succeeded with no previous binary on disk")
	}
}

func TestSetAutoPersists(t *testing.T) {
	h := newHarness(t, "v1.3.0", fakeRelease{tag: "v1.3.0"})
	if err := h.u.SetAuto(false); err != nil {
		t.Fatal(err)
	}

	reloaded := New(Options{Current: "v1.3.0", BinPath: h.bin, DataDir: h.dataDir})
	if reloaded.State().Auto {
		t.Error("automatic updates came back on after a restart")
	}
}

func TestShouldAutoApply(t *testing.T) {
	night := time.Date(2026, 10, 2, 4, 15, 0, 0, time.Local)

	cases := []struct {
		name  string
		setup func(u *Updater)
		at    time.Time
		want  bool
	}{
		{"in the window", func(u *Updater) {}, night, true},
		{"outside the window", func(u *Updater) {}, night.Add(3 * time.Hour), false},
		{"turned off", func(u *Updater) { u.settings.Auto = false }, night, false},
		{"skipped release", func(u *Updater) { u.settings.Skip = "v1.3.1" }, night, false},
		{"failed tonight already", func(u *Updater) { u.attempted["v1.3.1"] = night.Add(-30 * time.Minute) }, night, false},
		{"failed yesterday", func(u *Updater) { u.attempted["v1.3.1"] = night.Add(-24 * time.Hour) }, night, true},
		{"core restarting", func(u *Updater) { u.busy = func() bool { return true } }, night, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, "v1.3.0", fakeRelease{tag: "v1.3.1"})
			if _, err := h.u.Check(context.Background()); err != nil {
				t.Fatal(err)
			}
			c.setup(h.u)
			if got := h.u.shouldAutoApply(c.at); got != c.want {
				t.Errorf("shouldAutoApply = %v, want %v", got, c.want)
			}
		})
	}
}
