package xkeen

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Runtime is a snapshot of the XKeen installation on this router.
type Runtime struct {
	Generation  int    `json:"generation"` // 1 = Skrill0 (S24xray), 2 = jameszeroX fork (S05xkeen)
	Dispatcher  string `json:"-"`
	InitScript  string `json:"-"`
	Core        string `json:"core"` // xray | mihomo
	CoreBin     string `json:"-"`    // /opt/sbin/<core>, used for `xray api …`
	Mode        string `json:"mode"` // TProxy | Hybrid | Redirect | Other
	Version     string `json:"version"`
	XrayConfDir string `json:"-"`
	RoutingFile string `json:"-"`
	MihomoConf  string `json:"-"`
	XkeenJSON   string `json:"-"`
	Installed   bool   `json:"installed"`
}

// Detector resolves the XKeen layout. Every field is an optional override from
// config.yaml; empty ones are auto-detected. Root is "" in production and a
// temp dir in tests.
type Detector struct {
	Root        string
	Dispatcher  string
	InitScript  string
	XrayConfDir string
	RoutingFile string
	MihomoConf  string
	XkeenJSON   string

	mu       sync.Mutex
	cached   Runtime
	cachedAt time.Time
	initStat time.Time

	topoMu sync.Mutex
	topo   Topology
	topoAt time.Time
}

const (
	CoreXray   = "xray"
	CoreMihomo = "mihomo"
)

var (
	nameClientRe = regexp.MustCompile(`(?m)^\s*name_client=["']?([a-zA-Z0-9_-]+)["']?`)
	modeProxyRe  = regexp.MustCompile(`(?m)^\s*mode_proxy=["']?([a-zA-Z0-9_-]+)["']?`)
	versionRe    = regexp.MustCompile(`(?m)^\s*xkeen_current_version=["']?([^"'\s]+)["']?`)
	buildRe      = regexp.MustCompile(`(?m)^\s*xkeen_build=["']?([^"'\s]+)["']?`)
	ansiRe       = regexp.MustCompile(`\x1b\[[0-9;]*m`)
)

func NewDetector(root, dispatcher, initScript, xrayConfDir, routingFile, mihomoConf, xkeenJSON string) *Detector {
	return &Detector{
		Root:        root,
		Dispatcher:  dispatcher,
		InitScript:  initScript,
		XrayConfDir: xrayConfDir,
		RoutingFile: routingFile,
		MihomoConf:  mihomoConf,
		XkeenJSON:   xkeenJSON,
	}
}

func (d *Detector) path(p string) string {
	if d.Root == "" {
		return p
	}
	return filepath.Join(d.Root, p)
}

// Runtime returns a cached snapshot. The cache is invalidated when the init
// script changes (that is where `xkeen -xray` / `-mihomo` flips the core) and
// every 30s otherwise, so a router does not pay for an exec on every poll.
func (d *Detector) Runtime() Runtime {
	d.mu.Lock()
	defer d.mu.Unlock()

	init := d.resolveInitScript()
	stat := time.Time{}
	if st, err := os.Stat(init); err == nil {
		stat = st.ModTime()
	}

	fresh := time.Since(d.cachedAt) < 30*time.Second
	if fresh && stat.Equal(d.initStat) && d.cached.InitScript == init {
		return d.cached
	}

	d.cached = d.detect(init)
	d.cachedAt = time.Now()
	d.initStat = stat

	return d.cached
}

// Topology returns the cached config topology. Reading it parses every JSON in
// the core config directory, which is too expensive for a per-poll status call.
func (d *Detector) Topology() Topology {
	d.topoMu.Lock()
	defer d.topoMu.Unlock()

	if time.Since(d.topoAt) < 15*time.Second && d.topo.Mode != "" {
		return d.topo
	}

	d.topo = ReadTopology(d.Runtime())
	d.topoAt = time.Now()

	return d.topo
}

// InvalidateTopology drops the cache after the panel rewrites the config.
func (d *Detector) InvalidateTopology() {
	d.topoMu.Lock()
	d.topoAt = time.Time{}
	d.topoMu.Unlock()
}

// resolveInitScript prefers the explicit override, then the 2.x layout, then 1.x.
func (d *Detector) resolveInitScript() string {
	if d.InitScript != "" {
		if _, err := os.Stat(d.InitScript); err == nil {
			return d.InitScript
		}
	}

	for _, p := range []string{"/opt/etc/init.d/S05xkeen", "/opt/etc/init.d/S24xray"} {
		full := d.path(p)
		if _, err := os.Stat(full); err == nil {
			return full
		}
	}

	return d.path("/opt/etc/init.d/S05xkeen")
}

func (d *Detector) detect(initScript string) Runtime {
	r := Runtime{
		InitScript:  initScript,
		Dispatcher:  d.Dispatcher,
		XrayConfDir: d.XrayConfDir,
		RoutingFile: d.RoutingFile,
		MihomoConf:  d.MihomoConf,
		XkeenJSON:   d.XkeenJSON,
		Core:        CoreXray,
	}

	if r.Dispatcher == "" {
		r.Dispatcher = d.path("/opt/sbin/xkeen")
	}
	if r.XrayConfDir == "" {
		r.XrayConfDir = d.path("/opt/etc/xray/configs")
	}
	if r.RoutingFile == "" {
		r.RoutingFile = filepath.Join(r.XrayConfDir, "05_routing.json")
	}
	if r.MihomoConf == "" {
		r.MihomoConf = d.path("/opt/etc/mihomo/config.yaml")
	}
	if r.XkeenJSON == "" {
		r.XkeenJSON = d.path("/opt/etc/xkeen/xkeen.json")
	}

	if strings.HasSuffix(initScript, "S05xkeen") {
		r.Generation = 2
	} else {
		r.Generation = 1
	}

	initBody, err := os.ReadFile(initScript)
	r.Installed = err == nil
	if err == nil {
		if m := nameClientRe.FindSubmatch(initBody); m != nil {
			r.Core = string(m[1])
		}
	}

	// mode_proxy lives in the netfilter hook XKeen regenerates on every start —
	// the same source `S05xkeen status` reads.
	if hook, err := os.ReadFile(d.path("/opt/etc/ndm/netfilter.d/proxy.sh")); err == nil {
		if m := modeProxyRe.FindSubmatch(hook); m != nil {
			r.Mode = string(m[1])
		}
	}
	if r.Mode == "" {
		r.Mode = "Other"
	}

	// The cores live next to the dispatcher (/opt/sbin), where XKeen installs them
	r.CoreBin = filepath.Join(filepath.Dir(r.Dispatcher), r.Core)

	r.Version = d.detectVersion(r.Dispatcher)

	return r
}

// detectVersion reads the fork's single source of truth (01_info_variable.sh)
// and only falls back to `xkeen -v`, whose output is ANSI-coloured prose.
func (d *Detector) detectVersion(dispatcher string) string {
	varFile := filepath.Join(filepath.Dir(dispatcher), ".xkeen", "01_info", "01_info_variable.sh")
	if body, err := os.ReadFile(varFile); err == nil {
		version := ""
		if m := versionRe.FindSubmatch(body); m != nil {
			version = string(m[1])
		}
		if m := buildRe.FindSubmatch(body); m != nil && version != "" {
			version += " " + string(m[1])
		}
		if version != "" {
			return version
		}
	}

	if _, err := os.Stat(dispatcher); err != nil {
		return ""
	}

	out, err := exec.Command(dispatcher, "-v").Output()
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(ansiRe.ReplaceAll(out, nil)), "\n") {
		_, rest, found := strings.Cut(line, "XKeen ")
		if !found {
			continue
		}
		if before, ok := strings.CutSuffix(rest, ")"); ok {
			rest = before
		}
		if before, _, ok := strings.Cut(rest, "("); ok {
			rest = before
		}
		return strings.TrimSpace(rest)
	}

	return ""
}
