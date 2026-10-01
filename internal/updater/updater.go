package updater

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	DefaultRepo = "Dearonski/xkeen-panel"

	checkEvery = 12 * time.Hour
	tickEvery  = 10 * time.Minute

	// The new binary must prove itself before the guard gives up on it
	confirmAfter = 45 * time.Second
	guardDelay   = 2 * time.Minute
)

// Kept in data/ so the UI can change them without rewriting config.yaml and losing its comments.
type Settings struct {
	Auto bool `json:"auto"`

	// Rolled back from or failed to start: automatic updates leave it alone, the button still installs it
	Skip string `json:"skip,omitempty"`

	Previous string `json:"previous,omitempty"`
}

type State struct {
	Current     string    `json:"current"`
	Latest      *Release  `json:"latest,omitempty"`
	Available   bool      `json:"available"`
	Supported   bool      `json:"supported"`
	Reason      string    `json:"reason,omitempty"` // why updates are unavailable on this build
	Auto        bool      `json:"auto"`
	AutoHour    int       `json:"auto_hour"`
	Skip        string    `json:"skip,omitempty"`
	Previous    string    `json:"previous,omitempty"`
	Updating    bool      `json:"updating"`
	LastCheck   time.Time `json:"last_check,omitempty"`
	CheckError  string    `json:"check_error,omitempty"`
	ApplyError  string    `json:"apply_error,omitempty"`
	FailedStart string    `json:"failed_start,omitempty"` // release that did not come up and was rolled back
}

type Options struct {
	Current    string
	BinPath    string
	DataDir    string
	InitScript string
	AutoHour   int
	Repo       string
	APIBase    string
	Client     *http.Client
	Log        func(format string, args ...interface{})
	Publish    func(State)
	Restart    func() error
	Busy       func() bool // the core is restarting; not the moment to restart the panel too
}

type Updater struct {
	current    string
	binPath    string
	dataDir    string
	initScript string
	autoHour   int
	repo       string
	apiBase    string
	asset      string
	client     *http.Client
	logf       func(format string, args ...interface{})
	publish    func(State)
	restart    func() error
	busy       func() bool

	guardDelay   time.Duration
	confirmAfter time.Duration

	mu          sync.Mutex
	settings    Settings
	latest      *Release
	lastCheck   time.Time
	checkErr    string
	applyErr    string
	updating    bool
	failedStart string
	attempted   map[string]time.Time
}

func New(opts Options) *Updater {
	u := &Updater{
		current:    opts.Current,
		binPath:    opts.BinPath,
		dataDir:    opts.DataDir,
		initScript: opts.InitScript,
		autoHour:   opts.AutoHour,
		repo:       opts.Repo,
		apiBase:    opts.APIBase,
		asset:      currentAsset(),
		client:     opts.Client,
		logf:       opts.Log,
		publish:    opts.Publish,
		restart:    opts.Restart,
		busy:       opts.Busy,
		settings:   Settings{Auto: true},
		attempted:  map[string]time.Time{},

		guardDelay:   guardDelay,
		confirmAfter: confirmAfter,
	}
	if u.repo == "" {
		u.repo = DefaultRepo
	}
	if u.apiBase == "" {
		u.apiBase = "https://api.github.com"
	}
	if u.client == nil {
		u.client = &http.Client{Timeout: 5 * time.Minute}
	}

	if err := readJSON(u.settingsPath(), &u.settings); err != nil && !os.IsNotExist(err) {
		u.log("[UPDATE] Настройки обновлений не прочитаны (%v) — автообновление включено", err)
	}

	return u
}

func (u *Updater) settingsPath() string { return filepath.Join(u.dataDir, "update.json") }

func (u *Updater) log(format string, args ...interface{}) {
	if u.logf != nil {
		u.logf(format, args...)
	}
}

func (u *Updater) supported() (bool, string) {
	if _, ok := parseSemver(u.current); !ok {
		return false, "сборка без версии (dev) не обновляется"
	}
	if u.asset == "" {
		return false, "для этой платформы релизы не публикуются"
	}
	if u.binPath == "" {
		return false, "не удалось определить путь к бинарю"
	}
	return true, ""
}

func (u *Updater) State() State {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.stateLocked()
}

func (u *Updater) stateLocked() State {
	supported, reason := u.supported()

	state := State{
		Current:     u.current,
		Latest:      u.latest,
		Supported:   supported,
		Reason:      reason,
		Auto:        u.settings.Auto,
		AutoHour:    u.autoHour,
		Skip:        u.settings.Skip,
		Updating:    u.updating,
		LastCheck:   u.lastCheck,
		CheckError:  u.checkErr,
		ApplyError:  u.applyErr,
		FailedStart: u.failedStart,
	}

	if u.settings.Previous != "" {
		if _, err := os.Stat(u.prevPath()); err == nil {
			state.Previous = u.settings.Previous
		}
	}

	if u.latest != nil {
		cur, okCur := parseSemver(u.current)
		next, okNext := parseSemver(u.latest.Tag)
		state.Available = okCur && okNext && newer(cur, next)
	}

	return state
}

func (u *Updater) notify() {
	if u.publish != nil {
		u.publish(u.State())
	}
}

func (u *Updater) Check(ctx context.Context) (State, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	release, err := u.fetchLatest(ctx)

	u.mu.Lock()
	u.lastCheck = time.Now()
	if err != nil {
		u.checkErr = err.Error()
	} else {
		u.checkErr = ""
		u.latest = &release
	}
	state := u.stateLocked()
	u.mu.Unlock()

	u.notify()

	if err != nil {
		return state, fmt.Errorf("проверка обновлений: %w", err)
	}
	return state, nil
}

func (u *Updater) Apply() error {
	if ok, reason := u.supported(); !ok {
		return fmt.Errorf("обновление недоступно: %s", reason)
	}

	u.mu.Lock()
	if u.updating {
		u.mu.Unlock()
		return fmt.Errorf("обновление уже выполняется")
	}
	state := u.stateLocked()
	if !state.Available {
		u.mu.Unlock()
		return fmt.Errorf("нет более новой версии — сначала проверьте обновления")
	}
	release := *u.latest
	u.updating = true
	u.applyErr = ""
	u.attempted[release.Tag] = time.Now()
	u.mu.Unlock()

	u.notify()
	go u.runApply(release)

	return nil
}

func (u *Updater) runApply(release Release) {
	u.log("[UPDATE] Устанавливаю %s (сейчас %s)", release.Tag, u.current)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if err := u.install(ctx, release); err != nil {
		u.fail(fmt.Errorf("обновление до %s не установлено: %w", release.Tag, err))
		return
	}

	u.mu.Lock()
	u.settings.Previous = u.current
	err := writeJSON(u.settingsPath(), u.settings)
	u.mu.Unlock()
	if err != nil {
		u.log("[UPDATE] Не удалось сохранить настройки: %v", err)
	}

	u.log("[UPDATE] %s установлена, перезапускаю панель", release.Tag)
	if err := u.restart(); err != nil {
		u.fail(fmt.Errorf("новая версия установлена, но панель не перезапустилась: %w — перезапустите её вручную", err))
	}
}

func (u *Updater) fail(err error) {
	u.log("[UPDATE] %v", err)
	u.mu.Lock()
	u.updating = false
	u.applyErr = err.Error()
	u.mu.Unlock()
	u.notify()
}

func (u *Updater) Rollback() error {
	u.mu.Lock()
	if u.updating {
		u.mu.Unlock()
		return fmt.Errorf("обновление уже выполняется")
	}
	previous := u.stateLocked().Previous
	if previous == "" {
		u.mu.Unlock()
		return fmt.Errorf("предыдущей версии нет на диске")
	}
	if err := u.rollbackBinary(); err != nil {
		u.mu.Unlock()
		return err
	}
	u.settings.Skip = u.current
	u.settings.Previous = ""
	u.updating = true
	if err := writeJSON(u.settingsPath(), u.settings); err != nil {
		u.log("[UPDATE] Не удалось сохранить настройки: %v", err)
	}
	u.mu.Unlock()

	u.log("[UPDATE] Откат на %s, %s больше не ставится автоматически", previous, u.current)
	u.notify()

	go func() {
		if err := u.restart(); err != nil {
			u.fail(fmt.Errorf("предыдущая версия возвращена на диск, но панель не перезапустилась: %w", err))
		}
	}()

	return nil
}

// Checks keep running either way, so the UI can still offer an update by hand.
func (u *Updater) SetAuto(auto bool) error {
	u.mu.Lock()
	u.settings.Auto = auto
	err := writeJSON(u.settingsPath(), u.settings)
	u.mu.Unlock()

	u.notify()
	return err
}

func (u *Updater) Startup(ctx context.Context, healthy func() bool) {
	var failed pendingMarker
	if err := readJSON(u.failedPath(), &failed); err == nil {
		os.Remove(u.failedPath())
		u.log("[UPDATE] %s не запустилась и была откачена на %s", failed.To, u.current)

		u.mu.Lock()
		u.failedStart = failed.To
		u.settings.Skip = failed.To
		u.settings.Previous = ""
		if err := writeJSON(u.settingsPath(), u.settings); err != nil {
			u.log("[UPDATE] Не удалось сохранить настройки: %v", err)
		}
		u.mu.Unlock()
	}

	var pending pendingMarker
	if err := readJSON(u.pendingPath(), &pending); err != nil {
		return
	}
	if pending.To != u.current {
		os.Remove(u.pendingPath())
		return
	}

	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(u.confirmAfter):
		}
		if !healthy() {
			u.log("[UPDATE] %s запущена, но не отвечает — откат выполнит сторож", u.current)
			return
		}
		if err := os.Remove(u.pendingPath()); err != nil && !os.IsNotExist(err) {
			u.log("[UPDATE] Не удалось снять маркер обновления: %v", err)
			return
		}
		u.log("[UPDATE] Обновление с %s до %s завершено", pending.From, u.current)
	}()
}

func (u *Updater) Run(ctx context.Context) {
	if ok, reason := u.supported(); !ok {
		u.log("[UPDATE] Автообновление недоступно: %s", reason)
		return
	}

	// Stagger the first check so routers updated together do not all hit GitHub at once
	first := time.Minute + time.Duration(rand.Intn(120))*time.Second
	select {
	case <-ctx.Done():
		return
	case <-time.After(first):
	}

	u.tick(ctx, time.Now())

	ticker := time.NewTicker(tickEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			u.tick(ctx, now)
		}
	}
}

func (u *Updater) tick(ctx context.Context, now time.Time) {
	u.mu.Lock()
	due := now.Sub(u.lastCheck) >= checkEvery
	u.mu.Unlock()

	if due {
		if _, err := u.Check(ctx); err != nil {
			u.log("[UPDATE] %v", err)
			return
		}
	}

	if !u.shouldAutoApply(now) {
		return
	}

	if err := u.Apply(); err != nil {
		u.log("[UPDATE] Автообновление: %v", err)
	}
}

func (u *Updater) shouldAutoApply(now time.Time) bool {
	u.mu.Lock()
	defer u.mu.Unlock()

	state := u.stateLocked()
	if !state.Auto || !state.Available || state.Updating || !state.Supported {
		return false
	}
	if now.Hour() != u.autoHour {
		return false
	}
	if state.Latest.Tag == u.settings.Skip {
		return false
	}
	// A release that failed to install is retried the next night, not every tick
	if at, ok := u.attempted[state.Latest.Tag]; ok && now.Sub(at) < 20*time.Hour {
		return false
	}
	if u.busy != nil && u.busy() {
		return false
	}
	return true
}
