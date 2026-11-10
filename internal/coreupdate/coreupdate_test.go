package coreupdate

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"xkeen-panel/internal/xkeen"
)

func fakeXray(version string, testExit int) string {
	return fmt.Sprintf(`#!/bin/sh
case "$1" in
version) echo "Xray %s (Xray, Penetrates Everything.) abc (go1.27 linux/arm64)" ;;
run) echo "config test result"; exit %d ;;
esac
`, version, testExit)
}

func zipWith(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type fixture struct {
	u        *Updater
	bin      string
	restarts atomic.Int32
	running  atomic.Bool
}

func newFixture(t *testing.T, archive []byte, digest string, withDgst bool) *fixture {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "sbin", "xray")
	confDir := filepath.Join(dir, "etc", "xray", "configs")
	os.MkdirAll(filepath.Dir(bin), 0755)
	os.MkdirAll(confDir, 0755)
	if err := os.WriteFile(bin, []byte(fakeXray("25.1.30", 0)), 0755); err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(archive)
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/repos/XTLS/Xray-core/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		assets := []map[string]string{{
			"name":                 "Xray-linux-arm64-v8a.zip",
			"browser_download_url": srv.URL + "/dl/xray.zip",
			"digest":               digest,
		}}
		if withDgst {
			assets = append(assets, map[string]string{
				"name":                 "Xray-linux-arm64-v8a.zip.dgst",
				"browser_download_url": srv.URL + "/dl/xray.zip.dgst",
			})
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"tag_name": "v26.11.9", "body": "notes", "html_url": "https://example.com", "assets": assets,
		})
	})
	mux.HandleFunc("/dl/xray.zip", func(w http.ResponseWriter, r *http.Request) { w.Write(archive) })
	mux.HandleFunc("/dl/xray.zip.dgst", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "MD5= 00\nSHA2-256= %s\nSHA2-512= 00\n", hex.EncodeToString(sum[:]))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	f := &fixture{bin: bin}
	f.running.Store(true)
	f.u = New(Options{
		Runtime: func() xkeen.Runtime {
			return xkeen.Runtime{Core: xkeen.CoreXray, CoreBin: bin, XrayConfDir: confDir}
		},
		DataDir: filepath.Join(dir, "data"),
		APIBase: srv.URL,
		Restart: func() { f.restarts.Add(1) },
		Running: func(string) bool { return f.running.Load() },
		Model:   func() string { return "" },
		Arch:    "arm64",
	})
	f.u.settleFor = 10 * time.Millisecond
	f.u.startTimeout = 50 * time.Millisecond
	return f
}

func (f *fixture) version(t *testing.T) string {
	t.Helper()
	v, err := binaryVersion(context.Background(), f.bin)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func (f *fixture) apply(t *testing.T) error {
	t.Helper()
	if _, err := f.u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	return f.u.install(context.Background(), *f.u.latest)
}

func goodArchive(t *testing.T) []byte {
	return zipWith(t, map[string]string{"xray": fakeXray("26.11.9", 0), "geoip.dat": "x"})
}

func TestCheckFindsNewerRelease(t *testing.T) {
	f := newFixture(t, goodArchive(t), "", true)

	s, err := f.u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !s.Supported || !s.Available || s.Current != "25.1.30" || s.Latest == nil || s.Latest.Tag != "v26.11.9" {
		t.Fatalf("got %+v", s)
	}
}

func TestInstallSwapsBinaryAndKeepsPrevious(t *testing.T) {
	archive := goodArchive(t)
	sum := sha256.Sum256(archive)
	f := newFixture(t, archive, "sha256:"+hex.EncodeToString(sum[:]), false)

	if err := f.apply(t); err != nil {
		t.Fatal(err)
	}
	if got := f.version(t); got != "26.11.9" {
		t.Fatalf("installed %s", got)
	}
	if _, err := os.Stat(prevPath(f.bin)); err != nil {
		t.Fatal("previous binary not kept")
	}
	if f.restarts.Load() != 1 || f.u.settings.Previous != "25.1.30" {
		t.Fatalf("restarts %d, previous %q", f.restarts.Load(), f.u.settings.Previous)
	}
}

func TestInstallFallsBackToDgst(t *testing.T) {
	f := newFixture(t, goodArchive(t), "", true)
	if err := f.apply(t); err != nil {
		t.Fatal(err)
	}
	if got := f.version(t); got != "26.11.9" {
		t.Fatalf("installed %s", got)
	}
}

func TestInstallRejectsWrongChecksum(t *testing.T) {
	f := newFixture(t, goodArchive(t), "sha256:"+strings.Repeat("0", 64), false)

	if err := f.apply(t); err == nil || !strings.Contains(err.Error(), "контрольная сумма") {
		t.Fatalf("want a checksum error, got %v", err)
	}
	if f.version(t) != "25.1.30" || f.restarts.Load() != 0 {
		t.Fatal("binary changed or core restarted")
	}
}

func TestInstallRefusesCoreThatRejectsConfigs(t *testing.T) {
	archive := zipWith(t, map[string]string{"xray": fakeXray("26.11.9", 23)})
	f := newFixture(t, archive, "", true)

	if err := f.apply(t); err == nil || !strings.Contains(err.Error(), "не принимает текущие конфиги") {
		t.Fatalf("want a config error, got %v", err)
	}
	if f.version(t) != "25.1.30" || f.restarts.Load() != 0 {
		t.Fatal("binary changed or core restarted")
	}
	if _, err := os.Stat(newPath(f.bin)); !os.IsNotExist(err) {
		t.Fatal("staged binary left behind")
	}
}

func TestInstallRollsBackWhenCoreDoesNotStart(t *testing.T) {
	f := newFixture(t, goodArchive(t), "", true)
	f.running.Store(false)

	err := f.apply(t)
	if err == nil || !strings.Contains(err.Error(), "возвращено на 25.1.30") {
		t.Fatalf("want a rollback error, got %v", err)
	}
	if f.version(t) != "25.1.30" || f.restarts.Load() != 2 {
		t.Fatalf("version %s, restarts %d", f.version(t), f.restarts.Load())
	}
}

func TestRollbackRestoresPrevious(t *testing.T) {
	f := newFixture(t, goodArchive(t), "", true)
	if err := f.apply(t); err != nil {
		t.Fatal(err)
	}

	if err := f.u.Rollback(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for f.u.State().Updating && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	s := f.u.State()
	if s.Current != "25.1.30" || s.Previous != "" || f.restarts.Load() != 2 {
		t.Fatalf("got %+v, restarts %d", s, f.restarts.Load())
	}
}

func TestSoftfloatModelsGetSoftfloatBinary(t *testing.T) {
	u := &Updater{arch: "mipsle", model: func() string { return `{"model":"Keenetic Lite","hw_id":"KN-1212"}` }}
	if got := u.binaryMember(); got != "xray_softfloat" {
		t.Fatalf("got %s", got)
	}
	u.model = func() string { return `{"hw_id":"KN-1010"}` }
	if got := u.binaryMember(); got != "xray" {
		t.Fatalf("got %s", got)
	}
}

func TestUnsupportedOnMihomo(t *testing.T) {
	u := New(Options{Runtime: func() xkeen.Runtime { return xkeen.Runtime{Core: xkeen.CoreMihomo} }, DataDir: t.TempDir(), Arch: "arm64"})
	if s := u.State(); s.Supported || s.Reason == "" {
		t.Fatalf("got %+v", s)
	}
}

func TestNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"25.1.30", "v26.11.9", true},
		{"26.11.9", "v26.11.9", false},
		{"26.11.9", "v26.10.31", false},
		{"25.9.11", "v25.10.1", true},
		{"", "v26.11.9", false},
	}
	for _, c := range cases {
		if got := newer(c.current, c.latest); got != c.want {
			t.Errorf("newer(%q, %q) = %v", c.current, c.latest, got)
		}
	}
}
