package coreupdate

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"xkeen-panel/internal/xkeen"
)

const (
	repo = "XTLS/Xray-core"

	checkEvery   = 12 * time.Hour
	firstCheck   = 2 * time.Minute
	startTimeout = 30 * time.Second
	// A core that dies on a bad config usually exits within a few seconds of starting
	settleFor = 5 * time.Second
)

type State struct {
	Current    string    `json:"current"`
	Latest     *Release  `json:"latest,omitempty"`
	Available  bool      `json:"available"`
	Supported  bool      `json:"supported"`
	Reason     string    `json:"reason,omitempty"`
	Previous   string    `json:"previous,omitempty"`
	Updating   bool      `json:"updating"`
	Step       string    `json:"step,omitempty"`
	LastCheck  time.Time `json:"last_check,omitempty"`
	CheckError string    `json:"check_error,omitempty"`
	ApplyError string    `json:"apply_error,omitempty"`
}

type Options struct {
	Runtime func() xkeen.Runtime
	DataDir string
	APIBase string
	Client  *http.Client
	// Restarts the core and restores the pool pin, returning once the core is back
	Restart func()
	Busy    func() bool
	Running func(bin string) bool
	Model   func() string
	Arch    string
	Log     func(format string, args ...interface{})
	Publish func(State)
}

type settings struct {
	Previous string `json:"previous,omitempty"`
}

type Updater struct {
	runtime func() xkeen.Runtime
	dataDir string
	apiBase string
	client  *http.Client
	restart func()
	busy    func() bool
	running func(bin string) bool
	model   func() string
	arch    string
	logf    func(format string, args ...interface{})
	publish func(State)

	settleFor    time.Duration
	startTimeout time.Duration

	mu        sync.Mutex
	settings  settings
	current   string
	latest    *Release
	lastCheck time.Time
	checkErr  string
	applyErr  string
	updating  bool
	step      string
}

func New(opts Options) *Updater {
	u := &Updater{
		runtime:      opts.Runtime,
		dataDir:      opts.DataDir,
		apiBase:      opts.APIBase,
		client:       opts.Client,
		restart:      opts.Restart,
		busy:         opts.Busy,
		running:      opts.Running,
		model:        opts.Model,
		arch:         opts.Arch,
		logf:         opts.Log,
		publish:      opts.Publish,
		settleFor:    settleFor,
		startTimeout: startTimeout,
	}
	if u.apiBase == "" {
		u.apiBase = "https://api.github.com"
	}
	if u.client == nil {
		u.client = &http.Client{Timeout: 10 * time.Minute}
	}
	if u.arch == "" {
		u.arch = runtime.GOARCH
	}
	if u.running == nil {
		u.running = processRunning
	}
	if u.model == nil {
		u.model = routerModel
	}
	if u.busy == nil {
		u.busy = func() bool { return false }
	}
	if err := readJSON(u.settingsPath(), &u.settings); err != nil && !os.IsNotExist(err) {
		u.log("[XRAY] Состояние обновлений ядра не прочитано: %v", err)
	}
	return u
}

func (u *Updater) settingsPath() string { return filepath.Join(u.dataDir, "xray-update.json") }

func (u *Updater) log(format string, args ...interface{}) {
	if u.logf != nil {
		u.logf(format, args...)
	}
}

func (u *Updater) supported() (xkeen.Runtime, string) {
	rt := u.runtime()
	switch {
	case rt.Core != xkeen.CoreXray:
		return rt, "Ядро обновляется из панели только для Xray"
	case assetName(u.arch) == "":
		return rt, "Для этой архитектуры роутера нет сборки Xray"
	case rt.CoreBin == "":
		return rt, "Бинарь Xray не найден"
	}
	if _, err := os.Stat(rt.CoreBin); err != nil {
		return rt, "Бинарь Xray не найден: " + rt.CoreBin
	}
	return rt, ""
}

func (u *Updater) State() State {
	u.mu.Lock()
	known := u.current != ""
	u.mu.Unlock()
	if !known {
		u.refreshCurrent()
	}

	u.mu.Lock()
	defer u.mu.Unlock()
	return u.stateLocked()
}

func (u *Updater) stateLocked() State {
	_, reason := u.supported()
	s := State{
		Current:    u.current,
		Latest:     u.latest,
		Supported:  reason == "",
		Reason:     reason,
		Previous:   u.settings.Previous,
		Updating:   u.updating,
		Step:       u.step,
		LastCheck:  u.lastCheck,
		CheckError: u.checkErr,
		ApplyError: u.applyErr,
	}
	if u.latest != nil && u.current != "" {
		s.Available = newer(u.current, u.latest.Tag)
	}
	return s
}

func (u *Updater) notify() {
	if u.publish == nil {
		return
	}
	u.mu.Lock()
	s := u.stateLocked()
	u.mu.Unlock()
	u.publish(s)
}

func (u *Updater) refreshCurrent() {
	rt, reason := u.supported()
	if reason != "" {
		return
	}
	v, err := binaryVersion(context.Background(), rt.CoreBin)
	if err != nil {
		u.log("[XRAY] Не удалось узнать версию ядра: %v", err)
		return
	}
	u.mu.Lock()
	u.current = v
	u.mu.Unlock()
}

func (u *Updater) Check(ctx context.Context) (State, error) {
	if _, reason := u.supported(); reason != "" {
		return u.State(), fmt.Errorf("%s", reason)
	}
	u.refreshCurrent()

	release, err := u.fetchLatest(ctx)

	u.mu.Lock()
	u.lastCheck = time.Now()
	if err != nil {
		u.checkErr = err.Error()
	} else {
		u.checkErr = ""
		u.latest = &release
	}
	u.mu.Unlock()

	u.notify()
	return u.State(), err
}

func (u *Updater) Apply() error {
	u.mu.Lock()
	latest := u.latest
	switch {
	case u.updating:
		u.mu.Unlock()
		return fmt.Errorf("обновление ядра уже идёт")
	case latest == nil:
		u.mu.Unlock()
		return fmt.Errorf("сначала проверьте наличие новой версии")
	}
	if u.busy() {
		u.mu.Unlock()
		return fmt.Errorf("ядро сейчас перезапускается — попробуйте через минуту")
	}
	u.updating, u.applyErr = true, ""
	u.mu.Unlock()
	u.notify()

	go u.runApply(*latest)
	return nil
}

func (u *Updater) runApply(release Release) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	err := u.install(ctx, release)
	u.refreshCurrent()

	u.mu.Lock()
	u.updating, u.step = false, ""
	if err != nil {
		u.applyErr = err.Error()
	}
	u.mu.Unlock()

	if err != nil {
		u.log("[XRAY] Обновление до %s не выполнено: %v", release.Tag, err)
	}
	u.notify()
}

func (u *Updater) setStep(step string) {
	u.mu.Lock()
	u.step = step
	u.mu.Unlock()
	u.notify()
}

func (u *Updater) Rollback() error {
	rt, reason := u.supported()
	if reason != "" {
		return fmt.Errorf("%s", reason)
	}
	if _, err := os.Stat(prevPath(rt.CoreBin)); err != nil {
		return fmt.Errorf("прежней версии нет на диске")
	}

	u.mu.Lock()
	if u.updating {
		u.mu.Unlock()
		return fmt.Errorf("обновление ядра уже идёт")
	}
	u.updating, u.applyErr, u.step = true, "", "Возвращаю прежнюю версию"
	u.mu.Unlock()
	u.notify()

	go u.rollback(rt.CoreBin)
	return nil
}

func (u *Updater) rollback(bin string) {
	u.mu.Lock()
	previous := u.settings.Previous
	u.mu.Unlock()

	err := u.restorePrevious(bin)
	if err == nil {
		u.restart()
		u.log("[XRAY] Ядро возвращено на %s", previous)
		u.mu.Lock()
		u.settings.Previous = ""
		saveErr := writeJSON(u.settingsPath(), u.settings)
		u.mu.Unlock()
		if saveErr != nil {
			u.log("[XRAY] Не удалось сохранить состояние обновлений ядра: %v", saveErr)
		}
	}
	u.refreshCurrent()

	u.mu.Lock()
	u.updating, u.step = false, ""
	if err != nil {
		u.applyErr = err.Error()
	}
	u.mu.Unlock()
	u.notify()
}

func (u *Updater) Run(ctx context.Context) {
	timer := time.NewTimer(firstCheck)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if _, reason := u.supported(); reason == "" {
			if _, err := u.Check(ctx); err != nil {
				u.log("[XRAY] Не удалось проверить новую версию ядра: %v", err)
			}
		}
		timer.Reset(checkEvery)
	}
}
