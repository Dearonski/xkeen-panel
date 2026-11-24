package infraupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"xkeen-panel/internal/xkeen"
)

const (
	xkeenRepo = "jameszeroX/xkeen"

	checkEvery = 12 * time.Hour
	firstCheck = 3 * time.Minute
	// Both commands download from GitHub through mirrors, and XKeen's own update also runs opkg
	runTimeout = 20 * time.Minute

	crontabPath = "/opt/var/spool/cron/crontabs/root"

	ItemXKeen = "xkeen"
	ItemGeo   = "geo"
)

type Release struct {
	Tag         string    `json:"tag"`
	Notes       string    `json:"notes"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"published_at"`
}

type XKeenState struct {
	Current    string    `json:"current"`
	Latest     *Release  `json:"latest,omitempty"`
	Available  bool      `json:"available"`
	Supported  bool      `json:"supported"`
	Reason     string    `json:"reason,omitempty"`
	Updating   bool      `json:"updating"`
	LastCheck  time.Time `json:"last_check,omitempty"`
	CheckError string    `json:"check_error,omitempty"`
	ApplyError string    `json:"apply_error,omitempty"`
}

type GeoState struct {
	Files      int       `json:"files"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
	Schedule   string    `json:"schedule,omitempty"`
	Supported  bool      `json:"supported"`
	Reason     string    `json:"reason,omitempty"`
	Updating   bool      `json:"updating"`
	ApplyError string    `json:"apply_error,omitempty"`
}

type State struct {
	XKeen XKeenState `json:"xkeen"`
	Geo   GeoState   `json:"geo"`
}

type Options struct {
	Runtime func() xkeen.Runtime
	// Drops the cached runtime, which still names the version XKeen had before it replaced itself
	Refresh func()
	GeoDir  string
	Crontab string
	APIBase string
	Client  *http.Client
	Run     func(dispatcher string, args ...string) (string, error)
	// Called once the core is back, so the pool pin lost in the restart is put back without waiting for the next check
	Settled func()
	Log     func(format string, args ...interface{})
	Publish func(State)
}

type Updater struct {
	runtime func() xkeen.Runtime
	refresh func()
	geoDir  string
	crontab string
	apiBase string
	client  *http.Client
	run     func(dispatcher string, args ...string) (string, error)
	settled func()
	logf    func(format string, args ...interface{})
	publish func(State)

	mu        sync.Mutex
	updating  string
	latest    *Release
	lastCheck time.Time
	checkErr  string
	applyErr  map[string]string
}

func New(opts Options) *Updater {
	u := &Updater{
		runtime:  opts.Runtime,
		refresh:  opts.Refresh,
		geoDir:   opts.GeoDir,
		crontab:  opts.Crontab,
		apiBase:  opts.APIBase,
		client:   opts.Client,
		run:      opts.Run,
		settled:  opts.Settled,
		logf:     opts.Log,
		publish:  opts.Publish,
		applyErr: map[string]string{},
	}
	if u.apiBase == "" {
		u.apiBase = "https://api.github.com"
	}
	if u.client == nil {
		u.client = &http.Client{Timeout: 30 * time.Second}
	}
	if u.crontab == "" {
		u.crontab = crontabPath
	}
	if u.run == nil {
		u.run = func(dispatcher string, args ...string) (string, error) {
			return xkeen.Maintenance(dispatcher, runTimeout, args...)
		}
	}
	if u.refresh == nil {
		u.refresh = func() {}
	}
	if u.settled == nil {
		u.settled = func() {}
	}
	return u
}

func (u *Updater) log(format string, args ...interface{}) {
	if u.logf != nil {
		u.logf(format, args...)
	}
}

func splitVersion(full string) (version, build string) {
	version, build, _ = strings.Cut(strings.TrimSpace(full), " ")
	return version, build
}

func (u *Updater) xkeenSupported() (xkeen.Runtime, string) {
	rt := u.runtime()
	version, build := splitVersion(rt.Version)
	switch {
	case !rt.Installed || rt.Dispatcher == "":
		return rt, "XKeen не найден"
	case rt.Generation != 2:
		return rt, "Из панели обновляется только XKeen 2.x"
	case version == "":
		return rt, "Версия XKeen не определена"
	case build != "" && build != "Stable":
		return rt, "Установлена сборка " + build + " — она обновляется командой xkeen -uk"
	}
	return rt, ""
}

func (u *Updater) geoFiles() (int, time.Time) {
	entries, err := os.ReadDir(u.geoDir)
	if err != nil {
		return 0, time.Time{}
	}
	count, newest := 0, time.Time{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".dat" {
			continue
		}
		info, err := e.Info()
		// XKeen keeps zero-sized stubs under the old file names
		if err != nil || info.Size() < 1024 {
			continue
		}
		count++
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	return count, newest
}

func (u *Updater) State() State {
	// Flags first: a version read before an update finished must not be paired with the flag saying it is over
	u.mu.Lock()
	updating, latest, lastCheck, checkErr := u.updating, u.latest, u.lastCheck, u.checkErr
	xkeenErr, geoErr := u.applyErr[ItemXKeen], u.applyErr[ItemGeo]
	u.mu.Unlock()

	rt, xkeenReason := u.xkeenSupported()
	version, _ := splitVersion(rt.Version)
	files, updatedAt := u.geoFiles()

	geoReason := ""
	switch {
	case !rt.Installed || rt.Dispatcher == "":
		geoReason = "XKeen не найден"
	case rt.Generation != 2:
		geoReason = "Из панели базы обновляются только на XKeen 2.x"
	case files == 0:
		geoReason = "Геобазы не найдены"
	}

	state := State{
		XKeen: XKeenState{
			Current:    version,
			Latest:     latest,
			Supported:  xkeenReason == "",
			Reason:     xkeenReason,
			Updating:   updating == ItemXKeen,
			LastCheck:  lastCheck,
			CheckError: checkErr,
			ApplyError: xkeenErr,
		},
		Geo: GeoState{
			Files:      files,
			UpdatedAt:  updatedAt,
			Schedule:   geoSchedule(u.crontab),
			Supported:  geoReason == "",
			Reason:     geoReason,
			Updating:   updating == ItemGeo,
			ApplyError: geoErr,
		},
	}
	if latest != nil && xkeenReason == "" {
		state.XKeen.Available = newer(version, latest.Tag)
	}
	return state
}

func (u *Updater) notify() {
	if u.publish != nil {
		u.publish(u.State())
	}
}

func (u *Updater) Check(ctx context.Context) (State, error) {
	if _, reason := u.xkeenSupported(); reason != "" {
		return u.State(), fmt.Errorf("%s", reason)
	}

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

func (u *Updater) fetchLatest(ctx context.Context) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/repos/%s/releases/latest", u.apiBase, xkeenRepo), nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("User-Agent", "xkeen-panel")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := u.client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return Release{}, fmt.Errorf("GitHub ограничил запросы (HTTP %d), попробуйте позже", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GitHub ответил HTTP %d", resp.StatusCode)
	}

	var payload struct {
		TagName     string    `json:"tag_name"`
		Body        string    `json:"body"`
		HTMLURL     string    `json:"html_url"`
		PublishedAt time.Time `json:"published_at"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&payload); err != nil {
		return Release{}, fmt.Errorf("ответ GitHub не разобран: %w", err)
	}
	if payload.TagName == "" {
		return Release{}, fmt.Errorf("в ответе GitHub нет версии")
	}
	return Release{Tag: payload.TagName, Notes: payload.Body, URL: payload.HTMLURL, PublishedAt: payload.PublishedAt}, nil
}

func (u *Updater) Apply(item string) error {
	rt, reason := u.runtime(), ""
	switch item {
	case ItemXKeen:
		rt, reason = u.xkeenSupported()
	case ItemGeo:
		if state := u.State().Geo; !state.Supported {
			reason = state.Reason
		}
	default:
		return fmt.Errorf("неизвестный компонент %q", item)
	}
	if reason != "" {
		return fmt.Errorf("%s", reason)
	}

	u.mu.Lock()
	switch {
	case u.updating != "":
		u.mu.Unlock()
		return fmt.Errorf("обновление уже идёт")
	case item == ItemXKeen && u.latest == nil:
		u.mu.Unlock()
		return fmt.Errorf("сначала проверьте наличие новой версии")
	}
	if xkeen.IsRestarting() {
		u.mu.Unlock()
		return fmt.Errorf("ядро сейчас перезапускается — попробуйте через минуту")
	}
	u.updating = item
	delete(u.applyErr, item)
	u.mu.Unlock()
	u.notify()

	go u.runApply(item, rt)
	return nil
}

func (u *Updater) runApply(item string, rt xkeen.Runtime) {
	var err error
	if item == ItemXKeen {
		err = u.updateXKeen(rt)
	} else {
		err = u.updateGeo(rt)
	}

	u.mu.Lock()
	u.updating = ""
	if err != nil {
		u.applyErr[item] = err.Error()
	}
	u.mu.Unlock()

	u.notify()
	u.settled()
}

func (u *Updater) updateXKeen(rt xkeen.Runtime) error {
	from, _ := splitVersion(rt.Version)
	u.log("[XKEEN] Обновляю XKeen (сейчас %s)", from)

	output, err := u.run(rt.Dispatcher, "-uk")
	u.refresh()
	to, _ := splitVersion(u.runtime().Version)

	switch {
	case err != nil:
		err = fmt.Errorf("xkeen -uk завершился с ошибкой: %s", failure(output, err))
	case to == from:
		err = fmt.Errorf("версия XKeen не изменилась: %s", failure(output, nil))
	}
	if err != nil {
		u.log("[XKEEN] Обновление не выполнено: %v", err)
		return err
	}
	u.log("[XKEEN] XKeen обновлён: %s → %s", from, to)
	return nil
}

func (u *Updater) updateGeo(rt xkeen.Runtime) error {
	_, before := u.geoFiles()
	u.log("[GEO] Обновляю геобазы")

	output, err := u.run(rt.Dispatcher, "-ug")
	_, after := u.geoFiles()

	switch {
	case err != nil:
		err = fmt.Errorf("xkeen -ug завершился с ошибкой: %s", failure(output, err))
	case !after.After(before):
		err = fmt.Errorf("файлы баз не изменились: %s", failure(output, nil))
	}
	if err != nil {
		u.log("[GEO] Обновление не выполнено: %v", err)
		return err
	}
	u.log("[GEO] Геобазы обновлены")
	return nil
}

func failure(output string, err error) string {
	if tail := xkeen.TailLines(output, 3); tail != "" {
		return tail
	}
	if err != nil {
		return err.Error()
	}
	return "xkeen ничего не сообщил"
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
		if _, reason := u.xkeenSupported(); reason == "" {
			if _, err := u.Check(ctx); err != nil {
				u.log("[XKEEN] Не удалось проверить новую версию XKeen: %v", err)
			}
		}
		timer.Reset(checkEvery)
	}
}

func parseVersion(s string) ([]int, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if s == "" {
		return nil, false
	}
	var parts []int
	for _, p := range strings.Split(s, ".") {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, false
		}
		parts = append(parts, n)
	}
	return parts, true
}

// XKeen itself treats any tag other than its own as an update; ordering keeps a re-published older tag from being offered
func newer(current, latest string) bool {
	c, ok1 := parseVersion(current)
	l, ok2 := parseVersion(latest)
	if !ok1 || !ok2 {
		return current != "" && latest != "" && current != latest
	}
	for i := 0; i < len(c) || i < len(l); i++ {
		var a, b int
		if i < len(c) {
			a = c[i]
		}
		if i < len(l) {
			b = l[i]
		}
		if a != b {
			return b > a
		}
	}
	return false
}

var weekdays = []string{"по воскресеньям", "по понедельникам", "по вторникам", "по средам", "по четвергам", "по пятницам", "по субботам"}

// geoSchedule describes the cron task `xkeen -ugc` installs, empty when there is none.
func geoSchedule(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 7 || strings.HasPrefix(fields[0], "#") || fields[len(fields)-1] != "-ug" {
			continue
		}
		minute, errM := strconv.Atoi(fields[0])
		hour, errH := strconv.Atoi(fields[1])
		if errM != nil || errH != nil || fields[2] != "*" || fields[3] != "*" {
			return strings.Join(fields[:5], " ")
		}
		at := fmt.Sprintf("%02d:%02d", hour, minute)
		if fields[4] == "*" {
			return "каждый день в " + at
		}
		if day, err := strconv.Atoi(fields[4]); err == nil && day >= 0 && day <= 7 {
			return weekdays[day%7] + " в " + at
		}
		return strings.Join(fields[:5], " ")
	}
	return ""
}
