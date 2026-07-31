package xkeen

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile creates path with its parent directories.
func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// fork2Root builds the layout of a real 2.x install: S05xkeen with a core
// marker, the netfilter hook carrying mode_proxy, and the variable file.
func fork2Root(t *testing.T, core, mode string) string {
	t.Helper()
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "opt/etc/init.d/S05xkeen"), "#!/bin/sh\nname_client=\""+core+"\"\nstart_auto=\"on\"\n")
	writeFile(t, filepath.Join(root, "opt/etc/ndm/netfilter.d/proxy.sh"), "#!/bin/sh\nname_client='"+core+"'\nmode_proxy='"+mode+"'\n")
	writeFile(t, filepath.Join(root, "opt/sbin/xkeen"), "#!/bin/sh\n")
	writeFile(t, filepath.Join(root, "opt/sbin/.xkeen/01_info/01_info_variable.sh"),
		"xkeen_current_version=\"2.0\"\nxkeen_build=\"Stable\"\n")

	return root
}

func TestRuntimeDetectsFork2(t *testing.T) {
	root := fork2Root(t, CoreXray, "Hybrid")
	d := NewDetector(root, "", "", "", "", "", "")

	r := d.Runtime()

	if r.Generation != 2 {
		t.Errorf("Generation = %d, want 2", r.Generation)
	}
	if r.Core != CoreXray {
		t.Errorf("Core = %q, want %q", r.Core, CoreXray)
	}
	if r.Mode != "Hybrid" {
		t.Errorf("Mode = %q, want Hybrid", r.Mode)
	}
	if r.Version != "2.0 Stable" {
		t.Errorf("Version = %q, want \"2.0 Stable\"", r.Version)
	}
	if !r.Installed {
		t.Error("Installed = false, want true")
	}
	if want := filepath.Join(root, "opt/etc/xray/configs/05_routing.json"); r.RoutingFile != want {
		t.Errorf("RoutingFile = %q, want %q", r.RoutingFile, want)
	}
}

// Single-quoted name_client is what the netfilter hook uses; the init script
// uses double quotes. Both must parse.
func TestRuntimeDetectsMihomoCore(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "opt/etc/init.d/S05xkeen"), "name_client='mihomo'\n")

	if got := NewDetector(root, "", "", "", "", "", "").Runtime().Core; got != CoreMihomo {
		t.Errorf("Core = %q, want %q", got, CoreMihomo)
	}
}

func TestRuntimeFallsBackToGeneration1(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "opt/etc/init.d/S24xray"), "#!/bin/sh\n")

	r := NewDetector(root, "", "", "", "", "", "").Runtime()

	if r.Generation != 1 {
		t.Errorf("Generation = %d, want 1", r.Generation)
	}
	if r.Core != CoreXray {
		t.Errorf("Core = %q, want xray on 1.x", r.Core)
	}
}

// A stale init_script from an old config.yaml (S24xray on a 2.x box) must not
// win over the layout actually present on disk.
func TestRuntimeIgnoresStaleInitScriptOverride(t *testing.T) {
	root := fork2Root(t, CoreXray, "TProxy")
	stale := filepath.Join(root, "opt/etc/init.d/S24xray")

	r := NewDetector(root, "", stale, "", "", "", "").Runtime()

	if r.Generation != 2 {
		t.Errorf("Generation = %d, want 2 — stale override should be ignored", r.Generation)
	}
}

func TestRuntimeNotInstalled(t *testing.T) {
	r := NewDetector(t.TempDir(), "", "", "", "", "", "").Runtime()

	if r.Installed {
		t.Error("Installed = true on empty root")
	}
	if r.Mode != "Other" {
		t.Errorf("Mode = %q, want Other", r.Mode)
	}
}

func TestRuntimeOverridesWin(t *testing.T) {
	root := fork2Root(t, CoreXray, "Hybrid")
	confDir := filepath.Join(root, "custom/configs")

	r := NewDetector(root, "", "", confDir, "", "", "").Runtime()

	if r.XrayConfDir != confDir {
		t.Errorf("XrayConfDir = %q, want %q", r.XrayConfDir, confDir)
	}
	if want := filepath.Join(confDir, "05_routing.json"); r.RoutingFile != want {
		t.Errorf("RoutingFile = %q, want %q", r.RoutingFile, want)
	}
}
